package api

import (
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/yegors/co-atc/pkg/logger"
)

// StreamAudio streams audio for a frequency
func (h *Handler) StreamAudio(w http.ResponseWriter, r *http.Request) {
	// Get frequency ID from URL
	id := chi.URLParam(r, "id")
	if id == "" {
		http.Error(w, "Missing frequency ID", http.StatusBadRequest)
		return
	}

	// Get client ID from query parameter
	clientID := r.URL.Query().Get("id")
	if clientID == "" {
		// Generate a random client ID if not provided
		clientID = fmt.Sprintf("client-%d", time.Now().UnixNano())
	}

	clientRemoteAddr := r.RemoteAddr

	// A stream the owner can end by cutting its account off (D83).
	r, done := h.cuttable(r)
	defer done()

	// A guest holds two streams of a frequency at most, in any format
	// (guest.go); a HEAD opens none.
	if r.Method != http.MethodHead {
		release, ok := h.guestStream(r, id)
		if !ok {
			h.logger.Warn("Refused a guest's stream: two are open already",
				logger.String("id", id), logger.String("remote_addr", clientRemoteAddr))
			WriteJSON(w, http.StatusTooManyRequests, map[string]string{"error": fmt.Sprintf(
				"%d streams of this frequency are open for this account, the most it may hold: close another page or tab that plays it",
				guestStreamsPerFrequency)})
			return
		}
		defer release()
	}

	// The station's own MP3 when the page asks for it and the stream has it:
	// what every browser plays, Safari included, at a twenty-fifth of the WAV's
	// rate (docs-fr/35-plan.md, D2 bis). Otherwise the WAV stream, as before.
	switch r.URL.Query().Get("format") {
	case "mp3":
		if mp3, ok := h.frequenciesService.MP3Stream(id); ok {
			h.streamRelayed(w, r, id, mp3, "audio/mpeg", 0)
			return
		}
	case "ulaw":
		// The decoded sound in mu-law, for a page that plays it itself
		// through Web Audio (www/live-player.js; D2 ter).
		if ulaw, rate, ok := h.frequenciesService.ULawStream(id); ok {
			h.streamRelayed(w, r, id, ulaw, "audio/basic", rate)
			return
		}
	}

	// Set binary streaming headers
	w.Header().Set("Content-Type", "audio/wav")
	w.Header().Set("Cache-Control", "no-cache, no-store")
	w.Header().Set("Transfer-Encoding", "chunked")
	// No "Access-Control-Allow-Origin: *" here: the CORS middleware has already
	// named the page's origin and allowed credentials, which is what a stream
	// fetched with its session cookie from another port needs -- a browser
	// refuses "*" on a request that carries credentials.
	w.Header().Set("Connection", "keep-alive")                // Add keep-alive
	w.Header().Set("Keep-Alive", "timeout=86400, max=604800") // Add keep-alive timeout

	// For HEAD requests, just return the headers
	if r.Method == "HEAD" {
		return
	}

	// Use the request's context directly - it will be canceled when the client disconnects
	ctx := r.Context()

	h.logger.Debug("Client requesting audio stream",
		logger.String("id", id),
		logger.String("client_id", clientID),
		logger.String("remote_addr", clientRemoteAddr))

	// Get audio stream with client ID
	stream, contentType, err := h.frequenciesService.GetAudioStream(ctx, id, clientID)
	if err != nil {
		// Check if the error is due to client already being connected
		if strings.Contains(err.Error(), "client already connected") {
			h.logger.Debug("Client already connected, returning success",
				logger.String("id", id),
				logger.String("client_id", clientID),
				logger.String("remote_addr", clientRemoteAddr))

			// Return a minimal response indicating the client is already connected
			w.Header().Set("X-Already-Connected", "true")
			w.WriteHeader(http.StatusOK)
			return
		}

		h.logger.Error("Failed to get audio stream",
			logger.String("id", id),
			logger.String("client_id", clientID),
			logger.String("remote_addr", clientRemoteAddr),
			logger.Error(err),
		)
		http.Error(w, "Stream unavailable", http.StatusServiceUnavailable)
		return
	}
	defer stream.Close() // Crucial: Ensures ClientStreamReader.Close() is called

	h.logger.Debug("Client connected to audio stream",
		logger.String("id", id),
		logger.String("client_id", clientID),
		logger.String("remote_addr", clientRemoteAddr),
		logger.String("content_type", contentType),
	)

	// Connection monitoring setup
	connectionStartTime := time.Now()

	// Use a buffer to improve performance
	buf := make([]byte, 4096)

	// Track consecutive errors for client disconnect detection
	consecutiveErrors := 0
	bytesWritten := 0

	// Stream data to client
	lastProgressLog := time.Now()
	for {
		// Check if client has disconnected
		select {
		case <-ctx.Done():
			h.logger.Info("Client context done, stopping stream",
				logger.String("id", id),
				logger.String("client_id", clientID),
				logger.String("remote_addr", clientRemoteAddr),
				logger.String("reason", ctx.Err().Error()),
				logger.Int("total_bytes_written", bytesWritten),
				logger.String("connection_duration", time.Since(connectionStartTime).String()),
			)
			return
		default:
			// Continue streaming
		}

		// Read from stream - this will timeout after 5 seconds if no data
		n, err := stream.Read(buf)

		if err != nil {
			if err == io.EOF {
				h.logger.Warn("Stream EOF reached unexpectedly",
					logger.String("id", id),
					logger.String("client_id", clientID),
					logger.Int("bytes_written_before_eof", bytesWritten),
					logger.String("connection_duration", time.Since(connectionStartTime).String()))
				return
			}

			h.logger.Warn("Error reading from stream",
				logger.String("id", id),
				logger.String("client_id", clientID),
				logger.String("error_type", fmt.Sprintf("%T", err)),
				logger.Error(err),
				logger.Int("consecutive_errors", consecutiveErrors+1))

			consecutiveErrors++
			if consecutiveErrors > 3 {
				h.logger.Error("Too many consecutive read errors, closing stream",
					logger.String("id", id),
					logger.String("client_id", clientID),
					logger.Int("total_consecutive_errors", consecutiveErrors),
					logger.Int("bytes_written_before_failure", bytesWritten),
					logger.String("connection_duration", time.Since(connectionStartTime).String()))
				return
			}

			// Brief pause before retrying
			time.Sleep(100 * time.Millisecond)
			continue
		}

		// Reset error counter on successful read
		consecutiveErrors = 0

		// If we got data, write it to the client
		if n > 0 {
			_, err = w.Write(buf[:n])
			if err != nil {
				h.logger.Warn("Error writing to client, closing stream",
					logger.String("id", id),
					logger.String("client_id", clientID),
					logger.String("error_type", fmt.Sprintf("%T", err)),
					logger.Error(err),
					logger.Int("bytes_written_before_error", bytesWritten),
					logger.String("connection_duration", time.Since(connectionStartTime).String()))
				return
			}

			bytesWritten += n

			// Flush data immediately
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}

			// Log every 100KB of data or every 60 seconds, whichever comes first
			if bytesWritten%102400 < n || time.Since(lastProgressLog) > 60*time.Second {
				h.logger.Debug("Streaming progress",
					logger.String("id", id),
					logger.String("client_id", clientID),
					logger.Int("bytes_written", bytesWritten),
					logger.String("connection_duration", time.Since(connectionStartTime).String()))
				lastProgressLog = time.Now()
			}
		}
	}
}

