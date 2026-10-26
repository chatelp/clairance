package api

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/yegors/co-atc/internal/companion"
	"github.com/yegors/co-atc/pkg/logger"
	"github.com/yegors/co-atc/pkg/tool"
)

// The process, as the launcher's contract describes it
// (docs-fr/36-contrat-lanceur.md): what the launcher read until 26/10 in the
// configuration, the log, git and the process table, said by the server
// itself, with absolute paths. Added to /api/v1/health/detail, whose fields
// before it are unchanged.

// contractVersion is the contract's version, said in the health detail. An
// older server says none.
const contractVersion = 1

// HealthProcess is the process: where it runs from, what it listens on, where
// its files are, what it costs.
type HealthProcess struct {
	Instance       string            `json:"instance,omitempty"`
	StartedAt      *time.Time        `json:"started_at,omitempty"`
	ReadyAt        *time.Time        `json:"ready_at,omitempty"`
	Executable     string            `json:"executable,omitempty"`
	WorkingDir     string            `json:"working_dir,omitempty"`
	Config         string            `json:"config,omitempty"`
	Listeners      []Listener        `json:"listeners"`
	Paths          HealthPaths       `json:"paths"`
	SidecarURL     string            `json:"sidecar_url,omitempty"`
	ADSBSource     *HealthADSBSource `json:"adsb_source,omitempty"`
	FootprintBytes *int64            `json:"footprint_bytes,omitempty"` // macOS only, see memory_darwin.go
	MemoryPressure string            `json:"memory_pressure,omitempty"`
	Repository     *HealthRepository `json:"repository,omitempty"`
}

// Listener is one port the server listens on: as configured, as this machine
// reaches it, and whether it opened.
type Listener struct {
	Addr    string `json:"addr"`
	URL     string `json:"url"`
	Primary bool   `json:"primary"`
	OK      bool   `json:"ok"`
	Error   string `json:"error,omitempty"`
}

// HealthPaths are the files the server keeps, absolute. Log is the file of
// the day being written, rotation included; absent when the log goes to
// stdout alone.
type HealthPaths struct {
	StaticDir    string `json:"static_dir,omitempty"`
	DatabasesDir string `json:"databases_dir,omitempty"`
	Database     string `json:"database,omitempty"`
	Log          string `json:"log,omitempty"`
	AudioArchive string `json:"audio_archive,omitempty"`
}

// HealthADSBSource is where the aircraft come from. The URL is given for the
// receivers on the network, which the launcher may probe to tell a station
// down from a server cut off; never for an external service, whose address
// may carry a key.
type HealthADSBSource struct {
	Type string `json:"type"`
	URL  string `json:"url,omitempty"`
}

// HealthRepository is the git checkout the server runs from, when it runs
// from one: its branch and commit, and how far behind it the binary is.
type HealthRepository struct {
	Dir           string `json:"dir"`
	Branch        string `json:"branch,omitempty"`
	Head          string `json:"head,omitempty"`
	CommitsBehind *int   `json:"commits_behind,omitempty"`
}

// HealthCompanion is a program running beside the server: the sidecar, the
// station link, the keep-awake assertion. Managed is whether the server
// started it and watches over it; a sidecar run by someone else is listed,
// unmanaged, when it answers.
type HealthCompanion struct {
	Role           string     `json:"role"`
	Managed        bool       `json:"managed"`
	PID            int        `json:"pid,omitempty"`
	State          string     `json:"state"`
	StartedAt      *time.Time `json:"started_at,omitempty"`
	Restarts       int        `json:"restarts"`
	LastExit       string     `json:"last_exit,omitempty"`
	LastOutput     string     `json:"last_output,omitempty"`
	NextStartAt    *time.Time `json:"next_start_at,omitempty"`
	Command        []string   `json:"command,omitempty"`
	Kind           string     `json:"kind,omitempty"`            // the sidecar: "model", or "fake" for a test instance's stand-in
	Mode           string     `json:"mode,omitempty"`            // the station link: "continuous"
	FootprintBytes *int64     `json:"footprint_bytes,omitempty"` // macOS only
}

