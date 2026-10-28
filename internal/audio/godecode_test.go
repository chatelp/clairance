package audio

// The Go decoding of the stations' MP3 streams (godecode.go), tested as
// production would run it: over HTTP, through the processor, with a
// connection that drops, a stream that stalls, a station that changes what it
// sends. The decoder itself is tested in third_party/goxiph/mp3/live_test.go.

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"math"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yegors/co-atc/pkg/logger"
)

func mp3Fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "third_party", "goxiph", "mp3", "testdata", "clairance", name+".mp3"))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func ffmpegReference(t *testing.T, name string) []int16 {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "third_party", "goxiph", "mp3", "testdata", "clairance", name+".ref.s16le"))
	if err != nil {
		t.Fatal(err)
	}
	return pcm16(b)
}

func pcm16(b []byte) []int16 {
	out := make([]int16, len(b)/2)
	for i := range out {
		out[i] = int16(binary.LittleEndian.Uint16(b[2*i:]))
	}
	return out
}

func quietLog(t *testing.T) *logger.Logger {
	t.Helper()
	log, err := logger.New(logger.Config{Level: "error", Format: "console"})
	if err != nil {
		t.Fatal(err)
	}
	return log
}

// snr16 is the signal-to-noise ratio of b against a, in dB, over their common
// length, b shifted by lag.
func snr16(a, b []int16, lag int) float64 {
	var sig, noise float64
	for i := range a {
		j := i + lag
		if j < 0 || j >= len(b) {
			continue
		}
		d := float64(a[i]) - float64(b[j])
		sig += float64(a[i]) * float64(a[i])
		noise += d * d
	}
	if noise == 0 {
		return math.Inf(1)
	}
	return 10 * math.Log10(sig/noise)
}

// However the input is cut into pieces, the resampler gives the same output.
func TestTheResamplerDoesNotDependOnHowTheInputArrives(t *testing.T) {
	rng := rand.New(rand.NewSource(3))
	in := make([]float64, 40000)
	for i := range in {
		in[i] = 0.6*math.Sin(2*math.Pi*700*float64(i)/8000) + 0.2*(rng.Float64()-0.5)
	}
	for _, to := range []int{16000, 24000, 8000, 11025} {
		whole := newResampler(8000, to, 32).process(in, nil)
		r := newResampler(8000, to, 32)
		var pieces []int16
		for at := 0; at < len(in); {
			n := min(1+rng.Intn(900), len(in)-at)
			pieces = r.process(in[at:at+n], pieces)
			at += n
		}
		if len(pieces) != len(whole) {
			t.Fatalf("to %d Hz: %d samples in pieces, %d at once", to, len(pieces), len(whole))
		}
		for i := range whole {
			if pieces[i] != whole[i] {
				t.Fatalf("to %d Hz: sample %d differs", to, i)
			}
		}
		// Every input sample comes out, less the kernel's look-ahead.
		if want := len(in) * to / 8000; len(whole) < want-64*to/8000 || len(whole) > want {
			t.Errorf("to %d Hz: %d samples, want about %d", to, len(whole), want)
		}
	}
}

// A tone at 8 kHz comes out at 16 and 24 kHz as the same tone, far below what
// a transcription could hear.
func TestTheResamplerKeepsATone(t *testing.T) {
	in := make([]float64, 16000)
	for i := range in {
		in[i] = 0.5 * math.Sin(2*math.Pi*1000*float64(i)/8000)
	}
	for _, to := range []int{16000, 24000} {
		out := newResampler(8000, to, 32).process(in, nil)
		ideal := make([]int16, len(out))
		for i := range ideal {
			ideal[i] = toInt16(0.5 * math.Sin(2*math.Pi*1000*float64(i)/float64(to)))
		}
		// Away from the edges, where the kernel sees beyond the signal.
		a, b := to/10, len(out)-to/10
		if snr := snr16(ideal[a:b], out[a:b], 0); snr < 70 {
			t.Errorf("to %d Hz: %.1f dB from the ideal tone, want 70 or more", to, snr)
		} else {
			t.Logf("to %d Hz: %.1f dB from the ideal tone", to, snr)
		}
	}
}

// The station's format, decoded at its own rate, is ffmpeg's decoding to
// within rounding.
func TestDecodingTheStationsFormatMatchesFFmpeg(t *testing.T) {
	var out bytes.Buffer
	err := decodeMP3(bytes.NewReader(mp3Fixture(t, "station-8k-vbr")), 8000, &out, nil, nil, quietLog(t))
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("ended with %v, want the end of the stream", err)
	}
	got, ref := pcm16(out.Bytes()), ffmpegReference(t, "station-8k-vbr")
	if snr := snr16(ref, got, 0); snr < 90 {
		t.Errorf("%.1f dB from ffmpeg, want 90 or more", snr)
	}
}

// slowServer serves body in small pieces, as a radio stream arrives.
func slowServer(t *testing.T, body func(n int) []byte) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b := body(int(hits.Add(1)))
		if b == nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "audio/mpeg")
		for at := 0; at < len(b); at += 1500 {
			if _, err := w.Write(b[at:min(at+1500, len(b))]); err != nil {
				return
			}
			w.(http.Flusher).Flush()
			select {
			case <-r.Context().Done():
				return
			case <-time.After(2 * time.Millisecond):
			}
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

// Over HTTP: the whole stream is decoded and brought to the asked rate, and
// its end is an error (a live stream has none), so that the caller reconnects.
func TestAStreamOverHTTP(t *testing.T) {
	b := mp3Fixture(t, "station-8k-vbr")
	srv, _ := slowServer(t, func(int) []byte { return b })
	var out bytes.Buffer
	err := decodeHTTP(context.Background(), srv.URL, 24000, &out, nil, nil, quietLog(t))
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("ended with %v", err)
	}
	ref := ffmpegReference(t, "station-8k-vbr")
	if got, want := out.Len()/2, len(ref)*3; got < want-2000 || got > want+2000 {
		t.Errorf("%d samples at 24 kHz, want about %d", got, want)
	}
}

