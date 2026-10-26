package frequencies

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/yegors/co-atc/internal/audio"
	cfg "github.com/yegors/co-atc/internal/config"
	"github.com/yegors/co-atc/internal/storage/sqlite"
	"github.com/yegors/co-atc/internal/transcription"
	"github.com/yegors/co-atc/internal/websocket"
	"github.com/yegors/co-atc/pkg/logger"
)

// Import the logger package's exported functions
var (
	String = logger.String
	Int    = logger.Int
	Error  = logger.Error
	Bool   = logger.Bool
)

// StreamProcessor manages a single frequency stream that can be shared among multiple clients.
type StreamProcessor struct {
	id                string
	audioProcessor    *audio.CentralAudioProcessor
	contentType       string
	status            string
	lastActivity      time.Time
	clients           map[string]*ClientStreamReader
	clientsMu         sync.RWMutex
	audioURL          string
	client            *Client
	ctx               context.Context
	cancel            context.CancelFunc
	logger            *logger.Logger
	clientLastActive  map[string]time.Time // Track when each client was last active
	clientCleanupTick *time.Ticker         // Ticker for cleaning up inactive clients
}

// NewStreamProcessor creates a new stream processor for a frequency.
func NewStreamProcessor(
	ctx context.Context,
	id string,
	audioURL string,
	ffmpegReconnect bool,
	inputOptions []string,
	client *Client,
	config *cfg.Config,
	logger *logger.Logger,
) (*StreamProcessor, error) {
	procCtx, procCancel := context.WithCancel(ctx)

	// Create audio processor
	audioConfig := audio.CentralProcessorConfig{
		FFmpegPath:               config.Transcription.FFmpegPath,
		SampleRate:               config.Transcription.FFmpegSampleRate,
		Channels:                 config.Transcription.FFmpegChannels,
		Format:                   config.Transcription.FFmpegFormat,
		ReconnectDelay:           time.Duration(config.Frequencies.ReconnectIntervalSecs) * time.Second,
		FFmpegTimeoutSecs:        config.Frequencies.FFmpegTimeoutSecs,
		FFmpegReconnectDelaySecs: config.Frequencies.FFmpegReconnectDelaySecs,
		NoFFmpegReconnect:        !ffmpegReconnect,
		InputOptions:             inputOptions,
		GoDecoder:                config.Frequencies.Decoder == "go",
	}

	audioProcessor, err := audio.NewCentralAudioProcessor(
		procCtx,
		id,
		audioURL,
		audioConfig,
		logger.Named("audio"),
	)
	if err != nil {
		procCancel()
		return nil, fmt.Errorf("failed to create audio processor: %w", err)
	}

	sp := &StreamProcessor{
		id:               id,
		audioProcessor:   audioProcessor,
		contentType:      "audio/wav", // We're now serving WAV format
		status:           "initializing",
		lastActivity:     time.Now(),
		clients:          make(map[string]*ClientStreamReader),
		clientLastActive: make(map[string]time.Time),
		clientsMu:        sync.RWMutex{},
		audioURL:         audioURL,
		client:           client,
		ctx:              procCtx,
		cancel:           procCancel,
		logger:           logger.Named("freq-stream").With(String("id", id)),
	}

	// Start a ticker to clean up inactive clients every 10 seconds
	sp.clientCleanupTick = time.NewTicker(10 * time.Second)
	go sp.cleanupInactiveClients()

	return sp, nil
}

// cleanupInactiveClients periodically checks for and removes inactive clients
func (sp *StreamProcessor) cleanupInactiveClients() {
	for {
		select {
		case <-sp.ctx.Done():
			// Stop the cleanup when the processor is stopped
			if sp.clientCleanupTick != nil {
				sp.clientCleanupTick.Stop()
			}
			return
		case <-sp.clientCleanupTick.C:
			sp.removeInactiveClients()
		}
	}
}