// ProcessInfo is what only main knows of the process.
type ProcessInfo struct {
	Instance   string
	StartedAt  time.Time
	Executable string
	WorkingDir string
	Config     string
}

// processState is what main tells the handler, while it serves.
type processState struct {
	mu         sync.Mutex
	info       ProcessInfo
	listeners  []Listener
	readyAt    time.Time
	companions []*companion.Process

	repoAt time.Time
	repo   *HealthRepository
}

// SetProcess gives the handler what only main knows of the process.
func (h *Handler) SetProcess(info ProcessInfo) {
	h.process.mu.Lock()
	h.process.info = info
	h.process.mu.Unlock()
}

// SetListeners says what the server listens on, once it does.
func (h *Handler) SetListeners(ls []Listener) {
	h.process.mu.Lock()
	h.process.listeners = append([]Listener(nil), ls...)
	h.process.mu.Unlock()
}

// SetReady records when the server was ready: every service started, the
// ports open.
func (h *Handler) SetReady(at time.Time) {
	h.process.mu.Lock()
	h.process.readyAt = at
	h.process.mu.Unlock()
}

// AttachCompanions gives the handler the companions main started (nil ones
// are skipped), for the health.
func (h *Handler) AttachCompanions(ps ...*companion.Process) {
	h.process.mu.Lock()
	defer h.process.mu.Unlock()
	for _, p := range ps {
		if p != nil {
			h.process.companions = append(h.process.companions, p)
		}
	}
}

// describeProcess fills the process and the companions of the health detail.
func (h *Handler) describeProcess(d *HealthDetail) {
	h.process.mu.Lock()
	info, readyAt := h.process.info, h.process.readyAt
	listeners := append([]Listener{}, h.process.listeners...)
	companions := append([]*companion.Process(nil), h.process.companions...)
	h.process.mu.Unlock()

	p := HealthProcess{Instance: info.Instance, StartedAt: utcOrNil(info.StartedAt), ReadyAt: utcOrNil(readyAt),
		Executable: absPath(info.Executable), WorkingDir: absPath(info.WorkingDir), Config: absPath(info.Config),
		Listeners: listeners}
	if h.config != nil {
		static := h.config.Server.StaticDir
		if static == "" {
			static = "www"
		}
		p.Paths.StaticDir = absPath(static)
		p.Paths.DatabasesDir = absPath(h.config.Storage.SQLiteBasePath)
		p.SidecarURL = h.config.Transcription.Local.ServerURL
		if t := h.config.ADSB.SourceType; t != "" {
			p.ADSBSource = &HealthADSBSource{Type: t}
			switch t {
			case "tar1090":
				if base := strings.TrimRight(h.config.ADSB.Tar1090BaseURL, "/"); base != "" {
					p.ADSBSource.URL = base + "/aircraft.json"
				}
			case "readsb-api":
				p.ADSBSource.URL = h.config.ADSB.ReadsbAPIURL
			}
		}
	}
	if h.db != nil {
		p.Paths.Database = absPath(h.db.Path())
	}
	p.Paths.Log = absPath(logger.File())
	if h.audioArchive != nil {
		p.Paths.AudioArchive = absPath(h.audioArchive.Root())
	}
	p.Repository = h.repository(d.Commit)

	d.Companions = []HealthCompanion{}
	if c, ok := h.sidecarCompanion(d.Sidecar); ok {
		d.Companions = append(d.Companions, c)
	}
	for _, cp := range companions {
		st := cp.State()
		c := HealthCompanion{Role: st.Role, Managed: true, PID: st.PID, State: st.State, StartedAt: utcOrNil(st.StartedAt),
			Restarts: st.Restarts, LastExit: st.LastExit, LastOutput: st.LastOutput, NextStartAt: utcOrNil(st.NextStartAt),
			Command: st.Command}
		if st.Role == "station_link" {
			c.Mode = "continuous"
		}
		d.Companions = append(d.Companions, c)
	}

	// What each costs, read once for all.
	pids := []int{os.Getpid()}
	for _, c := range d.Companions {
		if c.PID > 0 {
			pids = append(pids, c.PID)
		}
	}
	mem := processFootprint(pids)
	if b, ok := mem[os.Getpid()]; ok {
		p.FootprintBytes = &b
	}
	for i := range d.Companions {
		if b, ok := mem[d.Companions[i].PID]; ok && d.Companions[i].PID > 0 {
			d.Companions[i].FootprintBytes = &b
		}
	}
	p.MemoryPressure = memoryPressure()
	d.Process = p
}

