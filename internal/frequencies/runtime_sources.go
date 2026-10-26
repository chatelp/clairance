package frequencies

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"time"

	"github.com/yegors/co-atc/internal/audio"
	cfg "github.com/yegors/co-atc/internal/config"
	"github.com/yegors/co-atc/internal/transcription/phraseology"
	"github.com/yegors/co-atc/internal/websocket"
)

// Sources added and removed while the server runs.
//
// Upstream reads its frequencies once, from the configuration. A receiving
// station that can publish one stream per channel -- RTLSDR-Airband does, one
// Icecast mount per channel -- changes those streams whenever it changes what
// it listens to, and transcribing each channel on its own instead of a mix of
// them is worth four times the matched callsigns on the station this was
// written for (docs-fr/28-gros-porteurs-par-canal.md).
//
// Following the station is the station's business, as for labels (label.go):
// co-atc exposes the mechanism, and whoever knows what is on the air says so
// through the API. Sources from the configuration are the operator's and cannot
// be replaced or removed this way; added ones last until removed or until the
// server stops, and are not written anywhere.

// ErrConfiguredSource is returned when an added source would replace, or a
// removal would drop, a frequency that comes from the configuration.
var ErrConfiguredSource = errors.New("frequency comes from the configuration")

// ErrNoTranscription is returned when an added source asks for transcription
// and the transcription gate says there is none to be had.
var ErrNoTranscription = errors.New("transcription unavailable")

// ErrUnknownSource is returned when removing a source that was never added.
var ErrUnknownSource = errors.New("no added source with this id")

