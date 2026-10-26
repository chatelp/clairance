// Package adsb provides ADS-B aircraft tracking, trajectory analysis, and flight phase detection.
//
// # Trajectory-Based Phase Detection
//
// The TrajectoryTracker maintains a rolling window of recent ADS-B observations for each
// tracked aircraft. Instead of making phase decisions from a single data point (which is
// fragile near coverage edges), the system analyzes trajectory windows to compute derived
// state vectors—velocity trends, acceleration, altitude slopes, and distance-to-station
// rates—that enable robust, noise-resistant flight phase classification.
//
// Each aircraft's ring buffer holds ~90 seconds of data at 1-second fetch intervals.
// The derived state is recomputed once per fetch cycle using three analysis windows:
//
//   - Short (10s): Current velocity and acceleration. Responds quickly to real changes.
//   - Medium (30s): Trend detection (climbing, descending, turning). Smooths out noise.
//   - Long (full buffer): High-confidence decisions and gap detection.
//
// Key algorithms:
//   - Ordinary Least Squares (OLS) linear regression for altitude/speed/distance trends
//   - Median filtering for vertical rate (resists outlier spikes from bad transponder data)
//   - Circular difference for heading rate (handles 0°/360° wraparound)
//   - Data gap detection to avoid corrupting trends with stale data
//
// The ring buffer is pre-allocated and reuses slots, producing zero GC pressure during
// steady-state operation. Memory cost is ~18 KB per aircraft, ~9 MB for 500 aircraft.

package adsb

import (
	"math"
	"sort"
	"sync"
	"time"

	"github.com/yegors/co-atc/internal/config"
	"github.com/yegors/co-atc/pkg/logger"
)

// ─── Trajectory Snapshot ──────────────────────────────────────────────────────

// TrajectorySnapshot is a single ADS-B observation stored in the ring buffer.
// Fields are a flattened subset of ADSBTarget, with a proper time.Time timestamp
// and a validity flag. Invalid snapshots (no position data) are stored but excluded
// from derived state computations.
type TrajectorySnapshot struct {
	Timestamp time.Time // UTC wall-clock time when this observation was ingested
	// PosTimestamp is when the receiver last decoded the position (Timestamp
	// minus seen_pos), zero when the source does not say. A poll repeats the
	// previous fix whenever no position was decoded since -- about half the
	// polls at one per second against a receiver giving a position every ~1.7 s
	// (measured 13/10/2026) -- so the live prediction extrapolates from this
	// time, not from the time of the poll, or it stays anchored on the old fix.
	PosTimestamp time.Time
	Lat          float64 // Latitude (degrees)
	Lon          float64 // Longitude (degrees)
	AltBaro      float64 // Barometric altitude (feet)
	AltGeom      float64 // Geometric altitude (feet)
	GS           float64 // Ground speed (knots)
	TAS          float64 // True airspeed (knots)
	IAS          float64 // Indicated airspeed (knots)
	Track        float64 // Track angle (degrees, 0-360)
	// TrackUnknown: the receiver gave no track (a Mode S position without
	// velocity), so Track's 0 is not north. Nothing is predicted from it.
	TrackUnknown bool
	MagHeading   float64 // Magnetic heading (degrees)
	TrueHeading  float64 // True heading (degrees)
	BaroRate     float64 // Barometric vertical rate (feet per minute)
	GeomRate     float64 // Geometric vertical rate (feet per minute)
	Roll         float64 // Roll angle (degrees)
	TrackRate    float64 // Rate of change of track (degrees per second)
	OnGround     bool    // Whether the aircraft is on the ground
	NavAltMCP    float64 // MCP (Mode Control Panel) altitude setting (feet)
	NavAltFMS    float64 // FMS (Flight Management System) altitude setting (feet)
	Seen         float64 // Seconds since last ADS-B message (data freshness)
	Valid        bool    // False if key fields are missing (no position)
}

