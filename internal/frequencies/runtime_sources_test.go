package frequencies

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	cfg "github.com/yegors/co-atc/internal/config"
	"github.com/yegors/co-atc/pkg/logger"
)

// A service whose streams all answer 404: nothing here needs audio, only the
// bookkeeping of what is connected. Reconnection is slowed to an hour so that no
// restart happens during a test.
func newSourcesService(t *testing.T) (*Service, string) {
	t.Helper()
	gone := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(gone.Close)
	log, err := logger.New(logger.Config{Level: "error", Format: "console"})
	if err != nil {
		t.Fatal(err)
	}
	c := &cfg.Config{}
	c.Server.Port = 8000
	c.Transcription.SampleRate = 16000
	c.Frequencies.ReconnectIntervalSecs = 3600
	c.Frequencies.Sources = []cfg.FrequencyConfig{{ID: "mix", Name: "Configured mix", URL: gone.URL + "/aero.mp3"}}
	s := NewService(c, log, nil, nil, nil, nil)
	t.Cleanup(s.Stop)
	return s, gone.URL
}

func channel(base, id string) cfg.FrequencyConfig {
	return cfg.FrequencyConfig{ID: id, Name: "Channel " + id, FrequencyMHz: 125.825,
		URL: base + "/aero-" + id + ".mp3", Language: "en"}
}

func processorOf(s *Service, id string) *StreamProcessor {
	s.streamsMu.RLock()
	defer s.streamsMu.RUnlock()
	return s.activeStreams[id]
}

func TestAnAddedSourceIsListedThenRemoved(t *testing.T) {
	s, base := newSourcesService(t)
	if err := s.AddSource(channel(base, "125825")); err != nil {
		t.Fatalf("AddSource: %v", err)
	}
	f, ok := s.GetFrequencyByID("125825")
	if !ok || !f.Runtime || f.FrequencyMHz != 125.825 {
		t.Fatalf("the added source should be listed as added: %+v", f)
	}
	if processorOf(s, "125825") == nil {
		t.Error("the added source should be connected")
	}
	if n := len(s.GetAllFrequencies()); n != 2 {
		t.Errorf("configured + added = 2 frequencies, got %d", n)
	}

	if err := s.RemoveSource("125825"); err != nil {
		t.Fatalf("RemoveSource: %v", err)
	}
	if _, ok := s.GetFrequencyByID("125825"); ok {
		t.Error("a removed source should no longer be listed")
	}
	if processorOf(s, "125825") != nil {
		t.Error("a removed source should be disconnected")
	}
}

// A caller that follows a station resends its whole list on every poll; a
// source that has not changed must not be reconnected each time.
func TestAddingTheSameSourceAgainKeepsItsConnection(t *testing.T) {
	s, base := newSourcesService(t)
	_ = s.AddSource(channel(base, "125825"))
	before := processorOf(s, "125825")
	if err := s.AddSource(channel(base, "125825")); err != nil {
		t.Fatalf("AddSource again: %v", err)
	}
	if processorOf(s, "125825") != before {
		t.Error("the same source added twice should keep its connection")
	}
}

// A station that switches groups lists its channels in a new order; the ones
// that stay on the air must keep their connection, or their audio is lost and
// the server's burst replayed.
func TestANewNameOrOrderKeepsTheConnection(t *testing.T) {
	s, base := newSourcesService(t)
	_ = s.AddSource(channel(base, "125825"))
	before := processorOf(s, "125825")
	moved := channel(base, "125825")
	moved.Order, moved.Name = 3, "Renamed"
	if err := s.AddSource(moved); err != nil {
		t.Fatalf("AddSource: %v", err)
	}
	if processorOf(s, "125825") != before {
		t.Error("a new name or order should not reconnect")
	}
	if f, _ := s.GetFrequencyByID("125825"); f.Name != "Renamed" || f.Order != 3 {
		t.Errorf("the listing should show the new name and order: %+v", f)
	}
}

func TestADifferentSourceUnderTheSameIDReplacesIt(t *testing.T) {
	s, base := newSourcesService(t)
	_ = s.AddSource(channel(base, "125825"))
	before := processorOf(s, "125825")
	moved := channel(base, "125825")
	moved.URL = base + "/elsewhere.mp3"
	if err := s.AddSource(moved); err != nil {
		t.Fatalf("AddSource: %v", err)
	}
	if processorOf(s, "125825") == before {
		t.Error("a changed source should be reconnected")
	}
	if f, _ := s.GetFrequencyByID("125825"); f.URL != moved.URL {
		t.Errorf("url = %q, want %q", f.URL, moved.URL)
	}
}

