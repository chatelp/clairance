package config

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// atRepoRoot runs the test from the repository root, where the
// configuration's relative paths point.
func atRepoRoot(t *testing.T) {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(filepath.Join("..", "..")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(wd) })
}

// loadAndValidate is what the server does at startup before anything else.
func loadAndValidate(t *testing.T, path string) *Config {
	t.Helper()
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load %s: %v", path, err)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("validate %s: %v", path, err)
	}
	return cfg
}

// The owner's configuration still carries [atc_chat], the OpenAI keys of
// [transcription] and those of [post_processing]. Removing the code behind them
// must not stop the server from starting with that file as it is: a key that no
// longer does anything is ignored, never refused.
func TestAConfigurationWrittenForTheRemovedPathsStillStarts(t *testing.T) {
	atRepoRoot(t)
	path := filepath.Join("internal", "config", "testdata", "legacy-full.toml")

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// The fixture is only worth something while it carries the old sections.
	for _, key := range []string{"[atc_chat]", "realtime_model", "openai_api_key", `model = "gpt-4o-transcribe"`,
		"noise_reduction", "prompt_path", "turn_detection_type", "retry_max_attempts",
		`model = "gpt-4o"`, "context_transcriptions", "system_prompt_path"} {
		if !strings.Contains(string(raw), key) {
			t.Fatalf("the fixture lost %q; it must keep every legacy key", key)
		}
	}

	cfg := loadAndValidate(t, path)

	// What the local path reads is still read.
	if cfg.Transcription.Backend != "local" || cfg.PostProcessing.Backend != "local" {
		t.Errorf("backends: transcription %q, post_processing %q, want local and local",
			cfg.Transcription.Backend, cfg.PostProcessing.Backend)
	}
	if cfg.Transcription.Local.ServerURL == "" || cfg.Transcription.Local.SegmentSilenceMs == 0 {
		t.Errorf("[transcription.local] was not read: %+v", cfg.Transcription.Local)
	}
	if cfg.PostProcessing.AirlinesDatPath == "" || cfg.PostProcessing.MinDigits == 0 {
		t.Errorf("[post_processing] local keys were not read: %+v", cfg.PostProcessing)
	}
	if len(cfg.Frequencies.Sources) == 0 || cfg.ADSB.SourceType == "" {
		t.Error("frequencies or adsb were not read")
	}

	// Its live keys are those of the example of the same day, so the defaults
	// the code grew since must leave it loading as that example does.
	full := loadAndValidate(t, frozenExample)
	cfg.ConfigPath, full.ConfigPath = "", ""
	cfg.Warnings, full.Warnings = nil, nil
	if diffs := diffValues("", reflect.ValueOf(*full), reflect.ValueOf(*cfg)); len(diffs) > 0 {
		t.Errorf("the legacy configuration loads differently from the full example:\n  %s",
			strings.Join(diffs, "\n  "))
	}
}

// legacyWarnings loads the frozen legacy configuration and returns what it
// says at startup.
func legacyWarnings(t *testing.T) []string {
	t.Helper()
	atRepoRoot(t)
	return loadAndValidate(t, filepath.Join("internal", "config", "testdata", "legacy-full.toml")).Warnings
}

// unread says what Load says about a key nothing reads.
func unread(key string) string {
	return key + " is not a configuration key of this version and is ignored: remove it, or check its spelling"
}

// ffmpegGone says what Load says about a key of ffmpeg, removed with it.
func ffmpegGone(key string) string {
	return key + " is ignored: ffmpeg is no longer used, every source is read inside the binary; remove it"
}

// ffmpegWarnings is what a file written while ffmpeg decoded the streams --
// the frozen legacy one, the full example of 10/10 -- is told of its keys.
var ffmpegWarnings = []string{
	"transcription.ffmpeg_sample_rate (24000) is read as transcription.sample_rate: write it so",
	ffmpegGone("frequencies.ffmpeg_timeout_secs"), ffmpegGone("frequencies.ffmpeg_reconnect_delay_secs"),
	ffmpegGone("transcription.ffmpeg_path"), ffmpegGone("transcription.ffmpeg_channels"),
	ffmpegGone("transcription.ffmpeg_format"),
}