// TrajectorySnapshotFromADSB converts an ADSBTarget and ground state into a
// TrajectorySnapshot suitable for the ring buffer. The snapshot is marked invalid
// when both lat and lon are zero (no position data available). ts is when the
// server ingested it; dataAt is the time the source's seen_pos counts from
// (ReceiverTime), ts when the source does not say.
func TrajectorySnapshotFromADSB(adsb *ADSBTarget, onGround bool, ts, dataAt time.Time) TrajectorySnapshot {
	if adsb == nil {
		return TrajectorySnapshot{Timestamp: ts, Valid: false}
	}
	lat, lon, hasPosition := adsb.Position()
	if dataAt.IsZero() {
		dataAt = ts
	}
	posTs := dataAt
	if hasPosition && adsb.SeenPos != nil && *adsb.SeenPos > 0 {
		posTs = dataAt.Add(-time.Duration(*adsb.SeenPos * float64(time.Second)))
	}
	return TrajectorySnapshot{
		Timestamp:    ts,
		PosTimestamp: posTs,
		Lat:          lat,
		Lon:          lon,
		AltBaro:      adsb.AltBaro.Float64(),
		AltGeom:      adsb.AltGeom.Float64(),
		GS:           NumberOrZero(adsb.GS),
		TAS:          NumberOrZero(adsb.TAS),
		IAS:          NumberOrZero(adsb.IAS),
		Track:        NumberOrZero(adsb.Track),
		TrackUnknown: adsb.Track == nil,
		MagHeading:   NumberOrZero(adsb.MagHeading),
		TrueHeading:  NumberOrZero(adsb.TrueHeading),
		BaroRate:     NumberOrZero(adsb.BaroRate),
		GeomRate:     NumberOrZero(adsb.GeomRate),
		Roll:         NumberOrZero(adsb.Roll),
		TrackRate:    NumberOrZero(adsb.TrackRate),
		OnGround:     onGround,
		NavAltMCP:    NumberOrZero(adsb.NavAltitudeMCP),
		NavAltFMS:    NumberOrZero(adsb.NavAltitudeFMS),
		Seen:         NumberOrZero(adsb.Seen),
		Valid:        hasPosition,
	}
}

// ReceiverTime is the time a target's seen and seen_pos count from: the
// "now" of the aircraft.json a receiver writes itself (tar1090, readsb),
// which it writes once a second and the server reads up to a second later;
// else, and never later than, the server's own now. Counted from the server's
// now, every position was dated up to a second too recent, and drawn that
// much behind (replay bench, 0.5 s: 115 m in cruise).
func ReceiverTime(raw *RawAircraftData, sourceType string, serverNow time.Time) time.Time {
	switch sourceType {
	case SourceTypeTar1090, SourceTypeReadsbAPI, SourceTypeReadsbFile:
	default:
		return serverNow // the external sources set "now" to the server's own second
	}
	if raw == nil || raw.Now <= 0 {
		return serverNow
	}
	at := time.Unix(0, int64(raw.Now*float64(time.Second))).UTC()
	if at.After(serverNow) {
		return serverNow
	}
	return at
}

// PositionTime is when the snapshot's position was decoded: PosTimestamp when
// the source gave it, else the time of the poll.
func (s *TrajectorySnapshot) PositionTime() time.Time {
	if !s.PosTimestamp.IsZero() {
		return s.PosTimestamp
	}
	return s.Timestamp
}

// ─── Derived State ────────────────────────────────────────────────────────────

// DerivedState holds quantities computed from the trajectory window. It is
// recalculated once per fetch cycle when new data arrives (DirtyDerived flag).
// Phase rules operate on these derived values rather than raw instantaneous readings.
type DerivedState struct {
	// Smoothed current state (short window median/mean)
	GroundSpeedKts  float64 // Smoothed ground speed (knots)
	VerticalRateFPM float64 // Smoothed barometric vertical rate (feet per minute)
	TrackDeg        float64 // Smoothed track angle (degrees)

	// Acceleration (rate of change over short window)
	GSAccelKtsPerSec   float64 // d(GS)/dt — positive = accelerating
	VRAccelFPMPerSec   float64 // d(VerticalRate)/dt — positive = increasing climb
	TrackRateDegPerSec float64 // Heading rate of change (degrees per second)

	// Altitude statistics (medium window)
	AltMean     float64 // Mean barometric altitude (feet)
	AltMin      float64 // Minimum barometric altitude (feet)
	AltMax      float64 // Maximum barometric altitude (feet)
	AltTrendFPM float64 // OLS regression slope of altitude vs time, converted to fpm

	// Speed statistics (medium window)
	GSMean           float64 // Mean ground speed (knots)
	GSMin            float64 // Minimum ground speed (knots)
	GSMax            float64 // Maximum ground speed (knots)
	GSTrendKtsPerSec float64 // OLS regression slope of GS vs time (knots per second)

	// Vertical rate statistics (medium window)
	VRMean   float64 // Mean vertical rate (fpm)
	VRStdDev float64 // Standard deviation of vertical rate

	// Distance and bearing to the reference airport -- see PhaseReference. Upstream
	// measured these from the receiver, which is only the airport when the two
	// coincide.
	DistToAirportNM   float64 // Current distance to the reference airport (nautical miles)
	DistTrendNMPerSec float64 // OLS regression slope of distance vs time (negative = approaching)
	BearingToAirport  float64 // Current bearing to the reference airport (degrees)

	// Airport is the followed airport these were measured against, the one the
	// aircraft is flying to or from (airportFor); ref is that airport, for the
	// rules that need its runways.
	Airport string
	ref     *PhaseReference

	// Data quality indicators
	ValidPointCount   int     // Number of valid snapshots in the analysis window
	WindowDurationSec float64 // Time span from oldest to newest valid snapshot (seconds)
	DataGapDetected   bool    // True if any gap > 2× fetch interval found in the window

	// Boolean flags derived from trends (set by ComputeDerivedState)
	IsDescending         bool // AltTrendFPM below descent threshold AND VRMean < 0
	IsClimbing           bool // AltTrendFPM above climb threshold AND VRMean > 0
	IsLevel              bool // (AltMax - AltMin) within level band over medium window
	IsDecelerating       bool // GSTrendKtsPerSec below deceleration threshold
	IsAccelerating       bool // GSTrendKtsPerSec above acceleration threshold
	IsTurning            bool // |TrackRateDegPerSec| above turning threshold
	IsApproachingAirport bool // DistTrendNMPerSec < 0 (closing on the reference airport)

	ComputedAt time.Time // When this derived state was last computed
}

