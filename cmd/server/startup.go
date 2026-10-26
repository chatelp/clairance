package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/yegors/co-atc/internal/adsb"
	"github.com/yegors/co-atc/internal/api"
	"github.com/yegors/co-atc/internal/config"
	"github.com/yegors/co-atc/internal/frequencies"
	"github.com/yegors/co-atc/internal/reference"
	"github.com/yegors/co-atc/internal/storage/sqlite"
	"github.com/yegors/co-atc/internal/transcription"
	"github.com/yegors/co-atc/internal/transcription/phraseology"
	"github.com/yegors/co-atc/internal/weather"
	"github.com/yegors/co-atc/internal/websocket"
	"github.com/yegors/co-atc/pkg/logger"
	"github.com/yegors/co-atc/pkg/tool"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// The steps main() takes to bring the server up, in the order it takes them.
// Each one builds a piece, starts it when that is part of building it, and
// exits the process on a failure the server cannot run without -- exactly as
// main() did when they were written inline.

// loadConfig loads and validates the configuration, and returns the path it was
// actually read from.
func loadConfig(configPath string) (*config.Config, string) {
	// Load configuration with fallback logic
	cfg, resolvedConfigPath, err := config.LoadWithFallbackAndPath(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading configuration: %v\n", err)
		os.Exit(1)
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "Invalid configuration: %v\n", err)
		os.Exit(1)
	}
	return cfg, resolvedConfigPath
}

