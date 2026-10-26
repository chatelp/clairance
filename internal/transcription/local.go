package transcription

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/yegors/co-atc/internal/storage/sqlite"
	"github.com/yegors/co-atc/internal/websocket"
	"github.com/yegors/co-atc/pkg/logger"
)

// LocalProcessor transcribes a frequency with a local model instead of the OpenAI
// realtime API, by posting audio to the sidecar described in docs/LOCAL-STT.md.
//
// The realtime API took a continuous stream and told us where speech began and
// ended. A local model does not, so this processor cuts the stream into
// transmissions itself. That turns out to be easy on an SDR feed: RTLSDR-Airband
// emits digital silence between transmissions — measured at -90 dBFS exactly, not
// a noise floor — so squelch openings are unambiguous. On a noisier source the
// maximum-segment guard still bounds how long a segment can grow.
type LocalProcessor struct {
	frequencyID string
	language    string
	audioReader io.ReadCloser
	config      Config
	sink        *transcriptionSink
	client      *http.Client
	timing      *TranscriptionTiming // shared by every frequency; nil records nothing
	ctx         context.Context
	cancel      context.CancelFunc
	logger      *logger.Logger
}

// NewLocalProcessor builds a processor backed by the local STT sidecar.
func NewLocalProcessor(
	ctx context.Context,
	frequencyID string,
	audioReader io.ReadCloser,
	config Config,
	wsServer *websocket.Server,
	storage *sqlite.TranscriptionStorage,
	log *logger.Logger,
	fileLogger *FileLogger,
	timing *TranscriptionTiming,
) (ProcessorInterface, error) {
	if config.Local.ServerURL == "" {
		return nil, fmt.Errorf("transcription.local.server_url is required when backend = \"local\"")
	}

	// The language comes from the frequency catalogue, not from the model. Measured
	// on this station, automatic detection is wrong on 16% of French and 28% of
	// English transmissions, and it fails hardest on clips with no usable speech.
	// The catalogue does not have that problem.
	language := config.FrequencyLanguages[frequencyID]
	if language == "" {
		language = config.Language
	}
	if language == "" {
		language = "en"
	}

	timeout := config.Local.TimeoutSeconds
	if timeout <= 0 {
		timeout = 60 // a zero Timeout means "no limit", which would hang on a dead sidecar
	}

	procCtx, procCancel := context.WithCancel(ctx)
	return &LocalProcessor{
		frequencyID: frequencyID,
		language:    language,
		audioReader: audioReader,
		config:      config,
		sink: &transcriptionSink{
			frequencyID: frequencyID,
			storage:     storage,
			wsServer:    wsServer,
			fileLogger:  fileLogger,
			logger:      log,
		},
		client: &http.Client{Timeout: time.Duration(timeout) * time.Second},
		timing: timing,
		ctx:    procCtx,
		cancel: procCancel,
		logger: log.Named("local-xscribe").With(String("frequency_id", frequencyID)),
	}, nil
}

func (p *LocalProcessor) Start() error {
	p.logger.Info("Starting local transcription",
		String("server", p.config.Local.ServerURL),
		String("language", p.language))
	go p.run()
	return nil
}

func (p *LocalProcessor) Stop() error {
	p.logger.Info("Stopping local transcription")
	p.cancel()
	return p.audioReader.Close()
}