// removeInactiveClients removes clients that haven't been active for more than 30 seconds
func (sp *StreamProcessor) removeInactiveClients() {
	sp.clientsMu.Lock()
	defer sp.clientsMu.Unlock()

	now := time.Now()
	inactiveThreshold := 30 * time.Second
	inactiveClients := []string{}

	// Log current client state for debugging
	if len(sp.clients) > 0 {
		sp.logger.Debug("Client activity check",
			Int("total_clients", len(sp.clients)),
			String("threshold", inactiveThreshold.String()))
	}

	// Find inactive clients
	for clientID, lastActive := range sp.clientLastActive {
		inactiveDuration := now.Sub(lastActive)
		if inactiveDuration > inactiveThreshold {
			inactiveClients = append(inactiveClients, clientID)
			sp.logger.Warn("Client marked as inactive due to timeout",
				String("clientID", clientID),
				String("inactive_duration", inactiveDuration.String()),
				String("threshold", inactiveThreshold.String()))
		}
	}

	// Also check for clients with closed readers but still in the map
	for clientID, reader := range sp.clients {
		if reader != nil {
			reader.mu.Lock()
			isClosed := reader.closed
			reader.mu.Unlock()

			if isClosed {
				// Reader is closed but still in map, add to cleanup list
				if !contains(inactiveClients, clientID) {
					inactiveClients = append(inactiveClients, clientID)
					sp.logger.Warn("Found closed client reader still in map, marking for cleanup",
						String("clientID", clientID))
				}
			}
		}
	}

	// Remove inactive clients
	for _, clientID := range inactiveClients {
		lastActive, hasLastActive := sp.clientLastActive[clientID]
		var inactiveDuration time.Duration
		if hasLastActive {
			inactiveDuration = now.Sub(lastActive)
		}

		sp.logger.Warn("Removing inactive client",
			String("clientID", clientID),
			String("inactive_duration", inactiveDuration.String()),
			Bool("had_last_active", hasLastActive))

		if reader, exists := sp.clients[clientID]; exists {
			reader.Close()
			delete(sp.clients, clientID)
		}
		delete(sp.clientLastActive, clientID)
		// The shared reader goes too: left behind, it kept its position, and
		// every write woke it for nothing.
		sp.audioProcessor.RemoveReader(clientID)
	}

	if len(inactiveClients) > 0 {
		sp.logger.Warn("Removed inactive clients",
			Int("count", len(inactiveClients)),
			Int("remaining", len(sp.clients)))
	}
}

// Helper function to check if slice contains string
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// Start begins processing the audio stream.
func (sp *StreamProcessor) Start() error {
	sp.logger.Info("Starting stream processor")

	// Start the audio processor
	if err := sp.audioProcessor.Start(); err != nil {
		return fmt.Errorf("failed to start audio processor: %w", err)
	}

	sp.status = "streaming"
	sp.lastActivity = time.Now()

	// The transcription will be started by the Service.Start method
	// based on the actual TranscribeAudio configuration value

	return nil
}

// processStream method has been removed as it's no longer needed
// The audio processor now handles all streaming functionality

// Stop stops the stream processor and cleans up resources.
func (sp *StreamProcessor) Stop() {
	sp.logger.Info("Stopping stream processor")

	// Cancel the context to stop all operations
	sp.cancel()

	// Stop the client cleanup ticker
	if sp.clientCleanupTick != nil {
		sp.clientCleanupTick.Stop()
	}

	// Force close all client connections immediately
	sp.clientsMu.Lock()
	// First, call Close on all readers. This cancels their contexts.
	for clientID, reader := range sp.clients {
		sp.logger.Info("Closing client connection during shutdown",
			String("clientID", clientID))
		if reader != nil {
			reader.Close() // This is the new ClientStreamReader.Close(), it no longer calls RemoveClient.
		}
	}
	// Now that all readers are marked closed and their contexts cancelled,
	// clear the maps.
	sp.clients = make(map[string]*ClientStreamReader)
	sp.clientLastActive = make(map[string]time.Time)
	sp.clientsMu.Unlock()

	// Stop the audio processor
	if sp.audioProcessor != nil {
		if err := sp.audioProcessor.Stop(); err != nil {
			sp.logger.Error("Error stopping audio processor", Error(err))
		}
	}

	sp.logger.Info("Stream processor stopped")
}

