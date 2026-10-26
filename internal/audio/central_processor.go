package audio

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/yegors/co-atc/pkg/logger"
)

// Import logger functions
var (
	String = logger.String
	Int    = logger.Int
	Error  = logger.Error
)

// ConnectionStatus represents the current connection state of a frequency
type ConnectionStatus string

const (
	StatusConnecting ConnectionStatus = "connecting"
	StatusConnected  ConnectionStatus = "connected"
	StatusFailed     ConnectionStatus = "failed"
	StatusStopped    ConnectionStatus = "stopped"
)

// StatusChangeCallback is called when the connection status changes
type StatusChangeCallback func(frequencyID string, status ConnectionStatus, errorMsg string)

// CentralAudioProcessor manages audio processing for a frequency
// that can be shared between browser streaming and transcription.
//
// Every source is read inside the binary since the Go decoder passed its
// 24-hour production trial (docs-fr/35-plan.md, N5 and B1 ter): an MP3
// stream over HTTP is decoded by the copy of goxiph in
// third_party (godecode.go), raw PCM over UDP is received as it is (udp.go).
// Both write 16-bit mono PCM at the configured rate, which is what the
// readers below share. ffmpeg, which read everything before, is gone with
// what only it could read (SRT, a sound card).
type CentralAudioProcessor struct {
	id             string
	audioURL       string
	sampleRate     int
	pcm            PCMInput // what a raw PCM source sends; unused for MP3
	multiReader    *MultiReader
	ctx            context.Context
	cancel         context.CancelFunc
	logger         *logger.Logger
	mu             sync.Mutex
	isRunning      bool
	lastError      error
	lastActivity   time.Time
	reconnectTimer *time.Timer
	reconnectDelay time.Duration
	statusCallback StatusChangeCallback // Callback for status changes
	currentStatus  ConnectionStatus     // Current connection status
	flow           flowMeter            // bytes decoded, the last minute (flow.go)

	// decodeCancel stops the decoder running, decoded is the end of its pipe
	// the readers are fed from.
	decodeCancel context.CancelFunc
	decoded      io.ReadCloser

	// mp3 relays the stream's own MP3 frames to browsers (mp3relay.go), fed
	// by the MP3 decoder.
	mp3 *mp3Relay
	// ulaw relays the decoded sound in mu-law to pages that play it
	// themselves (ulaw.go), fed by the MP3 decoder.
	ulaw *mp3Relay

	// activity says when the frequency carries sound (activity.go); nil
	// until SetActivityCallback.
	activity *activityMeter
}

// Channels is what every source is brought to: one channel. A radio
// frequency carries one voice at a time, and the transcription reads mono.
const Channels = 1

// CentralProcessorConfig contains configuration for the central audio processor
type CentralProcessorConfig struct {
	SampleRate     int           // of the PCM written, whatever the source's
	ReconnectDelay time.Duration // after a stream that ends or fails

	// PCM is what a raw PCM source over UDP sends: nothing in its packets
	// says it. Unused for an MP3 stream, which says its own format.
	PCM PCMInput
}

// NewCentralAudioProcessor creates a new central audio processor.
func NewCentralAudioProcessor(
	ctx context.Context,
	id string,
	audioURL string,
	config CentralProcessorConfig,
	logger *logger.Logger,
) (*CentralAudioProcessor, error) {
	procCtx, procCancel := context.WithCancel(ctx)

	// Create multi-reader for sharing the stream
	multiReader := NewMultiReader(procCtx, logger.Named("multi-reader"))

	return &CentralAudioProcessor{
		id:             id,
		audioURL:       audioURL,
		sampleRate:     config.SampleRate,
		pcm:            config.PCM,
		mp3:            newMP3Relay(),
		ulaw:           newRelay(ulawBurstChunks),
		multiReader:    multiReader,
		ctx:            procCtx,
		cancel:         procCancel,
		logger:         logger.Named("central-audio-processor").With(String("id", id)),
		isRunning:      false,
		lastActivity:   time.Now(),
		reconnectDelay: config.ReconnectDelay,
	}, nil
}

