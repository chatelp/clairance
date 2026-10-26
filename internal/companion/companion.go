// Package companion runs the programs co-atc starts beside itself -- the
// station link, the keep-awake assertion -- and keeps them running for as long
// as it runs (docs-fr/36-contrat-lanceur.md, plan 35 B3).
//
// Until 26/10 a script outside the repository started them, by name and with
// the station's address written in it, and the launcher found them again in
// the process table. Now co-atc starts each in a process group of its own,
// restarts it after a delay that grows while it keeps dying, and stops it with
// itself: SIGTERM to the group, SIGKILL after a grace period. Whoever started
// co-atc starts one program.
//
// The transcription sidecar keeps its own supervisor (internal/transcription,
// sidecar.go): it predates this one, probes a health route this package knows
// nothing of, and is meant to be offered upstream as it is.
package companion

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/yegors/co-atc/pkg/logger"
)

// Config is one companion.
type Config struct {
	Role string   // what it is for, as the launcher's health names it: "station_link"
	Path string   // the program, found already
	Args []string // its arguments
	Env  []string // added to co-atc's environment; secrets go here, never in Args

	// Output receives each line the program writes, on stdout or stderr; nil
	// drops them. The last one is kept either way, for the health.
	Output func(line string)

	// The delays. Zero takes the defaults: a first restart after 1 s, doubled
	// at each death that comes less than a minute after the start, 2 min at
	// most; 5 s between SIGTERM and SIGKILL.
	FirstDelay, MaxDelay, StableAfter, Grace time.Duration
}

// The states a companion can be in.
const (
	StateRunning    = "running"
	StateRestarting = "restarting"
	StateStopped    = "stopped"
)

// State is a companion as the launcher's health shows it.
type State struct {
	Role        string
	PID         int // 0 when no process runs
	State       string
	StartedAt   time.Time // of the process running now
	Restarts    int
	LastExit    string // how the previous process ended
	LastOutput  string // the last line it wrote
	Command     []string
	NextStartAt time.Time // while restarting
}

// Process is one supervised companion.
type Process struct {
	cfg Config
	log *logger.Logger

	mu        sync.Mutex
	cmd       *exec.Cmd
	done      chan struct{} // closed when cmd has been waited for
	waitErr   error
	state     string
	startedAt time.Time
	restarts  int
	lastExit  string
	lastLine  string
	nextStart time.Time

	stop     chan struct{} // closed by Stop
	stopOnce sync.Once
	finished chan struct{} // closed when the supervisor has returned
}

// Start starts the program and supervises it until ctx ends or Stop is
// called. An error means it could not be started at all -- a missing or
// unrunnable program -- and nothing is left running.
func Start(ctx context.Context, cfg Config, log *logger.Logger) (*Process, error) {
	if cfg.FirstDelay <= 0 {
		cfg.FirstDelay = time.Second
	}
	if cfg.MaxDelay <= 0 {
		cfg.MaxDelay = 2 * time.Minute
	}
	if cfg.StableAfter <= 0 {
		cfg.StableAfter = time.Minute
	}
	if cfg.Grace <= 0 {
		cfg.Grace = 5 * time.Second
	}
	p := &Process{cfg: cfg, log: log.Named(cfg.Role), stop: make(chan struct{}), finished: make(chan struct{})}
	if err := p.spawn(); err != nil {
		return nil, err
	}
	go p.supervise(ctx)
	return p, nil
}

func (p *Process) spawn() error {
	cmd := exec.Command(p.cfg.Path, p.cfg.Args...)
	startInOwnGroup(cmd)
	cmd.Env = append(os.Environ(), p.cfg.Env...)
	out := &lineWriter{emit: p.line}
	cmd.Stdout, cmd.Stderr = out, out
	// A grandchild that keeps the output pipe open must not hold Wait, and with
	// it the restart, for ever.
	cmd.WaitDelay = 2 * time.Second
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting %s (%s): %w", p.cfg.Role, p.cfg.Path, err)
	}
	done := make(chan struct{})
	p.mu.Lock()
	p.cmd, p.done, p.waitErr = cmd, done, nil
	p.state, p.startedAt, p.nextStart = StateRunning, time.Now(), time.Time{}
	p.mu.Unlock()
	go func() {
		err := cmd.Wait()
		out.flush()
		p.mu.Lock()
		p.waitErr = err
		p.mu.Unlock()
		close(done)
	}()
	p.log.Info("Started", logger.Int("pid", cmd.Process.Pid), logger.String("command", p.commandLine()))
	return nil
}

