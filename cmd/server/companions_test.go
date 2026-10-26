//go:build !windows

package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/yegors/co-atc/internal/auth"
	"github.com/yegors/co-atc/internal/config"
	"github.com/yegors/co-atc/pkg/logger"
)

// buildStationLink compiles cmd/radio-ctl-sync, as bin/ would hold it.
func buildStationLink(t *testing.T) string {
	t.Helper()
	gobin := filepath.Join(runtime.GOROOT(), "bin", "go")
	if _, err := os.Stat(gobin); err != nil {
		t.Skip("no Go toolchain to build radio-ctl-sync with")
	}
	out := filepath.Join(t.TempDir(), "radio-ctl-sync")
	cmd := exec.Command(gobin, "build", "-o", out, "./cmd/radio-ctl-sync")
	cmd.Dir = filepath.Join("..", "..")
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("building radio-ctl-sync: %v\n%s", err, b)
	}
	return out
}

// fakeRadioCtl is a station on the air with two channels, neither to be
// transcribed, whose streams are at streams. It counts the readings.
func fakeRadioCtl(t *testing.T, streams string) (*httptest.Server, *atomic.Int64) {
	t.Helper()
	var reads atomic.Int64
	state := fmt.Sprintf(`{"airband":"running","version":7,"depuis":"2026-10-26T18:00:00+01:00","mode":"chaine",
 "libelle":"A l antenne","flux_separes":{"actifs":true,"manquants":[]},
 "selection":{"nom":"libre","titre":"Selection libre - 124,350 + 123,875","frequences":[
  {"designation":"124.350","mhz":124.35,"id":"124350","service":"CDG approche","langue":"en","flux":"/aero-124350.mp3",
   "flux_url":"%[1]s/aero-124350.mp3","flux_present":true},
  {"designation":"123.875","mhz":123.875,"id":"123875","service":"Orly approche","langue":"mixte","flux":"/aero-123875.mp3",
   "flux_url":"%[1]s/aero-123875.mp3","flux_present":true}]}}`, streams)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/radio/etat":
			reads.Add(1)
			io.WriteString(w, state)
		case "/radio/frequences":
			io.WriteString(w, `{"frequences":[{"id":"124350","transcrire":false},{"id":"123875","transcrire":false}]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &reads
}

// linkConfig is the example with sign-in on, an account, and the station
// link on, at the fake station.
func linkConfig(t *testing.T, program, station string) (string, int) {
	t.Helper()
	dir := t.TempDir()
	path, port := exampleConfig(t, dir, false)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	hash, err := auth.HashPassword("correcthorsebattery")
	if err != nil {
		t.Fatal(err)
	}
	cfg := regexp.MustCompile(`(?m)^enabled = false .*$`).ReplaceAllLiteralString(string(raw), "enabled = true")
	// The example has the section, every key commented out.
	header := regexp.MustCompile(`(?m)^\[station_link\]$`)
	if n := len(header.FindAllStringIndex(cfg, -1)); n != 1 {
		t.Fatalf("the example configuration changed: [station_link] found %d times, want 1", n)
	}
	cfg = header.ReplaceAllLiteralString(cfg, fmt.Sprintf(`[station_link]
enabled = true
program = %q
station_url = %q
streams_url = "http://audio.lan"
interval_seconds = 1`, program, station))
	cfg += fmt.Sprintf(`
[[auth.users]]
name = "pierre"
password_hash = %q
`, hash)
	if err := os.WriteFile(path, []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	return path, port
}

// get reads a route from this machine, with no session.
func get(url string) (int, string) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err.Error()
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

var linkStarted = regexp.MustCompile(`station_link\s+Started\s+\{"pid": (\d+)`)

// linkPIDs are the station links the server has said it started, in order.
func linkPIDs(c *child) []int {
	var out []int
	for _, m := range linkStarted.FindAllStringSubmatch(c.out.String(), -1) {
		pid, _ := strconv.Atoi(m[1])
		out = append(out, pid)
	}
	return out
}

// gone says a process has ended: no longer there, or a zombie nobody has
// reaped yet.
func gone(pid int) bool {
	if syscall.Kill(pid, 0) != nil {
		return true
	}
	stat, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return false
	}
	// pid (comm) state ...: the state is the first field after the comm.
	s := string(stat)
	i := strings.LastIndexByte(s, ')')
	return i > 0 && len(s) > i+2 && s[i+2] == 'Z'
}

func waitUntil(t *testing.T, c *child, what string, within time.Duration, ok func() bool) time.Duration {
	t.Helper()
	began := time.Now()
	for !ok() {
		select {
		case <-c.exited:
			t.Fatalf("the server stopped while waiting for %s (%v):\n%s", what, c.err, c.out.String())
		default:
		}
		if time.Since(began) > within {
			t.Fatalf("no %s within %v:\n%s", what, within, c.out.String())
		}
		time.Sleep(50 * time.Millisecond)
	}
	return time.Since(began)
}

// The real server with [station_link] on, against a fake radio-ctl: the link
// starts with it, sets the sources through the key alone -- sign-in is on and
// it has no password --, is started again when it is killed, and stops with
// the server. Killed outright, the server leaves no link behind.
func TestTheStationLinkLivesAndStopsWithTheServer(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed: the frequencies service needs it")
	}
	program := buildStationLink(t)
	gone404 := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(gone404.Close)
	station, reads := fakeRadioCtl(t, gone404.URL)

	for _, end := range []string{"SIGTERM", "SIGKILL"} {
		t.Run(end, func(t *testing.T) {
			cfgPath, port := linkConfig(t, program, station.URL)
			c := runServer(t, cfgPath)
			t.Cleanup(func() {
				for _, pid := range linkPIDs(c) {
					_ = syscall.Kill(pid, syscall.SIGKILL)
				}
			})
			base := fmt.Sprintf("http://127.0.0.1:%d", port)

			// The sources the station has on the air, set through the key, as
			// the link says in the server's log.
			took := waitUntil(t, c, "the station's two sources", 30*time.Second, func() bool {
				out := c.out.String()
				return strings.Contains(out, "set 124350: ") && strings.Contains(out, "set 123875: ")
			})
			t.Logf("sources set %v after the server was started", took.Round(time.Millisecond))
			// And as the server has them, once its 5 s answer has expired.
			waitUntil(t, c, "the two sources in the health", 10*time.Second, func() bool {
				code, body := get(base + "/api/v1/health/detail")
				return code == http.StatusOK && strings.Contains(body, `"id":"124350"`) && strings.Contains(body, `"id":"123875"`)
			})
			if code, _ := get(base + "/api/v1/frequencies"); code != http.StatusUnauthorized {
				t.Errorf("sign-in is on, yet /frequencies answers %d without a session", code)
			}
			if _, body := get(base + "/api/v1/health/detail"); !strings.Contains(body, `"state":"on_air"`) {
				t.Errorf("the receiver's state was not reported: %s", body)
			}
			if reads.Load() == 0 {
				t.Error("the station was never read")
			}
			first := linkPIDs(c)
			if len(first) != 1 {
				t.Fatalf("%d links started, want 1:\n%s", len(first), c.out.String())
			}

			if end == "SIGTERM" {
				// Killed, it is started again, a second later.
				killed := time.Now()
				_ = syscall.Kill(first[0], syscall.SIGKILL)
				waitUntil(t, c, "a second link", 10*time.Second, func() bool { return len(linkPIDs(c)) == 2 })
				t.Logf("link restarted %v after being killed", time.Since(killed).Round(time.Millisecond))
				second := linkPIDs(c)[1]
				if second == first[0] {
					t.Fatal("restarted with the same pid")
				}
				// It reads the station again: one reading a second.
				before := reads.Load()
				waitUntil(t, c, "a reading by the second link", 10*time.Second, func() bool { return reads.Load() > before+1 })

				// Stopped with the server, which exits cleanly.
				stopped := time.Now()
				_ = c.cmd.Process.Signal(syscall.SIGTERM)
				select {
				case <-c.exited:
					if c.err != nil {
						t.Errorf("stopping on SIGTERM: %v\n%s", c.err, c.out.String())
					}
				case <-time.After(30 * time.Second):
					t.Fatalf("the server did not stop on SIGTERM:\n%s", c.out.String())
				}
				t.Logf("server and link stopped %v after SIGTERM", time.Since(stopped).Round(time.Millisecond))
				if !gone(second) {
					t.Errorf("the link (pid %d) outlived the server", second)
				}
				return
			}

			// Killed outright, the server cannot stop it: the link sees its
			// parent gone and stops itself.
			_ = syscall.Kill(-c.cmd.Process.Pid, syscall.SIGKILL)
			<-c.exited
			killed := time.Now()
			deadline := killed.Add(5 * time.Second)
			for !gone(first[0]) {
				if time.Now().After(deadline) {
					t.Fatalf("the link (pid %d) outlived a killed server by 5 s", first[0])
				}
				time.Sleep(50 * time.Millisecond)
			}
			t.Logf("link gone %v after the server was killed", time.Since(killed).Round(time.Millisecond))
		})
	}
}

// On macOS the Mac is kept awake by caffeinate, which follows this process;
// elsewhere nothing is started.
func TestKeepAwakeIsCaffeinateFollowingThisProcess(t *testing.T) {
	log, err := logger.New(logger.Config{Level: "error", Format: "console"})
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Defaults()
	if p := startKeepAwake(context.Background(), &cfg, log); p != nil {
		t.Fatal("started without being asked")
	}
	cfg.Server.KeepAwake = true
	defer func(goos, prog string) { keepAwakeOS, caffeinate = goos, prog }(keepAwakeOS, caffeinate)

	keepAwakeOS = "linux"
	if p := startKeepAwake(context.Background(), &cfg, log); p != nil {
		t.Fatal("started off macOS")
	}

	// A stand-in that lives as caffeinate would: sleep, given the same arguments
	// it ignores.
	stand, err := exec.LookPath("sleep")
	if err != nil {
		t.Skip("no sleep to stand in for caffeinate")
	}
	keepAwakeOS, caffeinate = "darwin", stand
	p := startKeepAwake(context.Background(), &cfg, log)
	if p == nil {
		t.Fatal("not started on macOS")
	}
	defer p.Stop()
	st := p.State()
	want := []string{stand, "-i", "-w", strconv.Itoa(os.Getpid())}
	if st.Role != "keep_awake" || strings.Join(st.Command, " ") != strings.Join(want, " ") {
		t.Errorf("state: %+v, want the command %q", st, want)
	}
}
