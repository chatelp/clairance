package adsb

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/yegors/co-atc/pkg/logger"
)

// ─── Runway-In-Use Detection ─────────────────────────────────────────────────
//
// RunwayInUseTracker observes aircraft movements (approaches, landings,
// departures) to determine which runway ends are currently active. It maintains
// a rolling time window of weighted evidence events and produces a probability
// distribution over runway ends.
//
// The primary consumer is ruleApproach: when we have enough data to be
// confident about the active runway, approaches to non-active runways (e.g.
// perpendicular cross-runways during base turns) are suppressed.
//
// Thread-safe: the fetch goroutine calls RecordEvent and IsActiveRunway from
// the same goroutine, but the tracker may also be queried from API handlers.

// RunwayEventType classifies evidence events by their source.
type RunwayEventType int

const (
	RunwayEventApproach RunwayEventType = iota // Aircraft entered APP on this runway end
	RunwayEventLanding                         // Aircraft touched down (T/D)
	RunwayEventClimb                           // Aircraft climbed out (CLB) on this runway end
)

func (t RunwayEventType) String() string {
	switch t {
	case RunwayEventApproach:
		return "approach"
	case RunwayEventLanding:
		return "landing"
	case RunwayEventClimb:
		return "climb"
	default:
		return "unknown"
	}
}

// RunwayEvent records a single piece of evidence that a runway end is in use.
type RunwayEvent struct {
	RunwayEnd string          `json:"runway_end"` // "05-23/05" format (matches RunwayApproachInfo.RunwayID)
	Type      RunwayEventType `json:"type"`
	Hex       string          `json:"hex"` // Aircraft that generated this event
	Timestamp time.Time       `json:"timestamp"`
}

// RunwayScore holds the computed score and probability for a runway end.
type RunwayScore struct {
	RunwayEnd   string  `json:"runway_end"`
	Score       float64 `json:"score"`
	Probability float64 `json:"probability"` // normalized 0-1
	EventCount  int     `json:"event_count"`

	// When the airport's runway in use last changed, the same for every score
	// of the airport; nil until a change has been seen since the server
	// started -- the first runway detected is not a change, the server only
	// does not know since when it has been in use.
	Since *time.Time `json:"since"`
}

// RunwayInUseTracker maintains a rolling window of runway usage evidence
// and computes which runway ends are currently active.
type RunwayInUseTracker struct {
	mu             sync.RWMutex
	events         []RunwayEvent
	windowDuration time.Duration
	weights        [3]float64 // indexed by RunwayEventType
	decayRate      float64    // per-minute exponential decay
	logger         *logger.Logger

	// Cached state from last recompute
	scores        []RunwayScore // sorted descending by score
	activeSet     map[string]bool
	lastActiveEnd string    // for change detection
	lastChangeAt  time.Time // when lastActiveEnd replaced another; zero until then
	lastLogTime   time.Time // throttle periodic Info log

	// Persisted state — retained when all events expire from the window so
	// that the last known active runway is shown instead of N/A during
	// low-traffic periods.
	everHadData bool

	// All known runway end IDs from runway data, for parallel detection.
	// When one runway of a parallel pair is active, the other is automatically
	// included in the active set (e.g. 06L active → 06R also active).
	knownRunwayEnds []string

	// The direction an aircraft flies on each known runway end, in degrees
	// true: parallel runways need not share a number (see parallelHeadingDeg).
	endHeadings map[string]float64
}

const (
	// Minimum probability (relative) for a runway to be considered "active".
	// Handles parallel runway operations (e.g. two parallel runways both at ~40%).
	activeMinProbability = 0.15

	// Minimum interval between periodic score logs.
	logThrottleInterval = 60 * time.Second

	// Runway ends this close in heading are parallel even when their numbers
	// differ: Paris-CDG's two pairs are 08L/08R and 09L/09R, 85 and 86 degrees
	// true, flown in the same direction together. With the number alone, 09
	// active kept rejecting every approach to 08R. Orly's 06 and 07, 12 degrees
	// apart, stay separate.
	parallelHeadingDeg = 5.0
)

