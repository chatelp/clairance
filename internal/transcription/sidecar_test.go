package transcription

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yegors/co-atc/pkg/logger"
)

func testLogger(t *testing.T) *logger.Logger {
	t.Helper()
	log, err := logger.New(logger.Config{Level: "error", Format: "console"})
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	return log
}

func healthServer(t *testing.T) *httptest.Server {
	t.Helper()
	// No pid: a sidecar that does not report one is tolerated, so these tests
	// exercise spawning and stopping without also asserting ownership.
	return healthServerSaying(t, `{"status":"ok"}`)
}

func healthServerSaying(t *testing.T, body string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// With no command configured, an unreachable sidecar must stop the server rather
// than let it run half deaf -- and the error has to say what to do about it.
func TestProbeOnlyFailsWhenNothingIsListening(t *testing.T) {
	s := NewSidecar(SidecarConfig{
		ServerURL:             "http://127.0.0.1:1",
		StartupTimeoutSeconds: 1,
	}, testLogger(t))

	err := s.Start(context.Background())
	if err == nil {
		t.Fatal("expected an error when no sidecar is listening")
	}
	for _, want := range []string{"127.0.0.1:1", "server_url", "command"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error should mention %q, got: %v", want, err)
		}
	}
}

func TestProbeOnlySucceedsAgainstARunningSidecar(t *testing.T) {
	srv := healthServer(t)
	s := NewSidecar(SidecarConfig{ServerURL: srv.URL, StartupTimeoutSeconds: 5}, testLogger(t))

	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	// Nothing was spawned, so Stop must leave the foreign process alone.
	s.Stop()
	if err := s.probe(context.Background()); err != nil {
		t.Errorf("a sidecar we did not start must survive Stop: %v", err)
	}
}

// A sidecar that dies on startup should be reported at once, with its own output,
// instead of making the operator wait out the whole timeout for a bare failure.
func TestChildThatExitsIsReportedWithItsOutput(t *testing.T) {
	s := NewSidecar(SidecarConfig{
		ServerURL:             "http://127.0.0.1:1",
		Command:               []string{"sh", "-c", "echo 'ModuleNotFoundError: mlx_whisper' >&2; exit 3"},
		StartupTimeoutSeconds: 30,
	}, testLogger(t))

	started := time.Now()
	err := s.Start(context.Background())
	if err == nil {
		t.Fatal("expected an error when the sidecar exits at startup")
	}
	if elapsed := time.Since(started); elapsed > 10*time.Second {
		t.Errorf("should fail as soon as the child exits, took %s", elapsed)
	}
	if !strings.Contains(err.Error(), "exited") {
		t.Errorf("error should say the sidecar exited, got: %v", err)
	}
	if !strings.Contains(err.Error(), "ModuleNotFoundError") {
		t.Errorf("error should carry the sidecar's own output, got: %v", err)
	}
}

const degradedBody = `{
  "status": "degraded",
  "degraded": ["fr"],
  "second_opinion": true,
  "second_opinion_ok": 12,
  "second_opinion_failed": 3,
  "models": {
    "en": {"id":"sfabriece/x","kind":"hub","available":true},
    "fr": {"id":"/Volumes/Crucial X8/m","kind":"path","available":false,
           "detail":"path not found -- an external drive may be unplugged"}
  }
}`

// The failure this reporting exists for: a model on an external drive goes away,
// transcription keeps working, and without this nothing anywhere says the station
// just lost its second opinion.
func TestStartReadsAndKeepsADegradedHealth(t *testing.T) {
	srv := healthServerSaying(t, degradedBody)
	s := NewSidecar(SidecarConfig{ServerURL: srv.URL, StartupTimeoutSeconds: 5}, testLogger(t))

	// A degraded sidecar must still start the server: the primary model answers.
	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("a degraded sidecar must not stop startup: %v", err)
	}

	h := s.LastHealth()
	if h.Status != "degraded" {
		t.Errorf("status: got %q, want degraded", h.Status)
	}
	if len(h.Degraded) != 1 || h.Degraded[0] != "fr" {
		t.Errorf("degraded: got %v, want [fr]", h.Degraded)
	}
	if !h.SecondOpinion || h.SecondOpinionOK != 12 || h.SecondOpinionFailed != 3 {
		t.Errorf("counters not carried: %+v", h)
	}
	if m := h.Models["fr"]; m.Available || m.Detail == "" {
		t.Errorf("the unavailable model must carry its reason, got %+v", m)
	}
}

