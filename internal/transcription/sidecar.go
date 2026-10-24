package transcription

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/yegors/co-atc/pkg/logger"
)

// Sidecar ties the local speech-to-text service to the lifetime of this server.
//
// Upstream's docs/LOCAL-STT.md specifies the sidecar as an HTTP service behind a
// URL, which deliberately leaves it free to run elsewhere -- another terminal,
// another machine, a container. That stays true: with no Command configured this
// only probes the URL. But a transcription service has no reason to outlive the
// program that is its only client, so when Command is set we start one and stop
// it on the way out.
//
// The probe is not optional either way. Without it co-atc starts perfectly,
// serves the map and the audio, and silently transcribes nothing: every
// transmission logs one error and is dropped. Measured on this station, a busy
// day would produce ~1300 such lines and no transcript. The ADS-B source is
// validated at startup for exactly this reason; this is the same guarantee for
// the other half of the product.
type Sidecar struct {
	url     string
	command []string
	timeout time.Duration
	logger  *logger.Logger

	mu      sync.Mutex
	cmd     *exec.Cmd
	output  *tailBuffer
	stopped bool

	// A child is reaped as soon as it is started. Without that, an exec.Cmd whose
	// process has died still reports nothing: ProcessState is only filled in by
	// Wait, and signal 0 succeeds against a zombie. A sidecar that exits on
	// startup would then be indistinguishable from one still loading, and the
	// operator would wait out the whole timeout for a bare "did not answer".
	done    chan struct{}
	waitErr error

	lastHealth Health

	// The server started with nothing to transcribe and no sidecar to run
	// (Idle): nothing was started or probed, and nothing vouches for a sidecar
	// at the URL. reached is set once a source asking for transcription found
	// one answering there (Ensure).
	idle    bool
	reached bool
}

// Health is what the sidecar says about itself. The field that matters is
// Degraded: the French model on this station is a symlink to an external drive,
// and unplugging it does not stop transcription -- it silently stops the second
// opinion, and with it 8.5% of callsign matches. Nothing else in the system
// would say so.
type Health struct {
	Status              string   `json:"status"` // "ok" or "degraded"
	Degraded            []string `json:"degraded"`
	SecondOpinion       bool     `json:"second_opinion"`
	SecondOpinionOK     int      `json:"second_opinion_ok"`
	SecondOpinionFailed int      `json:"second_opinion_failed"`
	PID                 int      `json:"pid"`
	// The English model's name as configured -- a Hugging Face id or a local
	// path. The sign-in page shows its family, never the name itself.
	ModelEN string `json:"model_en"`
	// What the models run on, as the sidecar declares it: the GPU's name, or
	// "mlx". Empty from a sidecar that does not say.
	Backend string `json:"backend"`
	// The model's queue by lane (sidecar/lanes.py), the second readings not
	// fetched yet, and whether clips are being archived -- what the launcher's
	// health shows (docs-fr/34).
	Queue struct {
		First  int `json:"first"`
		Second int `json:"second"`
	} `json:"queue"`
	SecondPending           int  `json:"second_pending"`
	Archiving               bool `json:"archiving"`
	ArchivingStoppedLowDisk bool `json:"archiving_stopped_low_disk"`
	Models                  map[string]struct {
		ID        string `json:"id"`
		Kind      string `json:"kind"`
		Available bool   `json:"available"`
		Detail    string `json:"detail"`
	} `json:"models"`
}

// SidecarConfig is what main needs to supply; it maps onto [transcription.local].
type SidecarConfig struct {
	ServerURL             string
	Command               []string
	StartupTimeoutSeconds int
}