// ─── Per-Aircraft Trajectory ──────────────────────────────────────────────────

// AircraftTrajectory holds the ring buffer and derived state for a single aircraft.
// The ring buffer is a fixed-capacity slice that overwrites the oldest entry when full,
// avoiding allocations during steady-state operation.
type AircraftTrajectory struct {
	Hex          string               // ICAO hex identifier
	Snapshots    []TrajectorySnapshot // Ring buffer (fixed capacity)
	WriteIdx     int                  // Next write position
	Count        int                  // Number of entries stored (up to capacity)
	Derived      DerivedState         // Latest derived state
	Prediction   TrajectoryPrediction // Hindcast + forecast predictions
	DirtyDerived bool                 // True when new snapshots have been added since last compute
	LastSeen     time.Time            // Tracks staleness for cleanup

	// airport is the followed airport on whose runway axis the aircraft was
	// last seen, "" until then (airportFor).
	airport string
}

// NewAircraftTrajectory creates a ring buffer with the given capacity.
func NewAircraftTrajectory(hex string, capacity int) *AircraftTrajectory {
	return &AircraftTrajectory{
		Hex:       hex,
		Snapshots: make([]TrajectorySnapshot, capacity),
	}
}

// AddSnapshot writes a snapshot to the ring buffer, advancing the write index.
func (at *AircraftTrajectory) AddSnapshot(snap TrajectorySnapshot) {
	at.Snapshots[at.WriteIdx] = snap
	at.WriteIdx = (at.WriteIdx + 1) % cap(at.Snapshots)
	if at.Count < cap(at.Snapshots) {
		at.Count++
	}
	at.DirtyDerived = true
	at.LastSeen = snap.Timestamp
}

// ForEachSnapshot iterates over all stored snapshots from oldest to newest,
// calling fn for each. This avoids allocating a temporary slice.
func (at *AircraftTrajectory) ForEachSnapshot(fn func(snap *TrajectorySnapshot)) {
	capacity := cap(at.Snapshots)
	startIdx := 0
	if at.Count == capacity {
		startIdx = at.WriteIdx // In a full buffer, write position holds the oldest entry
	}
	for i := 0; i < at.Count; i++ {
		idx := (startIdx + i) % capacity
		fn(&at.Snapshots[idx])
	}
}

// Latest returns a pointer to the most recently added snapshot, or nil if empty.
func (at *AircraftTrajectory) Latest() *TrajectorySnapshot {
	if at.Count == 0 {
		return nil
	}
	idx := (at.WriteIdx - 1 + cap(at.Snapshots)) % cap(at.Snapshots)
	return &at.Snapshots[idx]
}

// ─── Trajectory Tracker (top-level) ───────────────────────────────────────────

// TrajectoryConfig holds tunable parameters for the trajectory system.
type TrajectoryConfig struct {
	BufferDurationSec    int // How many seconds of history to keep (default: 90)
	BufferCapacity       int // Max snapshots per aircraft (computed from duration / fetch interval + margin)
	FetchIntervalSec     int // ADS-B fetch interval in seconds (from ADSBConfig)
	MinPointsForAnalysis int // Minimum valid points before full trajectory analysis (default: 5)
	StaleTimeoutSec      int // Remove aircraft not seen for this long (default: 300)
	CleanupIntervalSec   int // How often to run the cleanup goroutine (default: 30)

	// Thresholds for derived boolean flags
	DescentVRThresholdFPM   float64 // AltTrend below this = descending (default: -200)
	ClimbVRThresholdFPM     float64 // AltTrend above this = climbing (default: 200)
	LevelAltBandFt          float64 // (AltMax-AltMin) within this = level (default: 200)
	TurningRateThresholdDeg float64 // |TrackRate| above this = turning (default: 1.5)
	DecelerationThreshold   float64 // GSTrend below this = decelerating (default: -0.5)
	AccelerationThreshold   float64 // GSTrend above this = accelerating (default: 0.5)
}

