package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/yegors/co-atc/pkg/logger"
)

// RuntimeSettings are the few operational values that can be changed while the
// server runs, from the settings panel.
//
// They live in a file of their own rather than being written back into the TOML.
// The TOML is the documentation: it carries the measured tables and the reasons
// behind every default, and a program that rewrites it would destroy exactly what
// makes it worth reading. This file records only what was changed from the panel,
// and the TOML stays the source of the defaults.
type RuntimeSettings struct {
	DBRetentionGB float64 `json:"db_retention_gb"`
	LogLevel      string  `json:"log_level"`

	// ReferenceAirport is the airport that approach, departure, takeoff and
	// landing are judged against, and whose weather is fetched. It starts as
	// [station] airport_code and can be changed from the panel -- the airport a
	// receiver sits on is not always the one worth watching.
	ReferenceAirport string `json:"reference_airport"`

	// AlsoAirports are followed besides the reference airport: each aircraft is
	// judged against the one whose runway axis it flies, else the nearest. Orly
	// and De Gaulle are 13 NM apart and a receiver near Paris sees both
	// (docs-fr/05-decisions.md, Q40). The weather stays the reference airport's.
	AlsoAirports []string `json:"also_airports,omitempty"`

	// Matching is how a transmission is tied to an aircraft. Absent from a file
	// saved before it existed, which then means the defaults.
	Matching *MatchingRules `json:"matching,omitempty"`

	// Vitals is the station's console on the sign-in page: aggregated figures
	// served without a session by /api/v1/vitals. Absent from a file saved
	// before it existed, which then means on.
	Vitals *VitalsSettings `json:"vitals,omitempty"`

	// AIPNames is how the list of official names (internal/aip, Q55) is kept.
	// Absent from a file saved before it existed, which then means off.
	AIPNames *AIPNamesSettings `json:"aip_names,omitempty"`

	// DBArchive is where the retention copies a day before deleting it
	// (28/10, D85), set by the owner from the panel. Absent means [storage]
	// archive_dir; an empty Dir, no archive.
	DBArchive *DBArchiveSettings `json:"db_archive,omitempty"`
}

// DBArchiveSettings name the database archive. Dir is absolute, an existing
// directory the server can write, outside the databases' own; the server
// never makes it.
type DBArchiveSettings struct {
	Dir string `json:"dir"`
}

// AIPNamesSettings say whether the server makes the list again by itself when
// a new AIRAC cycle comes into force. Off by default: it is a request to an
// outside site, and the settings page has a button (design, 15/10).
type AIPNamesSettings struct {
	AutoUpdate bool `json:"auto_update"`

	// NameProcedures shows, beside the words of a transmission, the official
	// name of the procedure they probably were ("ELCOB 7G" for "elcope seven
	// golf"). Absent means on: it was measured before it was offered.
	NameProcedures *bool `json:"name_procedures,omitempty"`
	// Naming is how alike the words must sound: "surer", "measured" (the
	// default) or "more" (PlaceFloors).
	Naming string `json:"naming,omitempty"`
}

// PlaceFloors are the three sound likenesses offered for naming procedures,
// each measured on the owner's listening of 77 cases of Paris approaches
// (16/10): 1.10 named 9, all right; 0.95 named 18, all right; 0.90 named 19,
// 18 right. A value between two measures would promise what nobody measured.
var PlaceFloors = map[string]float64{"surer": 1.10, "measured": 0.95, "more": 0.90}

// PlaceNaming says whether procedures heard are named, and how alike they
// must sound.
func (r *Runtime) PlaceNaming() (float64, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	a := r.settings.AIPNames
	if a != nil && a.NameProcedures != nil && !*a.NameProcedures {
		return 0, false
	}
	level := "measured"
	if a != nil && a.Naming != "" {
		level = a.Naming
	}
	floor, ok := PlaceFloors[level]
	if !ok {
		floor = PlaceFloors["measured"]
	}
	return floor, true
}

