package api

import (
	"context"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/yegors/co-atc/internal/transcription"
)

// The receiver: the radio that feeds the audio streams, which co-atc does not
// run. Whatever does -- radio-ctl-sync for the owner's station -- reports its
// state here, so the page can say "lent" or "off" rather than show an empty
// bar. Kept in memory: a report is only worth its freshness.

// Receiver states, as the reporter maps its station's own modes onto them.
var receiverStates = map[string]bool{
	"on_air":    true, // listening, streams flowing
	"lent":      true, // the receiver is used for something else (exploration)
	"switching": true, // changing what it listens to
	"off":       true, // stopped
	"unknown":   true, // the reporter could not read the station
}

// A report older than this is shown as stale: the reporter has stopped.
const receiverStaleAfter = 60 * time.Second

// ReceiverReport is what the reporter says, in its station's words for the
// label, title and detail.
type ReceiverReport struct {
	State  string `json:"state"`
	Label  string `json:"label,omitempty"`  // the station's name for its state, e.g. "A l antenne"
	Title  string `json:"title,omitempty"`  // what it listens to, e.g. "Selection libre - 124,350 + 126,425"
	Since  string `json:"since,omitempty"`  // RFC 3339, when that began
	Mode   string `json:"mode,omitempty"`   // the station's own mode name
	Detail string `json:"detail,omitempty"` // anything else worth a line
}

type receiverState struct {
	ReceiverReport
	ReportedAt time.Time `json:"reported_at"`
	Stale      bool      `json:"stale"`
}

type statusCache struct {
	mu         sync.Mutex
	receiver   *ReceiverReport
	reportedAt time.Time

	sidecarAt      time.Time
	sidecar        transcription.Health
	sidecarReached bool
}

// PutReceiver takes the receiver's state from whatever runs it.
func (h *Handler) PutReceiver(w http.ResponseWriter, r *http.Request) {
	var rep ReceiverReport
	dec := json.NewDecoder(io.LimitReader(r.Body, 4096))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&rep); err != nil {
		http.Error(w, "invalid report: "+err.Error(), http.StatusBadRequest)
		return
	}
	rep.State = strings.TrimSpace(rep.State)
	if !receiverStates[rep.State] {
		http.Error(w, "invalid report: state must be on_air, lent, switching, off or unknown", http.StatusBadRequest)
		return
	}
	h.status.mu.Lock()
	h.status.receiver = &rep
	h.status.reportedAt = time.Now()
	h.status.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

// GetStatus is the state of what co-atc depends on, in one light answer for a
// banner polled by every open page: the receiver as last reported, the
// transcription sidecar (read at most every 15 s, it is busy decoding), the
// ADS-B feed and the audio archive.
func (h *Handler) GetStatus(w http.ResponseWriter, r *http.Request) {
	out := map[string]any{}

	out["receiver"] = h.receiverNow()

	if h.sttSidecar != nil && h.sttSidecar.Off() {
		// Nothing asks for transcription and nothing was started: not a
		// sidecar that is down, a server running without one.
		out["transcription"] = map[string]any{"status": "off", "reason": "no source transcribes"}
	} else if h.sttSidecar != nil {
		health, reached, _ := h.sidecarHealth(r.Context())
		state := health.Status
		if state == "" {
			state = "unknown"
		}
		// What answers: "mlx" or a GPU's name from the real sidecar, "fake"
		// from a test instance's stand-in, which the page must not show as a
		// working transcription.
		backend := strings.TrimSpace(health.Backend)
		if len(backend) > 48 {
			backend = ""
		}
		out["transcription"] = map[string]any{"status": state, "degraded": health.Degraded, "reachable": reached,
			"backend": backend, "model": ModelFamily(health.ModelEN)}
	} else {
		out["transcription"] = nil
	}

	if h.adsbService != nil {
		last, ok := h.adsbService.GetStatus()
		// In sight now: the day's database also holds those gone hours ago.
		active := 0
		for _, a := range h.adsbService.GetAllAircraft() {
			if a.Status == "active" {
				active++
			}
		}
		// messages_per_second: the receiver's own counter, a health figure
		// the aircraft count does not give; 0 when the source has none.
		adsb := map[string]any{"ok": ok, "last_fetch": last.UTC(), "aircraft": active,
			"messages_per_second": math.Round(h.adsbService.MessageRate())}
		// While every fetch fails: since when and why (14/10).
		if since, why := h.adsbService.FetchFailure(); !since.IsZero() {
			adsb["failing_since"] = since.UTC()
			adsb["failing_why"] = why
		}
		out["adsb"] = adsb
	}
	out["audio_archive"] = map[string]bool{"available": h.audioArchive.Available()}

	WriteJSON(w, http.StatusOK, out)
}

// receiverNow is the receiver as last reported, or nil before any report.
func (h *Handler) receiverNow() *receiverState {
	h.status.mu.Lock()
	defer h.status.mu.Unlock()
	if h.status.receiver == nil {
		return nil
	}
	return &receiverState{
		ReceiverReport: *h.status.receiver,
		ReportedAt:     h.status.reportedAt.UTC(),
		Stale:          time.Since(h.status.reportedAt) > receiverStaleAfter,
	}
}

// sidecarHealth is the sidecar's health, read again at most every 15 s -- it
// is busy decoding -- and shared by /status and /health/detail: whether it
// answered that reading, and when it was taken.
func (h *Handler) sidecarHealth(ctx context.Context) (transcription.Health, bool, time.Time) {
	h.status.mu.Lock()
	defer h.status.mu.Unlock()
	if time.Since(h.status.sidecarAt) > 15*time.Second {
		ctx, cancel := context.WithTimeout(ctx, 1500*time.Millisecond)
		health, err := h.sttSidecar.Refresh(ctx)
		cancel()
		h.status.sidecar, h.status.sidecarReached, h.status.sidecarAt = health, err == nil, time.Now()
	}
	return h.status.sidecar, h.status.sidecarReached, h.status.sidecarAt
}