// TrajectoryTracker manages trajectory buffers for all tracked aircraft.
// It is safe for concurrent use: the fetch goroutine calls Ingest and DeterminePhase
// on the same goroutine, while a background cleanup goroutine periodically purges
// stale entries under a write lock.
type TrajectoryTracker struct {
	mu           sync.RWMutex
	aircraft     map[string]*AircraftTrajectory
	config       TrajectoryConfig
	ref          *phaseRef // the airports phases are judged against, shared with the service
	phasesConfig *config.FlightPhasesConfig
	logger       *logger.Logger

	// One runway-in-use tracker per followed airport: runway ends are named
	// without their airport (LFPO and LFPB both have 07-25), and the evidence of
	// one airport says nothing of another's.
	runwayMu       sync.RWMutex
	runwayTrackers map[string]*RunwayInUseTracker
	newRunways     func() *RunwayInUseTracker
	runwayFile     string                        // where the evidence is kept across a restart, "" for nowhere
	runwaySaved    map[string]RunwayTrackerState // read at start, for an airport not followed yet
	stopCh         chan struct{}
	wg             sync.WaitGroup
}

// NewTrajectoryTracker creates and starts the tracker with the given configuration.
// The cleanup goroutine runs in the background until Stop is called.
func NewTrajectoryTracker(
	cfg TrajectoryConfig,
	ref *phaseRef,
	phasesConfig *config.FlightPhasesConfig,
	log *logger.Logger,
) *TrajectoryTracker {
	tt := &TrajectoryTracker{
		aircraft:     make(map[string]*AircraftTrajectory),
		config:       cfg,
		ref:          ref,
		phasesConfig: phasesConfig,
		newRunways: func() *RunwayInUseTracker {
			return NewRunwayInUseTracker(
				phasesConfig.RunwayInUseWindowMinutes,
				phasesConfig.RunwayInUseApproachWeight,
				phasesConfig.RunwayInUseLandingWeight,
				phasesConfig.RunwayInUseClimbWeight,
				phasesConfig.RunwayInUseDecayRate,
				log,
			)
		},
		logger: log.Named("trajectory"),
		stopCh: make(chan struct{}),
	}
	tt.resetRunwayTrackers()
	tt.wg.Add(1)
	go tt.cleanupLoop()
	tt.logger.Info("Trajectory tracker started",
		logger.Int("buffer_duration_sec", cfg.BufferDurationSec),
		logger.Int("buffer_capacity", cfg.BufferCapacity),
		logger.Int("min_points", cfg.MinPointsForAnalysis),
	)
	return tt
}

// referenceChanged is called by the service after it has stored new airports.
// The evidence of which runway is in use belonged to the previous airports:
// kept, it would decay over the next hour while pointing at runways that no
// longer exist in the data. An airport still followed keeps its own.
func (tt *TrajectoryTracker) referenceChanged() {
	tt.resetRunwayTrackers()
	for _, ref := range tt.ref.all() {
		tt.logger.Info("Phase reference changed",
			logger.String("airport", ref.Airport),
			logger.Int("runway_pairs", len(ref.Runways.RunwayThresholds)))
	}
}

// resetRunwayTrackers gives each followed airport a runway-in-use tracker,
// keeping the one of an airport already followed.
func (tt *TrajectoryTracker) resetRunwayTrackers() {
	tt.runwayMu.Lock()
	defer tt.runwayMu.Unlock()
	next := map[string]*RunwayInUseTracker{}
	for _, ref := range tt.ref.all() {
		if old, ok := tt.runwayTrackers[ref.Airport]; ok {
			next[ref.Airport] = old
			continue
		}
		rt := tt.newRunways()
		rt.SetRunwayData(ref.Runways)
		if st, ok := tt.runwaySaved[ref.Airport]; ok {
			rt.Restore(st)
			delete(tt.runwaySaved, ref.Airport)
		}
		next[ref.Airport] = rt
	}
	tt.runwayTrackers = next
}

// runwayTrackerFor returns the runway-in-use tracker of a followed airport, nil
// for one that is not followed.
func (tt *TrajectoryTracker) runwayTrackerFor(airport string) *RunwayInUseTracker {
	tt.runwayMu.RLock()
	defer tt.runwayMu.RUnlock()
	return tt.runwayTrackers[airport]
}