// VitalsSettings switch the public figures of the sign-in page. Off, the route
// answers 404 as if it did not exist.
type VitalsSettings struct {
	Enabled bool `json:"enabled"`
}

// DefaultVitals is the console as it starts: on.
func DefaultVitals() VitalsSettings { return VitalsSettings{Enabled: true} }

// MatchingRules are the association rules that can be changed from the panel.
// Each was measured against a shuffled sky before being offered
// (docs-fr/05-decisions.md, Q30 and Q46).
type MatchingRules struct {
	// Letters reads a spoken number followed by spelled letters as one flight
	// part, "seven uniform echo" as 7UE: 59% of the callsigns over Paris have
	// letters. Measured on 24/09: +27% true matches, precision 73% -> 76%.
	Letters bool `json:"letters"`
	// ApproxOperators accepts an airline name heard roughly ("welling" for
	// Vueling), among the operators in the sky only.
	ApproxOperators bool `json:"approx_operators"`
	// MinDigits is the shortest spoken number that may stand for a flight
	// number: 2 matched by chance about as often as by truth, 3 is the default.
	MinDigits int `json:"min_digits"`
	// OneDigitOff accepts a number one digit away from an aircraft's. Measured
	// 81% noise; offered to try, off by default.
	OneDigitOff bool `json:"one_digit_off"`

	// An aircraft heard on the frequency in the last two minutes may be named
	// by an abbreviation: its last letters ("sierra bravo"), its last two
	// digits when no other aircraft just heard ends so, or its airline alone
	// when it is the only one of that airline just heard. Measured on 24/09
	// against another frequency's recent aircraft: +8 and +13 true matches,
	// almost no chance; the airline alone +68, but ADS-B cannot tell whether
	// it named the right aircraft, so it stays off until checked by ear.
	ContextLetters bool `json:"context_letters"`
	ContextDigits  bool `json:"context_digits"`
	ContextNames   bool `json:"context_names"`

	// PrefixDigits accepts the first two digits of a flight number when the
	// transmission names the airline ("Air France Five One" for AFR511). Off
	// until measured (docs-fr/30-association-regles-et-reglages.md).
	PrefixDigits bool `json:"prefix_digits"`

	// Sectors compares a transmission only with the aircraft its frequency can
	// be talking to, for the frequencies that name an airport and a kind. The
	// radii and ceilings were agreed with the owner on 26/09. Measured on 24/09
	// on De Gaulle approach, then on 26/09 on Orly and three De Gaulle
	// frequencies: chance divided by 2.4, as many right aircraft, precision
	// 91% -> 96% (docs-fr/05-decisions.md, D59).
	Sectors bool                  `json:"sectors"`
	Sector  map[string]SectorSize `json:"sector"`

	// MinScore refuses a match below this score; 0 keeps the matcher's own
	// floor (0.6). 0.9 requires the exact flight number: measured on 03 and
	// 04/10, precision 95% -> 97% for 27% fewer matches (D73, the "cautious"
	// preset). The file's post_processing.min_score still applies if higher.
	MinScore float64 `json:"min_score"`
}

// SectorSize is how far and how high a kind of frequency reaches.
type SectorSize struct {
	RadiusNM float64 `json:"radius_nm"`
	MaxAltFt float64 `json:"max_alt_ft"`
}

// DefaultSectorSizes are the radii and ceilings agreed on 26/09.
func DefaultSectorSizes() map[string]SectorSize {
	return map[string]SectorSize{
		"approach":  {RadiusNM: 60, MaxAltFt: 20000},
		"departure": {RadiusNM: 60, MaxAltFt: 20000},
		"tower":     {RadiusNM: 15, MaxAltFt: 6000},
		"ground":    {RadiusNM: 5, MaxAltFt: 1500},
	}
}