// NewRunwayInUseTracker creates a tracker with the given scoring parameters.
func NewRunwayInUseTracker(
	windowMinutes int,
	approachWeight, landingWeight, climbWeight, decayRate float64,
	log *logger.Logger,
) *RunwayInUseTracker {
	rt := &RunwayInUseTracker{
		windowDuration: time.Duration(windowMinutes) * time.Minute,
		weights:        [3]float64{approachWeight, landingWeight, climbWeight},
		decayRate:      decayRate,
		logger:         log.Named("runway-use"),
		activeSet:      make(map[string]bool),
	}
	rt.logger.Info("Runway-in-use tracker started",
		logger.Int("window_minutes", windowMinutes),
		logger.Float64("approach_weight", approachWeight),
		logger.Float64("landing_weight", landingWeight),
		logger.Float64("climb_weight", climbWeight),
		logger.Float64("decay_rate", decayRate),
	)
	return rt
}

// RecordEvent records evidence of runway usage and recomputes scores.
//
// The runway in use is counted in aircraft, not in position reports: each
// aircraft keeps one event per type, on the last runway it was seen using, and
// a report only refreshes it. Counted per report, an aircraft on a long final
// weighed a hundred times one seen briefly, and the one or two reports an
// intercepting aircraft spends nearer the parallel runway's axis were counted
// for that runway (docs-fr/05-decisions.md, D69).
func (rt *RunwayInUseTracker) RecordEvent(runwayID string, eventType RunwayEventType, hex string) {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	now := time.Now().UTC()
	replaced := false
	if hex != "" {
		for i := range rt.events {
			if rt.events[i].Hex == hex && rt.events[i].Type == eventType {
				rt.events[i].RunwayEnd = runwayID
				rt.events[i].Timestamp = now
				replaced = true
				break
			}
		}
	}
	if !replaced {
		rt.events = append(rt.events, RunwayEvent{
			RunwayEnd: runwayID,
			Type:      eventType,
			Hex:       hex,
			Timestamp: now,
		})
	}

	rt.logger.Debug("Runway event recorded",
		logger.String("runway", runwayID),
		logger.String("type", eventType.String()),
		logger.String("hex", hex),
	)

	rt.recompute(now)
}

// IsActiveRunway checks whether a runway end is in the current active set.
// Returns true during startup grace (never had data) so that all runways are
// accepted until enough traffic has been observed. Once data has been seen,
// uses the active set (which persists across inactivity periods).
func (rt *RunwayInUseTracker) IsActiveRunway(runwayID string) bool {
	rt.mu.RLock()
	defer rt.mu.RUnlock()

	// Startup grace: never had any data → accept all runways
	if !rt.everHadData {
		return true
	}
	return rt.activeSet[runwayID]
}

// HasData returns true if there are any events in the current window.
func (rt *RunwayInUseTracker) HasData() bool {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	return len(rt.events) > 0
}

// GetTopScores returns the top N runway scores, sorted by score descending.
func (rt *RunwayInUseTracker) GetTopScores(n int) []RunwayScore {
	rt.mu.RLock()
	defer rt.mu.RUnlock()

	if n > len(rt.scores) {
		n = len(rt.scores)
	}
	result := make([]RunwayScore, n)
	copy(result, rt.scores[:n])
	if !rt.lastChangeAt.IsZero() {
		for i := range result {
			since := rt.lastChangeAt
			result[i].Since = &since
		}
	}
	return result
}

// RunwaysInUse counts the runways in the active set, a runway used in both
// directions once: "06L-24R/06L" and "06L-24R/24R" are one runway. Zero until
// the first evidence; the last known set is kept through quiet hours, as
// IsActiveRunway keeps it.
func (rt *RunwayInUseTracker) RunwaysInUse() int {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	pairs := make(map[string]bool, len(rt.activeSet))
	for id := range rt.activeSet {
		pairs[strings.SplitN(id, "/", 2)[0]] = true
	}
	return len(pairs)
}