// The legacy file writes both duplicates in both homes, like the example of
// its day, and 37 keys of the removed paths: the 17 of [atc_chat], 16 of
// [transcription] and 4 of [post_processing] (docs-fr/fiches-cloud/A-rapport.md);
// and the six keys of ffmpeg, removed since (B1 ter). It is told each key by
// name, the section gone in one line, and nothing else.
func TestTheLegacyConfigurationIsToldItsDuplicatesAndItsDeadKeysByName(t *testing.T) {
	got := legacyWarnings(t)
	var want []string
	want = append(want,
		"flight_phases.airport_range_nm repeats station.airport_range_nm (5): one key is enough, in [station]",
		"post_processing.airlines_dat_path repeats reference.airlines_dat_path (assets/airlines.dat): one key is enough, in [reference]",
	)
	want = append(want, ffmpegWarnings...)
	for _, key := range []string{
		"transcription.openai_api_key", "transcription.model", "transcription.noise_reduction",
		"transcription.chunk_ms", "transcription.buffer_size_kb", "transcription.reconnect_interval_sec",
		"transcription.max_retries", "transcription.turn_detection_type", "transcription.prefix_padding_ms",
		"transcription.silence_duration_ms", "transcription.vad_threshold", "transcription.retry_max_attempts",
		"transcription.retry_initial_backoff_ms", "transcription.retry_max_backoff_ms",
		"transcription.timeout_seconds", "transcription.prompt_path",
		"post_processing.model", "post_processing.context_transcriptions",
		"post_processing.timeout_seconds", "post_processing.system_prompt_path",
	} {
		want = append(want, unread(key))
	}
	want = append(want, "[atc_chat] is not a section of this version and is ignored with its 17 keys: remove it")
	if len(got) != len(want) {
		t.Errorf("%d warnings, want %d:\n  %s", len(got), len(want), strings.Join(got, "\n  "))
	}
	for _, w := range want {
		found := false
		for _, g := range got {
			found = found || g == w
		}
		if !found {
			t.Errorf("missing %q", w)
		}
	}
}

// Unknown keys are said and ignored in every section, [adsb] included, which
// upstream alone refused. Starting on a line that does nothing beats not
// starting over it.
func TestAnUnknownKeyIsSaidByNameAndNeverRefused(t *testing.T) {
	atRepoRoot(t)
	text := withLines(withLines(minimalConfig, "adsb", "fetch_interval_secs = 2"), "server", "prot = 8000")
	text += "\n[voice_chat]\nenabled = true\n"
	cfg := loadAndValidate(t, writeConfig(t, text))
	want := []string{unread("adsb.fetch_interval_secs"), unread("server.prot"),
		"[voice_chat] is not a section of this version and is ignored with its 1 key: remove it"}
	if !reflect.DeepEqual(cfg.Warnings, want) {
		t.Errorf("warnings:\n  %s\nwant:\n  %s", strings.Join(cfg.Warnings, "\n  "), strings.Join(want, "\n  "))
	}
	if cfg.ADSB.FetchIntervalSecs != 1 {
		t.Errorf("the misspelled key must not have changed fetch_interval_seconds: %d", cfg.ADSB.FetchIntervalSecs)
	}
}

// The example shipped with the repository must start as it is.
func TestTheExampleConfigurationStarts(t *testing.T) {
	atRepoRoot(t)
	loadAndValidate(t, filepath.Join("configs", "config.toml.example"))
}