// NewSidecar returns a supervisor for the local STT service. It does nothing
// until Start is called.
func NewSidecar(cfg SidecarConfig, log *logger.Logger) *Sidecar {
	timeout := time.Duration(cfg.StartupTimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	return &Sidecar{
		url:     strings.TrimRight(cfg.ServerURL, "/"),
		command: cfg.Command,
		timeout: timeout,
		logger:  log.Named("stt-sidecar"),
		output:  &tailBuffer{limit: 4096},
	}
}

// Start launches the sidecar when one is configured, then waits for it to answer
// /health. It returns an error rather than degrading quietly: a server that
// cannot transcribe should say so at startup, not one log line at a time.
func (s *Sidecar) Start(ctx context.Context) error {
	if len(s.command) > 0 {
		if err := s.spawn(); err != nil {
			return err
		}
	}

	deadline := time.Now().Add(s.timeout)
	var lastErr error
	announced := false
	for {
		if err := s.probe(ctx); err == nil {
			// Something answers -- but is it ours? When we spawned a child, a
			// probe succeeding while that child is dead means another process
			// already holds the port. Accepting it is how seventeen servers came
			// to share one sidecar in a night: each spawned its own, each failed
			// to bind, and each was reassured by the first one's answer.
			//
			// Using a sidecar someone else runs is legitimate -- that is what an
			// empty command is for. Doing it by accident is not.
			if len(s.command) > 0 {
				if why, ok := s.notOurs(); !ok {
					// About to fail: give the child the moment it needs to finish
					// dying and flush its reason. "address already in use" is the
					// actionable half of this error, and it arrives a few
					// milliseconds after the probe that revealed the problem.
					s.waitForChild(500 * time.Millisecond)
					// And stop it if it is somehow still alive: refusing to start
					// is no reason to leave a process behind. The timeout path
					// does the same.
					s.Stop()
					return fmt.Errorf(
						"something is already answering at %s/health, but it is not the sidecar we started: %s.\n"+
							"Stop the other one, or leave transcription.local.command empty to use it on purpose.\n%s",
						s.url, why, s.output.String())
				}
				s.logger.Info("Local STT sidecar is up", logger.String("url", s.url))
			} else {
				s.logger.Info("Local STT sidecar reached", logger.String("url", s.url))
			}
			s.warnIfDegraded()
			return nil
		} else {
			lastErr = err
			// Say we are waiting, once. Sixty silent seconds before a failure
			// look like a hang, and the operator cannot tell whether to go and
			// start the sidecar themselves.
			if !announced {
				announced = true
				s.logger.Info("Waiting for the local STT sidecar",
					logger.String("url", s.url+"/health"),
					logger.String("timeout", s.timeout.String()))
			}
		}

		// A child that has already exited will never answer; say so now, with
		// whatever it printed, instead of waiting out the timeout.
		if err := s.exited(); err != nil {
			return fmt.Errorf("local STT sidecar exited during startup: %w\n%s", err, s.output.String())
		}

		if time.Now().After(deadline) {
			s.Stop()
			if len(s.command) > 0 {
				return fmt.Errorf("local STT sidecar did not answer %s/health within %s: %w\n%s",
					s.url, s.timeout, lastErr, s.output.String())
			}
			return fmt.Errorf("no local STT sidecar answering at %s/health: %w\n"+
				"Start one (see sidecar/README.md), point transcription.local.server_url at it, "+
				"or set transcription.local.command so co-atc starts it itself", s.url, lastErr)
		}

		select {
		case <-ctx.Done():
			s.Stop()
			return ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
}

// Idle records that the server runs without transcription: no source asks for
// it and no command starts a sidecar, so there is nothing to start and nothing
// to wait for. Refusing to start there, as Start does, would keep someone with
// no sidecar yet -- no Apple Silicon, no GPU -- from the map and the audio,
// which need none. A source that asks for transcription later goes through
// Ensure.
func (s *Sidecar) Idle() {
	s.mu.Lock()
	s.idle = true
	s.mu.Unlock()
}

// Off reports that the server runs without transcription: it started Idle and
// no source has found a sidecar answering since.
func (s *Sidecar) Off() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.idle && !s.reached
}

// Ensure is asked before a source that transcribes is started. A sidecar Start
// reached with the server is taken as it is: it was probed then and is
// supervised now, and refusing a source while it restarts would cost that
// source its audio as well. After Idle nothing vouches for one, so it is probed
// now, each time, and an error says what to do when nothing answers.
func (s *Sidecar) Ensure(ctx context.Context) error {
	s.mu.Lock()
	idle := s.idle
	s.mu.Unlock()
	if !idle {
		return nil
	}
	if err := s.probe(ctx); err != nil {
		return fmt.Errorf("no local STT sidecar answers at %s/health (%v): "+
			"start one (see sidecar/README.md), or set transcribe_audio to false for this source", s.url, err)
	}
	s.mu.Lock()
	first := !s.reached
	s.reached = true
	s.mu.Unlock()
	if first {
		s.logger.Info("Local STT sidecar reached: transcription is on", logger.String("url", s.url))
		s.warnIfDegraded()
	}
	return nil
}

// Stop terminates a sidecar we started. One that was already running when we
// arrived is left alone -- it is not ours to kill.
func (s *Sidecar) Stop() {
	s.mu.Lock()
	cmd := s.cmd
	if cmd == nil || cmd.Process == nil || s.stopped {
		s.mu.Unlock()
		return
	}
	s.stopped = true
	s.mu.Unlock()

	pid := cmd.Process.Pid
	s.logger.Info("Stopping local STT sidecar", logger.Int("pid", pid))

	// Signal the whole process group (sidecar_unix.go says why).
	terminateGroup(cmd)

	s.mu.Lock()
	done := s.done
	s.mu.Unlock()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		s.logger.Warn("Local STT sidecar did not stop on SIGTERM, killing", logger.Int("pid", pid))
		killGroup(cmd)
		<-done
	}
}

func (s *Sidecar) spawn() error {
	cmd := exec.Command(s.command[0], s.command[1:]...)
	startInOwnGroup(cmd)
	// Tells our sidecar to stop if we are killed outright. Stop() handles every
	// ordinary exit, but not SIGKILL -- and a sidecar that outlives us holds the
	// port, which is how seventeen servers came to share one in a night. A
	// sidecar that does not know this variable simply ignores it, which is what
	// any other implementation of the contract will do.
	cmd.Env = append(os.Environ(), "COATC_SPAWNED=1")
	// The sidecar's own logs stay visible on the terminal, and a copy is kept so
	// a startup failure can be reported with its cause attached.
	cmd.Stdout = io.MultiWriter(os.Stdout, s.output)
	cmd.Stderr = io.MultiWriter(os.Stderr, s.output)

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start local STT sidecar %q: %w", strings.Join(s.command, " "), err)
	}

	done := make(chan struct{})
	s.mu.Lock()
	s.cmd = cmd
	s.done = done
	s.mu.Unlock()

	go func() {
		err := cmd.Wait()
		s.mu.Lock()
		s.waitErr = err
		s.mu.Unlock()
		close(done)
	}()

	s.logger.Info("Started local STT sidecar",
		logger.Int("pid", cmd.Process.Pid),
		logger.String("command", strings.Join(s.command, " ")))
	return nil
}

