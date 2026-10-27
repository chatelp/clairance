package audio

import (
	"io"
	"time"
)

// The sound for a page that plays it itself (docs-fr/35-plan.md, D2 ter;
// decided by the owner on 26/10): what the Go decoder has decoded, at the
// stream's own rate, mono, in G.711 mu-law -- one byte a sample, 64 kb/s at
// the station's 8 kHz, the telephone's coding, made for the narrow AM voice of
// the airband. The page decodes it with a 256-entry table and plays it through
// Web Audio: it starts at once, and what it measures is what it plays. An
// <audio> element did neither (Chromium waited 17 s before playing the MP3,
// Safari played it past Web Audio).

// ulawBurstChunks is what a new listener gets at once: four decoded frames,
// about 0.3 s at the station's 72 ms, enough for the page's own reserve.
const ulawBurstChunks = 4

// ulawOf codes one sample in [-1, 1] in G.711 mu-law.
func ulawOf(v float64) byte {
	const bias, clip = 0x84, 32635
	s := int(v * 32768)
	sign := 0
	if s < 0 {
		s, sign = -s, 0x80
	}
	if s > clip {
		s = clip
	}
	s += bias
	exp := 7
	for mask := 0x4000; s&mask == 0 && exp > 0; mask >>= 1 {
		exp--
	}
	mant := (s >> (exp + 3)) & 0x0F
	return ^byte(sign | exp<<4 | mant)
}

// ulawChunk codes a block of mono samples.
func ulawChunk(mono []float64) []byte {
	out := make([]byte, len(mono))
	for i, v := range mono {
		out[i] = ulawOf(v)
	}
	return out
}

// ULawReader is a listener to the stream's decoded sound in mu-law, at the
// stream's own rate (the second value), for an MP3 stream; false otherwise
// (raw PCM over UDP), and the caller serves the WAV stream. Like MP3Reader,
// a stream that has not given its first frame yet is waited for.
func (p *CentralAudioProcessor) ULawReader(wait time.Duration) (io.ReadCloser, int, bool) {
	if p.ulaw == nil {
		return nil, 0, false
	}
	deadline := time.Now().Add(wait)
	for {
		if r, ok := p.ulaw.reader(); ok {
			p.ulaw.mu.Lock()
			rate := p.ulaw.format[0]
			p.ulaw.mu.Unlock()
			return r, rate, true
		}
		if !isHTTP(p.audioURL) || time.Now().After(deadline) {
			return nil, 0, false
		}
		time.Sleep(100 * time.Millisecond)
	}
}
