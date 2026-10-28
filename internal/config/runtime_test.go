package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yegors/co-atc/pkg/logger"
)

func newRuntimeIn(t *testing.T, dir string) *Runtime {
	t.Helper()
	log, err := logger.New(logger.Config{Level: "error", Format: "console"})
	if err != nil {
		t.Fatal(err)
	}
	cfg := &Config{}
	cfg.Storage.DBRetentionGB = DefaultDBRetentionGB
	cfg.Logging.Level = "info"
	cfg.Station.AirportCode = "LFPZ"
	return NewRuntime(cfg, filepath.Join(dir, "config.toml"), log)
}

// recorder is a hook that accepts LFPO, LFPG and LFPB and remembers what it
// applied, each call's airports joined by "+".
type recorder struct{ applied []string }

func (r *recorder) hook() ReferenceAirportHook {
	return ReferenceAirportHook{
		Validate: func(codes []string) error {
			for _, code := range codes {
				if code != "LFPO" && code != "LFPG" && code != "LFPB" {
					return errors.New("unknown airport " + code)
				}
			}
			return nil
		},
		Apply: func(codes []string) { r.applied = append(r.applied, strings.Join(codes, "+")) },
	}
}

// Orly and De Gaulle together (Q40): applied in one call, the reference first,
// and kept across a restart.
func TestFurtherAirportsAreAppliedAndPersisted(t *testing.T) {
	dir := t.TempDir()
	r := newRuntimeIn(t, dir)
	rec := &recorder{}
	r.SetReferenceAirportHook(rec.hook())

	next := r.Settings()
	next.ReferenceAirport = "LFPO"
	next.AlsoAirports = []string{" lfpg ", "LFPO", "", "LFPG"} // as typed: repeats, blanks, the reference itself
	if err := r.Apply(next); err != nil {
		t.Fatalf("Orly with De Gaulle should be accepted: %v", err)
	}
	if len(rec.applied) != 1 || rec.applied[0] != "LFPO+LFPG" {
		t.Errorf("the hook should have applied LFPO+LFPG once, got %v", rec.applied)
	}
	after := newRuntimeIn(t, dir).Settings()
	if got := strings.Join(after.ReferenceAirports(), "+"); got != "LFPO+LFPG" {
		t.Errorf("after a restart, want LFPO+LFPG, got %s", got)
	}

	// Dropping De Gaulle is a change too.
	next = r.Settings()
	next.AlsoAirports = nil
	if err := r.Apply(next); err != nil {
		t.Fatal(err)
	}
	if rec.applied[len(rec.applied)-1] != "LFPO" {
		t.Errorf("want LFPO alone applied, got %v", rec.applied)
	}
}

func TestOneBadFurtherAirportRefusesTheChange(t *testing.T) {
	r := newRuntimeIn(t, t.TempDir())
	rec := &recorder{}
	r.SetReferenceAirportHook(rec.hook())
	next := r.Settings()
	next.ReferenceAirport = "LFPO"
	next.AlsoAirports = []string{"LFPG", "ZZZZ"}
	if err := r.Apply(next); err == nil {
		t.Fatal("an unknown further airport must refuse the whole change")
	}
	if len(rec.applied) != 0 || r.Settings().ReferenceAirport != "LFPZ" {
		t.Errorf("nothing should have moved: applied %v, reference %s", rec.applied, r.Settings().ReferenceAirport)
	}
}

func TestAtMostThreeFurtherAirports(t *testing.T) {
	r := newRuntimeIn(t, t.TempDir())
	r.SetReferenceAirportHook((&recorder{}).hook())
	next := r.Settings()
	next.ReferenceAirport = "LFPO"
	next.AlsoAirports = []string{"LFPG", "LFPB", "LFPN", "LFPV"}
	if err := r.Apply(next); err == nil {
		t.Error("four further airports must be refused")
	}
}

func TestReferenceAirportDefaultsToTheConfig(t *testing.T) {
	r := newRuntimeIn(t, t.TempDir())
	if got := r.Settings().ReferenceAirport; got != "LFPZ" {
		t.Errorf("should start from [station] airport_code, got %q", got)
	}
}

