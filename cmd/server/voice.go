package main

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/yegors/co-atc/internal/adsb"
	"github.com/yegors/co-atc/internal/storage/sqlite"
	"github.com/yegors/co-atc/pkg/logger"
)

// voiceIndex answers, for a callsign, what the radio has said about it.
//
// The whole table is summarised on a timer rather than queried per aircraft. The
// map is small -- one entry per aircraft voice has named, twenty on a busy
// morning against three hundred in the sky -- and a bulk refresh serves three
// hundred lookups from one scan of the callsign index.
type voiceIndex struct {
	storage  *sqlite.TranscriptionStorage
	logger   *logger.Logger
	interval time.Duration

	mu   sync.RWMutex
	byCS map[string]sqlite.VoiceSummary
}

func newVoiceIndex(ctx context.Context, storage *sqlite.TranscriptionStorage, interval time.Duration, log *logger.Logger) *voiceIndex {
	if interval <= 0 {
		interval = 5 * time.Second
	}
	v := &voiceIndex{
		storage:  storage,
		logger:   log.Named("voice-index"),
		interval: interval,
		byCS:     map[string]sqlite.VoiceSummary{},
	}
	v.refresh()

	go func() {
		t := time.NewTicker(v.interval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				// A tick and the stop can be ready together, and select picks
				// either: the database may already be closed.
				if ctx.Err() != nil {
					return
				}
				v.refresh()
			}
		}
	}()
	return v
}

func (v *voiceIndex) refresh() {
	summaries, err := v.storage.VoiceSummaries()
	if err != nil {
		// A failed refresh keeps the previous map rather than emptying it: stale
		// voice data is better than aircraft flickering out of the filter.
		v.logger.Error("Failed to refresh the voice index", logger.Error(err))
		return
	}
	v.mu.Lock()
	v.byCS = summaries
	v.mu.Unlock()
}

// VoiceFor implements adsb.VoiceIndex.
func (v *voiceIndex) VoiceFor(callsign string) *adsb.VoiceData {
	v.mu.RLock()
	s, ok := v.byCS[strings.TrimSpace(callsign)]
	v.mu.RUnlock()
	if !ok {
		return nil
	}
	return &adsb.VoiceData{
		Transmissions: s.Transmissions,
		LastHeard:     s.LastHeard,
		LastText:      s.LastText,
		LastID:        s.LastID, LastFrequencyID: s.LastFrequencyID,
	}
}
