package audio

// Raw PCM over UDP read in Go (udp.go), tested with real datagrams on the
// loopback: what an SDR program sends, cut into packets of any size, mono or
// stereo, through the processor as production would run it.

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/yegors/co-atc/internal/audio/audiotest"
)

// syncBuffer is a bytes.Buffer the decoder writes to while the test reads it.
type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) bytes() []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]byte(nil), s.b.Bytes()...)
}

// sendPCM sends samples (interleaved when channels is 2) to port as raw
// 16-bit little-endian PCM, in datagrams of the given sizes in bytes, taken in
// turn -- odd sizes cut a sample across two datagrams.
func sendPCM(t *testing.T, port int, samples []int16, sizes []int) {
	t.Helper()
	conn, err := net.Dial("udp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	var raw []byte
	for _, s := range samples {
		raw = binary.LittleEndian.AppendUint16(raw, uint16(s))
	}
	for at, i := 0, 0; at < len(raw); i++ {
		n := min(sizes[i%len(sizes)], len(raw)-at)
		if _, err := conn.Write(raw[at : at+n]); err != nil {
			t.Fatal(err)
		}
		at += n
		// The loopback drops what the receiver's buffer cannot hold; a sender
		// paced like a real one loses nothing.
		time.Sleep(200 * time.Microsecond)
	}
}

// receiving starts decodeUDP on a free port and returns the port, the output,
// and a stop that waits for it to end and returns why it did.
func receiving(t *testing.T, in PCMInput, rate int) (int, *syncBuffer, func() error) {
	t.Helper()
	port := audiotest.FreeUDPPort(t)
	ctx, cancel := context.WithCancel(context.Background())
	out := &syncBuffer{}
	ended := make(chan error, 1)
	go func() {
		ended <- decodeUDP(ctx, fmt.Sprintf("udp://127.0.0.1:%d", port), in, rate, out, quietLog(t))
	}()
	// Listening is not instant: wait until the port is taken.
	deadline := time.Now().Add(2 * time.Second)
	for {
		c, err := net.ListenPacket("udp", fmt.Sprintf("127.0.0.1:%d", port))
		if err != nil {
			break
		}
		c.Close()
		if time.Now().After(deadline) {
			t.Fatal("the source never listened")
		}
		time.Sleep(5 * time.Millisecond)
	}
	return port, out, func() error {
		cancel()
		select {
		case err := <-ended:
			return err
		case <-time.After(2 * time.Second):
			t.Fatal("decodeUDP did not end with its context")
			return nil
		}
	}
}

func toneSamples(n, rate int, hz, amp float64) []int16 {
	s := make([]int16, n)
	for i := range s {
		s[i] = toInt16(amp * math.Sin(2*math.Pi*hz*float64(i)/float64(rate)))
	}
	return s
}

// waitBytes waits until out holds n bytes.
func waitBytes(t *testing.T, out *syncBuffer, n int) []byte {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		if b := out.bytes(); len(b) >= n {
			return b
		}
		if time.Now().After(deadline) {
			t.Fatalf("%d bytes out, want %d", len(out.bytes()), n)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// GQRX's format, 48 kHz mono, in datagrams of odd sizes, comes out at 16 kHz
// as the same tone, every sample accounted for.
func TestRawPCMOverUDPComesOutAtTheConfiguredRate(t *testing.T) {
	port, out, stop := receiving(t, PCMInput{Rate: 48000, Channels: 1}, 16000)
	in := toneSamples(48000, 48000, 1000, 0.5) // one second
	sendPCM(t, port, in, []int{1001, 960, 3, 1500})
	// Less the kernel's look-ahead: 32 input samples.
	got := pcm16(waitBytes(t, out, 2*(16000-16)))
	if err := stop(); !errors.Is(err, context.Canceled) {
		t.Errorf("ended with %v, want the context's end", err)
	}
	ideal := toneSamples(len(got), 16000, 1000, 0.5)
	a, b := 1600, len(got)-1600
	if snr := snr16(ideal[a:b], got[a:b], 0); snr < 70 {
		t.Errorf("%.1f dB from the ideal tone, want 70 or more", snr)
	} else {
		t.Logf("48 kHz to 16 kHz over UDP: %.1f dB from the ideal tone, %d samples", snr, len(got))
	}
}

// At the output's own rate nothing is resampled: what is sent is what comes
// out, sample for sample.
func TestRawPCMAtTheOutputRateIsPassedAsItIs(t *testing.T) {
	port, out, stop := receiving(t, PCMInput{Rate: 16000, Channels: 1}, 16000)
	in := toneSamples(8000, 16000, 440, 0.7)
	in[17] = math.MaxInt16
	in[18] = math.MinInt16
	sendPCM(t, port, in, []int{640, 7})
	got := pcm16(waitBytes(t, out, 2*len(in)))
	stop()
	for i := range in {
		if got[i] != in[i] {
			t.Fatalf("sample %d: %d, sent %d", i, got[i], in[i])
		}
	}
}

// Two channels are mixed to one, as ffmpeg's -ac 1 did: their mean.
func TestStereoPCMIsMixedToMono(t *testing.T) {
	port, out, stop := receiving(t, PCMInput{Rate: 16000, Channels: 2}, 16000)
	left := toneSamples(4000, 16000, 440, 0.8)
	var in []int16
	for _, l := range left {
		in = append(in, l, 0) // the right channel silent
	}
	sendPCM(t, port, in, []int{1001, 2})
	got := pcm16(waitBytes(t, out, 2*len(left)))
	stop()
	for i, l := range left {
		if want := float64(l) / 2; math.Abs(float64(got[i])-want) > 1 {
			t.Fatalf("sample %d: %d, want %.1f", i, got[i], want)
		}
	}
}

// A rate or a channel count nothing can mean, or an address with no port, is
// refused with the reason before anything listens.
func TestWhatIsNotARawPCMSource(t *testing.T) {
	for _, c := range []struct {
		url  string
		in   PCMInput
		want string
	}{
		{"udp://127.0.0.1:7355", PCMInput{Rate: 0, Channels: 1}, "pcm_rate"},
		{"udp://127.0.0.1:7355", PCMInput{Rate: 48000, Channels: 6}, "pcm_channels"},
		{"udp://127.0.0.1", PCMInput{Rate: 48000, Channels: 1}, "a port is required"},
		{"http://127.0.0.1:7355", PCMInput{Rate: 48000, Channels: 1}, "not a udp:// address"},
	} {
		err := decodeUDP(context.Background(), c.url, c.in, 16000, io.Discard, quietLog(t))
		if err == nil || !bytes.Contains([]byte(err.Error()), []byte(c.want)) {
			t.Errorf("%s %+v: %v, want %q", c.url, c.in, err, c.want)
		}
	}
}

// The address forms ffmpeg took for an input: a host, every interface, and
// its @ for every interface.
func TestTheAddressesAUDPSourceListensOn(t *testing.T) {
	for raw, want := range map[string]string{
		"udp://127.0.0.1:7355":                  "127.0.0.1:7355",
		"udp://@:7355":                          ":7355",
		"udp://:7355":                           ":7355",
		"UDP://127.0.0.1:7355?fifo_size=100000": "127.0.0.1:7355",
		"udp://239.1.2.3:5004":                  "239.1.2.3:5004",
	} {
		a, err := udpAddr(raw)
		if err != nil {
			t.Errorf("%s: %v", raw, err)
			continue
		}
		if a.String() != want {
			t.Errorf("%s: %s, want %s", raw, a, want)
		}
	}
}

// Through the processor: the tone arrives on both readers, the browser's
// after its WAV header, and stopping ends them and frees the port.
func TestTheProcessorReadsRawPCMOverUDP(t *testing.T) {
	url := audiotest.UDPTone(t, 48000)
	before := runtime.NumGoroutine()
	p := processorFor(t, url, CentralProcessorConfig{
		ReconnectDelay: time.Hour,
		PCM:            PCMInput{Rate: 48000, Channels: 1},
	})
	var st statuses
	p.SetStatusCallback(st.record)
	if err := p.Start(); err != nil {
		t.Fatal(err)
	}
	st.waitFor(t, StatusConnected, 1)
	raw, err := p.CreateRawReader("transcription")
	if err != nil {
		t.Fatal(err)
	}
	wav, err := p.CreateReader("browser")
	if err != nil {
		t.Fatal(err)
	}
	pcm := make([]byte, 3200) // 100 ms at 16 kHz
	if _, err := io.ReadFull(raw, pcm); err != nil {
		t.Fatalf("raw PCM: %v", err)
	}
	if bytes.Count(pcm, []byte{0}) == len(pcm) {
		t.Error("the tone read as silence")
	}
	head := make([]byte, 44)
	if _, err := io.ReadFull(wav, head); err != nil || !bytes.Equal(head, createWAVHeader(16000, 1)) {
		t.Fatalf("the browser's reader should start with the WAV header: %v", err)
	}
	if _, err := io.ReadFull(wav, make([]byte, 3200)); err != nil {
		t.Fatalf("then PCM: %v", err)
	}
	if r, _, ok := p.ULawReader(time.Second); ok {
		r.Close()
		t.Error("raw PCM has no mu-law relay: the page must fall back to WAV")
	}
	if r, ok := p.MP3Reader(time.Second); ok {
		r.Close()
		t.Error("raw PCM has no MP3 to relay")
	}

	if err := p.Stop(); err != nil {
		t.Fatal(err)
	}
	for name, r := range map[string]io.Reader{"raw": raw, "wav": wav} {
		if _, err := r.Read(make([]byte, 64)); !errors.Is(err, io.EOF) {
			t.Errorf("%s reader after Stop: %v, want EOF", name, err)
		}
	}
	deadline := time.Now().Add(3 * time.Second)
	for runtime.NumGoroutine() > before+2 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if n := runtime.NumGoroutine(); n > before+2 {
		t.Errorf("%d goroutines after Stop, %d before Start", n, before)
	}
	// The port is free again: a source restarted listens on it.
	for {
		c, err := net.ListenPacket("udp", url[len("udp://"):])
		if err == nil {
			c.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the port is still held after Stop: %v", err)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// A port another program holds is a failure the processor reports and tries
// again after the delay, as for a stream that answers 404.
func TestAPortAlreadyHeldIsReportedAndRetried(t *testing.T) {
	held, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer held.Close()
	p := processorFor(t, "udp://"+held.LocalAddr().String(), CentralProcessorConfig{
		ReconnectDelay: 50 * time.Millisecond,
		PCM:            PCMInput{Rate: 16000, Channels: 1},
	})
	var st statuses
	p.SetStatusCallback(st.record)
	if err := p.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { p.Stop() })
	st.waitFor(t, StatusFailed, 2)
}
