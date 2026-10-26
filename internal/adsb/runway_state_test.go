package adsb

import (
	"path/filepath"
	"testing"
	"time"
)

// Paris-CDG on 26/10: 26L and 27R both in use, a restart, and the map showed
// 26L alone for the next hour.
func TestRunwaysInUseOutliveARestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runways-in-use.json")
	ref := PhaseReference{Airport: "LFPG", Lat: 49.0, Lon: 2.55, Runways: cdgRunways()}

	before := newTestTracker(t, ref)
	before.KeepRunwaysIn(path)
	rt := before.runwayTrackerFor("LFPG")
	for i, end := range []string{"08R-26L/26L", "09L-27R/27R", "08R-26L/26L", "09L-27R/27R"} {
		rt.RecordEvent(end, RunwayEventLanding, string(rune('a'+i)))
	}
	inUse := before.RunwaysInUse()
	if inUse == 0 {
		t.Fatal("no runway in use before the restart")
	}
	before.saveRunways()

	after := newTestTracker(t, ref)
	if got := after.RunwaysInUse(); got != 0 {
		t.Fatalf("a new tracker knows %d runways in use, want 0", got)
	}
	after.KeepRunwaysIn(path)
	if got := after.RunwaysInUse(); got != inUse {
		t.Errorf("after the restart: %d runways in use, want %d as before", got, inUse)
	}
	if s := after.GetRunwayScoresFor("LFPG", 2); len(s) != 2 {
		t.Errorf("after the restart: scores %+v, want 26L and 27R", s)
	}
}

// Evidence older than the window is not taken back, and an airport followed
// only after the start gets its own when it is.
func TestRestoredRunwayEvidenceStaysInTheWindow(t *testing.T) {
	rt := newChoiceTracker(t, cdgRunways())
	rt.Restore(RunwayTrackerState{Events: []RunwayEvent{
		{RunwayEnd: "08R-26L/26L", Type: RunwayEventLanding, Hex: "old", Timestamp: time.Now().Add(-2 * time.Hour)},
	}})
	if got := rt.RunwaysInUse(); got != 0 {
		t.Errorf("evidence two hours old: %d runways in use, want 0", got)
	}

	path := filepath.Join(t.TempDir(), "runways-in-use.json")
	tt := newTestTracker(t, PhaseReference{})
	tt.runwayFile = path
	fresh := newChoiceTracker(t, cdgRunways())
	fresh.RecordEvent("09L-27R/27R", RunwayEventLanding, "x")
	tt.runwayTrackers = map[string]*RunwayInUseTracker{"LFPG": fresh}
	tt.saveRunways()

	later := newTestTracker(t, PhaseReference{})
	later.KeepRunwaysIn(path)
	later.ref.storeAll([]PhaseReference{{Airport: "LFPG", Lat: 49.0, Lon: 2.55, Runways: cdgRunways()}})
	later.referenceChanged()
	if got, want := later.RunwaysInUse(), fresh.RunwaysInUse(); got == 0 || got != want {
		t.Errorf("airport followed after the start: %d runways in use, want %d", got, want)
	}
}
