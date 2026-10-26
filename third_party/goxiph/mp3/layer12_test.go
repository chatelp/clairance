package mp3

import (
	"math"
	"testing"
)

// The scale factors are sixty-four steps two decibels apart, which the format tabulates and this
// computes. Getting the step wrong is a level error that grows with the index rather than a broken
// stream, so it has to be checked against the values the table gives.
func TestLayer12ScaleFactorsAreTwoDecibelSteps(t *testing.T) {
	for _, tc := range []struct {
		index int
		want  float64
	}{
		{0, 2.00000000000000},
		{1, 1.58740105196820},
		{2, 1.25992104989487},
		{3, 1.00000000000000},
		{62, 1.20155434598434e-06},
	} {
		if got := layer12Scale(tc.index); math.Abs(got-tc.want)/tc.want > 1e-12 {
			t.Errorf("scale factor %d is %g, want %g", tc.index, got, tc.want)
		}
	}
	// Every three steps halve, which is what makes the whole thing a table of one formula.
	for i := range 60 {
		if got := layer12Scale(i) / layer12Scale(i+3); math.Abs(got-2) > 1e-12 {
			t.Errorf("steps %d and %d differ by %g, want two", i, i+3, got)
		}
	}
}

// A packed quantiser holds three samples in one codeword, which is only worth doing where the
// codeword is shorter than three separate ones would be.
func TestPackedQuantisersFitThreeSamples(t *testing.T) {
	for _, tc := range []struct {
		class  uint8
		levels int
		bits   uint
	}{
		{17, 3, 5},
		{18, 5, 7},
		{19, 9, 10},
	} {
		q, ok := quantiserFor(tc.class)
		if !ok || !q.packed {
			t.Fatalf("class %d resolved to %+v, want a packed quantiser", tc.class, q)
		}
		if q.levels != tc.levels || q.bits != tc.bits {
			t.Errorf("class %d gives %d levels in %d bits, want %d in %d",
				tc.class, q.levels, q.bits, tc.levels, tc.bits)
		}
		// The codeword has to hold every combination and no more than one bit's worth spare.
		if span := math.Pow(float64(tc.levels), 3); span > math.Exp2(float64(q.bits)) {
			t.Errorf("class %d: %g combinations do not fit in %d bits", tc.class, span, q.bits)
		}
		if plain := 3 * math.Ceil(math.Log2(float64(tc.levels))); float64(q.bits) >= plain {
			t.Errorf("class %d costs %d bits packed and %g apart, so packing gains nothing",
				tc.class, q.bits, plain)
		}
	}
}

// A plain quantiser spends its allocation on one sample and covers one step fewer than the codes it
// has, because the codeword that would sit at the top is not used.
func TestPlainQuantisersSpendTheirAllocation(t *testing.T) {
	for class := uint8(2); class < 17; class++ {
		q, ok := quantiserFor(class)
		if !ok || q.packed {
			t.Fatalf("class %d resolved to %+v, want a plain quantiser", class, q)
		}
		if q.bits != uint(class) || q.levels != 1<<class-1 {
			t.Errorf("class %d gives %d levels in %d bits", class, q.levels, q.bits)
		}
	}
	if _, ok := quantiserFor(0); ok {
		t.Error("class zero should name no quantiser at all")
	}
}

// How far up the subbands reach depends on what the frame has to spend, and a stream with little of
// it stops well short of the top rather than spreading itself thin.
func TestLayer2SubbandsReachFurtherAtHigherRates(t *testing.T) {
	for _, tc := range []struct {
		rate    int
		kbps    int
		mode    Mode
		bands   int
		version Version
	}{
		{44100, 192, Stereo, 30, MPEG1}, // 96 kbps a channel is enough to reach the top
		{48000, 192, Stereo, 27, MPEG1}, // 48 kHz is the one rate that stops short of it anyway
		{44100, 64, Stereo, 8, MPEG1},   // 32 kbps a channel, and only eight subbands carry anything
		{32000, 64, Stereo, 12, MPEG1},  // the slowest rate spreads the same over twelve
		{44100, 128, Mono, 30, MPEG1},   // mono keeps all of it
		{22050, 64, Stereo, 30, MPEG2},  // the half rate versions have one plan and use it always
	} {
		h := Header{Version: tc.version, Layer: Layer2, Mode: tc.mode,
			SampleRate: tc.rate, Bitrate: tc.kbps * 1000}
		if _, bands := allocPlan(h); bands != tc.bands {
			t.Errorf("%d kbps %v at %d Hz reaches %d subbands, want %d",
				tc.kbps, tc.mode, tc.rate, bands, tc.bands)
		}
	}
}

// Layer I gives every subband the same four bits, and every allocation code names a quantiser it can
// spend them on.
func TestLayer1AllocatesEverySubband(t *testing.T) {
	runs, bands := allocPlan(Header{Version: MPEG1, Layer: Layer1, Mode: Stereo, SampleRate: 44100})
	if bands != 32 {
		t.Errorf("layer I reaches %d subbands, want 32", bands)
	}
	if len(runs) != 1 || runs[0].bits != 4 || runs[0].bands != 32 {
		t.Fatalf("layer I reads its allocations as %+v", runs)
	}
	// Layer I has no packed quantisers, so no code in its row may name one.
	for _, class := range allocClasses[runs[0].row] {
		if class >= 17 {
			t.Errorf("layer I names the packed quantiser %d", class)
		}
	}
}

// Joint stereo stops coding the channels apart somewhere, and where depends on the mode extension.
// Above it one set of samples serves both, so a bound read wrong halves or doubles the stereo image.
func TestStereoBoundFollowsTheModeExtension(t *testing.T) {
	for ext, want := range map[int]int{0: 4, 1: 8, 2: 12, 3: 16} {
		h := Header{Mode: JointStereo, ModeExtension: ext}
		if got := stereoBands(h, 30); got != want {
			t.Errorf("extension %d gives a bound of %d, want %d", ext, got, want)
		}
	}
	// Never past where the subbands stop, whatever the extension says.
	if got := stereoBands(Header{Mode: JointStereo, ModeExtension: 3}, 12); got != 12 {
		t.Errorf("the bound reached %d with only twelve subbands", got)
	}
	if got := stereoBands(Header{Mode: Mono}, 32); got != 0 {
		t.Errorf("a mono frame has a bound of %d, want none", got)
	}
	if got := stereoBands(Header{Mode: Stereo}, 32); got != 32 {
		t.Errorf("a stereo frame stops sharing at %d, and should never share", got)
	}
}
