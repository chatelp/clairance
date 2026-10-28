//go:build !windows

// Not built on Windows: these tests signal process groups, which it has not.

package companion

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/yegors/co-atc/pkg/logger"
)

// The companion in these tests is this test binary, asked through the
// environment to behave as a program would: run until stopped, die at once,
// ignore SIGTERM, or fork a helper of its own.
const modeEnv = "COMPANION_TEST_MODE"

func TestMain(m *testing.M) {
	switch os.Getenv(modeEnv) {
	case "":
		os.Exit(m.Run())
	case "run":
		fmt.Printf("started %d\n", os.Getpid())
		time.Sleep(time.Hour)
	case "die":
		fmt.Printf("started %d\n", time.Now().UnixNano())
		fmt.Fprintln(os.Stderr, "station: no route to host")
		os.Exit(3)
	case "ignore-term":
		signal.Ignore(syscall.SIGTERM)
		fmt.Printf("started %d\n", os.Getpid())
		time.Sleep(time.Hour)
	case "fork":
		helper := exec.Command(os.Args[0])
		helper.Env = append(os.Environ(), modeEnv+"=run")
		if err := helper.Start(); err != nil {
			os.Exit(4)
		}
		fmt.Printf("helper %d\n", helper.Process.Pid)
		time.Sleep(time.Hour)
	}
	os.Exit(0)
}

func quiet(t *testing.T) *logger.Logger {
	t.Helper()
	log, err := logger.New(logger.Config{Level: "error", Format: "console"})
	if err != nil {
		t.Fatal(err)
	}
	return log
}

// lines collects what the companion writes.
type lines struct {
	mu  sync.Mutex
	all []string
}

func (l *lines) add(s string) {
	l.mu.Lock()
	l.all = append(l.all, s)
	l.mu.Unlock()
}

func (l *lines) with(prefix string) []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	var out []string
	for _, s := range l.all {
		if strings.HasPrefix(s, prefix) {
			out = append(out, strings.TrimPrefix(s, prefix))
		}
	}
	return out
}