// AddClient adds a new client to the stream processor.
func (sp *StreamProcessor) AddClient(clientID string) *ClientStreamReader {
	sp.clientsMu.Lock()
	defer sp.clientsMu.Unlock()

	// Check if client already exists
	if existingReader, exists := sp.clients[clientID]; exists {
		// Check if the existing reader is actually closed.
		// Accessing existingReader.closed requires its own mutex if it's frequently contended,
		// but here we hold sp.clientsMu, providing some protection. A specific check is safer.
		existingReader.mu.Lock() // Lock the specific reader to check its closed status
		isClosed := existingReader.closed
		existingReader.mu.Unlock()

		if !isClosed {
			sp.logger.Info("Client already connected and active, updating last activity time",
				String("clientID", clientID))
			// Update last activity time
			sp.clientLastActive[clientID] = time.Now()
			return existingReader // Return the active, existing reader
		} else {
			sp.logger.Info("Found existing but closed client reader, removing it before creating a new one",
				String("clientID", clientID))
			// The reader.Close() should have already canceled its context.
			// We just need to remove it from the maps here.
			delete(sp.clients, clientID)
			delete(sp.clientLastActive, clientID)
			// Proceed to create a new reader for this clientID
		}
	}

	sp.logger.Info("Adding new client (or replacing closed one)", String("clientID", clientID))

	// A page keeps one client id for its life. Asking again after its stream
	// ended, it was handed its old shared reader back, still at the position it
	// stopped at: up to the whole buffer behind live (87 s at 24 kHz; 2 s
	// measured after a 1.5 s gap). A new client starts at live.
	sp.audioProcessor.RemoveReader(clientID)

	// Create a reader from the audio processor
	audioReader, err := sp.audioProcessor.CreateReader(clientID)
	if err != nil {
		sp.logger.Error("Failed to create audio reader", Error(err), String("clientID", clientID))
		// Return a dummy reader that will return EOF
		return &ClientStreamReader{
			ReadCloser:   io.NopCloser(strings.NewReader("")),
			logger:       sp.logger.Named("client-stream-reader"),
			streamID:     sp.id,
			once:         sync.Once{},
			processor:    sp,
			clientID:     clientID,
			lastActivity: time.Now(),
			ctx:          context.Background(),
			cancel:       func() {},
			closed:       true,
		}
	}

	// Create a non-closing reader with processor and clientID
	nonClosingReader := &NonClosingReader{
		ReadCloser: audioReader,
		processor:  sp,
		clientID:   clientID,
	}

	// Create a context with cancel for this client
	ctx, cancel := context.WithCancel(sp.ctx)

	// Create a new reader that reads from the audio processor
	reader := &ClientStreamReader{
		ReadCloser:   nonClosingReader,
		logger:       sp.logger.Named("client-stream-reader"),
		streamID:     sp.id,
		once:         sync.Once{},
		processor:    sp,
		clientID:     clientID,
		lastActivity: time.Now(),
		ctx:          ctx,
		cancel:       cancel,
	}

	// Store the client and track activity time
	sp.clients[clientID] = reader
	sp.clientLastActive[clientID] = time.Now()

	// Log the current client count
	sp.logger.Info("Client added",
		String("clientID", clientID),
		Int("total_clients", len(sp.clients)))

	return reader
}

// IsClientConnected checks if a client is already connected without affecting the connection
func (sp *StreamProcessor) IsClientConnected(clientID string) bool {
	sp.clientsMu.RLock()
	defer sp.clientsMu.RUnlock()

	if existingReader, exists := sp.clients[clientID]; exists {
		existingReader.mu.Lock()
		isClosed := existingReader.closed
		existingReader.mu.Unlock()
		return !isClosed
	}
	return false
}

// GetClientCount returns the number of connected clients.
func (sp *StreamProcessor) GetClientCount() int {
	sp.clientsMu.RLock()
	defer sp.clientsMu.RUnlock()
	return len(sp.clients)
}

// NonClosingReader wraps a ReadCloser but prevents Close() from affecting the underlying reader.
// It also updates the last activity time of the client when Read is called.
type NonClosingReader struct {
	io.ReadCloser
	processor *StreamProcessor
	clientID  string
}

// Read reads data and updates the last activity time
func (ncr *NonClosingReader) Read(p []byte) (n int, err error) {
	n, err = ncr.ReadCloser.Read(p)

	// Update last activity time if processor and clientID are set
	if ncr.processor != nil && ncr.clientID != "" && n > 0 {
		ncr.processor.updateClientActivity(ncr.clientID)
	}

	return n, err
}

// Close is a no-op to prevent closing the underlying reader.
func (ncr *NonClosingReader) Close() error {
	// This is intentionally a no-op to prevent closing the shared buffer
	return nil
}

// updateClientActivity updates the last activity time for a client
func (sp *StreamProcessor) updateClientActivity(clientID string) {
	now := time.Now()

	// First, try a read lock to check the condition if an update might be needed.
	sp.clientsMu.RLock()
	lastActive, exists := sp.clientLastActive[clientID]
	needsUpdate := false
	if exists && now.Sub(lastActive) >= 5*time.Second {
		needsUpdate = true
	}
	sp.clientsMu.RUnlock() // Release read lock

	// If no update is needed based on the read-locked check, return early.
	if !needsUpdate {
		return
	}

	// If an update is likely needed, acquire a full write lock.
	sp.clientsMu.Lock()
	defer sp.clientsMu.Unlock() // Ensure write lock is released on return

	// Re-check the condition under the write lock, as the state might have changed
	// between releasing the RLock and acquiring the WLock, or the client might have been removed.
	// Also, ensure the client still exists in the map before updating.
	if currentLastActive, stillExists := sp.clientLastActive[clientID]; stillExists {
		// Only update if the condition (5 seconds passed) still holds.
		// This handles the case where another goroutine might have updated it
		// or the client was re-added in the small window between RUnlock and Lock.
		if now.Sub(currentLastActive) >= 5*time.Second {
			sp.clientLastActive[clientID] = now
		}
	}
	// If the client was removed or updated by another routine, we simply do nothing here,
	// which is safe.
}

