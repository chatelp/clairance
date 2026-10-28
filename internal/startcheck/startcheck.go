// Package startcheck is what the server checks at start, before it relies on
// it (docs-fr/35-plan.md, B5): the programs it runs, the sidecar's program,
// script and models, the free space of the disks it writes to, its data
// directory, the station's own files. Each absence is named with what suffers
// from it, one line in the log and one entry in /api/v1/health/detail for the
// launcher. The server starts all the same: it says what is missing, it does
// not refuse to run. On 26/10 production started without ffmpeg on its PATH
// and had no audio at all, which nothing said; ffmpeg has since been removed
// (B1 ter), the audio is read inside the binary, and is not looked for.
package startcheck

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/yegors/co-atc/internal/audio"
	"github.com/yegors/co-atc/internal/audioarchive"
	"github.com/yegors/co-atc/internal/config"
	"github.com/yegors/co-atc/pkg/disk"
	"github.com/yegors/co-atc/pkg/logger"
	"github.com/yegors/co-atc/pkg/tool"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Missing is one thing the server depends on and did not find at start.
type Missing struct {
	Kind   string `json:"kind"`           // source, tool, sidecar, model, disk, directory, station_file
	What   string `json:"what"`           // what is missing, by name
	Path   string `json:"path,omitempty"` // where it was looked for
	Effect string `json:"effect"`         // what suffers from it
	Level  string `json:"level"`          // LevelError or LevelWarn
	Hint   string `json:"hint,omitempty"` // what to do about it
}

// The two levels: an absence that stops the audio, the transcription or the
// databases is an error; one that costs less is a warning.
const (
	LevelError = "error"
	LevelWarn  = "warn"
)

// Found is a program looked for and found, said at start with where: what a
// launcher's PATH holds is the question of 26/10.
type Found struct {
	What string
	Path string
}

// Report is what the check found missing, and the programs it found.
type Report struct {
	Missing []Missing // never nil: empty when nothing is missing
	Found   []Found
}

// Inputs is what the check reads: the configuration; the databases' retention
// in force, the settings panel's, which may differ from the file's; the free
// space of the filesystem that holds a directory, disk.Space when nil.
type Inputs struct {
	Config        *config.Config
	DBRetentionGB float64
	DBArchiveDir  string // the archive in force: the panel's, else [storage] archive_dir
	Free          func(dir string) (int64, bool)
}

// The sidecar's own defaults, from sidecar/config.py: archiving stops below
// this much free space, unless --save-audio-min-free-gb says otherwise.
const sidecarMinFreeGB = 8.0

// stationPageFiles are the page's files that belong to one station, with
// what the page lacks without each and the tool that makes it.
var stationPageFiles = []struct{ Name, Effect, Tool string }{
	{"airport-layout.geojson", "the map draws no airport layout (taxiways, aprons, terminals)", "tools/airport-layout/fetch.py"},
}

// Run checks what the configuration depends on.
func Run(in Inputs) Report {
	free := in.Free
	if free == nil {
		free = func(dir string) (int64, bool) {
			f, _, ok := disk.Space(dir)
			return f, ok
		}
	}
	c := &check{cfg: in.Config, free: free, r: Report{Missing: []Missing{}}}
	c.sources()
	c.pdftotext()
	c.sidecar()
	c.databases(in.DBRetentionGB, in.DBArchiveDir)
	c.archive()
	c.stationFiles()
	return c.r
}

type check struct {
	cfg  *config.Config
	free func(dir string) (int64, bool)
	r    Report
}

func (c *check) missing(m Missing) { c.r.Missing = append(c.r.Missing, m) }

// sources names each configured source the server cannot read: one ffmpeg
// read before it was removed (SRT, a sound card, a file), or raw PCM whose
// rate is not given. The audio needs nothing installed, so this is what can
// still leave a frequency without it; the source is refused at start, and
// its status says why, but a line here is what the launcher shows.
func (c *check) sources() {
	for _, src := range c.cfg.Frequencies.Sources {
		pcm := audio.PCMInput{Rate: src.PCMRate, Channels: max(src.PCMChannels, 1)}
		if err := audio.SourceProblem(src.URL, pcm); err != nil {
			c.missing(Missing{Kind: "source", What: "the source of " + src.ID, Path: src.URL,
				Effect: "no audio on " + src.ID + ": " + err.Error(), Level: LevelError,
				Hint: "an MP3 stream over http(s), or raw PCM over udp with its pcm_rate (configs/config.toml.example)"})
		}
	}
}

// pdftotext reads the charts of the official names list (internal/aip),
// which the settings page can make at any time.
func (c *check) pdftotext() {
	if p, ok := tool.Find("pdftotext"); ok {
		c.r.Found = append(c.r.Found, Found{"pdftotext", p})
		return
	}
	c.missing(Missing{Kind: "tool", What: "pdftotext", Path: lookedFor("pdftotext"),
		Effect: "the official names (AIP) are made without their charts: no published procedure route, only the procedures the AIP text names",
		Level:  LevelWarn, Hint: "install poppler (brew install poppler; apt install poppler-utils)"})
}

