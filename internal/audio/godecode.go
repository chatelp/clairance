package audio

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/yegors/co-atc/pkg/logger"
	"github.com/yegors/co-atc/third_party/goxiph/audio"
	"github.com/yegors/co-atc/third_party/goxiph/mp3"
)

// Decoding an MP3 stream over HTTP in Go, in place of ffmpeg (docs-fr/35-plan.md,
// B1; Q60): the stream is read, decoded by the copy of goxiph in third_party,
// mixed to mono and brought to the configured rate, and written as the same
// signed 16-bit little-endian PCM ffmpeg wrote, so that everything after it
// is unchanged. Nothing outside the binary is needed. Tested beside ffmpeg
// from 26/10 behind frequencies.decoder = "go", in production for 24 hours
// (N5), then the only decoder (B1 ter), ffmpeg removed.

// goStallAfter is how long a stream may send nothing before it is taken as
// dead. The station's streams send frames through silence too (a frame every
// 72 ms), so a quiet minute is a broken connection, not a quiet frequency.
var goStallAfter = 15 * time.Second

// goResamplerTaps is the half-width of the resampler's kernel.
const goResamplerTaps = 32

var goHTTPClient = &http.Client{Transport: &http.Transport{
	Proxy:                 http.ProxyFromEnvironment,
	DialContext:           (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
	ResponseHeaderTimeout: 15 * time.Second,
	TLSHandshakeTimeout:   10 * time.Second,
}}

// decodeHTTP reads the MP3 stream at url and writes it to w as 16-bit mono PCM
// at rate, until the stream ends, fails, or ctx is done. It returns why it
// stopped; the caller reconnects.
func decodeHTTP(ctx context.Context, url string, rate int, w io.Writer, relay, ulaw *mp3Relay, log *logger.Logger) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Clairance")
	resp, err := goHTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("stream answered HTTP %d", resp.StatusCode)
	}

	// A stream that stops sending without closing is cut here, rather than
	// left to hold the decoder for ever.
	body := &stallReader{r: resp.Body}
	stall := time.AfterFunc(goStallAfter, cancel)
	defer stall.Stop()
	body.onRead = func() { stall.Reset(goStallAfter) }

	return decodeMP3(body, rate, w, relay, ulaw, log)
}

// decodeMP3 decodes a live MP3 stream from r to w, as decodeHTTP describes.
// relay, when not nil, receives the stream's frames as read (mp3relay.go);
// ulaw, the decoded sound in mu-law at the stream's own rate (ulaw.go).
func decodeMP3(r io.Reader, rate int, w io.Writer, relay, ulaw *mp3Relay, log *logger.Logger) error {
	d, fr, err := mp3.NewLiveDecoder(r)
	if err != nil {
		return fmt.Errorf("not an MP3 stream: %w", err)
	}
	format := d.Format()
	if relay != nil {
		relay.setFormat(format.SampleRate, format.Channels)
		fr.OnFrame = relay.frame
	}
	if ulaw != nil {
		ulaw.setFormat(format.SampleRate, 1)
	}
	log.Info("Decoding the stream in Go",
		Int("stream_rate", format.SampleRate), Int("channels", format.Channels), Int("rate", rate))

	buf, err := audio.NewBuffer(format, 0)
	if err != nil {
		return err
	}
	rs := newResampler(format.SampleRate, rate, goResamplerTaps)
	var mono []float64
	var pcm []int16
	var bytesOut []byte
	resyncs := 0
	for {
		if err := d.Next(buf); err != nil {
			if fr.Resyncs > 0 {
				log.Info("Stream resynced on damaged frames", Int("resyncs", fr.Resyncs))
			}
			if errors.Is(err, io.EOF) {
				return io.ErrUnexpectedEOF // a live stream has no end
			}
			return err
		}
		if fr.Resyncs != resyncs {
			log.Debug("Stream resynced", Int("resyncs", fr.Resyncs))
			resyncs = fr.Resyncs
		}
		n := len(buf.Data[0])
		mono = mono[:0]
		for i := 0; i < n; i++ {
			var v float64
			for c := range buf.Data {
				v += float64(buf.Data[c][i])
			}
			mono = append(mono, v/float64(len(buf.Data)))
		}
		if ulaw != nil {
			ulaw.frame(ulawChunk(mono))
		}
		pcm = rs.process(mono, pcm[:0])
		bytesOut = bytesOut[:0]
		for _, s := range pcm {
			bytesOut = binary.LittleEndian.AppendUint16(bytesOut, uint16(s))
		}
		if _, err := w.Write(bytesOut); err != nil {
			return err
		}
	}
}

// stallReader calls onRead after every read that brought bytes.
type stallReader struct {
	r      io.Reader
	onRead func()
}

func (s *stallReader) Read(p []byte) (int, error) {
	n, err := s.r.Read(p)
	if n > 0 && s.onRead != nil {
		s.onRead()
	}
	return n, err
}

// isHTTP reports that a source is a stream over http or https: MP3, decoded
// here.
func isHTTP(audioURL string) bool {
	u := strings.ToLower(audioURL)
	return strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://")
}