// DefaultMatchingRules are the rules in force until changed from the panel:
// letters, approximate names, and an aircraft just heard named by its last
// letters or digits, decided by the owner on 26/09 after Q46; and each
// frequency's sector, once validated on 26/09 (D59).
func DefaultMatchingRules(minDigits int) MatchingRules {
	if minDigits <= 0 {
		minDigits = 3
	}
	return MatchingRules{Letters: true, ApproxOperators: true, MinDigits: minDigits,
		ContextLetters: true, ContextDigits: true, Sectors: true, Sector: DefaultSectorSizes()}
}

func (m MatchingRules) validate() error {
	if m.MinDigits < 2 || m.MinDigits > 4 {
		return fmt.Errorf("matching.min_digits must be 2, 3 or 4, got %d", m.MinDigits)
	}
	if m.MinScore < 0 || m.MinScore > 2 {
		return fmt.Errorf("matching.min_score must be between 0 and 2, got %g", m.MinScore)
	}
	for kind, size := range m.Sector {
		if !SectorKinds[kind] {
			return fmt.Errorf("matching.sector: unknown kind %q", kind)
		}
		if size.RadiusNM < 1 || size.RadiusNM > 250 {
			return fmt.Errorf("matching.sector.%s.radius_nm must be between 1 and 250, got %g", kind, size.RadiusNM)
		}
		if size.MaxAltFt < 500 || size.MaxAltFt > 60000 {
			return fmt.Errorf("matching.sector.%s.max_alt_ft must be between 500 and 60000, got %g", kind, size.MaxAltFt)
		}
	}
	return nil
}

// withSectorDefaults fills the kinds a saved file does not mention, so that a
// kind added later has a size without anyone having to set it.
func (m *MatchingRules) withSectorDefaults() {
	if m.Sector == nil {
		m.Sector = map[string]SectorSize{}
	}
	for kind, size := range DefaultSectorSizes() {
		if _, ok := m.Sector[kind]; !ok {
			m.Sector[kind] = size
		}
	}
}

// MaxAlsoAirports bounds the airports followed besides the reference one. Two
// or three is what one receiver can watch; each costs a runway tracker, and the
// measurement of 23/09 found little beyond Orly and De Gaulle.
const MaxAlsoAirports = 3

// ReferenceAirports returns the airports followed, the reference one first.
func (s RuntimeSettings) ReferenceAirports() []string {
	if s.ReferenceAirport == "" {
		return nil
	}
	return append([]string{s.ReferenceAirport}, s.AlsoAirports...)
}

// normalizeAlso uppercases the further airports and drops blanks, repeats and
// the reference airport itself.
func normalizeAlso(principal string, codes []string) []string {
	var out []string
	seen := map[string]bool{principal: true}
	for _, c := range codes {
		c = normalizeAirport(c)
		if c == "" || seen[c] {
			continue
		}
		seen[c] = true
		out = append(out, c)
	}
	return out
}