// sidecar checks what the server starts when [transcription.local] command
// is set: its program, its script, the models it names by a local path. A
// sidecar run elsewhere (no command) has nothing here to check; its /health
// is probed at start.
func (c *check) sidecar() {
	command := c.cfg.Transcription.Local.Command
	if len(command) == 0 {
		return
	}
	// The server refuses to run without the transcription it is asked for
	// (cmd/server, startSidecar): a sidecar that cannot start stops it.
	const stops = "the transcription sidecar cannot start, and the server stops: it does not run without the transcription it is asked for"
	if p, ok := tool.Find(command[0]); ok {
		c.r.Found = append(c.r.Found, Found{"sidecar program " + command[0], p})
	} else {
		c.missing(Missing{Kind: "sidecar", What: "the sidecar's program " + command[0], Path: lookedFor(command[0]),
			Effect: stops, Level: LevelError,
			Hint: "install it, or name it by its full path in [transcription.local] command"})
	}
	for _, arg := range command[1:] {
		if strings.HasSuffix(arg, ".py") && !strings.HasPrefix(arg, "-") {
			if _, err := os.Stat(arg); err != nil {
				c.missing(Missing{Kind: "sidecar", What: "the sidecar's script", Path: absolute(arg), Effect: stops, Level: LevelError,
					Hint: "the path is read from the directory the server runs in"})
			}
			break
		}
	}
	models := []struct{ option, effect, level string }{
		{"--model-en", "no transcription: the English model, the one every transmission is read with, is not there", LevelError},
		{"--model-fr", "French is refused, and the second reading of French is lost", LevelWarn},
		{"--fr-detector", "the language detector does not open the second reading", LevelWarn},
	}
	for _, m := range models {
		model := optionValue(command, m.option)
		if !localModel(model) {
			continue // a Hugging Face id, downloaded by the sidecar: not judged here
		}
		if _, err := os.Stat(expandHome(model)); err != nil {
			c.missing(Missing{Kind: "model", What: "the model of " + m.option, Path: model, Effect: m.effect, Level: m.level,
				Hint: "an external disk may be unplugged"})
		}
	}
}

// databases checks that the data directory can be written, and that its disk
// holds what the databases may still grow to before the retention deletes a
// day (db_retention_gb, which the settings panel can change).
func (c *check) databases(retentionGB float64, archiveDir string) {
	dir := c.cfg.Storage.SQLiteBasePath
	existing := existingAncestor(dir)
	if err := writable(existing); err != nil {
		c.missing(Missing{Kind: "directory", What: "the data directory", Path: absolute(dir),
			Effect: "the databases cannot be written, and the server stops", Level: LevelError,
			Hint: "make it writable, or name another in [storage] sqlite_base_path (" + err.Error() + ")"})
		return
	}
	if retentionGB <= 0 {
		retentionGB = config.DefaultDBRetentionGB
	}
	var used int64
	if entries, err := os.ReadDir(dir); err == nil {
		for _, e := range entries {
			// The daily databases and their journals: co-atc-YYYY-MM-DD.db, -wal, -shm.
			if !e.Type().IsRegular() || !strings.HasPrefix(e.Name(), "co-atc-") {
				continue
			}
			if info, err := e.Info(); err == nil {
				used += info.Size()
			}
		}
	}
	// The archive the retention copies a day to before deleting it (D85): not
	// found, nothing is deleted, and the databases grow past the retention.
	if a := archiveDir; a != "" {
		if info, err := os.Stat(a); err != nil || !info.IsDir() {
			c.missing(Missing{Kind: "directory", What: "the database archive", Path: absolute(a),
				Effect: "the days past the retention are kept, not deleted, until it is back: the databases grow past db_retention_gb",
				Level:  LevelWarn, Hint: "an external disk may be unplugged; or change the archive in Settings > Server, or empty it to delete them as before"})
		}
	}
	headroom := int64(retentionGB*(1<<30)) - used
	free, ok := c.free(existing)
	if !ok || headroom <= 0 || free >= headroom {
		return
	}
	c.missing(Missing{Kind: "disk", What: "free space for the databases", Path: absolute(dir),
		Effect: fmt.Sprintf("the disk fills before the retention deletes a day: the databases may grow %s more (db_retention_gb %g, %s used), %s free",
			gb(headroom), retentionGB, gb(used), gb(free)),
		Level: LevelWarn, Hint: "free space on that disk, or lower the retention in the settings"})
}

// archive checks the free space where the sidecar keeps its clips, against
// the floor below which it stops keeping them.
func (c *check) archive() {
	command := c.cfg.Transcription.Local.Command
	dir := audioarchive.DirOf(c.cfg.Transcription.Local.AudioArchiveDir, command)
	if strings.TrimSpace(dir) == "" {
		return
	}
	floor := sidecarMinFreeGB
	if v, err := strconv.ParseFloat(optionValue(command, "--save-audio-min-free-gb"), 64); err == nil {
		floor = v
	}
	// Where the sidecar measures it: the archive's directory, or its first
	// parent that exists -- the directory is made with the first clip.
	free, ok := c.free(existingAncestor(dir))
	if !ok || float64(free) >= floor*(1<<30) {
		return
	}
	c.missing(Missing{Kind: "disk", What: "free space for the audio archive", Path: absolute(dir),
		Effect: fmt.Sprintf("the sidecar keeps no clip below %g GB free (--save-audio-min-free-gb), %s free: transcription continues, the transmissions cannot be listened to again",
			floor, gb(free)),
		Level: LevelWarn, Hint: "free space on that disk, or lower audio_archive_max_gb"})
}

