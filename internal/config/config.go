package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/yegors/co-atc/internal/auth"
)

// Config represents the main application configuration structure
// containing all configuration sections
type Config struct {
	// ConfigPath is where this configuration was read from. Not a TOML field:
	// it is filled in by the loader, so that anything written beside the
	// configuration -- runtime settings, accounts -- lands in the same place.
	ConfigPath string `toml:"-"`

	// Warnings is what Load has to say about the file and cannot log itself,
	// the logger being built from this very configuration: a key written in
	// two sections, a key nothing reads any more. One line each, naming the
	// key; main prints them once the logger exists. Not a TOML field.
	Warnings []string `toml:"-"`

	Server         ServerConfig         `toml:"server"`          // HTTP server settings
	ADSB           ADSBConfig           `toml:"adsb"`            // Aircraft tracking data source settings
	Frequencies    FrequenciesConfig    `toml:"frequencies"`     // Radio frequency monitoring settings
	Logging        LoggingConfig        `toml:"logging"`         // Application logging settings
	Storage        StorageConfig        `toml:"storage"`         // Data persistence settings
	Station        StationConfig        `toml:"station"`         // Physical location settings
	Reference      ReferenceConfig      `toml:"reference"`       // Reference data (aircraft, airlines, airports, runways, navaids)
	Transcription  TranscriptionConfig  `toml:"transcription"`   // Audio transcription settings
	PostProcessing PostProcessingConfig `toml:"post_processing"` // Post-processing settings for transcriptions
	FlightPhases   FlightPhasesConfig   `toml:"flight_phases"`   // Flight phase detection settings
	Weather        WeatherConfig        `toml:"wx"`              // Weather data fetching and caching settings
	Auth           AuthConfig           `toml:"auth"`            // Who may use this server; see internal/auth
	StationLink    StationLinkConfig    `toml:"station_link"`    // radio-ctl-sync, run by co-atc; see docs-fr/36-contrat-lanceur.md
}

// AuthConfig decides who may reach the data.
//
// Disabled by default, and that default is deliberate: this file ships in a public
// repository, and someone cloning it must not find themselves locked out of their
// own receiver by a password they never set. Turning it on is one line plus one
// account, created with: co-atc -add-user <name>
type AuthConfig struct {
	Enabled           bool         `toml:"enabled"`
	SessionTTLHours   int          `toml:"session_ttl_hours"` // how long a session lives without use
	MaxAttempts       int          `toml:"max_attempts"`      // wrong passwords allowed per address, per window
	AttemptWindowMins int          `toml:"attempt_window_minutes"`
	UsageDays         int          `toml:"usage_days"` // days of each account's usage kept for the owner (D84); 0 keeps none
	Users             []UserConfig `toml:"users"`
}

// UserConfig is one account. The password itself never appears here -- only its
// Argon2id hash, produced by co-atc -add-user, which reads the password without
// echoing it and never writes it anywhere.
type UserConfig struct {
	Name         string `toml:"name"`
	PasswordHash string `toml:"password_hash"`
	Role         string `toml:"role"` // "owner" (the default) or "viewer", who reads and changes nothing
}

// ServerConfig contains HTTP server configuration settings
type ServerConfig struct {
	Port             int    `toml:"port"`                  // Primary HTTP port for the server
	Host             string `toml:"host"`                  // Host address to bind to (e.g., 127.0.0.1 for localhost only, 0.0.0.0 for all interfaces)
	ReadTimeoutSecs  int    `toml:"read_timeout_seconds"`  // Maximum duration for reading the entire request (0 = no timeout)
	WriteTimeoutSecs int    `toml:"write_timeout_seconds"` // Maximum duration for writing the response (0 = no timeout, recommended for streaming)
	IdleTimeoutSecs  int    `toml:"idle_timeout_seconds"`  // Maximum duration to wait for the next request when keep-alives are enabled
	AdditionalPorts  []int  `toml:"additional_ports"`      // Additional HTTP ports to listen on (useful for multiple interfaces)

	// A directory to serve the page from instead of the one built into the
	// binary (package www), to work on the page without rebuilding: "www" serves
	// the working tree, an edit live at the next reload. Empty: the built-in
	// page, that of the binary's own commit.
	StaticDir string `toml:"static_dir"`

	// TLS served by co-atc itself. Leave both empty behind a reverse proxy that
	// terminates TLS; fill them to serve HTTPS with no proxy at all. Neither shape
	// is privileged: the standalone install must be as usable as the proxied one.
	TLSCert string `toml:"tls_cert"`
	TLSKey  string `toml:"tls_key"`

	// Reverse proxies whose X-Forwarded-Proto and X-Forwarded-For may be believed,
	// as CIDRs. EMPTY BY DEFAULT, and that matters: a forwarded header trusted from
	// anyone lets any client on the network declare itself already on HTTPS, or
	// wear another address to escape the login rate limit.
	TrustedProxies []string `toml:"trusted_proxies"`

	// Pages served from elsewhere that may call this server with the signed-in
	// session, as origins (scheme://host[:port]). The server itself -- under
	// whatever name the browser reaches it, on any of its ports -- is always
	// allowed and is not listed; any other page gets no CORS header and no
	// WebSocket. EMPTY BY DEFAULT. "*" is refused.
	AllowedOrigins []string `toml:"allowed_origins"`

	// Who may listen to the receiver through the server -- the live streams
	// and the archived clip of each transmission: "owner" (the default), or
	// "all", viewer accounts included. See internal/auth/role.go.
	AudioReplay string `toml:"audio_replay"`

	// Instance names this server for whoever starts it -- "production", "test"
	// -- in the launcher's health (docs-fr/36). The launcher recognised
	// production by the path of its configuration until 26/10.
	Instance string `toml:"instance"`

	// KeepAwake keeps the Mac from sleeping while the server runs, as the
	// production script's caffeinate did: co-atc runs `caffeinate -i -w <its
	// pid>` beside itself. macOS only; elsewhere it is said once and ignored.
	KeepAwake bool `toml:"keep_awake"`
}

// StationLinkConfig is the [station_link] section: the link with a radio-ctl
// station (cmd/radio-ctl-sync), which co-atc starts, keeps running and stops
// with itself (docs-fr/36-contrat-lanceur.md). Off by default: a server
// without such a station sees nothing change.
type StationLinkConfig struct {
	Enabled bool `toml:"enabled"`

	// Program is radio-ctl-sync; empty looks next to co-atc's own binary, then
	// on the PATH and where Homebrew installs.
	Program string `toml:"program"`

	StationURL      string `toml:"station_url"`      // radio-ctl's base URL
	StationHost     string `toml:"station_host"`     // the Host header radio-ctl expects, if any
	StreamsURL      string `toml:"streams_url"`      // base URL of the station's Icecast mounts
	MixID           string `toml:"mix_id"`           // a mixed stream whose label follows the station
	IntervalSeconds int    `toml:"interval_seconds"` // how often the station is read
}

// The two answers to server.audio_replay.
const (
	AudioReplayOwner = "owner"
	AudioReplayAll   = "all"
)