// recompute prunes old events, recalculates time-decayed scores, and updates
// the active set. Must be called with rt.mu held for writing.
func (rt *RunwayInUseTracker) recompute(now time.Time) {
	// ── Prune events outside the window ──
	cutoff := now.Add(-rt.windowDuration)
	writeIdx := 0
	for _, e := range rt.events {
		if e.Timestamp.After(cutoff) {
			rt.events[writeIdx] = e
			writeIdx++
		}
	}
	rt.events = rt.events[:writeIdx]

	// ── Compute time-decayed weighted scores per runway end ──
	type accumulator struct {
		score float64
		count int
	}
	scoreMap := make(map[string]*accumulator)

	for _, e := range rt.events {
		minutesAgo := now.Sub(e.Timestamp).Minutes()
		weight := rt.weights[e.Type] * math.Pow(rt.decayRate, minutesAgo)

		acc, ok := scoreMap[e.RunwayEnd]
		if !ok {
			acc = &accumulator{}
			scoreMap[e.RunwayEnd] = acc
		}
		acc.score += weight
		acc.count++
	}

	// ── Build sorted score list ──
	scores := make([]RunwayScore, 0, len(scoreMap))
	totalScore := 0.0
	for end, acc := range scoreMap {
		totalScore += acc.score
		scores = append(scores, RunwayScore{
			RunwayEnd:  end,
			Score:      acc.score,
			EventCount: acc.count,
		})
	}

	sort.Slice(scores, func(i, j int) bool {
		return scores[i].Score > scores[j].Score
	})

	// ── Normalize to probabilities ──
	if totalScore > 0 {
		for i := range scores {
			scores[i].Probability = scores[i].Score / totalScore
		}
	}

	// If we have live data, update scores and mark that we've seen data.
	// If all events expired (low traffic), keep the last known scores/activeSet
	// so the UI shows the last active runway instead of N/A.
	if len(scores) > 0 {
		rt.scores = scores
		rt.everHadData = true

		// ── Build active set (probability >= threshold) ──
		rt.activeSet = make(map[string]bool, len(scores))
		for _, s := range scores {
			if s.Probability >= activeMinProbability {
				rt.activeSet[s.RunwayEnd] = true
			}
		}

		// ── Expand for parallel runways ──
		// If 06L is active, 06R is automatically included (and vice versa).
		// This breaks the chicken-and-egg where the second parallel runway
		// can never get APP events because IsActiveRunway rejects it.
		rt.expandActiveSetForParallels()
	}
	// else: keep previous rt.scores and rt.activeSet intact

	// ── Logging ──
	newActiveEnd := ""
	if len(rt.scores) > 0 {
		newActiveEnd = rt.scores[0].RunwayEnd
	}

	// Log on active runway change
	if newActiveEnd != rt.lastActiveEnd && newActiveEnd != "" {
		if rt.lastActiveEnd != "" && sameDirection(rt.lastActiveEnd, newActiveEnd) {
			// The lead passing between two runways flown the same way together
			// (De Gaulle's 26L and 27R, 0.57 and 0.43 on 13/10, swapping every few
			// minutes) is not a change of runway: "since" stays.
			rt.logger.Debug("Lead passed within the same direction",
				logger.String("previous", formatRunwayEnd(rt.lastActiveEnd)),
				logger.String("current", formatRunwayEnd(newActiveEnd)),
			)
		} else if rt.lastActiveEnd != "" {
			rt.lastChangeAt = now
			rt.logger.Info("Active runway changed",
				logger.String("previous", formatRunwayEnd(rt.lastActiveEnd)),
				logger.String("current", formatRunwayEnd(newActiveEnd)),
				logger.String("scores", formatScores(rt.scores)),
			)
		} else {
			rt.logger.Info("Initial active runway detected",
				logger.String("runway", formatRunwayEnd(newActiveEnd)),
				logger.String("scores", formatScores(rt.scores)),
			)
		}
		rt.lastActiveEnd = newActiveEnd
	}

	// Periodic score summary (throttled)
	if now.Sub(rt.lastLogTime) >= logThrottleInterval {
		if len(rt.scores) > 0 {
			n := 3
			if n > len(rt.scores) {
				n = len(rt.scores)
			}
			rt.logger.Info("Runway in use",
				logger.String("scores", formatScores(rt.scores[:n])),
				logger.Int("total_events", len(rt.events)),
			)
		}
		rt.lastLogTime = now
	}
}

