package adsb

/*
FLIGHT PHASE DETECTION SYSTEM
==============================

The Co-ATC system uses a two-tier approach for flight phase detection:

  PRIORITY 1 — Immediate ground state transitions (T/O and T/D)
    Detected instantly when the OnGround state flips via IsFlying().
    These are binary state changes that do not benefit from trajectory analysis.

  PRIORITY 2 — All other phases via trajectory analysis
    The TrajectoryTracker (trajectory.go, trajectory_phase.go) maintains a rolling
    window of ~90 seconds of ADS-B observations per aircraft and uses statistical
    analysis (OLS regression, median filtering, distance trends) to classify each
    aircraft into one of 10 phases:

      NEW — Newly detected or parked on ground
      TAX — Taxiing (1–50 kts ground speed)
      T/O — Takeoff (ground→air transition, preserved for UI visibility)
      CLB — Initial climb out (runway heading, near airport, hasRecentTakeoff)
      DEP — Departure (climbing away, below cruise — general)
      CRZ — Cruise (above cruise altitude)
      ARR — Arrival (approaching station, below cruise, descending/level)
      APP — Approach (on final, descending, runway-aligned)
      T/D — Touchdown (air→ground transition, preserved for UI visibility)
      UNK — Unknown (airborne but doesn't match any specific condition)

  ARR is NOT a catch-all — it requires the aircraft to be genuinely approaching
  the station. UNK is the honest catch-all for anything that doesn't match.

SPECIAL FEATURES:
  - Trajectory-based noise resistance: decisions use windowed trends, not single readings
  - Signal Lost Landing: auto-marks aircraft as landed when signal lost near airport,
    enhanced by trajectory descent trend analysis
  - Phase Preservation: T/O and T/D kept visible for configurable duration
  - Sensor Data Validation: corrects erroneous readings before trajectory ingestion
  - Data gap detection: handles coverage gaps without spurious phase flips
*/

import (
	"context"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/yegors/co-atc/internal/config"
	"github.com/yegors/co-atc/internal/websocket"
	"github.com/yegors/co-atc/pkg/logger"
)

func roundTo(value float64, decimals int) float64 {
	pow := math.Pow(10, float64(decimals))
	return math.Round(value*pow) / pow
}

func roundedInterface(value interface{}, decimals int) interface{} {
	switch v := value.(type) {
	case float64:
		return roundTo(v, decimals)
	case *float64:
		if v == nil {
			return nil
		}
		return roundTo(*v, decimals)
	default:
		return value
	}
}

func normalizeRealtimeDelta(delta map[string]interface{}) map[string]interface{} {
	normalized := make(map[string]interface{}, len(delta))
	for key, value := range delta {
		switch key {
		case "lat", "lon":
			normalized[key] = roundedInterface(value, 6)
		case "alt_baro", "alt_geom", "gs", "tas", "ias", "track", "true_heading", "mag_heading", "baro_rate", "geom_rate", "vertical_speed", "vertical_rate":
			normalized[key] = roundedInterface(value, 0)
		default:
			normalized[key] = value
		}
	}
	return normalized
}

func roundPtr(value *float64, decimals int) *float64 {
	if value == nil {
		return nil
	}
	rounded := roundTo(*value, decimals)
	return &rounded
}

func normalizeRealtimeAircraft(aircraft *Aircraft) *Aircraft {
	if aircraft == nil {
		return nil
	}

	clone := *aircraft
	if aircraft.ADSB == nil {
		return &clone
	}

	adsbCopy := *aircraft.ADSB
	if adsbCopy.AltBaro.IsSet() {
		adsbCopy.AltBaro = FlexibleFloat64(roundTo(adsbCopy.AltBaro.Float64(), 0))
	}
	if adsbCopy.AltGeom.IsSet() {
		adsbCopy.AltGeom = FlexibleFloat64(roundTo(adsbCopy.AltGeom.Float64(), 0))
	}
	adsbCopy.Lat = roundPtr(adsbCopy.Lat, 6)
	adsbCopy.Lon = roundPtr(adsbCopy.Lon, 6)
	adsbCopy.GS = roundPtr(adsbCopy.GS, 0)
	adsbCopy.IAS = roundPtr(adsbCopy.IAS, 0)
	adsbCopy.TAS = roundPtr(adsbCopy.TAS, 0)
	adsbCopy.Track = roundPtr(adsbCopy.Track, 0)
	adsbCopy.MagHeading = roundPtr(adsbCopy.MagHeading, 0)
	adsbCopy.TrueHeading = roundPtr(adsbCopy.TrueHeading, 0)
	adsbCopy.BaroRate = roundPtr(adsbCopy.BaroRate, 0)
	adsbCopy.GeomRate = roundPtr(adsbCopy.GeomRate, 0)
	adsbCopy.NavHeading = roundPtr(adsbCopy.NavHeading, 0)
	adsbCopy.NavAltitudeMCP = roundPtr(adsbCopy.NavAltitudeMCP, 0)
	adsbCopy.NavAltitudeFMS = roundPtr(adsbCopy.NavAltitudeFMS, 0)

	clone.ADSB = &adsbCopy
	return &clone
}

// WebSocketServer defines the interface for a WebSocket server
type WebSocketServer interface {
	Broadcast(message *websocket.Message)
}

// ReferenceService defines the interface for reference data lookups (aircraft, airlines)
type ReferenceService interface {
	LookupAircraft(hex string) *ReferenceAircraftInfo
	LookupAirline(code string) string
	LookupAirlineCountry(code string) string
}

// ReferenceAircraftInfo represents aircraft info from the reference data service
type ReferenceAircraftInfo struct {
	Hex               string
	Registration      string
	TypeCode          string
	ManufacturerModel string
	Year              string
	Owner             string
}

// Storage defines the interface for aircraft data storage
type Storage interface {
	GetAll() []*Aircraft
	GetAllWithLastSeenFilter(lastSeenMinutes int) []*Aircraft
	GetAllMinimal(lastSeenMinutes int) []*Aircraft // Minimal mode: skips phase history and date queries
	GetByHex(hex string) (*Aircraft, bool)
	GetFiltered(
		minAltitude, maxAltitude float64,
		status []string,
		tookOffAfter, tookOffBefore, landedAfter, landedBefore *time.Time,
	) []*Aircraft
	Upsert(aircraft *Aircraft)
	Count() int
	GetAllPositionHistory(hex string) ([]Position, error)
	GetPositionHistoryWithLimit(hex string, limit int) ([]Position, error)

	// Phase change methods
	InsertPhaseChange(hex, flight, phase string, timestamp time.Time, adsbId *int) error
	GetPhaseHistory(hex string) ([]PhaseChange, error)
	GetCurrentPhase(hex string) (*PhaseChange, error)
	GetLatestTakeoffTime(hex string) (*time.Time, error)
	GetLatestLandingTime(hex string) (*time.Time, error)
	GetLatestADSBTargetID(hex string) (*int, error)

	// Batch phase change methods for performance optimization
	GetCurrentPhasesBatch(hexCodes []string) (map[string]*PhaseChange, error)
	GetLatestADSBTargetIDsBatch(hexCodes []string) (map[string]*int, error)
	InsertPhaseChangesBatch(changes []PhaseChangeInsert) error

	// Batch methods for fetch hot path optimization
	GetAircraftOnGroundBatch(hexCodes []string) (map[string]bool, error)
	GetLatestADSBDataBatch(hexCodes []string) (map[string]*ADSBTarget, error)
	GetLatestTakeoffTimesBatch(hexCodes []string) (map[string]*time.Time, error)
	GetLatestLandingTimesBatch(hexCodes []string) (map[string]*time.Time, error)

	// Targeted query for inactive aircraft status updates
	GetStaleActiveAircraft(activeHexCodes []string, cutoff time.Time) ([]*Aircraft, error)
}

// Service is the main service for ADS-B data processing
type Service struct {
	voiceState // what the radio has said about each aircraft; see voice.go

	client          *Client
	storage         Storage
	fetchInterval   time.Duration
	logger          *logger.Logger
	lastFetchTime   time.Time
	lastFetchStatus bool
	// Since when every fetch has failed, and why, in a few words; zero and
	// empty while the source answers. The ADS-B light says it (14/10: after a
	// network cut a test instance failed for 12 h, "no route to host", while
	// a new process reached the station; a restart was the remedy).
	failingSince       time.Time
	failingWhy         string
	messages           messageRate // the receiver's counter, for messages per second
	mu                 sync.RWMutex
	stopCh             chan struct{}
	wg                 sync.WaitGroup
	refService         ReferenceService          // Reference data lookup service
	stationLat         float64                   // Station latitude from config
	stationLon         float64                   // Station longitude from config
	stationElevFeet    float64                   // Station elevation in feet
	wsServer           WebSocketServer           // WebSocket server for broadcasting events
	signalLostTimeout  time.Duration             // Time after which aircraft is marked as signal_lost
	ref                *phaseRef                 // Airport phases are judged against; see PhaseReference
	runwayJudge        *RunwayJudge              // The one judge of each aircraft's runway (D69)
	flightPhasesConfig config.FlightPhasesConfig // Flight phases configuration
	changeDetector     *ChangeDetector           // Tracks aircraft changes
	trajectoryTracker  *TrajectoryTracker        // Trajectory-based phase detection
	live               *livePredictor            // Predicted states between polls; nil without tracker or page

	// clock is the service's time, the wall clock when nil; the replay bench
	// (replay_test.go) runs the whole chain on a simulated one.
	clock func() time.Time
}