// ADSBConfig contains ADS-B aircraft tracking data source configuration
type ADSBConfig struct {
	// Source selection
	SourceType string `toml:"source_type"` // Data source type: "external-rapidapi", "external-opensky", "tar1090", "readsb-api", or "readsb-file"

	// External API source settings (used when source_type = "external-rapidapi")
	ExternalSourceURL string `toml:"external_source_url"` // URL template for external API with format placeholders for lat, lon, and distance
	APIHost           string `toml:"api_host"`            // API host header value (e.g., for RapidAPI)
	APIKey            string `toml:"api_key"`             // API key for authentication with external service
	SearchRadiusNM    int    `toml:"search_radius_nm"`    // Search radius in nautical miles for external API queries

	// OpenSky source settings (used when source_type = "external-opensky")
	OpenSkyBaseURL               string `toml:"opensky_base_url"`                // OpenSky API base URL (e.g. https://opensky-network.org/api)
	OpenSkyTokenURL              string `toml:"opensky_token_url"`               // OAuth2 token endpoint URL
	OpenSkyAuthMode              string `toml:"opensky_auth_mode"`               // Auth mode: "anonymous" or "oauth2"
	OpenSkyOAuth2CredentialsPath string `toml:"opensky_oauth2_credentials_path"` // Path to OAuth2 client credentials JSON file

	// Tar1090 source settings (used when source_type = "tar1090")
	Tar1090BaseURL string `toml:"tar1090_base_url"` // Base URL where aircraft.json, receiver.json, stats.json are served

	// readsb API settings (used when source_type = "readsb-api")
	ReadsbAPIURL string `toml:"readsb_api_url"` // Full readsb API URL (e.g., http://host:30152/?all)

	// readsb file settings (used when source_type = "readsb-file")
	ReadsbDataDir string `toml:"readsb_data_dir"` // Optional override directory for readsb runtime files (auto-detected when empty)

	// Common settings
	FetchIntervalSecs     int `toml:"fetch_interval_seconds"`      // How often to fetch new aircraft data (in seconds)
	SignalLostTimeoutSecs int `toml:"signal_lost_timeout_seconds"` // Time after which aircraft is marked as signal_lost (in seconds, default: 60)
}

// LoggingConfig contains application logging configuration
type LoggingConfig struct {
	Level  string `toml:"level"`  // Log level: "debug", "info", "warn", or "error"
	Format string `toml:"format"` // Log format: "json" (structured) or "console" (human-readable)

	// Where to keep the log, and how much of it. Empty file means stdout only,
	// which is upstream's behaviour and leaves the size unbounded. One file per
	// day, a ceiling per file for the day that goes wrong, and a count so the
	// whole thing stops growing. Measured here: one server writes 17 MB a day.
	File      string `toml:"file"`
	MaxSizeMB int    `toml:"max_size_mb"`
	MaxFiles  int    `toml:"max_files"`
}

// StorageConfig contains data persistence configuration
type StorageConfig struct {
	Type           string `toml:"type"`             // Storage backend type (currently only "sqlite" is supported)
	SQLiteBasePath string `toml:"sqlite_base_path"` // Base path for SQLite database files (actual filename will be generated as co-atc-YYYY-MM-DD.db)
	// DBRetentionGB caps the daily databases together: past it the oldest is
	// deleted, today's never. A size rather than a number of days (28/09): with a
	// server run on demand, days said nothing about what a disk can hold, and a
	// restart after a week's pause deleted every file at once.
	DBRetentionGB float64 `toml:"db_retention_gb"`
	// ArchiveDir, when set, receives each day the retention lets go, gzipped
	// and checked, before it is deleted (28/10, D85); a directory that cannot
	// be found deletes nothing. Empty: the days past the retention are deleted.
	ArchiveDir string `toml:"archive_dir"`
	// DBRetentionDays is no longer used; kept so an old configuration still
	// loads, and says so at startup.
	DBRetentionDays int `toml:"db_retention_days"`
}

// StationFilesDir is where the page's files that belong to this station are:
// what is made for one station and cannot be built into the binary -- today
// the airport layout, written by tools/airport-layout/fetch.py. A directory of
// its own beside the databases, served as the page's /data/: nothing else of
// the data directory, the databases least of all, is ever served.
func (c *Config) StationFilesDir() string {
	return filepath.Join(c.Storage.SQLiteBasePath, "station")
}

// DefaultDBRetentionGB is the retention when none is configured: 8 to 11 full
// days of a continuously running server (1.8 GB measured on 24/09, 2.6 GB a day
// at the busiest rate, docs-fr D32), months of one run on demand.
const DefaultDBRetentionGB = 20.0

// StationConfig contains physical location configuration for the monitoring station
type StationConfig struct {
	Latitude                float64 `toml:"latitude"`                   // Latitude of the station in decimal degrees (-90 to 90)
	Longitude               float64 `toml:"longitude"`                  // Longitude of the station in decimal degrees (-180 to 180)
	ElevationFeet           int     `toml:"elevation_feet"`             // Elevation of the station above sea level in feet
	AirportCode             string  `toml:"airport_code"`               // ICAO code of the airport (e.g., "CYYZ")
	RunwayExtensionLengthNM float64 `toml:"runway_extension_length_nm"` // Length of runway extensions in nautical miles
	AirportRangeNM          float64 `toml:"airport_range_nm"`           // Range in nautical miles to consider aircraft as being at this airport (default: 5.0); also what the flight phases use
	DisplayRangeNM          float64 `toml:"display_range_nm"`           // Range in nautical miles for displaying airports, runways, and navaids on map (default: 100.0)
}

// ReferenceConfig contains paths to reference data CSV files
type ReferenceConfig struct {
	AircraftCSVPath    string `toml:"aircraft_csv_path"`            // Path to aircraft.csv (wiedehopf/tar1090-db)
	AirlinesDATPath    string `toml:"airlines_dat_path"`            // Path to airlines.dat (OpenFlights)
	AirportsCSVPath    string `toml:"airports_csv_path"`            // Path to airports.csv (OurAirports)
	FrequenciesCSVPath string `toml:"airport_frequencies_csv_path"` // Path to airport-frequencies.csv (OurAirports)
	RunwaysCSVPath     string `toml:"runways_csv_path"`             // Path to runways.csv (OurAirports)
	NavaidsCSVPath     string `toml:"navaids_csv_path"`             // Path to navaids.csv (OurAirports)
}

// TranscriptionConfig contains settings for audio transcription.
//
// Transcription is local: whole transmissions are posted to the sidecar in
// sidecar/, which honours upstream's docs/LOCAL-STT.md, and no audio leaves
// the machine. The OpenAI realtime backend was removed (D3, D71); its keys in
// an older configuration -- openai_api_key, model, prompt_path,
// noise_reduction, the VAD and retry settings -- are ignored.
type TranscriptionConfig struct {
	// Backend must be "local" or empty. "openai" is refused at startup rather
	// than ignored: a server that silently transcribed nothing would be worse.
	Backend string             `toml:"backend"`
	Local   LocalSTTFileConfig `toml:"local"`

	// Expected language when the frequency catalogue gives none (e.g. "en").
	Language string `toml:"language"`

	// File logging settings
	LogDir string `toml:"log_dir"` // Optional directory for transcription log files (append-only logs by date and frequency)

	// What is kept of the files that grow every day outside the databases, swept
	// at start and every hour (cmd/server/retention.go); today's are never
	// deleted. AudioArchiveMaxGB caps the sidecar's archive of clips
	// (Local.AudioArchiveDir, or its --save-audio), oldest clips first;
	// RawLogMaxDays is the age past which LogDir/raw/{date}_{frequency}.log
	// goes. 0 keeps everything.
	AudioArchiveMaxGB float64 `toml:"audio_archive_max_gb"`
	RawLogMaxDays     int     `toml:"raw_log_max_days"`

	// SampleRate is the rate, in Hz, of the PCM every source is brought to:
	// what the page plays as WAV, what the transcription cuts and the sidecar
	// reads. Always mono 16-bit (internal/audio). Its older name,
	// ffmpeg_sample_rate, from when ffmpeg decoded the streams, is still read
	// when written, and said at start (B1 ter).
	SampleRate int `toml:"sample_rate"`
}

// BackendLocal is the one transcription and post-processing backend.
const BackendLocal = "local"

// removedBackend is what an older configuration may still name.
const removedBackend = "openai"

