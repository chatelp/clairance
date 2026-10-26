//go:build !windows

package main

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"syscall"
	"testing"
	"time"
)

// event is one line of the status channel.
type event struct {
	Status    string `json:"coatc_status"`
	At        string `json:"at"`
	PID       int    `json:"pid"`
	Step      string `json:"step"`
	URL       string `json:"url"`
	Cause     string `json:"cause"`
	Error     string `json:"error"`
	Listeners []struct {
		Addr    string `json:"addr"`
		URL     string `json:"url"`
		Primary bool   `json:"primary"`
		OK      bool   `json:"ok"`
	} `json:"listeners"`
}

// events are the status lines the child wrote, among its other output, as a
// launcher reading its stderr would pick them out.
func events(t *testing.T, c *child) []event {
	t.Helper()
	var out []event
	for _, line := range strings.Split(c.out.String(), "\n") {
		if !strings.HasPrefix(line, `{"coatc_status":`) {
			continue
		}
		var e event
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			t.Fatalf("a status line that is not JSON: %q: %v", line, err)
		}
		if _, err := time.Parse(time.RFC3339Nano, e.At); err != nil {
			t.Errorf("%s: at %q: %v", e.Status, e.At, err)
		}
		out = append(out, e)
	}
	return out
}

func names(es []event) string {
	var n []string
	for _, e := range es {
		s := e.Status
		if e.Step != "" {
			s += ":" + e.Step
		}
		if e.Cause != "" {
			s += ":" + e.Cause
		}
		n = append(n, s)
	}
	return strings.Join(n, " ")
}

// A start told on the status channel, from the first line to ready, and the
// stop that follows a SIGTERM.
func TestTheStartAndTheStopAreToldOnTheStatusChannel(t *testing.T) {
	if _, err := os.Stat("/usr/bin/ffmpeg"); err != nil {
		if _, err := os.Stat("/opt/homebrew/bin/ffmpeg"); err != nil {
			t.Skip("ffmpeg not installed: the frequencies service needs it")
		}
	}
	cfgPath, port := exampleConfig(t, t.TempDir(), false)
	c := runServer(t, cfgPath, "-status-fd", "2")
	ready := waitUntil(t, c, "the ready event", 60*time.Second, func() bool {
		return strings.Contains(c.out.String(), `{"coatc_status":"ready"`)
	})
	t.Logf("ready %v after the start", ready.Round(time.Millisecond))

	es := events(t, c)
	want := "starting step:adsb_source step:storage step:services step:listen ready"
	if got := names(es); got != want {
		t.Fatalf("events: %s\nwant:   %s", got, want)
	}
	if es[0].PID != c.cmd.Process.Pid || es[5].PID != c.cmd.Process.Pid {
		t.Errorf("pid: %d and %d, want %d", es[0].PID, es[5].PID, c.cmd.Process.Pid)
	}
	url := fmt.Sprintf("http://127.0.0.1:%d", port)
	if r := es[5]; r.URL != url || len(r.Listeners) != 2 || !r.Listeners[0].Primary || !r.Listeners[0].OK || r.Listeners[0].URL != url {
		t.Errorf("ready: %+v", r)
	}
	// What the ready line promises: the server answers there.
	if code, _ := get(url + "/api/v1/health"); code != http.StatusOK {
		t.Errorf("ready, yet %s/api/v1/health answers %d", url, code)
	}

	_ = c.cmd.Process.Signal(syscall.SIGTERM)
	select {
	case <-c.exited:
	case <-time.After(30 * time.Second):
		t.Fatalf("the server did not stop:\n%s", c.out.String())
	}
	if c.err != nil {
		t.Errorf("exit: %v", c.err)
	}
	if got := names(events(t, c)); got != want+" stopping stopped" {
		t.Errorf("events: %s", got)
	}
}

// A start that fails says why, as a cause, then exits with status 1 -- the
// ADS-B source that does not answer, the primary port another server holds,
// the station link's program missing.
func TestAFailedStartSaysItsCause(t *testing.T) {
	cases := []struct {
		cause, error string
		edit         func(t *testing.T, cfg string, port int) string
	}{
		{"adsb_source", "connection refused", func(t *testing.T, cfg string, port int) string {
			return regexp.MustCompile(`(?m)^tar1090_base_url = .*$`).ReplaceAllLiteralString(cfg,
				fmt.Sprintf("tar1090_base_url = %q", fmt.Sprintf("http://127.0.0.1:%d/", freePort(t))))
		}},
		{"listen", "address already in use", func(t *testing.T, cfg string, port int) string {
			ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { ln.Close() })
			go http.Serve(ln, http.NotFoundHandler()) // someone else's server on the port
			return cfg
		}},
		{"station_link", "not found", func(t *testing.T, cfg string, port int) string {
			return strings.Replace(cfg, "[station_link]\n", "[station_link]\nenabled = true\nprogram = \"/nonexistent/radio-ctl-sync\"\n"+
				"station_url = \"http://station.lan\"\nstreams_url = \"http://audio.lan\"\n", 1)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.cause, func(t *testing.T) {
			cfgPath, port := exampleConfig(t, t.TempDir(), false)
			raw, err := os.ReadFile(cfgPath)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(cfgPath, []byte(tc.edit(t, string(raw), port)), 0o644); err != nil {
				t.Fatal(err)
			}
			c := runServer(t, cfgPath, "-status-fd", "2")
			select {
			case <-c.exited:
			case <-time.After(60 * time.Second):
				t.Fatalf("still running:\n%s", c.out.String())
			}
			if ee, ok := c.err.(*exec.ExitError); !ok || ee.ExitCode() != 1 {
				t.Errorf("exit: %v, want status 1", c.err)
			}
			es := events(t, c)
			if len(es) == 0 {
				t.Fatalf("no event:\n%s", c.out.String())
			}
			last := es[len(es)-1]
			if last.Status != "fatal" || last.Cause != tc.cause || !strings.Contains(last.Error, tc.error) {
				t.Errorf("last event: %+v, want fatal %s with %q\n%s", last, tc.cause, tc.error, names(es))
			}
			for _, e := range es {
				if e.Status == "ready" {
					t.Errorf("said ready: %s", names(es))
				}
			}
		})
	}
}