// now is the time on the service's clock, in UTC.
func (s *Service) now() time.Time {
	if s.clock != nil {
		return s.clock().UTC()
	}
	return time.Now().UTC()
}

// AircraftBulkResponse represents server response with bulk aircraft data
type AircraftBulkResponse struct {
	Aircraft []*Aircraft    `json:"aircraft"`
	Count    int            `json:"count"`
	Counts   AircraftCounts `json:"counts"`
}

// NewService creates a new ADS-B service
func NewService(
	client *Client,
	storage Storage,
	fetchInterval time.Duration,
	logger *logger.Logger,
	stationCfg config.StationConfig,
	adsbCfg config.ADSBConfig,
	flightPhasesConfig config.FlightPhasesConfig,
	wsServer WebSocketServer,
) *Service {
	// Set default signal lost timeout if not configured
	signalLostTimeout := time.Duration(adsbCfg.SignalLostTimeoutSecs) * time.Second
	if signalLostTimeout == 0 {
		signalLostTimeout = 60 * time.Second // Default to 60 seconds
	}

	service := &Service{
		client:          client,
		storage:         storage,
		fetchInterval:   fetchInterval,
		logger:          logger.Named("adsb"),
		stopCh:          make(chan struct{}),
		stationLat:      stationCfg.Latitude,
		stationLon:      stationCfg.Longitude,
		stationElevFeet: float64(stationCfg.ElevationFeet),
		// Until the reference airport is known the receiver stands in for it,
		// which is upstream's behaviour exactly.
		ref: newPhaseRef(PhaseReference{
			Airport: stationCfg.AirportCode,
			Lat:     stationCfg.Latitude,
			Lon:     stationCfg.Longitude,
		}),
		wsServer:           wsServer,
		signalLostTimeout:  signalLostTimeout,
		flightPhasesConfig: flightPhasesConfig,
	}

	// Always enable WebSocket streaming for aircraft updates
	logger.Info("Initializing WebSocket change detection for aircraft streaming")
	service.changeDetector = NewChangeDetector(logger)

	service.runwayJudge = NewRunwayJudge()

	// Initialize trajectory tracker for phase detection
	if flightPhasesConfig.Enabled {
		fetchIntervalSec := adsbCfg.FetchIntervalSecs
		if fetchIntervalSec < 1 {
			fetchIntervalSec = 1
		}
		bufferCap := flightPhasesConfig.TrajectoryBufferDurationSec/fetchIntervalSec + 10 // +10 margin
		service.trajectoryTracker = NewTrajectoryTracker(
			TrajectoryConfig{
				BufferDurationSec:       flightPhasesConfig.TrajectoryBufferDurationSec,
				BufferCapacity:          bufferCap,
				FetchIntervalSec:        fetchIntervalSec,
				MinPointsForAnalysis:    flightPhasesConfig.TrajectoryMinPoints,
				StaleTimeoutSec:         flightPhasesConfig.TrajectoryStaleTimeoutSec,
				CleanupIntervalSec:      flightPhasesConfig.TrajectoryCleanupIntervalSec,
				DescentVRThresholdFPM:   flightPhasesConfig.TrajectoryDescentThresholdFPM,
				ClimbVRThresholdFPM:     flightPhasesConfig.TrajectoryClimbThresholdFPM,
				LevelAltBandFt:          flightPhasesConfig.TrajectoryLevelBandFt,
				TurningRateThresholdDeg: flightPhasesConfig.TrajectoryTurningRateDeg,
				DecelerationThreshold:   -0.5,
				AccelerationThreshold:   0.5,
			},
			service.ref,
			&service.flightPhasesConfig,
			logger,
		)
		if wsServer != nil {
			service.live = newLivePredictor(service.trajectoryTracker, wsServer)
		}
	}

	return service
}

// broadcastAircraftChange broadcasts a single aircraft change via WebSocket
func (s *Service) broadcastAircraftChange(change AircraftChange) {
	var messageType string
	switch change.Type {
	case "added":
		messageType = websocket.MessageTypeAircraftAdded
	case "updated":
		messageType = websocket.MessageTypeAircraftUpdate
	case "removed":
		messageType = websocket.MessageTypeAircraftRemoved
	}

	data := map[string]interface{}{
		"type": change.Type,
		"hex":  change.Hex,
	}

	observedAt := s.now()
	if change.Aircraft != nil && !change.Aircraft.LastSeen.IsZero() {
		observedAt = change.Aircraft.LastSeen.UTC()
	}
	data["observed_at"] = observedAt.Format(time.RFC3339Nano)

	// For "added", send full aircraft object
	// For "updated", send only the delta (changed fields), and the two
	// figures the page dates its position by (seen, seen_pos): the whole
	// aircraft rode along with every delta, ~5 KB 57 times a second, about
	// 300 KB/s of JSON for each page (measured 16/10), for these two.
	// For "removed", just send hex
	if change.Delta != nil {
		data["delta"] = normalizeRealtimeDelta(change.Delta)
		if change.Aircraft != nil && change.Aircraft.ADSB != nil {
			// null when the receiver does not say, as the aircraft's own.
			data["timing"] = map[string]interface{}{
				"seen":     change.Aircraft.ADSB.Seen,
				"seen_pos": change.Aircraft.ADSB.SeenPos,
			}
		}
	} else if change.Aircraft != nil {
		data["aircraft"] = normalizeRealtimeAircraft(change.Aircraft)
	}

	message := &websocket.Message{
		Type: messageType,
		Data: data,
	}

	if s.wsServer != nil {
		s.wsServer.Broadcast(message)
	}
}

// Start starts the ADS-B service
func (s *Service) Start(ctx context.Context) error {
	s.logger.Info("Starting ADS-B service",
		logger.Duration("fetch_interval", s.fetchInterval),
	)

	// Initial fetch
	if err := s.fetchAndProcess(ctx); err != nil {
		s.logger.Error("Failed to fetch initial ADS-B data", logger.Error(err))
		s.setFetchStatus(false)
		return fmt.Errorf("failed initial ADS-B fetch: %w", err)
	}
	s.setFetchStatus(true)

	// Start background fetching
	s.wg.Add(1)
	go s.fetchLoop(ctx)

	// Start background prediction streaming (between ADS-B polls)
	if s.live != nil {
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.live.run(ctx, s.stopCh, s.now)
		}()
	}

	return nil
}

// Stop stops the ADS-B service
// KeepRunwaysIn has the runways in use kept in path across restarts
// (runway_state.go).
func (s *Service) KeepRunwaysIn(path string) {
	if s.trajectoryTracker != nil {
		s.trajectoryTracker.KeepRunwaysIn(path)
	}
}

func (s *Service) Stop() {
	s.logger.Info("Stopping ADS-B service")
	close(s.stopCh)
	s.wg.Wait()
	if s.trajectoryTracker != nil {
		s.trajectoryTracker.Stop()
	}
	s.logger.Info("ADS-B service stopped")
}

// fetchLoop periodically fetches and processes ADS-B data
func (s *Service) fetchLoop(ctx context.Context) {
	defer s.wg.Done()

	ticker := time.NewTicker(s.fetchInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := s.fetchAndProcess(ctx); err != nil {
				s.logger.Error("Failed to fetch ADS-B data", logger.Error(err))
				s.setFetchStatus(false)
				s.noteFetchFailure(err)
			} else {
				s.setFetchStatus(true)
				s.noteFetchFailure(nil)
			}
		case <-s.stopCh:
			return
		case <-ctx.Done():
			return
		}
	}
}