// PostProcessingConfig is the second stage of the transcription pipeline: the
// one that assigns a speaker, a callsign and any clearance to a stored
// transcription. It is the phraseology grammar matched against live ADS-B, and
// needs no key; see docs-fr/17-appariement.md. Upstream's GPT-4o pass was
// removed with the OpenAI backend; its keys (model, context_transcriptions,
// system_prompt_path, timeout_seconds) are ignored.
type PostProcessingConfig struct {
	Enabled         bool `toml:"enabled"`          // Enable or disable post-processing
	IntervalSeconds int  `toml:"interval_seconds"` // How often to run the post-processing (in seconds)
	BatchSize       int  `toml:"batch_size"`       // Maximum number of transcriptions to process in each batch

	// Backend must be "local" or empty; "openai" is refused at startup.
	Backend              string  `toml:"backend"`
	AirlinesDatPath      string  `toml:"airlines_dat_path"`       // Filled from [reference] airlines_dat_path; its own key is the older home, still read with a warning
	MinScore             float64 `toml:"min_score"`               // below this, the matcher refuses rather than guesses
	MinDigits            int     `toml:"min_digits"`              // shortest spoken number that may be a flight number
	FleetLastSeenMinutes int     `toml:"fleet_last_seen_minutes"` // how stale an ADS-B target may be and still be a candidate
}

// FrequenciesConfig contains settings for radio frequency monitoring
type FrequenciesConfig struct {
	Sources               []FrequencyConfig `toml:"sources"`                 // List of radio frequencies to monitor
	BufferSizeKB          int               `toml:"buffer_size_kb"`          // Audio buffer size in kilobytes
	ReconnectIntervalSecs int               `toml:"reconnect_interval_secs"` // Seconds to wait before reconnecting after stream failure
}

// LocalSTTFileConfig is the [transcription.local] section of config.toml.
type LocalSTTFileConfig struct {
	ServerURL      string `toml:"server_url"`      // Base URL of the sidecar
	TimeoutSeconds int    `toml:"timeout_seconds"` // HTTP timeout for one transmission

	// Command starts the sidecar with the server and stops it on exit. Leave it
	// empty to use a sidecar you run yourself at ServerURL -- upstream's
	// docs/LOCAL-STT.md specifies a URL precisely so it may live elsewhere.
	// Either way the server probes /health at startup and refuses to run half
	// deaf. Measured on this machine: a cold sidecar answers in 1.6 s.
	Command               []string `toml:"command"`
	StartupTimeoutSeconds int      `toml:"startup_timeout_seconds"`

	// Where the sidecar keeps the audio of each transmission (its --save-audio),
	// so that a transmission can be listened to again from the page. Empty: the
	// --save-audio of Command, if any. See internal/audioarchive.
	AudioArchiveDir string `toml:"audio_archive_dir"`

	// Where one transmission begins and ends. An SDR feed is digitally silent
	// between transmissions, so these defaults suit it; a noisier source needs a
	// higher silence_threshold and relies on segment_max_seconds as a backstop.
	SegmentSilenceMs  int     `toml:"segment_silence_ms"`
	SegmentMinMs      int     `toml:"segment_min_ms"`
	SegmentMaxSeconds int     `toml:"segment_max_seconds"`
	SegmentPrerollMs  int     `toml:"segment_preroll_ms"`
	SilenceThreshold  float64 `toml:"silence_threshold"`
}

// FrequencyConfig contains configuration for a single monitored radio frequency
type FrequencyConfig struct {
	ID              string  `toml:"id"`               // Unique identifier for this frequency
	Airport         string  `toml:"airport"`          // ICAO code of the airport (e.g., "CYYZ" for Toronto Pearson)
	Name            string  `toml:"name"`             // Human-readable name (e.g., "CYYZ Tower")
	FrequencyMHz    float64 `toml:"frequency_mhz"`    // Actual radio frequency in MHz (e.g., 118.7)
	URL             string  `toml:"url"`              // URL to the audio stream
	Order           int     `toml:"order"`            // Display order in the UI (lower numbers first)
	TranscribeAudio bool    `toml:"transcribe_audio"` // Whether to transcribe audio for this frequency

	// Expected language of this frequency, e.g. "en" or "fr". Taken from the
	// frequency catalogue and passed to the local backend, which honours it
	// rather than detecting: measured detection here is wrong on 16% of French
	// and 28% of English transmissions. Empty falls back to [transcription] language.
	Language string `toml:"language"`

	// PCMRate and PCMChannels describe a source of raw PCM over UDP (url
	// "udp://127.0.0.1:7355"), as GQRX, SDR++ or rtl_fm send it: signed 16-bit
	// little-endian samples at PCMRate Hz, PCMChannels of them interleaved, 1
	// when unset. Nothing in the packets says it, so the source must. Unused
	// for an MP3 stream over HTTP, which says its own format. They replace
	// ffmpeg_input_options (["-f", "s16le", "-ar", "48000", "-ac", "1"]),
	// still read when written, and said at start (B1 ter).
	PCMRate     int `toml:"pcm_rate"`
	PCMChannels int `toml:"pcm_channels"`

	// SectorAirport and SectorKind say which aircraft this frequency can be
	// talking to: those near that airport (ICAO code) within the radius and
	// below the ceiling the settings give for its kind -- "approach",
	// "departure", "tower" or "ground". Empty: the whole sky, as upstream.
	SectorAirport string `toml:"sector_airport"`
	SectorKind    string `toml:"sector_kind"`
}

// SectorKinds are the kinds of frequency a sector can be drawn for.
var SectorKinds = map[string]bool{"approach": true, "departure": true, "tower": true, "ground": true}