func sameAirports(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// ReferenceAirportHook validates and applies a change of reference airports,
// the reference one first. The config package cannot do either itself -- it
// takes the reference data, the ADS-B service and the weather service -- so the
// server wires them in.
type ReferenceAirportHook struct {
	Validate func(codes []string) error // no side effect: refuse before anything moves
	Apply    func(codes []string)       // cannot fail once Validate has passed
}

// Runtime holds the live values and persists changes.
type Runtime struct {
	mu          sync.RWMutex
	settings    RuntimeSettings
	path        string
	dataDir     string // the daily databases', which an archive must stay out of
	log         *logger.Logger
	airportHook *ReferenceAirportHook
}

// RuntimeSettingsFile is where changes made from the panel are kept, beside the
// configuration they override.
const RuntimeSettingsFile = "runtime-settings.json"

// NewRuntime starts from the configured defaults, then applies whatever a previous
// session saved. A missing or unreadable file is not an error: it means nothing has
// been changed from the panel yet, which is the normal state.
func NewRuntime(cfg *Config, configPath string, log *logger.Logger) *Runtime {
	r := &Runtime{
		settings: RuntimeSettings{
			DBRetentionGB:    cfg.Storage.DBRetentionGB,
			LogLevel:         cfg.Logging.Level,
			ReferenceAirport: normalizeAirport(cfg.Station.AirportCode),
		},
		path:    filepath.Join(filepath.Dir(configPath), RuntimeSettingsFile),
		dataDir: cfg.Storage.SQLiteBasePath,
		log:     log.Named("runtime-settings"),
	}
	r.settings.DBArchive = &DBArchiveSettings{Dir: cfg.Storage.ArchiveDir}
	defaults := DefaultMatchingRules(cfg.PostProcessing.MinDigits)
	r.settings.Matching = &defaults
	vitals := DefaultVitals()
	r.settings.Vitals = &vitals

	b, err := os.ReadFile(r.path)
	if err != nil {
		return r
	}
	var saved RuntimeSettings
	if err := json.Unmarshal(b, &saved); err != nil {
		r.log.Warn("Ignoring unreadable runtime settings", logger.String("path", r.path), logger.Error(err))
		return r
	}
	// A file saved before 28/09 holds db_retention_days instead, which no
	// longer exists: it is ignored and the configured size applies.
	if saved.DBRetentionGB > 0 {
		r.settings.DBRetentionGB = saved.DBRetentionGB
	}
	if saved.LogLevel != "" {
		r.settings.LogLevel = saved.LogLevel
	}
	if a := normalizeAirport(saved.ReferenceAirport); a != "" {
		r.settings.ReferenceAirport = a
		also := normalizeAlso(a, saved.AlsoAirports)
		if len(also) > MaxAlsoAirports {
			also = also[:MaxAlsoAirports]
		}
		r.settings.AlsoAirports = also
	}
	if saved.Matching != nil {
		saved.Matching.withSectorDefaults()
		if err := saved.Matching.validate(); err != nil {
			r.log.Warn("Ignoring saved matching rules", logger.Error(err))
		} else {
			m := *saved.Matching
			r.settings.Matching = &m
		}
	}
	if saved.Vitals != nil {
		v := *saved.Vitals
		r.settings.Vitals = &v
	}
	if saved.DBArchive != nil {
		d := *saved.DBArchive
		r.settings.DBArchive = &d
	}
	if saved.AIPNames != nil {
		a := *saved.AIPNames
		if _, ok := PlaceFloors[a.Naming]; a.Naming != "" && !ok {
			r.log.Warn("Ignoring an unknown naming level", logger.String("naming", a.Naming))
			a.Naming = ""
		}
		r.settings.AIPNames = &a
	}
	r.log.Info("Applied saved runtime settings",
		logger.String("path", r.path),
		logger.Float64("db_retention_gb", r.settings.DBRetentionGB),
		logger.String("log_level", r.settings.LogLevel),
		logger.String("reference_airport", r.settings.ReferenceAirport),
		logger.String("also_airports", strings.Join(r.settings.AlsoAirports, ",")))
	return r
}

func normalizeAirport(code string) string {
	return strings.ToUpper(strings.TrimSpace(code))
}

// SetReferenceAirportHook wires in how a reference airport is checked and
// applied. Without it the reference airport cannot be changed from the panel:
// a setting that is saved but never applied is the failure this file refuses.
func (r *Runtime) SetReferenceAirportHook(h ReferenceAirportHook) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.airportHook = &h
}

// UseReferenceAirports records the airports actually in force at startup, the
// reference one first, when some saved from the panel turned out unusable and
// the server fell back or left them out. It is not written to disk: the saved
// choice stays, and each start says in the log why it was not used.
func (r *Runtime) UseReferenceAirports(codes []string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(codes) == 0 {
		return
	}
	r.settings.ReferenceAirport = normalizeAirport(codes[0])
	r.settings.AlsoAirports = normalizeAlso(r.settings.ReferenceAirport, codes[1:])
}