var sourceID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$`)

func validateSource(fc cfg.FrequencyConfig) error {
	if !sourceID.MatchString(fc.ID) {
		return fmt.Errorf("id %q: letters, digits, '.', '_' and '-' only, 64 characters at most", fc.ID)
	}
	// A network address only: a station says what it publishes, and the
	// operator's own hardware -- a capture device, a file -- stays in the
	// configuration file, under their hand.
	u, err := url.Parse(fc.URL)
	if err != nil || u.Host == "" || !networkSchemes[u.Scheme] {
		return fmt.Errorf("url %q: a network address is required (http, https, srt, udp, rtp, rtsp, rtmp, tcp)", fc.URL)
	}
	if err := audio.SourceProblem(fc.URL, PCMInputOf(fc)); err != nil {
		return err
	}
	if len([]rune(fc.Name)) > 120 {
		return fmt.Errorf("name is %d characters, maximum is 120", len([]rune(fc.Name)))
	}
	if fc.Language != "" && fc.Language != "en" && fc.Language != "fr" {
		return fmt.Errorf("language %q: \"en\", \"fr\" or empty", fc.Language)
	}
	if fc.SectorKind != "" && !cfg.SectorKinds[fc.SectorKind] {
		return fmt.Errorf("sector_kind %q: approach, departure, tower, ground or empty", fc.SectorKind)
	}
	if (fc.SectorKind == "") != (fc.SectorAirport == "") {
		return fmt.Errorf("sector_airport and sector_kind go together")
	}
	if fc.SectorAirport != "" && !sectorAirport.MatchString(fc.SectorAirport) {
		return fmt.Errorf("sector_airport %q: an ICAO code, four letters", fc.SectorAirport)
	}
	return nil
}

var sectorAirport = regexp.MustCompile(`^[A-Z]{4}$`)

var networkSchemes = map[string]bool{
	"http": true, "https": true, "srt": true, "udp": true,
	"rtp": true, "rtsp": true, "rtmp": true, "tcp": true,
}

// sameDisplay also covers the sector: it is read at each match, so a change
// needs no reconnection.
func sameDisplay(a, b cfg.FrequencyConfig) bool {
	return a.Name == b.Name && a.Airport == b.Airport && a.FrequencyMHz == b.FrequencyMHz && a.Order == b.Order &&
		a.SectorAirport == b.SectorAirport && a.SectorKind == b.SectorKind
}

// sameConnection reports that two versions of a source can share one
// connection: only what is displayed differs. Reconnecting costs audio -- the
// server's burst is replayed and the gap is lost -- so a new name or position
// in the list must not cause one.
func sameConnection(a, b cfg.FrequencyConfig) bool {
	return a.ID == b.ID && a.URL == b.URL && a.TranscribeAudio == b.TranscribeAudio &&
		a.Language == b.Language && PCMInputOf(a) == PCMInputOf(b)
}

// PCMInputOf is what a raw PCM source sends, as its configuration says: one
// channel when pcm_channels is not written.
func PCMInputOf(fc cfg.FrequencyConfig) audio.PCMInput {
	in := audio.PCMInput{Rate: fc.PCMRate, Channels: fc.PCMChannels}
	if in.Channels == 0 {
		in.Channels = 1
	}
	return in
}

// AddSource starts a frequency that is not in the configuration. Adding the
// same source again changes nothing, so a caller can repeat its whole list. A
// source added again with a new name or order keeps its connection; with a new
// address, language or transcription setting, it is reconnected.
func (s *Service) AddSource(fc cfg.FrequencyConfig) error {
	if err := validateSource(fc); err != nil {
		return err
	}
	s.changeMu.Lock()
	defer s.changeMu.Unlock()

	s.sourcesMu.RLock()
	old, exists := s.frequenciesConfig[fc.ID]
	added := s.runtime[fc.ID]
	s.sourcesMu.RUnlock()
	if exists && !added {
		return ErrConfiguredSource
	}
	if exists && sameConnection(*old, fc) {
		if !sameDisplay(*old, fc) {
			updated := fc
			s.sourcesMu.Lock()
			s.frequenciesConfig[fc.ID] = &updated
			s.sourcesMu.Unlock()
			s.announceSources()
		}
		return nil
	}
	// Asked before the old version is stopped: a refused source leaves what
	// was running as it was.
	if fc.TranscribeAudio && s.transcriptionGate != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		err := s.transcriptionGate(ctx)
		cancel()
		if err != nil {
			return fmt.Errorf("%w: %v", ErrNoTranscription, err)
		}
	}
	if exists {
		s.removeSource(fc.ID)
	}

	s.transcriptionManager.SetFrequencyLanguage(fc.ID, fc.Language)
	src := fc
	s.sourcesMu.Lock()
	s.frequenciesConfig[fc.ID] = &src
	s.runtime[fc.ID] = true
	s.sourcesMu.Unlock()

	// A source that fails to start stays listed with its failed status, as a
	// configured one does: the caller sees it, and can remove or re-add it.
	s.startSource(&src)
	s.announceSources()
	return nil
}

// RemoveSource stops and forgets a source added by AddSource.
func (s *Service) RemoveSource(id string) error {
	s.changeMu.Lock()
	defer s.changeMu.Unlock()

	s.sourcesMu.RLock()
	_, exists := s.frequenciesConfig[id]
	added := s.runtime[id]
	s.sourcesMu.RUnlock()
	if !exists {
		return ErrUnknownSource
	}
	if !added {
		return ErrConfiguredSource
	}
	s.removeSource(id)
	s.announceSources()
	return nil
}

// removeSource does the work of RemoveSource; changeMu is held by the caller.
func (s *Service) removeSource(id string) {
	s.transcriptionManager.StopTranscription(id)

	s.streamsMu.Lock()
	processor := s.activeStreams[id]
	delete(s.activeStreams, id)
	s.streamsMu.Unlock()
	if processor != nil {
		processor.Stop()
	}

	s.sourcesMu.Lock()
	delete(s.frequenciesConfig, id)
	delete(s.runtime, id)
	s.sourcesMu.Unlock()

	s.transcriptionManager.SetFrequencyLanguage(id, "")
	_ = s.labels.Set(id, "")
	s.statusMu.Lock()
	delete(s.connectionStatus, id)
	s.statusMu.Unlock()
}

// announceSources tells connected pages that the list of frequencies changed,
// so that they fetch it again rather than wait for a reload.
func (s *Service) announceSources() {
	if s.wsServer == nil {
		return
	}
	s.wsServer.Broadcast(&websocket.Message{
		Type: websocket.MessageTypeFrequenciesChanged,
		Data: map[string]interface{}{},
	})
}

// SetTranscriptionGate gives AddSource what to ask before it starts a source
// that transcribes: the server may run without transcription, and a source
// that asks for it there must be refused rather than transcribed by nobody.
// Called before Start.
func (s *Service) SetTranscriptionGate(gate func(context.Context) error) {
	s.transcriptionGate = gate
}

// SetSectors gives transcription the sector of each frequency, read for every
// transmission. Called before Start.
func (s *Service) SetSectors(sectorOf func(frequencyID string) (phraseology.Sector, bool)) {
	s.transcriptionManager.SetSectors(sectorOf)
}

// SetMatchingRules gives transcription the association rules in force, read
// for every transmission. Called before Start.
func (s *Service) SetMatchingRules(rules func() phraseology.Rules) {
	s.transcriptionManager.SetMatchingRules(rules)
}

// SectorOf returns the airport and kind of frequency id serves, empty when it
// names none. Read for every transmission, so it follows sources added and
// changed while the server runs.
func (s *Service) SectorOf(id string) (airport, kind string) {
	s.sourcesMu.RLock()
	defer s.sourcesMu.RUnlock()
	if fc, ok := s.frequenciesConfig[id]; ok {
		return fc.SectorAirport, fc.SectorKind
	}
	return "", ""
}