func TestChangingTheAirportIsAppliedAndPersisted(t *testing.T) {
	dir := t.TempDir()
	r := newRuntimeIn(t, dir)
	rec := &recorder{}
	r.SetReferenceAirportHook(rec.hook())

	next := r.Settings()
	next.ReferenceAirport = " lfpo " // as typed
	if err := r.Apply(next); err != nil {
		t.Fatalf("a valid airport should be accepted: %v", err)
	}
	if len(rec.applied) != 1 || rec.applied[0] != "LFPO" {
		t.Errorf("the hook should have applied LFPO once, got %v", rec.applied)
	}
	if got := r.Settings().ReferenceAirport; got != "LFPO" {
		t.Errorf("setting should read LFPO, got %q", got)
	}
	// And it survives a restart.
	if got := newRuntimeIn(t, dir).Settings().ReferenceAirport; got != "LFPO" {
		t.Errorf("after a restart the saved airport should win, got %q", got)
	}
}

func TestARefusedAirportLeavesNothingBehind(t *testing.T) {
	dir := t.TempDir()
	r := newRuntimeIn(t, dir)
	rec := &recorder{}
	r.SetReferenceAirportHook(rec.hook())

	next := r.Settings()
	next.ReferenceAirport = "ZZZZ"
	if err := r.Apply(next); err == nil {
		t.Fatal("an unknown airport must be refused")
	}
	if len(rec.applied) != 0 {
		t.Errorf("nothing should have been applied, got %v", rec.applied)
	}
	if got := r.Settings().ReferenceAirport; got != "LFPZ" {
		t.Errorf("the airport in force should be unchanged, got %q", got)
	}
	if _, err := os.Stat(filepath.Join(dir, RuntimeSettingsFile)); !os.IsNotExist(err) {
		t.Errorf("a refused change must not be written to disk")
	}
}

// Validation is all or nothing: a bad retention blocks a good airport too.
func TestAnInvalidFieldBlocksTheAirportToo(t *testing.T) {
	r := newRuntimeIn(t, t.TempDir())
	rec := &recorder{}
	r.SetReferenceAirportHook(rec.hook())

	next := r.Settings()
	next.ReferenceAirport = "LFPO"
	next.DBRetentionGB = 0
	if err := r.Apply(next); err == nil {
		t.Fatal("a retention of 0 must be refused")
	}
	if len(rec.applied) != 0 {
		t.Errorf("the airport must not be applied when another field is refused, got %v", rec.applied)
	}
}

func TestAnUnchangedAirportDoesNotCallTheHook(t *testing.T) {
	r := newRuntimeIn(t, t.TempDir())
	rec := &recorder{}
	r.SetReferenceAirportHook(rec.hook())

	next := r.Settings() // LFPZ, which the hook would refuse
	next.LogLevel = "warn"
	if err := r.Apply(next); err != nil {
		t.Fatalf("changing only the log level must not re-validate the airport: %v", err)
	}
	if len(rec.applied) != 0 {
		t.Errorf("the hook should not run for an unchanged airport, got %v", rec.applied)
	}
}

// Without the hook the change cannot be applied, so it must not be accepted:
// a setting saved and never applied is what this file refuses.
func TestWithoutAHookTheAirportCannotChange(t *testing.T) {
	r := newRuntimeIn(t, t.TempDir())
	next := r.Settings()
	next.ReferenceAirport = "LFPO"
	if err := r.Apply(next); err == nil {
		t.Error("without a hook, changing the airport must be refused")
	}
}