// Stop shuts down the background cleanup goroutine and waits for it to finish.
func (tt *TrajectoryTracker) Stop() {
	close(tt.stopCh)
	tt.wg.Wait()
	tt.saveRunways()
	tt.logger.Info("Trajectory tracker stopped")
}

// RecordRunwayLanding records a landing event for runway-in-use detection.
// Called by the service when a T/D is detected near a runway threshold of
// airport.
func (tt *TrajectoryTracker) RecordRunwayLanding(airport, runwayID string, hex string) {
	if rt := tt.runwayTrackerFor(airport); rt != nil {
		rt.RecordEvent(runwayID, RunwayEventLanding, hex)
	}
}

// GetRunwayScores returns the top N runway-in-use scores of the principal
// airport, for external consumers.
func (tt *TrajectoryTracker) GetRunwayScores(n int) []RunwayScore {
	return tt.GetRunwayScoresFor(tt.ref.load().Airport, n)
}

// GetRunwayScoresFor returns the top N runway-in-use scores of a followed
// airport.
func (tt *TrajectoryTracker) GetRunwayScoresFor(airport string, n int) []RunwayScore {
	if rt := tt.runwayTrackerFor(airport); rt != nil {
		return rt.GetTopScores(n)
	}
	return nil
}

// RunwaysInUse counts the runways in use at every followed airport.
func (tt *TrajectoryTracker) RunwaysInUse() int {
	tt.runwayMu.RLock()
	defer tt.runwayMu.RUnlock()
	n := 0
	for _, rt := range tt.runwayTrackers {
		n += rt.RunwaysInUse()
	}
	return n
}

// AlignedAirport returns the followed airport on one of whose runway axes the
// aircraft was seen, "" if none -- the stronger of the two ways an aircraft is
// given an airport (airportFor).
func (tt *TrajectoryTracker) AlignedAirport(hex string) string {
	tt.mu.RLock()
	defer tt.mu.RUnlock()
	if at, ok := tt.aircraft[hex]; ok {
		return at.airport
	}
	return ""
}

// LastPosition returns the aircraft's last valid position no older than maxAge.
// A landing is often detected after the receiver has stopped decoding
// positions -- aircraft.json drops a position after a minute -- and this is
// where the aircraft last was.
func (tt *TrajectoryTracker) LastPosition(hex string, maxAge time.Duration) (float64, float64, bool) {
	tt.mu.RLock()
	defer tt.mu.RUnlock()
	at, ok := tt.aircraft[hex]
	if !ok {
		return 0, 0, false
	}
	var last *TrajectorySnapshot
	at.ForEachSnapshot(func(snap *TrajectorySnapshot) {
		if snap.Valid {
			last = snap
		}
	})
	if last == nil || time.Since(last.Timestamp) > maxAge {
		return 0, 0, false
	}
	return last.Lat, last.Lon, true
}

// AirportOf returns the followed airport an aircraft is being judged against,
// as of its last derived state; the principal when it has none yet.
func (tt *TrajectoryTracker) AirportOf(hex string) *PhaseReference {
	tt.mu.RLock()
	at, ok := tt.aircraft[hex]
	tt.mu.RUnlock()
	if ok && at.Derived.ref != nil {
		return at.Derived.ref
	}
	return tt.ref.load()
}

// Ingest adds a new snapshot for the given aircraft. If the aircraft has no
// buffer yet, one is created. Called once per aircraft per fetch cycle.
func (tt *TrajectoryTracker) Ingest(hex string, snap TrajectorySnapshot) {
	tt.mu.Lock()
	at, ok := tt.aircraft[hex]
	if !ok {
		at = NewAircraftTrajectory(hex, tt.config.BufferCapacity)
		tt.aircraft[hex] = at
	}
	at.AddSnapshot(snap)
	tt.mu.Unlock()
}

// EnsureDerived recomputes the derived state for the given aircraft if it is
// dirty (new data since last computation). This is called by DeterminePhase
// before reading derived fields.
func (tt *TrajectoryTracker) EnsureDerived(hex string) *DerivedState {
	tt.mu.RLock()
	at, ok := tt.aircraft[hex]
	tt.mu.RUnlock()
	if !ok || at.Count == 0 {
		return nil
	}
	if at.DirtyDerived {
		tt.computeDerivedState(at)
		tt.computePredictions(at)
		at.DirtyDerived = false
	}
	return &at.Derived
}

// computePredictions updates hindcast and forecast for an aircraft.
// Called after computeDerivedState to keep predictions in sync with derived state.
func (tt *TrajectoryTracker) computePredictions(at *AircraftTrajectory) {
	computeHindcast(at, &at.Derived)
	computeForecast(at, &at.Derived)
}