// fetchAndProcess fetches and processes ADS-B data
func (s *Service) fetchAndProcess(ctx context.Context) error {
	// Fetch raw data
	rawData, err := s.client.FetchData(ctx)
	if err != nil {
		return err
	}
	s.messages.note(rawData.Messages, s.now())

	// Process raw data
	newAircraft := s.ProcessRawData(rawData)

	// Create a map of active aircraft hex codes
	activeAircraft := make(map[string]bool)
	for _, a := range newAircraft {
		activeAircraft[a.Hex] = true
	}

	// Collect all hex codes for batch operations
	hexCodes := make([]string, len(newAircraft))
	for i, a := range newAircraft {
		hexCodes[i] = a.Hex
	}

	// PERFORMANCE: Batch queries replace ~8000 per-aircraft queries with 6 batch queries per cycle
	existingOnGround, _ := s.storage.GetAircraftOnGroundBatch(hexCodes)
	existingADSB, _ := s.storage.GetLatestADSBDataBatch(hexCodes)
	takeoffTimes, _ := s.storage.GetLatestTakeoffTimesBatch(hexCodes)
	landingTimes, _ := s.storage.GetLatestLandingTimesBatch(hexCodes)
	currentPhases, _ := s.storage.GetCurrentPhasesBatch(hexCodes)
	adsbTargetIDs, _ := s.storage.GetLatestADSBTargetIDsBatch(hexCodes)

	// Process each aircraft for ground state determination and takeoff/landing detection
	for _, a := range newAircraft {
		// Check if aircraft exists in database using pre-fetched batch data
		_, found := existingOnGround[a.Hex]

		var prevTAS, prevGS, prevAlt float64
		if found {
			if adsbData, ok := existingADSB[a.Hex]; ok && adsbData != nil {
				prevTAS = NumberOrZero(adsbData.TAS)
				prevGS = NumberOrZero(adsbData.GS)
				prevAlt = adsbData.AltBaro.Float64()
			}
		}

		// Validate and correct sensor data for potential errors
		currentTAS := NumberOrZero(a.ADSB.TAS)
		currentGS := NumberOrZero(a.ADSB.GS)
		lat, lon, _ := a.ADSB.Position()
		correctedTAS, correctedGS, correctedAlt := ValidateSensorData(
			currentTAS, currentGS, a.ADSB.AltBaro.Float64(),
			prevTAS, prevGS, prevAlt,
			lat, lon, s.ref.load().Lat, s.ref.load().Lon,
			s.flightPhasesConfig.AirportRangeNM,
			&s.flightPhasesConfig,
		)

		// Apply corrected values back to ADSB data so the entire pipeline
		// (phase detection, DB storage, change detector) uses consistent corrected data
		if correctedTAS != currentTAS || correctedGS != currentGS || correctedAlt != a.ADSB.AltBaro.Float64() {
			if found {
				s.logger.Debug("Sensor data corrected",
					logger.String("hex", a.Hex),
					logger.String("flight", a.Flight),
					logger.Float64("original_tas", currentTAS),
					logger.Float64("corrected_tas", correctedTAS),
					logger.Float64("original_gs", currentGS),
					logger.Float64("corrected_gs", correctedGS),
					logger.Float64("original_alt", a.ADSB.AltBaro.Float64()),
					logger.Float64("corrected_alt", correctedAlt),
				)
			}
			if correctedTAS != currentTAS {
				a.ADSB.TAS = NumberPtr(correctedTAS)
			}
			if correctedGS != currentGS {
				a.ADSB.GS = NumberPtr(correctedGS)
			}
			if correctedAlt != a.ADSB.AltBaro.Float64() {
				a.ADSB.AltBaro = FlexibleFloat64(correctedAlt)
			}
		}

		if a.ADSB.SourceType == SourceTypeExternalOpenSky && a.ADSB.OnGroundReported != nil {
			// Prefer source-provided OpenSky on_ground when available.
			a.OnGround = *a.ADSB.OnGroundReported
		} else {
			// Determine if aircraft is currently flying using corrected values and config
			currentlyFlying := IsFlying(NumberOrZero(a.ADSB.TAS), NumberOrZero(a.ADSB.GS), a.ADSB.AltBaro.Float64(), &s.flightPhasesConfig)

			// Guard against false on_ground from missing data: Mode S first contacts may
			// report zero altitude, zero speed, and no position. That's "no data available",
			// not "on the ground". Defaulting to on_ground here would cause a false T/O
			// when real data arrives on the next cycle.
			noUsableData := a.ADSB.AltBaro.Float64() == 0 && NumberOrZero(a.ADSB.GS) == 0 && NumberOrZero(a.ADSB.TAS) == 0
			if noUsableData {
				// No sensor data at all — preserve previous ground state if known,
				// otherwise assume airborne (safer than triggering false T/O later)
				if prevOnGround, known := existingOnGround[a.Hex]; known {
					a.OnGround = prevOnGround
				} else {
					a.OnGround = false
				}
			} else {
				a.OnGround = !currentlyFlying
			}
		}

		// Feed corrected data into trajectory tracker for phase analysis
		if s.trajectoryTracker != nil && a.ADSB != nil {
			polled := s.now()
			snap := TrajectorySnapshotFromADSB(a.ADSB, a.OnGround, polled, ReceiverTime(rawData, a.ADSB.SourceType, polled))
			s.trajectoryTracker.Ingest(a.Hex, snap)
		}
		if a.ADSB != nil {
			if lat, lon, ok := a.ADSB.Position(); ok && a.ADSB.Track != nil {
				s.runwayJudge.Observe(a.Hex, s.ref.all(), RunwayObservation{
					Lat: lat, Lon: lon, TrackDeg: *a.ADSB.Track, AltFt: a.ADSB.AltBaro.Float64(),
					BaroRateFPM: a.ADSB.BaroRate, OnGround: a.OnGround,
					MaxApproachFt: float64(s.flightPhasesConfig.ApproachMaxAltitudeFt), At: s.now(),
				})
			}
		}

		if found {
			// Use pre-fetched takeoff and landing times
			a.DateTookoff = takeoffTimes[a.Hex]
			a.DateLanded = landingTimes[a.Hex]
		} else {
			// New aircraft — log and send WebSocket notification
			s.logger.Info("New aircraft detected",
				logger.String("hex", a.Hex),
				logger.String("flight", a.Flight),
				logger.Float64("altitude", a.ADSB.AltBaro.Float64()),
				logger.Bool("on_ground", a.OnGround),
			)
			if s.wsServer != nil {
				s.wsServer.Broadcast(&websocket.Message{
					Type: "status_update",
					Data: map[string]interface{}{
						"hex":        a.Hex,
						"flight":     a.Flight,
						"altitude":   a.ADSB.AltBaro.Float64(),
						"on_ground":  a.OnGround,
						"timestamp":  s.now().Format(time.RFC3339),
						"new_status": "new_aircraft",
					},
				})
			}
		}
	}

	// PRIORITY 1: Handle immediate ground state transitions (takeoff/landing)
	immediatePhaseChanges := s.detectGroundStateTransitions(newAircraft, existingOnGround, currentPhases, adsbTargetIDs)
	if len(immediatePhaseChanges) > 0 {
		err := s.storage.InsertPhaseChangesBatch(immediatePhaseChanges)
		if err != nil {
			s.logger.Error("Failed to insert immediate ground transition phases", logger.Error(err))
		} else {
			s.sendImmediateGroundTransitionAlerts(immediatePhaseChanges)
		}
	}

	// Update all aircraft in the database
	for _, a := range newAircraft {
		s.storage.Upsert(a)
	}

	// Update status of existing aircraft that are no longer active
	s.updateAircraftStatus(activeAircraft)

	// PRIORITY 2: Handle all other phase changes (normal phase detection)
	newPhaseChanges := s.processPhaseChangesBatch(newAircraft, immediatePhaseChanges, currentPhases, adsbTargetIDs, takeoffTimes)

	s.setLastFetchTime(s.now())

	// Detect and broadcast changes using enriched newAircraft (no DB read)
	if s.changeDetector != nil {
		// Enrich newAircraft with phase data from batch + new changes
		phaseMap := make(map[string]PhaseChange)
		for hex, phase := range currentPhases {
			if phase != nil {
				phaseMap[hex] = *phase
			}
		}
		// Override with immediate phase changes. The airport goes with the phase:
		// the PAR view reads it to keep another airport's traffic off its runways.
		for _, change := range immediatePhaseChanges {
			phaseMap[change.Hex] = PhaseChange{Phase: change.Phase, Timestamp: change.Timestamp, Airport: change.Airport}
		}
		// Override with newly detected phase changes
		for _, change := range newPhaseChanges {
			phaseMap[change.Hex] = PhaseChange{Phase: change.Phase, Timestamp: change.Timestamp, Airport: change.Airport}
		}
		for _, a := range newAircraft {
			if phase, ok := phaseMap[a.Hex]; ok {
				a.Phase = &PhaseData{Current: []PhaseChange{phase}}
			}
		}

		// Enrich with BSDB data (in-memory lookups)
		s.enrichWithRefData(newAircraft)
		s.enrichWithATCDerivedData(newAircraft)

		// Overlay trajectory-based predictions (computed fresh after phase detection)
		if s.trajectoryTracker != nil {
			for _, a := range newAircraft {
				if forecast := s.trajectoryTracker.GetForecast(a.Hex); len(forecast) > 0 {
					a.Future = PredictionPointsToPositions(forecast)
				}
				if hindcast := s.trajectoryTracker.GetHindcast(a.Hex); len(hindcast) > 0 {
					a.Hindcast = PredictionPointsToPositions(hindcast)
				}
			}
		}

		changes := s.changeDetector.DetectChanges(newAircraft)
		if len(changes) > 0 {
			s.logger.Debug("Detected aircraft changes",
				logger.Int("change_count", len(changes)))

			for _, change := range changes {
				s.broadcastAircraftChange(change)
			}
		}
	}

	s.logger.Debug("Updated aircraft data",
		logger.Int("count", len(newAircraft)),
		logger.Int("total", s.storage.Count()),
	)

	return nil
}

func (s *Service) enrichWithATCDerivedData(aircraft []*Aircraft) {
	for _, a := range aircraft {
		if a == nil {
			continue
		}
		// Every path that sends aircraft out passes here, so the runway goes
		// wherever the aircraft goes.
		a.Runway = s.runwayJudge.Get(a.Hex)
		if a.ADSB == nil {
			continue
		}
		a.ADSB.ATCDerived = computeATCDerivedMetrics(a.ADSB, a.Distance)
	}
}

// SetReferenceService sets the reference data lookup service and updates runway data
func (s *Service) SetReferenceService(refService ReferenceService) {
	s.refService = refService
}

