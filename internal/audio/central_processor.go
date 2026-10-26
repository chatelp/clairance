package audio

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/yegors/co-atc/pkg/logger"
	"github.com/yegors/co-atc/pkg/tool"
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
// Every source goes through ffmpeg: whatever it can read is a source.
type CentralAudioProcessor struct {
	id                       string
	audioURL                 string
	ffmpegPath               string
	sampleRate               int
	channels                 int
	ffmpegTimeoutSecs        int // FFmpeg connection timeout in seconds
	ffmpegReconnectDelaySecs int // FFmpeg reconnect delay in seconds
	noFFmpegReconnect        bool
	ffmpegCmd                *exec.Cmd
	ffmpegStdout             io.ReadCloser
	inputOptions             []string // ffmpeg options placed before -i, from the source
	multiReader              *MultiReader
	ctx                      context.Context
	cancel                   context.CancelFunc
	logger                   *logger.Logger
	mu                       sync.Mutex
	isRunning                bool
	lastError                error
	lastActivity             time.Time
	reconnectTimer           *time.Timer
	monitorTicker            *time.Ticker
	reconnectDelay           time.Duration
	format                   string
	contentType              string
	statusCallback           StatusChangeCallback // Callback for status changes
	currentStatus            ConnectionStatus     // Current connection status
	flow                     flowMeter            // bytes out of ffmpeg, the last minute (flow.go)

	// goDecoder decodes HTTP MP3 streams in Go instead of ffmpeg (godecode.go);
	// goCancel stops the one running.
	goDecoder bool
	goCancel  context.CancelFunc

	// mp3 relays the stream's own MP3 frames to browsers (mp3relay.go), fed
	// by the Go decoder.
	mp3 *mp3Relay
	// ulaw relays the decoded sound in mu-law to pages that play it
	// themselves (ulaw.go), fed by the Go decoder.
	ulaw *mp3Relay

	// activity says when the frequency carries sound (activity.go); nil
	// until SetActivityCallback.
	activity *activityMeter
}

// CentralProcessorConfig contains configuration for the central audio processor
type CentralProcessorConfig struct {
	FFmpegPath               string
	SampleRate               int
	Channels                 int
	Format                   string
	ReconnectDelay           time.Duration
	FFmpegTimeoutSecs        int // FFmpeg connection timeout in seconds (0 = no timeout)
	FFmpegReconnectDelaySecs int // FFmpeg reconnect delay in seconds

	// NoFFmpegReconnect leaves reconnecting to this processor, which restarts
	// ffmpeg after ReconnectDelay, instead of asking ffmpeg to do it. ffmpeg's
	// own reconnection retries a stream that has ended or answers 404 in a tight
	// loop, which is right for a stream that always exists and wrong for one a
	// station creates and removes as it changes what it listens to.
	NoFFmpegReconnect bool

	// InputOptions go on ffmpeg's command line before -i, as the source gives
	// them: the format and rate of raw PCM over UDP, or the capture device of
	// a receiver plugged into the sound card. Empty for a stream ffmpeg
	// recognises by itself, which is every HTTP one.
	InputOptions []string

	// GoDecoder decodes an HTTP MP3 stream in Go (godecode.go) rather than with
	// ffmpeg; other sources still go through ffmpeg.
	GoDecoder bool
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

	// "ffmpeg" wherever it is installed, not only on the PATH of whatever
	// started the server (pkg/tool): from launchd or `open` the PATH holds none
	// of Homebrew, and production had no audio on 26/10.
	ffmpegPath, _ := tool.Find(config.FFmpegPath)

	// Create multi-reader for sharing the stream
	multiReader := NewMultiReader(procCtx, logger.Named("multi-reader"))

	return &CentralAudioProcessor{
		id:                       id,
		audioURL:                 audioURL,
		ffmpegPath:               ffmpegPath,
		sampleRate:               config.SampleRate,
		channels:                 config.Channels,
		ffmpegTimeoutSecs:        config.FFmpegTimeoutSecs,
		ffmpegReconnectDelaySecs: config.FFmpegReconnectDelaySecs,
		noFFmpegReconnect:        config.NoFFmpegReconnect,
		inputOptions:             config.InputOptions,
		goDecoder:                config.GoDecoder,
		mp3:                      newMP3Relay(),
		ulaw:                     newRelay(ulawBurstChunks),
		multiReader:              multiReader,
		ctx:                      procCtx,
		cancel:                   procCancel,
		logger:                   logger.Named("central-audio-processor").With(String("id", id)),
		isRunning:                false,
		lastActivity:             time.Now(),
		contentType:              "audio/wav", // We'll be serving WAV format
		format:                   config.Format,
		reconnectDelay:           config.ReconnectDelay,
	}, nil
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
		Int("sample_rate", p.sampleRate),
		Int("channels", p.channels))

	// Notify connecting status
	p.notifyStatusChange(StatusConnecting, "")

	if err := p.startFFmpeg(); err != nil {
		p.notifyStatusChange(StatusFailed, err.Error())
		return fmt.Errorf("failed to start ffmpeg: %w", err)
	}

	// Start monitoring
	p.startMonitoring()

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

	// Stop monitoring
	if p.monitorTicker != nil {
		p.monitorTicker.Stop()
		p.monitorTicker = nil
	}

	// Cancel context to stop all operations
	p.cancel()

	p.stopFFmpeg()
	p.mp3.close()
	p.ulaw.close()

	// Close multi-reader
	p.multiReader.Close()

	p.isRunning = false
	// Notify stopped status
	p.notifyStatusChange(StatusStopped, "")
	return nil
}