// Refresh failing is not the same as the sidecar being broken: it is busy
// decoding for seconds at a time. The last reading stands, and the caller is
// told the probe failed rather than being handed an invented outage.
func TestRefreshFailureKeepsTheLastReading(t *testing.T) {
	srv := healthServerSaying(t, degradedBody)
	s := NewSidecar(SidecarConfig{ServerURL: srv.URL, StartupTimeoutSeconds: 5}, testLogger(t))
	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	srv.Close() // the sidecar stops answering

	h, err := s.Refresh(context.Background())
	if err == nil {
		t.Error("Refresh should report that it could not reach the sidecar")
	}
	if h.Status != "degraded" || len(h.Degraded) != 1 {
		t.Errorf("the last known reading must survive a failed probe, got %+v", h)
	}
}

// How seventeen servers came to share one sidecar in a night: each spawned its
// own, each failed to bind the port, and each was reassured by the first one's
// answer to the health probe. A probe that succeeds says something is there --
// not that it is ours.
func TestRefusesToStartOnSomeoneElsesSidecar(t *testing.T) {
	// pid 1 is launchd: a pid that is certainly not the child we are about to
	// spawn. The real sidecar reports its own.
	srv := healthServerSaying(t, `{"status":"ok","pid":1}`)
	s := NewSidecar(SidecarConfig{
		ServerURL: srv.URL,
		// Our own child cannot bind, so it dies -- exactly what the sixteen did.
		Command:               []string{"sh", "-c", "echo 'address already in use' >&2; exit 1"},
		StartupTimeoutSeconds: 5,
	}, testLogger(t))

	err := s.Start(context.Background())
	if err == nil {
		t.Fatal("starting on a sidecar we did not spawn must be refused")
	}
	for _, want := range []string{"not the sidecar we started", "command empty"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the error should say %q and what to do about it, got: %v", want, err)
		}
	}
	// And it should carry the child's own reason, which is the actionable part.
	if !strings.Contains(err.Error(), "address already in use") {
		t.Errorf("the error should carry the child's output, got: %v", err)
	}
}

// The deliberate case stays allowed: no command means the operator runs the
// sidecar themselves, and a foreign one answering is the whole point.
func TestAForeignSidecarIsFineWhenNoCommandIsSet(t *testing.T) {
	srv := healthServer(t)
	s := NewSidecar(SidecarConfig{ServerURL: srv.URL, StartupTimeoutSeconds: 5}, testLogger(t))
	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("an empty command means using someone else's sidecar on purpose: %v", err)
	}
}

func countLines(t *testing.T, path string) int {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	return strings.Count(string(b), "\n")
}

// A sidecar that dies on its own is started again (28/09: one native crash in MLX
// cost a whole evening's transcripts), and one that keeps dying is given up on
// rather than restarted forever.
func TestADeadSidecarIsRestartedThenGivenUp(t *testing.T) {
	restartDelay = 20 * time.Millisecond
	t.Cleanup(func() { restartDelay = 5 * time.Second })
	srv := healthServer(t)
	starts := filepath.Join(t.TempDir(), "starts")
	s := NewSidecar(SidecarConfig{
		ServerURL:             srv.URL,
		Command:               []string{"sh", "-c", "echo up >> " + starts + "; sleep 0.2; exit 1"},
		StartupTimeoutSeconds: 5,
	}, testLogger(t))
	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.Supervise(ctx)

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) && countLines(t, starts) < 1+maxRestarts {
		time.Sleep(50 * time.Millisecond)
	}
	if n := countLines(t, starts); n != 1+maxRestarts {
		t.Fatalf("started %d times, want the first start and %d restarts", n, maxRestarts)
	}
	time.Sleep(600 * time.Millisecond)
	if n := countLines(t, starts); n != 1+maxRestarts {
		t.Errorf("started %d times: it should have given up after %d restarts", n, maxRestarts)
	}
}

// Stop is the operator's decision, not a crash: nothing is restarted after it.
func TestStopEndsSupervision(t *testing.T) {
	restartDelay = 20 * time.Millisecond
	t.Cleanup(func() { restartDelay = 5 * time.Second })
	srv := healthServer(t)
	starts := filepath.Join(t.TempDir(), "starts")
	s := NewSidecar(SidecarConfig{
		ServerURL:             srv.URL,
		Command:               []string{"sh", "-c", "echo up >> " + starts + "; sleep 60"},
		StartupTimeoutSeconds: 5,
	}, testLogger(t))
	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	s.Supervise(context.Background())
	// Start returns on the health answer, which here is not the child's: wait
	// for the child itself to be up before stopping it.
	for i := 0; i < 100 && countLines(t, starts) < 1; i++ {
		time.Sleep(20 * time.Millisecond)
	}
	s.Stop()
	time.Sleep(300 * time.Millisecond)
	if n := countLines(t, starts); n != 1 {
		t.Errorf("started %d times after Stop, want 1", n)
	}
}