// FlightPhasesConfig contains settings for flight phase detection
type FlightPhasesConfig struct {
	Enabled                       bool    `toml:"enabled"`                          // Enable enhanced flight phase detection
	CruiseAltitudeFt              int     `toml:"cruise_altitude_ft"`               // Minimum altitude for cruise phase
	DepartureAltitudeFt           int     `toml:"departure_altitude_ft"`            // Minimum altitude for departure phase
	TaxiingMinSpeedKts            int     `toml:"taxiing_min_speed_kts"`            // Minimum ground speed for taxiing
	TaxiingMaxSpeedKts            int     `toml:"taxiing_max_speed_kts"`            // Maximum ground speed for taxiing
	ApproachCenterlineToleranceNM float64 `toml:"approach_centerline_tolerance_nm"` // Distance from runway centerline
	ApproachMaxDistanceNM         int     `toml:"approach_max_distance_nm"`         // Maximum distance from runway threshold
	ApproachMaxAltitudeFt         int     `toml:"approach_max_altitude_ft"`         // Maximum altitude for approach phase detection
	ApproachHeadingToleranceDeg   float64 `toml:"approach_heading_tolerance_deg"`   // Heading alignment tolerance

	// Timeout configurations - grouped together for clarity
	// These control different aspects of phase transition timing:

	// 1. Long-term inactive aircraft cleanup (default: 3600 seconds = 1 hour)
	// Aircraft that haven't had ANY phase change for this duration revert to NEW
	// This is the ultimate cleanup for parked/inactive aircraft
	PhaseChangeTimeoutSeconds int `toml:"phase_change_timeout_seconds"`

	// 2. Ground phase transition prevention (default: 60 seconds, but you may want 1800 = 30 min)
	// Prevents TAX→NEW and T/D→NEW transitions when aircraft briefly stops
	// Helps maintain phase stability during normal ground operations
	PhaseTransitionTimeoutSeconds int `toml:"phase_transition_timeout_seconds"`

	// 3. Critical phase preservation (default: 60 seconds)
	// Keeps T/O and T/D phases visible for at least this duration
	// Ensures pilots/controllers can see these important events
	PhasePreservationSeconds int `toml:"phase_preservation_seconds"`

	// 4. Recent takeoff detection window (default: 30 minutes)
	// How long after takeoff an aircraft is considered "recently departed"
	// Used for DEP phase eligibility
	RecentTakeoffTimeoutMinutes int `toml:"recent_takeoff_timeout_minutes"`

	// Other phase detection parameters
	AirportRangeNM float64 `toml:"airport_range_nm"` // Distance considered "close to airport"; filled from [station] airport_range_nm, its own key is the older home, still read with a warning

	// Ground detection thresholds (NEW - making existing constants configurable)
	FlyingMinTASKts         float64 `toml:"flying_min_tas_kts"`        // Minimum true airspeed to be considered flying
	FlyingMinAltFt          float64 `toml:"flying_min_alt_ft"`         // Minimum altitude to be considered flying
	HelicopterAltMultiplier float64 `toml:"helicopter_alt_multiplier"` // Multiplier for helicopter detection
	HighSpeedThresholdKts   float64 `toml:"high_speed_threshold_kts"`  // Speed above which aircraft is always considered flying
	HighAltitudeOverrideFt  float64 `toml:"high_altitude_override_ft"` // Altitude above which aircraft is always considered flying (handles bad speed data)

	// Enhanced sensor validation thresholds (NEW)
	ImpossibleAltDropThresholdFt    float64 `toml:"impossible_alt_drop_threshold_ft"`    // Altitude above which drops to zero are always considered sensor errors
	ImpossibleSpeedDropThresholdKts float64 `toml:"impossible_speed_drop_threshold_kts"` // Speed above which drops to zero at altitude are considered sensor errors
	ImpossibleSpeedDropMinAltFt     float64 `toml:"impossible_speed_drop_min_alt_ft"`    // Minimum altitude for impossible speed drop detection

	// Signal lost landing detection
	SignalLostLandingEnabled  bool    `toml:"signal_lost_landing_enabled"`    // Enable automatic landing detection for signal lost aircraft
	SignalLostLandingMaxAltFt float64 `toml:"signal_lost_landing_max_alt_ft"` // Max altitude for signal lost landing detection

	// Trajectory-based phase detection
	// The trajectory system maintains a rolling window of recent ADS-B observations per
	// aircraft and uses statistical analysis (linear regression, median filtering) to make
	// noise-resistant phase decisions instead of relying on single data points.
	TrajectoryBufferDurationSec   int     `toml:"trajectory_buffer_duration_sec"`   // Seconds of ADS-B history per aircraft (default: 90)
	TrajectoryMinPoints           int     `toml:"trajectory_min_points"`            // Min valid points before full trajectory analysis (default: 5)
	TrajectoryStaleTimeoutSec     int     `toml:"trajectory_stale_timeout_sec"`     // Remove aircraft not seen for this long (default: 300)
	TrajectoryCleanupIntervalSec  int     `toml:"trajectory_cleanup_interval_sec"`  // Cleanup goroutine interval (default: 30)
	TrajectoryDescentThresholdFPM float64 `toml:"trajectory_descent_threshold_fpm"` // Altitude trend below this = descending (default: -200)
	TrajectoryClimbThresholdFPM   float64 `toml:"trajectory_climb_threshold_fpm"`   // Altitude trend above this = climbing (default: 200)
	TrajectoryLevelBandFt         float64 `toml:"trajectory_level_band_ft"`         // (AltMax-AltMin) within this = level flight (default: 200)
	TrajectoryTurningRateDeg      float64 `toml:"trajectory_turning_rate_deg"`      // |TrackRate| above this = turning (default: 1.5 deg/sec)

	// Runway-in-use detection
	// Observes aircraft approach/landing/departure patterns to determine which runway
	// ends are currently active. Used to suppress false APP on perpendicular runways.
	RunwayInUseWindowMinutes  int     `toml:"runway_in_use_window_minutes"`  // Rolling evidence window (default: 60)
	RunwayInUseApproachWeight float64 `toml:"runway_in_use_approach_weight"` // Weight for APP events (default: 5.0)
	RunwayInUseLandingWeight  float64 `toml:"runway_in_use_landing_weight"`  // Weight for T/D events (default: 3.0)
	RunwayInUseClimbWeight    float64 `toml:"runway_in_use_climb_weight"`    // Weight for CLB events (default: 1.5)
	RunwayInUseDecayRate      float64 `toml:"runway_in_use_decay_rate"`      // Per-minute time decay (default: 0.98)
}

// Load reads the configuration at path over Defaults: a key the file leaves
// out keeps its default, a key it sets wins.
func Load(path string) (*Config, error) {
	config := Defaults()

	// Check if the file exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, fmt.Errorf("config file not found: %s", path)
	}

	// Read the config file
	config.ConfigPath = path
	meta, err := toml.DecodeFile(path, &config)
	if err != nil {
		return nil, fmt.Errorf("failed to decode config file: %w", err)
	}

	config.reconcileDuplicates(meta)
	renamed, err := config.readFFmpegKeys(path)
	if err != nil {
		return nil, fmt.Errorf("failed to decode config file: %w", err)
	}

	// A key nothing reads -- left over from a removed feature, or misspelled --
	// is said, with its name, and ignored. Never refused: the owner's file still
	// carries [atc_chat] and the OpenAI keys, and a server that will not start
	// over a line that does nothing is worse than one that says so. (Upstream
	// refused unknown keys under [adsb] alone; the same line now covers it.)
	// A whole section nothing reads gets one line with its size, not one per
	// key: [atc_chat] is seventeen keys in the owner's file.
	undecoded := meta.Undecoded()
	gone := map[string]int{}
	for _, key := range undecoded {
		if len(key) == 1 && meta.Type(key...) == "Hash" {
			gone[key[0]] = 0
		}
	}
	for _, key := range undecoded {
		if _, inGone := gone[key[0]]; inGone && len(key) > 1 {
			gone[key[0]]++
		}
	}
	said := map[string]bool{} // a key of [[frequencies.sources]] comes once per source
	for _, key := range undecoded {
		n, inGone := gone[key[0]]
		name := key.String()
		if said[name] || renamed[name] {
			continue
		}
		said[name] = true
		switch {
		case inGone && len(key) == 1:
			config.warnf("[%s] is not a section of this version and is ignored with its %d %s: remove it", key[0], n, plural(n, "key"))
		case inGone:
			// counted above
		case ffmpegRetired[name]:
			config.warnf("%s is ignored: ffmpeg is no longer used, every source is read inside the binary; remove it", name)
		default:
			config.warnf("%s is not a configuration key of this version and is ignored: remove it, or check its spelling", name)
		}
	}

	return &config, nil
}

// ffmpegRetired are the keys of ffmpeg, removed with it once the Go decoder
// was validated (docs-fr/35-plan.md, B1 ter): every source is now read inside
// the binary, an MP3 stream over HTTP or raw PCM over UDP. They do nothing;
// a file that still writes them is told why, not that it misspelled them.
var ffmpegRetired = map[string]bool{
	"transcription.ffmpeg_path":               true,
	"transcription.ffmpeg_channels":           true, // every source is mixed to mono
	"transcription.ffmpeg_format":             true, // always 16-bit PCM
	"frequencies.ffmpeg_timeout_secs":         true,
	"frequencies.ffmpeg_reconnect_delay_secs": true,
	"frequencies.decoder":                     true, // the Go decoder is the only one
	"frequencies.sources.ffmpeg_reconnect":    true, // the server reconnects, after reconnect_interval_secs
}