// Letters and approximate airline names are on until changed from the panel:
// the owner's decision of 26/09, after they measured +27% true matches (Q46).
// Sectors too, once validated on 26/09 (D59).
func TestMatchingRulesDefaultToLettersAndApproximateNamesOn(t *testing.T) {
	r := newRuntimeIn(t, t.TempDir())
	got := r.Matching()
	if !got.Letters || !got.ApproxOperators || got.MinDigits != 3 || !got.ContextLetters || !got.ContextDigits ||
		got.ContextNames || got.OneDigitOff || !got.Sectors {
		t.Errorf("got %+v", got)
	}
	if got.Sector["approach"] != (SectorSize{RadiusNM: 60, MaxAltFt: 20000}) || got.Sector["ground"] != (SectorSize{RadiusNM: 5, MaxAltFt: 1500}) {
		t.Errorf("sector sizes: %+v", got.Sector)
	}
}

func TestMatchingRulesChangedFromThePanelSurviveARestart(t *testing.T) {
	dir := t.TempDir()
	r := newRuntimeIn(t, dir)
	next := r.Settings()
	next.Matching.Letters, next.Matching.MinDigits = false, 4
	if err := r.Apply(next); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if got := newRuntimeIn(t, dir).Matching(); got.Letters || got.MinDigits != 4 || !got.ApproxOperators {
		t.Errorf("after a restart: %+v", got)
	}
}

func TestAMatchingRuleOutOfRangeIsRefusedWhole(t *testing.T) {
	r := newRuntimeIn(t, t.TempDir())
	next := r.Settings()
	next.Matching.MinDigits = 1
	next.LogLevel = "debug"
	if err := r.Apply(next); err == nil {
		t.Fatal("one digit should be refused")
	}
	if r.Settings().LogLevel != "info" || r.Matching().MinDigits != 3 {
		t.Error("nothing of a refused change should be applied")
	}
}