// GetHindcast returns hindcast prediction points for an aircraft, or nil.
func (tt *TrajectoryTracker) GetHindcast(hex string) []PredictionPoint {
	tt.mu.RLock()
	at, ok := tt.aircraft[hex]
	tt.mu.RUnlock()
	if !ok {
		return nil
	}
	return at.Prediction.Hindcast
}

// GetForecast returns forecast prediction points for an aircraft, or nil.
func (tt *TrajectoryTracker) GetForecast(hex string) []PredictionPoint {
	tt.mu.RLock()
	at, ok := tt.aircraft[hex]
	tt.mu.RUnlock()
	if !ok {
		return nil
	}
	return at.Prediction.Forecast
}

// ─── Cleanup ──────────────────────────────────────────────────────────────────

func (tt *TrajectoryTracker) cleanupLoop() {
	defer tt.wg.Done()
	ticker := time.NewTicker(time.Duration(tt.config.CleanupIntervalSec) * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			tt.cleanup()
			tt.saveRunways()
		case <-tt.stopCh:
			return
		}
	}
}

func (tt *TrajectoryTracker) cleanup() {
	now := time.Now().UTC()
	staleThreshold := time.Duration(tt.config.StaleTimeoutSec) * time.Second
	tt.mu.Lock()
	removed := 0
	for hex, at := range tt.aircraft {
		if now.Sub(at.LastSeen) > staleThreshold {
			delete(tt.aircraft, hex)
			removed++
		}
	}
	tt.mu.Unlock()
	if removed > 0 {
		tt.logger.Debug("Trajectory cleanup",
			logger.Int("removed", removed),
			logger.Int("remaining", len(tt.aircraft)),
		)
	}
}

// ─── Derived State Computation ────────────────────────────────────────────────