// SourceProblem says why a source cannot be read, or nil: an MP3 stream
// over http or https, or raw PCM over udp with its rate and channels.
// Anything else was read by ffmpeg, removed in B1 ter.
func SourceProblem(audioURL string, pcm PCMInput) error {
	switch {
	case isHTTP(audioURL):
		return nil
	case isUDP(audioURL):
		if err := pcm.Valid(); err != nil {
			return fmt.Errorf("raw PCM over UDP: %w", err)
		}
		_, err := udpAddr(audioURL)
		return err
	default:
		return fmt.Errorf("%q: not a source this version reads -- an MP3 stream over http(s), or raw PCM over udp; "+
			"SRT and sound cards went with ffmpeg", audioURL)
	}
}

// SetStatusCallback sets the callback function for status changes
func (p *CentralAudioProcessor) SetStatusCallback(callback StatusChangeCallback) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.statusCallback = callback
}

// notifyStatusChange notifies listeners of a status change
func (p *CentralAudioProcessor) notifyStatusChange(status ConnectionStatus, errorMsg string) {
	p.currentStatus = status
	if p.statusCallback != nil {
		// Call callback in a goroutine to prevent blocking
		go p.statusCallback(p.id, status, errorMsg)
	}
}

// Start starts the audio processor
func (p *CentralAudioProcessor) Start() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.isRunning {
		return nil
	}

	p.logger.Info("Starting central audio processor",
		String("url", p.audioURL),
		Int("sample_rate", p.sampleRate))

	// Notify connecting status
	p.notifyStatusChange(StatusConnecting, "")

	if err := p.startDecoder(); err != nil {
		p.notifyStatusChange(StatusFailed, err.Error())
		return fmt.Errorf("failed to start the decoder: %w", err)
	}

	p.isRunning = true
	// Notify connected status
	p.notifyStatusChange(StatusConnected, "")
	return nil
}

// Stop stops the audio processor
func (p *CentralAudioProcessor) Stop() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.isRunning {
		return nil
	}

	p.logger.Info("Stopping central audio processor")

	// Cancel context to stop all operations
	p.cancel()

	p.stopDecoder()
	p.mp3.close()
	p.ulaw.close()

	// Close multi-reader
	p.multiReader.Close()

	p.isRunning = false
	// Notify stopped status
	p.notifyStatusChange(StatusStopped, "")
	return nil
}

// startDecoder starts reading the source in Go: what the decoder writes goes
// through a pipe to processOutput, and a read error there -- the stream that
// ends, the connection that drops, the port that cannot be had -- schedules
// the next attempt.
func (p *CentralAudioProcessor) startDecoder() error {
	if err := SourceProblem(p.audioURL, p.pcm); err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(p.ctx)
	pr, pw := io.Pipe()
	p.decodeCancel = cancel
	p.decoded = pr
	go func() {
		var err error
		if isUDP(p.audioURL) {
			err = decodeUDP(ctx, p.audioURL, p.pcm, p.sampleRate, pw, p.logger)
		} else {
			err = decodeHTTP(ctx, p.audioURL, p.sampleRate, pw, p.mp3, p.ulaw, p.logger)
		}
		if ctx.Err() != nil {
			err = ctx.Err()
		}
		pw.CloseWithError(err)
	}()
	go p.processOutput(pr)
	return nil
}

// stopDecoder stops the decoder, if one runs, and any restart scheduled.
func (p *CentralAudioProcessor) stopDecoder() {
	if p.decodeCancel != nil {
		p.decodeCancel()
		p.decodeCancel = nil
	}
	if p.decoded != nil {
		_ = p.decoded.Close()
		p.decoded = nil
	}

	if p.reconnectTimer != nil {
		p.reconnectTimer.Stop()
		p.reconnectTimer = nil
	}
}

