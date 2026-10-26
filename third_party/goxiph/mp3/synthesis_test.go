package mp3

import (
	"bufio"
	"math"
	"os"
	"strconv"
	"strings"
	"testing"
)

// impulseResponses reads the reference decoder's response to one impulse in each subband, which is
// what the window was recovered from and what the filterbank is checked against.
func impulseResponses(t *testing.T) map[int][]float64 {
	t.Helper()
	f, err := os.Open("testdata/impulse.txt")
	if err != nil {
		t.Skipf("no impulse responses: %v", err)
	}
	defer f.Close()

	out := map[int][]float64{}
	scan := bufio.NewScanner(f)
	scan.Buffer(make([]byte, 1<<20), 1<<20)
	for scan.Scan() {
		fields := strings.Fields(scan.Text())
		if len(fields) != 514 || fields[0] != "K" {
			continue
		}
		k, _ := strconv.Atoi(fields[1])
		row := make([]float64, 512)
		for i := range row {
			row[i], _ = strconv.ParseFloat(fields[i+2], 64)
		}
		out[k] = row
	}
	return out
}

// TestSynthesisReproducesTheReferenceImpulses is the filterbank's gate.
//
// One impulse in one subband, run through this decoder's synthesis, has to give what the reference
// gave for the same impulse. The filterbank is linear, so agreeing on every subband's response means
// agreeing on every signal — there is nothing else it can do.
//
// This checks the window, the cosine matrix, the shuffle from the delay line into the window, and the
// order the delay line is kept in. A fault in any of them shows here.
func TestSynthesisReproducesTheReferenceImpulses(t *testing.T) {
	want := impulseResponses(t)
	if len(want) == 0 {
		t.Skip("no responses")
	}

	var worst float64
	worstAt := [2]int{-1, -1}
	for k, ref := range want {
		var s synthesiser
		var sub [32]float64
		var out [32]float64

		got := make([]float64, 0, 512)
		for slot := range 16 {
			sub = [32]float64{}
			if slot == 0 {
				sub[k] = 1
			}
			s.synthesise(&sub, &out)
			got = append(got, out[:]...)
		}

		var peak float64
		for _, v := range ref {
			peak = math.Max(peak, math.Abs(v))
		}
		for i := range ref {
			// Against the response's own peak, because a tap near a zero crossing has no scale of
			// its own to be measured by.
			d := math.Abs(got[i]-ref[i]) / peak
			if d > worst {
				worst, worstAt = d, [2]int{k, i}
			}
		}
	}

	t.Logf("%d subband responses compared, worst tap off by %.2e (subband %d, sample %d)",
		len(want), worst, worstAt[0], worstAt[1])
	// A part in ten thousand of the response's peak. The reference works in single precision and the
	// window was measured from it, so agreement closer than that is not on offer.
	if worst > 1e-4 {
		t.Errorf("the filterbank differs from the reference by %.2e of the peak", worst)
	}
}

// The delay line is half a second of history, so a decoder that starts part way through a stream, or
// seeks, has to throw it away rather than play what the previous audio left in it.
func TestSynthesisResetClearsTheHistory(t *testing.T) {
	var s synthesiser
	var sub, out [32]float64
	sub[3] = 1
	for range 4 {
		s.synthesise(&sub, &out)
	}

	var fresh synthesiser
	s.reset()
	sub = [32]float64{}
	var after, clean [32]float64
	s.synthesise(&sub, &after)
	fresh.synthesise(&sub, &clean)
	if after != clean {
		t.Error("a reset filterbank still carries what came before it")
	}
}

// The window is what the whole filterbank rests on, and it was measured rather than transcribed. Its
// shape is the check: a prototype low-pass window rises to the middle and falls away, and the taps
// that could not be measured are the ones the cosine kills.
func TestSynthWindowShape(t *testing.T) {
	var peak float64
	peakAt := -1
	for i, v := range synthWindow {
		if math.Abs(v) > peak {
			peak, peakAt = math.Abs(v), i
		}
	}
	if peakAt < 240 || peakAt > 272 {
		t.Errorf("the window peaks at tap %d, want the middle", peakAt)
	}
	if synthWindow[0] != 0 {
		t.Errorf("the window starts at %g, want nothing", synthWindow[0])
	}
	// The ends are tiny against the middle, which is what makes it a window rather than a rectangle.
	for _, i := range []int{1, 2, 3, 509, 510, 511} {
		if math.Abs(synthWindow[i]) > peak/1000 {
			t.Errorf("tap %d is %g against a peak of %g", i, synthWindow[i], peak)
		}
	}
}

// The cosine matrix is computed, so what is worth checking is that it is the right cosine: the rows
// that the specification makes vanish have to vanish, since those are the taps the window could not
// be measured at.
func TestSynthMatrixVanishesWhereItShould(t *testing.T) {
	zero := 0
	for i := range 64 {
		all := true
		for k := range 32 {
			if math.Abs(synthMatrix[i][k]) > 1e-12 {
				all = false
				break
			}
		}
		if all {
			zero++
		}
	}
	if zero != 1 {
		t.Errorf("%d rows of the matrix vanish, want the one at sixteen", zero)
	}
	for k := range 32 {
		if math.Abs(synthMatrix[16][k]) > 1e-12 {
			t.Errorf("row sixteen does not vanish at subband %d", k)
		}
	}
}

// The filterbank computes thirty-one of its sixty-four rows and derives the rest, so the symmetry it
// derives them by is now load-bearing. It is a property of the cosine rather than of the code, which
// is exactly why it needs a test: nothing else would notice if the matrix moved under it.
func TestSynthMatrixSymmetryHoldsWhereTheFilterbankAssumesIt(t *testing.T) {
	same := func(a, b [32]float64, sign float64) bool {
		for k := range 32 {
			if math.Abs(a[k]-sign*b[k]) > 1e-12 {
				return false
			}
		}
		return true
	}

	// The two constant rows are constant to within what the cosine of a right angle comes to in
	// floating point, which is not zero but a few times 1e-15 at the far end of the row. The filterbank substitutes the exact value, so it
	// is closer to the truth than the matrix is rather than an approximation of it.
	for k := range 32 {
		if math.Abs(synthMatrix[16][k]) > 1e-13 {
			t.Fatalf("row 16 is not zero at %d: %g", k, synthMatrix[16][k])
		}
		if math.Abs(synthMatrix[48][k]+1) > 1e-13 {
			t.Fatalf("row 48 is not minus one at %d: %g", k, synthMatrix[48][k])
		}
	}
	for i := range 16 {
		if !same(synthMatrix[32-i], synthMatrix[i], -1) {
			t.Errorf("row %d is not the negative of row %d", 32-i, i)
		}
	}
	for i := 33; i < 48; i++ {
		if !same(synthMatrix[96-i], synthMatrix[i], 1) {
			t.Errorf("row %d does not repeat row %d", 96-i, i)
		}
	}
}

// The filterbank is the largest single cost in an MP3 decode, so it is benchmarked on its own: the
// end-to-end figure moves by a few percent when it changes and the machine's own noise is larger
// than that.
func BenchmarkSynthesise(b *testing.B) {
	var s synthesiser
	var sub, out [32]float64
	for i := range sub {
		sub[i] = math.Sin(float64(i) * 0.7)
	}
	b.ReportAllocs()
	for range b.N {
		s.synthesise(&sub, &out)
	}
}
