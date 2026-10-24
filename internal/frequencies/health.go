package frequencies

import "time"

// FrequencyHealth is one frequency as the launcher's health reads it
// (/api/v1/health/detail, docs-fr/34): whether audio arrives, and whether it
// is turned into text. Status alone cannot say it -- "connected" means ffmpeg
// started, and from 16 to 18/10 every frequency read "connected" with nothing
// coming out of ffmpeg. A time that has not happened since start is absent.
type FrequencyHealth struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Status      string `json:"status"` // connecting, connected, failed, stopped; available before the first
	Error       string `json:"error,omitempty"`
	Transcribes bool   `json:"transcribes"`

	BytesLast60s int64      `json:"bytes_last_60s"`
	LastByteAt   *time.Time `json:"last_byte_at,omitempty"`

	// The last transmission cut and sent to the sidecar, and the last that
	// came back as text: audio with no transmission is a quiet frequency,
	// transmissions with no text a deaf transcription.
	LastSegmentAt     *time.Time `json:"last_segment_at,omitempty"`
	LastTranscribedAt *time.Time `json:"last_transcribed_at,omitempty"`
}

// Health returns every frequency's health, in display order. Reads memory
// only: no database, no network.
func (s *Service) Health() []FrequencyHealth {
	freqs := s.GetAllFrequencies()
	out := make([]FrequencyHealth, 0, len(freqs))
	for _, f := range freqs {
		h := FrequencyHealth{ID: f.ID, Name: f.Name, Status: f.Status, Error: f.LastError, Transcribes: f.TranscribeAudio}

		s.streamsMu.RLock()
		sp := s.activeStreams[f.ID]
		s.streamsMu.RUnlock()
		if sp != nil && sp.audioProcessor != nil {
			var last time.Time
			h.BytesLast60s, last = sp.audioProcessor.Flow()
			h.LastByteAt = utcOrNil(last)
		}

		if s.transcriptionManager != nil {
			sent, done := s.transcriptionManager.Activity(f.ID)
			h.LastSegmentAt, h.LastTranscribedAt = utcOrNil(sent), utcOrNil(done)
		}
		out = append(out, h)
	}
	return out
}

func utcOrNil(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	u := t.UTC()
	return &u
}
