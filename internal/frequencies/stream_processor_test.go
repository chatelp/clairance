package frequencies

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/yegors/co-atc/internal/audio"
	"github.com/yegors/co-atc/internal/audio/audiotest"
	cfg "github.com/yegors/co-atc/internal/config"
	"github.com/yegors/co-atc/pkg/logger"
)

// listedOnly is the address of a source a test lists but never connects.
const listedOnly = "http://station.invalid/aero.mp3"

// toneRate is the rate of the tone the tests listen to: raw PCM over UDP on
// the loopback, in real time (internal/audio/audiotest), the output's own
// rate. It replaced the tone ffmpeg generated (-f lavfi) when ffmpeg went.
const toneRate = 16000

func toneConfig() *cfg.Config {
	c := &cfg.Config{}
	c.Server.Port = 8000
	c.Transcription.SampleRate = toneRate
	c.Frequencies.ReconnectIntervalSecs = 3600
	return c
}

func quietLog(t *testing.T) *logger.Logger {
	t.Helper()
	log, err := logger.New(logger.Config{Level: "error", Format: "console"})
	if err != nil {
		t.Fatal(err)
	}
	return log
}

// newToneProcessor is a started stream processor on the tone.
func newToneProcessor(t *testing.T) *StreamProcessor {
	t.Helper()
	log := quietLog(t)
	sp, err := NewStreamProcessor(context.Background(), "125825", audiotest.UDPTone(t, toneRate),
		audio.PCMInput{Rate: toneRate, Channels: 1}, NewClient(0, log), toneConfig(), log)
	if err != nil {
		t.Fatal(err)
	}
	if err := sp.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(sp.Stop)
	return sp
}

func lastActive(sp *StreamProcessor, id string) (time.Time, bool) {
	sp.clientsMu.RLock()
	defer sp.clientsMu.RUnlock()
	at, ok := sp.clientLastActive[id]
	return at, ok
}

func setLastActive(sp *StreamProcessor, id string, at time.Time) {
	sp.clientsMu.Lock()
	sp.clientLastActive[id] = at
	sp.clientsMu.Unlock()
}

func readSome(t *testing.T, r io.Reader, n int) {
	t.Helper()
	if _, err := io.ReadFull(r, make([]byte, n)); err != nil {
		t.Fatalf("reading %d bytes: %v", n, err)
	}
}

// A browser asking twice with its id gets the reader it has. (Removal by id
// went with RemoveClient, which nothing called: clients leave by the sweep.)
func TestAClientIsAddedOnce(t *testing.T) {
	sp := newToneProcessor(t)
	a := sp.AddClient("page-1")
	if again := sp.AddClient("page-1"); again != a {
		t.Error("an open client asked for again should be the same reader")
	}
	if n := sp.GetClientCount(); n != 1 || !sp.IsClientConnected("page-1") {
		t.Fatalf("%d clients, connected %v", n, sp.IsClientConnected("page-1"))
	}
	readSome(t, a, 44+320) // the WAV header, then audio
}

// The sweep removes a client silent for more than 30 s and one whose reader
// was closed by its handler, and leaves an active one. Time is set, not waited.
func TestTheSweepRemovesInactiveAndClosedClients(t *testing.T) {
	sp := newToneProcessor(t)
	silent := sp.AddClient("silent")
	closed := sp.AddClient("closed")
	sp.AddClient("active")
	setLastActive(sp, "silent", time.Now().Add(-time.Minute))
	closed.Close() // what the HTTP handler's defer does

	sp.removeInactiveClients()
	if n := sp.GetClientCount(); n != 1 || !sp.IsClientConnected("active") {
		t.Fatalf("after the sweep: %d clients, active connected %v", n, sp.IsClientConnected("active"))
	}
	for name, id := range map[string]string{"silent": "silent", "closed": "closed"} {
		if _, ok := lastActive(sp, id); ok {
			t.Errorf("%s client's activity should be forgotten", name)
		}
	}
	if _, err := silent.Read(make([]byte, 64)); !errors.Is(err, io.EOF) {
		t.Errorf("a client removed for silence: %v, want EOF", err)
	}
}

// Reading keeps a client alive; the time is written at most every 5 s, so
// that every read does not take the write lock.
func TestReadingKeepsAClientActive(t *testing.T) {
	sp := newToneProcessor(t)
	r := sp.AddClient("page-1")
	readSome(t, r, 44)

	stale := time.Now().Add(-6 * time.Second)
	setLastActive(sp, "page-1", stale)
	readSome(t, r, 320)
	if at, _ := lastActive(sp, "page-1"); !at.After(stale.Add(5 * time.Second)) {
		t.Errorf("a read 6 s after the last should refresh it, still %v", at)
	}

	recent := time.Now().Add(-2 * time.Second)
	setLastActive(sp, "page-1", recent)
	readSome(t, r, 320)
	if at, _ := lastActive(sp, "page-1"); !at.Equal(recent) {
		t.Errorf("a read 2 s after the last should not write it, got %v", at)
	}
}

