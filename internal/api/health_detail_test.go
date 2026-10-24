package api

import (
	"net/http"
	"os/exec"
	"testing"
	"time"

	"github.com/yegors/co-atc/internal/frequencies"
)

// fromLoopback makes a request look as the launcher's does: from this
// machine, to 127.0.0.1 with the port, as URLSession and curl send it.
func fromLoopback(remote, host string) func(*http.Request) {
	return func(r *http.Request) {
		r.RemoteAddr = remote
		r.Host = host
	}
}

// This machine without a session, or the owner from anywhere; nobody else.
// A proxy on this machine connects from the loopback on someone else's
// behalf, and a page elsewhere that rebinds its name to 127.0.0.1 still sends
// its name: neither is this machine.
func TestTheHealthDetailIsForThisMachineOrTheOwner(t *testing.T) {
	s := newStack(t, stackOptions{viewer: true})
	cases := []struct {
		name string
		set  []func(*http.Request)
		want int
	}{
		{"from elsewhere, no session", nil, http.StatusForbidden},
		{"this machine, IPv4 with the port", []func(*http.Request){fromLoopback("127.0.0.1:50123", "127.0.0.1:8000")}, http.StatusOK},
		{"this machine, IPv6 with the port", []func(*http.Request){fromLoopback("[::1]:50123", "[::1]:8000")}, http.StatusOK},
		{"this machine, localhost", []func(*http.Request){fromLoopback("127.0.0.1:50123", "localhost:8000")}, http.StatusOK},
		{"this machine, no port", []func(*http.Request){fromLoopback("127.0.0.1:50123", "localhost")}, http.StatusOK},
		{"through a proxy on this machine", []func(*http.Request){fromLoopback("127.0.0.1:50123", "127.0.0.1:8000"),
			func(r *http.Request) { r.Header.Set("X-Forwarded-For", "100.64.0.7") }}, http.StatusForbidden},
		{"a proxy that says Forwarded", []func(*http.Request){fromLoopback("127.0.0.1:50123", "127.0.0.1:8000"),
			func(r *http.Request) { r.Header.Set("Forwarded", "for=100.64.0.7") }}, http.StatusForbidden},
		{"a rebound name", []func(*http.Request){fromLoopback("127.0.0.1:50123", "rebind.example:8000")}, http.StatusForbidden},
	}
	for _, c := range cases {
		if rec := s.do("GET", "/api/v1/health/detail", "", c.set...); rec.Code != c.want {
			t.Errorf("%s: %d, want %d (%s)", c.name, rec.Code, c.want, rec.Body.String())
		}
	}

	s.signInAs(viewerUser)
	if rec := s.do("GET", "/api/v1/health/detail", ""); rec.Code != http.StatusForbidden {
		t.Errorf("a viewer's session from elsewhere: %d, want 403", rec.Code)
	}
	s.signIn()
	if rec := s.do("GET", "/api/v1/health/detail", ""); rec.Code != http.StatusOK {
		t.Errorf("the owner's session from elsewhere: %d, want 200", rec.Code)
	}
}

const queuedHealth = `{"status":"ok","pid":4242,"backend":"fake","queue":{"first":2,"second":5},"second_pending":3,` +
	`"archiving":true,"archiving_stopped_low_disk":false,"models":{"en":{"id":"whisper-en","available":true}}}`

// What the launcher reads: the process, the sidecar as it says, the
// frequencies, the archive's cap and the databases; a figure not known is
// absent, not zero.
func TestTheHealthDetailSaysWhatTheLauncherWatches(t *testing.T) {
	s := newStack(t, stackOptions{sidecar: healthSaying(t, queuedHealth)})
	s.cfg.Transcription.AudioArchiveMaxGB = 60

	var d HealthDetail
	rec := s.do("GET", "/api/v1/health/detail", "", fromLoopback("127.0.0.1:50123", "127.0.0.1:8000"))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/health/detail: %d %s", rec.Code, rec.Body.String())
	}
	decode(t, rec, &d)

	if d.PID == 0 || d.UptimeSeconds < 0 {
		t.Errorf("the process: pid %d, uptime %d", d.PID, d.UptimeSeconds)
	}
	sc := d.Sidecar
	if sc == nil || !sc.Reachable || sc.Status != "ok" || sc.PID != 4242 || sc.Queue == nil ||
		sc.Queue.First != 2 || sc.Queue.Second != 5 || sc.SecondPending != 3 || !sc.Archiving || sc.ReadAt == nil {
		t.Errorf("the sidecar: %+v", sc)
	}
	if d.ADSB == nil {
		t.Error("no ADS-B block")
	}
	if d.Frequencies == nil {
		t.Error("frequencies is null, want a list")
	}
	if d.Receiver != nil {
		t.Errorf("a receiver before any report: %+v", d.Receiver)
	}
	if d.AudioArchive.CapBytes != 60<<30 || d.AudioArchive.Bytes != nil || d.AudioArchive.MeasuredAt != nil {
		t.Errorf("the archive before any purge: %+v", d.AudioArchive)
	}
	if d.Databases.DirBytes == nil || *d.Databases.DirBytes == 0 || d.Databases.DiskFreeBytes == nil {
		t.Errorf("the databases: %+v", d.Databases)
	}
}

// The bytes out of ffmpeg, on a real ffmpeg reading a tone at its own pace:
// what tells a frequency that hears from one that only says "connected".
func TestTheHealthDetailCountsTheAudioThatArrives(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	s := newStack(t, stackOptions{tone: true})
	deadline := time.Now().Add(10 * time.Second)
	var f frequencies.FrequencyHealth
	for time.Now().Before(deadline) {
		for _, h := range s.freqs.Health() {
			if h.ID == "tone" {
				f = h
			}
		}
		if f.BytesLast60s >= 3*32000 {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if f.BytesLast60s < 3*32000 || f.LastByteAt == nil || time.Since(*f.LastByteAt) > 2*time.Second {
		t.Fatalf("the tone after a few seconds: %+v", f)
	}
	if f.Status != "connected" || f.LastSegmentAt != nil || f.LastTranscribedAt != nil {
		t.Errorf("a tone that transcribes nothing: %+v", f)
	}
	t.Logf("tone: %d bytes in the last minute, last at %v", f.BytesLast60s, f.LastByteAt)
}