// Service manages frequency audio streams with persistent connections.
type Service struct {
	// Live labels, set from outside. See label.go: a mixed stream's contents can
	// change while the server runs, and only whoever changed them knows.
	labels               *labels
	client               *Client
	frequenciesConfig    map[string]*cfg.FrequencyConfig
	bufferSize           int
	config               *cfg.Config
	logger               *logger.Logger
	activeStreams        map[string]*StreamProcessor
	streamsMu            sync.RWMutex
	ctx                  context.Context
	cancel               context.CancelFunc
	streamPortIndex      atomic.Uint64 // round-robin port selection; atomic: the list is built under a read lock
	allServerPorts       []int         // Combined list of primary and additional ports
	transcriptionManager *transcription.TranscriptionManager
	wsServer             *websocket.Server               // WebSocket server for broadcasting status updates
	connectionStatus     map[string]connectionStatusInfo // Track connection status per frequency
	statusMu             sync.RWMutex                    // Mutex for connectionStatus map

	// Sources can be added and removed while the server runs (runtime_sources.go),
	// so frequenciesConfig is guarded; runtime marks the sources that were added.
	sourcesMu sync.RWMutex
	runtime   map[string]bool
	changeMu  sync.Mutex // one add or remove at a time

	// Asked before an added source that transcribes is started; nil lets every
	// source through. See SetTranscriptionGate.
	transcriptionGate func(context.Context) error
}

// connectionStatusInfo stores the current connection status and error for a frequency
type connectionStatusInfo struct {
	Status    string
	Error     string
	UpdatedAt time.Time
}