// run reads PCM, cuts it into transmissions, and sends each one to the sidecar.
func (p *LocalProcessor) run() {
	rate := p.config.SampleRate
	if rate <= 0 {
		rate = 24000
	}
	frame := rate / 50 // 20 ms of s16le mono
	buf := make([]byte, frame*2)

	seg := newSegmenter(rate, p.config.Local)
	// Transmissions are dated by where they sit in the stream (stream_clock.go):
	// those cut while a server's burst is still arriving wait until it is in.
	clock := newStreamClock(rate)
	type cut struct {
		pcm  []byte
		mark leadAt
	}
	var waiting []cut
	var replays replayFilter
	// release sends what waits for a date, once the clock knows it -- or at
	// once when forced, at a reconnection, while the clock is still the old
	// connection's.
	release := func(now time.Time, force bool) {
		if len(waiting) == 0 || !(force || clock.settled() || now.Sub(waiting[0].mark.at) > maxWaitForDate) {
			return
		}
		for _, c := range waiting {
			date := clock.date(c.mark)
			if !replays.fresh(date) {
				p.logger.Info("Transmission replayed by a reconnection, not transcribed again",
					String("dated", date.Format("15:04:05.000")))
				continue
			}
			go p.send(c.pcm, rate, date)
		}
		waiting = waiting[:0]
	}
	var lastData time.Time
	var lastMark leadAt
	for {
		select {
		case <-p.ctx.Done():
			return
		default:
		}

		n, err := io.ReadFull(p.audioReader, buf)
		if err != nil {
			if p.ctx.Err() != nil {
				return
			}
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				if n == 0 {
					p.logger.Debug("Audio reader drained, waiting")
					time.Sleep(200 * time.Millisecond)
					continue
				}
			} else {
				p.logger.Error("Audio read failed", Error(err))
				return
			}
		}

		now := time.Now()
		if !lastData.IsZero() && now.Sub(lastData) > reconnectGap {
			// A gap in delivery is a reconnection. A transmission it interrupted
			// ends there, dated by the old connection's clock: glued to the new
			// connection's audio, whose burst starts in the past, it was dated
			// forty seconds early in a test (docs-fr/05-decisions.md, D57).
			if seg.speaking {
				if pcm := seg.close(); pcm != nil {
					waiting = append(waiting, cut{pcm, lastMark})
				}
			}
			seg.pre = seg.pre[:0]
			release(now, true)
		}
		clock.received(n/2, now)
		lastData, lastMark = now, clock.mark(now)
		if pcm := seg.push(buf[:n]); pcm != nil {
			waiting = append(waiting, cut{pcm, lastMark})
		}
		// A stream that never settles still gets transcribed, dated as best known.
		release(now, false)
	}
}

// maxWaitForDate bounds how long a transmission waits for a burst to end.
const maxWaitForDate = 10 * time.Second

