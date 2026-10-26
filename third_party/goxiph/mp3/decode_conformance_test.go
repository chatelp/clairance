//go:build conformance

package mp3

import (
	"encoding/binary"
	"errors"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/yegors/co-atc/third_party/goxiph/audio"
)

// TestConformanceAgainstTheReferencePCM is the gate everything else was leading to.
//
// The compliance set pairs each bitstream with the reference decoder's output for it, so the whole
// decoder can be judged at once against the number the standard itself defines. ISO/IEC 11172-4
// annex A gives two: full compliance wants the root-mean-square difference below 2^-15 over the
// square root of twelve, with no sample off by more than 2^-14; limited accuracy wants the difference
// below 2^-11 over the same root. In the units the reference output is stored in — sixteen bits, so
// a full-scale of 32768 — those come to about 101 dB and 77 dB of signal to noise.
//
// The first frames of a stream are not decodable by anyone: their main data begins before the file
// does, and the filterbank's delay line is empty. They are skipped on both sides rather than counted
// as error.
func TestConformanceAgainstTheReferencePCM(t *testing.T) {
	// The MPEG-1 set and the half rate one, which the compliance files name apart.
	full, limited := gradeVectors(t, "l3-*.bit")
	lsfFull, lsfLimited := gradeVectors(t, "M2L3_*.bit")
	full, limited = full+lsfFull, limited+lsfLimited

	if full < 17 {
		t.Errorf("%d layer III streams meet full compliance, below the 17 held", full)
	}
	if full+limited < 17 {
		t.Errorf("%d layer III streams reach limited accuracy or better, below the 17 held", full+limited)
	}
}

// Layers I and II have their own compliance set, judged by the same two numbers. They share the
// filterbank with layer III and nothing else, so they are graded apart.
func TestConformanceLayer1And2AgainstTheReferencePCM(t *testing.T) {
	for _, tc := range []struct {
		pattern       string
		full, reached int
	}{
		{"l1-*.bit", 8, 8},
		{"l2-*.bit", 8, 8},
	} {
		t.Run(tc.pattern, func(t *testing.T) {
			full, limited := gradeVectors(t, tc.pattern)
			if full < tc.full {
				t.Errorf("%d streams meet full compliance, below the %d held", full, tc.full)
			}
			if full+limited < tc.reached {
				t.Errorf("%d streams reach limited accuracy or better, below the %d held",
					full+limited, tc.reached)
			}
		})
	}
}

// gradeVectors decodes every vector matching a pattern and reports how many reach each bar.
func gradeVectors(t *testing.T, pattern string) (full, limited int) {
	t.Helper()
	files, err := filepath.Glob(vectorDir + "/" + pattern)
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(files)
	if len(files) == 0 {
		t.Skip("no vectors")
	}

	poor, skipped := 0, 0
	worstName, worstDB := "", math.Inf(1)

	for _, path := range files {
		name := strings.TrimSuffix(filepath.Base(path), ".bit")
		if strings.Contains(name, "nonstandard") {
			continue
		}
		want, err := readReferencePCM(path[:len(path)-4] + ".pcm")
		if err != nil {
			continue
		}
		got, channels, err := decodeAll(path)
		if err != nil || len(got) == 0 {
			skipped++
			continue
		}

		db, shift, ok := signalToNoise(got, want, channels)
		if !ok {
			skipped++
			continue
		}
		switch {
		case db >= 101:
			full++
		case db >= 77:
			limited++
		default:
			poor++
		}
		if db < worstDB {
			worstDB, worstName = db, name
		}
		t.Logf("%-28s %6.1f dB  (aligned %+d granules)", name, db, shift)
	}

	total := full + limited + poor
	if total == 0 {
		t.Skip("nothing decoded")
	}
	t.Logf("%d streams: %d fully compliant, %d to limited accuracy, %d below it; %d skipped",
		total, full, limited, poor, skipped)
	t.Logf("the worst is %s at %.1f dB", worstName, worstDB)
	return full, limited
}

// quantise16 puts a sample on the same grid the reference output is stored on.
//
// Both sides have to be quantised, and saturated, before they are compared. Layer III overshoots full
// scale legitimately — one vector here reaches eight times it — and the reference's output is sixteen
// bit, so it clips. Comparing a float that overshoots against an integer that saturated reports a
// fault where there is none, and reports it as a catastrophe rather than a discrepancy.
func quantise16(v float32) int16 {
	x := math.Round(float64(v) * 32768)
	if x > 32767 {
		return 32767
	}
	if x < -32768 {
		return -32768
	}
	return int16(x)
}

// readReferencePCM reads the compliance set's own output, sixteen bits little endian, interleaved.
func readReferencePCM(path string) ([]int16, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	out := make([]int16, len(raw)/2)
	for i := range out {
		out[i] = int16(binary.LittleEndian.Uint16(raw[2*i:]))
	}
	return out, nil
}

// decodeAll runs a whole stream through the decoder.
func decodeAll(path string) ([][]float32, int, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	defer f.Close()

	d, err := NewDecoder(f)
	if err != nil {
		return nil, 0, err
	}
	buf, err := audio.NewBuffer(d.Format(), 0)
	if err != nil {
		return nil, 0, err
	}
	channels := d.Format().Channels
	out := make([][]float32, channels)
	for {
		err := d.Next(buf)
		if errors.Is(err, io.EOF) {
			return out, channels, nil
		}
		if err != nil {
			return out, channels, nil
		}
		for ch := range channels {
			out[ch] = append(out[ch], buf.Data[ch][:buf.Frames()]...)
		}
	}
}

// signalToNoise compares a decode against the reference output, in decibels against full scale.
//
// The alignment is searched rather than assumed. How many frames at the head of a stream come out at
// all is a decoder's own decision — their main data begins before the file does and the filterbank
// has no history yet — so two decoders can legitimately differ by a frame or two at the start. A
// comparison lined up wrongly by even one frame reads as noise and says nothing about the audio, so
// the offset that fits best is the one measured, and how far it had to look is reported.
func signalToNoise(got [][]float32, want []int16, channels int) (float64, int, bool) {
	if len(got) == 0 || len(got[0]) == 0 || len(want) == 0 {
		return 0, 0, false
	}
	haveFrames := len(got[0])
	wantFrames := len(want) / channels

	best, bestShift, found := math.Inf(-1), 0, false
	for shift := -4 * 576; shift <= 4*576; shift += 576 {
		gotOff, wantOff := 0, 0
		if shift > 0 {
			gotOff = shift
		} else {
			wantOff = -shift
		}
		n := min(haveFrames-gotOff, wantFrames-wantOff)
		if n < 1152 {
			continue
		}
		var mse float64
		for ch := range channels {
			for i := range n {
				d := float64(quantise16(got[ch][gotOff+i])) - float64(want[(wantOff+i)*channels+ch])
				mse += d * d
			}
		}
		mse /= float64(n * channels)
		db := 99.0
		if mse > 0 {
			db = 10 * math.Log10(32767*32767/mse)
		}
		if db > best {
			best, bestShift, found = db, shift, true
		}
	}
	return best, bestShift / 576, found
}
