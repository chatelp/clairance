package api

import (
	"context"
	"encoding/binary"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/yegors/co-atc/internal/auth"
)

// The page's origin when the audio comes from another port: the stream is
// fetched with the session cookie (audio.crossOrigin = 'use-credentials'),
// and a browser refuses a credentialed answer whose allowed origin is "*".
const pageOrigin = "http://localhost:8000"

// openStream requests the tone on the real port, as the page does, and
// returns the response; the context ends the request.
func (s *stack) openStream(ctx context.Context, id, clientID string) *http.Response {
	s.t.Helper()
	req, err := http.NewRequestWithContext(ctx, "GET", s.srv.URL+"/api/v1/stream/"+id+"?id="+clientID, nil)
	if err != nil {
		s.t.Fatal(err)
	}
	req.Header.Set("Origin", pageOrigin)
	if s.cookie != nil {
		req.AddCookie(s.cookie)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		s.t.Fatalf("GET stream: %v", err)
	}
	return resp
}

// readWAVHeader reads the 44 bytes every stream starts with and checks them
// against the configured decoding: 16 kHz, mono, 16 bits.
func readWAVHeader(t *testing.T, r io.Reader) {
	t.Helper()
	h := make([]byte, 44)
	if _, err := io.ReadFull(r, h); err != nil {
		t.Fatalf("reading the WAV header: %v", err)
	}
	if string(h[0:4]) != "RIFF" || string(h[8:12]) != "WAVE" || string(h[12:16]) != "fmt " || string(h[36:40]) != "data" {
		t.Fatalf("not a WAV header: %q", h)
	}
	if channels := binary.LittleEndian.Uint16(h[22:24]); channels != 1 {
		t.Errorf("channels = %d, want 1", channels)
	}
	if rate := binary.LittleEndian.Uint32(h[24:28]); rate != 16000 {
		t.Errorf("sample rate = %d, want 16000", rate)
	}
	if bits := binary.LittleEndian.Uint16(h[34:36]); bits != 16 {
		t.Errorf("bits per sample = %d, want 16", bits)
	}
}

func expectCORSForThePage(t *testing.T, h http.Header, what string) {
	t.Helper()
	if got := h.Get("Access-Control-Allow-Origin"); got != pageOrigin {
		t.Errorf("%s: Access-Control-Allow-Origin = %q, want the page's origin %q exactly, never *", what, got, pageOrigin)
	}
	if got := h.Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Errorf("%s: Access-Control-Allow-Credentials = %q, want true", what, got)
	}
}