// SetPhaseReference sets the airport that approach, departure, takeoff and
// landing are judged against, with its runways. It may be called at any time,
// from the settings panel as well as at startup: the reference is swapped
// atomically and the runway-in-use evidence of the previous airport is dropped.
//
// A reference without a position falls back to the receiver, as upstream did.
func (s *Service) SetPhaseReference(r PhaseReference) {
	s.SetPhaseReferences([]PhaseReference{r})
}

// SetPhaseReferences follows several airports at once, the principal first: each
// aircraft is judged against the one whose runway axis it flies, else the
// nearest (airportFor). The principal alone falls back to the receiver when it
// has no position; a further airport without one is left out.
func (s *Service) SetPhaseReferences(refs []PhaseReference) {
	if len(refs) == 0 {
		refs = []PhaseReference{{}}
	}
	out := make([]PhaseReference, 0, len(refs))
	for i, r := range refs {
		if r.Lat == 0 && r.Lon == 0 {
			if i > 0 {
				continue
			}
			r.Lat, r.Lon = s.stationLat, s.stationLon
		}
		out = append(out, r)
	}
	s.ref.storeAll(out)
	if s.trajectoryTracker != nil {
		s.trajectoryTracker.referenceChanged()
	}
}

// PhaseReference returns the principal airport phases are judged against.
func (s *Service) PhaseReference() PhaseReference {
	return *s.ref.load()
}

// PhaseReferences returns every airport followed, the principal first.
func (s *Service) PhaseReferences() []PhaseReference {
	return append([]PhaseReference(nil), s.ref.all()...)
}

// GetRunwayInUseScoresFor returns the top N runway-in-use scores of a followed
// airport.
func (s *Service) GetRunwayInUseScoresFor(airport string, n int) []RunwayScore {
	if s.trajectoryTracker != nil {
		return s.trajectoryTracker.GetRunwayScoresFor(airport, n)
	}
	return nil
}

// RunwaysInUse counts the runways in use at every followed airport, as the
// runway-in-use trackers judge them.
func (s *Service) RunwaysInUse() int {
	if s.trajectoryTracker != nil {
		return s.trajectoryTracker.RunwaysInUse()
	}
	return 0
}

// phaseAirport is the followed airport a phase of this aircraft belongs to: the
// one it is judged against, for the phases that are about an airport. Cruise
// and the rest belong to none, and so does a takeoff or landing away from the
// airport -- a light aircraft touching down at an airfield nobody follows is
// not landing at the nearest one that is.
//
// "Away" is airport_range_nm, except for an aircraft seen on one of the
// airport's runway axes: the receiver loses airliners a few hundred feet up, so
// the last position of a landing is a minute old, 5 to 7.5 NM out (measured on
// 26/09 at Orly and De Gaulle). Such an aircraft is allowed the approach
// distance, approach_max_distance_nm.
func (s *Service) phaseAirport(a *Aircraft, phase string) string {
	switch phase {
	case "APP", "ARR", "DEP", "CLB", "T/O", "T/D":
	default:
		return ""
	}
	ref := s.ref.load()
	aligned := ""
	if s.trajectoryTracker != nil {
		ref = s.trajectoryTracker.AirportOf(a.Hex)
		aligned = s.trajectoryTracker.AlignedAirport(a.Hex)
	}
	if phase == "T/O" || phase == "T/D" {
		var lat, lon float64
		ok := false
		if a.ADSB != nil {
			lat, lon, ok = a.ADSB.Position()
		}
		// Measured on 26/09: the landings at Orly and De Gaulle were detected a
		// minute after the last position, when aircraft.json had dropped it.
		if !ok && s.trajectoryTracker != nil {
			lat, lon, ok = s.trajectoryTracker.LastPosition(a.Hex, 2*time.Minute)
		}
		if !ok {
			return ""
		}
		limit := s.flightPhasesConfig.AirportRangeNM
		if aligned == ref.Airport {
			limit = math.Max(limit, float64(s.flightPhasesConfig.ApproachMaxDistanceNM))
		}
		if MetersToNM(Haversine(lat, lon, ref.Lat, ref.Lon)) > limit {
			return ""
		}
	}
	return ref.Airport
}

// landingRunway finds the runway an aircraft touching down is aligned with,
// among every followed airport's; the nearest threshold wins.
func (s *Service) landingRunway(lat, lon, track, alt float64) (string, *RunwayApproachInfo) {
	var airport string
	var best *RunwayApproachInfo
	for _, ref := range s.ref.all() {
		info := DetectRunwayApproach(lat, lon, track, alt, ref.Runways, s.flightPhasesConfig)
		if info != nil && info.OnApproach && (best == nil || info.DistanceToThreshold < best.DistanceToThreshold) {
			airport, best = ref.Airport, info
		}
	}
	return airport, best
}

// GetRunwayInUseScores returns the top N runway-in-use probability scores.
func (s *Service) GetRunwayInUseScores(n int) []RunwayScore {
	if s.trajectoryTracker != nil {
		return s.trajectoryTracker.GetRunwayScores(n)
	}
	return nil
}

// enrichWithRefData enriches aircraft data with reference data (aircraft.csv)
func (s *Service) enrichWithRefData(aircraft []*Aircraft) {
	if s.refService == nil {
		return
	}

	for _, a := range aircraft {
		if info := s.refService.LookupAircraft(a.Hex); info != nil {
			a.BSDB = &BSDBData{
				Registration:     info.Registration,
				ICAOTypeCode:     info.TypeCode,
				Manufacturer:     info.ManufacturerModel,
				Type:             info.ManufacturerModel,
				RegisteredOwners: info.Owner,
			}
		}
		// Enrich airline country from airline code (not stored in DB, derived at runtime)
		if a.Airline != "" && a.AirlineCountry == "" && len(a.Flight) >= 3 {
			a.AirlineCountry = s.refService.LookupAirlineCountry(strings.ToUpper(a.Flight[:3]))
		}
	}
}

// GetAllAircraft returns all aircraft
func (s *Service) GetAllAircraft() []*Aircraft {
	aircraft := s.storage.GetAll()
	s.enrichWithRefData(aircraft)
	s.enrichWithATCDerivedData(aircraft)
	return aircraft
}

// GetAllAircraftWithLastSeenFilter returns aircraft seen within the last N minutes
// This uses database-level filtering for better performance on large databases
func (s *Service) GetAllAircraftWithLastSeenFilter(lastSeenMinutes int) []*Aircraft {
	aircraft := s.storage.GetAllWithLastSeenFilter(lastSeenMinutes)
	s.enrichWithRefData(aircraft)
	s.enrichWithATCDerivedData(aircraft)
	return aircraft
}

// GetAllAircraftMinimal returns aircraft with minimal data (optimized for simple API)
// Skips phase history and takeoff/landing time queries for better performance
func (s *Service) GetAllAircraftMinimal(lastSeenMinutes int) []*Aircraft {
	aircraft := s.storage.GetAllMinimal(lastSeenMinutes)
	s.enrichWithRefData(aircraft)
	s.enrichWithATCDerivedData(aircraft)
	return aircraft
}

// GetAircraftByHex returns an aircraft by its hex ID
func (s *Service) GetAircraftByHex(hex string) (*Aircraft, bool) {
	aircraft, found := s.storage.GetByHex(hex)
	if found && aircraft != nil {
		s.enrichWithRefData([]*Aircraft{aircraft})
		s.enrichWithATCDerivedData([]*Aircraft{aircraft})

		// Overlay trajectory-based predictions (hindcast + forecast)
		if s.trajectoryTracker != nil {
			if forecast := s.trajectoryTracker.GetForecast(hex); len(forecast) > 0 {
				aircraft.Future = PredictionPointsToPositions(forecast)
			}
			if hindcast := s.trajectoryTracker.GetHindcast(hex); len(hindcast) > 0 {
				aircraft.Hindcast = PredictionPointsToPositions(hindcast)
			}
		}
		// An aircraft the tracker does not forecast (no tracker, or no longer
		// tracked): the same model from its last stored sample, from when its
		// position was decoded -- last_seen is the poll less the whole seconds
		// of seen.
		if len(aircraft.Future) == 0 && aircraft.ADSB != nil {
			poll := aircraft.LastSeen.Add(time.Duration(math.Trunc(NumberOrZero(aircraft.ADSB.Seen))) * time.Second)
			posAt := poll.Add(-time.Duration(NumberOrZero(aircraft.ADSB.SeenPos) * float64(time.Second)))
			aircraft.Future = PredictionPointsToPositions(ForecastFromTarget(aircraft.ADSB, posAt))
		}
	}
	return aircraft, found
}

// GetAllPositionHistory returns all position history for an aircraft
func (s *Service) GetAllPositionHistory(hex string) ([]Position, error) {
	return s.storage.GetAllPositionHistory(hex)
}

// GetPositionHistoryWithLimit returns position history for an aircraft with a specified limit
func (s *Service) GetPositionHistoryWithLimit(hex string, limit int) ([]Position, error) {
	return s.storage.GetPositionHistoryWithLimit(hex, limit)
}

// GetPhaseHistory returns the phase change history for an aircraft (newest first)
func (s *Service) GetPhaseHistory(hex string) ([]PhaseChange, error) {
	return s.storage.GetPhaseHistory(hex)
}

