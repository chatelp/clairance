package lifecycle

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// A line is one JSON object, which begins with the event, ends the line, and
// carries the time and the fields.
func TestALineBeginsWithItsEvent(t *testing.T) {
	at := time.Date(2026, 10, 26, 17, 2, 6, 0, time.FixedZone("CET", 3600))
	b := Line(Fatal, map[string]any{"cause": "adsb_source", "error": "dial tcp: no route to host"}, at)
	if !bytes.HasPrefix(b, []byte(`{"coatc_status":"fatal",`)) || !bytes.HasSuffix(b, []byte("}\n")) || bytes.Count(b, []byte("\n")) != 1 {
		t.Fatalf("line: %q", b)
	}
	var got map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"coatc_status": "fatal", "at": "2026-10-26T16:02:06Z", "cause": "adsb_source", "error": "dial tcp: no route to host"}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %v, want %v", k, got[k], v)
		}
	}
	if len(got) != len(want) {
		t.Errorf("fields: %v", got)
	}
	if b := Line(Stopped, nil, at); string(b) != `{"coatc_status":"stopped","at":"2026-10-26T16:02:06Z"}`+"\n" {
		t.Errorf("no fields: %q", b)
	}
}

// Nothing is said before Open, and everything after, on the descriptor given.
func TestEventsGoWhereOpenSaid(t *testing.T) {
	defer func() { mu.Lock(); out = nil; mu.Unlock() }()
	Say(Ready, map[string]any{"pid": 1}) // nowhere: no -status-fd
	var buf bytes.Buffer
	mu.Lock()
	out = &buf
	mu.Unlock()
	Say(Starting, map[string]any{"pid": 4242})
	Failed("config", errString("invalid server port: 0"))
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 2 || !strings.Contains(lines[0], `"pid":4242`) || !strings.Contains(lines[1], `"cause":"config"`) {
		t.Errorf("lines: %q", lines)
	}
	if err := Open(-1); err == nil {
		t.Error("a negative descriptor was accepted")
	}
	if err := Open(987); err == nil {
		t.Error("a descriptor nobody opened was accepted")
	}
}

type errString string

func (e errString) Error() string { return string(e) }