// Restarts allowed within restartWindow before Supervise gives up: a sidecar
// that dies again and again has a cause that restarting will not fix, and a
// crash loop would only bury it.
const (
	maxRestarts   = 5
	restartWindow = 10 * time.Minute
)

// restartDelay is how long Supervise waits before restarting. A variable so
// tests need not wait.
var restartDelay = 5 * time.Second

// Supervise restarts a sidecar we started when it dies on its own, and returns
// at once; the watching is done in the background until ctx ends or Stop is
// called.
//
// Measured on 28/09: the sidecar died of a native crash in MLX eight minutes into
// a session, and co-atc went on for the rest of it -- 550 "connection refused",
// no transcript -- because it only ever started its sidecar once. For a server
// that runs unattended for hours, one crash must cost seconds, not the evening.
// A sidecar that is not ours (no command) is left to whoever runs it.
func (s *Sidecar) Supervise(ctx context.Context) {
	if len(s.command) == 0 {
		return
	}
	go func() {
		var restarts []time.Time
		for {
			s.mu.Lock()
			done := s.done
			s.mu.Unlock()
			if done == nil {
				return
			}
			select {
			case <-ctx.Done():
				return
			case <-done:
			}

			s.mu.Lock()
			stopped, waitErr := s.stopped, s.waitErr
			s.mu.Unlock()
			if stopped {
				return
			}
			now := time.Now()
			kept := restarts[:0]
			for _, t := range restarts {
				if now.Sub(t) < restartWindow {
					kept = append(kept, t)
				}
			}
			restarts = kept
			if len(restarts) >= maxRestarts {
				s.logger.Error("Local STT sidecar keeps dying, no longer restarting it: transcription is stopped",
					logger.Int("restarts", len(restarts)),
					logger.String("window", restartWindow.String()),
					logger.String("last_output", s.output.String()))
				return
			}
			s.logger.Error("Local STT sidecar died, restarting it",
				logger.Error(waitErr),
				logger.String("last_output", s.output.String()))

			select {
			case <-ctx.Done():
				return
			case <-time.After(restartDelay):
			}
			s.mu.Lock()
			stopped = s.stopped
			s.mu.Unlock()
			if stopped {
				return
			}
			restarts = append(restarts, time.Now())
			if err := s.spawn(); err != nil {
				s.logger.Error("Could not restart the local STT sidecar", logger.Error(err))
				return
			}
			deadline := time.Now().Add(s.timeout)
			for time.Now().Before(deadline) {
				if err := s.probe(ctx); err == nil {
					s.logger.Info("Local STT sidecar restarted", logger.Int("restarts_in_window", len(restarts)))
					break
				}
				if s.exited() != nil {
					break // dead again: the loop above counts it
				}
				select {
				case <-ctx.Done():
					return
				case <-time.After(500 * time.Millisecond):
				}
			}
		}
	}()
}