// A client whose reader closed can connect again with the same id.
func TestAClosedClientCanConnectAgain(t *testing.T) {
	sp := newToneProcessor(t)
	first := sp.AddClient("page-1")
	first.Close()
	again := sp.AddClient("page-1")
	if again == first || !sp.IsClientConnected("page-1") || sp.GetClientCount() != 1 {
		t.Fatalf("a closed client should be replaced by a new reader")
	}
	readSome(t, again, 44+320)
}

func TestStoppingTheProcessorEndsEveryClient(t *testing.T) {
	sp := newToneProcessor(t)
	a, b := sp.AddClient("a"), sp.AddClient("b")
	sp.Stop()
	if n := sp.GetClientCount(); n != 0 {
		t.Errorf("%d clients after Stop", n)
	}
	for name, r := range map[string]io.Reader{"a": a, "b": b} {
		if _, err := r.Read(make([]byte, 64)); !errors.Is(err, io.EOF) {
			t.Errorf("client %s after Stop: %v, want EOF", name, err)
		}
	}
}

// newToneService is a service whose one configured frequency is the tone, not
// started: GetAudioStream connects it on the first request.
func newToneService(t *testing.T) *Service {
	t.Helper()
	c := toneConfig()
	c.Frequencies.Sources = []cfg.FrequencyConfig{{ID: "125825", Name: "Tone", URL: audiotest.UDPTone(t, toneRate), PCMRate: toneRate}}
	s := NewService(c, quietLog(t), nil, nil, nil, nil)
	t.Cleanup(s.Stop)
	return s
}

// What the stream handler relies on: an unknown frequency is an error, a
// client already listening is told so, and a frequency takes ten listeners.
func TestGetAudioStreamRules(t *testing.T) {
	s := newToneService(t)
	ctx := context.Background()
	if _, _, err := s.GetAudioStream(ctx, "absent", "page-1"); err == nil {
		t.Error("an unknown frequency should be refused")
	}

	r, contentType, err := s.GetAudioStream(ctx, "125825", "page-1")
	if err != nil {
		t.Fatal(err)
	}
	if contentType != "audio/wav" {
		t.Errorf("content type %q", contentType)
	}
	readSome(t, r, 44+320)
	if _, _, err := s.GetAudioStream(ctx, "125825", "page-1"); err == nil || !strings.Contains(err.Error(), "client already connected") {
		t.Errorf("the same client again: %v, want \"client already connected\" (the handler matches on it)", err)
	}

	for i := 2; i <= 10; i++ {
		if _, _, err := s.GetAudioStream(ctx, "125825", fmt.Sprintf("page-%d", i)); err != nil {
			t.Fatalf("listener %d: %v", i, err)
		}
	}
	if _, _, err := s.GetAudioStream(ctx, "125825", "page-11"); err == nil {
		t.Error("an eleventh listener on one frequency should be refused")
	}

	r.Close()
	processorOf(s, "125825").removeInactiveClients()
	if _, _, err := s.GetAudioStream(ctx, "125825", "page-1"); err != nil {
		t.Errorf("a listener who left can come back: %v", err)
	}
}

// The list the page reads: in configured order, with the last reported status
// and error, and a relative stream path on a port taken in turn.
func TestTheFrequencyListCarriesOrderStatusAndPorts(t *testing.T) {
	log := quietLog(t)
	c := toneConfig()
	c.Server.AdditionalPorts = []int{8001}
	c.Frequencies.Sources = []cfg.FrequencyConfig{
		{ID: "b", Name: "Second", URL: listedOnly, Order: 2},
		{ID: "a", Name: "First", URL: listedOnly, Order: 1},
	}
	s := NewService(c, log, nil, nil, nil, nil)
	t.Cleanup(s.Stop)

	s.broadcastFrequencyStatus("b", "failed", "404 Not Found")
	all := s.GetAllFrequencies()
	if len(all) != 2 || all[0].ID != "a" || all[1].ID != "b" {
		t.Fatalf("by order: %v", all)
	}
	if all[0].Status != "available" || all[1].Status != "failed" || all[1].LastError != "404 Not Found" {
		t.Errorf("statuses: %q, %q (%q)", all[0].Status, all[1].Status, all[1].LastError)
	}
	if all[0].StreamURL != "/api/v1/stream/a" {
		t.Errorf("stream path %q", all[0].StreamURL)
	}

	one, _ := s.GetFrequencyByID("a")
	two, _ := s.GetFrequencyByID("a")
	if one.StreamPort == two.StreamPort || (one.StreamPort != 8000 && one.StreamPort != 8001) {
		t.Errorf("ports should alternate between 8000 and 8001: %d then %d", one.StreamPort, two.StreamPort)
	}
	if _, ok := s.GetFrequencyByID("absent"); ok {
		t.Error("an unknown id should not be found")
	}
}
