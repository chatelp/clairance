package api

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/yegors/co-atc/internal/companion"
)

// What the launcher read in this process's files and in the process table,
// said by the server (docs-fr/36-contrat-lanceur.md): the contract's version,
// the process with absolute paths, what it listens on, what it costs, and the
// companions -- here a sidecar run by someone else, and a station link.
func TestTheHealthDetailDescribesTheProcess(t *testing.T) {
	s := newStack(t, stackOptions{sidecar: healthSaying(t, queuedHealth)})
	started := time.Now().Add(-time.Minute)
	s.handler.SetProcess(ProcessInfo{Instance: "production", StartedAt: started, Executable: os.Args[0],
		WorkingDir: ".", Config: s.cfg.ConfigPath})
	s.handler.SetListeners([]Listener{
		{Addr: "127.0.0.1:8000", URL: "http://127.0.0.1:8000", Primary: true, OK: true},
		{Addr: "127.0.0.1:8001", URL: "http://127.0.0.1:8001", Error: "listen tcp 127.0.0.1:8001: bind: address already in use"},
	})
	s.handler.SetReady(started.Add(2 * time.Second))
	var link *companion.Process
	if sleep, err := exec.LookPath("sleep"); err == nil && runtime.GOOS != "windows" {
		link, err = companion.Start(context.Background(), companion.Config{Role: "station_link", Path: sleep, Args: []string{"30"}}, s.log)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(link.Stop)
		s.handler.AttachCompanions(link, nil)
	}

	var d HealthDetail
	decode(t, s.do("GET", "/api/v1/health/detail", "", fromLoopback("127.0.0.1:50123", "127.0.0.1:8000")), &d)
	if d.Contract != 1 {
		t.Errorf("contract %d, want 1", d.Contract)
	}
	p := d.Process
	if p.Instance != "production" || p.StartedAt == nil || !p.StartedAt.Equal(started.UTC().Truncate(time.Nanosecond)) ||
		p.ReadyAt == nil || p.ReadyAt.Sub(*p.StartedAt) != 2*time.Second {
		t.Errorf("the process: instance %q, started %v, ready %v", p.Instance, p.StartedAt, p.ReadyAt)
	}
	for name, path := range map[string]string{"executable": p.Executable, "working_dir": p.WorkingDir, "config": p.Config,
		"static_dir": p.Paths.StaticDir, "databases_dir": p.Paths.DatabasesDir, "database": p.Paths.Database} {
		if !filepath.IsAbs(path) {
			t.Errorf("%s %q is not absolute", name, path)
		}
	}
	if p.Config != s.cfg.ConfigPath || filepath.Dir(p.Paths.Database) != p.Paths.DatabasesDir ||
		!strings.HasPrefix(filepath.Base(p.Paths.Database), "co-atc-") {
		t.Errorf("paths: config %q, %+v", p.Config, p.Paths)
	}
	if len(p.Listeners) != 2 || !p.Listeners[0].Primary || !p.Listeners[0].OK || p.Listeners[1].OK || p.Listeners[1].Error == "" {
		t.Errorf("listeners: %+v", p.Listeners)
	}
	if p.ADSBSource == nil || p.ADSBSource.Type != "tar1090" || p.ADSBSource.URL != s.tar1090.srv.URL+"/aircraft.json" {
		t.Errorf("ADS-B source: %+v", p.ADSBSource)
	}
	if runtime.GOOS == "darwin" {
		if p.FootprintBytes == nil || *p.FootprintBytes < 1<<20 {
			t.Errorf("footprint: %v", p.FootprintBytes)
		}
	} else if p.FootprintBytes != nil {
		t.Errorf("a footprint off macOS: %v", *p.FootprintBytes)
	}
	// The tests run inside the repository.
	if r := p.Repository; r == nil || r.Dir == "" {
		t.Errorf("repository: %+v", r)
	} else if _, err := exec.LookPath("git"); err == nil && r.Head == "" {
		t.Errorf("repository without its commit: %+v", r)
	}

	byRole := map[string]HealthCompanion{}
	for _, c := range d.Companions {
		byRole[c.Role] = c
	}
	if c, ok := byRole["sidecar"]; !ok || c.Managed || c.PID != 4242 || c.State != "running" || c.Kind != "fake" {
		t.Errorf("someone else's sidecar: %+v", c)
	}
	if link != nil {
		c := byRole["station_link"]
		if !c.Managed || c.PID == 0 || c.PID != link.State().PID || c.State != "running" || c.Mode != "continuous" ||
			c.StartedAt == nil || c.Restarts != 0 || len(c.Command) != 2 {
			t.Errorf("the station link: %+v", c)
		}
		if runtime.GOOS == "darwin" && (c.FootprintBytes == nil || *c.FootprintBytes == 0) {
			t.Errorf("the link's footprint: %v", c.FootprintBytes)
		}
	}
	if _, ok := byRole["keep_awake"]; ok {
		t.Error("a nil companion was listed")
	}
}

// Without a sidecar, or with transcription off, there is no sidecar
// companion; the list is a list all the same.
func TestNoSidecarNoSidecarCompanion(t *testing.T) {
	for _, o := range []stackOptions{{}, {idle: true}} {
		s := newStack(t, o)
		var d HealthDetail
		decode(t, s.do("GET", "/api/v1/health/detail", "", fromLoopback("127.0.0.1:50123", "127.0.0.1:8000")), &d)
		if d.Companions == nil || len(d.Companions) != 0 {
			t.Errorf("%+v: companions %+v", o, d.Companions)
		}
	}
}
