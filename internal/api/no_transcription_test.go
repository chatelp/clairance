package api

import (
	"net/http"
	"strings"
	"testing"
)

// A server started without transcription says so, refuses a source that asks
// for it while nothing answers, and keeps serving.
func TestAServerWithoutTranscriptionSaysSoAndRefusesASource(t *testing.T) {
	s := newStack(t, stackOptions{idle: true})
	s.signIn()

	var st struct {
		Transcription map[string]any `json:"transcription"`
	}
	s.get("/api/v1/status", &st)
	if st.Transcription["status"] != "off" || st.Transcription["reason"] != "no source transcribes" {
		t.Errorf("/status transcription = %v, want off, no source transcribes", st.Transcription)
	}
	var state ServerState
	s.get("/api/v1/server", &state)
	if tr := state.Transcription; tr == nil || tr.Status != "off" || tr.Detail != "no source transcribes" {
		t.Errorf("/server transcription = %+v, want off, no source transcribes", tr)
	}

	body := `{"name":"Approach","frequency_mhz":125.825,"url":"` + s.gone + `/a.mp3","transcribe_audio":true}`
	rec := s.do("PUT", "/api/v1/sources/125825", body)
	if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), "no local STT sidecar answers") {
		t.Fatalf("a transcribing source with no sidecar: %d %s, want 503 saying why", rec.Code, rec.Body.String())
	}
	listen := strings.Replace(body, `"transcribe_audio":true`, `"transcribe_audio":false`, 1)
	if rec := s.do("PUT", "/api/v1/sources/125825", listen); rec.Code != http.StatusOK {
		t.Fatalf("the same source without transcription: %d %s", rec.Code, rec.Body.String())
	}
	s.get("/api/v1/health", nil)
}

// Once a source has found a sidecar, the state is the sidecar's own.
func TestAServerWithoutTranscriptionTurnsItOnForASource(t *testing.T) {
	s := newStack(t, stackOptions{idle: true, sidecar: healthSaying(t, `{"status":"ok","degraded":[]}`)})
	s.signIn()
	body := `{"name":"Approach","frequency_mhz":125.825,"url":"` + s.gone + `/a.mp3","transcribe_audio":true}`
	if rec := s.do("PUT", "/api/v1/sources/125825", body); rec.Code != http.StatusOK {
		t.Fatalf("a transcribing source with a sidecar answering: %d %s", rec.Code, rec.Body.String())
	}
	var st struct {
		Transcription map[string]any `json:"transcription"`
	}
	s.get("/api/v1/status", &st)
	if st.Transcription["status"] != "ok" || st.Transcription["reachable"] != true {
		t.Errorf("/status transcription = %v, want ok, reachable", st.Transcription)
	}
}
