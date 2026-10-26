package api

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/yegors/co-atc/internal/frequencies"
	"github.com/yegors/co-atc/pkg/logger"
)

// GetAllFrequencies returns all frequencies with recent transcriptions
func (h *Handler) GetAllFrequencies(w http.ResponseWriter, r *http.Request) {
	// Get all frequencies
	frequencies := h.frequenciesService.GetAllFrequencies()
	for _, f := range frequencies {
		h.streamPortFor(r, f)
	}

	// Fetch last 100 transcriptions per frequency
	transcriptionsByFreq := make(map[string]interface{})
	if h.transcriptionStorage != nil {
		for _, freq := range frequencies {
			txns, err := h.transcriptionStorage.GetTranscriptionsByFrequency(freq.ID, 100, 0)
			if err != nil {
				h.logger.Error("Failed to fetch transcriptions for frequency",
					logger.String("frequency_id", freq.ID),
					logger.Error(err))
				continue
			}
			if len(txns) > 0 {
				explainTranscriptions(txns)
				transcriptionsByFreq[freq.ID] = txns
			}
		}
	}

	// Create response
	response := map[string]interface{}{
		"timestamp":      time.Now().UTC(),
		"count":          len(frequencies),
		"frequencies":    frequencies,
		"transcriptions": transcriptionsByFreq,
	}

	// Write response
	WriteJSON(w, http.StatusOK, response)
}

// GetFrequencyByID returns a frequency by its ID
func (h *Handler) GetFrequencyByID(w http.ResponseWriter, r *http.Request) {
	// Get frequency ID from URL
	id := chi.URLParam(r, "id")
	if id == "" {
		http.Error(w, "Missing frequency ID", http.StatusBadRequest)
		return
	}

	// Get frequency data
	frequency, found := h.frequenciesService.GetFrequencyByID(id)
	if !found {
		http.Error(w, "Frequency not found", http.StatusNotFound)
		return
	}
	h.streamPortFor(r, frequency)

	// Write response
	WriteJSON(w, http.StatusOK, frequency)
}

// streamPortFor drops the stream's port for a browser behind a trusted
// proxy. The streams are spread over the server's additional ports so that a
// browser's six connections per host are not taken by long-lived streams; a
// browser behind a proxy reaches only the address the proxy serves (on 26/10,
// the owner's Traefik, for a friend's remote access), and asks for the stream
// there. Each call builds the frequencies afresh, so the change is the
// caller's own.
func (h *Handler) streamPortFor(r *http.Request, f *frequencies.Frequency) {
	if h.auth != nil && h.auth.Proxies().Forwarded(r) {
		f.StreamPort = 0
	}
}
