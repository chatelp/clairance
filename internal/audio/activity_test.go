package audio

import (
	"encoding/binary"
	"math"
	"math/rand"
	"testing"
)

type activityEvent struct {
	speaking bool
	level    float64
}

func pcmOf(rate int, seconds float64, amp float64) []byte {
	n := int(float64(rate) * seconds)
	b := make([]byte, 2*n)
	for i := 0; i < n; i++ {
		v := amp * math.Sin(2*math.Pi*700*float64(i)/float64(rate))
		binary.LittleEndian.PutUint16(b[2*i:], uint16(int16(v*32767)))
	}
	return b
}

// Two seconds of speech, then silence: open at once, levels four times a
// second while it lasts, closed 600 ms into the silence; a short pause inside
// a transmission does not close it. However the bytes are cut, the same.
func TestActivityOpensAtSoundAndClosesAfterSixHundredMilliseconds(t *testing.T) {
	rate := 16000
	stream := append(append(append(pcmOf(rate, 1, 0.3), pcmOf(rate, 0.3, 0)...), pcmOf(rate, 1, 0.3)...), pcmOf(rate, 2, 0)...)
	for _, chunk := range []int{1, 7, 640, 4096} {
		var got []activityEvent
		m := newActivityMeter(rate, 0.005, func(s bool, l float64) { got = append(got, activityEvent{s, l}) })
		rng := rand.New(rand.NewSource(int64(chunk)))
		for at := 0; at < len(stream); {
			n := chunk
			if chunk == 1 {
				n = 1 + rng.Intn(9)
			}
			n = min(n, len(stream)-at)
			m.push(stream[at : at+n])
			at += n
		}
		opens, closes, levels := 0, 0, 0
		for _, e := range got {
			switch {
			case e.speaking && e.level > 0:
				levels++
			case !e.speaking:
				closes++
			}
			if e.speaking && opens == 0 {
				opens = 1
			}
		}
		if opens != 1 || closes != 1 || !got[0].speaking || got[len(got)-1].speaking {
			t.Errorf("chunks of %d: %v, want one opening, one closing, not two transmissions", chunk, got)
		}
		// 2.3 s of transmission, a level each 240 ms.
		if levels < 8 || levels > 12 {
			t.Errorf("chunks of %d: %d levels, want about 10", chunk, levels)
		}
		// A tone at -10.5 dBFS RMS reads near the top of the meter.
		if l := got[1].level; l < 0.9 {
			t.Errorf("chunks of %d: level %.2f for a loud tone", chunk, l)
		}
	}
}

// Silence at the squelch's -90 dBFS never opens.
func TestSilenceNeverOpens(t *testing.T) {
	m := newActivityMeter(8000, 0, func(bool, float64) { t.Fatal("silence reported as speech") })
	m.push(pcmOf(8000, 3, 0.00003))
}