// readFFmpegKeys reads the two ffmpeg keys that still mean something under
// another name: [transcription] ffmpeg_sample_rate, now sample_rate, and a
// source's ffmpeg_input_options, which described raw PCM over UDP and is now
// pcm_rate and pcm_channels. Each is applied when its new name is not
// written, and the file is told. It returns the keys it handled, which are
// not said again as keys nothing reads.
func (c *Config) readFFmpegKeys(path string) (map[string]bool, error) {
	var old struct {
		Transcription struct {
			FFmpegSampleRate *int `toml:"ffmpeg_sample_rate"`
		} `toml:"transcription"`
		Frequencies struct {
			Sources []struct {
				FFmpegInputOptions []string `toml:"ffmpeg_input_options"`
				PCMRate            *int     `toml:"pcm_rate"`
			} `toml:"sources"`
		} `toml:"frequencies"`
	}
	meta, err := toml.DecodeFile(path, &old)
	if err != nil {
		return nil, err
	}
	handled := map[string]bool{}

	if r := old.Transcription.FFmpegSampleRate; r != nil {
		handled["transcription.ffmpeg_sample_rate"] = true
		if meta.IsDefined("transcription", "sample_rate") {
			c.warnf("transcription.ffmpeg_sample_rate (%d) is ignored: sample_rate is written, and is its new name; remove it", *r)
		} else {
			c.Transcription.SampleRate = *r
			c.warnf("transcription.ffmpeg_sample_rate (%d) is read as transcription.sample_rate: write it so", *r)
		}
	}

	for i, s := range old.Frequencies.Sources {
		if s.FFmpegInputOptions == nil || i >= len(c.Frequencies.Sources) {
			continue
		}
		handled["frequencies.sources.ffmpeg_input_options"] = true
		src := &c.Frequencies.Sources[i]
		rate, channels, ok := pcmFromFFmpegOptions(s.FFmpegInputOptions)
		switch {
		case s.PCMRate != nil:
			c.warnf("frequencies.sources %q: ffmpeg_input_options is ignored: pcm_rate is written; remove it", src.ID)
		case ok:
			src.PCMRate, src.PCMChannels = rate, channels
			c.warnf("frequencies.sources %q: ffmpeg_input_options is read as pcm_rate = %d, pcm_channels = %d: write it so",
				src.ID, rate, channels)
		default:
			c.warnf("frequencies.sources %q: ffmpeg_input_options %q is ignored: ffmpeg is no longer used, and only raw 16-bit PCM over UDP is read without it",
				src.ID, s.FFmpegInputOptions)
		}
	}
	return handled, nil
}

// pcmFromFFmpegOptions reads the input options ffmpeg was given for raw PCM,
// ["-f", "s16le", "-ar", "48000", "-ac", "1"], in any order; without -ac,
// one channel, as ffmpeg took it. Anything else -- a capture device, another
// sample format, no rate -- is not raw PCM this version reads.
func pcmFromFFmpegOptions(opts []string) (rate, channels int, ok bool) {
	channels = 1
	format := ""
	if len(opts)%2 != 0 {
		return 0, 0, false
	}
	for i := 0; i < len(opts); i += 2 {
		v := opts[i+1]
		var err error
		switch opts[i] {
		case "-f":
			format = v
		case "-ar":
			rate, err = strconv.Atoi(v)
		case "-ac":
			channels, err = strconv.Atoi(v)
		default:
			return 0, 0, false
		}
		if err != nil {
			return 0, 0, false
		}
	}
	if format != "s16le" || rate <= 0 || (channels != 1 && channels != 2) {
		return 0, 0, false
	}
	return rate, channels, true
}

// reconcileDuplicates gives each value that two sections used to carry one
// home: airport_range_nm lives in [station], airlines_dat_path in
// [reference]. The older key is still read when written, so that an existing
// file keeps its behaviour, and the file is told once at startup.
func (c *Config) reconcileDuplicates(meta toml.MetaData) {
	oneHome(c, meta, "flight_phases", "station", "airport_range_nm",
		&c.FlightPhases.AirportRangeNM, &c.Station.AirportRangeNM)
	oneHome(c, meta, "post_processing", "reference", "airlines_dat_path",
		&c.PostProcessing.AirlinesDatPath, &c.Reference.AirlinesDATPath)
}

// oneHome settles key between its old section and its new one. Neither
// written, or only the new: the old field takes the new value, nothing to
// say. Only the old: the new field takes it, with a line saying where it now
// belongs. Both: each is kept as written -- the two had separate effects and
// a file that set them differently meant it -- and the file is told.
func oneHome[T comparable](c *Config, meta toml.MetaData, oldSection, newSection, key string, oldField, newField *T) {
	oldWritten := meta.IsDefined(oldSection, key)
	newWritten := meta.IsDefined(newSection, key)
	switch {
	case oldWritten && newWritten && *oldField == *newField:
		c.warnf("%s.%s repeats %s.%s (%v): one key is enough, in [%s]",
			oldSection, key, newSection, key, *newField, newSection)
	case oldWritten && newWritten:
		c.warnf("%s.%s (%v) differs from %s.%s (%v): both are kept as written, but the key belongs in [%s] alone",
			oldSection, key, *oldField, newSection, key, *newField, newSection)
	case oldWritten:
		*newField = *oldField
		c.warnf("%s.%s (%v) is read as %s.%s: write it in [%s]",
			oldSection, key, *oldField, newSection, key, newSection)
	default:
		*oldField = *newField
	}
}

func plural(n int, word string) string {
	if n == 1 {
		return word
	}
	return word + "s"
}

func (c *Config) warnf(format string, args ...any) {
	c.Warnings = append(c.Warnings, fmt.Sprintf(format, args...))
}

// LoadWithFallbackAndPath loads configuration and returns the resolved config file path.
func LoadWithFallbackAndPath(preferredPath string) (*Config, string, error) {
	// List of paths to check in order of preference
	searchPaths := []string{
		preferredPath,         // User-specified path (if provided)
		"configs/config.toml", // Legacy location in configs/ folder
		"config.toml",         // Root directory
	}

	// Remove duplicates while preserving order
	uniquePaths := make([]string, 0, len(searchPaths))
	seen := make(map[string]bool)
	for _, path := range searchPaths {
		if path != "" && !seen[path] {
			uniquePaths = append(uniquePaths, path)
			seen[path] = true
		}
	}

	var lastErr error
	for _, path := range uniquePaths {
		if _, err := os.Stat(path); err == nil {
			// File exists, try to load it
			config, err := Load(path)
			if err != nil {
				lastErr = fmt.Errorf("failed to load config from %s: %w", path, err)
				continue
			}

			resolvedPath := path
			if absPath, absErr := filepath.Abs(path); absErr == nil {
				resolvedPath = absPath
			}

			return config, resolvedPath, nil
		}
		lastErr = fmt.Errorf("config file not found: %s", path)
	}

	return nil, "", fmt.Errorf("config file not found in any of the expected locations: %v. Last error: %w", uniquePaths, lastErr)
}