// NewService creates a new frequencies service.
func NewService(
	config *cfg.Config,
	logger *logger.Logger,
	wsServer *websocket.Server,
	transcriptionStorage *sqlite.TranscriptionStorage,
	clearanceStorage *sqlite.ClearanceStorage,
	fleet transcription.FleetProvider,
) *Service {
	// EXPERIMENT: Reduce buffer size to see impact on perceived lag from "live"
	bufferSize := 4 * 1024 // 4KB buffer, approx 2 seconds at 16kbps
	if config.Frequencies.BufferSizeKB > 0 {
		bufferSize = config.Frequencies.BufferSizeKB * 1024
	}

	freqsConfig := make(map[string]*cfg.FrequencyConfig)
	for i := range config.Frequencies.Sources {
		src := config.Frequencies.Sources[i]
		freqsConfig[src.ID] = &src
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Create transcription manager
	// Expected language per frequency, straight from the catalogue.
	frequencyLanguages := make(map[string]string, len(config.Frequencies.Sources))
	for _, src := range config.Frequencies.Sources {
		if src.Language != "" {
			frequencyLanguages[src.ID] = src.Language
		}
	}

	transcriptionConfig := transcription.Config{
		FrequencyLanguages: frequencyLanguages,
		Local: transcription.LocalSTTConfig{
			ServerURL:         config.Transcription.Local.ServerURL,
			TimeoutSeconds:    config.Transcription.Local.TimeoutSeconds,
			SegmentSilenceMs:  config.Transcription.Local.SegmentSilenceMs,
			SegmentMinMs:      config.Transcription.Local.SegmentMinMs,
			SegmentMaxSeconds: config.Transcription.Local.SegmentMaxSeconds,
			SegmentPrerollMs:  config.Transcription.Local.SegmentPrerollMs,
			SilenceThreshold:  config.Transcription.Local.SilenceThreshold,
		},
		Language:         config.Transcription.Language,
		FFmpegSampleRate: config.Transcription.FFmpegSampleRate,
		LogDir:           config.Transcription.LogDir,
	}

	postProcessingConfig := transcription.PostProcessingConfig{
		Enabled:         config.PostProcessing.Enabled,
		IntervalSeconds: config.PostProcessing.IntervalSeconds,
		BatchSize:       config.PostProcessing.BatchSize,
		AirlinesDatPath: config.PostProcessing.AirlinesDatPath,
		MinScore:        config.PostProcessing.MinScore,
		MinDigits:       config.PostProcessing.MinDigits,
	}

	transcriptionManager := transcription.NewTranscriptionManager(
		wsServer,
		transcriptionStorage,
		clearanceStorage,
		logger.Named("transcribe"),
		transcriptionConfig,
		postProcessingConfig,
		fleet,
	)

	// Prepare the list of all available server ports for round-robin stream URL generation
	allPorts := []int{config.Server.Port}
	if len(config.Server.AdditionalPorts) > 0 {
		allPorts = append(allPorts, config.Server.AdditionalPorts...)
	}

	return &Service{
		labels:               newLabels(),
		client:               NewClient(0, logger),
		frequenciesConfig:    freqsConfig,
		runtime:              map[string]bool{},
		bufferSize:           bufferSize,
		config:               config,
		logger:               logger.Named("freq-service"),
		activeStreams:        make(map[string]*StreamProcessor),
		streamsMu:            sync.RWMutex{},
		ctx:                  ctx,
		cancel:               cancel,
		allServerPorts:       allPorts,
		transcriptionManager: transcriptionManager,
		wsServer:             wsServer,
		connectionStatus:     make(map[string]connectionStatusInfo),
	}
}

// Start initializes connections to all configured frequencies.
func (s *Service) Start(ctx context.Context) error {
	s.logger.Info("Starting frequencies service with persistent connections")

	// Start a stream processor for each configured frequency
	s.sourcesMu.RLock()
	configured := make([]*cfg.FrequencyConfig, 0, len(s.frequenciesConfig))
	for _, freqConfig := range s.frequenciesConfig {
		configured = append(configured, freqConfig)
	}
	s.sourcesMu.RUnlock()
	for _, freqConfig := range configured {
		s.startSource(freqConfig)
	}

	s.logger.Info("All frequency stream processors started")

	// Start post-processing if enabled
	if s.config.PostProcessing.Enabled {
		s.logger.Info("Starting post-processing")
		if err := s.transcriptionManager.StartPostProcessing(s.ctx); err != nil {
			s.logger.Error("Failed to start post-processing", Error(err))
			// Continue even if post-processing fails
		}
	} else {
		s.logger.Info("Post-processing is disabled")
	}

	return nil
}

// startSource connects to one frequency's stream and, if it is to be
// transcribed, starts its transcription. A failure is logged and broadcast
// rather than returned: one unreachable stream must not keep the others down.
func (s *Service) startSource(freqConfig *cfg.FrequencyConfig) bool {
	id := freqConfig.ID
	s.logger.Info("Starting stream processor for frequency",
		String("id", id),
		String("name", freqConfig.Name),
		String("url", freqConfig.URL))

	processor, err := NewStreamProcessor(
		s.ctx,
		id,
		freqConfig.URL,
		freqConfig.ReconnectsInFFmpeg(),
		freqConfig.FFmpegInputOptions,
		s.client,
		s.config,
		s.logger,
	)

	if err != nil {
		s.logger.Error("Failed to create stream processor",
			String("id", id),
			Error(err))
		// Broadcast failure status
		s.broadcastFrequencyStatus(id, audio.StatusFailed, err.Error())
		return false
	}

	// Set status callback to broadcast status changes via WebSocket
	processor.audioProcessor.SetStatusCallback(s.broadcastFrequencyStatus)
	processor.audioProcessor.SetActivityCallback(s.config.Transcription.Local.SilenceThreshold, s.broadcastFrequencyActivity)

	err = processor.Start()
	if err != nil {
		s.logger.Error("Failed to start stream processor",
			String("id", id),
			Error(err))
		// Broadcast failure status
		s.broadcastFrequencyStatus(id, audio.StatusFailed, err.Error())
		return false
	}

	s.streamsMu.Lock()
	s.activeStreams[id] = processor
	s.streamsMu.Unlock()

	// Start transcription with external audio if enabled
	frequency := &Frequency{
		ID:              id,
		Name:            freqConfig.Name,
		URL:             freqConfig.URL,
		TranscribeAudio: freqConfig.TranscribeAudio,
	}

	if frequency.TranscribeAudio {
		s.logger.Info("Starting transcription with external audio for frequency",
			String("id", id),
			String("name", freqConfig.Name),
			Bool("transcribe_audio", freqConfig.TranscribeAudio))

		if err := s.transcriptionManager.StartTranscriptionWithExternalAudio(
			s.ctx,
			frequency.ID,
			frequency.Name,
			frequency.TranscribeAudio,
			processor.audioProcessor,
		); err != nil {
			s.logger.Error("Failed to start transcription with external audio for frequency",
				String("id", id),
				Error(err))
		}
	} else {
		s.logger.Info("Transcription not enabled for frequency",
			String("id", id),
			String("name", freqConfig.Name),
			Bool("transcribe_audio", freqConfig.TranscribeAudio))
	}
	return true
}

// MeanTranscriptionSeconds is the rolling mean of the time a transmission
// takes to transcribe, over every frequency; false until one has been.
func (s *Service) MeanTranscriptionSeconds() (float64, bool) {
	if s.transcriptionManager == nil {
		return 0, false
	}
	return s.transcriptionManager.MeanTranscriptionSeconds()
}

// Stop stops all stream processors and cleans up resources.
func (s *Service) Stop() {
	s.logger.Info("Frequencies service stopping")

	// Stop all transcriptions
	s.transcriptionManager.StopAllTranscriptions()

	// Cancel the main context to signal all stream processors to stop
	s.cancel()

	// Create a WaitGroup to wait for all processors to stop
	var wg sync.WaitGroup

	// Stop each stream processor
	s.streamsMu.Lock()
	for id, processor := range s.activeStreams {
		if processor != nil {
			wg.Add(1)
			go func(id string, proc *StreamProcessor) {
				defer wg.Done()
				s.logger.Info("Stopping stream processor", String("id", id))
				proc.Stop()
			}(id, processor)
		}
	}
	s.streamsMu.Unlock()

	// Wait for all processors to stop with a timeout
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		s.logger.Info("All stream processors stopped")
	case <-time.After(5 * time.Second):
		s.logger.Warn("Timeout waiting for stream processors to stop")
	}

	// Clear the active streams map
	s.streamsMu.Lock()
	s.activeStreams = make(map[string]*StreamProcessor)
	s.streamsMu.Unlock()

	s.logger.Info("Frequencies service stopped")
}

