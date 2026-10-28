package mp3

import (
	"math"
	"testing"
)

// The power law keeps the sign and takes the magnitude to the four-thirds. Losing the sign is the
// obvious fault and it is silent: the spectrum still has the right shape, rectified.
func TestPow43KeepsTheSign(t *testing.T) {
	for _, v := range []int{-100, -16, -15, -1, 0, 1, 15, 16, 100, 8206} {
		got := pow43(v)
		want := math.Pow(math.Abs(float64(v)), 4.0/3)
		if v < 0 {
			want = -want
		}
		if math.Abs(got-want) > 1e-9*math.Max(1, math.Abs(want)) {
			t.Errorf("pow43(%d) is %g, want %g", v, got, want)
		}
	}
	if pow43(0) != 0 {
		t.Error("pow43(0) is not zero")
	}
}

// The gain and the scale factor are both powers of two in quarter steps, so a granule with neither
// leaves the power law alone and one with a known pair scales it by a known amount.
func TestRequantiseAppliesGainAndScaleFactor(t *testing.T) {
	h := Header{Version: MPEG1, Layer: Layer3, Mode: Mono, SampleRate: 44100}
	g := GranuleInfo{GlobalGain: dequantiserOffset}
	layout := layoutFor(h, g)

	var sp spectrum
	sp[0], sp[1], sp[2] = 1, -8, 27
	var sf scaleFactors
	var out [576]float32

	requantise(&sp, &sf, layout, g, false, &out)
	for i, want := range []float64{1, -16, 81} {
		if math.Abs(float64(out[i])-want) > 1e-4 {
			t.Errorf("bin %d is %g, want %g", i, out[i], want)
		}
	}

	// Four quarter steps of gain is one octave.
	g.GlobalGain = dequantiserOffset + 4
	requantise(&sp, &sf, layout, g, false, &out)
	if math.Abs(float64(out[0])-2) > 1e-4 {
		t.Errorf("an octave of gain gave %g, want 2", out[0])
	}

	// A scale factor takes it back down, in steps of its own. The step is two quarters per unit
	// unless the granule asks for four, so a factor of two cancels an octave of gain.
	g.GlobalGain = dequantiserOffset + 4
	sf.value[0] = 2
	requantise(&sp, &sf, layout, g, false, &out)
	if math.Abs(float64(out[0])-1) > 1e-4 {
		t.Errorf("a scale factor of two against an octave of gain gave %g, want 1", out[0])
	}

	// And the finer step halves that, so the same factor now takes off only half an octave.
	g.ScalefacScale = true
	requantise(&sp, &sf, layout, g, false, &out)
	if want := math.Exp2(-1.0); math.Abs(float64(out[0])-want) > 1e-4 {
		t.Errorf("the finer step gave %g, want %g", out[0], want)
	}
}

// Coupling a pair as a sum and a difference costs the granule two quarter steps, because what comes
// out is the sum of two channels rather than one of them.
func TestRequantiseAllowsForMidSide(t *testing.T) {
	h := Header{Version: MPEG1, Layer: Layer3, Mode: JointStereo, SampleRate: 44100}
	g := GranuleInfo{GlobalGain: dequantiserOffset}
	layout := layoutFor(h, g)

	var sp spectrum
	sp[0] = 1
	var sf scaleFactors
	var plain, coupled [576]float32
	requantise(&sp, &sf, layout, g, false, &plain)
	requantise(&sp, &sf, layout, g, true, &coupled)

	if want := float64(plain[0]) * math.Exp2(-0.5); math.Abs(float64(coupled[0])-want) > 1e-6 {
		t.Errorf("mid-side gave %g, want %g", coupled[0], want)
	}
}

// The bins above the last band are not described by anything and must come back as nothing.
func TestRequantiseClearsAboveTheBands(t *testing.T) {
	h := Header{Version: MPEG1, Layer: Layer3, Mode: Mono, SampleRate: 44100}
	g := GranuleInfo{GlobalGain: dequantiserOffset}

	var sp spectrum
	for i := range sp {
		sp[i] = 1
	}
	var sf scaleFactors
	var out [576]float32
	for i := range out {
		out[i] = 99
	}
	requantise(&sp, &sf, layoutFor(h, g), g, false, &out)
	for i, v := range out {
		if v == 99 {
			t.Fatalf("bin %d was left untouched", i)
		}
	}
}

// The intensity positions divide a right angle into twelfths, and the two ends are the whole of the
// band in one channel. A formula that does not reach them would need a special case, and one that
// overshoots would put more energy out than came in.
func TestIntensityPanSpansTheAngle(t *testing.T) {
	for _, tc := range []struct {
		pos         int
		left, right float64
	}{
		{0, 0, 1},
		{3, 0.5, 0.5},
		{6, 1, 0},
	} {
		l, r := intensityPan(tc.pos)
		if math.Abs(l-tc.left) > 1e-6 || math.Abs(r-tc.right) > 1e-6 {
			t.Errorf("position %d gives (%g, %g), want (%g, %g)", tc.pos, l, r, tc.left, tc.right)
		}
	}
	for pos := range 7 {
		if l, r := intensityPan(pos); math.Abs(l+r-1) > 1e-9 {
			t.Errorf("position %d shares %g of the band", pos, l+r)
		}
	}
}

// Mid and side is its own inverse up to a factor of two, which is what makes it free to apply.
func TestMidSideRecombines(t *testing.T) {
	h := Header{Version: MPEG1, Layer: Layer3, Mode: JointStereo, ModeExtension: 2, SampleRate: 44100}
	g := GranuleInfo{}
	var left, right [576]float32
	for i := range 576 {
		left[i] = float32(i%7) - 3
		right[i] = float32(i%5) - 2
	}
	mid, side := left, right

	var sf scaleFactors
	applyStereo(&left, &right, h, g, &sf, layoutFor(h, g), 0)
	for i := range 576 {
		if left[i] != mid[i]+side[i] || right[i] != mid[i]-side[i] {
			t.Fatalf("bin %d came back as (%g, %g)", i, left[i], right[i])
		}
	}
}

// At the lower sampling frequencies the shares are powers of two rather than angles, and which
// channel is halved alternates with the position.
func TestIntensitySharesAtLowerRates(t *testing.T) {
	h := Header{Version: MPEG2, Layer: Layer3, Mode: JointStereo, SampleRate: 22050}
	for pos := range 8 {
		l, r := intensityShares(pos, h, 0)
		if l != 1 && r != 1 {
			t.Errorf("position %d halves both channels: (%g, %g)", pos, l, r)
		}
		if l > 1 || r > 1 {
			t.Errorf("position %d amplifies: (%g, %g)", pos, l, r)
		}
	}
	// Position zero shares the band whole to both, which is what an uncoupled band looks like.
	if l, r := intensityShares(0, h, 0); l != 1 || r != 1 {
		t.Errorf("position zero gives (%g, %g), want (1, 1)", l, r)
	}
}