// GetFilteredAircraft returns aircraft filtered by altitude, status, and date ranges
func (s *Service) GetFilteredAircraft(
	minAltitude, maxAltitude float64,
	status []string,
	tookOffAfter, tookOffBefore, landedAfter, landedBefore *time.Time,
) []*Aircraft {
	aircraft := s.storage.GetFiltered(
		minAltitude, maxAltitude,
		status,
		tookOffAfter, tookOffBefore, landedAfter, landedBefore,
	)
	s.enrichWithRefData(aircraft)
	s.enrichWithATCDerivedData(aircraft)
	return aircraft
}

// HandleBulkRequest processes client requests for bulk aircraft data
func (s *Service) HandleBulkRequest(filters map[string]interface{}) (*AircraftBulkResponse, error) {
	// Parse filters from the request
	minAltitude := 0.0
	maxAltitude := 60000.0
	var status []string
	lastSeenMinutes := 0
	excludeOtherAirportsGrounded := false
	showAir := true
	showGround := true
	var phases []string

	// Extract filters
	if val, ok := filters["min_altitude"].(float64); ok {
		minAltitude = val
	}
	if val, ok := filters["max_altitude"].(float64); ok {
		maxAltitude = val
	}
	if val, ok := filters["status"].([]interface{}); ok {
		for _, s := range val {
			if str, ok := s.(string); ok {
				status = append(status, str)
			}
		}
	}
	if val, ok := filters["last_seen_minutes"].(float64); ok {
		lastSeenMinutes = int(val)
	}
	if val, ok := filters["exclude_other_airports_grounded"].(bool); ok {
		excludeOtherAirportsGrounded = val
	}
	if val, ok := filters["show_air"].(bool); ok {
		showAir = val
	}
	if val, ok := filters["show_ground"].(bool); ok {
		showGround = val
	}
	if val, ok := filters["phases"].([]interface{}); ok {
		for _, p := range val {
			if str, ok := p.(string); ok {
				phases = append(phases, str)
			}
		}
	}

	// If both Air and Ground are disabled, return empty result
	if !showAir && !showGround {
		return &AircraftBulkResponse{
			Aircraft: []*Aircraft{},
			Count:    0,
			Counts: AircraftCounts{
				GroundActive: 0,
				GroundTotal:  0,
				AirActive:    0,
				AirTotal:     0,
			},
		}, nil
	}

	// Get filtered aircraft using existing filtering logic
	var aircraft []*Aircraft
	if minAltitude > 0 || maxAltitude < 60000 || len(status) > 0 {
		aircraft = s.GetFilteredAircraft(minAltitude, maxAltitude, status, nil, nil, nil, nil)
	} else {
		aircraft = s.GetAllAircraft()
	}

	// Apply additional filters
	if lastSeenMinutes > 0 {
		aircraft = s.filterByLastSeen(aircraft, lastSeenMinutes)
	}

	if excludeOtherAirportsGrounded {
		aircraft = s.filterByAirportGrounded(aircraft)
	}

	// Apply Air/Ground and Phase filters
	aircraft = s.filterByAirGroundAndPhases(aircraft, showAir, showGround, phases)

	// Carried, not filtered: the browser decides what to show.
	s.attachVoice(aircraft)

	// Calculate counts
	groundActive, groundTotal, airActive, airTotal := s.calculateCounts(aircraft)

	return &AircraftBulkResponse{
		Aircraft: aircraft,
		Count:    len(aircraft),
		Counts: AircraftCounts{
			GroundActive: groundActive,
			GroundTotal:  groundTotal,
			AirActive:    airActive,
			AirTotal:     airTotal,
		},
	}, nil
}

func (s *Service) filterByLastSeen(aircraft []*Aircraft, minutes int) []*Aircraft {
	cutoffTime := s.now().Add(-time.Duration(minutes) * time.Minute)
	filtered := make([]*Aircraft, 0)
	for _, a := range aircraft {
		if a.LastSeen.After(cutoffTime) {
			filtered = append(filtered, a)
		}
	}
	return filtered
}

// filterByAirGroundAndPhases filters aircraft based on Air/Ground scope and phases
func (s *Service) filterByAirGroundAndPhases(aircraft []*Aircraft, showAir, showGround bool, phases []string) []*Aircraft {
	filtered := make([]*Aircraft, 0)

	for _, a := range aircraft {
		// Apply Air/Ground filter
		if a.OnGround && !showGround {
			continue
		}
		if !a.OnGround && !showAir {
			continue
		}

		// Apply phase filter if phases are specified
		if len(phases) > 0 {
			phaseMatch := false
			if a.Phase != nil && len(a.Phase.Current) > 0 {
				currentPhase := a.Phase.Current[0].Phase
				for _, phase := range phases {
					if currentPhase == phase {
						phaseMatch = true
						break
					}
				}
			}
			if !phaseMatch {
				continue
			}
		}

		filtered = append(filtered, a)
	}

	return filtered
}

func (s *Service) filterByAirportGrounded(aircraft []*Aircraft) []*Aircraft {
	filtered := make([]*Aircraft, 0)
	airportRangeNM := 5.0 // Default range, should come from config

	for _, a := range aircraft {
		if !a.OnGround {
			filtered = append(filtered, a)
		} else if a.ADSB != nil && a.ADSB.HasPosition() {
			lat, lon, _ := a.ADSB.Position()
			// Distance from the followed airports: an aircraft on the ground is
			// interesting when it is on one of them, wherever the receiver is.
			for _, ref := range s.ref.all() {
				if MetersToNM(Haversine(lat, lon, ref.Lat, ref.Lon)) <= airportRangeNM {
					filtered = append(filtered, a)
					break
				}
			}
		}
	}
	return filtered
}

func (s *Service) calculateCounts(aircraft []*Aircraft) (int, int, int, int) {
	groundActive, groundTotal, airActive, airTotal := 0, 0, 0, 0

	for _, a := range aircraft {
		if a.OnGround {
			groundTotal++
			if a.Status == "active" {
				groundActive++
			}
		} else {
			airTotal++
			if a.Status == "active" {
				airActive++
			}
		}
	}

	return groundActive, groundTotal, airActive, airTotal
}

// GetStatus returns the service status
// MessageRate is the receiver's messages per second over the last few fetches,
// 0 while unknown (a source without a counter, or fewer than two samples).
func (s *Service) MessageRate() float64 {
	return s.messages.perSecond()
}

func (s *Service) GetStatus() (time.Time, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastFetchTime, s.lastFetchStatus
}

// GetSourceStatus returns ADS-B source health and metadata snapshot.
func (s *Service) GetSourceStatus() SourceStatus {
	if s.client == nil {
		return SourceStatus{
			Status: "error",
			Aircraft: SourceChannelStatus{
				Available: false,
				LastError: "ADS-B client not initialized",
				Data:      nil,
			},
			Receiver: SourceChannelStatus{Available: false, Data: nil},
			Stats:    SourceChannelStatus{Available: false, Data: nil},
		}
	}

	return s.client.GetSourceStatus()
}

// setLastFetchTime sets the last fetch time
func (s *Service) setLastFetchTime(t time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastFetchTime = t
}

// noteFetchFailure keeps when the current run of failed fetches began and
// why the last one failed; a success clears both.
func (s *Service) noteFetchFailure(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err == nil {
		s.failingSince, s.failingWhy = time.Time{}, ""
		return
	}
	if s.failingSince.IsZero() {
		s.failingSince = s.now()
	}
	s.failingWhy = fetchFailureReason(err)
}

// FetchFailure says since when every fetch has failed and why, or a zero
// time while the source answers.
func (s *Service) FetchFailure() (time.Time, string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.failingSince, s.failingWhy
}

// fetchFailureReason turns a fetch error into a few words a reader can act
// on, without the URL.
func fetchFailureReason(err error) string {
	msg := err.Error()
	switch {
	case strings.Contains(msg, "no route to host"):
		return "no route to the station"
	case strings.Contains(msg, "connection refused"):
		return "the station refuses the connection"
	case strings.Contains(msg, "i/o timeout"), strings.Contains(msg, "Client.Timeout"), strings.Contains(msg, "deadline exceeded"):
		return "the station does not answer in time"
	case strings.Contains(msg, "no such host"):
		return "the station's name is not found"
	case strings.Contains(msg, "status"):
		return "the station answers with an error"
	default:
		return "the station cannot be read"
	}
}

// setFetchStatus sets the fetch status
func (s *Service) setFetchStatus(status bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastFetchStatus = status
}