// ClientStreamReader manages the lifecycle of a single client's audio stream resources.
// Its Close method is crucial for cleanup.
type ClientStreamReader struct {
	io.ReadCloser // The client's dedicated circular buffer
	logger        *logger.Logger
	streamID      string
	once          sync.Once // Ensures cleanup actions are performed only once
	processor     *StreamProcessor
	clientID      string
	lastActivity  time.Time          // Track when this client was last active
	ctx           context.Context    // Context for cancellation
	cancel        context.CancelFunc // Function to cancel the context
	closed        bool               // Flag to track if the reader is closed
	mu            sync.Mutex         // Mutex to protect the closed flag
}

// Close cleans up resources for this specific client stream.
func (csr *ClientStreamReader) Close() error {
	var err error // Variable to store error from ReadCloser.Close if any
	csr.once.Do(func() {
		csr.mu.Lock()
		if csr.closed {
			csr.mu.Unlock()
			return // Already closed by another goroutine
		}
		csr.closed = true
		csr.mu.Unlock()

		csr.logger.Info("Closing client stream reader, cancelling context and cleaning up local resources",
			String("streamID", csr.streamID),
			String("clientID", csr.clientID))

		// Cancel the context to signal all operations using this reader to stop
		if csr.cancel != nil {
			csr.cancel()
		}

		// The responsibility to remove this client from the StreamProcessor's maps
		// is now with the caller that initiated the close (e.g., removeInactiveClients,
		// or the HTTP handler after io.Copy, or StreamProcessor.Stop).
		// DO NOT CALL: csr.processor.RemoveClient(csr.clientID) from here to avoid deadlocks.

		// Close the underlying reader if it's a real resource specific to this client.
		// For NonClosingReader, ReadCloser.Close() is a no-op.
		if csr.ReadCloser != nil {
			internalErr := csr.ReadCloser.Close()
			if internalErr != nil {
				// Store the first error encountered during closing.
				// Since NonClosingReader.Close() is a no-op and returns nil, this 'err' will likely remain nil.
				err = internalErr
				csr.logger.Error("Error closing underlying ReadCloser in ClientStreamReader",
					String("streamID", csr.streamID),
					String("clientID", csr.clientID),
					Error(internalErr))
			}
		}
	})
	return err
}

// Read reads data from the underlying reader with timeout handling
func (csr *ClientStreamReader) Read(p []byte) (n int, err error) {
	// Check if already closed
	csr.mu.Lock()
	if csr.closed {
		csr.mu.Unlock()
		return 0, io.EOF
	}
	csr.mu.Unlock()

	// Note: csr.lastActivity is not updated here anymore.
	// The crucial last activity for processor cleanup is updated by NonClosingReader.Read
	// calling processor.updateClientActivity.

	// Check if context is already canceled before attempting to read
	select {
	case <-csr.ctx.Done():
		// Context for this specific client stream has been canceled.
		// This could be due to the HTTP request ending, or the StreamProcessor stopping this client.
		// Ensure Close is called (it's idempotent) to mark csr.closed = true.
		csr.Close()
		return 0, io.EOF
	default:
		// Context is not done, proceed to read from the underlying source.
	}

	// csr.ReadCloser is NonClosingReader, which wraps the shared CircularBuffer.
	// CircularBuffer.Read will block until data is available or the buffer itself is closed.
	n, err = csr.ReadCloser.Read(p)

	// Handle the result of the read operation
	if err != nil {
		// If an error occurred, including io.EOF (if CircularBuffer was closed),
		// we should ensure this ClientStreamReader is also marked as closed.
		// csr.Close() is idempotent and handles this.
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrClosedPipe) {
			csr.logger.Info("Underlying reader returned EOF or closed pipe, closing client stream reader",
				String("streamID", csr.streamID), String("clientID", csr.clientID))
			csr.Close()
		} else {
			csr.logger.Error("Error reading from underlying ReadCloser",
				String("streamID", csr.streamID), String("clientID", csr.clientID), Error(err))
			// For other errors, also ensure this client reader is closed to prevent further issues.
			csr.Close()
		}
		return n, err // Propagate the original error (n might be >0 with an error)
	}

	// If n == 0 and err == nil:
	// The current CircularBuffer.Read is designed to block until data is available or it's closed
	// (returning n>0 or io.EOF). It should not return (0, nil).
	// If it somehow did, returning (0, nil) is generally acceptable for io.Copy, which would retry.
	// No special handling needed here for that theoretical case; just return what was received.
	return n, nil
}

