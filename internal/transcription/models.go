package transcription

import (
	"github.com/yegors/co-atc/internal/transcription/phraseology"
	"github.com/yegors/co-atc/pkg/logger"
)

// Import the logger package's exported functions
var (
	String = logger.String
	Int    = logger.Int
	Int64  = logger.Int64
	Error  = logger.Error
)

// LocalSTTConfig configures the local speech-to-text sidecar backend.
//
// The sidecar receives whole transmissions rather than a continuous stream, so
// this side has to decide where a transmission begins and ends. On an SDR feed
// that is straightforward: RTLSDR-Airband writes digital silence between
// transmissions, so squelch activity is unambiguous.
type LocalSTTConfig struct {
	ServerURL      string `toml:"server_url"`      // Base URL of the sidecar, e.g. http://127.0.0.1:8178
	TimeoutSeconds int    `toml:"timeout_seconds"` // HTTP timeout for one transmission

	SegmentSilenceMs  int     `toml:"segment_silence_ms"`  // Silence that ends a transmission (default 600)
	SegmentMinMs      int     `toml:"segment_min_ms"`      // Shorter than this is discarded (default 400)
	SegmentMaxSeconds int     `toml:"segment_max_seconds"` // Hard bound on one segment (default 30)
	SegmentPrerollMs  int     `toml:"segment_preroll_ms"`  // Audio kept before speech starts (default 200)
	SilenceThreshold  float64 `toml:"silence_threshold"`   // RMS below which a frame is silence (default 0.005)
}

// Config represents the configuration for the transcription service.
// Transcription is local: the sidecar in sidecar/, see docs/LOCAL-STT.md.
type Config struct {
	Local              LocalSTTConfig
	FrequencyLanguages map[string]string // frequency id -> expected language, from the catalogue
	Language           string            // expected language when the catalogue gives none
	SampleRate         int               // rate of the PCM the frequencies service decodes
	LogDir             string            // Optional directory for transcription log files
}

// PostProcessingConfig configures the second stage of the pipeline, the
// phraseology grammar that fills in speaker, callsign and clearances of a
// stored transcription against live ADS-B. See docs-fr/17-appariement.md.
type PostProcessingConfig struct {
	Enabled         bool
	IntervalSeconds int
	BatchSize       int

	AirlinesDatPath string
	MinScore        float64
	MinDigits       int

	// Rules, when set, gives the association rules in force for each
	// transmission, so that a change from the settings panel applies to the
	// next one. Nil keeps the matcher as MinDigits configured it.
	Rules func() phraseology.Rules

	// SectorOf, when set, gives the part of the sky a frequency can be talking
	// to, read for every transmission; false means the whole sky.
	SectorOf func(frequencyID string) (phraseology.Sector, bool)
}