// A file saved before matching rules existed keeps its other settings and
// gets the default rules.
func TestAnOlderSettingsFileGetsTheDefaultRules(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, RuntimeSettingsFile),
		[]byte(`{"db_retention_gb": 30, "log_level": "warn"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	r := newRuntimeIn(t, dir)
	if r.Settings().DBRetentionGB != 30 || !r.Matching().Letters {
		t.Errorf("got %+v, matching %+v", r.Settings(), r.Matching())
	}
}

// A file saved when retention was a number of days keeps its other settings,
// and the retention falls back to the configured size: seven days never meant
// any particular size, so it is not converted into one.
func TestARetentionInDaysGivesWayToTheConfiguredSize(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, RuntimeSettingsFile),
		[]byte(`{"db_retention_days": 7, "log_level": "warn", "reference_airport": "LFPO"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	r := newRuntimeIn(t, dir)
	got := r.Settings()
	if got.DBRetentionGB != DefaultDBRetentionGB || got.LogLevel != "warn" || got.ReferenceAirport != "LFPO" {
		t.Errorf("got %+v", got)
	}
}

// The size is bounded: nothing below 1 GB, which would delete yesterday on a busy
// day, nor above 1000.
func TestTheRetentionSizeIsBounded(t *testing.T) {
	r := newRuntimeIn(t, t.TempDir())
	for _, gb := range []float64{0.5, 1001} {
		next := r.Settings()
		next.DBRetentionGB = gb
		if err := r.Apply(next); err == nil {
			t.Errorf("a retention of %g GB must be refused", gb)
		}
	}
	next := r.Settings()
	next.DBRetentionGB = 12.5
	if err := r.Apply(next); err != nil || r.DBRetentionGB() != 12.5 {
		t.Errorf("12.5 GB: err %v, in force %g", err, r.DBRetentionGB())
	}
}

// The sizes are the operator's to change, within bounds, and the change is kept.
func TestSectorSizesChangeWithinBoundsAndPersist(t *testing.T) {
	dir := t.TempDir()
	r := newRuntimeIn(t, dir)
	next := r.Settings()
	next.Matching.Sectors = true
	next.Matching.Sector["tower"] = SectorSize{RadiusNM: 20, MaxAltFt: 8000}
	if err := r.Apply(next); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	got := newRuntimeIn(t, dir).Matching()
	if !got.Sectors || got.Sector["tower"] != (SectorSize{RadiusNM: 20, MaxAltFt: 8000}) || got.Sector["approach"].RadiusNM != 60 {
		t.Errorf("after a restart: %+v", got)
	}
	bad := r.Settings()
	bad.Matching.Sector["ground"] = SectorSize{RadiusNM: 0, MaxAltFt: 1500}
	if err := r.Apply(bad); err == nil {
		t.Error("a zero radius should be refused")
	}
	edited := r.Matching()
	edited.Sector["approach"] = SectorSize{RadiusNM: 1, MaxAltFt: 1000}
	if r.Matching().Sector["approach"].RadiusNM != 60 {
		t.Error("editing a copy must not change the rules in force")
	}
}

// The sign-in page's figures are on until turned off; off is kept across a
// restart; a file saved before the setting existed means on.
func TestTheVitalsSettingIsOnByDefaultAndKept(t *testing.T) {
	dir := t.TempDir()
	r := newRuntimeIn(t, dir)
	if !r.VitalsEnabled() || r.Settings().Vitals == nil || !r.Settings().Vitals.Enabled {
		t.Fatal("the vitals should be on by default")
	}
	next := r.Settings()
	next.Vitals = &VitalsSettings{Enabled: false}
	if err := r.Apply(next); err != nil {
		t.Fatal(err)
	}
	if newRuntimeIn(t, dir).VitalsEnabled() {
		t.Error("turned off, the vitals came back on after a restart")
	}

	old := t.TempDir()
	if err := os.WriteFile(filepath.Join(old, RuntimeSettingsFile), []byte(`{"db_retention_gb":30,"log_level":"info"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if !newRuntimeIn(t, old).VitalsEnabled() {
		t.Error("a file saved before the setting existed turned the vitals off")
	}
}

// The database archive set from the panel (D85): absolute, found, a directory
// that can be written, outside the databases'; never made; kept across a
// restart; and an unplugged disk does not stop the other settings from being
// saved.
func TestTheDatabaseArchiveIsCheckedWhenItChanges(t *testing.T) {
	dir := t.TempDir()
	r := newRuntimeIn(t, dir)
	r.dataDir = filepath.Join(dir, "data")
	os.MkdirAll(r.dataDir, 0o755)
	if r.DBArchiveDir() != "" {
		t.Fatalf("an archive by default: %q", r.DBArchiveDir())
	}
	set := func(archive string) error {
		s := r.Settings()
		s.DBArchive = &DBArchiveSettings{Dir: archive}
		return r.Apply(s)
	}
	file := filepath.Join(dir, "a-file")
	os.WriteFile(file, nil, 0o644)
	missing := filepath.Join(dir, "unplugged", "archive")
	for _, bad := range []string{"relative/archive", missing, file, r.dataDir, filepath.Join(r.dataDir, "archive")} {
		if bad == filepath.Join(r.dataDir, "archive") {
			os.MkdirAll(bad, 0o755)
		}
		if err := set(bad); err == nil {
			t.Errorf("%q accepted as the archive", bad)
		}
	}
	if _, err := os.Stat(filepath.Dir(missing)); !os.IsNotExist(err) {
		t.Error("the archive's directory was made")
	}

	archive := t.TempDir()
	if err := set(archive + "/"); err != nil {
		t.Fatal(err)
	}
	if r.DBArchiveDir() != archive {
		t.Errorf("archive %q, want %q", r.DBArchiveDir(), archive)
	}
	if again := newRuntimeIn(t, dir); again.DBArchiveDir() != archive {
		t.Errorf("after a restart: %q", again.DBArchiveDir())
	}

	// Unplugged: the other settings still save, and the archive stays.
	os.Remove(archive)
	s := r.Settings()
	s.LogLevel = "warn"
	if err := r.Apply(s); err != nil {
		t.Errorf("a change of log level with the archive unplugged: %v", err)
	}
	if r.DBArchiveDir() != archive {
		t.Errorf("the archive changed to %q", r.DBArchiveDir())
	}
	if err := set(""); err != nil || r.DBArchiveDir() != "" {
		t.Errorf("no archive: %v, %q", err, r.DBArchiveDir())
	}
}