// sidecarCompanion is the sidecar as a companion: the one this server started,
// or one run by someone else that answers. Its pid is the one it reports --
// the Python process, whose memory is the one worth reading -- when it
// answers, else the one started.
func (h *Handler) sidecarCompanion(s *HealthSidecar) (HealthCompanion, bool) {
	if h.sttSidecar == nil || s == nil || s.Status == "off" {
		return HealthCompanion{}, false
	}
	kind := ""
	if b := strings.ToLower(strings.TrimSpace(h.sttSidecar.LastHealth().Backend)); b == "fake" {
		kind = "fake"
	} else if s.Reachable {
		kind = "model"
	}
	child, managed := h.sttSidecar.Child()
	if !managed {
		if !s.Reachable || s.PID == 0 {
			return HealthCompanion{}, false
		}
		return HealthCompanion{Role: "sidecar", PID: s.PID, State: companion.StateRunning, Kind: kind}, true
	}
	c := HealthCompanion{Role: "sidecar", Managed: true, PID: child.PID, State: child.State,
		StartedAt: utcOrNil(child.StartedAt), Restarts: child.Restarts, LastExit: child.LastExit,
		Command: child.Command, Kind: kind}
	if child.PID != 0 && s.Reachable && s.PID != 0 {
		c.PID = s.PID
	}
	return c, true
}

// absPath is p made absolute against the working directory; "" stays "".
func absPath(p string) string {
	if p == "" {
		return ""
	}
	if abs, err := filepath.Abs(p); err == nil {
		return abs
	}
	return p
}

// repositoryTTL is how long one reading of the checkout serves: three git
// commands, read at most once a minute.
const repositoryTTL = time.Minute

// repository says, when the server runs from a git checkout, which commit that
// checkout is at and how far behind it the binary is -- what the launcher
// worked out with git itself until now. Only where a .git is found, so that a
// Mac without the developer tools is never asked to install them.
func (h *Handler) repository(binary string) *HealthRepository {
	h.process.mu.Lock()
	defer h.process.mu.Unlock()
	if !h.process.repoAt.IsZero() && time.Since(h.process.repoAt) < repositoryTTL {
		return h.process.repo
	}
	h.process.repoAt = time.Now()
	h.process.repo = readRepository(binary)
	return h.process.repo
}

func readRepository(binary string) *HealthRepository {
	wd, err := os.Getwd()
	if err != nil {
		return nil
	}
	top := ""
	for dir := wd; ; dir = filepath.Dir(dir) {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			top = dir
			break
		}
		if filepath.Dir(dir) == dir {
			return nil
		}
	}
	r := &HealthRepository{Dir: top}
	git, ok := tool.Find("git")
	if !ok {
		return r
	}
	run := func(args ...string) (string, bool) {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		out, err := exec.CommandContext(ctx, git, append([]string{"-C", top}, args...)...).Output()
		return strings.TrimSpace(string(out)), err == nil
	}
	r.Branch, _ = run("rev-parse", "--abbrev-ref", "HEAD")
	r.Head, _ = run("rev-parse", "--short", "HEAD")
	commit := strings.TrimSuffix(binary, "-dirty")
	if commit != "" && commit != "unknown" {
		if n, ok := run("rev-list", "--count", commit+"..HEAD"); ok {
			if v, err := strconv.Atoi(n); err == nil {
				r.CommitsBehind = &v
			}
		}
	}
	return r
}