// computeDerivedState recalculates all derived fields from the ring buffer contents.
// This runs once per fetch cycle per aircraft (~90 data points, O(N) single pass).
func (tt *TrajectoryTracker) computeDerivedState(at *AircraftTrajectory) {
	d := &at.Derived
	*d = DerivedState{ComputedAt: time.Now().UTC()} // Reset

	latest := at.Latest()
	if latest == nil || !latest.Valid {
		return
	}

	// Collect valid snapshots and detect data gaps
	shortWindowSec := 10.0
	mediumWindowSec := 30.0
	fetchInterval := float64(tt.config.FetchIntervalSec)
	if fetchInterval < 1 {
		fetchInterval = 1
	}
	gapThreshold := fetchInterval * 2.5

	var (
		allValid    []TrajectorySnapshot
		shortValid  []TrajectorySnapshot
		mediumValid []TrajectorySnapshot
	)

	shortCutoff := latest.Timestamp.Add(-time.Duration(shortWindowSec * float64(time.Second)))
	mediumCutoff := latest.Timestamp.Add(-time.Duration(mediumWindowSec * float64(time.Second)))

	var prevTimestamp time.Time
	gapDetected := false

	at.ForEachSnapshot(func(snap *TrajectorySnapshot) {
		if !snap.Valid {
			return
		}
		allValid = append(allValid, *snap)
		if !snap.Timestamp.Before(mediumCutoff) {
			mediumValid = append(mediumValid, *snap)
		}
		if !snap.Timestamp.Before(shortCutoff) {
			shortValid = append(shortValid, *snap)
		}
		// Gap detection
		if !prevTimestamp.IsZero() {
			gap := snap.Timestamp.Sub(prevTimestamp).Seconds()
			if gap > gapThreshold {
				gapDetected = true
			}
		}
		prevTimestamp = snap.Timestamp
	})

	d.ValidPointCount = len(allValid)
	d.DataGapDetected = gapDetected
	if len(allValid) >= 2 {
		d.WindowDurationSec = allValid[len(allValid)-1].Timestamp.Sub(allValid[0].Timestamp).Seconds()
	}

	if len(allValid) == 0 {
		return
	}

	// ── Current distance and bearing to the airport this aircraft is about ──
	// One choice for the whole computation: an airport changed mid-way would
	// otherwise mix two airports in a single state.
	ref := tt.airportFor(at, latest.Lat, latest.Lon, latest.Track, latest.AltBaro)
	d.Airport, d.ref = ref.Airport, ref
	d.DistToAirportNM = MetersToNM(Haversine(latest.Lat, latest.Lon, ref.Lat, ref.Lon))
	d.BearingToAirport = CalculateBearing(latest.Lat, latest.Lon, ref.Lat, ref.Lon)

	// ── Short window: smoothed current values ──
	if len(shortValid) > 0 {
		d.GroundSpeedKts = mean(shortValid, func(s TrajectorySnapshot) float64 { return s.GS })
		d.VerticalRateFPM = medianFloat(shortValid, func(s TrajectorySnapshot) float64 { return s.BaroRate })
		d.TrackDeg = circularMean(shortValid, func(s TrajectorySnapshot) float64 { return s.Track })
	} else {
		d.GroundSpeedKts = latest.GS
		d.VerticalRateFPM = latest.BaroRate
		d.TrackDeg = latest.Track
	}

	// ── Short window: acceleration ──
	if len(shortValid) >= 2 {
		first := shortValid[0]
		last := shortValid[len(shortValid)-1]
		dt := last.Timestamp.Sub(first.Timestamp).Seconds()
		if dt > 0 {
			d.GSAccelKtsPerSec = (last.GS - first.GS) / dt
			d.VRAccelFPMPerSec = (last.BaroRate - first.BaroRate) / dt
			d.TrackRateDegPerSec = circularDiff(last.Track, first.Track) / dt
		}
	}

	// ── Medium window: altitude statistics ──
	window := mediumValid
	if len(window) == 0 {
		window = allValid
	}

	d.AltMean = mean(window, func(s TrajectorySnapshot) float64 { return s.AltBaro })
	d.AltMin, d.AltMax = minMax(window, func(s TrajectorySnapshot) float64 { return s.AltBaro })
	d.AltTrendFPM = olsSlope(window, func(s TrajectorySnapshot) float64 { return s.AltBaro }) * 60.0 // Convert ft/sec to fpm

	// ── Medium window: speed statistics ──
	d.GSMean = mean(window, func(s TrajectorySnapshot) float64 { return s.GS })
	d.GSMin, d.GSMax = minMax(window, func(s TrajectorySnapshot) float64 { return s.GS })
	d.GSTrendKtsPerSec = olsSlope(window, func(s TrajectorySnapshot) float64 { return s.GS })

	// ── Medium window: vertical rate statistics ──
	d.VRMean = mean(window, func(s TrajectorySnapshot) float64 { return s.BaroRate })
	d.VRStdDev = stdDev(window, func(s TrajectorySnapshot) float64 { return s.BaroRate })

	// ── Medium window: distance to the reference airport, trend ──
	d.DistTrendNMPerSec = olsSlopeWithXY(window, func(s TrajectorySnapshot) (float64, float64) {
		return s.Timestamp.Sub(window[0].Timestamp).Seconds(),
			MetersToNM(Haversine(s.Lat, s.Lon, ref.Lat, ref.Lon))
	})

	// ── Boolean flags ──
	cfg := tt.config
	d.IsDescending = d.AltTrendFPM < cfg.DescentVRThresholdFPM && d.VRMean < 0
	d.IsClimbing = d.AltTrendFPM > cfg.ClimbVRThresholdFPM && d.VRMean > 0
	d.IsLevel = (d.AltMax - d.AltMin) < cfg.LevelAltBandFt
	d.IsDecelerating = d.GSTrendKtsPerSec < cfg.DecelerationThreshold
	d.IsAccelerating = d.GSTrendKtsPerSec > cfg.AccelerationThreshold
	d.IsTurning = math.Abs(d.TrackRateDegPerSec) > cfg.TurningRateThresholdDeg
	d.IsApproachingAirport = d.DistTrendNMPerSec < -0.001 // Slightly negative threshold to avoid noise
}

// ─── Statistical Helpers ──────────────────────────────────────────────────────

// mean computes the arithmetic mean of a field extracted from snapshots.
func mean(snaps []TrajectorySnapshot, extract func(TrajectorySnapshot) float64) float64 {
	if len(snaps) == 0 {
		return 0
	}
	sum := 0.0
	for _, s := range snaps {
		sum += extract(s)
	}
	return sum / float64(len(snaps))
}

// minMax returns the minimum and maximum values of a field.
func minMax(snaps []TrajectorySnapshot, extract func(TrajectorySnapshot) float64) (float64, float64) {
	if len(snaps) == 0 {
		return 0, 0
	}
	mn := math.Inf(1)
	mx := math.Inf(-1)
	for _, s := range snaps {
		v := extract(s)
		if v < mn {
			mn = v
		}
		if v > mx {
			mx = v
		}
	}
	return mn, mx
}

// medianFloat computes the median of a field. Allocates a temporary slice for sorting.
func medianFloat(snaps []TrajectorySnapshot, extract func(TrajectorySnapshot) float64) float64 {
	n := len(snaps)
	if n == 0 {
		return 0
	}
	vals := make([]float64, n)
	for i, s := range snaps {
		vals[i] = extract(s)
	}
	sort.Float64s(vals)
	if n%2 == 0 {
		return (vals[n/2-1] + vals[n/2]) / 2
	}
	return vals[n/2]
}