// exited reports how the child ended, or nil while it is still running (or was
// never ours to begin with).
func (s *Sidecar) exited() error {
	s.mu.Lock()
	done, cmd, waitErr := s.done, s.cmd, s.waitErr
	s.mu.Unlock()
	if done == nil || cmd == nil {
		return nil
	}
	select {
	case <-done:
		if waitErr != nil {
			return waitErr
		}
		return fmt.Errorf("exited cleanly without serving %s/health", s.url)
	default:
		return nil
	}
}

func (s *Sidecar) probe(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.url+"/health", nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 16384))
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health returned %d", resp.StatusCode)
	}
	var h Health
	if err := json.Unmarshal(body, &h); err == nil {
		s.mu.Lock()
		s.lastHealth = h
		s.mu.Unlock()
	}
	return nil
}

// LastHealth returns what the sidecar last said about itself. Callers that want
// it current call Refresh first.
func (s *Sidecar) LastHealth() Health {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastHealth
}

// Refresh re-reads the sidecar's health. Used by the operational state endpoint,
// so an external drive pulled out an hour ago shows up the moment someone looks.
func (s *Sidecar) Refresh(ctx context.Context) (Health, error) {
	if err := s.probe(ctx); err != nil {
		return s.LastHealth(), err
	}
	return s.LastHealth(), nil
}

// tailBuffer keeps the last `limit` bytes written to it, so a failure can be
// reported with the sidecar's own output instead of a bare exit status.
type tailBuffer struct {
	mu    sync.Mutex
	buf   bytes.Buffer
	limit int
}

func (t *tailBuffer) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.buf.Write(p)
	if t.buf.Len() > t.limit {
		b := t.buf.Bytes()
		trimmed := append([]byte(nil), b[t.buf.Len()-t.limit:]...)
		t.buf.Reset()
		t.buf.Write(trimmed)
	}
	return len(p), nil
}

func (t *tailBuffer) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return strings.TrimSpace(t.buf.String())
}

// warnIfDegraded says out loud what would otherwise be a silence. A missing
// model does not stop the server -- the primary transcript still arrives -- so
// without this the only symptom is fewer aircraft identified, with no cause.
func (s *Sidecar) warnIfDegraded() {
	h := s.LastHealth()
	for _, lang := range h.Degraded {
		m := h.Models[lang]
		s.logger.Warn("A transcription model is unreachable -- the server will run without it",
			logger.String("language", lang),
			logger.String("model", m.ID),
			logger.String("detail", m.Detail))
	}
	if h.SecondOpinion && len(h.Degraded) == 0 {
		s.logger.Info("Second opinion is on", logger.String("url", s.url))
	}
}

// notOurs decides whether the process answering /health is the child we started.
//
// Timing cannot answer this: a child that fails to bind the port dies in
// milliseconds, and whether its death has been reaped by the time the first
// probe returns is a race. So the sidecar reports its own pid and we compare
// process groups -- ours runs in its own, set at spawn, and every descendant
// shares it. That covers a command that is a wrapper script as well as one that
// is the interpreter itself.
// On Windows, which has no process groups, only the pid itself is matched.
func (s *Sidecar) notOurs() (string, bool) {
	s.mu.Lock()
	cmd := s.cmd
	s.mu.Unlock()
	if cmd == nil || cmd.Process == nil {
		return "", true
	}
	if gone := s.exited(); gone != nil {
		return fmt.Sprintf("ours %v", gone), false
	}

	reported := s.LastHealth().PID
	if reported == 0 {
		// An older sidecar that does not report its pid. Not a reason to refuse:
		// it may well be ours, and refusing on an absent field would break an
		// install that works.
		return "", true
	}
	if reported == cmd.Process.Pid {
		return "", true
	}
	if inGroupOf(reported, cmd.Process.Pid) {
		return "", true
	}
	return fmt.Sprintf("pid %d answered, ours is %d", reported, cmd.Process.Pid), false
}

// waitForChild gives a child that is on its way out the time to exit and flush,
// so its own message reaches the error the operator reads.
func (s *Sidecar) waitForChild(d time.Duration) {
	s.mu.Lock()
	done := s.done
	s.mu.Unlock()
	if done == nil {
		return
	}
	select {
	case <-done:
	case <-time.After(d):
	}
}