// The stream as the page plays it, on a tone arriving as raw PCM over UDP,
// on one stack.
func TestTheStreamAsThePagePlaysIt(t *testing.T) {
	shared := newStack(t, stackOptions{tone: true})
	shared.signIn()

	// audio/wav, allowed for the page's exact origin with credentials, a WAV
	// header and then audio, for as long as the page listens.
	t.Run("WAV for the page's origin", func(t *testing.T) {
		s := shared.with(t)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		resp := s.openStream(ctx, "tone", "wav-1")
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("stream: %d", resp.StatusCode)
		}
		if ct := resp.Header.Get("Content-Type"); ct != "audio/wav" {
			t.Errorf("Content-Type = %q, want audio/wav", ct)
		}
		if cc := resp.Header.Get("Cache-Control"); !strings.Contains(cc, "no-store") {
			t.Errorf("Cache-Control = %q, want no-store", cc)
		}
		if resp.Header.Get("X-Already-Connected") != "" {
			t.Error("a first request is not 'already connected'")
		}
		expectCORSForThePage(t, resp.Header, "GET stream")
		readWAVHeader(t, resp.Body)
		// A second of audio at 16 kHz mono 16 bits is 32 000 bytes; ask for
		// a quarter of it, which the tone delivers in well under ten seconds.
		if _, err := io.ReadFull(resp.Body, make([]byte, 8000)); err != nil {
			t.Fatalf("reading audio after the header: %v", err)
		}
	})

	// The page asks for mu-law first (www/live-player.js), the MP3 for the
	// <audio> element; raw PCM over UDP has neither, and the WAV comes back
	// at once -- not after the wait given to an MP3 stream that has not sent
	// its first frame yet -- at the configured rate, the page's fallback.
	for _, format := range []string{"ulaw", "mp3", "wav"} {
		t.Run("asked for "+format+", raw PCM answers WAV", func(t *testing.T) {
			s := shared.with(t)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			start := time.Now()
			resp := s.openStream(ctx, "tone", "fallback-"+format+"&format="+format)
			defer resp.Body.Close()
			if ct := resp.Header.Get("Content-Type"); resp.StatusCode != http.StatusOK || ct != "audio/wav" {
				t.Fatalf("?format=%s: %d %q, want 200 audio/wav", format, resp.StatusCode, ct)
			}
			readWAVHeader(t, resp.Body)
			if _, err := io.ReadFull(resp.Body, make([]byte, 3200)); err != nil {
				t.Fatalf("audio after the header: %v", err)
			}
			if d := time.Since(start); d > 3*time.Second {
				t.Errorf("?format=%s: the WAV came after %v", format, d)
			}
		})
	}

	// A page that asks again with its id while its stream is open is told so
	// and given nothing, rather than a second copy of the audio.
	t.Run("a second request with the same id is told it is connected", func(t *testing.T) {
		s := shared.with(t)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		first := s.openStream(ctx, "tone", "same-1")
		defer first.Body.Close()
		readWAVHeader(t, first.Body)

		second := s.openStream(ctx, "tone", "same-1")
		defer second.Body.Close()
		if second.StatusCode != http.StatusOK || second.Header.Get("X-Already-Connected") != "true" {
			t.Errorf("same id while open: %d, X-Already-Connected %q, want 200 and true", second.StatusCode,
				second.Header.Get("X-Already-Connected"))
		}
		if b, _ := io.ReadAll(second.Body); len(b) != 0 {
			t.Errorf("same id while open: %d bytes of body, want none", len(b))
		}
		// Another page, with its own id, gets its own stream.
		other := s.openStream(ctx, "tone", "same-2")
		defer other.Body.Close()
		if other.StatusCode != http.StatusOK || other.Header.Get("X-Already-Connected") != "" {
			t.Errorf("another id: %d, X-Already-Connected %q", other.StatusCode, other.Header.Get("X-Already-Connected"))
		}
		readWAVHeader(t, other.Body)
	})

	// A page that stops listening frees its reader: asking again with the
	// same id a moment later gets a fresh stream, not "already connected".
	t.Run("a client that closes frees its reader", func(t *testing.T) {
		s := shared.with(t)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		first := s.openStream(ctx, "tone", "close-1")
		readWAVHeader(t, first.Body)
		if _, err := io.ReadFull(first.Body, make([]byte, 4000)); err != nil {
			t.Fatal(err)
		}
		cancel() // the page goes away
		first.Body.Close()

		// The handler notices on its next read; the tone keeps those short.
		deadline := time.Now().Add(5 * time.Second)
		for {
			ctx2, cancel2 := context.WithTimeout(context.Background(), 10*time.Second)
			again := s.openStream(ctx2, "tone", "close-1")
			if again.StatusCode == http.StatusOK && again.Header.Get("X-Already-Connected") == "" {
				readWAVHeader(t, again.Body)
				if _, err := io.ReadFull(again.Body, make([]byte, 4000)); err != nil {
					t.Errorf("the fresh stream after a close: %v", err)
				}
				again.Body.Close()
				cancel2()
				return
			}
			again.Body.Close()
			cancel2()
			if time.Now().After(deadline) {
				t.Fatalf("five seconds after the page closed its stream, the same id is still 'already connected' (%d)",
					again.StatusCode)
			}
			time.Sleep(100 * time.Millisecond)
		}
	})
}

// What the stream route answers without opening any audio: HEAD, refusals,
// the preflight.
func TestStreamHeadersAndRefusals(t *testing.T) {
	s := newStack(t, stackOptions{})
	s.signIn()

	// HEAD gives the page the headers to decide with, and no audio.
	rec := s.do("HEAD", "/api/v1/stream/mix", "", func(r *http.Request) { r.Header.Set("Origin", pageOrigin) })
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Type") != "audio/wav" || rec.Body.Len() != 0 {
		t.Errorf("HEAD: %d %q with %d bytes, want 200 audio/wav and nothing", rec.Code, rec.Header().Get("Content-Type"), rec.Body.Len())
	}
	expectCORSForThePage(t, rec.Header(), "HEAD stream")

	if rec := s.do("GET", "/api/v1/stream/nowhere?id=page-1", ""); rec.Code != http.StatusServiceUnavailable {
		t.Errorf("unknown frequency: %d, want 503", rec.Code)
	}
	// The browser's preflight carries no cookie; it is answered before the lock.
	saved := s.cookie
	s.cookie = nil
	rec = s.do("OPTIONS", "/api/v1/stream/mix", "", func(r *http.Request) {
		r.Header.Set("Origin", pageOrigin)
		r.Header.Set("Access-Control-Request-Method", "GET")
	})
	if rec.Code != http.StatusOK {
		t.Errorf("preflight: %d, want 200", rec.Code)
	}
	expectCORSForThePage(t, rec.Header(), "preflight")
	if m := rec.Header().Get("Access-Control-Allow-Methods"); !strings.Contains(m, "GET") {
		t.Errorf("Access-Control-Allow-Methods = %q", m)
	}
	s.cookie = saved
	// Without an Origin there is nothing to allow, and nothing is said.
	rec = s.do("HEAD", "/api/v1/stream/mix", "")
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("no Origin: Access-Control-Allow-Origin = %q, want none", got)
	}
	// And a stream without a session is refused like any data route, cookie
	// or not.
	req := func(r *http.Request) { r.Header.Set("Cookie", auth.CookieName+"=not-a-session") }
	s.cookie = nil
	if rec := s.do("GET", "/api/v1/stream/mix?id=page-1", "", req); rec.Code != http.StatusUnauthorized {
		t.Errorf("a made-up session: %d, want 401", rec.Code)
	}
}