// Settings returns a copy of the values in force.
func (r *Runtime) Settings() RuntimeSettings {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s := r.settings
	if s.Matching != nil {
		m := s.Matching.clone()
		s.Matching = &m
	}
	if s.Vitals != nil {
		v := *s.Vitals
		s.Vitals = &v
	}
	if s.DBArchive != nil {
		d := *s.DBArchive
		s.DBArchive = &d
	}
	if s.AIPNames != nil {
		a := *s.AIPNames
		s.AIPNames = &a
	}
	return s
}

// AIPAutoUpdate says whether the list of official names is made again by
// itself at each new AIRAC cycle.
func (r *Runtime) AIPAutoUpdate() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.settings.AIPNames != nil && r.settings.AIPNames.AutoUpdate
}

// VitalsEnabled says whether the sign-in page's public figures are served. Read
// on every request, so turning them off from the panel applies to the next one.
func (r *Runtime) VitalsEnabled() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.settings.Vitals == nil || r.settings.Vitals.Enabled
}

// clone copies the rules, sizes included: a caller that edits its copy must
// not change the rules in force.
func (m MatchingRules) clone() MatchingRules {
	c := m
	c.Sector = make(map[string]SectorSize, len(m.Sector))
	for k, v := range m.Sector {
		c.Sector[k] = v
	}
	return c
}

// Matching returns the association rules in force. It is read for every
// transmission, so a change from the panel applies to the next one.
func (r *Runtime) Matching() MatchingRules {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.settings.Matching.clone()
}

// DBArchiveDir is where the retention archives a day before deleting it, ""
// for none; read on every pass, like the retention.
func (r *Runtime) DBArchiveDir() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.settings.DBArchive == nil {
		return ""
	}
	return r.settings.DBArchive.Dir
}

// CheckArchiveDir says whether dir can receive the database archive: absolute,
// an existing directory the server can write into, and not the databases' own
// directory nor inside it. "" is no archive, and passes. The server never
// makes the directory: under /Volumes, an unplugged disk would leave a folder
// on the internal one.
func CheckArchiveDir(dir, dataDir string) error {
	if dir == "" {
		return nil
	}
	if !filepath.IsAbs(dir) {
		return fmt.Errorf("the archive must be an absolute path, got %q", dir)
	}
	dir = filepath.Clean(dir)
	info, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("the archive %s cannot be found (is its disk plugged in?)", dir)
	}
	if !info.IsDir() {
		return fmt.Errorf("the archive %s is not a directory", dir)
	}
	if dataDir != "" {
		if data, err := filepath.Abs(dataDir); err == nil {
			if rel, err := filepath.Rel(data, dir); err == nil && (rel == "." || !strings.HasPrefix(rel, "..")) {
				return fmt.Errorf("the archive cannot be the databases' own directory %s, nor inside it", data)
			}
		}
	}
	probe, err := os.CreateTemp(dir, ".clairance-write-test-*")
	if err != nil {
		return fmt.Errorf("the archive %s cannot be written: %v", dir, err)
	}
	probe.Close()
	os.Remove(probe.Name())
	return nil
}

// DBRetentionGB is read on every retention tick rather than captured once, so a
// change from the panel takes effect on the next pass.
func (r *Runtime) DBRetentionGB() float64 {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.settings.DBRetentionGB
}

