package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/yegors/co-atc/internal/config"
)

// mp3Server serves the station-shaped recording of third_party/goxiph as an
// endless stream, a frame's worth at a time.
func mp3Server(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "third_party", "goxiph", "mp3", "testdata", "clairance", "station-8k-vbr.mp3"))
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "audio/mpeg")
		for {
			for at := 0; at < len(b); at += 300 {
				if _, err := w.Write(b[at:min(at+300, len(b))]); err != nil {
					return
				}
				w.(http.Flusher).Flush()
				select {
				case <-r.Context().Done():
					return
				case <-time.After(5 * time.Millisecond):
				}
			}
		}
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

// streamHead opens a stream on the real port and returns its content type
// and first bytes, or fails.
func streamHead(t *testing.T, s *stack, path string) (string, []byte) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", s.srv.URL+path, nil)
	req.AddCookie(s.cookie)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s: %d", path, resp.StatusCode)
	}
	head := make([]byte, 64)
	n, _ := resp.Body.Read(head)
	return resp.Header.Get("Content-Type"), head[:n]
}

// The page asks for the station's own MP3 and gets it, frame-aligned, when
// the stream is decoded in Go; without asking, the WAV stream, as before.
// Who may listen does not change: a viewer is refused unless audio_replay
// says otherwise.
func TestTheStationsOwnMP3ForTheBrowser(t *testing.T) {
	s := newStack(t, stackOptions{goMP3: true, viewer: true})
	s.signIn()
	// Wait for the first frames to reach the relay.
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, ok := s.freqs.MP3Stream("station"); ok {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("no MP3 relayed within 5 s")
		}
		time.Sleep(20 * time.Millisecond)
	}

	ct, head := streamHead(t, s, "/api/v1/stream/station?id=page-1&format=mp3")
	if ct != "audio/mpeg" {
		t.Errorf("format=mp3: Content-Type %q, want audio/mpeg", ct)
	}
	if len(head) < 4 || head[0] != 0xFF || head[1]&0xE0 != 0xE0 {
		t.Errorf("format=mp3: starts %x, want a frame header", head)
	}
	if ct, _ := streamHead(t, s, "/api/v1/stream/station?id=page-2"); ct != "audio/wav" {
		t.Errorf("no format: Content-Type %q, want audio/wav", ct)
	}
	// A frequency with no MP3 (here, one whose stream answers 404) falls
	// back to WAV rather than failing: the page asks for MP3 everywhere.
	if _, ok := s.freqs.MP3Stream("mix"); ok {
		t.Error("an MP3 stream for a source that never answered")
	}

	s.signInAs(viewerUser)
	if rec := s.do("GET", "/api/v1/stream/station?format=mp3", ""); rec.Code != http.StatusForbidden {
		t.Errorf("a viewer, audio_replay = owner: %d, want 403", rec.Code)
	}
	s.cfg.Server.AudioReplay = config.AudioReplayAll
	if ct, _ := streamHead(t, s, "/api/v1/stream/station?id=viewer-1&format=mp3"); ct != "audio/mpeg" {
		t.Errorf("a viewer, audio_replay = all: Content-Type %q", ct)
	}
}
