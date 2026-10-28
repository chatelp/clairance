package transcription

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/yegors/co-atc/internal/audio"
	"github.com/yegors/co-atc/internal/storage/sqlite"
	"github.com/yegors/co-atc/internal/transcription/phraseology"
	"github.com/yegors/co-atc/internal/websocket"
	"github.com/yegors/co-atc/pkg/logger"
)

// TranscriptionManager manages transcription processors for frequencies
type TranscriptionManager struct {
	processors           map[string]ProcessorInterface
	mu                   sync.RWMutex
	wsServer             *websocket.Server
	transcriptionStorage *sqlite.TranscriptionStorage
	clearanceStorage     *sqlite.ClearanceStorage
	logger               *logger.Logger
	transcriptionConfig  Config
	postProcessor        batchProcessor
	postProcessingConfig PostProcessingConfig
	fleet                FleetProvider // live ADS-B for the grammar; nil disables matching
	fileLogger           *FileLogger   // Optional file logger for transcriptions
	timing               *TranscriptionTiming
}

// NewTranscriptionManager creates a new transcription manager
func NewTranscriptionManager(
	wsServer *websocket.Server,
	transcriptionStorage *sqlite.TranscriptionStorage,
	clearanceStorage *sqlite.ClearanceStorage,
	logger *logger.Logger,
	transcriptionConfig Config,
	postProcessingConfig PostProcessingConfig,
	fleet FleetProvider,
) *TranscriptionManager {
	// Create file logger if log_dir is configured
	var fileLogger *FileLogger
	if transcriptionConfig.LogDir != "" {
		var err error
		fileLogger, err = NewFileLogger(transcriptionConfig.LogDir, logger)
		if err != nil {
			logger.Error("Failed to create transcription file logger",
				String("log_dir", transcriptionConfig.LogDir),
				Error(err))
			// Continue without file logging
			fileLogger = nil
		}
	}

	return &TranscriptionManager{
		processors:           make(map[string]ProcessorInterface),
		wsServer:             wsServer,
		transcriptionStorage: transcriptionStorage,
		clearanceStorage:     clearanceStorage,
		logger:               logger,
		transcriptionConfig:  transcriptionConfig,
		postProcessingConfig: postProcessingConfig,
		fleet:                fleet,
		fileLogger:           fileLogger,
		timing:               &TranscriptionTiming{},
	}
}

// MeanTranscriptionSeconds is the rolling mean of the time a transmission
// takes to transcribe, over every frequency; false until one has been.
func (m *TranscriptionManager) MeanTranscriptionSeconds() (float64, bool) {
	return m.timing.Mean()
}

// Activity returns when a frequency last sent a transmission to the sidecar
// and when one last came back as text; zero times for what has not happened.
func (m *TranscriptionManager) Activity(frequencyID string) (sent, transcribed time.Time) {
	return m.timing.Activity(frequencyID)
}

// StartTranscriptionWithExternalAudio starts transcription for a frequency using an external audio processor
func (m *TranscriptionManager) StartTranscriptionWithExternalAudio(
	ctx context.Context,
	frequencyID string,
	frequencyName string,
	transcribeAudio bool,
	audioProcessor interface{},
) error {
	// Skip if transcription is not enabled for this frequency
	if !transcribeAudio {
		m.logger.Info("Transcription not enabled for frequency",
			logger.String("id", frequencyID),
			logger.String("name", frequencyName))
		return nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if processor already exists
	if _, exists := m.processors[frequencyID]; exists {
		m.logger.Info("Transcription already started for frequency",
			logger.String("id", frequencyID),
			logger.String("name", frequencyName))
		return nil
	}

	m.logger.Info("Starting transcription with external audio for frequency",
		logger.String("id", frequencyID),
		logger.String("name", frequencyName))

	// Create external processor based on the type of audio processor
	var processor ProcessorInterface
	var err error

	// We only support CentralAudioProcessor now
	ap, ok := audioProcessor.(*audio.CentralAudioProcessor)
	if !ok {
		return fmt.Errorf("unsupported audio processor type: %T, only CentralAudioProcessor is supported", audioProcessor)
	}

	// Create a raw PCM reader (no WAV header) for transcription
	reader, readerErr := ap.CreateRawReader(fmt.Sprintf("transcription-%s", frequencyID))
	if readerErr != nil {
		return fmt.Errorf("failed to create reader from central processor: %w", readerErr)
	}

	// Create a processor that uses the reader
	processor, err = NewLocalProcessor(
		ctx,
		frequencyID,
		reader,
		m.transcriptionConfig,
		m.wsServer,
		m.transcriptionStorage,
		m.logger,
		m.fileLogger,
		m.timing,
	)
	if err != nil {
		return fmt.Errorf("failed to create external processor: %w", err)
	}

	// Start processor
	if err := processor.Start(); err != nil {
		return fmt.Errorf("failed to start external processor: %w", err)
	}

	// Store processor
	m.processors[frequencyID] = processor

	return nil
}

// SetMatchingRules gives the association rules in force, read for every
// transmission. Called before post-processing starts.
func (m *TranscriptionManager) SetMatchingRules(rules func() phraseology.Rules) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.postProcessingConfig.Rules = rules
}