// GetAudioStream returns a reader for a frequency's audio stream.
// It accepts a client ID to track individual client connections.
func (s *Service) GetAudioStream(ctx context.Context, id string, clientID string) (io.ReadCloser, string, error) {
	// Create a context with timeout to prevent hanging
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// Check if the frequency exists
	s.sourcesMu.RLock()
	freqConfig, ok := s.frequenciesConfig[id]
	s.sourcesMu.RUnlock()
	if !ok {
		return nil, "", fmt.Errorf("frequency configuration not found: %s", id)
	}

	s.logger.Info("Client requesting audio stream",
		String("id", id),
		String("clientID", clientID))

	// Check if this client is already connected to this frequency
	s.streamsMu.RLock()
	processor, exists := s.activeStreams[id]
	s.streamsMu.RUnlock()

	if exists && processor.IsClientConnected(clientID) {
		s.logger.Info("Client already connected to this frequency, rejecting duplicate request",
			String("id", id),
			String("clientID", clientID))
		return nil, "", fmt.Errorf("client already connected to this frequency")
	}

	// Check if we've reached the maximum number of active clients
	s.streamsMu.RLock()
	totalClients := 0
	for _, proc := range s.activeStreams {
		totalClients += proc.GetClientCount()
	}
	s.streamsMu.RUnlock()

	// Limit to 20 concurrent clients total and 5 per frequency to prevent resource exhaustion
	if totalClients > 100 {
		s.logger.Warn("Too many concurrent clients, rejecting connection",
			String("id", id),
			String("clientID", clientID),
			Int("total_clients", totalClients))
		return nil, "", fmt.Errorf("too many concurrent clients (max 100)")
	}

	// Check if we already have a processor for this frequency
	s.streamsMu.RLock()
	processor, exists = s.activeStreams[id]
	s.streamsMu.RUnlock()

	// If processor exists, check client count for this specific frequency
	if exists && processor.GetClientCount() >= 10 {
		s.logger.Warn("Too many clients for this frequency, rejecting connection",
			String("id", id),
			String("clientID", clientID),
			Int("client_count", processor.GetClientCount()))
		return nil, "", fmt.Errorf("too many clients for this frequency (max 10)")
	}

	// We already have the processor from the check above, no need to get it again

	if !exists {
		s.logger.Info("Stream processor not found, creating new one", String("id", id))

		s.streamsMu.Lock()
		// Check again in case another goroutine created it while we were waiting for the lock
		processor, exists = s.activeStreams[id]
		if !exists {
			var err error
			processor, err = NewStreamProcessor(
				s.ctx,
				id,
				freqConfig.URL,
				freqConfig.ReconnectsInFFmpeg(),
				freqConfig.FFmpegInputOptions,
				s.client,
				s.config,
				s.logger,
			)

			if err != nil {
				s.streamsMu.Unlock()
				s.logger.Error("Failed to create stream processor", String("id", id), Error(err))
				return nil, "", fmt.Errorf("failed to create stream processor: %w", err)
			}

			processor.audioProcessor.SetActivityCallback(s.config.Transcription.Local.SilenceThreshold, s.broadcastFrequencyActivity)
			err = processor.Start()
			if err != nil {
				s.streamsMu.Unlock()
				s.logger.Error("Failed to start stream processor", String("id", id), Error(err))
				return nil, "", fmt.Errorf("failed to start stream processor: %w", err)
			}

			s.activeStreams[id] = processor
		}
		s.streamsMu.Unlock()
	}

	// Check if the context has been canceled
	select {
	case <-ctx.Done():
		return nil, "", ctx.Err()
	default:
		// Continue
	}

	// Add the client to the stream processor
	clientReader := processor.AddClient(clientID)

	s.logger.Debug("Client connected to audio stream",
		String("id", id),
		String("clientID", clientID),
		String("contentType", processor.contentType))

	return clientReader, processor.contentType, nil
}

