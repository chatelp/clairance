package auth

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The trace is read back after a restart, the newest event first; its file
// is the owner's alone to read.
func TestTheTraceOutlivesARestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), TraceFile)
	tr := NewTrace(path)
	for _, ev := range []SignInEvent{
		{Event: EventRefused, Reason: ReasonUnknownName, From: "203.0.113.5"},
		{Event: EventSignedIn, User: "guest", From: "203.0.113.6", Via: "192.0.2.1", Agent: "Safari"},
	} {
		if err := tr.Record(ev); err != nil {
			t.Fatal(err)
		}
	}
	if st, err := os.Stat(path); err != nil || st.Mode().Perm() != 0o600 {
		t.Fatalf("the trace file: %v, %v; want it readable by its owner only", st.Mode(), err)
	}

	got := NewTrace(path).Recent(0)
	if len(got) != 2 || got[0].Event != EventSignedIn || got[0].User != "guest" || got[0].Via != "192.0.2.1" ||
		got[1].Reason != ReasonUnknownName || got[0].At.IsZero() {
		t.Errorf("after a restart: %+v", got)
	}
}

// The file is bounded: past its size it is set aside once, replacing the
// one before, and what is read back is the last events.
func TestTheTraceFileIsBounded(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, TraceFile)
	tr := NewTrace(path)
	agent := strings.Repeat("A", traceAgentMax+50) // cut to traceAgentMax
	for i := 0; i < 8000; i++ {
		if err := tr.Record(SignInEvent{Event: EventSignedIn, User: "guest", From: fmt.Sprintf("203.0.113.%d", i%250), Agent: agent}); err != nil {
			t.Fatal(err)
		}
	}
	var total int64
	for _, p := range []string{path, path + ".1"} {
		st, err := os.Stat(p)
		if err != nil {
			t.Fatal(err)
		}
		total += st.Size()
	}
	if total > 2*traceFileMax+4096 {
		t.Errorf("%d bytes on disk for the trace, want two files of %d at most", total, traceFileMax)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 2 {
		t.Errorf("%d files in the directory, want the trace and the one before", len(entries))
	}
	got := NewTrace(path).Recent(0)
	if len(got) != traceKept || len(got[0].Agent) != traceAgentMax {
		t.Errorf("read back %d events, agent of %d; want %d and %d", len(got), len(got[0].Agent), traceKept, traceAgentMax)
	}
}

// Past the limit, a client's further tries are written once a minute: a
// flood of them must not push everything else out.
func TestRefusalsForTooManyAttemptsAreWrittenOnceAMinute(t *testing.T) {
	clock := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
	tr := NewTrace("")
	tr.now = func() time.Time { return clock }
	for i := 0; i < 100; i++ {
		_ = tr.Record(SignInEvent{Event: EventRefused, Reason: ReasonTooManyAttempts, From: "203.0.113.5"})
		clock = clock.Add(time.Second)
	}
	_ = tr.Record(SignInEvent{Event: EventRefused, Reason: ReasonTooManyAttempts, From: "203.0.113.6"})
	_ = tr.Record(SignInEvent{Event: EventRefused, Reason: ReasonWrongPassword, User: "pierre", From: "203.0.113.5"})
	got := tr.Recent(0)
	// 100 s of tries: at 0 and at 60 s; another client; a wrong password.
	if len(got) != 4 || got[0].Reason != ReasonWrongPassword || got[1].From != "203.0.113.6" {
		t.Errorf("%d events: %+v", len(got), got)
	}
}