// The sidecar says what it computes on; the probe keeps it with the rest of
// its health, and a sidecar that does not say leaves it empty.
func TestTheProbeReadsTheBackend(t *testing.T) {
	srv := healthServerSaying(t, `{"status":"ok","backend":"fake","model_en":"sfabriece/whisper-large-v3-atco2-asr-mlx"}`)
	s := NewSidecar(SidecarConfig{ServerURL: srv.URL}, testLogger(t))
	h, err := s.Refresh(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if h.Backend != "fake" || h.ModelEN != "sfabriece/whisper-large-v3-atco2-asr-mlx" {
		t.Errorf("backend %q, model_en %q", h.Backend, h.ModelEN)
	}
	silent := NewSidecar(SidecarConfig{ServerURL: healthServer(t).URL}, testLogger(t))
	if h, _ := silent.Refresh(context.Background()); h.Backend != "" {
		t.Errorf("a sidecar that does not say: backend %q, want empty", h.Backend)
	}
}

// A server that started with nothing to transcribe is off, and stays off while
// nothing answers; a source that asks is told what to do. Once a sidecar
// answers, it is on.
func TestAnIdleSidecarIsOffUntilOneAnswers(t *testing.T) {
	s := NewSidecar(SidecarConfig{ServerURL: "http://127.0.0.1:1"}, testLogger(t))
	s.Idle()
	if !s.Off() {
		t.Fatal("an idle sidecar must say the server runs without transcription")
	}
	err := s.Ensure(context.Background())
	if err == nil {
		t.Fatal("nothing answers: Ensure must refuse")
	}
	for _, want := range []string{"127.0.0.1:1/health", "sidecar/README.md", "transcribe_audio"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal should mention %q, got: %v", want, err)
		}
	}
	if !s.Off() {
		t.Error("a refused source must not turn transcription on")
	}

	srv := healthServer(t)
	on := NewSidecar(SidecarConfig{ServerURL: srv.URL}, testLogger(t))
	on.Idle()
	if err := on.Ensure(context.Background()); err != nil {
		t.Fatalf("a sidecar answers: Ensure: %v", err)
	}
	if on.Off() {
		t.Error("a sidecar was reached: transcription is on")
	}
	// Unsupervised, so asked again for the next source.
	srv.Close()
	if err := on.Ensure(context.Background()); err == nil {
		t.Error("after Idle the sidecar is probed for each source: it has gone")
	}
}

// A sidecar reached at startup is supervised: a source is not refused because
// it happens to be restarting, as before.
func TestASidecarStartedWithTheServerIsNotProbedForASource(t *testing.T) {
	srv := healthServer(t)
	s := NewSidecar(SidecarConfig{ServerURL: srv.URL, StartupTimeoutSeconds: 5}, testLogger(t))
	if err := s.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	srv.Close()
	if err := s.Ensure(context.Background()); err != nil {
		t.Errorf("Ensure after Start: %v, want nil", err)
	}
	if s.Off() {
		t.Error("a started sidecar is not off")
	}
}

// The sidecar this server started, as the launcher's health shows it
// (docs-fr/36): running with its pid, restarting, given up on with how it
// ended, stopped; a sidecar run by someone else is not described as ours.
func TestTheChildIsDescribedForTheLauncher(t *testing.T) {
	restartDelay = 20 * time.Millisecond
	t.Cleanup(func() { restartDelay = 5 * time.Second })
	srv := healthServer(t)

	foreign := NewSidecar(SidecarConfig{ServerURL: srv.URL}, testLogger(t))
	if err := foreign.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if c, ok := foreign.Child(); ok {
		t.Errorf("someone else's sidecar described as ours: %+v", c)
	}

	starts := filepath.Join(t.TempDir(), "starts")
	dying := NewSidecar(SidecarConfig{ServerURL: srv.URL, StartupTimeoutSeconds: 5,
		Command: []string{"sh", "-c", "echo up >> " + starts + "; sleep 0.2; exit 1"}}, testLogger(t))
	if err := dying.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	c, ok := dying.Child()
	if !ok || c.State != "running" || c.PID == 0 || c.StartedAt.IsZero() || c.Restarts != 0 || c.LastExit != "" ||
		len(c.Command) != 3 || c.Command[0] != "sh" {
		t.Errorf("started: %+v", c)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	dying.Supervise(ctx)
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if c, _ = dying.Child(); c.State == "failed" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if c.State != "failed" || c.PID != 0 || c.Restarts != maxRestarts || c.LastExit != "exit status 1" {
		t.Errorf("given up on: %+v", c)
	}

	running := NewSidecar(SidecarConfig{ServerURL: srv.URL, StartupTimeoutSeconds: 5,
		Command: []string{"sh", "-c", "sleep 60"}}, testLogger(t))
	if err := running.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	running.Stop()
	if c, ok := running.Child(); !ok || c.State != "stopped" || c.PID != 0 {
		t.Errorf("stopped: %+v", c)
	}
}
