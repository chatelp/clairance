package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"

	cfg "github.com/yegors/co-atc/internal/config"
	"github.com/yegors/co-atc/internal/frequencies"
	"github.com/yegors/co-atc/pkg/logger"
)

// PutSource adds a frequency while the server runs, or replaces one added
// earlier. The same body sent twice changes nothing, so a caller can resend its
// whole list on every poll. See internal/frequencies/runtime_sources.go.
//
//	curl -X PUT -d '{"name":"CDG approach","frequency_mhz":125.825,
//	  "url":"http://audio.lan/aero-125825.mp3","language":"en",
//	  "transcribe_audio":true}' .../api/v1/sources/125825
//
// Behind authentication, like the other writes: this decides what the server
// connects to and what it transcribes.
func (h *Handler) PutSource(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name            string  `json:"name"`
		Airport         string  `json:"airport"`
		FrequencyMHz    float64 `json:"frequency_mhz"`
		URL             string  `json:"url"`
		Order           int     `json:"order"`
		TranscribeAudio bool    `json:"transcribe_audio"`
		Language        string  `json:"language"`
		SectorAirport   string  `json:"sector_airport"`
		SectorKind      string  `json:"sector_kind"`
		// Raw PCM over UDP only: its rate and channels, which nothing in the
		// packets says. See config.FrequencyConfig.
		PCMRate     int `json:"pcm_rate"`
		PCMChannels int `json:"pcm_channels"`
		// Accepted and ignored: radio-ctl-sync sent it for ffmpeg, removed in
		// B1 ter, and a body with a field this one does not know is refused.
		// The server reconnects a stream itself, after reconnect_interval_secs.
		FFmpegReconnect *bool `json:"ffmpeg_reconnect"`
	}
	dec := json.NewDecoder(io.LimitReader(r.Body, 4096))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil {
		http.Error(w, "invalid request: "+err.Error(), http.StatusBadRequest)
		return
	}

	id := chi.URLParam(r, "id")
	err := h.frequenciesService.AddSource(cfg.FrequencyConfig{
		ID:              id,
		Airport:         body.Airport,
		Name:            body.Name,
		FrequencyMHz:    body.FrequencyMHz,
		URL:             body.URL,
		Order:           body.Order,
		TranscribeAudio: body.TranscribeAudio,
		Language:        body.Language,
		SectorAirport:   body.SectorAirport,
		SectorKind:      body.SectorKind,
		PCMRate:         body.PCMRate,
		PCMChannels:     body.PCMChannels,
	})
	switch {
	case errors.Is(err, frequencies.ErrConfiguredSource):
		http.Error(w, err.Error(), http.StatusConflict)
		return
	case errors.Is(err, frequencies.ErrNoTranscription):
		h.logger.Warn("Source refused: it asks for transcription and no sidecar answers",
			logger.String("id", id), logger.Error(err))
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	case err != nil:
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	h.logger.Info("Source set from outside",
		logger.String("id", id), logger.String("url", body.URL))

	freq, _ := h.frequenciesService.GetFrequencyByID(id)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(freq)
}

// DeleteSource stops and forgets a frequency added by PutSource. Frequencies
// from the configuration cannot be removed this way.
func (h *Handler) DeleteSource(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	err := h.frequenciesService.RemoveSource(id)
	switch {
	case errors.Is(err, frequencies.ErrUnknownSource):
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	case errors.Is(err, frequencies.ErrConfiguredSource):
		http.Error(w, err.Error(), http.StatusConflict)
		return
	case err != nil:
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	h.logger.Info("Source removed from outside", logger.String("id", id))
	w.WriteHeader(http.StatusNoContent)
}