// SetSectors gives the sector of each frequency, read for every transmission.
// Called before post-processing starts.
func (m *TranscriptionManager) SetSectors(sectorOf func(frequencyID string) (phraseology.Sector, bool)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.postProcessingConfig.SectorOf = sectorOf
}

// SetFrequencyLanguage records the expected language of a frequency that was
// not in the catalogue at startup. It applies to transcriptions started after it.
func (m *TranscriptionManager) SetFrequencyLanguage(frequencyID, language string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.transcriptionConfig.FrequencyLanguages == nil {
		m.transcriptionConfig.FrequencyLanguages = map[string]string{}
	}
	if language == "" {
		delete(m.transcriptionConfig.FrequencyLanguages, frequencyID)
		return
	}
	m.transcriptionConfig.FrequencyLanguages[frequencyID] = language
}

// StopTranscription stops transcription for a frequency
func (m *TranscriptionManager) StopTranscription(frequencyID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if processor exists
	processor, exists := m.processors[frequencyID]
	if !exists {
		m.logger.Info("No transcription processor found for frequency", logger.String("id", frequencyID))
		return
	}

	m.logger.Info("Stopping transcription for frequency", logger.String("id", frequencyID))

	// Stop processor
	if err := processor.Stop(); err != nil {
		m.logger.Error("Error stopping transcription processor",
			logger.String("id", frequencyID),
			logger.Error(err))
	}

	// Remove processor
	delete(m.processors, frequencyID)
}

// StopAllTranscriptions stops all transcription processors and post-processing
func (m *TranscriptionManager) StopAllTranscriptions() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.logger.Info("Stopping all transcription processors", logger.Int("count", len(m.processors)))

	// Stop all processors
	for id, processor := range m.processors {
		if err := processor.Stop(); err != nil {
			m.logger.Error("Error stopping transcription processor",
				logger.String("id", id),
				logger.Error(err))
		}
	}

	// Clear processors
	m.processors = make(map[string]ProcessorInterface)

	// Stop post-processor
	m.StopPostProcessing()

	// Close file logger
	if m.fileLogger != nil {
		if err := m.fileLogger.Close(); err != nil {
			m.logger.Error("Failed to close file logger", Error(err))
		}
	}
}

// StartPostProcessing starts the post-processing of transcriptions
func (m *TranscriptionManager) StartPostProcessing(ctx context.Context) error {
	if m.postProcessor != nil {
		m.logger.Info("Post-processing already started")
		return nil
	}

	grammar, err := NewGrammarProcessor(
		ctx,
		m.transcriptionStorage,
		m.clearanceStorage,
		m.wsServer,
		m.fleet,
		m.postProcessingConfig,
		m.logger,
	)
	if err != nil {
		return fmt.Errorf("failed to create local post-processor: %w", err)
	}
	if err := grammar.Start(); err != nil {
		return fmt.Errorf("failed to start local post-processor: %w", err)
	}
	m.postProcessor = grammar
	m.logger.Info("Post-processing started (local grammar)")
	return nil
}

// StopPostProcessing stops the post-processing of transcriptions
func (m *TranscriptionManager) StopPostProcessing() {
	if m.postProcessor == nil {
		m.logger.Info("No post-processor to stop")
		return
	}

	m.logger.Info("Stopping post-processor")
	m.postProcessor.Stop()
	m.postProcessor = nil
}

// batchProcessor is the second stage of the pipeline: it takes stored raw
// transcriptions and fills in who spoke, which aircraft it concerned, and any
// clearance issued. GrammarProcessor works it out locally; the manager only
// needs to start and stop it.
type batchProcessor interface {
	Start() error
	Stop() error
}
