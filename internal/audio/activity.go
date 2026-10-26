package audio

import (
	"encoding/binary"
	"math"
	"sync"
)

// Someone speaking on a frequency, measured by the server on the audio it
// decodes, for every page (docs-fr/35-plan.md, D2 bis). The page measured it
// itself, from the stream it plays, through Web Audio: Safari plays a live
// stream without routing it there (26/10: sound, but no "someone is
// speaking" and no level), a page that does not listen measured nothing, and
// a friend without the sound saw nothing either.

// ActivityCallback is told when a frequency starts or stops carrying sound,
// and the level while it does, from 0 to 1, four times a second.
type ActivityCallback func(frequencyID string, speaking bool, level float64)

const (
	activityFrameMs     = 20  // the transcription's own frame
	activityCloseMs     = 600 // silence that ends a transmission, as the transcription's segment_silence_ms
	activityReportEvery = 12  // frames between two levels while speaking: 240 ms
)

// activityMeter follows a stream of 16-bit mono PCM: open on a frame above the
// threshold, closed after activityCloseMs of frames below it.
type activityMeter struct {
	mu         sync.Mutex
	frameBytes int
	threshold  float64
	closeAfter int
	report     func(speaking bool, level float64)

	pending  []byte
	speaking bool
	silent   int
	frames   int
	loudest  float64
}

func newActivityMeter(rate int, threshold float64, report func(bool, float64)) *activityMeter {
	if threshold <= 0 {
		threshold = 0.005 // the transcription's default: the stream sits at -90 dBFS when squelch is closed
	}
	return &activityMeter{
		frameBytes: rate * activityFrameMs / 1000 * 2,
		threshold:  threshold,
		closeAfter: activityCloseMs / activityFrameMs,
		report:     report,
	}
}

// push takes the next PCM bytes, in any size.
func (a *activityMeter) push(pcm []byte) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.pending = append(a.pending, pcm...)
	for len(a.pending) >= a.frameBytes {
		a.frame(a.pending[:a.frameBytes])
		a.pending = a.pending[a.frameBytes:]
	}
	if cap(a.pending) > 8*a.frameBytes && len(a.pending) < a.frameBytes {
		a.pending = append([]byte(nil), a.pending...)
	}
}

func (a *activityMeter) frame(f []byte) {
	var sum float64
	n := len(f) / 2
	for i := 0; i < n; i++ {
		v := float64(int16(binary.LittleEndian.Uint16(f[2*i:]))) / 32768
		sum += v * v
	}
	rms := math.Sqrt(sum / float64(n))
	loud := rms > a.threshold
	if loud {
		a.silent = 0
		a.loudest = math.Max(a.loudest, rms)
		if !a.speaking {
			a.speaking, a.frames = true, 0
			a.report(true, levelOf(rms))
			a.loudest = 0
			return
		}
	} else if a.speaking {
		if a.silent++; a.silent >= a.closeAfter {
			a.speaking, a.loudest = false, 0
			a.report(false, 0)
			return
		}
	}
	if a.speaking {
		if a.frames++; a.frames%activityReportEvery == 0 {
			a.report(true, levelOf(a.loudest))
			a.loudest = 0
		}
	}
}

// levelOf maps an RMS to 0..1 over -60 to -10 dBFS, the range the page's
// meter showed from Web Audio.
func levelOf(rms float64) float64 {
	if rms <= 0 {
		return 0
	}
	db := 20 * math.Log10(rms)
	return math.Max(0, math.Min(1, (db+60)/50))
}

// SetActivityCallback has the processor say when its frequency carries sound
// (activity.go), measured on the PCM it decodes with the transcription's
// silence threshold.
func (p *CentralAudioProcessor) SetActivityCallback(threshold float64, cb ActivityCallback) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.activity = newActivityMeter(p.sampleRate, threshold, func(speaking bool, level float64) {
		cb(p.id, speaking, level)
	})
}
