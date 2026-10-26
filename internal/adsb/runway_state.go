package adsb

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/yegors/co-atc/pkg/logger"
)

// The runways in use, kept across a restart of the server. The trackers learn
// them from the movements they see, over an hour: after each start, Paris-CDG
// showed one of its two runways in use until the other had been seen again,
// and the procedures drawn on the map were half of them (the owner, 26/10).
// Their evidence is written to a small file every cleanup tick and at stop,
// and read back at start as far as it is still in the window.

type runwayFileContent struct {
	SavedAt  time.Time                     `json:"saved_at"`
	Airports map[string]RunwayTrackerState `json:"airports"`
}

// KeepRunwaysIn has the trackers' evidence kept in path across restarts, and
// takes back what was kept there.
func (tt *TrajectoryTracker) KeepRunwaysIn(path string) {
	tt.runwayMu.Lock()
	tt.runwayFile = path
	tt.runwayMu.Unlock()

	raw, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return
	}
	var c runwayFileContent
	if err == nil {
		err = json.Unmarshal(raw, &c)
	}
	if err != nil {
		tt.logger.Warn("Runway evidence not read", logger.String("path", path), logger.Error(err))
		return
	}
	tt.runwayMu.Lock()
	defer tt.runwayMu.Unlock()
	tt.runwaySaved = map[string]RunwayTrackerState{}
	for airport, st := range c.Airports {
		if rt, ok := tt.runwayTrackers[airport]; ok {
			rt.Restore(st)
		} else {
			// The airports are not known yet: restored when they are.
			tt.runwaySaved[airport] = st
		}
	}
}

// saveRunways writes the trackers' evidence, whole or not at all.
func (tt *TrajectoryTracker) saveRunways() {
	tt.runwayMu.RLock()
	path := tt.runwayFile
	c := runwayFileContent{SavedAt: time.Now().UTC(), Airports: map[string]RunwayTrackerState{}}
	for airport, rt := range tt.runwayTrackers {
		if st := rt.State(); len(st.Events) > 0 {
			c.Airports[airport] = st
		}
	}
	tt.runwayMu.RUnlock()
	if path == "" {
		return
	}
	raw, err := json.Marshal(c)
	if err == nil {
		tmp := path + ".tmp"
		if err = os.MkdirAll(filepath.Dir(path), 0o755); err == nil {
			if err = os.WriteFile(tmp, raw, 0o644); err == nil {
				err = os.Rename(tmp, path)
			}
		}
	}
	if err != nil {
		tt.logger.Warn("Runway evidence not written", logger.String("path", path), logger.Error(err))
	}
}