// startFFmpeg starts the ffmpeg process for HTTP streams
func (p *CentralAudioProcessor) startFFmpeg() error {
	if p.goDecoder && goDecodes(p.audioURL, p.inputOptions, p.channels, p.format) {
		return p.startGoDecoder()
	}
	p.logger.Debug("Starting ffmpeg process",
		String("path", p.ffmpegPath),
		String("url", p.audioURL))

	// Create ffmpeg command with enhanced arguments
	p.ffmpegCmd = exec.CommandContext(p.ctx, p.ffmpegPath, p.ffmpegArgs()...)

	// Get stdout pipe
	var err error
	p.ffmpegStdout, err = p.ffmpegCmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	// Start ffmpeg
	if err := p.ffmpegCmd.Start(); err != nil {
		return fmt.Errorf("failed to start ffmpeg: %w", err)
	}

	// Start copying data from ffmpeg to multi-reader
	go p.processFFmpegOutput()

	return nil
}

// ffmpegArgs is ffmpeg's command line for this processor's stream.
//
// The timeout and reconnection knobs are options of ffmpeg's HTTP protocol:
// given with any other input -- raw PCM over UDP, a capture device -- ffmpeg
// stops with "Option reconnect not found" (ffmpeg 8.0.1, measured 10/10). They
// go only with http and https; the source's own input options cover the rest.
func (p *CentralAudioProcessor) ffmpegArgs() []string {
	args := []string{
		"-loglevel", "error", // Minimal logging
		"-fflags", "nobuffer", // Disable input buffering
		"-flags", "low_delay", // Enable low delay mode
	}

	if isHTTP(p.audioURL) {
		// Add timeout if configured (convert seconds to microseconds)
		if p.ffmpegTimeoutSecs > 0 {
			args = append(args, "-timeout", fmt.Sprintf("%d", p.ffmpegTimeoutSecs*1000000))
		}
		if !p.noFFmpegReconnect {
			args = append(args,
				"-reconnect", "1", // Enable reconnection
				"-reconnect_at_eof", "1", // Reconnect at end of file
				"-reconnect_streamed", "1", // Reconnect for streamed inputs
				"-reconnect_delay_max", fmt.Sprintf("%d", p.ffmpegReconnectDelaySecs), // Configurable reconnect delay
			)
		}
	}
	args = append(args, p.inputOptions...)
	args = append(args,
		"-i", p.audioURL, // Input URL, or a capture device named by the input options
		"-f", p.format, // Output format (should be s16le for raw PCM)
		"-acodec", "pcm_s16le", // Audio codec
		"-ac", fmt.Sprintf("%d", p.channels), // Channels
		"-ar", fmt.Sprintf("%d", p.sampleRate), // Sample rate
		"-flush_packets", "1", // Flush packets immediately
		"pipe:1", // Output to stdout
	)
	return args
}

// isHTTP reports that ffmpeg will read the source with its HTTP protocol.
func isHTTP(audioURL string) bool {
	u := strings.ToLower(audioURL)
	return strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://")
}

// stopFFmpeg stops the ffmpeg process
func (p *CentralAudioProcessor) stopFFmpeg() {
	p.stopGoDecoder()
	if p.ffmpegCmd != nil && p.ffmpegCmd.Process != nil {
		p.logger.Info("Stopping ffmpeg process")

		// Try to kill the process, but don't log errors during shutdown
		// These errors are expected as ffmpeg may already be terminated
		_ = p.ffmpegCmd.Process.Kill()

		// Wait for the process to exit, but don't log errors
		// The exit status might be non-zero or the process might already be gone
		_ = p.ffmpegCmd.Wait()
	}

	if p.reconnectTimer != nil {
		p.reconnectTimer.Stop()
		p.reconnectTimer = nil
	}
}

