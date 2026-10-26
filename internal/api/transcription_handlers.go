package api

import (
	"fmt"
	"github.com/yegors/co-atc/internal/transcription"
	"github.com/yegors/co-atc/internal/transcription/phraseology"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/yegors/co-atc/internal/storage/sqlite"
	"github.com/yegors/co-atc/pkg/logger"
)

// HandleWebSocket handles WebSocket connections
func (h *Handler) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	h.logger.Info("WebSocket connection request received")

	// Handle the WebSocket connection; a guest's page is not sent what a
	// guest is not told (guest.go).
	if h.guest(r) {
		h.wsServer.HandleConnectionFiltered(w, r, guestMessage)
		return
	}
	h.wsServer.HandleConnection(w, r)
}

// GetAllTranscriptions returns all transcriptions with pagination
func (h *Handler) GetAllTranscriptions(w http.ResponseWriter, r *http.Request) {
	// Parse pagination parameters
	limit, offset := parsePaginationParams(r)

	// Get transcriptions from storage
	transcriptions, err := h.transcriptionStorage.GetTranscriptions(limit, offset)
	if err != nil {
		h.logger.Error("Failed to retrieve transcriptions", logger.Error(err))
		http.Error(w, "Failed to retrieve transcriptions", http.StatusInternalServerError)
		return
	}
	explainTranscriptions(transcriptions)

	// Create response
	response := map[string]interface{}{
		"timestamp":      time.Now(),
		"count":          len(transcriptions),
		"transcriptions": transcriptions,
	}

	// Write response
	WriteJSON(w, http.StatusOK, response)
}

// GetTranscriptionsByFrequency returns transcriptions for a specific frequency
func (h *Handler) GetTranscriptionsByFrequency(w http.ResponseWriter, r *http.Request) {
	// Get frequency ID from URL
	id := chi.URLParam(r, "id")
	if id == "" {
		http.Error(w, "Missing frequency ID", http.StatusBadRequest)
		return
	}

	// Parse pagination parameters
	limit, offset := parsePaginationParams(r)

	// Get transcriptions from storage
	transcriptions, err := h.transcriptionStorage.GetTranscriptionsByFrequency(id, limit, offset)
	if err != nil {
		h.logger.Error("Failed to retrieve transcriptions by frequency", logger.Error(err))
		http.Error(w, "Failed to retrieve transcriptions", http.StatusInternalServerError)
		return
	}
	explainTranscriptions(transcriptions)

	// Create response
	response := map[string]interface{}{
		"timestamp":      time.Now(),
		"frequency_id":   id,
		"count":          len(transcriptions),
		"transcriptions": transcriptions,
	}

	// Write response
	WriteJSON(w, http.StatusOK, response)
}

