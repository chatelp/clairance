// Package lifecycle tells whoever started the server how its start goes, on a
// file descriptor of their choosing (co-atc -status-fd N): one JSON object per
// line, each beginning with {"coatc_status": (docs-fr/36-contrat-lanceur.md).
//
// A server that fails to start exits before it can answer anything, so the
// cause of a failure cannot come from the API. Until 26/10 the launcher dug it
// out of the log -- a tab, "fatal", a tab, then a JSON field -- and the day
// it misread one it said "Production did not start" of a production that ran.
// This says it in a form meant to be read by a program, once.
//
// Without -status-fd nothing is written: the log stays what it was.
package lifecycle

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

// The events, in the order a start goes through them.
const (
	Starting = "starting" // pid, version, commit, config
	Step     = "step"     // step, and timeout_s when it may take long
	Ready    = "ready"    // pid, url, listeners
	Fatal    = "fatal"    // cause, error; the process exits with status 1
	Stopping = "stopping"
	Stopped  = "stopped"
)

var (
	mu  sync.Mutex
	out io.Writer
)

// Open sends the events to file descriptor fd: 1 and 2 are the standard
// output and error, on every system; any other is a descriptor the parent
// left open, on Unix.
func Open(fd int) error {
	var w io.Writer
	switch fd {
	case 1:
		w = os.Stdout
	case 2:
		w = os.Stderr
	default:
		if fd < 0 {
			return fmt.Errorf("-status-fd %d: not a descriptor", fd)
		}
		f := os.NewFile(uintptr(fd), "status")
		if f == nil {
			return fmt.Errorf("-status-fd %d: not a descriptor", fd)
		}
		if _, err := f.Stat(); err != nil {
			return fmt.Errorf("-status-fd %d: %w", fd, err)
		}
		w = f
	}
	mu.Lock()
	out = w
	mu.Unlock()
	return nil
}

// Say writes one event with its fields, and the time. Nothing when no
// descriptor was opened; a write that fails is not the server's concern.
func Say(event string, fields map[string]any) {
	mu.Lock()
	defer mu.Unlock()
	if out == nil {
		return
	}
	_, _ = out.Write(Line(event, fields, time.Now()))
}

// Failed says why the server cannot start, as one of the causes the contract
// lists (config, adsb_source, sidecar, listen...). The caller exits after it,
// as it did before.
func Failed(cause string, err error) {
	msg := ""
	if err != nil {
		msg = err.Error()
	}
	Say(Fatal, map[string]any{"cause": cause, "error": msg})
}

// Line is one event as written: {"coatc_status":event,"at":...,fields...}.
// The event first, so that a reader can tell these lines from the others the
// same descriptor may carry -- the sidecar's output, a panic -- by their
// beginning.
func Line(event string, fields map[string]any, at time.Time) []byte {
	head, _ := json.Marshal(event)
	all := map[string]any{"at": at.UTC().Format(time.RFC3339Nano)}
	for k, v := range fields {
		all[k] = v
	}
	rest, err := json.Marshal(all)
	if err != nil {
		rest, _ = json.Marshal(map[string]any{"at": all["at"], "error": err.Error()})
	}
	b := append([]byte(`{"coatc_status":`), head...)
	b = append(b, ',')
	b = append(b, rest[1:]...) // past the opening brace
	return append(b, '\n')
}