// processFFmpegOutput processes the output from ffmpeg
func (p *CentralAudioProcessor) processFFmpegOutput() {
	p.logger.Info("Starting to process ffmpeg output")

	// Create buffer for reading
	buffer := make([]byte, 4096)
	bytesProcessed := 0
	lastLogTime := time.Now()

	for {
		select {
		case <-p.ctx.Done():
			p.logger.Info("Context canceled, stopping ffmpeg output processing",
				Int("total_bytes_processed", bytesProcessed))
			return
		default:
			// Read from ffmpeg
			n, err := p.ffmpegStdout.Read(buffer)
			if err != nil {
				if err == io.EOF {
					p.logger.Warn("FFmpeg output ended unexpectedly",
						Int("total_bytes_processed", bytesProcessed),
						String("duration_since_start", time.Since(lastLogTime).String()))
				} else {
					p.logger.Error("Error reading from ffmpeg", Error(err),
						Int("total_bytes_processed", bytesProcessed),
						String("duration_since_start", time.Since(lastLogTime).String()))
					p.lastError = err
				}

				// Attempt to restart ffmpeg after a delay
				p.mu.Lock()
				if p.isRunning && p.reconnectTimer == nil {
					p.logger.Warn("Scheduling ffmpeg restart due to read error",
						String("error_type", fmt.Sprintf("%T", err)),
						String("error_message", err.Error()))
					// Notify failed status
					p.notifyStatusChange(StatusFailed, err.Error())
					p.reconnectTimer = time.AfterFunc(p.reconnectDelay, func() {
						p.mu.Lock()
						defer p.mu.Unlock()

						p.reconnectTimer = nil
						if p.isRunning {
							p.logger.Info("Executing scheduled ffmpeg restart")
							p.notifyStatusChange(StatusConnecting, "")
							p.stopFFmpeg()
							if err := p.startFFmpeg(); err != nil {
								p.logger.Error("Failed to restart ffmpeg", Error(err))
								p.notifyStatusChange(StatusFailed, err.Error())
							} else {
								p.logger.Info("FFmpeg restarted successfully")
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
					p.logger.Debug("FFmpeg processing progress",
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

// startMonitoring starts monitoring the audio source (ffmpeg or SRT)
func (p *CentralAudioProcessor) startMonitoring() {
	// The loop keeps its own copy: Stop sets p.monitorTicker to nil, and reading
	// the field here raced with that and crashed on the nil ticker -- once per
	// stop, at worst, which removing sources at runtime makes routine.
	ticker := time.NewTicker(5 * time.Second)
	p.monitorTicker = ticker

	go func() {
		for {
			select {
			case <-p.ctx.Done():
				return
			case <-ticker.C:
				p.mu.Lock()
				// Monitor ffmpeg process
				if p.isRunning && p.ffmpegCmd != nil && p.ffmpegCmd.ProcessState != nil {
					p.logger.Warn("FFmpeg process has exited unexpectedly")

					if p.isRunning && p.reconnectTimer == nil {
						p.logger.Info("Restarting ffmpeg after unexpected exit")
						p.notifyStatusChange(StatusConnecting, "Reconnecting after process exit")
						p.stopFFmpeg()
						if err := p.startFFmpeg(); err != nil {
							p.logger.Error("Failed to restart ffmpeg", Error(err))
							p.notifyStatusChange(StatusFailed, err.Error())
						} else {
							p.notifyStatusChange(StatusConnected, "")
						}
					}
				}
				p.mu.Unlock()
			}
		}
	}()
}

// CreateReader creates a new reader for the audio stream (with WAV header for browser playback)
func (p *CentralAudioProcessor) CreateReader(id string) (io.ReadCloser, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.isRunning {
		if err := p.startFFmpeg(); err != nil {
			return nil, fmt.Errorf("failed to start processor: %w", err)
		}
		p.isRunning = true
	}

	// Create a reader with WAV header
	reader := p.multiReader.CreateReader(id)
	return NewWAVReader(reader, p.sampleRate, p.channels), nil
}

// CreateRawReader creates a new reader for raw PCM audio (no WAV header, for transcription)
func (p *CentralAudioProcessor) CreateRawReader(id string) (io.ReadCloser, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.isRunning {
		if err := p.startFFmpeg(); err != nil {
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
