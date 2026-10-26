package audio

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/url"
	"strconv"
	"strings"

	"github.com/yegors/co-atc/pkg/logger"
)

// Raw PCM over UDP, read in Go in place of ffmpeg (docs-fr/35-plan.md, B1
// ter; decided by the owner on 26/10). What GQRX, SDR++ or rtl_fm send is
// signed 16-bit little-endian samples, interleaved when there are two
// channels, with no header: there is nothing to decode, the packet is the
// sound. It is mixed to mono and brought to the configured rate by the
// resampler of the MP3 streams (resample.go), and written as the same PCM.

// PCMInput is what a raw PCM source sends: its rate in Hz and its channels.
// Nothing in the packets says it, so the source's configuration must.
type PCMInput struct {
	Rate     int
	Channels int
}

// Valid says what is wrong with p, or nil.
func (p PCMInput) Valid() error {
	if p.Rate < 1000 || p.Rate > 384000 {
		return fmt.Errorf("pcm_rate %d: the rate the program sends, in Hz (GQRX sends 48000)", p.Rate)
	}
	if p.Channels != 1 && p.Channels != 2 {
		return fmt.Errorf("pcm_channels %d: 1 or 2", p.Channels)
	}
	return nil
}

// isUDP reports that a source is raw PCM over UDP.
func isUDP(audioURL string) bool {
	return strings.HasPrefix(strings.ToLower(audioURL), "udp://")
}

// udpAddr is the address a udp:// URL listens on, host:port. An empty host
// listens on every interface; a multicast group is joined. What ffmpeg read
// in the query (fifo_size, buffer_size) has no use here and is ignored.
func udpAddr(raw string) (*net.UDPAddr, error) {
	u, err := url.Parse(raw)
	if err != nil || !strings.EqualFold(u.Scheme, "udp") {
		return nil, fmt.Errorf("%q: not a udp:// address", raw)
	}
	host := strings.TrimPrefix(u.Hostname(), "@") // ffmpeg's udp://@:7355
	port, err := strconv.Atoi(u.Port())
	if err != nil || port <= 0 || port > 65535 {
		return nil, fmt.Errorf("%q: a port is required, as in udp://127.0.0.1:7355", raw)
	}
	a := &net.UDPAddr{Port: port}
	if host != "" {
		ips, err := net.LookupIP(host)
		if err != nil || len(ips) == 0 {
			return nil, fmt.Errorf("%q: %v", raw, err)
		}
		a.IP = ips[0]
	}
	return a, nil
}

// listenUDP opens the socket a source is received on.
func listenUDP(a *net.UDPAddr) (*net.UDPConn, error) {
	if a.IP != nil && a.IP.IsMulticast() {
		return net.ListenMulticastUDP("udp", nil, a)
	}
	return net.ListenUDP("udp", a)
}

// decodeUDP receives raw PCM at audioURL and writes it to w as 16-bit mono
// PCM at rate, until the socket fails or ctx is done. A sender that stops is
// waited for, as ffmpeg did: rtl_fm sends nothing while its squelch is
// closed, so silence on the socket is not a broken source.
func decodeUDP(ctx context.Context, audioURL string, in PCMInput, rate int, w io.Writer, log *logger.Logger) error {
	if err := in.Valid(); err != nil {
		return err
	}
	a, err := udpAddr(audioURL)
	if err != nil {
		return err
	}
	conn, err := listenUDP(a)
	if err != nil {
		return err
	}
	defer conn.Close()
	stop := context.AfterFunc(ctx, func() { conn.Close() })
	defer stop()

	log.Info("Reading raw PCM over UDP",
		String("address", a.String()), Int("pcm_rate", in.Rate), Int("channels", in.Channels), Int("rate", rate))

	m := newPCMMixer(in, rate)
	buf := make([]byte, 65536) // the largest datagram
	for {
		n, _, err := conn.ReadFrom(buf)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return err
		}
		if out := m.push(buf[:n]); len(out) > 0 {
			if _, err := w.Write(out); err != nil {
				return err
			}
		}
	}
}

// pcmMixer turns raw 16-bit PCM, in packets of any size, into mono PCM at
// the output rate. A sample cut across two packets is kept until the next.
type pcmMixer struct {
	channels int
	rs       *resampler
	carry    []byte
	mono     []float64
	pcm      []int16
	out      []byte
}

func newPCMMixer(in PCMInput, rate int) *pcmMixer {
	return &pcmMixer{channels: in.Channels, rs: newResampler(in.Rate, rate, goResamplerTaps)}
}

// push takes the next bytes and returns the output they complete; the slice
// is reused by the next call.
func (m *pcmMixer) push(b []byte) []byte {
	frame := 2 * m.channels
	m.carry = append(m.carry, b...)
	whole := len(m.carry) - len(m.carry)%frame
	m.mono = m.mono[:0]
	for i := 0; i < whole; i += frame {
		var v float64
		for c := 0; c < m.channels; c++ {
			v += float64(int16(binary.LittleEndian.Uint16(m.carry[i+2*c:])))
		}
		m.mono = append(m.mono, v/float64(m.channels)/32768)
	}
	m.carry = append(m.carry[:0], m.carry[whole:]...)
	m.pcm = m.rs.process(m.mono, m.pcm[:0])
	m.out = m.out[:0]
	for _, s := range m.pcm {
		m.out = binary.LittleEndian.AppendUint16(m.out, uint16(s))
	}
	return m.out
}

// pcmFromFFmpegOptions reads the input options ffmpeg was given for raw PCM
// (["-f", "s16le", "-ar", "48000", "-ac", "1"]): false for anything else,
// which is not raw PCM the Go path reads. Without -ac, one channel, as ffmpeg.
func pcmFromFFmpegOptions(opts []string) (PCMInput, bool) {
	in := PCMInput{Channels: 1}
	format := ""
	if len(opts)%2 != 0 {
		return in, false
	}
	for i := 0; i < len(opts); i += 2 {
		v := opts[i+1]
		var err error
		switch opts[i] {
		case "-f":
			format = v
		case "-ar":
			in.Rate, err = strconv.Atoi(v)
		case "-ac":
			in.Channels, err = strconv.Atoi(v)
		default:
			return in, false
		}
		if err != nil {
			return in, false
		}
	}
	return in, format == "s16le" && in.Valid() == nil
}