// The configuration is the operator's: nothing from outside replaces or drops it.
func TestConfiguredSourcesAreNotTouchedFromOutside(t *testing.T) {
	s, base := newSourcesService(t)
	if err := s.AddSource(channel(base, "mix")); !errors.Is(err, ErrConfiguredSource) {
		t.Errorf("replacing a configured source: got %v, want ErrConfiguredSource", err)
	}
	if err := s.RemoveSource("mix"); !errors.Is(err, ErrConfiguredSource) {
		t.Errorf("removing a configured source: got %v, want ErrConfiguredSource", err)
	}
	if err := s.RemoveSource("never-added"); !errors.Is(err, ErrUnknownSource) {
		t.Errorf("removing an unknown source: got %v, want ErrUnknownSource", err)
	}
	if f, ok := s.GetFrequencyByID("mix"); !ok || f.Runtime {
		t.Errorf("the configured source should be intact: %+v", f)
	}
}

func TestMalformedSourcesAreRefused(t *testing.T) {
	s, base := newSourcesService(t)
	bad := map[string]func(*cfg.FrequencyConfig){
		"empty id":         func(f *cfg.FrequencyConfig) { f.ID = "" },
		"id with a slash":  func(f *cfg.FrequencyConfig) { f.ID = "a/b" },
		"no url":           func(f *cfg.FrequencyConfig) { f.URL = "" },
		"file url":         func(f *cfg.FrequencyConfig) { f.URL = "file:///etc/passwd" },
		"unknown language": func(f *cfg.FrequencyConfig) { f.Language = "de" },
	}
	for name, spoil := range bad {
		f := channel(base, "125825")
		spoil(&f)
		if err := s.AddSource(f); err == nil {
			t.Errorf("%s: should be refused", name)
		}
	}
	if n := len(s.GetAllFrequencies()); n != 1 {
		t.Errorf("nothing refused should be listed: %d frequencies", n)
	}
}

// A sector is read at each match: changing it keeps the connection.
func TestASectorIsSetAndChangedWithoutReconnecting(t *testing.T) {
	s, base := newSourcesService(t)
	src := channel(base, "123875")
	src.SectorAirport, src.SectorKind = "LFPO", "approach"
	if err := s.AddSource(src); err != nil {
		t.Fatalf("AddSource: %v", err)
	}
	if a, k := s.SectorOf("123875"); a != "LFPO" || k != "approach" {
		t.Errorf("SectorOf = %q, %q", a, k)
	}
	before := processorOf(s, "123875")
	src.SectorKind = "departure"
	if err := s.AddSource(src); err != nil {
		t.Fatalf("AddSource: %v", err)
	}
	if processorOf(s, "123875") != before {
		t.Error("a new sector should not reconnect")
	}
	if _, k := s.SectorOf("123875"); k != "departure" {
		t.Errorf("kind = %q, want departure", k)
	}
	for name, spoil := range map[string]func(*cfg.FrequencyConfig){
		"unknown kind":         func(f *cfg.FrequencyConfig) { f.SectorKind = "cruise" },
		"airport without kind": func(f *cfg.FrequencyConfig) { f.SectorKind = "" },
		"lowercase airport":    func(f *cfg.FrequencyConfig) { f.SectorAirport = "lfpo" },
	} {
		f := src
		spoil(&f)
		if err := s.AddSource(f); err == nil {
			t.Errorf("%s: should be refused", name)
		}
	}
}

// A source asking for transcription is refused when the gate says there is
// none; what was running under its id keeps running, and a source that does
// not transcribe never asks.
func TestATranscribingSourceIsRefusedWithoutTranscription(t *testing.T) {
	s, base := newSourcesService(t)
	asked := 0
	s.SetTranscriptionGate(func(context.Context) error {
		asked++
		return errors.New("no sidecar")
	})

	listen := channel(base, "125825")
	if err := s.AddSource(listen); err != nil {
		t.Fatalf("a source that does not transcribe: %v", err)
	}
	if asked != 0 {
		t.Errorf("a source that does not transcribe asked the gate %d times", asked)
	}
	before := processorOf(s, "125825")

	transcribe := listen
	transcribe.TranscribeAudio = true
	err := s.AddSource(transcribe)
	if !errors.Is(err, ErrNoTranscription) {
		t.Fatalf("the same source asking for transcription: %v, want ErrNoTranscription", err)
	}
	if asked != 1 {
		t.Errorf("the gate was asked %d times, want 1", asked)
	}
	f, ok := s.GetFrequencyByID("125825")
	if !ok || f.TranscribeAudio || processorOf(s, "125825") != before {
		t.Errorf("a refused source must leave the running one as it was: %+v", f)
	}

	other := channel(base, "118700")
	other.TranscribeAudio = true
	if err := s.AddSource(other); !errors.Is(err, ErrNoTranscription) {
		t.Fatalf("a new source asking for transcription: %v, want ErrNoTranscription", err)
	}
	if _, ok := s.GetFrequencyByID("118700"); ok {
		t.Error("a refused new source must not be listed")
	}
}