// send posts one transmission and stores whatever comes back.
func (p *LocalProcessor) send(pcm []byte, rate int, at time.Time) {
	req, err := http.NewRequestWithContext(p.ctx, "POST",
		p.config.Local.ServerURL+"/transcribe", bytes.NewReader(pcm))
	if err != nil {
		p.logger.Error("Failed to build sidecar request", Error(err))
		return
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("X-Sample-Rate", strconv.Itoa(rate))
	req.Header.Set("X-Channels", "1")
	req.Header.Set("X-Language", p.language)
	req.Header.Set("X-Frequency-Id", p.frequencyID)
	// The transmission's own date, so an archived clip joins its database row
	// exactly: X-Created-At is the created_at this row will be stored with (UTC), and
	// X-Segment-At the same moment in UTC to the millisecond. The sidecar's clock
	// cannot serve, since under a queue it sees a transmission long after it ended.
	req.Header.Set("X-Created-At", at.UTC().Format(time.RFC3339))
	req.Header.Set("X-Segment-At", at.UTC().Format("2006-01-02T15:04:05.000Z"))
	// The first reading now, a second one later (14/10): the sidecar answers
	// as soon as the English text is ready and keeps the French reading for
	// GET /second/{id}, in a lane that never delays another first reading.
	req.Header.Set("X-Second", "later")

	// From the request to the decoded answer: what one transmission costs,
	// waiting behind others in the sidecar's queue included.
	started := time.Now()
	p.timing.Sent(p.frequencyID, started)
	resp, err := p.client.Do(req)
	if err != nil {
		if p.ctx.Err() == nil {
			p.logger.Error("Sidecar request failed", Error(err))
		}
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		p.logger.Error("Sidecar returned an error",
			Int("status", resp.StatusCode), String("body", string(body)))
		return
	}

	var out struct {
		Text          string  `json:"text"`
		Language      string  `json:"language"`
		Duration      float64 `json:"duration"`
		SpeechSeconds float64 `json:"speech_seconds"`
		Rejected      string  `json:"rejected"`
		Realtime      float64 `json:"realtime_factor"`
		// A second reading of the same audio, present only when the sidecar's
		// French gate opened. Empty the rest of the time, which is most of it.
		SecondText     string `json:"second_text"`
		SecondLanguage string `json:"second_language"`
		// Set when a second reading may follow, asked for later.
		SecondPending string `json:"second_pending"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		p.logger.Error("Failed to decode sidecar response", Error(err))
		return
	}

	// A rejection is a result, not a failure: the sidecar heard no speech, or has
	// no model for this frequency's language. Storing nothing is the correct
	// outcome — a wrong-language transcript reads as a real one downstream.
	if out.Rejected != "" {
		p.logger.Debug("Transmission not transcribed",
			String("reason", out.Rejected),
			String("duration", fmt.Sprintf("%.1fs", out.Duration)))
		return
	}
	if out.Text == "" {
		return
	}
	p.timing.Record(time.Since(started))
	p.timing.Transcribed(p.frequencyID, time.Now())

	p.logger.Info("Transcribed",
		String("language", out.Language),
		String("duration", fmt.Sprintf("%.1fs", out.Duration)),
		String("speech", fmt.Sprintf("%.1fs", out.SpeechSeconds)),
		String("realtime", fmt.Sprintf("%.1fx", out.Realtime)),
		String("text", out.Text))

	if out.SecondText != "" {
		p.logger.Info("Second opinion",
			String("language", out.SecondLanguage),
			String("text", out.SecondText))
	}

	id, err := p.sink.emit(out.Text, at, out.Language, out.SecondText)
	if err != nil {
		p.logger.Error("Failed to store transcription", Error(err))
		return
	}
	if out.SecondPending != "" {
		go p.awaitSecond(id, out.SecondPending, time.Now())
	}
}

// MeanTranscriptionSeconds is the rolling mean of the time a transmission
// takes to transcribe, over every frequency; false until one has been.
func (p *LocalProcessor) MeanTranscriptionSeconds() (float64, bool) {
	return p.timing.Mean()
}

// TranscriptionTiming is an exponential rolling mean of the time the sidecar
// takes per transmission, request to decoded answer, counted for the
// transmissions it transcribed: a rejection (no speech, no model for the
// language) costs a fraction of a decode and would pull the figure down. The
// sign-in page shows it; the first transmission sets it, each next one moves
// it by timingAlpha of the difference.
//
// It also keeps, per frequency, when a transmission was last sent to the
// sidecar and when one last came back as text: audio arriving with no
// transmission cut is a quiet frequency, transmissions sent with no text back
// is a deaf transcription, and the launcher's health tells them apart by these
// two times (docs-fr/34).
type TranscriptionTiming struct {
	mu   sync.Mutex
	mean float64 // seconds
	seen bool

	sent        map[string]time.Time // frequency -> last transmission sent
	transcribed map[string]time.Time // frequency -> last one transcribed
}

// timingAlpha weighs the newest transmission: 0.1 is about the last ten.
const timingAlpha = 0.1

// Record adds one transmission's time.
func (t *TranscriptionTiming) Record(d time.Duration) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.seen {
		t.mean, t.seen = d.Seconds(), true
		return
	}
	t.mean += timingAlpha * (d.Seconds() - t.mean)
}

// Sent records that a frequency's transmission went to the sidecar.
func (t *TranscriptionTiming) Sent(frequencyID string, at time.Time) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.sent == nil {
		t.sent = map[string]time.Time{}
	}
	t.sent[frequencyID] = at
}

// Transcribed records that a frequency's transmission came back as text.
func (t *TranscriptionTiming) Transcribed(frequencyID string, at time.Time) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.transcribed == nil {
		t.transcribed = map[string]time.Time{}
	}
	t.transcribed[frequencyID] = at
}

// Activity returns when a frequency last sent a transmission and when one
// last came back as text; a zero time for what has not happened since start.
func (t *TranscriptionTiming) Activity(frequencyID string) (sent, transcribed time.Time) {
	if t == nil {
		return time.Time{}, time.Time{}
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.sent[frequencyID], t.transcribed[frequencyID]
}

// Mean returns the rolling mean in seconds, and false while nothing has been
// transcribed.
func (t *TranscriptionTiming) Mean() (float64, bool) {
	if t == nil {
		return 0, false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.mean, t.seen
}

// --------------------------------------------------------------------- segmenter

// segmenter cuts a continuous PCM stream into transmissions on squelch activity.
type segmenter struct {
	rate       int
	threshold  float64
	closeAfter int // frames of silence that end a transmission
	minFrames  int
	maxFrames  int
	preroll    int

	speaking bool
	silence  int
	frames   int
	buf      bytes.Buffer
	pre      [][]byte
}

func newSegmenter(rate int, c LocalSTTConfig) *segmenter {
	per := func(ms, def int) int {
		if ms <= 0 {
			ms = def
		}
		return ms / 20 // frames of 20 ms
	}
	th := c.SilenceThreshold
	if th <= 0 {
		th = 0.005 // ≈ -46 dBFS; the stream sits at -90 dBFS when squelch is closed
	}
	return &segmenter{
		rate:       rate,
		threshold:  th,
		closeAfter: per(c.SegmentSilenceMs, 600),
		minFrames:  per(c.SegmentMinMs, 400),
		maxFrames:  per(c.SegmentMaxSeconds*1000, 30000),
		preroll:    per(c.SegmentPrerollMs, 200),
	}
}

// push feeds one 20 ms frame and returns a finished transmission, or nil.
func (s *segmenter) push(frame []byte) []byte {
	loud := rms(frame) > s.threshold

	if !s.speaking {
		// Keep a short pre-roll so the first syllable is not clipped.
		s.pre = append(s.pre, append([]byte(nil), frame...))
		if len(s.pre) > s.preroll {
			s.pre = s.pre[1:]
		}
		if !loud {
			return nil
		}
		s.speaking = true
		s.silence = 0
		s.frames = 0
		s.buf.Reset()
		for _, f := range s.pre {
			s.buf.Write(f)
			s.frames++
		}
		s.pre = s.pre[:0]
	}

	s.buf.Write(frame)
	s.frames++
	if loud {
		s.silence = 0
	} else {
		s.silence++
	}

	if s.silence >= s.closeAfter || s.frames >= s.maxFrames {
		return s.close()
	}
	return nil
}

func (s *segmenter) close() []byte {
	s.speaking = false
	if s.frames-s.silence < s.minFrames {
		s.buf.Reset() // too short to be a transmission
		return nil
	}
	out := append([]byte(nil), s.buf.Bytes()...)
	s.buf.Reset()
	return out
}

func rms(frame []byte) float64 {
	if len(frame) < 2 {
		return 0
	}
	var sum float64
	n := len(frame) / 2
	for i := 0; i < n; i++ {
		v := float64(int16(binary.LittleEndian.Uint16(frame[i*2:]))) / 32768.0
		sum += v * v
	}
	return math.Sqrt(sum / float64(n))
}

// secondWaitTotal bounds how long a second reading is waited for: past it the
// sidecar has forgotten it, or the queue is so long it no longer matters.
const secondWaitTotal = 10 * time.Minute

// awaitSecond fetches a second reading asked for later, long-polling the
// sidecar a minute at a time, and stores it when it says something.
func (p *LocalProcessor) awaitSecond(id int64, key string, firstAt time.Time) {
	deadline := time.Now().Add(secondWaitTotal)
	client := &http.Client{Timeout: 75 * time.Second}
	for time.Now().Before(deadline) && p.ctx.Err() == nil {
		req, err := http.NewRequestWithContext(p.ctx, "GET",
			p.config.Local.ServerURL+"/second/"+url.PathEscape(key)+"?wait=60", nil)
		if err != nil {
			return
		}
		resp, err := client.Do(req)
		if err != nil {
			if p.ctx.Err() == nil {
				p.logger.Warn("Second reading not fetched", Error(err))
				time.Sleep(5 * time.Second)
				continue
			}
			return
		}
		var out struct {
			Status         string `json:"status"`
			SecondText     string `json:"second_text"`
			SecondLanguage string `json:"second_language"`
		}
		decodeErr := json.NewDecoder(resp.Body).Decode(&out)
		resp.Body.Close()
		if resp.StatusCode == http.StatusNotFound {
			return // the sidecar restarted, or forgot it
		}
		if decodeErr != nil || out.Status == "pending" {
			continue
		}
		if out.SecondText == "" {
			return // the French gate stayed shut
		}
		// "after": how long the exchange was shown without its French
		// reading, what the listener sees of D81.
		again := p.sink.second(id, out.SecondText)
		p.logger.Info("Second opinion",
			logger.Int64("id", id),
			String("after", fmt.Sprintf("%.1fs", time.Since(firstAt).Seconds())),
			logger.Bool("rematch", again),
			String("language", out.SecondLanguage),
			String("text", out.SecondText))
		return
	}
}