// processOutput copies what the decoder writes to the readers, until it
// stops; then it schedules a restart after the reconnect delay.
func (p *CentralAudioProcessor) processOutput(decoded io.Reader) {
	p.logger.Info("Starting to process the decoded audio")

	// Create buffer for reading
	buffer := make([]byte, 4096)
	bytesProcessed := 0
	lastLogTime := time.Now()

	for {
		select {
		case <-p.ctx.Done():
			p.logger.Info("Context canceled, stopping the decoded audio processing",
				Int("total_bytes_processed", bytesProcessed))
			return
		default:
			n, err := decoded.Read(buffer)
			if err != nil {
				if err == io.EOF {
					p.logger.Warn("The source ended unexpectedly",
						Int("total_bytes_processed", bytesProcessed),
						String("duration_since_start", time.Since(lastLogTime).String()))
				} else {
					p.logger.Error("Error reading the source", Error(err),
						Int("total_bytes_processed", bytesProcessed),
						String("duration_since_start", time.Since(lastLogTime).String()))
					p.lastError = err
				}

				// Attempt to restart the decoder after a delay
				p.mu.Lock()
				if p.isRunning && p.reconnectTimer == nil {
					p.logger.Warn("Scheduling a restart of the source after a read error",
						String("error_type", fmt.Sprintf("%T", err)),
						String("error_message", err.Error()))
					// Notify failed status
					p.notifyStatusChange(StatusFailed, err.Error())
					p.reconnectTimer = time.AfterFunc(p.reconnectDelay, func() {
						p.mu.Lock()
						defer p.mu.Unlock()

						p.reconnectTimer = nil
						if p.isRunning {
							p.logger.Info("Executing scheduled restart of the source")
							p.notifyStatusChange(StatusConnecting, "")
							p.stopDecoder()
							if err := p.startDecoder(); err != nil {
								p.logger.Error("Failed to restart the source", Error(err))
								p.notifyStatusChange(StatusFailed, err.Error())
							} else {
								p.logger.Info("Source restarted")
								p.notifyStatusChange(StatusConnected, "")
							}
						}
					})
				}
				p.mu.Unlock()
				return
			}

			if n > 0 {
				bytesProcessed += n
				// Update last activity time
				p.lastActivity = time.Now()
				p.flow.add(n, p.lastActivity)

				// Log progress every 30 seconds
				if time.Since(lastLogTime) > 30*time.Second {
					p.logger.Debug("Decoding progress",
						Int("bytes_processed", bytesProcessed),
						Int("bytes_this_read", n),
						String("duration", time.Since(lastLogTime).String()))
					lastLogTime = time.Now()
				}

				if a := p.activity; a != nil {
					a.push(buffer[:n])
				}

				// Write to multi-reader
				if _, err := p.multiReader.Write(buffer[:n]); err != nil {
					p.logger.Error("Error writing to multi-reader", Error(err),
						Int("bytes_processed_before_error", bytesProcessed))
					return
				}
			}
		}
	}
}

// CreateReader creates a new reader for the audio stream (with WAV header for browser playback)
func (p *CentralAudioProcessor) CreateReader(id string) (io.ReadCloser, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.isRunning {
		if err := p.startDecoder(); err != nil {
			return nil, fmt.Errorf("failed to start processor: %w", err)
		}
		p.isRunning = true
	}

	// Create a reader with WAV header
	reader := p.multiReader.CreateReader(id)
	return NewWAVReader(reader, p.sampleRate, Channels), nil
}

// CreateRawReader creates a new reader for raw PCM audio (no WAV header, for transcription)
func (p *CentralAudioProcessor) CreateRawReader(id string) (io.ReadCloser, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.isRunning {
		if err := p.startDecoder(); err != nil {
			return nil, fmt.Errorf("failed to start processor: %w", err)
		}
		p.isRunning = true
	}

	// Return raw PCM reader without WAV header
	return p.multiReader.CreateReader(id), nil
}

// RemoveReader removes a reader
func (p *CentralAudioProcessor) RemoveReader(id string) {
	p.multiReader.RemoveReader(id)
}

// GetStatus returns the status of the processor
func (p *CentralAudioProcessor) GetStatus() (string, time.Time, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.isRunning {
		return "stopped", p.lastActivity, nil
	}

	if p.lastError != nil {
		return "error", p.lastActivity, p.lastError
	}

	return "running", p.lastActivity, nil
}