// updateAircraftStatus updates the status of aircraft that are no longer active.
// Uses a targeted query instead of GetAll() — only fetches aircraft that actually need status updates.
func (s *Service) updateAircraftStatus(activeAircraft map[string]bool) {
	now := s.now()
	cutoff := now.Add(-s.signalLostTimeout)

	// Build list of active hex codes for exclusion
	activeHexCodes := make([]string, 0, len(activeAircraft))
	for hex := range activeAircraft {
		activeHexCodes = append(activeHexCodes, hex)
	}

	// Targeted query: only aircraft that are active, not currently broadcasting, and stale
	staleAircraft, err := s.storage.GetStaleActiveAircraft(activeHexCodes, cutoff)
	if err != nil {
		s.logger.Error("Failed to get stale active aircraft", logger.Error(err))
		return
	}

	var inactiveAircraft []*Aircraft

	for _, aircraft := range staleAircraft {
		aircraft.Status = "signal_lost"
		s.storage.Upsert(aircraft)

		// Only track aircraft with position for signal-lost landing detection
		hasPosition := aircraft.ADSB != nil && aircraft.ADSB.HasPosition()
		if hasPosition {
			inactiveAircraft = append(inactiveAircraft, aircraft)
		}

		timeSinceLastSeen := now.Sub(aircraft.LastSeen)
		s.logger.Debug("Aircraft status updated",
			logger.String("hex", aircraft.Hex),
			logger.String("flight", aircraft.Flight),
			logger.String("new_status", "signal_lost"),
			logger.Bool("on_ground", aircraft.OnGround),
			logger.Duration("time_since_last_seen", timeSinceLastSeen),
		)

		// Send WebSocket message (skip for grounded aircraft)
		if s.wsServer != nil && !aircraft.OnGround {
			s.wsServer.Broadcast(&websocket.Message{
				Type: "status_update",
				Data: map[string]interface{}{
					"hex":                  aircraft.Hex,
					"flight":               aircraft.Flight,
					"new_status":           "signal_lost",
					"on_ground":            aircraft.OnGround,
					"time_since_last_seen": timeSinceLastSeen.Seconds(),
					"timestamp":            now.Format(time.RFC3339),
				},
			})
		}
	}

	// Check for signal lost landings
	landingPhaseChanges := s.detectSignalLostLandings(inactiveAircraft)
	if len(landingPhaseChanges) > 0 {
		err := s.storage.InsertPhaseChangesBatch(landingPhaseChanges)
		if err != nil {
			s.logger.Error("Failed to insert signal lost landing phases", logger.Error(err))
		} else {
			s.sendImmediateGroundTransitionAlerts(landingPhaseChanges)
		}
	}
}

// detectGroundStateTransitions detects immediate takeoff/landing events.
// Uses pre-fetched batch data to avoid per-aircraft database queries.
func (s *Service) detectGroundStateTransitions(aircraft []*Aircraft, existingOnGround map[string]bool, currentPhases map[string]*PhaseChange, adsbTargetIDs map[string]*int) []PhaseChangeInsert {
	var immediatePhaseChanges []PhaseChangeInsert
	now := s.now()

	for _, a := range aircraft {
		// Check if aircraft exists using pre-fetched batch data
		prevOnGround, found := existingOnGround[a.Hex]
		if !found {
			continue // New aircraft - will be handled by normal phase detection
		}

		// Check if ground state changed
		if prevOnGround != a.OnGround {
			var newPhase string
			var eventType string

			// Get current phase from pre-fetched batch data to prevent rapid T/O ↔ T/D flapping
			currentPhase := currentPhases[a.Hex]

			if !prevOnGround && a.OnGround {
				// Aircraft was airborne, now on ground = LANDING

				// Anti-flapping: Prevent T/O → T/D transition if T/O was recent
				if currentPhase != nil && currentPhase.Phase == "T/O" {
					timeSinceTakeoff := s.now().Sub(currentPhase.Timestamp).Seconds()
					preservationThreshold := float64(s.flightPhasesConfig.PhasePreservationSeconds)
					if timeSinceTakeoff < preservationThreshold {
						s.logger.Warn("Preventing rapid T/O → T/D flapping",
							logger.String("hex", a.Hex),
							logger.String("flight", a.Flight),
							logger.Float64("time_since_takeoff", timeSinceTakeoff),
							logger.Float64("threshold_seconds", preservationThreshold),
							logger.Float64("altitude", a.ADSB.AltBaro.Float64()),
						)
						continue // Skip this transition
					}
				}

				newPhase = "T/D"
				eventType = "landing"

				// Record landing for runway-in-use detection
				if s.trajectoryTracker != nil && a.ADSB != nil {
					lat, lon, ok := a.ADSB.Position()
					if ok {
						airport, runwayInfo := s.landingRunway(lat, lon, NumberOrZero(a.ADSB.Track), a.ADSB.AltBaro.Float64())
						if runwayInfo != nil {
							s.trajectoryTracker.RecordRunwayLanding(airport, runwayInfo.RunwayID, a.Hex)
						}
					}
				}

				s.logger.Info("IMMEDIATE LANDING DETECTED",
					logger.String("hex", a.Hex),
					logger.String("flight", a.Flight),
					logger.Bool("was_on_ground", prevOnGround),
					logger.Bool("now_on_ground", a.OnGround),
					logger.Float64("altitude", a.ADSB.AltBaro.Float64()),
					logger.Float64("ground_speed", NumberOrZero(a.ADSB.GS)),
				)

			} else if prevOnGround && !a.OnGround {
				// Aircraft was on ground, now airborne = TAKEOFF

				// Guard against false takeoff from incomplete ADSB data:
				// When an aircraft first appears with no/zero altitude it gets marked on_ground=true.
				// If the next cycle delivers real altitude showing the aircraft well above ground level,
				// that's data becoming available — not an actual takeoff. Skip T/O and let the
				// trajectory tracker assign the correct phase (e.g. CRZ, ARR, UNK).
				alt := a.ADSB.AltBaro.Float64()
				if alt > float64(s.flightPhasesConfig.DepartureAltitudeFt) {
					s.logger.Info("Skipping false takeoff — altitude above departure threshold (incomplete data becoming available)",
						logger.String("hex", a.Hex),
						logger.String("flight", a.Flight),
						logger.Float64("altitude", alt),
						logger.Float64("departure_threshold_ft", float64(s.flightPhasesConfig.DepartureAltitudeFt)),
					)
					continue
				}

				// Anti-flapping: Prevent T/D → T/O transition if T/D was recent
				if currentPhase != nil && currentPhase.Phase == "T/D" {
					timeSinceLanding := s.now().Sub(currentPhase.Timestamp).Seconds()
					preservationThreshold := float64(s.flightPhasesConfig.PhasePreservationSeconds)
					if timeSinceLanding < preservationThreshold {
						s.logger.Warn("Preventing rapid T/D → T/O flapping",
							logger.String("hex", a.Hex),
							logger.String("flight", a.Flight),
							logger.Float64("time_since_landing", timeSinceLanding),
							logger.Float64("threshold_seconds", preservationThreshold),
							logger.Float64("altitude", a.ADSB.AltBaro.Float64()),
						)
						continue // Skip this transition
					}
				}

				newPhase = "T/O"
				eventType = "takeoff"

				s.logger.Info("IMMEDIATE TAKEOFF DETECTED",
					logger.String("hex", a.Hex),
					logger.String("flight", a.Flight),
					logger.Bool("was_on_ground", prevOnGround),
					logger.Bool("now_on_ground", a.OnGround),
					logger.Float64("altitude", a.ADSB.AltBaro.Float64()),
					logger.Float64("ground_speed", NumberOrZero(a.ADSB.GS)),
				)
			}

			if newPhase != "" {
				// Use pre-fetched ADSB target ID
				adsbId := adsbTargetIDs[a.Hex]

				immediatePhaseChanges = append(immediatePhaseChanges, PhaseChangeInsert{
					Hex:       a.Hex,
					Flight:    a.Flight,
					Phase:     newPhase,
					Timestamp: now,
					ADSBId:    adsbId,
					EventType: eventType,
					Airport:   s.phaseAirport(a, newPhase),
				})
			}
		}
	}

	return immediatePhaseChanges
}