// Apply validates and applies a change, then persists it. Nothing is applied unless
// every field validates: a half-applied settings change is harder to reason about
// than a rejected one.
func (r *Runtime) Apply(next RuntimeSettings) error {
	if next.DBRetentionGB < 1 || next.DBRetentionGB > 1000 {
		return fmt.Errorf("db_retention_gb must be between 1 and 1000, got %g", next.DBRetentionGB)
	}
	switch next.LogLevel {
	case "debug", "info", "warn", "error":
	default:
		return fmt.Errorf("log_level must be debug, info, warn or error, got %q", next.LogLevel)
	}

	if next.Matching == nil {
		return fmt.Errorf("matching rules missing")
	}
	// The archive is checked only when it changes: an unplugged disk must not
	// stop the other settings from being saved.
	if next.DBArchive == nil {
		next.DBArchive = &DBArchiveSettings{Dir: r.DBArchiveDir()}
	}
	archive := DBArchiveSettings{Dir: strings.TrimSpace(next.DBArchive.Dir)}
	if archive.Dir != "" {
		archive.Dir = filepath.Clean(archive.Dir)
	}
	if archive.Dir != r.DBArchiveDir() {
		if err := CheckArchiveDir(archive.Dir, r.dataDir); err != nil {
			return err
		}
	}
	next.DBArchive = &archive
	if a := next.AIPNames; a != nil && a.Naming != "" {
		if _, ok := PlaceFloors[a.Naming]; !ok {
			return fmt.Errorf("aip_names.naming must be surer, measured or more, got %q", a.Naming)
		}
	}
	next.Matching.withSectorDefaults()
	if err := next.Matching.validate(); err != nil {
		return err
	}

	// The reference airports are checked with the rest, before anything is applied.
	next.ReferenceAirport = normalizeAirport(next.ReferenceAirport)
	next.AlsoAirports = normalizeAlso(next.ReferenceAirport, next.AlsoAirports)
	if len(next.AlsoAirports) > MaxAlsoAirports {
		return fmt.Errorf("also_airports: %d at most, got %d", MaxAlsoAirports, len(next.AlsoAirports))
	}
	r.mu.RLock()
	current, hook := r.settings.ReferenceAirports(), r.airportHook
	r.mu.RUnlock()
	airportChanged := !sameAirports(next.ReferenceAirports(), current)
	if airportChanged {
		if next.ReferenceAirport == "" {
			return fmt.Errorf("reference_airport cannot be empty")
		}
		if hook == nil {
			return fmt.Errorf("reference_airport cannot be changed while the server runs in this configuration")
		}
		if err := hook.Validate(next.ReferenceAirports()); err != nil {
			return fmt.Errorf("reference_airport: %w", err)
		}
	}

	if err := logger.SetLevel(next.LogLevel); err != nil {
		return fmt.Errorf("failed to set log level: %w", err)
	}
	if airportChanged {
		hook.Apply(next.ReferenceAirports())
	}

	m := next.Matching.clone()
	next.Matching = &m
	vitals := DefaultVitals()
	if next.Vitals != nil {
		vitals = *next.Vitals
	}
	next.Vitals = &vitals
	aipNames := AIPNamesSettings{}
	if next.AIPNames != nil {
		aipNames = *next.AIPNames
	}
	next.AIPNames = &aipNames
	r.mu.Lock()
	r.settings = next
	r.mu.Unlock()

	b, err := json.MarshalIndent(next, "", "  ")
	if err != nil {
		return err
	}
	// Written through a temporary file: a truncated settings file would be silently
	// ignored at the next start, and the change would vanish without a word.
	tmp := r.path + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o644); err != nil {
		return fmt.Errorf("failed to write runtime settings: %w", err)
	}
	if err := os.Rename(tmp, r.path); err != nil {
		return fmt.Errorf("failed to install runtime settings: %w", err)
	}

	r.log.Info("Runtime settings changed",
		logger.Float64("db_retention_gb", next.DBRetentionGB),
		logger.String("db_archive_dir", next.DBArchive.Dir),
		logger.String("log_level", next.LogLevel),
		logger.String("reference_airport", next.ReferenceAirport),
		logger.String("also_airports", strings.Join(next.AlsoAirports, ",")),
		logger.Bool("match_letters", m.Letters),
		logger.Bool("match_approx_operators", m.ApproxOperators),
		logger.Int("match_min_digits", m.MinDigits),
		logger.Bool("match_one_digit_off", m.OneDigitOff),
		logger.Bool("match_context_letters", m.ContextLetters),
		logger.Bool("match_context_digits", m.ContextDigits),
		logger.Bool("match_context_names", m.ContextNames),
		logger.Bool("match_prefix_digits", m.PrefixDigits),
		logger.Bool("match_sectors", m.Sectors),
		logger.Bool("vitals_enabled", vitals.Enabled))
	return nil
}