// Validate validates the configuration
func (c *Config) Validate() error {
	// Validate frequencies config
	if err := c.ValidateFrequencies(); err != nil {
		return err
	}

	if err := c.validateBackends(); err != nil {
		return err
	}
	// The rate every source is brought to (internal/audio): ffmpeg refused a
	// rate it could not make; the resampler divides by it.
	if r := c.Transcription.SampleRate; r < 8000 || r > 48000 {
		return fmt.Errorf("invalid transcription.sample_rate: %d (8000 to 48000 Hz; 16000 is Whisper's)", r)
	}
	if c.Auth.Enabled {
		named := 0
		for _, u := range c.Auth.Users {
			if strings.TrimSpace(u.Name) != "" && u.PasswordHash != "" {
				named++
			}
		}
		if named == 0 {
			return fmt.Errorf("auth.enabled is true but no user has a password_hash; run: co-atc -add-user <name>")
		}
	}
	for _, u := range c.Auth.Users {
		if _, err := auth.NormalizeRole(u.Role); err != nil {
			return fmt.Errorf("auth.users %q: %w", u.Name, err)
		}
	}
	if c.Auth.UsageDays < 0 {
		return fmt.Errorf("invalid auth.usage_days: %d (must be >= 0; 0 keeps no usage)", c.Auth.UsageDays)
	}
	if c.Transcription.AudioArchiveMaxGB < 0 {
		return fmt.Errorf("invalid transcription.audio_archive_max_gb: %v (must be >= 0; 0 keeps everything)", c.Transcription.AudioArchiveMaxGB)
	}
	if c.Transcription.RawLogMaxDays < 0 {
		return fmt.Errorf("invalid transcription.raw_log_max_days: %d (must be >= 0; 0 keeps everything)", c.Transcription.RawLogMaxDays)
	}
	switch c.Server.AudioReplay {
	case "", AudioReplayOwner, AudioReplayAll:
	default:
		return fmt.Errorf("invalid server.audio_replay: %q (must be %q or %q)", c.Server.AudioReplay, AudioReplayOwner, AudioReplayAll)
	}
	for _, o := range c.Server.AllowedOrigins {
		if _, err := auth.NormalizeOrigin(o); err != nil {
			return fmt.Errorf("server.allowed_origins: %w", err)
		}
	}
	if (c.Server.TLSCert == "") != (c.Server.TLSKey == "") {
		return fmt.Errorf("server.tls_cert and server.tls_key must be set together")
	}
	if err := c.validateStationLink(); err != nil {
		return err
	}

	if c.PostProcessing.Enabled {
		if c.PostProcessing.AirlinesDatPath == "" {
			return fmt.Errorf("reference.airlines_dat_path is required when post-processing is enabled: the grammar needs the airline telephony")
		}
		if c.PostProcessing.MinScore < 0 {
			return fmt.Errorf("invalid post_processing.min_score: %v (must be >= 0)", c.PostProcessing.MinScore)
		}
	}

	// Validate server config
	if c.Server.Port <= 0 || c.Server.Port > 65535 {
		return fmt.Errorf("invalid server port: %d", c.Server.Port)
	}
	// Validate AdditionalPorts
	portsSeen := make(map[int]bool)
	portsSeen[c.Server.Port] = true
	for _, p := range c.Server.AdditionalPorts {
		if p <= 0 || p > 65535 {
			return fmt.Errorf("invalid additional server port: %d", p)
		}
		if portsSeen[p] {
			return fmt.Errorf("duplicate port configured: %d (primary or additional)", p)
		}
		portsSeen[p] = true
	}

	// The page is built in; a directory named in its place must be there, or
	// every file of the page would be a 404.
	if dir := c.Server.StaticDir; dir != "" {
		if info, err := os.Stat(dir); err != nil || !info.IsDir() {
			return fmt.Errorf("server.static_dir %q is not a directory: name the page's directory, or leave it empty to serve the page built into the binary", dir)
		}
	}

	// Validate ADSB config
	if c.ADSB.SourceType == "" {
		return fmt.Errorf("adsb.source_type is required (must be one of: external-rapidapi, external-opensky, tar1090, readsb-api, readsb-file)")
	}

	switch c.ADSB.SourceType {
	case "external-rapidapi":
		if c.ADSB.ExternalSourceURL == "" {
			return fmt.Errorf("external_source_url is required when source_type is external-rapidapi")
		}
		if c.ADSB.APIHost == "" {
			return fmt.Errorf("api_host is required when source_type is external-rapidapi")
		}
		if c.ADSB.APIKey == "" {
			return fmt.Errorf("api_key is required when source_type is external-rapidapi")
		}
		if c.ADSB.SearchRadiusNM <= 0 {
			return fmt.Errorf("search_radius_nm must be positive when source_type is external-rapidapi")
		}
	case "external-opensky":
		if c.ADSB.OpenSkyBaseURL == "" {
			return fmt.Errorf("opensky_base_url is required when source_type is external-opensky")
		}
		if c.ADSB.SearchRadiusNM <= 0 {
			return fmt.Errorf("search_radius_nm must be positive when source_type is external-opensky")
		}
		if c.ADSB.OpenSkyAuthMode == "" {
			c.ADSB.OpenSkyAuthMode = Defaults().ADSB.OpenSkyAuthMode
		}
		switch c.ADSB.OpenSkyAuthMode {
		case "anonymous":
			// no additional required fields
		case "oauth2":
			if c.ADSB.OpenSkyTokenURL == "" {
				return fmt.Errorf("opensky_token_url is required when source_type is external-opensky and opensky_auth_mode is oauth2")
			}
			if c.ADSB.OpenSkyOAuth2CredentialsPath == "" {
				return fmt.Errorf("opensky_oauth2_credentials_path is required when source_type is external-opensky and opensky_auth_mode is oauth2")
			}
		default:
			return fmt.Errorf("invalid opensky_auth_mode: %s (must be one of: anonymous, oauth2)", c.ADSB.OpenSkyAuthMode)
		}
	case "tar1090":
		if c.ADSB.Tar1090BaseURL == "" {
			return fmt.Errorf("tar1090_base_url is required when source_type is tar1090")
		}
	case "readsb-api":
		if c.ADSB.ReadsbAPIURL == "" {
			return fmt.Errorf("readsb_api_url is required when source_type is readsb-api")
		}
	case "readsb-file":
		// No mandatory fields. Auto-detection uses standard filesystem paths when readsb_data_dir is empty.
	default:
		return fmt.Errorf("invalid ADSB source type: %s (must be one of: external-rapidapi, external-opensky, tar1090, readsb-api, readsb-file)", c.ADSB.SourceType)
	}

	if c.ADSB.FetchIntervalSecs <= 0 {
		return fmt.Errorf("invalid fetch interval: %d", c.ADSB.FetchIntervalSecs)
	}
	if c.Storage.DBRetentionGB <= 0 {
		c.Storage.DBRetentionGB = Defaults().Storage.DBRetentionGB
	}

	// Validate logging config
	switch c.Logging.Level {
	case "debug", "info", "warn", "error":
		// Valid log level
	default:
		return fmt.Errorf("invalid log level: %s", c.Logging.Level)
	}

	switch c.Logging.Format {
	case "json", "console":
		// Valid log format
	default:
		return fmt.Errorf("invalid log format: %s", c.Logging.Format)
	}

	// Validate storage config
	if c.Storage.Type != "sqlite" {
		return fmt.Errorf("invalid storage type: %s (only 'sqlite' is supported)", c.Storage.Type)
	}

	if c.Storage.Type == "sqlite" && c.Storage.SQLiteBasePath == "" {
		return fmt.Errorf("sqlite_base_path is required when storage type is sqlite")
	}

	// Validate Station config
	if err := c.ValidateStation(); err != nil {
		return err
	}

	// Validate Flight Phases config
	if err := c.ValidateFlightPhases(); err != nil {
		return err
	}

	// Validate Weather config
	if err := c.ValidateWeather(); err != nil {
		return err
	}

	return nil
}

// validateStationLink checks [station_link] when it is on. The link talks to
// this server in plain HTTP on the loopback, with a key that only the
// loopback may use: a server bound to one network address alone, or serving
// TLS itself, cannot be reached that way.
func (c *Config) validateStationLink() error {
	l := c.StationLink
	if !l.Enabled {
		return nil
	}
	if strings.TrimSpace(l.StationURL) == "" || strings.TrimSpace(l.StreamsURL) == "" {
		return fmt.Errorf("station_link.station_url and station_link.streams_url are required when station_link.enabled is true")
	}
	if l.IntervalSeconds < 1 {
		return fmt.Errorf("invalid station_link.interval_seconds: %d (must be >= 1)", l.IntervalSeconds)
	}
	switch c.Server.Host {
	case "", "0.0.0.0", "::", "127.0.0.1", "::1", "localhost":
	default:
		return fmt.Errorf("station_link needs the server on the loopback, and server.host %q is not: "+
			"use 127.0.0.1, or 0.0.0.0 to be reached from the network as well", c.Server.Host)
	}
	if c.Server.TLSCert != "" {
		return fmt.Errorf("station_link talks to this server in plain HTTP on the loopback, which server.tls_cert prevents: " +
			"leave TLS to a reverse proxy, or run radio-ctl-sync yourself")
	}
	return nil
}