// SetRunwayData provides the tracker with all known runway end IDs for parallel
// runway detection. When one runway of a parallel pair (e.g. 06L) is active,
// the tracker automatically includes the parallel (06R) in the active set.
// Reset forgets every piece of evidence, as when the reference airport changes:
// the approaches and climbs seen so far were onto another airport's runways, and
// "last known active runway" would name one that is no longer in the data.
func (rt *RunwayInUseTracker) Reset() {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.events = nil
	rt.scores = nil
	rt.activeSet = nil
	rt.lastActiveEnd = ""
	rt.lastChangeAt = time.Time{}
	rt.everHadData = false
}

// RunwayTrackerState is what a tracker keeps across a restart of the server
// (runway_state.go): its evidence, and the runway it last named with since
// when.
type RunwayTrackerState struct {
	Events        []RunwayEvent `json:"events"`
	LastActiveEnd string        `json:"last_active_end,omitempty"`
	LastChangeAt  time.Time     `json:"last_change_at,omitempty"`
}

// State is the tracker's evidence, to be restored after a restart.
func (rt *RunwayInUseTracker) State() RunwayTrackerState {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	return RunwayTrackerState{
		Events:        append([]RunwayEvent(nil), rt.events...),
		LastActiveEnd: rt.lastActiveEnd,
		LastChangeAt:  rt.lastChangeAt,
	}
}

// Restore takes back the evidence a tracker had before a restart, as far as
// it is still in the window, ahead of what has been seen since.
func (rt *RunwayInUseTracker) Restore(st RunwayTrackerState) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	now := time.Now().UTC()
	var kept []RunwayEvent
	for _, e := range st.Events {
		if now.Sub(e.Timestamp) < rt.windowDuration {
			kept = append(kept, e)
		}
	}
	if len(kept) == 0 {
		return
	}
	rt.events = append(kept, rt.events...)
	if rt.lastActiveEnd == "" {
		rt.lastActiveEnd = st.LastActiveEnd
		rt.lastChangeAt = st.LastChangeAt
	}
	rt.recompute(now)
	rt.logger.Info("Runway evidence restored",
		logger.Int("events", len(kept)),
		logger.String("scores", formatScores(rt.scores)),
	)
}

func (rt *RunwayInUseTracker) SetRunwayData(runways RunwayData) {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	var ends []string
	headings := make(map[string]float64)
	for pairKey, thresholds := range runways.RunwayThresholds {
		for endID, t := range thresholds {
			id := pairKey + "/" + endID
			ends = append(ends, id)
			if o, ok := thresholds[getOppositeThreshold(endID, pairKey)]; ok {
				headings[id] = CalculateBearing(t.Latitude, t.Longitude, o.Latitude, o.Longitude)
			}
		}
	}
	rt.knownRunwayEnds = ends
	rt.endHeadings = headings

	if len(ends) > 0 {
		rt.logger.Info("Runway data loaded for parallel detection",
			logger.Int("runway_ends", len(ends)),
		)
	}
}

// runwayEndIdent extracts the end identifier from a full runway ID.
// "06L-24R/06L" → "06L", "05-23/05" → "05"
func runwayEndIdent(runwayID string) string {
	parts := strings.SplitN(runwayID, "/", 2)
	if len(parts) == 2 {
		return parts[1]
	}
	return runwayID
}