// newLogger creates the logger from [logging].
func newLogger(cfg *config.Config) *logger.Logger {
	log, err := logger.New(logger.Config{
		Level:     cfg.Logging.Level,
		Format:    cfg.Logging.Format,
		File:      cfg.Logging.File,
		MaxSizeMB: cfg.Logging.MaxSizeMB,
		MaxFiles:  cfg.Logging.MaxFiles,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating logger: %v\n", err)
		os.Exit(1)
	}
	return log
}

// buildADSBClient creates the ADS-B client and probes its source once: a
// server that cannot read its source does not start.
func buildADSBClient(cfg *config.Config, log *logger.Logger) *adsb.Client {
	adsbClient := adsb.NewClient(
		cfg.ADSB,
		cfg.Station.Latitude,
		cfg.Station.Longitude,
		time.Duration(cfg.Server.ReadTimeoutSecs)*time.Second,
		log,
	)

	probeCtx, probeCancel := context.WithTimeout(context.Background(), 10*time.Second)
	if err := adsbClient.ValidateSource(probeCtx); err != nil {
		probeCancel()
		fatalLog := log.WithOptions(zap.AddStacktrace(zapcore.PanicLevel))
		fatalLog.Fatal("ADS-B source validation failed", logger.Error(err), logger.String("source_type", cfg.ADSB.SourceType))
	}
	probeCancel()
	log.Info("ADS-B source validation succeeded", logger.String("source_type", cfg.ADSB.SourceType))
	return adsbClient
}

// startSidecar reaches or starts the transcription sidecar, and supervises it.
// signalCtx is the one armed in main(): a signal during the health probe stops
// the wait and takes the child with it. The caller stops the sidecar.
func startSidecar(signalCtx context.Context, cfg *config.Config, log *logger.Logger) *transcription.Sidecar {
	// The other half of the product gets the same treatment. Without this the
	// server starts, serves the map and the audio, and transcribes nothing --
	// one error line per transmission and no transcript, which is a worse
	// failure than not starting at all.
	sttSidecar := transcription.NewSidecar(transcription.SidecarConfig{
		ServerURL:             cfg.Transcription.Local.ServerURL,
		Command:               cfg.Transcription.Local.Command,
		StartupTimeoutSeconds: cfg.Transcription.Local.StartupTimeoutSeconds,
	}, log)
	// Unless nothing asks for it. Then the map and the audio need no sidecar,
	// and a source that asks for transcription later has it probed then.
	if !transcriptionWanted(cfg) {
		log.Info("No source transcribes and no sidecar is configured: running without transcription",
			logger.String("server_url", cfg.Transcription.Local.ServerURL),
			logger.String("to_transcribe", "set transcribe_audio = true on a source and run a sidecar at server_url, see sidecar/README.md"))
		sttSidecar.Idle()
		return sttSidecar
	}
	sidecarCtx, sidecarCancel := context.WithTimeout(signalCtx, 5*time.Minute)
	if err := sttSidecar.Start(sidecarCtx); err != nil {
		sidecarCancel()
		fatalLog := log.WithOptions(zap.AddStacktrace(zapcore.PanicLevel))
		fatalLog.Fatal("Local transcription sidecar unavailable", logger.Error(err))
	}
	sidecarCancel()
	// A sidecar that dies later is restarted, instead of leaving the server
	// transcribing nothing until someone notices (28/09).
	sttSidecar.Supervise(signalCtx)
	return sttSidecar
}

// transcriptionWanted reports whether the server needs its sidecar from the
// start: a configured source transcribes, or [transcription.local] command asks
// for one to be run.
func transcriptionWanted(cfg *config.Config) bool {
	if len(cfg.Transcription.Local.Command) > 0 {
		return true
	}
	for _, src := range cfg.Frequencies.Sources {
		if src.TranscribeAudio {
			return true
		}
	}
	return false
}

// dailyStorage is today's database and the stores that share it.
type dailyStorage struct {
	dir            string
	aircraft       *sqlite.AircraftStorage
	transcriptions *sqlite.TranscriptionStorage
	clearances     *sqlite.ClearanceStorage
}

// openStorage opens today's database, after the retention pass that may delete
// older ones. It also returns the values the settings panel can change while
// the server runs, which that pass reads. The caller closes the database.
func openStorage(cfg *config.Config, configPath string, log *logger.Logger) (*config.Runtime, *dailyStorage) {
	// Generate today's database filename
	today := time.Now().Format("2006-01-02")
	dbFilename := fmt.Sprintf("co-atc-%s.db", today)
	dbPath := filepath.Join(cfg.Storage.SQLiteBasePath, dbFilename)

	// Ensure the directory exists
	dbDir := cfg.Storage.SQLiteBasePath
	if err := os.MkdirAll(dbDir, 0755); err != nil {
		log.Error("Failed to create database directory", logger.Error(err), logger.String("path", dbDir))
		os.Exit(1)
	}

	// Values that can be changed while the server runs, from the settings panel.
	// Read before the first retention pass, so a size set from the panel is the
	// one that pass obeys, not the configured default.
	runtimeSettings := config.NewRuntime(cfg, configPath, log)

	if cfg.Storage.DBRetentionDays > 0 {
		log.Warn("db_retention_days is no longer used: the daily databases are kept within db_retention_gb",
			logger.Int("db_retention_days", cfg.Storage.DBRetentionDays),
			logger.Float64("db_retention_gb", runtimeSettings.DBRetentionGB()))
	}
	if err := cleanupOldDailyDatabases(dbDir, dbPath, runtimeSettings.DBRetentionGB(), log); err != nil {
		log.Warn("Failed to clean up old database files",
			logger.Error(err),
			logger.String("path", dbDir),
			logger.Float64("retention_gb", runtimeSettings.DBRetentionGB()))
	}

	log.Info("Using daily database", logger.String("path", dbPath))

	// Create SQLite storage with no retention settings
	sqliteStorage, err := sqlite.NewAircraftStorage(
		dbPath,
		log,
	)
	if err != nil {
		log.Error("Failed to create SQLite storage", logger.Error(err))
		os.Exit(1)
	}
	log.Info("Using SQLite storage", logger.String("path", dbPath))

	// Create transcription storage
	transcriptionStorage := sqlite.NewTranscriptionStorage(sqliteStorage.GetDB(), log)

	// Create clearance storage
	clearanceStorage := sqlite.NewClearanceStorage(sqliteStorage.GetDB(), log)

	return runtimeSettings, &dailyStorage{
		dir:            dbDir,
		aircraft:       sqliteStorage,
		transcriptions: transcriptionStorage,
		clearances:     clearanceStorage,
	}
}

// buildADSBService creates the ADS-B service on today's database, broadcasting
// to the WebSocket hub. It is started later, once the reference data is in.
func buildADSBService(cfg *config.Config, adsbClient *adsb.Client, store *dailyStorage, wsServer *websocket.Server, log *logger.Logger) *adsb.Service {
	return adsb.NewService(
		adsbClient,
		store.aircraft,
		time.Duration(cfg.ADSB.FetchIntervalSecs)*time.Second,
		log,
		cfg.Station,
		cfg.ADSB,
		cfg.FlightPhases,
		wsServer,
	)
}

// refAdapter wraps reference.Service to implement adsb.ReferenceService interface
type refAdapter struct {
	service *reference.Service
}

func (a *refAdapter) LookupAircraft(hex string) *adsb.ReferenceAircraftInfo {
	info := a.service.LookupAircraft(hex)
	if info == nil {
		return nil
	}
	return &adsb.ReferenceAircraftInfo{
		Hex:               info.Hex,
		Registration:      info.Registration,
		TypeCode:          info.TypeCode,
		ManufacturerModel: info.ManufacturerModel,
		Year:              info.Year,
		Owner:             info.Owner,
	}
}

func (a *refAdapter) LookupAirline(code string) string {
	return a.service.LookupAirline(code)
}

func (a *refAdapter) LookupAirlineCountry(code string) string {
	return a.service.LookupAirlineCountry(code)
}

// loadReference loads the reference data and puts the reference airport in
// force. It returns the service as the loader gave it, error or not; the
// reference airport, nil without reference data; and the airport code the
// weather is fetched for.
func loadReference(cfg *config.Config, runtimeSettings *config.Runtime, adsbService *adsb.Service, log *logger.Logger) (*reference.Service, *referenceAirport, string) {
	// Load reference data (aircraft, airlines, airports, runways, navaids)
	refService, err := reference.NewService(reference.ServiceConfig{
		AircraftCSVPath:    cfg.Reference.AircraftCSVPath,
		AirlinesDATPath:    cfg.Reference.AirlinesDATPath,
		AirportsCSVPath:    cfg.Reference.AirportsCSVPath,
		FrequenciesCSVPath: cfg.Reference.FrequenciesCSVPath,
		RunwaysCSVPath:     cfg.Reference.RunwaysCSVPath,
		NavaidsCSVPath:     cfg.Reference.NavaidsCSVPath,
		StationLat:         cfg.Station.Latitude,
		StationLon:         cfg.Station.Longitude,
		HomeAirportCode:    cfg.Station.AirportCode,
		DisplayRangeNM:     cfg.Station.DisplayRangeNM,
		ExtensionLengthNM:  cfg.Station.RunwayExtensionLengthNM,
	}, log)
	// The reference airport: the one saved from the settings panel, else
	// [station] airport_code. Without reference data, phases keep being judged
	// from the receiver, as upstream did.
	var refAirport *referenceAirport
	effectiveAirport := cfg.Station.AirportCode
	if err != nil {
		log.Warn("Failed to load reference data", logger.Error(err))
	} else {
		adsbService.SetReferenceService(&refAdapter{service: refService})
		refAirport = &referenceAirport{ref: refService, adsb: adsbService, log: log}
		inForce := refAirport.start(runtimeSettings.Settings().ReferenceAirports(), cfg.Station.AirportCode)
		runtimeSettings.UseReferenceAirports(inForce)
		effectiveAirport = inForce[0]
		log.Info("Reference data loaded",
			logger.Int("aircraft_count", refService.AircraftCount()),
			logger.Int("airline_count", refService.AirlineCount()))
	}
	return refService, refAirport, effectiveAirport
}

// startWeather creates the weather service for the airport in force, lets the
// settings panel change that airport, and starts the service.
func startWeather(cfg *config.Config, effectiveAirport string, refAirport *referenceAirport, runtimeSettings *config.Runtime, log *logger.Logger) *weather.Service {
	// Create weather service
	weatherConfigConverted := weather.ConfigWeatherConfig{
		RefreshIntervalMinutes: cfg.Weather.RefreshIntervalMinutes,
		APIBaseURL:             cfg.Weather.APIBaseURL,
		RequestTimeoutSeconds:  cfg.Weather.RequestTimeoutSeconds,
		MaxRetries:             cfg.Weather.MaxRetries,
		FetchMETAR:             cfg.Weather.FetchMETAR,
		FetchTAF:               cfg.Weather.FetchTAF,
		FetchNOTAMs:            cfg.Weather.FetchNOTAMs,
		CacheExpiryMinutes:     cfg.Weather.CacheExpiryMinutes,
	}
	weatherService := weather.NewService(weatherConfigConverted, effectiveAirport, log)

	// From here the panel can change the reference airport: the three services
	// that hold a piece of it exist.
	if refAirport != nil {
		refAirport.weather = weatherService
		runtimeSettings.SetReferenceAirportHook(config.ReferenceAirportHook{
			Validate: refAirport.validate,
			Apply:    refAirport.apply,
		})
	}

	// Start weather service
	if err := weatherService.Start(); err != nil {
		log.Error("Failed to start weather service", logger.Error(err))
		os.Exit(1)
	}
	return weatherService
}

// startFrequencies creates the frequencies service -- the audio streams, their
// transcription and the association of what is said with the aircraft -- and
// starts it.
func startFrequencies(ctx context.Context, cfg *config.Config, adsbService *adsb.Service, refService *reference.Service, runtimeSettings *config.Runtime, wsServer *websocket.Server, store *dailyStorage, sttSidecar *transcription.Sidecar, log *logger.Logger) *frequencies.Service {
	// Create frequencies service.
	// The fleet adapter is what lets a spoken callsign be attached to a real
	// target.
	fleet := &adsbFleet{service: adsbService, lastSeenMinutes: cfg.PostProcessing.FleetLastSeenMinutes}

	// Said once at start rather than once per stream: without ffmpeg no
	// frequency has audio, and on 26/10 that was a line per stream asked for,
	// lost among the rest. Sources can arrive later (PUT /sources), so asked
	// whether or not any is configured yet.
	if p, ok := tool.Find(cfg.Transcription.FFmpegPath); ok {
		log.Info("ffmpeg found", logger.String("path", p))
	} else {
		log.Error("ffmpeg not found: no frequency will have audio",
			logger.String("ffmpeg_path", cfg.Transcription.FFmpegPath), logger.String("PATH", os.Getenv("PATH")),
			logger.String("looked_in", strings.Join(tool.Dirs, ":")))
	}

	// What the radio has said about each aircraft, carried on the aircraft itself
	// so the map can show which targets the controller is actually talking to.
	adsbService.SetVoiceIndex(newVoiceIndex(context.Background(), store.transcriptions, 5*time.Second, log))
	frequenciesService := frequencies.NewService(cfg, log, wsServer, store.transcriptions, store.clearances, fleet)
	// Each frequency's sector: its airport and kind, the sizes from the settings
	// panel, the airport's position from the reference data. Off unless the
	// panel says so, and for a frequency that names no airport.
	frequenciesService.SetSectors(func(id string) (phraseology.Sector, bool) {
		m := runtimeSettings.Matching()
		if !m.Sectors {
			return phraseology.Sector{}, false
		}
		code, kind := frequenciesService.SectorOf(id)
		size, known := m.Sector[kind]
		if code == "" || !known || refService == nil {
			return phraseology.Sector{}, false // no reference data: no airport to draw around
		}
		ap := refService.GetAirport(code)
		if ap == nil {
			return phraseology.Sector{}, false
		}
		return phraseology.Sector{Lat: ap.Latitude, Lon: ap.Longitude, RadiusNM: size.RadiusNM, MaxAltFt: size.MaxAltFt}, true
	})
	// The association rules from the settings panel, read for every transmission.
	frequenciesService.SetMatchingRules(func() phraseology.Rules {
		m := runtimeSettings.Matching()
		return phraseology.Rules{Letters: m.Letters, ApproxOperators: m.ApproxOperators,
			MinDigits: m.MinDigits, OneDigitOff: m.OneDigitOff,
			ContextLetters: m.ContextLetters, ContextDigits: m.ContextDigits, ContextNames: m.ContextNames,
			PrefixDigits: m.PrefixDigits, MinScore: m.MinScore}
	})

	// A source added while the server runs and asking for transcription is
	// refused when nothing answers for it, instead of being transcribed by
	// nobody.
	frequenciesService.SetTranscriptionGate(sttSidecar.Ensure)

	// Start frequencies service
	if err := frequenciesService.Start(ctx); err != nil {
		log.Error("Failed to start frequencies service", logger.Error(err))
		os.Exit(1)
	}
	return frequenciesService
}

// buildRouter creates the API router and attaches what only main() knows: the
// live settings, the database file actually opened, the sidecar.
func buildRouter(cfg *config.Config, adsbService *adsb.Service, frequenciesService *frequencies.Service, weatherService *weather.Service, refService *reference.Service, wsServer *websocket.Server, store *dailyStorage, runtimeSettings *config.Runtime, sttSidecar *transcription.Sidecar, log *logger.Logger) *api.Router {
	// Create API router
	router := api.NewRouter(adsbService, frequenciesService, weatherService, refService, cfg, log, wsServer, store.transcriptions, store.clearances)
	router.Handler().AttachRuntime(runtimeSettings, store.aircraft.GetDB(), sttSidecar)
	router.Handler().SetBuild(version, commit)
	return router
}