// stdDev computes the sample standard deviation of a field.
func stdDev(snaps []TrajectorySnapshot, extract func(TrajectorySnapshot) float64) float64 {
	n := len(snaps)
	if n < 2 {
		return 0
	}
	m := mean(snaps, extract)
	sumSq := 0.0
	for _, s := range snaps {
		diff := extract(s) - m
		sumSq += diff * diff
	}
	return math.Sqrt(sumSq / float64(n-1))
}

// olsSlope computes the OLS linear regression slope of a field against time.
// Time is measured in seconds from the first snapshot. Returns the slope in
// units-per-second (e.g., feet-per-second for altitude).
func olsSlope(snaps []TrajectorySnapshot, extract func(TrajectorySnapshot) float64) float64 {
	n := len(snaps)
	if n < 2 {
		return 0
	}
	t0 := snaps[0].Timestamp
	var sumT, sumY, sumTY, sumTT float64
	for _, s := range snaps {
		t := s.Timestamp.Sub(t0).Seconds()
		y := extract(s)
		sumT += t
		sumY += y
		sumTY += t * y
		sumTT += t * t
	}
	nf := float64(n)
	denom := nf*sumTT - sumT*sumT
	if math.Abs(denom) < 1e-12 {
		return 0 // All points at same time
	}
	return (nf*sumTY - sumT*sumY) / denom
}

// olsSlopeWithXY computes OLS slope with an explicit (x, y) extractor,
// used for distance-to-station trend where x = time offset, y = distance.
func olsSlopeWithXY(snaps []TrajectorySnapshot, extractXY func(TrajectorySnapshot) (float64, float64)) float64 {
	n := len(snaps)
	if n < 2 {
		return 0
	}
	var sumX, sumY, sumXY, sumXX float64
	for _, s := range snaps {
		x, y := extractXY(s)
		sumX += x
		sumY += y
		sumXY += x * y
		sumXX += x * x
	}
	nf := float64(n)
	denom := nf*sumXX - sumX*sumX
	if math.Abs(denom) < 1e-12 {
		return 0
	}
	return (nf*sumXY - sumX*sumY) / denom
}

// olsR2 computes the R² (coefficient of determination) for the OLS linear
// regression of a field against time. Returns 0 if fewer than 3 points or no
// variance in the data. R² = 1 - SS_res/SS_tot.
func olsR2(snaps []TrajectorySnapshot, extract func(TrajectorySnapshot) float64) float64 {
	n := len(snaps)
	if n < 3 {
		return 0
	}
	t0 := snaps[0].Timestamp

	// Compute OLS coefficients (intercept + slope)
	var sumT, sumY, sumTY, sumTT float64
	for _, s := range snaps {
		t := s.Timestamp.Sub(t0).Seconds()
		y := extract(s)
		sumT += t
		sumY += y
		sumTY += t * y
		sumTT += t * t
	}
	nf := float64(n)
	denom := nf*sumTT - sumT*sumT
	if math.Abs(denom) < 1e-12 {
		return 0
	}
	slope := (nf*sumTY - sumT*sumY) / denom
	intercept := (sumY - slope*sumT) / nf

	// Compute R² = 1 - SS_res / SS_tot
	meanY := sumY / nf
	var ssRes, ssTot float64
	for _, s := range snaps {
		t := s.Timestamp.Sub(t0).Seconds()
		y := extract(s)
		predicted := intercept + slope*t
		ssRes += (y - predicted) * (y - predicted)
		ssTot += (y - meanY) * (y - meanY)
	}
	if ssTot < 1e-20 {
		return 0 // No variance
	}
	r2 := 1.0 - ssRes/ssTot
	if r2 < 0 {
		r2 = 0
	}
	return r2
}

// circularMean computes the mean of angles (degrees) handling 0°/360° wraparound.
func circularMean(snaps []TrajectorySnapshot, extract func(TrajectorySnapshot) float64) float64 {
	if len(snaps) == 0 {
		return 0
	}
	var sinSum, cosSum float64
	for _, s := range snaps {
		rad := extract(s) * math.Pi / 180.0
		sinSum += math.Sin(rad)
		cosSum += math.Cos(rad)
	}
	meanRad := math.Atan2(sinSum/float64(len(snaps)), cosSum/float64(len(snaps)))
	deg := meanRad * 180.0 / math.Pi
	return math.Mod(deg+360, 360)
}

// circularDiff returns the signed angular difference (a - b) in degrees,
// handling the 0°/360° wraparound. Result is in [-180, 180].
func circularDiff(a, b float64) float64 {
	diff := a - b
	for diff > 180 {
		diff -= 360
	}
	for diff < -180 {
		diff += 360
	}
	return diff
}