// supervise restarts the program each time it dies on its own.
func (p *Process) supervise(ctx context.Context) {
	defer close(p.finished)
	delay := p.cfg.FirstDelay
	for {
		p.mu.Lock()
		done, started := p.done, p.startedAt
		p.mu.Unlock()
		select {
		case <-ctx.Done():
			return
		case <-p.stop:
			return
		case <-done:
		}
		if p.stopping() {
			return
		}

		p.mu.Lock()
		ran := time.Since(started)
		p.lastExit = exitText(p.waitErr)
		if ran >= p.cfg.StableAfter {
			delay = p.cfg.FirstDelay
		}
		p.state, p.nextStart = StateRestarting, time.Now().Add(delay)
		lastExit, lastLine := p.lastExit, p.lastLine
		p.mu.Unlock()
		p.log.Error("Died, restarting it", logger.String("exit", lastExit), logger.String("ran", ran.Round(time.Millisecond).String()),
			logger.String("last_output", lastLine), logger.String("restart_in", delay.String()))

		for {
			select {
			case <-ctx.Done():
				p.setStopped()
				return
			case <-p.stop:
				p.setStopped()
				return
			case <-time.After(delay):
			}
			delay = min(2*delay, p.cfg.MaxDelay)
			p.mu.Lock()
			p.restarts++
			p.mu.Unlock()
			if err := p.spawn(); err != nil {
				p.mu.Lock()
				p.lastExit = err.Error()
				p.nextStart = time.Now().Add(delay)
				p.mu.Unlock()
				p.log.Error("Could not restart it", logger.Error(err), logger.String("next_try_in", delay.String()))
				continue
			}
			break
		}
	}
}

// Stop stops the program and its group, and stops restarting it. It waits
// for it to be gone: SIGTERM, then SIGKILL after the grace period.
func (p *Process) Stop() {
	if p == nil {
		return
	}
	p.stopOnce.Do(func() { close(p.stop) })
	// First the supervisor, which may be starting a process this very moment:
	// once it has returned, whatever runs is the last one.
	<-p.finished
	p.mu.Lock()
	cmd, done := p.cmd, p.done
	p.mu.Unlock()
	if done != nil && !isClosed(done) {
		p.log.Info("Stopping", logger.Int("pid", cmd.Process.Pid))
		terminateGroup(cmd)
		select {
		case <-done:
		case <-time.After(p.cfg.Grace):
			p.log.Warn("Did not stop on SIGTERM, killing it", logger.Int("pid", cmd.Process.Pid))
			killGroup(cmd)
			<-done
		}
		p.mu.Lock()
		p.lastExit = exitText(p.waitErr)
		p.mu.Unlock()
	}
	p.setStopped()
}

// State says what the companion is doing now.
func (p *Process) State() State {
	p.mu.Lock()
	defer p.mu.Unlock()
	s := State{Role: p.cfg.Role, State: p.state, Restarts: p.restarts, LastExit: p.lastExit,
		LastOutput: p.lastLine, Command: append([]string{p.cfg.Path}, p.cfg.Args...)}
	if p.state == StateRunning && p.cmd != nil && p.cmd.Process != nil && !isClosed(p.done) {
		s.PID, s.StartedAt = p.cmd.Process.Pid, p.startedAt
	}
	if p.state == StateRestarting {
		s.NextStartAt = p.nextStart
	}
	return s
}

func (p *Process) stopping() bool {
	select {
	case <-p.stop:
		return true
	default:
		return false
	}
}

func (p *Process) setStopped() {
	p.mu.Lock()
	p.state, p.nextStart = StateStopped, time.Time{}
	p.mu.Unlock()
}

func (p *Process) line(s string) {
	p.mu.Lock()
	p.lastLine = s
	p.mu.Unlock()
	if p.cfg.Output != nil {
		p.cfg.Output(s)
	}
}

func (p *Process) commandLine() string {
	return strings.Join(append([]string{p.cfg.Path}, p.cfg.Args...), " ")
}

func isClosed(c chan struct{}) bool {
	select {
	case <-c:
		return true
	default:
		return false
	}
}

func exitText(err error) string {
	if err == nil {
		return "exited cleanly"
	}
	return err.Error()
}

// lineWriter cuts what a program writes into lines.
type lineWriter struct {
	mu   sync.Mutex
	buf  bytes.Buffer
	emit func(string)
}

func (w *lineWriter) Write(b []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.buf.Write(b)
	for {
		i := bytes.IndexByte(w.buf.Bytes(), '\n')
		if i < 0 {
			break
		}
		line := strings.TrimRight(string(w.buf.Next(i+1)), "\r\n")
		if line != "" {
			w.emit(line)
		}
	}
	// A program that never ends its line must not grow this without bound.
	if w.buf.Len() > 4096 {
		w.emit(w.buf.String())
		w.buf.Reset()
	}
	return len(b), nil
}

func (w *lineWriter) flush() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if s := strings.TrimSpace(w.buf.String()); s != "" {
		w.emit(s)
	}
	w.buf.Reset()
}
