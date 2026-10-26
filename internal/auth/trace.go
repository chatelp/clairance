package auth

import (
	"bufio"
	"encoding/json"
	"os"
	"strings"
	"sync"
	"time"
)

// The sign-in trace: every sign-in, refused or not, and every sign-out, with
// the account, the client's address, its browser and the time. Once the
// server is reached from the Internet (Q62), it is how the owner knows who
// came in and who tried.
//
// Held in memory for reading, and appended to a file beside the accounts so
// that a restart, which ends every session, does not end the history. The
// file is bounded: past traceFileMax it becomes TraceFile + ".1", replacing
// the one before, so the disk holds two at most.
const (
	TraceFile     = "signins.jsonl" // one JSON object per line
	traceFileMax  = 1 << 20
	traceKept     = 500 // events held in memory: the most a reader is given
	traceAgentMax = 200 // a User-Agent is the client's to write, at any length
)

// What happened, and why a sign-in was refused.
const (
	EventSignedIn  = "signed_in"
	EventRefused   = "refused"
	EventSignedOut = "signed_out"

	ReasonWrongPassword   = "wrong_password"
	ReasonUnknownName     = "unknown_name"
	ReasonTooManyAttempts = "too_many_attempts"
)

// SignInEvent is one line of the trace.
//
// User is an account's name, never what was typed when it names no account:
// a password typed in the name field would otherwise be written down.
type SignInEvent struct {
	At     time.Time `json:"at"`
	Event  string    `json:"event"`
	User   string    `json:"user,omitempty"`
	Reason string    `json:"reason,omitempty"`
	From   string    `json:"from"`          // the client, as the trusted proxy gives it
	Via    string    `json:"via,omitempty"` // the proxy it came through, when it did
	Agent  string    `json:"agent,omitempty"`
}

// Trace is the sign-in trace. A zero path keeps it in memory only.
type Trace struct {
	mu        sync.Mutex
	path      string
	events    []SignInEvent        // oldest first, traceKept at most
	throttled map[string]time.Time // per address, the last too_many_attempts written
	now       func() time.Time
}

// NewTrace opens the trace kept at path, reading back the last events
// written there and in the file before it. A line that does not parse is
// skipped: a trace that cannot be read back must not stop a server.
func NewTrace(path string) *Trace {
	t := &Trace{path: path, throttled: map[string]time.Time{}, now: time.Now}
	if path != "" {
		for _, p := range []string{path + ".1", path} {
			t.load(p)
		}
	}
	return t
}

func (t *Trace) load(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64<<10), 64<<10)
	for sc.Scan() {
		var ev SignInEvent
		if json.Unmarshal(sc.Bytes(), &ev) == nil && ev.Event != "" {
			t.keepLocked(ev)
		}
	}
}

func (t *Trace) keepLocked(ev SignInEvent) {
	t.events = append(t.events, ev)
	if len(t.events) > traceKept {
		t.events = append(t.events[:0:0], t.events[len(t.events)-traceKept:]...)
	}
}

// Record adds an event, dated now when it has no date. A client refused for
// too many attempts is written once a minute at most: past the limit, each
// further try says nothing new, and a flood of them would push the rest out
// of the file. The error is the file's; the event is kept in memory anyway.
func (t *Trace) Record(ev SignInEvent) error {
	if t == nil {
		return nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	now := t.now()
	if ev.At.IsZero() {
		ev.At = now.UTC()
	}
	if len(ev.Agent) > traceAgentMax {
		ev.Agent = strings.ToValidUTF8(ev.Agent[:traceAgentMax], "")
	}
	if ev.Reason == ReasonTooManyAttempts {
		if last, ok := t.throttled[ev.From]; ok && now.Sub(last) < time.Minute {
			return nil
		}
		if len(t.throttled) >= 1024 {
			for from, at := range t.throttled {
				if now.Sub(at) >= time.Minute {
					delete(t.throttled, from)
				}
			}
		}
		t.throttled[ev.From] = now
	}
	t.keepLocked(ev)
	if t.path == "" {
		return nil
	}
	return t.appendLocked(ev)
}

func (t *Trace) appendLocked(ev SignInEvent) error {
	line, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(t.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	_, werr := f.Write(append(line, '\n'))
	st, serr := f.Stat()
	if cerr := f.Close(); werr == nil {
		werr = cerr
	}
	if werr != nil {
		return werr
	}
	if serr == nil && st.Size() > traceFileMax {
		return os.Rename(t.path, t.path+".1")
	}
	return nil
}

// Recent is the last n events, the newest first; all of those held when n
// is 0 or more than that.
func (t *Trace) Recent(n int) []SignInEvent {
	t.mu.Lock()
	defer t.mu.Unlock()
	if n <= 0 || n > len(t.events) {
		n = len(t.events)
	}
	out := make([]SignInEvent, 0, n)
	for i := len(t.events) - 1; i >= len(t.events)-n; i-- {
		out = append(out, t.events[i])
	}
	return out
}

// Path is the trace's file, empty when it is kept in memory only.
func (t *Trace) Path() string { return t.path }