// stationFiles checks the page's files that belong to this station: in the
// data directory's station/, or in the page directory's data/ when the page
// is served from a directory.
func (c *check) stationFiles() {
	for _, f := range stationPageFiles {
		where := []string{filepath.Join(c.cfg.StationFilesDir(), f.Name)}
		if c.cfg.Server.StaticDir != "" {
			where = append(where, filepath.Join(c.cfg.Server.StaticDir, "data", f.Name))
		}
		found := false
		for _, p := range where {
			if info, err := os.Stat(p); err == nil && !info.IsDir() {
				found = true
				break
			}
		}
		if !found {
			c.missing(Missing{Kind: "station_file", What: f.Name, Path: absolute(where[0]), Effect: f.Effect,
				Level: LevelWarn, Hint: "python3 " + f.Tool + " --config " + c.cfg.ConfigPath})
		}
	}
}

// Log writes the report, one line per thing missing at the level it calls
// for, and what was found.
func (r Report) Log(log *logger.Logger) {
	// An error here is a fact about the machine, not about the code: no stack.
	quiet := log.WithOptions(zap.AddStacktrace(zapcore.PanicLevel))
	for _, f := range r.Found {
		log.Info(f.What+" found", logger.String("path", f.Path))
	}
	for _, m := range r.Missing {
		fields := []logger.Field{logger.String("kind", m.Kind)}
		if m.Path != "" {
			fields = append(fields, logger.String("path", m.Path))
		}
		if m.Hint != "" {
			fields = append(fields, logger.String("hint", m.Hint))
		}
		if m.Kind == "tool" || m.Kind == "sidecar" {
			fields = append(fields, logger.String("PATH", os.Getenv("PATH")), logger.String("also_looked_in", strings.Join(tool.Dirs, ":")))
		}
		msg := "Missing: " + m.What + " -- " + m.Effect
		if m.Level == LevelError {
			quiet.Error(msg, fields...)
		} else {
			quiet.Warn(msg, fields...)
		}
	}
	if len(r.Missing) == 0 {
		log.Info("Startup check: nothing missing")
	} else {
		log.Info(fmt.Sprintf("Startup check: %d missing, listed in /api/v1/health/detail", len(r.Missing)))
	}
}

// lookedFor is where a program was looked for: as written when it is a path,
// else on the PATH and in tool.Dirs.
func lookedFor(name string) string {
	if strings.ContainsRune(name, filepath.Separator) {
		return name
	}
	return name + " on the PATH and in " + strings.Join(tool.Dirs, ", ")
}

// optionValue is the value of a command-line option, "--opt value" or
// "--opt=value"; "" when absent.
func optionValue(command []string, option string) string {
	for i, arg := range command {
		if arg == option && i+1 < len(command) {
			return command[i+1]
		}
		if v, ok := strings.CutPrefix(arg, option+"="); ok {
			return v
		}
	}
	return ""
}

// localModel says whether a model is named by a local path, which can be
// checked here; anything else is a Hugging Face id the sidecar downloads,
// owner/name. A path is what the sidecar takes for one (model_status in
// sidecar/whisper_server.py: /, ./, ~), "../", more than one slash, or a
// relative path whose first directory is here -- "models/whisper-...-ct2",
// as the example names a faster-whisper model.
func localModel(model string) bool {
	for _, prefix := range []string{"/", "./", "../", "~"} {
		if strings.HasPrefix(model, prefix) {
			return true
		}
	}
	first, _, ok := strings.Cut(model, "/")
	if !ok {
		return false
	}
	if strings.Count(model, "/") > 1 {
		return true
	}
	info, err := os.Stat(first)
	return err == nil && info.IsDir()
}

func expandHome(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, p[1:])
		}
	}
	return p
}

// existingAncestor is dir if it exists, else its nearest parent that does.
func existingAncestor(dir string) string {
	p := absolute(dir)
	for {
		if _, err := os.Stat(p); err == nil {
			return p
		}
		parent := filepath.Dir(p)
		if parent == p {
			return p
		}
		p = parent
	}
}

// writable says whether a file can be made in dir, by making one and
// removing it: what the server does next is make its databases there.
func writable(dir string) error {
	info, err := os.Stat(dir)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is a file", dir)
	}
	f, err := os.CreateTemp(dir, ".clairance-write-check-*")
	if err != nil {
		return err
	}
	name := f.Name()
	_ = f.Close()
	return os.Remove(name)
}

func absolute(p string) string {
	if abs, err := filepath.Abs(p); err == nil {
		return abs
	}
	return p
}

func gb(bytes int64) string {
	return fmt.Sprintf("%.1f GB", float64(bytes)/(1<<30))
}
