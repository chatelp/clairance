package startcheck

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yegors/co-atc/internal/config"
	"github.com/yegors/co-atc/pkg/logger"
	"github.com/yegors/co-atc/pkg/tool"
)

const gib = 1 << 30

// station is a configuration with everything there: pdftotext and python3 on
// a PATH of their own (and nowhere else), an MP3 stream, a data directory,
// the airport layout made, a disk with room. Each test takes one thing away.
type station struct {
	cfg  *config.Config
	bin  string // the PATH's only directory
	free int64  // what every disk has free
}

func newStation(t *testing.T) *station {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "bin")
	for _, name := range []string{"pdftotext", "python3"} {
		install(t, bin, name)
	}
	saved := tool.Dirs
	t.Cleanup(func() { tool.Dirs = saved })
	tool.Dirs = nil
	t.Setenv("PATH", bin)

	cfg := &config.Config{ConfigPath: filepath.Join(dir, "config.toml")}
	cfg.Storage.SQLiteBasePath = filepath.Join(dir, "data")
	cfg.Frequencies.Sources = []config.FrequencyConfig{{ID: "app", URL: "http://station.lan:8000/aero-124350.mp3"}}
	if err := os.MkdirAll(cfg.StationFilesDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfg.StationFilesDir(), "airport-layout.geojson"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	return &station{cfg: cfg, bin: bin, free: 500 * gib}
}

func install(t *testing.T, dir, name string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
}

func (s *station) run(retentionGB float64) Report {
	return Run(Inputs{Config: s.cfg, DBRetentionGB: retentionGB, Free: func(string) (int64, bool) { return s.free, true }})
}

// only says the report holds exactly these, by What, at these levels.
func only(t *testing.T, r Report, want map[string]string) {
	t.Helper()
	got := map[string]string{}
	for _, m := range r.Missing {
		got[m.What] = m.Level
		if m.Effect == "" || m.Kind == "" {
			t.Errorf("%s: no effect or no kind: %+v", m.What, m)
		}
	}
	if len(got) != len(want) {
		t.Errorf("missing %v, want %v", got, want)
		return
	}
	for what, level := range want {
		if got[what] != level {
			t.Errorf("missing %v, want %v", got, want)
			return
		}
	}
}

func find(r Report, what string) Missing {
	for _, m := range r.Missing {
		if m.What == what {
			return m
		}
	}
	return Missing{}
}

// Everything there: nothing missing, and the list is empty rather than null
// for the launcher.
func TestNothingMissingIsAnEmptyList(t *testing.T) {
	s := newStation(t)
	r := s.run(20)
	only(t, r, nil)
	b, err := json.Marshal(r.Missing)
	if err != nil || string(b) != "[]" {
		t.Errorf("nothing missing encodes as %s, want []", b)
	}
	var found []string
	for _, f := range r.Found {
		found = append(found, f.What)
	}
	if strings.Join(found, ",") != "pdftotext" {
		t.Errorf("found %v: pdftotext alone is a program the server runs", found)
	}
}

// With an empty PATH and nothing where Homebrew puts things -- a server
// started by launchd -- the audio still has everything it needs: ffmpeg is
// never asked for, whatever the sources, none configured included (one added
// later from the panel is read in Go too). pdftotext is a warning.
func TestFFmpegIsNeverAskedFor(t *testing.T) {
	s := newStation(t)
	t.Setenv("PATH", "")
	s.cfg.Frequencies.Sources = append(s.cfg.Frequencies.Sources,
		config.FrequencyConfig{ID: "sdr", URL: "udp://127.0.0.1:7355", PCMRate: 48000})
	r := s.run(20)
	only(t, r, map[string]string{"pdftotext": LevelWarn})
	if m := find(r, "pdftotext"); !strings.Contains(m.Effect, "charts") {
		t.Errorf("pdftotext should say what the AIP names lose: %+v", m)
	}
	s.cfg.Frequencies.Sources = nil
	only(t, s.run(20), map[string]string{"pdftotext": LevelWarn})
	for _, f := range r.Found {
		if strings.Contains(f.What, "ffmpeg") {
			t.Errorf("ffmpeg looked for: %+v", f)
		}
	}
}

// A source this version cannot read -- one ffmpeg read before it was removed,
// raw PCM without its rate -- is an error that names it and says why; an MP3
// stream and raw PCM with its rate are read, and say nothing.
func TestASourceThatCannotBeReadIsNamed(t *testing.T) {
	s := newStation(t)
	s.cfg.Frequencies.Sources = append(s.cfg.Frequencies.Sources,
		config.FrequencyConfig{ID: "sdr", URL: "udp://127.0.0.1:7355", PCMRate: 48000, PCMChannels: 2},
		config.FrequencyConfig{ID: "local", URL: "srt://192.0.2.1:42069"},
		config.FrequencyConfig{ID: "scanner", URL: ":0"},
		config.FrequencyConfig{ID: "gqrx", URL: "udp://127.0.0.1:7356"},
	)
	r := s.run(20)
	only(t, r, map[string]string{"the source of local": LevelError, "the source of scanner": LevelError,
		"the source of gqrx": LevelError})
	for id, says := range map[string]string{"local": "SRT and sound cards went with ffmpeg", "scanner": "not a source this version reads",
		"gqrx": "pcm_rate"} {
		m := find(r, "the source of "+id)
		if m.Kind != "source" || !strings.Contains(m.Effect, "no audio on "+id) || !strings.Contains(m.Effect, says) || m.Hint == "" {
			t.Errorf("%s: %+v, want its reason %q", id, m, says)
		}
	}
}