// GetTranscriptionsByTimeRange returns transcriptions within a time range
func (h *Handler) GetTranscriptionsByTimeRange(w http.ResponseWriter, r *http.Request) {
	// Parse time range parameters
	startTime, endTime, err := parseTimeRangeParams(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Parse pagination parameters
	limit, offset := parsePaginationParams(r)

	// Get transcriptions from storage
	transcriptions, err := h.transcriptionStorage.GetTranscriptionsByTimeRange(startTime, endTime, limit, offset)
	if err != nil {
		h.logger.Error("Failed to retrieve transcriptions by time range", logger.Error(err))
		http.Error(w, "Failed to retrieve transcriptions", http.StatusInternalServerError)
		return
	}
	explainTranscriptions(transcriptions)

	// Create response
	response := map[string]interface{}{
		"timestamp":      time.Now(),
		"start_time":     startTime,
		"end_time":       endTime,
		"count":          len(transcriptions),
		"transcriptions": transcriptions,
	}

	// Write response
	WriteJSON(w, http.StatusOK, response)
}

// GetTranscriptionsBySpeaker returns transcriptions by speaker type
func (h *Handler) GetTranscriptionsBySpeaker(w http.ResponseWriter, r *http.Request) {
	// Get speaker type from URL
	speakerType := chi.URLParam(r, "type")
	if speakerType == "" {
		http.Error(w, "Missing speaker type", http.StatusBadRequest)
		return
	}

	// Validate speaker type
	if speakerType != "ATC" && speakerType != "PILOT" {
		http.Error(w, "Invalid speaker type (must be 'ATC' or 'PILOT')", http.StatusBadRequest)
		return
	}

	// Parse pagination parameters
	limit, offset := parsePaginationParams(r)

	// Get transcriptions from storage
	transcriptions, err := h.transcriptionStorage.GetTranscriptionsBySpeaker(speakerType, limit, offset)
	if err != nil {
		h.logger.Error("Failed to retrieve transcriptions by speaker", logger.Error(err))
		http.Error(w, "Failed to retrieve transcriptions", http.StatusInternalServerError)
		return
	}
	explainTranscriptions(transcriptions)

	// Create response
	response := map[string]interface{}{
		"timestamp":      time.Now(),
		"speaker_type":   speakerType,
		"count":          len(transcriptions),
		"transcriptions": transcriptions,
	}

	// Write response
	WriteJSON(w, http.StatusOK, response)
}

// GetTranscriptionsByCallsign returns transcriptions by aircraft callsign
func (h *Handler) GetTranscriptionsByCallsign(w http.ResponseWriter, r *http.Request) {
	// Get callsign from URL
	callsign := chi.URLParam(r, "callsign")
	if callsign == "" {
		http.Error(w, "Missing callsign", http.StatusBadRequest)
		return
	}

	// Parse pagination parameters
	limit, offset := parsePaginationParams(r)

	// Get transcriptions from storage
	transcriptions, err := h.transcriptionStorage.GetTranscriptionsByCallsign(callsign, limit, offset)
	if err != nil {
		h.logger.Error("Failed to retrieve transcriptions by callsign", logger.Error(err))
		http.Error(w, "Failed to retrieve transcriptions", http.StatusInternalServerError)
		return
	}
	explainTranscriptions(transcriptions)

	// Attach what the grammar recovered from each of them -- levels, headings,
	// runways, squawks. One query for the page, not one per transcription.
	values := map[int64][]sqlite.PhraseologyValue{}
	if h.valueStorage != nil && len(transcriptions) > 0 {
		ids := make([]int64, 0, len(transcriptions))
		for _, t := range transcriptions {
			ids = append(ids, t.ID)
		}
		var err error
		if values, err = h.valueStorage.ValuesByTranscription(ids); err != nil {
			h.logger.Error("Failed to read phraseology values", logger.Error(err))
			values = map[int64][]sqlite.PhraseologyValue{}
		}
	}

	// Create response
	response := map[string]interface{}{
		"timestamp":      time.Now(),
		"callsign":       callsign,
		"count":          len(transcriptions),
		"transcriptions": transcriptions,
		"values":         values,
	}

	// Write response
	WriteJSON(w, http.StatusOK, response)
}

// Helper functions
func parsePaginationParams(r *http.Request) (int, int) {
	limit := 100 // Default limit
	offset := 0  // Default offset

	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	if offsetStr := r.URL.Query().Get("offset"); offsetStr != "" {
		if o, err := strconv.Atoi(offsetStr); err == nil && o >= 0 {
			offset = o
		}
	}

	return limit, offset
}

func parseTimeRangeParams(r *http.Request) (time.Time, time.Time, error) {
	startTimeStr := r.URL.Query().Get("start_time")
	endTimeStr := r.URL.Query().Get("end_time")

	if startTimeStr == "" {
		return time.Time{}, time.Time{}, fmt.Errorf("missing start_time parameter")
	}

	startTime, err := time.Parse(time.RFC3339, startTimeStr)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("invalid start_time format (use RFC3339)")
	}

	endTime := time.Now()
	if endTimeStr != "" {
		endTime, err = time.Parse(time.RFC3339, endTimeStr)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("invalid end_time format (use RFC3339)")
		}
	}

	return startTime, endTime, nil
}

// explainTranscriptions adds what each transmission says in plain words. The
// grammar is deterministic and cheap -- 17 µs a transmission, measured on the
// 5 512 of 04/10 -- so it is read again when served rather than stored.
func explainTranscriptions(list []*sqlite.TranscriptionRecord) {
	for _, t := range list {
		if t == nil {
			continue
		}
		t.Plain = plainOf(t.Content)
		t.Places = transcription.PlacesOf(t.Content)
	}
}

func plainOf(content string) []sqlite.PlainInstruction {
	var out []sqlite.PlainInstruction
	for _, in := range phraseology.Parse(content).Plain() {
		out = append(out, sqlite.PlainInstruction{Role: in.Role, Text: in.Text, TextFR: in.TextFR,
			Value: in.Value, Unit: in.Unit})
	}
	return out
}