func start(t *testing.T, mode string, cfg Config) (*Process, *lines) {
	t.Helper()
	out := &lines{}
	cfg.Role = "station_link"
	cfg.Path = os.Args[0]
	cfg.Env = append(cfg.Env, modeEnv+"="+mode)
	cfg.Output = out.add
	p, err := Start(context.Background(), cfg, quiet(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(p.Stop)
	return p, out
}

func waitFor(t *testing.T, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatalf("still waiting for %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func alive(pid int) bool {
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return p.Signal(syscall.Signal(0)) == nil
}

// It runs, says so in its state, and Stop takes it and what it forked: a
// helper left behind would hold whatever it holds, as the sidecar's port was
// on the night seventeen servers shared one.
func TestACompanionStopsWithWhatItForked(t *testing.T) {
	p, out := start(t, "fork", Config{})
	waitFor(t, "the helper", func() bool { return len(out.with("helper ")) == 1 })
	helper, _ := strconv.Atoi(out.with("helper ")[0])

	st := p.State()
	if st.State != StateRunning || st.PID == 0 || st.StartedAt.IsZero() || st.Role != "station_link" ||
		len(st.Command) != 1 || st.Command[0] != os.Args[0] {
		t.Fatalf("running: %+v", st)
	}
	pid := st.PID
	p.Stop()
	if st := p.State(); st.State != StateStopped || st.PID != 0 {
		t.Errorf("after Stop: %+v", st)
	}
	waitFor(t, "the companion and its helper to be gone", func() bool { return !alive(pid) && !alive(helper) })
}

// One that dies is started again, later each time it dies young, never later
// than the ceiling; its state says how it ended and what it said last.
func TestADyingCompanionIsRestartedLaterEachTime(t *testing.T) {
	p, out := start(t, "die", Config{FirstDelay: 40 * time.Millisecond, MaxDelay: 320 * time.Millisecond, StableAfter: time.Minute})
	waitFor(t, "six starts", func() bool { return len(out.with("started ")) >= 6 })

	var starts []time.Time
	for _, s := range out.with("started ")[:6] {
		ns, _ := strconv.ParseInt(s, 10, 64)
		starts = append(starts, time.Unix(0, ns))
	}
	// 40, 80, 160, 320, 320 ms, give or take the start of a process.
	want := []time.Duration{40, 80, 160, 320, 320}
	for i, w := range want {
		gap := starts[i+1].Sub(starts[i])
		if gap < w*time.Millisecond || gap > w*time.Millisecond+400*time.Millisecond {
			t.Errorf("restart %d after %v, want about %v", i+1, gap.Round(time.Millisecond), w*time.Millisecond)
		}
	}
	st := p.State()
	if st.Restarts < 5 || st.LastExit != "exit status 3" || st.LastOutput != "station: no route to host" {
		t.Errorf("state: %+v", st)
	}
	t.Logf("gaps: %v", func() (g []string) {
		for i := 1; i < len(starts); i++ {
			g = append(g, starts[i].Sub(starts[i-1]).Round(time.Millisecond).String())
		}
		return
	}())
}

// A run that lasted brings the delay back to the first one.
func TestAStableRunResetsTheDelay(t *testing.T) {
	p, out := start(t, "run", Config{FirstDelay: 30 * time.Millisecond, MaxDelay: 10 * time.Second, StableAfter: 50 * time.Millisecond})
	for i := 0; i < 4; i++ {
		waitFor(t, "a start", func() bool { return len(out.with("started ")) == i+1 })
		pid, _ := strconv.Atoi(out.with("started ")[i])
		time.Sleep(80 * time.Millisecond) // longer than StableAfter
		killed := time.Now()
		_ = syscall.Kill(pid, syscall.SIGKILL)
		waitFor(t, "the restart", func() bool { return len(out.with("started ")) == i+2 })
		if d := time.Since(killed); d > 600*time.Millisecond {
			t.Errorf("restart %d took %v after a stable run, want the first delay", i+1, d)
		}
	}
	if st := p.State(); st.Restarts != 4 || st.LastExit != "signal: killed" {
		t.Errorf("state: %+v", st)
	}
}

// Stop does not wait for SIGTERM for ever.
func TestACompanionThatIgnoresSIGTERMIsKilled(t *testing.T) {
	p, out := start(t, "ignore-term", Config{Grace: 200 * time.Millisecond})
	waitFor(t, "the start", func() bool { return len(out.with("started ")) == 1 })
	pid := p.State().PID
	began := time.Now()
	p.Stop()
	if d := time.Since(began); d < 200*time.Millisecond || d > 3*time.Second {
		t.Errorf("Stop took %v, want the grace period and a little more", d)
	}
	if alive(pid) {
		t.Error("still alive after Stop")
	}
	if st := p.State(); st.LastExit != "signal: killed" {
		t.Errorf("state: %+v", st)
	}
}

// Stopped while it waits to restart: nothing is started again.
func TestStopDuringTheDelayStartsNothing(t *testing.T) {
	p, out := start(t, "die", Config{FirstDelay: time.Hour})
	waitFor(t, "the restart to be pending", func() bool { return p.State().State == StateRestarting })
	if st := p.State(); st.NextStartAt.IsZero() || st.PID != 0 {
		t.Errorf("restarting: %+v", st)
	}
	p.Stop()
	time.Sleep(100 * time.Millisecond)
	if n := len(out.with("started ")); n != 1 || p.State().State != StateStopped {
		t.Errorf("%d starts, state %+v", n, p.State())
	}
}

// A program that cannot be started is an error, at once.
func TestAMissingProgramIsAnError(t *testing.T) {
	_, err := Start(context.Background(), Config{Role: "station_link", Path: "/nonexistent/radio-ctl-sync"}, quiet(t))
	if err == nil || !strings.Contains(err.Error(), "station_link") {
		t.Errorf("err = %v", err)
	}
}