// ValidateStation validates the station configuration
func (c *Config) ValidateStation() error {
	// Validate Latitude
	if c.Station.Latitude < -90 || c.Station.Latitude > 90 {
		return fmt.Errorf("invalid station latitude: %f", c.Station.Latitude)
	}

	// Validate Longitude
	if c.Station.Longitude < -180 || c.Station.Longitude > 180 {
		return fmt.Errorf("invalid station longitude: %f", c.Station.Longitude)
	}

	// Elevation can be negative, so we'll just check if it's within a reasonable range, e.g. -2000 to 30000 feet.
	if c.Station.ElevationFeet < -2000 || c.Station.ElevationFeet > 30000 {
		return fmt.Errorf("station elevation out of typical range: %d ft", c.Station.ElevationFeet)
	}

	// Airport code validation is now handled in ValidateWeather method

	// Default display range
	if c.Station.DisplayRangeNM <= 0 {
		c.Station.DisplayRangeNM = Defaults().Station.DisplayRangeNM
	}

	return nil
}

// ValidateFrequencies validates the frequencies configuration
func (c *Config) ValidateFrequencies() error {
	// Skip validation if no frequencies are configured
	if len(c.Frequencies.Sources) == 0 {
		return nil
	}

	// Validate buffer size
	if c.Frequencies.BufferSizeKB <= 0 {
		return fmt.Errorf("invalid buffer size: %d KB", c.Frequencies.BufferSizeKB)
	}

	// Validate reconnect interval
	if c.Frequencies.ReconnectIntervalSecs <= 0 {
		return fmt.Errorf("invalid reconnect interval: %d", c.Frequencies.ReconnectIntervalSecs)
	}

	// Validate frequency sources
	idMap := make(map[string]bool)
	orderMap := make(map[int]string) // Track orders to check for duplicates
	for i, freq := range c.Frequencies.Sources {
		// Validate ID
		if freq.ID == "" {
			return fmt.Errorf("frequency #%d: ID is required", i+1)
		}
		if idMap[freq.ID] {
			return fmt.Errorf("frequency #%d: duplicate ID: %s", i+1, freq.ID)
		}
		idMap[freq.ID] = true

		// Validate airport
		if freq.Airport == "" {
			return fmt.Errorf("frequency #%d: airport is required", i+1)
		}

		// Validate name
		if freq.Name == "" {
			return fmt.Errorf("frequency #%d: name is required", i+1)
		}

		// Validate frequency
		// Zero means there is no single frequency to state, which is the honest
		// answer for a mixed stream: RTLSDR-Airband's mixer puts several channels
		// on one mount, and any number chosen for it would be a fiction. The UI
		// shows the name alone in that case. Negative is still a mistake.
		if freq.FrequencyMHz < 0 {
			return fmt.Errorf("frequency #%d: invalid frequency: %f", i+1, freq.FrequencyMHz)
		}

		// Validate URL
		if freq.URL == "" {
			return fmt.Errorf("frequency #%d: URL is required", i+1)
		}

		// Validate order
		if freq.Order <= 0 {
			return fmt.Errorf("frequency #%d: order must be a positive integer", i+1)
		}
		if existingID, exists := orderMap[freq.Order]; exists {
			return fmt.Errorf("frequency #%d: duplicate order value %d (already used by %s)", i+1, freq.Order, existingID)
		}
		orderMap[freq.Order] = freq.ID
	}

	return nil
}

// ValidateFlightPhases validates the flight phases configuration
func (c *Config) ValidateFlightPhases() error {
	if !c.FlightPhases.Enabled {
		return nil // Skip validation if flight phases are disabled
	}

	// A value written as 0 means the default, as it always has here. (A key
	// left out already holds it: Load starts from Defaults.)
	d := Defaults().FlightPhases
	if c.FlightPhases.FlyingMinTASKts == 0 {
		c.FlightPhases.FlyingMinTASKts = d.FlyingMinTASKts
	}
	if c.FlightPhases.FlyingMinAltFt == 0 {
		c.FlightPhases.FlyingMinAltFt = d.FlyingMinAltFt
	}
	if c.FlightPhases.HelicopterAltMultiplier == 0 {
		c.FlightPhases.HelicopterAltMultiplier = d.HelicopterAltMultiplier
	}
	if c.FlightPhases.HighSpeedThresholdKts == 0 {
		c.FlightPhases.HighSpeedThresholdKts = d.HighSpeedThresholdKts
	}
	if c.FlightPhases.PhasePreservationSeconds == 0 {
		c.FlightPhases.PhasePreservationSeconds = d.PhasePreservationSeconds
	}
	if c.FlightPhases.PhaseTransitionTimeoutSeconds == 0 {
		c.FlightPhases.PhaseTransitionTimeoutSeconds = d.PhaseTransitionTimeoutSeconds
	}
	if c.FlightPhases.HighAltitudeOverrideFt == 0 {
		c.FlightPhases.HighAltitudeOverrideFt = d.HighAltitudeOverrideFt
	}
	if c.FlightPhases.ImpossibleAltDropThresholdFt == 0 {
		c.FlightPhases.ImpossibleAltDropThresholdFt = d.ImpossibleAltDropThresholdFt
	}
	if c.FlightPhases.ImpossibleSpeedDropThresholdKts == 0 {
		c.FlightPhases.ImpossibleSpeedDropThresholdKts = d.ImpossibleSpeedDropThresholdKts
	}
	if c.FlightPhases.ImpossibleSpeedDropMinAltFt == 0 {
		c.FlightPhases.ImpossibleSpeedDropMinAltFt = d.ImpossibleSpeedDropMinAltFt
	}
	if c.FlightPhases.SignalLostLandingMaxAltFt == 0 {
		c.FlightPhases.SignalLostLandingMaxAltFt = d.SignalLostLandingMaxAltFt
	}
	if c.FlightPhases.ApproachMaxAltitudeFt == 0 {
		c.FlightPhases.ApproachMaxAltitudeFt = d.ApproachMaxAltitudeFt
	}
	if c.FlightPhases.TrajectoryBufferDurationSec == 0 {
		c.FlightPhases.TrajectoryBufferDurationSec = d.TrajectoryBufferDurationSec
	}
	if c.FlightPhases.TrajectoryMinPoints == 0 {
		c.FlightPhases.TrajectoryMinPoints = d.TrajectoryMinPoints
	}
	if c.FlightPhases.TrajectoryStaleTimeoutSec == 0 {
		c.FlightPhases.TrajectoryStaleTimeoutSec = d.TrajectoryStaleTimeoutSec
	}
	if c.FlightPhases.TrajectoryCleanupIntervalSec == 0 {
		c.FlightPhases.TrajectoryCleanupIntervalSec = d.TrajectoryCleanupIntervalSec
	}
	if c.FlightPhases.TrajectoryDescentThresholdFPM == 0 {
		c.FlightPhases.TrajectoryDescentThresholdFPM = d.TrajectoryDescentThresholdFPM
	}
	if c.FlightPhases.TrajectoryClimbThresholdFPM == 0 {
		c.FlightPhases.TrajectoryClimbThresholdFPM = d.TrajectoryClimbThresholdFPM
	}
	if c.FlightPhases.TrajectoryLevelBandFt == 0 {
		c.FlightPhases.TrajectoryLevelBandFt = d.TrajectoryLevelBandFt
	}
	if c.FlightPhases.TrajectoryTurningRateDeg == 0 {
		c.FlightPhases.TrajectoryTurningRateDeg = d.TrajectoryTurningRateDeg
	}
	if c.FlightPhases.RunwayInUseWindowMinutes == 0 {
		c.FlightPhases.RunwayInUseWindowMinutes = d.RunwayInUseWindowMinutes
	}
	if c.FlightPhases.RunwayInUseApproachWeight == 0 {
		c.FlightPhases.RunwayInUseApproachWeight = d.RunwayInUseApproachWeight
	}
	if c.FlightPhases.RunwayInUseLandingWeight == 0 {
		c.FlightPhases.RunwayInUseLandingWeight = d.RunwayInUseLandingWeight
	}
	if c.FlightPhases.RunwayInUseClimbWeight == 0 {
		c.FlightPhases.RunwayInUseClimbWeight = d.RunwayInUseClimbWeight
	}
	if c.FlightPhases.RunwayInUseDecayRate == 0 {
		c.FlightPhases.RunwayInUseDecayRate = d.RunwayInUseDecayRate
	}

	// Validate altitude thresholds
	if c.FlightPhases.CruiseAltitudeFt <= 0 {
		return fmt.Errorf("cruise_altitude_ft must be positive: %d", c.FlightPhases.CruiseAltitudeFt)
	}
	if c.FlightPhases.DepartureAltitudeFt <= 0 {
		return fmt.Errorf("departure_altitude_ft must be positive: %d", c.FlightPhases.DepartureAltitudeFt)
	}

	// Validate speed thresholds
	if c.FlightPhases.TaxiingMinSpeedKts < 0 {
		return fmt.Errorf("taxiing_min_speed_kts must be non-negative: %d", c.FlightPhases.TaxiingMinSpeedKts)
	}
	if c.FlightPhases.TaxiingMaxSpeedKts <= c.FlightPhases.TaxiingMinSpeedKts {
		return fmt.Errorf("taxiing_max_speed_kts (%d) must be greater than taxiing_min_speed_kts (%d)",
			c.FlightPhases.TaxiingMaxSpeedKts, c.FlightPhases.TaxiingMinSpeedKts)
	}

	// Validate approach detection parameters
	if c.FlightPhases.ApproachCenterlineToleranceNM <= 0 {
		return fmt.Errorf("approach_centerline_tolerance_nm must be positive: %f", c.FlightPhases.ApproachCenterlineToleranceNM)
	}
	if c.FlightPhases.ApproachMaxDistanceNM <= 0 {
		return fmt.Errorf("approach_max_distance_nm must be positive: %d", c.FlightPhases.ApproachMaxDistanceNM)
	}
	if c.FlightPhases.ApproachHeadingToleranceDeg <= 0 || c.FlightPhases.ApproachHeadingToleranceDeg > 180 {
		return fmt.Errorf("approach_heading_tolerance_deg must be between 1 and 180: %f", c.FlightPhases.ApproachHeadingToleranceDeg)
	}

	// Validate new ground detection thresholds
	if c.FlightPhases.FlyingMinTASKts <= 0 {
		return fmt.Errorf("flying_min_tas_kts must be positive: %f", c.FlightPhases.FlyingMinTASKts)
	}
	if c.FlightPhases.FlyingMinAltFt <= 0 {
		return fmt.Errorf("flying_min_alt_ft must be positive: %f", c.FlightPhases.FlyingMinAltFt)
	}
	if c.FlightPhases.HelicopterAltMultiplier <= 0 {
		return fmt.Errorf("helicopter_alt_multiplier must be positive: %f", c.FlightPhases.HelicopterAltMultiplier)
	}
	if c.FlightPhases.HighSpeedThresholdKts <= 0 {
		return fmt.Errorf("high_speed_threshold_kts must be positive: %f", c.FlightPhases.HighSpeedThresholdKts)
	}

	// Validate phase preservation times
	if c.FlightPhases.PhasePreservationSeconds <= 0 {
		return fmt.Errorf("phase_preservation_seconds must be positive: %d", c.FlightPhases.PhasePreservationSeconds)
	}
	if c.FlightPhases.PhaseTransitionTimeoutSeconds <= 0 {
		return fmt.Errorf("phase_transition_timeout_seconds must be positive: %d", c.FlightPhases.PhaseTransitionTimeoutSeconds)
	}
	// Validate signal lost landing detection
	if c.FlightPhases.SignalLostLandingEnabled && c.FlightPhases.SignalLostLandingMaxAltFt <= 0 {
		return fmt.Errorf("signal_lost_landing_max_alt_ft must be positive when signal_lost_landing_enabled is true: %f", c.FlightPhases.SignalLostLandingMaxAltFt)
	}

	return nil
}