// streamRelayed sends a frequency's relayed sound (MP3 frames, or mu-law
// with its rate in X-Sample-Rate) until the browser leaves or the stream
// stops, flushing each read so that the sound is not held back.
func (h *Handler) streamRelayed(w http.ResponseWriter, r *http.Request, id string, mp3 io.ReadCloser, contentType string, rate int) {
	defer mp3.Close()
	w.Header().Set("Content-Type", contentType)
	if rate > 0 {
		w.Header().Set("X-Sample-Rate", strconv.Itoa(rate))
	}
	w.Header().Set("Cache-Control", "no-cache, no-store")
	if r.Method == http.MethodHead {
		return
	}
	go func() {
		<-r.Context().Done()
		mp3.Close() // ends the copy below when the browser leaves
	}()
	flusher, _ := w.(http.Flusher)
	buf := make([]byte, 4096)
	start, written := time.Now(), 0
	for {
		n, err := mp3.Read(buf)
		if n > 0 {
			if _, werr := w.Write(buf[:n]); werr != nil {
				break
			}
			written += n
			if flusher != nil {
				flusher.Flush()
			}
		}
		if err != nil {
			break
		}
	}
	h.logger.Info("Relayed stream ended", logger.String("id", id), logger.String("type", contentType), logger.String("remote_addr", r.RemoteAddr),
		logger.Int("bytes", written), logger.String("duration", time.Since(start).Round(time.Second).String()))
}
