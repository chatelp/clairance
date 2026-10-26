package config

// Defaults is the configuration a file may leave unsaid.
//
// Load starts from it and decodes the file over it, so a key absent from the
// file keeps its default and a key present wins -- which is what a short
// configuration needs to mean the same as a long one. Validate then refuses
// what is out of bounds, never what is absent.
//
// Nothing here can be guessed for a station: where the receiver is, which
// airport it watches, where its ADS-B data and its streams come from, and
// whether sign-in is wanted stay in the file. Everything else is the value
// configs/config.toml.example carried when every key was written out, so that
// the example reduced to what matters loads into exactly the same Config
// (TestTheReducedExampleLoadsTheSameConfiguration). Booleans have no default
// but false, because a switch is worth seeing in the file.
func Defaults() Config {
	return Config{
		Server: ServerConfig{
			Port:            8000,
			Host:            "127.0.0.1",
			IdleTimeoutSecs: 3600,
			AudioReplay:     AudioReplayOwner,
		},
		ADSB: ADSBConfig{
			ExternalSourceURL:            "https://adsbexchange-com1.p.rapidapi.com/v2/lat/%f/lon/%f/dist/%.0f/",
			APIHost:                      "adsbexchange-com1.p.rapidapi.com",
			SearchRadiusNM:               50,
			OpenSkyBaseURL:               "https://opensky-network.org/api",
			OpenSkyTokenURL:              "https://auth.opensky-network.org/auth/realms/opensky-network/protocol/openid-connect/token",
			OpenSkyAuthMode:              "anonymous",
			OpenSkyOAuth2CredentialsPath: "configs/opensky_credentials.json",
			FetchIntervalSecs:            1,
			SignalLostTimeoutSecs:        60,
		},
		Logging: LoggingConfig{
			Level:     "info",
			Format:    "console",
			MaxSizeMB: 100,
			MaxFiles:  7,
		},
		Storage: StorageConfig{
			Type:           "sqlite",
			SQLiteBasePath: "data",
			DBRetentionGB:  DefaultDBRetentionGB,
		},
		Station: StationConfig{
			RunwayExtensionLengthNM: 10,
			AirportRangeNM:          5,
			DisplayRangeNM:          100,
		},
		Reference: ReferenceConfig{
			AircraftCSVPath:    "assets/aircraft.csv",
			AirlinesDATPath:    "assets/airlines.dat",
			AirportsCSVPath:    "assets/airports.csv",
			FrequenciesCSVPath: "assets/airport-frequencies.csv",
			RunwaysCSVPath:     "assets/runways.csv",
			NavaidsCSVPath:     "assets/navaids.csv",
		},
		Frequencies: FrequenciesConfig{
			BufferSizeKB:             8,
			ReconnectIntervalSecs:    5,
			FFmpegReconnectDelaySecs: 2,
		},
		Transcription: TranscriptionConfig{
			Backend:          BackendLocal,
			Language:         "en",
			FFmpegPath:       "ffmpeg",
			FFmpegSampleRate: 24000,
			FFmpegChannels:   1,
			FFmpegFormat:     "s16le",
			// 2 GB is one to two days on two frequencies (sidecar/README.md);
			// 30 days of raw logs is a few MB.
			AudioArchiveMaxGB: 2,
			RawLogMaxDays:     30,
			Local: LocalSTTFileConfig{
				ServerURL:             "http://127.0.0.1:8178",
				TimeoutSeconds:        60,
				StartupTimeoutSeconds: 60,
				SegmentSilenceMs:      600,
				SegmentMinMs:          400,
				SegmentMaxSeconds:     30,
				SegmentPrerollMs:      200,
				SilenceThreshold:      0.005,
			},
		},
		Auth: AuthConfig{
			SessionTTLHours:   720,
			MaxAttempts:       8,
			AttemptWindowMins: 15,
		},
		PostProcessing: PostProcessingConfig{
			Backend:              BackendLocal,
			MinDigits:            3,
			FleetLastSeenMinutes: 1,
			IntervalSeconds:      10,
			BatchSize:            20,
		},
		FlightPhases: FlightPhasesConfig{
			CruiseAltitudeFt:                18000,
			DepartureAltitudeFt:             1000,
			TaxiingMinSpeedKts:              1,
			TaxiingMaxSpeedKts:              50,
			ApproachCenterlineToleranceNM:   0.5,
			ApproachMaxDistanceNM:           10,
			ApproachMaxAltitudeFt:           5000,
			ApproachHeadingToleranceDeg:     30,
			PhaseChangeTimeoutSeconds:       3600,
			PhaseTransitionTimeoutSeconds:   60,
			PhasePreservationSeconds:        60,
			RecentTakeoffTimeoutMinutes:     30,
			AirportRangeNM:                  5,
			FlyingMinTASKts:                 50,
			FlyingMinAltFt:                  700,
			HelicopterAltMultiplier:         2,
			HighSpeedThresholdKts:           200,
			HighAltitudeOverrideFt:          5000,
			ImpossibleAltDropThresholdFt:    10000,
			ImpossibleSpeedDropThresholdKts: 100,
			ImpossibleSpeedDropMinAltFt:     5000,
			SignalLostLandingMaxAltFt:       1000,
			TrajectoryBufferDurationSec:     90,
			TrajectoryMinPoints:             5,
			TrajectoryStaleTimeoutSec:       300,
			TrajectoryCleanupIntervalSec:    30,
			TrajectoryDescentThresholdFPM:   -200,
			TrajectoryClimbThresholdFPM:     200,
			TrajectoryLevelBandFt:           200,
			TrajectoryTurningRateDeg:        1.5,
			RunwayInUseWindowMinutes:        60,
			RunwayInUseApproachWeight:       5,
			RunwayInUseLandingWeight:        3,
			RunwayInUseClimbWeight:          1.5,
			RunwayInUseDecayRate:            0.98,
		},
		Weather: WeatherConfig{
			RefreshIntervalMinutes: 10,
			APIBaseURL:             "https://node.windy.com/airports",
			RequestTimeoutSeconds:  10,
			MaxRetries:             2,
			CacheExpiryMinutes:     60,
		},
		// Off; when turned on, the station is read as often as the production
		// script read it.
		StationLink: StationLinkConfig{
			IntervalSeconds: 10,
		},
	}
}