// detectSignalLostLandings checks for aircraft that lost signal near the airport
// and marks them as landed if they meet certain criteria
func (s *Service) detectSignalLostLandings(inactiveAircraft []*Aircraft) []PhaseChangeInsert {
	var landingPhaseChanges []PhaseChangeInsert

	// Check if signal lost landing detection is enabled
	if !s.flightPhasesConfig.SignalLostLandingEnabled {
		return landingPhaseChanges
	}

	now := s.now()

	for _, aircraft := range inactiveAircraft {
		// Skip if already on ground or no ADSB data
		if aircraft.OnGround || aircraft.ADSB == nil {
			continue
		}

		// Check if aircraft was in approach phase
		currentPhase, err := s.storage.GetCurrentPhase(aircraft.Hex)
		if err != nil || currentPhase == nil {
			continue
		}

		// Check if the trajectory shows the aircraft was descending toward the station.
		// This is a stronger signal than phase alone — if the trajectory buffer confirms
		// a steady descent toward the airport, we can be confident this is a landing.
		trajectoryConfirms := false
		if s.trajectoryTracker != nil {
			trajectoryConfirms = s.trajectoryTracker.WasDescendingTowardAirport(aircraft.Hex)
		}

		// Accept aircraft in APP phase, low altitude ARR, or trajectory-confirmed descent
		if currentPhase.Phase != "APP" &&
			!(currentPhase.Phase == "ARR" && aircraft.ADSB.AltBaro.Float64() < 2000) &&
			!trajectoryConfirms {
			continue
		}

		// Check proximity to airport
		lat, lon, hasPosition := aircraft.ADSB.Position()
		if !hasPosition {
			continue
		}
		// The airport the aircraft was being judged against: with several
		// followed, the one it was flying to.
		ref := s.ref.load()
		if s.trajectoryTracker != nil {
			ref = s.trajectoryTracker.AirportOf(aircraft.Hex)
		}
		distanceFromAirport := MetersToNM(Haversine(
			lat, lon,
			ref.Lat, ref.Lon,
		))

		// If aircraft was close to airport and low altitude when signal lost
		if distanceFromAirport <= s.flightPhasesConfig.AirportRangeNM &&
			aircraft.ADSB.AltBaro.Float64() < s.flightPhasesConfig.SignalLostLandingMaxAltFt {

			// Record for runway-in-use detection
			if s.trajectoryTracker != nil {
				runwayInfo := DetectRunwayApproach(
					lat, lon, NumberOrZero(aircraft.ADSB.Track),
					aircraft.ADSB.AltBaro.Float64(), ref.Runways, s.flightPhasesConfig,
				)
				if runwayInfo != nil && runwayInfo.OnApproach {
					s.trajectoryTracker.RecordRunwayLanding(ref.Airport, runwayInfo.RunwayID, aircraft.Hex)
				}
			}

			// Mark as landed
			aircraft.OnGround = true
			s.storage.Upsert(aircraft)

			// Create T/D phase record
			adsbId, _ := s.storage.GetLatestADSBTargetID(aircraft.Hex)
			landingPhaseChanges = append(landingPhaseChanges, PhaseChangeInsert{
				Hex:       aircraft.Hex,
				Flight:    aircraft.Flight,
				Phase:     "T/D",
				Timestamp: now,
				ADSBId:    adsbId,
				EventType: "signal_lost_landing",
				Airport:   ref.Airport,
			})

			s.logger.Info("Signal lost aircraft marked as landed",
				logger.String("hex", aircraft.Hex),
				logger.String("flight", aircraft.Flight),
				logger.Float64("last_altitude", aircraft.ADSB.AltBaro.Float64()),
				logger.Float64("distance_from_airport", distanceFromAirport),
			)
		}
	}

	return landingPhaseChanges
}

// processPhaseChangesBatch handles phase detection using pre-fetched batch data.
// Returns any new phase changes that were inserted (for downstream enrichment).
func (s *Service) processPhaseChangesBatch(aircraft []*Aircraft, immediatePhaseChanges []PhaseChangeInsert,
	currentPhases map[string]*PhaseChange, adsbTargetIDs map[string]*int, takeoffTimes map[string]*time.Time) []PhaseChangeInsert {
	if !s.flightPhasesConfig.Enabled {
		return nil
	}

	if len(aircraft) == 0 {
		return nil
	}

	// Create map of aircraft that just had immediate ground transitions
	immediateTransitions := make(map[string]string) // hex -> phase
	for _, change := range immediatePhaseChanges {
		immediateTransitions[change.Hex] = change.Phase
	}

	// Process each aircraft using pre-fetched batch data
	var phaseChanges []PhaseChangeInsert
	for _, a := range aircraft {
		// Skip aircraft that just had immediate ground transitions
		if _, hasImmediate := immediateTransitions[a.Hex]; hasImmediate {
			continue
		}

		currentPhase := currentPhases[a.Hex]
		newPhase := s.trajectoryTracker.DeterminePhase(a, currentPhase, takeoffTimes[a.Hex])

		// Apply phase stability rules and determine if change needed
		finalPhase, shouldInsert := s.evaluatePhaseChange(a, currentPhase, newPhase)

		if shouldInsert {
			phaseChanges = append(phaseChanges, PhaseChangeInsert{
				Hex:       a.Hex,
				Flight:    a.Flight,
				Phase:     finalPhase,
				Timestamp: s.now(),
				ADSBId:    adsbTargetIDs[a.Hex],
				Airport:   s.phaseAirport(a, finalPhase),
			})
		}
	}

	// Batch insert all phase changes
	if len(phaseChanges) > 0 {
		err := s.storage.InsertPhaseChangesBatch(phaseChanges)
		if err != nil {
			s.logger.Error("Failed to insert phase changes batch", logger.Error(err))
			return nil
		}

		// Send WebSocket alerts and log changes
		s.sendPhaseChangeAlerts(phaseChanges, currentPhases)
	}

	return phaseChanges
}

// evaluatePhaseChange applies phase stability rules and determines if a phase change
// should be inserted into the database.
//
// The trajectory-based phase detection (TrajectoryTracker.DeterminePhase) already provides
// noise-resistant classification through trajectory window analysis, so airborne flapping
// prevention is no longer needed. This function handles only:
//
//  1. T/O preservation — Keep takeoff phase visible for PhasePreservationSeconds
//  2. T/D post-landing flow — Transition T/D → TAX or T/D → NEW based on ground speed
//  3. Ground phase protection — Prevent premature TAX→NEW and T/D→NEW transitions
//  4. Inactive aircraft timeout — Revert long-parked aircraft to NEW
func (s *Service) evaluatePhaseChange(aircraft *Aircraft, latestPhase *PhaseChange, newPhase string) (string, bool) {
	currentPhase := newPhase

	// ── T/D post-landing flow ────────────────────────────────────────
	// After landing, guide the aircraft through T/D → TAX → NEW gracefully.
	if latestPhase != nil && latestPhase.Phase == "T/D" {
		if currentPhase == "NEW" {
			groundSpeed := 0.0
			if aircraft.ADSB != nil {
				groundSpeed = NumberOrZero(aircraft.ADSB.GS)
			}
			if aircraft.OnGround && aircraft.ADSB != nil &&
				groundSpeed >= float64(s.flightPhasesConfig.TaxiingMinSpeedKts) &&
				groundSpeed <= float64(s.flightPhasesConfig.TaxiingMaxSpeedKts) {
				currentPhase = "TAX"
			} else {
				// Keep T/D visible for the preservation period
				timeSinceLanding := s.now().Sub(latestPhase.Timestamp).Seconds()
				if timeSinceLanding < float64(s.flightPhasesConfig.PhasePreservationSeconds) {
					currentPhase = "T/D"
				} else if aircraft.ADSB != nil && groundSpeed >= float64(s.flightPhasesConfig.TaxiingMinSpeedKts) {
					currentPhase = "TAX"
				} else {
					currentPhase = "NEW"
				}
			}
		}
	}

	// ── T/O preservation ─────────────────────────────────────────────
	// Keep takeoff phase visible for at least PhasePreservationSeconds so
	// controllers can see it in the UI before it transitions to DEP.
	if latestPhase != nil && latestPhase.Phase == "T/O" {
		timeSinceTakeoff := s.now().Sub(latestPhase.Timestamp).Seconds()
		if timeSinceTakeoff < float64(s.flightPhasesConfig.PhasePreservationSeconds) {
			currentPhase = "T/O"
		}
	}

	// ── Phase stability: suppress directional→UNK flapping ──────────
	// Directional airborne phases (CLB, DEP, ARR) shouldn't downgrade to
	// UNK just because a brief turn reverses the distance trend. Only allow
	// transitions to other meaningful phases, not to UNK.
	if latestPhase != nil && currentPhase == "UNK" {
		switch latestPhase.Phase {
		case "CLB", "DEP", "ARR":
			currentPhase = latestPhase.Phase
		}
	}

	// ── Determine if a phase change should be recorded ───────────────
	var shouldInsert bool

	if latestPhase == nil {
		// Brand new aircraft — record initial phase
		shouldInsert = true
	} else if latestPhase.Phase != currentPhase {
		// Phase changed — check for premature ground transitions
		if currentPhase == "NEW" && (latestPhase.Phase == "T/D" || latestPhase.Phase == "TAX") {
			timeSinceLastPhase := s.now().Sub(latestPhase.Timestamp).Seconds()
			if timeSinceLastPhase < float64(s.flightPhasesConfig.PhaseTransitionTimeoutSeconds) {
				// Too soon — keep the current ground phase
				currentPhase = latestPhase.Phase
			} else {
				shouldInsert = true
			}
		} else {
			shouldInsert = true
		}
	} else {
		// Phase unchanged — check inactive aircraft timeout
		if latestPhase.Phase != "NEW" {
			timeSinceLastPhase := s.now().Sub(latestPhase.Timestamp).Seconds()
			if timeSinceLastPhase > float64(s.flightPhasesConfig.PhaseChangeTimeoutSeconds) {
				currentPhase = "NEW"
				shouldInsert = true
			}
		}
	}

	return currentPhase, shouldInsert
}