// GetAllFrequencies and GetFrequencyByID now only report on configured frequencies,
// as "active" status is per-client and not centrally tracked in the same way.
// We can indicate a general "available" status based on config existence.
func (s *Service) GetAllFrequencies() []*Frequency { // frequencies.Frequency from models.go
	var result []*Frequency

	// Get a snapshot of connection statuses
	s.statusMu.RLock()
	statusSnapshot := make(map[string]connectionStatusInfo)
	for k, v := range s.connectionStatus {
		statusSnapshot[k] = v
	}
	s.statusMu.RUnlock()

	s.sourcesMu.RLock()
	defer s.sourcesMu.RUnlock()
	for _, fc := range s.frequenciesConfig {
		streamURL, streamPort := s.buildStreamInfo(fc.ID)

		// Get the connection status if available
		status := "available"
		lastError := ""
		if statusInfo, exists := statusSnapshot[fc.ID]; exists {
			status = statusInfo.Status
			lastError = statusInfo.Error
		}

		result = append(result, &Frequency{
			ID:              fc.ID,
			Airport:         fc.Airport,
			Name:            s.labels.nameFor(fc.ID, fc.Name),
			FrequencyMHz:    fc.FrequencyMHz,
			URL:             fc.URL,
			StreamURL:       streamURL,
			StreamPort:      streamPort,
			Status:          status,
			LastError:       lastError,
			Order:           fc.Order,
			TranscribeAudio: fc.TranscribeAudio,
			Runtime:         s.runtime[fc.ID],
			SectorAirport:   fc.SectorAirport,
			SectorKind:      fc.SectorKind,
		})
	}

	// Sort frequencies by order instead of name
	sort.Slice(result, func(i, j int) bool {
		return result[i].Order < result[j].Order
	})

	return result
}

func (s *Service) GetFrequencyByID(id string) (*Frequency, bool) {
	s.sourcesMu.RLock()
	defer s.sourcesMu.RUnlock()
	fc, ok := s.frequenciesConfig[id]
	if !ok {
		return nil, false
	}
	streamURL, streamPort := s.buildStreamInfo(fc.ID)

	// Get the connection status if available
	status := "available"
	lastError := ""
	s.statusMu.RLock()
	if statusInfo, exists := s.connectionStatus[fc.ID]; exists {
		status = statusInfo.Status
		lastError = statusInfo.Error
	}
	s.statusMu.RUnlock()

	return &Frequency{
		ID:              fc.ID,
		Airport:         fc.Airport,
		Name:            s.labels.nameFor(fc.ID, fc.Name),
		Order:           fc.Order,
		FrequencyMHz:    fc.FrequencyMHz,
		URL:             fc.URL,
		StreamURL:       streamURL,
		StreamPort:      streamPort,
		Status:          status,
		LastError:       lastError,
		TranscribeAudio: fc.TranscribeAudio,
		Runtime:         s.runtime[fc.ID],
		SectorAirport:   fc.SectorAirport,
		SectorKind:      fc.SectorKind,
	}, true
}

// buildStreamInfo returns the stream URL path and the port to use for this stream.
// The port is selected via round-robin from all available server ports.
func (s *Service) buildStreamInfo(frequencyID string) (string, int) {
	// Get the next port via round-robin
	i := s.streamPortIndex.Add(1) - 1
	port := s.allServerPorts[i%uint64(len(s.allServerPorts))]

	// Return relative URL path so the browser uses the correct hostname
	return fmt.Sprintf("/api/v1/stream/%s", frequencyID), port
}

// broadcastFrequencyStatus sends a frequency status change to all connected WebSocket clients
// and stores the status for later retrieval via API
func (s *Service) broadcastFrequencyStatus(frequencyID string, status audio.ConnectionStatus, errorMsg string) {
	// Always store the status, even if WebSocket server isn't available
	s.statusMu.Lock()
	s.connectionStatus[frequencyID] = connectionStatusInfo{
		Status:    string(status),
		Error:     errorMsg,
		UpdatedAt: time.Now(),
	}
	s.statusMu.Unlock()

	s.logger.Debug("Frequency status updated",
		String("frequency_id", frequencyID),
		String("status", string(status)),
		String("error", errorMsg))

	// Broadcast via WebSocket if available
	if s.wsServer == nil {
		return
	}

	message := &websocket.Message{
		Type: websocket.MessageTypeFrequencyStatus,
		Data: map[string]interface{}{
			"frequency_id": frequencyID,
			"status":       string(status),
			"error":        errorMsg,
		},
	}

	s.wsServer.Broadcast(message)
}

// broadcastFrequencyActivity tells every page that someone is speaking on a
// frequency, or has stopped, and the level while they speak (internal/audio,
// activity.go): the page's "someone is speaking" and its level meter, the
// same in every browser and for a page that does not listen.
func (s *Service) broadcastFrequencyActivity(frequencyID string, speaking bool, level float64) {
	if s.wsServer == nil {
		return
	}
	s.wsServer.Broadcast(&websocket.Message{
		Type: websocket.MessageTypeFrequencyActivity,
		Data: map[string]interface{}{
			"frequency_id": frequencyID,
			"speaking":     speaking,
			"level":        math.Round(level*100) / 100,
		},
	})
}