// validateBackends accepts "local" or nothing for both stages, and refuses
// "openai" with the reason: the backend is gone, and a server started on it
// would transcribe nothing without saying why.
func (c *Config) validateBackends() error {
	for _, b := range []struct {
		key string
		val *string
	}{
		{"transcription.backend", &c.Transcription.Backend},
		{"post_processing.backend", &c.PostProcessing.Backend},
	} {
		switch *b.val {
		case "", BackendLocal:
			*b.val = BackendLocal
		case removedBackend:
			return fmt.Errorf("%s = %q: the OpenAI backend was removed, use %q (see sidecar/README.md)",
				b.key, removedBackend, BackendLocal)
		default:
			return fmt.Errorf("invalid %s: %q (want %q)", b.key, *b.val, BackendLocal)
		}
	}
	return nil
}

// ValidateWeather validates the weather configuration
func (c *Config) ValidateWeather() error {
	// Validate refresh interval
	if c.Weather.RefreshIntervalMinutes <= 0 {
		return fmt.Errorf("weather refresh_interval_minutes must be greater than 0: %d", c.Weather.RefreshIntervalMinutes)
	}

	// Validate request timeout
	if c.Weather.RequestTimeoutSeconds <= 0 {
		return fmt.Errorf("weather request_timeout_seconds must be greater than 0: %d", c.Weather.RequestTimeoutSeconds)
	}

	// Validate max retries
	if c.Weather.MaxRetries < 0 {
		return fmt.Errorf("weather max_retries must be 0 or greater: %d", c.Weather.MaxRetries)
	}

	// Validate cache expiry
	if c.Weather.CacheExpiryMinutes <= 0 {
		return fmt.Errorf("weather cache_expiry_minutes must be greater than 0: %d", c.Weather.CacheExpiryMinutes)
	}

	// Validate API base URL
	if c.Weather.APIBaseURL == "" {
		return fmt.Errorf("weather api_base_url cannot be empty")
	}

	// At least one weather type must be enabled
	if !c.Weather.FetchMETAR && !c.Weather.FetchTAF && !c.Weather.FetchNOTAMs {
		return fmt.Errorf("at least one weather type must be enabled (fetch_metar, fetch_taf, or fetch_notams)")
	}

	// Validate that airport code is set if weather fetching is enabled
	if (c.Weather.FetchMETAR || c.Weather.FetchTAF || c.Weather.FetchNOTAMs) && c.Station.AirportCode == "" {
		return fmt.Errorf("station airport_code is required when weather fetching is enabled")
	}

	return nil
}

// WeatherConfig contains weather data fetching and caching configuration
type WeatherConfig struct {
	RefreshIntervalMinutes int    `toml:"refresh_interval_minutes"` // Weather data refresh interval in minutes
	APIBaseURL             string `toml:"api_base_url"`             // Base URL for weather API (e.g., https://node.windy.com/airports)
	RequestTimeoutSeconds  int    `toml:"request_timeout_seconds"`  // HTTP request timeout in seconds
	MaxRetries             int    `toml:"max_retries"`              // Maximum number of retry attempts for failed requests
	FetchMETAR             bool   `toml:"fetch_metar"`              // Whether to fetch METAR data
	FetchTAF               bool   `toml:"fetch_taf"`                // Whether to fetch TAF data
	FetchNOTAMs            bool   `toml:"fetch_notams"`             // Whether to fetch NOTAM data
	CacheExpiryMinutes     int    `toml:"cache_expiry_minutes"`     // How long to keep cached data if refresh fails
}