// sendPhaseChangeAlerts sends WebSocket alerts for phase changes
func (s *Service) sendPhaseChangeAlerts(phaseChanges []PhaseChangeInsert, currentPhases map[string]*PhaseChange) {
	for _, change := range phaseChanges {
		aircraft, found := s.storage.GetByHex(change.Hex)
		if !found || aircraft == nil {
			continue
		}

		var previousPhase string
		if prevPhase := currentPhases[change.Hex]; prevPhase != nil {
			previousPhase = prevPhase.Phase
		}

		// Log the phase change with detailed aircraft data for debugging. The
		// distance that explains a phase is the one to the reference airport.
		distanceFromAirport := 0.0
		if lat, lon, hasPosition := aircraft.ADSB.Position(); hasPosition {
			ref := s.ref.load()
			distanceFromAirport = MetersToNM(Haversine(lat, lon, ref.Lat, ref.Lon))
		}

		if previousPhase == "" {
			s.logger.Info("New aircraft phase detected",
				logger.String("hex", aircraft.Hex),
				logger.String("flight", aircraft.Flight),
				logger.String("phase", change.Phase),
				logger.Float64("altitude", aircraft.ADSB.AltBaro.Float64()),
				logger.Float64("ground_speed", NumberOrZero(aircraft.ADSB.GS)),
				logger.Float64("vertical_rate", NumberOrZero(aircraft.ADSB.BaroRate)),
				logger.Float64("distance_from_airport", distanceFromAirport),
				logger.Bool("on_ground", aircraft.OnGround),
			)
		} else {
			s.logger.Debug("Phase change detected",
				logger.String("hex", aircraft.Hex),
				logger.String("flight", aircraft.Flight),
				logger.String("transition", previousPhase+" → "+change.Phase),
				logger.Float64("altitude", aircraft.ADSB.AltBaro.Float64()),
				logger.Float64("ground_speed", NumberOrZero(aircraft.ADSB.GS)),
				logger.Float64("vertical_rate", NumberOrZero(aircraft.ADSB.BaroRate)),
				logger.Float64("distance_from_airport", distanceFromAirport),
				logger.Bool("on_ground", aircraft.OnGround),
			)
		}

		// Send WebSocket message for phase change
		if s.wsServer != nil {
			// Create message data for phase change
			data := map[string]interface{}{
				"hex":        aircraft.Hex,
				"flight":     aircraft.Flight,
				"phase":      change.Phase,
				"prev_phase": previousPhase,
				"transition": previousPhase + " → " + change.Phase,
				"airport":    change.Airport,
				"altitude":   aircraft.ADSB.AltBaro.Float64(),
				"on_ground":  aircraft.OnGround,
				"timestamp":  change.Timestamp.Format(time.RFC3339),
			}

			// Broadcast the phase change message
			s.wsServer.Broadcast(&websocket.Message{
				Type: "phase_change",
				Data: data,
			})
		}

		// Handle special takeoff/landing phases
		if change.Phase == "T/O" {
			s.logger.Info("Aircraft TOOK OFF",
				logger.String("hex", aircraft.Hex),
				logger.String("flight", aircraft.Flight),
				logger.String("transition", previousPhase+" → T/O"),
				logger.Float64("altitude", aircraft.ADSB.AltBaro.Float64()),
				logger.Bool("on_ground", aircraft.OnGround),
			)

			// T/O phase change message is sent by the main phase change handler
		} else if change.Phase == "T/D" {
			s.logger.Info("Aircraft LANDED",
				logger.String("hex", aircraft.Hex),
				logger.String("flight", aircraft.Flight),
				logger.String("transition", previousPhase+" → T/D"),
				logger.Float64("altitude", aircraft.ADSB.AltBaro.Float64()),
				logger.Bool("on_ground", aircraft.OnGround),
			)

			// T/D phase change message is sent by the main phase change handler
		}
	}
}

// sendImmediateGroundTransitionAlerts sends immediate WebSocket alerts for ground state transitions
func (s *Service) sendImmediateGroundTransitionAlerts(phaseChanges []PhaseChangeInsert) {
	for _, change := range phaseChanges {
		// The two branches only ever differed by the line they logged (fiche E,
		// step 4, merged 12/10 under a test).
		var event string
		switch change.Phase {
		case "T/O":
			event = "Aircraft TOOK OFF (IMMEDIATE)"
		case "T/D":
			event = "Aircraft LANDED (IMMEDIATE)"
		default:
			continue
		}

		aircraft, found := s.storage.GetByHex(change.Hex)
		if !found || aircraft == nil {
			continue
		}

		// Get the previous phase for the transition message
		var previousPhase string
		phaseHistory, err := s.storage.GetPhaseHistory(change.Hex)
		if err == nil && len(phaseHistory) > 1 {
			// The first item is the current (just inserted), second is the previous
			previousPhase = phaseHistory[1].Phase
		}

		s.logger.Info(event,
			logger.String("hex", aircraft.Hex),
			logger.String("flight", aircraft.Flight),
			logger.Float64("altitude", aircraft.ADSB.AltBaro.Float64()),
			logger.String("timestamp", change.Timestamp.Format(time.RFC3339)),
		)

		if s.wsServer == nil {
			continue
		}
		s.wsServer.Broadcast(&websocket.Message{
			Type: "phase_change",
			Data: map[string]interface{}{
				"hex":        aircraft.Hex,
				"flight":     aircraft.Flight,
				"phase":      change.Phase,
				"prev_phase": previousPhase,
				"transition": previousPhase + " → " + change.Phase,
				"airport":    change.Airport,
				"altitude":   aircraft.ADSB.AltBaro.Float64(),
				"on_ground":  aircraft.OnGround,
				"timestamp":  change.Timestamp.Format(time.RFC3339),
			},
		})
	}
}

// ProcessRawData processes raw ADS-B data into aircraft objects
func (s *Service) ProcessRawData(rawData *RawAircraftData) []*Aircraft {
	s.logger.Debug("Processing raw ADS-B data",
		logger.Int("aircraft_count", len(rawData.Aircraft)),
	)

	aircraft := make([]*Aircraft, 0, len(rawData.Aircraft))
	now := s.now() // Ensure we use UTC time

	for _, raw := range rawData.Aircraft {
		// Skip aircraft without a hex identifier (unusable data)
		if raw.Hex == "" {
			continue
		}

		// Get the raw flight name and clean it
		flightRaw := raw.Flight
		flightName := strings.TrimSpace(CleanFlightName(flightRaw))

		// If flightName is empty but hex is available, try to derive tail number
		if flightName == "" && raw.Hex != "" {
			tailNumber, err := IcaoToTailNumber(raw.Hex) // Use exported function from atc_utils.go
			if err == nil && tailNumber != "" {
				flightName = tailNumber + "*" // Appended * to indicate derived tail number
				s.logger.Debug("Derived tail number from ICAO hex",
					logger.String("hex", raw.Hex),
					logger.String("tail_number", flightName))
			} else if err != nil {
				s.logger.Debug("Failed to derive tail number from ICAO hex",
					logger.String("hex", raw.Hex),
					logger.Error(err))
			}
		}

		// Determine airline from callsign only for valid flight numbers (3 letters + 1-4 numbers)
		var airlineName, airlineCountry string
		if len(flightName) >= 4 && len(flightName) <= 7 {
			// Check if the first 3 characters are letters
			firstThree := strings.ToUpper(flightName[:3])
			isAllLetters := true
			for _, c := range firstThree {
				if c < 'A' || c > 'Z' {
					isAllLetters = false
					break
				}
			}

			// Check if the remaining characters are digits (1-4 digits)
			remainingChars := flightName[3:]
			isAllDigits := true
			for _, c := range remainingChars {
				if c < '0' || c > '9' {
					isAllDigits = false
					break
				}
			}

			// Only lookup airline if it's a valid flight number (3 letters + 1-4 numbers)
			if isAllLetters && isAllDigits && len(remainingChars) >= 1 && len(remainingChars) <= 4 {
				icaoCode := firstThree
				if s.refService != nil {
					airlineName = s.refService.LookupAirline(icaoCode)
					airlineCountry = s.refService.LookupAirlineCountry(icaoCode)
				}
				s.logger.Debug("Detected valid flight number",
					logger.String("flight", flightName),
					logger.String("airline_code", icaoCode),
					logger.String("airline", airlineName))
			}
		}

		// Sensor validation and OnGround determination handled by fetchAndProcess using batch data

		aircraftStatus := "active" // Always set to active for aircraft in current ADSB data

		a := &Aircraft{
			Hex:            raw.Hex,
			Flight:         flightName,
			Airline:        airlineName,
			AirlineCountry: airlineCountry,
			Status:         aircraftStatus,
			LastSeen:       ReceiverTime(rawData, raw.SourceType, now).Add(-time.Duration(NumberOrZero(raw.Seen)) * time.Second),
			ADSB:           &raw,
		}

		a.ADSB.ATCDerived = computeATCDerivedMetrics(a.ADSB, a.Distance)

		// Trajectory-based predictions (hindcast + forecast)
		if s.trajectoryTracker != nil {
			if forecast := s.trajectoryTracker.GetForecast(raw.Hex); len(forecast) > 0 {
				a.Future = PredictionPointsToPositions(forecast)
			}
			// Hindcast: backward extrapolation before first ADS-B contact
			if hindcast := s.trajectoryTracker.GetHindcast(raw.Hex); len(hindcast) > 0 {
				a.Hindcast = PredictionPointsToPositions(hindcast)
			}
		}
		// No forecast from the tracker (a first poll, or no tracker at all):
		// the same model from this one sample, dated when it was decoded.
		if len(a.Future) == 0 {
			posAt := ReceiverTime(rawData, raw.SourceType, now).Add(-time.Duration(NumberOrZero(raw.SeenPos) * float64(time.Second)))
			a.Future = PredictionPointsToPositions(ForecastFromTarget(&raw, posAt))
		}

		aircraft = append(aircraft, a)
	}

	s.logger.Debug("Processed ADS-B data",
		logger.Int("processed_count", len(aircraft)),
	)

	return aircraft
}