// legacyWithBackend writes the frozen legacy configuration with the backend of
// one section ("transcription" or "post_processing") changed to line.
func legacyWithBackend(t *testing.T, section, line string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("internal", "config", "testdata", "legacy-full.toml"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	start := strings.Index(text, "\n["+section+"]\n")
	at := strings.Index(text[start:], "\nbackend = \"local\"\n")
	if start < 0 || at < 0 {
		t.Fatalf("fixture has no backend in [%s]", section)
	}
	at += start + 1
	text = text[:at] + line + text[at+len(`backend = "local"`):]
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// The OpenAI backend is gone. A configuration that still asks for it must stop
// the server with the reason, not start it transcribing nothing.
func TestTheRemovedOpenAIBackendIsRefusedWithItsReason(t *testing.T) {
	atRepoRoot(t)
	for _, section := range []string{"transcription", "post_processing"} {
		t.Run(section, func(t *testing.T) {
			cfg, err := Load(legacyWithBackend(t, section, `backend = "openai"`))
			if err != nil {
				t.Fatal(err)
			}
			err = cfg.Validate()
			want := section + `.backend = "openai": the OpenAI backend was removed, use "local"`
			if err == nil || !strings.Contains(err.Error(), want) {
				t.Fatalf("got %v, want %q", err, want)
			}
		})
	}
}

// An empty backend used to mean OpenAI. There is now one backend, so empty
// means local.
func TestAnEmptyBackendMeansLocal(t *testing.T) {
	atRepoRoot(t)
	for _, section := range []string{"transcription", "post_processing"} {
		cfg := loadAndValidate(t, legacyWithBackend(t, section, "# no backend"))
		if cfg.Transcription.Backend != BackendLocal || cfg.PostProcessing.Backend != BackendLocal {
			t.Errorf("[%s] without backend: got %q and %q, want %q", section,
				cfg.Transcription.Backend, cfg.PostProcessing.Backend, BackendLocal)
		}
	}
}

// frozenExample is configs/config.toml.example as it stood before its
// reduction (docs-fr/fiches-cloud/D-config-reduite.md), every key written out.
const frozenExample = "internal/config/testdata/example-2026-10-10.toml"

// The reduction removes from the example every key whose value is the code's
// default. Absent, such a key must give exactly the value it gave present: the
// frozen full example and the shipped one must load, validate and come out as
// the same Config, field by field.
func TestTheReducedExampleLoadsTheSameConfiguration(t *testing.T) {
	atRepoRoot(t)
	full := loadAndValidate(t, frozenExample)
	reduced := loadAndValidate(t, filepath.Join("configs", "config.toml.example"))

	// The full example writes airport_range_nm and airlines_dat_path in their
	// two homes, and the keys of ffmpeg, removed since (B1 ter), which is
	// exactly what it says at startup, and nothing else.
	want := []string{
		"flight_phases.airport_range_nm repeats station.airport_range_nm (5): one key is enough, in [station]",
		"post_processing.airlines_dat_path repeats reference.airlines_dat_path (assets/airlines.dat): one key is enough, in [reference]",
	}
	want = append(want, ffmpegWarnings...)
	if !reflect.DeepEqual(full.Warnings, want) {
		t.Errorf("full example warnings:\n  %s\nwant:\n  %s",
			strings.Join(full.Warnings, "\n  "), strings.Join(want, "\n  "))
	}

	// The reduced one has nothing to be told.
	if len(reduced.Warnings) != 0 {
		t.Errorf("the shipped example says at startup:\n  %s", strings.Join(reduced.Warnings, "\n  "))
	}

	// Where each was read from, and what each had to say, is not configuration.
	full.ConfigPath, reduced.ConfigPath = "", ""
	full.Warnings, reduced.Warnings = nil, nil

	if diffs := diffValues("", reflect.ValueOf(*full), reflect.ValueOf(*reduced)); len(diffs) > 0 {
		t.Errorf("the reduced example loads differently from the full one:\n  %s",
			strings.Join(diffs, "\n  "))
	}
	if !reflect.DeepEqual(full, reduced) {
		t.Error("reflect.DeepEqual disagrees with the field walk; the walk misses something")
	}
}

// diffValues walks two values of the same type and names every leaf that
// differs, as section.field = full | reduced, so a failure says which key.
func diffValues(path string, a, b reflect.Value) []string {
	switch a.Kind() {
	case reflect.Struct:
		var out []string
		for i := 0; i < a.NumField(); i++ {
			name := a.Type().Field(i).Tag.Get("toml")
			if name == "" || name == "-" {
				name = a.Type().Field(i).Name
			}
			out = append(out, diffValues(path+"."+name, a.Field(i), b.Field(i))...)
		}
		return out
	case reflect.Slice:
		if a.Len() != b.Len() {
			return []string{fmt.Sprintf("%s: %d elements | %d", path, a.Len(), b.Len())}
		}
		var out []string
		for i := 0; i < a.Len(); i++ {
			out = append(out, diffValues(fmt.Sprintf("%s[%d]", path, i), a.Index(i), b.Index(i))...)
		}
		return out
	case reflect.Ptr:
		if a.IsNil() != b.IsNil() {
			return []string{fmt.Sprintf("%s: %v | %v", path, a, b)}
		}
		if a.IsNil() {
			return nil
		}
		return diffValues(path, a.Elem(), b.Elem())
	default:
		if !reflect.DeepEqual(a.Interface(), b.Interface()) {
			return []string{fmt.Sprintf("%s: %v | %v", path, a.Interface(), b.Interface())}
		}
		return nil
	}
}

// writeConfig puts text in a temporary config.toml and returns its path.
func writeConfig(t *testing.T, text string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// minimalConfig says only what no default can guess: where the aircraft come
// from, where the station is, and which weather to fetch.
const minimalConfig = `
[adsb]
source_type = "tar1090"
tar1090_base_url = "http://receiver.local/tar1090/data/"

[station]
latitude = 48.8
longitude = 2.1
airport_code = "LFPO"

[wx]
fetch_metar = true
`

// A key left out holds the code's default, for every section: the almost
// empty configuration above validates and comes out as Defaults plus what it
// said.
func TestAnAlmostEmptyConfigurationStartsOnTheDefaults(t *testing.T) {
	atRepoRoot(t)
	cfg := loadAndValidate(t, writeConfig(t, minimalConfig))

	want := Defaults()
	want.ADSB.SourceType, want.ADSB.Tar1090BaseURL = "tar1090", "http://receiver.local/tar1090/data/"
	want.Station.Latitude, want.Station.Longitude, want.Station.AirportCode = 48.8, 2.1, "LFPO"
	want.Weather.FetchMETAR = true
	want.PostProcessing.AirlinesDatPath = want.Reference.AirlinesDATPath // the older home, filled from the new one
	cfg.ConfigPath = ""
	if diffs := diffValues("", reflect.ValueOf(want), reflect.ValueOf(*cfg)); len(diffs) > 0 {
		t.Errorf("not the defaults (want | got):\n  %s", strings.Join(diffs, "\n  "))
	}
	for _, check := range []struct {
		name string
		got  any
		want any
	}{
		{"cruise_altitude_ft", cfg.FlightPhases.CruiseAltitudeFt, 18000},
		{"flying_min_alt_ft", cfg.FlightPhases.FlyingMinAltFt, 700.0},
		{"airlines_dat_path", cfg.Reference.AirlinesDATPath, "assets/airlines.dat"},
		{"server_url", cfg.Transcription.Local.ServerURL, "http://127.0.0.1:8178"},
		{"opensky_base_url", cfg.ADSB.OpenSkyBaseURL, "https://opensky-network.org/api"},
		{"db_retention_gb", cfg.Storage.DBRetentionGB, DefaultDBRetentionGB},
	} {
		if !reflect.DeepEqual(check.got, check.want) {
			t.Errorf("%s: got %v, want %v", check.name, check.got, check.want)
		}
	}
}

// A default fills what is absent, not what is wrong: a value written out of
// bounds is still refused, with its name.
func TestAValueOutOfBoundsIsStillRefused(t *testing.T) {
	atRepoRoot(t)
	for _, bad := range []struct{ section, line, want string }{
		{"flight_phases", "enabled = true\ncruise_altitude_ft = 0", "cruise_altitude_ft must be positive"},
		{"adsb", "fetch_interval_seconds = 0", "invalid fetch interval"},
		{"wx", "refresh_interval_minutes = -1", "refresh_interval_minutes must be greater than 0"},
		{"logging", `level = "loud"`, "invalid log level"},
		{"server", `allowed_origins = ["*"]`, "server.allowed_origins"},
		{"server", `allowed_origins = ["mac.local:8000"]`, "want scheme://host[:port]"},
		{"server", `audio_replay = "everyone"`, "invalid server.audio_replay"},
		{"transcription", "audio_archive_max_gb = -1", "invalid transcription.audio_archive_max_gb"},
		{"transcription", "raw_log_max_days = -1", "invalid transcription.raw_log_max_days"},
		{"transcription", "sample_rate = 0", "invalid transcription.sample_rate"},
		{"transcription", "ffmpeg_sample_rate = 192000", "invalid transcription.sample_rate"},
	} {
		cfg, err := Load(writeConfig(t, withLines(minimalConfig, bad.section, bad.line)))
		if err != nil {
			t.Fatal(err)
		}
		err = cfg.Validate()
		if err == nil || !strings.Contains(err.Error(), bad.want) {
			t.Errorf("[%s] %s: got %v, want %q", bad.section, bad.line, err, bad.want)
		}
	}
}

// withLines adds lines to the section of a TOML text, after the header when
// the section exists and as a new table otherwise (TOML refuses a table
// written twice).
func withLines(text, section, lines string) string {
	header := "[" + section + "]\n"
	if strings.Contains(text, header) {
		return strings.Replace(text, header, header+lines+"\n", 1)
	}
	return text + "\n" + header + lines + "\n"
}

// airport_range_nm belongs to [station] and airlines_dat_path to [reference].
// The older key of each is still read, and said; written nowhere, the new
// home's value serves both readers.
func TestADuplicateKeyHasOneHomeAndTheOldOneIsStillRead(t *testing.T) {
	atRepoRoot(t)
	cases := []struct {
		name        string
		text        string
		rangeNM     float64
		airlines    string
		wantWarning string
	}{
		{"neither written: the default serves both",
			minimalConfig, 5, "assets/airlines.dat", ""},
		{"new home only: its value serves both",
			withLines(withLines(minimalConfig, "station", "airport_range_nm = 7"), "reference", `airlines_dat_path = "x/airlines.dat"`),
			7, "x/airlines.dat", ""},
		{"old home only: read, and told where it belongs",
			withLines(withLines(minimalConfig, "flight_phases", "airport_range_nm = 7"), "post_processing", `airlines_dat_path = "x/airlines.dat"`),
			7, "x/airlines.dat", "flight_phases.airport_range_nm (7) is read as station.airport_range_nm: write it in [station]"},
		{"both, equal: told it repeats",
			withLines(withLines(minimalConfig, "flight_phases", "airport_range_nm = 5"), "station", "airport_range_nm = 5"),
			5, "assets/airlines.dat", "flight_phases.airport_range_nm repeats station.airport_range_nm (5): one key is enough, in [station]"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := loadAndValidate(t, writeConfig(t, tc.text))
			if cfg.Station.AirportRangeNM != tc.rangeNM || cfg.FlightPhases.AirportRangeNM != tc.rangeNM {
				t.Errorf("airport_range_nm: station %v, flight_phases %v, want both %v",
					cfg.Station.AirportRangeNM, cfg.FlightPhases.AirportRangeNM, tc.rangeNM)
			}
			if cfg.Reference.AirlinesDATPath != tc.airlines || cfg.PostProcessing.AirlinesDatPath != tc.airlines {
				t.Errorf("airlines_dat_path: reference %q, post_processing %q, want both %q",
					cfg.Reference.AirlinesDATPath, cfg.PostProcessing.AirlinesDatPath, tc.airlines)
			}
			if tc.wantWarning == "" {
				if len(cfg.Warnings) != 0 {
					t.Errorf("nothing to say, said %q", cfg.Warnings)
				}
			} else if len(cfg.Warnings) == 0 || cfg.Warnings[0] != tc.wantWarning {
				t.Errorf("warnings %q, want first %q", cfg.Warnings, tc.wantWarning)
			}
		})
	}

	// Written differently in both homes, each reader keeps its own value, as
	// before, and the file is told.
	cfg := loadAndValidate(t, writeConfig(t, withLines(withLines(minimalConfig, "flight_phases", "airport_range_nm = 8"), "station", "airport_range_nm = 5")))
	if cfg.Station.AirportRangeNM != 5 || cfg.FlightPhases.AirportRangeNM != 8 {
		t.Errorf("both written differently: station %v, flight_phases %v, want 5 and 8", cfg.Station.AirportRangeNM, cfg.FlightPhases.AirportRangeNM)
	}
	want := "flight_phases.airport_range_nm (8) differs from station.airport_range_nm (5): both are kept as written, but the key belongs in [station] alone"
	if len(cfg.Warnings) != 1 || cfg.Warnings[0] != want {
		t.Errorf("warnings %q, want %q", cfg.Warnings, want)
	}
}

// The page is built into the binary: a server started where there is no www/
// is valid. A static_dir that names no directory is refused -- it would serve
// a 404 for every file of the page.
func TestThePageNeedsNoDirectoryUnlessOneIsNamed(t *testing.T) {
	atRepoRoot(t)
	cfg, err := Load(filepath.Join("configs", "config.toml.example"))
	if err != nil {
		t.Fatal(err)
	}
	wd, _ := os.Getwd()
	elsewhere := t.TempDir()
	if err := os.Chdir(elsewhere); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(wd)
	if err := cfg.Validate(); err != nil {
		t.Errorf("no www/ beside the server, no static_dir: %v", err)
	}
	cfg.Server.StaticDir = filepath.Join(elsewhere, "missing")
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "static_dir") {
		t.Errorf("static_dir naming nothing: %v, want a refusal naming static_dir", err)
	}
	if err := os.WriteFile(filepath.Join(elsewhere, "file"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	cfg.Server.StaticDir = filepath.Join(elsewhere, "file")
	if err := cfg.Validate(); err == nil {
		t.Error("static_dir naming a file: accepted")
	}
	cfg.Server.StaticDir = elsewhere
	if err := cfg.Validate(); err != nil {
		t.Errorf("static_dir naming a directory: %v", err)
	}
}

// [station_link] is off unless said; on, it needs the station's two addresses
// and a server the link can reach on the loopback without TLS
// (docs-fr/36-contrat-lanceur.md).
func TestTheStationLinkIsOffByDefaultAndCheckedWhenOn(t *testing.T) {
	atRepoRoot(t)
	if cfg := loadAndValidate(t, writeConfig(t, minimalConfig)); cfg.StationLink.Enabled || cfg.StationLink.IntervalSeconds != 10 {
		t.Errorf("by default: %+v", cfg.StationLink)
	}

	on := withLines(minimalConfig, "station_link", "enabled = true\nstation_url = \"http://station.lan\"\nstreams_url = \"http://audio.lan\"")
	cfg := loadAndValidate(t, writeConfig(t, on))
	if l := cfg.StationLink; !l.Enabled || l.StationURL != "http://station.lan" || l.StreamsURL != "http://audio.lan" || l.IntervalSeconds != 10 {
		t.Errorf("on: %+v", l)
	}
	if len(cfg.Warnings) != 0 {
		t.Errorf("warnings: %q", cfg.Warnings)
	}
	for _, host := range []string{"0.0.0.0", "::", "::1", "localhost"} {
		if err := validated(t, withLines(on, "server", fmt.Sprintf("host = %q", host))); err != nil {
			t.Errorf("host %s: %v", host, err)
		}
	}

	for _, bad := range []struct{ text, want string }{
		{withLines(minimalConfig, "station_link", "enabled = true\nstreams_url = \"http://audio.lan\""), "station_url"},
		{withLines(on, "station_link", `interval_seconds = 0`), "interval_seconds"},
		{withLines(on, "server", `host = "192.0.2.10"`), "loopback"},
		{withLines(on, "server", "tls_cert = \"c.pem\"\ntls_key = \"k.pem\""), "tls_cert"},
	} {
		if err := validated(t, bad.text); err == nil || !strings.Contains(err.Error(), bad.want) {
			t.Errorf("%q: err = %v, want one naming %s", bad.text, err, bad.want)
		}
	}
	// Off, none of that is asked.
	if err := validated(t, withLines(minimalConfig, "server", `host = "192.0.2.10"`)); err != nil {
		t.Errorf("off, on a network address: %v", err)
	}
}

func validated(t *testing.T, text string) error {
	t.Helper()
	cfg, err := Load(writeConfig(t, text))
	if err != nil {
		t.Fatal(err)
	}
	return cfg.Validate()
}
