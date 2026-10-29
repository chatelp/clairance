package audio

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/yegors/co-atc/pkg/logger"
)

func processorFor(t *testing.T, url string, c CentralProcessorConfig) *CentralAudioProcessor {
	t.Helper()
	log, err := logger.New(logger.Config{Level: "error", Format: "console"})
	if err != nil {
		t.Fatal(err)
	}
	if c.SampleRate == 0 {
		c.SampleRate = 16000
	}
	p, err := NewCentralAudioProcessor(context.Background(), "125825", url, c, log)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// statuses collects what a processor reports; the callback runs on its own
// goroutine for each change.
type statuses struct {
	mu  sync.Mutex
	got []ConnectionStatus
}

func (s *statuses) record(_ string, st ConnectionStatus, _ string) {
	s.mu.Lock()
	s.got = append(s.got, st)
	s.mu.Unlock()
}

func (s *statuses) count(st ConnectionStatus) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, g := range s.got {
		if g == st {
			n++
		}
	}
	return n
}

func (s *statuses) waitFor(t *testing.T, st ConnectionStatus, n int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for s.count(st) < n {
		if time.Now().After(deadline) {
			s.mu.Lock()
			defer s.mu.Unlock()
			t.Fatalf("waited for %d %q, got %v", n, st, s.got)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// A source that ends -- a station that stops publishing -- is reported failed,
// then restarted after the reconnect delay, by the processor itself.
func TestASourceThatEndsIsRestartedAfterTheDelay(t *testing.T) {
	b := mp3Fixture(t, "station-8k-vbr")
	srv, _ := slowServer(t, func(int) []byte { return b[:len(b)/20] })
	p := processorFor(t, srv.URL+"/aero-124350.mp3", CentralProcessorConfig{ReconnectDelay: 50 * time.Millisecond})
	var st statuses
	p.SetStatusCallback(st.record)
	if state, _, err := p.GetStatus(); state != "stopped" || err != nil {
		t.Errorf("before Start: %q, %v", state, err)
	}
	if err := p.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { p.Stop() })

	st.waitFor(t, StatusFailed, 1)
	st.waitFor(t, StatusConnected, 2)
}

// What only ffmpeg read -- SRT, a sound card, a file, raw PCM whose rate is
// not given -- is refused at start with the reason, and is not retried: no
// attempt can succeed.
func TestASourceThisVersionDoesNotReadIsRefusedWithTheReason(t *testing.T) {
	for _, c := range []struct {
		url  string
		pcm  PCMInput
		want string
	}{
		{"srt://192.0.2.1:42069", PCMInput{}, "SRT and sound cards are no longer read"},
		{":0", PCMInput{}, "not a source this version reads"},
		{"hw:1", PCMInput{}, "not a source this version reads"},
		{"/var/audio/a.wav", PCMInput{}, "not a source this version reads"},
		{"rtsp://192.0.2.1/live", PCMInput{}, "not a source this version reads"},
		{"udp://127.0.0.1:7355", PCMInput{Channels: 1}, "pcm_rate"},
		{"udp://127.0.0.1", PCMInput{Rate: 48000, Channels: 1}, "a port is required"},
	} {
		p := processorFor(t, c.url, CentralProcessorConfig{PCM: c.pcm, ReconnectDelay: 10 * time.Millisecond})
		var st statuses
		p.SetStatusCallback(st.record)
		err := p.Start()
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: %v, want %q", c.url, err, c.want)
		}
		st.waitFor(t, StatusFailed, 1)
		time.Sleep(50 * time.Millisecond)
		if n := st.count(StatusConnecting); n != 1 {
			t.Errorf("%s: %d attempts, want 1", c.url, n)
		}
		if _, err := p.CreateRawReader("x"); err == nil {
			t.Errorf("%s: a reader on a source that cannot be read", c.url)
		}
	}
	for _, url := range []string{"http://station.invalid/a.mp3", "HTTPS://s1.liveatc.net/lfpg"} {
		if err := SourceProblem(url, PCMInput{}); err != nil {
			t.Errorf("%s: %v", url, err)
		}
	}
	if err := SourceProblem("udp://@:7355", PCMInput{Rate: 48000, Channels: 2}); err != nil {
		t.Errorf("raw PCM over UDP: %v", err)
	}
}

// "connecting" then "connected" in a row: the last change is the one that
// stays, though each callback runs on its own goroutine (126425 stayed
// "connecting" while it played, relance 14, 28/10).
func TestTheLastStatusStays(t *testing.T) {
	for i := 0; i < 200; i++ {
		p := &CentralAudioProcessor{id: "126425"}
		var mu sync.Mutex
		last := ConnectionStatus("")
		calls := 0
		p.statusCallback = func(_ string, st ConnectionStatus, _ string) {
			mu.Lock()
			last = st
			calls++
			mu.Unlock()
		}
		p.notifyStatusChange(StatusConnecting, "")
		p.notifyStatusChange(StatusConnected, "")
		deadline := time.Now().Add(time.Second)
		for {
			mu.Lock()
			l, c := last, calls
			mu.Unlock()
			if l == StatusConnected {
				time.Sleep(time.Millisecond) // a late "connecting" would land now
				mu.Lock()
				l = last
				mu.Unlock()
				if l != StatusConnected {
					t.Fatalf("run %d: %q after %q", i, l, StatusConnected)
				}
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("run %d: last %q after %d calls", i, l, c)
			}
			time.Sleep(50 * time.Microsecond)
		}
	}
}