// runwayBaseNumber strips the L/R/C suffix from a runway end identifier
// to get the numeric heading. "06L" → "06", "24R" → "24", "33" → "33"
func runwayBaseNumber(endIdent string) string {
	if len(endIdent) == 0 {
		return ""
	}
	last := endIdent[len(endIdent)-1]
	if last == 'L' || last == 'R' || last == 'C' {
		return endIdent[:len(endIdent)-1]
	}
	return endIdent
}

// expandActiveSetForParallels adds parallel runway ends to the active set.
// If "06L-24R/06L" is active, this also marks "06R-24L/06R" as active
// (if it exists in known runway data). This breaks the chicken-and-egg
// problem where the second parallel runway can never accumulate approach
// events because IsActiveRunway rejects it.
func (rt *RunwayInUseTracker) expandActiveSetForParallels() {
	if len(rt.knownRunwayEnds) == 0 {
		return
	}

	// Collect base numbers of all currently active runway ends
	activeBases := make(map[string]bool)
	for activeID := range rt.activeSet {
		base := runwayBaseNumber(runwayEndIdent(activeID))
		if base != "" {
			activeBases[base] = true
		}
	}

	// Headings of the active ends, for parallels whose numbers differ
	var activeHeadings []float64
	for activeID := range rt.activeSet {
		if h, ok := rt.endHeadings[activeID]; ok {
			activeHeadings = append(activeHeadings, h)
		}
	}

	// Add any known runway end whose base number matches an active base, or
	// that is flown in the same direction as an active end
	for _, knownID := range rt.knownRunwayEnds {
		if rt.activeSet[knownID] {
			continue // already active
		}
		base := runwayBaseNumber(runwayEndIdent(knownID))
		if base != "" && activeBases[base] {
			rt.activeSet[knownID] = true
			continue
		}
		if h, ok := rt.endHeadings[knownID]; ok {
			for _, a := range activeHeadings {
				if d := math.Abs(math.Mod(h-a+540, 360) - 180); d <= parallelHeadingDeg {
					rt.activeSet[knownID] = true
					break
				}
			}
		}
	}
}

// formatRunwayEnd extracts a human-readable runway name from "05-23/05" format.
// Returns just the end identifier (e.g. "05") with the pair for context.
// sameDirection says whether two runway ends are flown the same way: their
// numbers (the magnetic heading in tens of degrees) differ by one at most,
// around the compass. De Gaulle's doublets, 26L/26R and 27L/27R, land and take
// off westward together; a change of runway is a change of direction. Ends
// whose number cannot be read are never the same direction.
func sameDirection(a, b string) bool {
	na, okA := runwayNumber(a)
	nb, okB := runwayNumber(b)
	if !okA || !okB {
		return false
	}
	d := na - nb
	if d < 0 {
		d = -d
	}
	if d > 18 {
		d = 36 - d
	}
	return d <= 1
}

// runwayNumber reads the number of a runway end, "09L-27R/27R" -> 27.
func runwayNumber(runwayID string) (int, bool) {
	end := runwayID
	if i := strings.LastIndex(runwayID, "/"); i >= 0 {
		end = runwayID[i+1:]
	}
	end = strings.TrimRight(end, "LRC")
	n, err := strconv.Atoi(end)
	if err != nil || n < 1 || n > 36 {
		return 0, false
	}
	return n, true
}

func formatRunwayEnd(runwayID string) string {
	// RunwayID format: "pairKey/endIdent" e.g. "05-23/05"
	parts := strings.SplitN(runwayID, "/", 2)
	if len(parts) == 2 {
		return fmt.Sprintf("RWY %s (%s)", parts[1], parts[0])
	}
	return runwayID
}

// formatScores produces a compact log-friendly string of runway scores.
func formatScores(scores []RunwayScore) string {
	var b strings.Builder
	for i, s := range scores {
		if i > 0 {
			b.WriteString(" | ")
		}
		fmt.Fprintf(&b, "%s: %.1f (%.0f%%, %d events)",
			formatRunwayEnd(s.RunwayEnd), s.Score, s.Probability*100, s.EventCount)
	}
	return b.String()
}