// What a server says instead of a stream ends the attempt with the reason.
func TestWhatIsNotAStream(t *testing.T) {
	notFound := httptest.NewServer(http.NotFoundHandler())
	defer notFound.Close()
	page := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		io.WriteString(w, strings.Repeat("<p>The mount point is not available.</p>\n", 3000))
	}))
	defer page.Close()
	for url, want := range map[string]string{notFound.URL: "HTTP 404", page.URL: "not an MP3 stream", "http://127.0.0.1:1/x.mp3": "connect"} {
		err := decodeHTTP(context.Background(), url, 16000, io.Discard, nil, nil, quietLog(t))
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: %v, want %q", url, err, want)
		}
	}
}

// A stream that stops sending without closing the connection is cut after
// goStallAfter, instead of holding the decoder for ever.
func TestAStalledStreamIsCut(t *testing.T) {
	defer func(d time.Duration) { goStallAfter = d }(goStallAfter)
	goStallAfter = 300 * time.Millisecond
	b := mp3Fixture(t, "station-8k-vbr")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(b[:len(b)/3])
		w.(http.Flusher).Flush()
		<-r.Context().Done() // and then nothing
	}))
	defer srv.Close()
	start := time.Now()
	err := decodeHTTP(context.Background(), srv.URL, 16000, io.Discard, nil, nil, quietLog(t))
	if err == nil {
		t.Fatal("a stalled stream ended without an error")
	}
	if d := time.Since(start); d > 3*time.Second {
		t.Errorf("cut after %v, want about %v", d, goStallAfter)
	}
}

// Through the processor, as production runs it: the audio arrives, the
// connection that drops is taken again after the delay, a station that
// switches from one format to another is followed, and stopping ends the
// readers and the goroutines.
func TestTheProcessorDecodesInGo(t *testing.T) {
	station, other := mp3Fixture(t, "station-8k-vbr"), mp3Fixture(t, "mpeg2-16k-cbr")
	srv, hits := slowServer(t, func(n int) []byte {
		if n%2 == 0 {
			return other // the station switched: another rate on the same mount
		}
		return station[:len(station)/4] // five seconds, then the connection drops
	})
	before := runtime.NumGoroutine()

	p := processorFor(t, srv.URL+"/aero-124350.mp3", CentralProcessorConfig{ReconnectDelay: 50 * time.Millisecond})
	var st statuses
	p.SetStatusCallback(st.record)
	if err := p.Start(); err != nil {
		t.Fatal(err)
	}
	raw, err := p.CreateRawReader("transcription")
	if err != nil {
		t.Fatal(err)
	}
	pcm := make([]byte, 32000) // a second at 16 kHz
	if _, err := io.ReadFull(raw, pcm); err != nil {
		t.Fatalf("raw PCM: %v", err)
	}
	var energy float64
	for _, s := range pcm16(pcm) {
		energy += float64(s) * float64(s)
	}
	if energy == 0 {
		t.Error("the stream read as silence")
	}

	// The connection drops, is taken again, and the other format follows.
	st.waitFor(t, StatusFailed, 2)
	st.waitFor(t, StatusConnected, 3)
	if hits.Load() < 3 {
		t.Errorf("%d connections, want the drops taken again", hits.Load())
	}
	if n, last := p.Flow(); n == 0 || time.Since(last) > 2*time.Second {
		t.Errorf("flow: %d bytes, last %v", n, last)
	}

	if err := p.Stop(); err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Read(make([]byte, 64)); !errors.Is(err, io.EOF) {
		t.Errorf("raw reader after Stop: %v, want EOF", err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for runtime.NumGoroutine() > before+2 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if n := runtime.NumGoroutine(); n > before+2 {
		t.Errorf("%d goroutines after Stop, %d before Start", n, before)
	}
}

// Decodes a recording of a station's stream with the Go path, for the paired
// test of the transcriptions (internal/transcription, paired_decoding_test.go):
//
//	GO_DECODE_IN=runs/b1-decodage/heure/124350.mp3 GO_DECODE_OUT=…/124350.go.s16le \
//	  go test ./internal/audio -run TestDecodeARecordingInGo
//
// Skipped without them; the recordings stay out of the repository.
func TestDecodeARecordingInGo(t *testing.T) {
	in, out := os.Getenv("GO_DECODE_IN"), os.Getenv("GO_DECODE_OUT")
	if in == "" || out == "" {
		t.Skip("GO_DECODE_IN and GO_DECODE_OUT not set")
	}
	rate := 24000
	if r, err := strconv.Atoi(os.Getenv("GO_DECODE_RATE")); err == nil && r > 0 {
		rate = r
	}
	f, err := os.Open(in)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var pcm bytes.Buffer
	if err := decodeMP3(f, rate, &pcm, nil, nil, quietLog(t)); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("ended with %v", err)
	}
	if err := os.WriteFile(out, pcm.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Logf("%s: %.1f s at %d Hz", out, float64(pcm.Len()/2)/float64(rate), rate)
}