// The sidecar's program and script, when the server starts it: an error each,
// since the server stops without the transcription it is asked for.
func TestTheSidecarsProgramAndScriptAreErrors(t *testing.T) {
	s := newStation(t)
	s.cfg.Transcription.Local.Command = []string{"/nonexistent/venv/bin/python3", "nonexistent/whisper_server.py", "--preload"}
	r := s.run(20)
	only(t, r, map[string]string{"the sidecar's program /nonexistent/venv/bin/python3": LevelError, "the sidecar's script": LevelError})
	if m := find(r, "the sidecar's script"); !strings.Contains(m.Effect, "server stops") || !filepath.IsAbs(m.Path) {
		t.Errorf("script: %+v", m)
	}

	// On the PATH, and a script that is there: nothing.
	script := filepath.Join(t.TempDir(), "whisper_server.py")
	if err := os.WriteFile(script, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	s.cfg.Transcription.Local.Command = []string{"python3", script, "--preload"}
	only(t, s.run(20), nil)

	// The same with an empty PATH.
	t.Setenv("PATH", "")
	only(t, s.run(20), map[string]string{"the sidecar's program python3": LevelError, "pdftotext": LevelWarn})

	// No command: a sidecar run elsewhere, nothing to check here.
	s.cfg.Transcription.Local.Command = nil
	s.cfg.Frequencies.Sources[0].TranscribeAudio = true
	only(t, s.run(20), map[string]string{"pdftotext": LevelWarn})
}

// A model named by a path is checked: the English one is an error, the French
// one and the detector warnings. A Hugging Face id is downloaded by the
// sidecar and not judged.
func TestModelsNamedByAPathAreChecked(t *testing.T) {
	s := newStation(t)
	script := filepath.Join(t.TempDir(), "whisper_server.py")
	if err := os.WriteFile(script, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	s.cfg.Transcription.Local.Command = []string{"python3", script,
		"--model-en", "/Volumes/Unplugged/whisper-large-v3-atco2-asr-mlx",
		"--model-fr=./nonexistent/whisper-large-v3-french-mlx",
		"--fr-detector", "~/nonexistent-clairance-model"}
	r := s.run(20)
	only(t, r, map[string]string{"the model of --model-en": LevelError, "the model of --model-fr": LevelWarn,
		"the model of --fr-detector": LevelWarn})
	if m := find(r, "the model of --model-en"); m.Path != "/Volumes/Unplugged/whisper-large-v3-atco2-asr-mlx" {
		t.Errorf("model path: %+v", m)
	}

	s.cfg.Transcription.Local.Command = []string{"python3", script,
		"--model-en", "mlx-community/whisper-large-v3-turbo", "--model-fr", t.TempDir()}
	only(t, s.run(20), nil)

	// models/... from where the server runs, as the example names a
	// faster-whisper model: a path when models/ is here.
	wd, _ := os.Getwd()
	here := t.TempDir()
	if err := os.MkdirAll(filepath.Join(here, "models"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(here); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(wd)
	s.cfg.Transcription.Local.Command = []string{"python3", script, "--model-en", "models/whisper-large-v3-atco2-asr-ct2"}
	only(t, s.run(20), map[string]string{"the model of --model-en": LevelError})
}

// A data directory that cannot be written stops the server: an error.
func TestADataDirectoryThatCannotBeWrittenIsAnError(t *testing.T) {
	s := newStation(t)
	file := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(file, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	s.cfg.Storage.SQLiteBasePath = filepath.Join(file, "data")
	r := s.run(20)
	if m := find(r, "the data directory"); m.Level != LevelError || !strings.Contains(m.Effect, "server stops") {
		t.Errorf("data directory under a file: %+v", r.Missing)
	}

	// Not made yet, under a directory that can be written: nothing; and the
	// check made nothing.
	s.cfg.Storage.SQLiteBasePath = filepath.Join(t.TempDir(), "later", "data")
	if m := find(s.run(20), "the data directory"); m.What != "" {
		t.Errorf("a data directory still to be made: %+v", m)
	}
	if _, err := os.Stat(s.cfg.Storage.SQLiteBasePath); !os.IsNotExist(err) {
		t.Errorf("the check made the data directory: %v", err)
	}
}

// The databases' disk is judged against the retention in force: what they may
// still grow to before the oldest day is deleted must fit.
func TestTheDatabasesDiskIsJudgedAgainstTheRetention(t *testing.T) {
	s := newStation(t)
	if err := os.WriteFile(filepath.Join(s.cfg.Storage.SQLiteBasePath, "co-atc-2026-10-25.db"), make([]byte, 1<<20), 0o644); err != nil {
		t.Fatal(err)
	}
	s.free = 5 * gib
	r := s.run(20)
	only(t, r, map[string]string{"free space for the databases": LevelWarn})
	if m := find(r, "free space for the databases"); !strings.Contains(m.Effect, "db_retention_gb 20") || !strings.Contains(m.Effect, "5.0 GB free") {
		t.Errorf("databases: %q", m.Effect)
	}
	// A retention the disk holds: nothing. The panel's value is the one used.
	only(t, s.run(4), nil)
	// Already past the retention: nothing more to grow.
	s.free = 0
	only(t, s.run(0.0005), nil)
}

// The archive's disk is judged against the sidecar's floor, its
// --save-audio-min-free-gb or 8 GB, where the sidecar measures it.
func TestTheArchiveDiskIsJudgedAgainstTheSidecarsFloor(t *testing.T) {
	s := newStation(t)
	script := filepath.Join(t.TempDir(), "whisper_server.py")
	if err := os.WriteFile(script, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(t.TempDir(), "not-yet", "archive")
	s.cfg.Transcription.Local.Command = []string{"python3", script, "--save-audio", archive}
	var asked []string
	free := int64(6 * gib)
	run := func() Report {
		asked = nil
		return Run(Inputs{Config: s.cfg, DBRetentionGB: 20, Free: func(dir string) (int64, bool) {
			if dir == s.cfg.Storage.SQLiteBasePath {
				return s.free, true // the databases' disk has room
			}
			asked = append(asked, dir)
			return free, true
		}})
	}
	r := run()
	only(t, r, map[string]string{"free space for the audio archive": LevelWarn})
	if m := find(r, "free space for the audio archive"); !strings.Contains(m.Effect, "below 8 GB") || m.Path != archive {
		t.Errorf("archive: %+v", m)
	}
	if want := filepath.Dir(filepath.Dir(archive)); asked[len(asked)-1] != want {
		t.Errorf("measured on %s, want the first parent that exists, %s", asked[len(asked)-1], want)
	}
	s.cfg.Transcription.Local.Command = append(s.cfg.Transcription.Local.Command, "--save-audio-min-free-gb=2")
	only(t, run(), nil)

	// Named by audio_archive_dir, for a sidecar run elsewhere: its default floor.
	s.cfg.Transcription.Local.Command = nil
	s.cfg.Transcription.Local.AudioArchiveDir = archive
	only(t, run(), map[string]string{"free space for the audio archive": LevelWarn})
	free = 9 * gib
	only(t, run(), nil)
}

// The station's own files: in the data directory's station/, or in the page
// directory's data/ when the page is served from one.
func TestTheStationsOwnFilesAreLookedForWhereTheyAreServedFrom(t *testing.T) {
	s := newStation(t)
	if err := os.Remove(filepath.Join(s.cfg.StationFilesDir(), "airport-layout.geojson")); err != nil {
		t.Fatal(err)
	}
	r := s.run(20)
	only(t, r, map[string]string{"airport-layout.geojson": LevelWarn})
	if m := find(r, "airport-layout.geojson"); !strings.Contains(m.Effect, "airport layout") ||
		!strings.Contains(m.Hint, "fetch.py --config "+s.cfg.ConfigPath) || m.Path != filepath.Join(s.cfg.StationFilesDir(), "airport-layout.geojson") {
		t.Errorf("layout: %+v", m)
	}
	pages := t.TempDir()
	install(t, filepath.Join(pages, "data"), "airport-layout.geojson")
	s.cfg.Server.StaticDir = pages
	only(t, s.run(20), nil)
}

// One line per thing missing, at its level, and a last line that counts.
func TestTheReportIsOneLineEachAtItsLevel(t *testing.T) {
	s := newStation(t)
	t.Setenv("PATH", "")
	s.cfg.Transcription.Local.Command = []string{"python3"}
	path := filepath.Join(t.TempDir(), "check.log")
	log, err := logger.New(logger.Config{Level: "info", Format: "json", File: path})
	if err != nil {
		t.Fatal(err)
	}
	s.run(20).Log(log)
	_ = log.Sync()
	files, _ := filepath.Glob(filepath.Join(filepath.Dir(path), "check*.log"))
	if len(files) != 1 {
		t.Fatalf("log files: %v", files)
	}
	raw, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	var lines []struct{ Level, Msg string }
	for _, l := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		var e struct{ Level, Msg string }
		if err := json.Unmarshal([]byte(l), &e); err != nil {
			t.Fatalf("%q: %v", l, err)
		}
		lines = append(lines, e)
	}
	want := []struct{ level, prefix string }{
		{"warn", "Missing: pdftotext -- "},
		{"error", "Missing: the sidecar's program python3 -- the transcription sidecar cannot start"},
		{"info", "Startup check: 2 missing"},
	}
	if len(lines) != len(want) {
		t.Fatalf("lines: %+v", lines)
	}
	for i, w := range want {
		if lines[i].Level != w.level || !strings.HasPrefix(lines[i].Msg, w.prefix) {
			t.Errorf("line %d: %s %q, want %s %q...", i, lines[i].Level, lines[i].Msg, w.level, w.prefix)
		}
	}
}
