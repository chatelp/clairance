package mp3

import (
	"math"
	"testing"
)

// The butterfly coefficients are a cosine and a sine of the same angle, so each pair squares to one.
// A pair that does not would change the energy of every subband boundary in the stream.
func TestAliasCoefficientsAreARotation(t *testing.T) {
	for i := range 8 {
		if got := aliasCS[i]*aliasCS[i] + aliasCA[i]*aliasCA[i]; math.Abs(got-1) > 1e-12 {
			t.Errorf("coefficient %d squares to %g, want one", i, got)
		}
		if aliasCA[i] <= 0 || aliasCS[i] <= 0 {
			t.Errorf("coefficient %d is (%g, %g); both should be positive", i, aliasCS[i], aliasCA[i])
		}
	}
	// The butterflies weaken away from the boundary, which is what makes the overlap local.
	for i := 1; i < 8; i++ {
		if aliasCA[i] >= aliasCA[i-1] {
			t.Errorf("butterfly %d mixes more than %d, and they should weaken", i, i-1)
		}
	}
}

// The alias reduction is a rotation, so applying it and its inverse leaves the subbands as they were.
// That is the property that says the two halves of the butterfly agree with each other.
func TestAntialiasIsInvertible(t *testing.T) {
	var sub, want [576]float32
	for i := range sub {
		sub[i] = float32(math.Sin(float64(i) * 0.37))
	}
	want = sub

	antialias(&sub, 31)
	// The inverse rotation, by hand.
	for b := range 31 {
		at := b * 18
		for i := range 8 {
			u := float64(sub[at+18+i])
			d := float64(sub[at+17-i])
			sub[at+18+i] = float32(u*aliasCS[i] + d*aliasCA[i])
			sub[at+17-i] = float32(-u*aliasCA[i] + d*aliasCS[i])
		}
	}
	for i := range sub {
		if math.Abs(float64(sub[i]-want[i])) > 1e-5 {
			t.Fatalf("sample %d came back as %g, want %g", i, sub[i], want[i])
		}
	}
}

// The four window shapes are what let a granule change transform length without a step at the join.
// A start block rises like a long one and falls like a short one; a stop block does the reverse.
func TestBlockWindowShapes(t *testing.T) {
	long := blockWindows[BlockLong]
	start := blockWindows[BlockStart]
	stop := blockWindows[BlockStop]

	for i := range 18 {
		if start[i] != long[i] {
			t.Errorf("a start block should rise like a long one; tap %d differs", i)
		}
		if stop[18+i] != long[18+i] {
			t.Errorf("a stop block should fall like a long one; tap %d differs", 18+i)
		}
	}
	for i := range 6 {
		if start[18+i] != 1 {
			t.Errorf("a start block should hold at one; tap %d is %g", 18+i, start[18+i])
		}
		if stop[12+i] != 1 {
			t.Errorf("a stop block should hold at one; tap %d is %g", 12+i, stop[12+i])
		}
		if start[30+i] != 0 {
			t.Errorf("a start block should end at nothing; tap %d is %g", 30+i, start[30+i])
		}
		if stop[i] != 0 {
			t.Errorf("a stop block should begin at nothing; tap %d is %g", i, stop[i])
		}
	}
	// The long window is half a period of a sine, so it rises to the middle and mirrors.
	for i := range 18 {
		if math.Abs(long[i]-long[35-i]) > 1e-12 {
			t.Errorf("the long window is not symmetric at tap %d", i)
		}
	}
}

// The transform's basis has to be the right cosine: its rows are orthogonal, which is what makes the
// inverse an inverse rather than a smear.
func TestImdctBasisRowsAreOrthogonal(t *testing.T) {
	for _, n := range []int{12, 36} {
		basis := imdctBasis(n)
		half := n / 2
		for a := range half {
			for b := a + 1; b < half; b++ {
				var dot float64
				for i := range n {
					dot += basis[i][a] * basis[i][b]
				}
				if math.Abs(dot) > 1e-9 {
					t.Errorf("n=%d: columns %d and %d overlap by %g", n, a, b, dot)
				}
			}
		}
	}
}

// Reordering is what puts a short block's three windows back together, and it must move every value
// exactly once — a reorder that drops or repeats one is silent until it is heard.
func TestReorderIsAPermutation(t *testing.T) {
	h := Header{Version: MPEG1, Layer: Layer3, Mode: Mono, SampleRate: 44100}
	g := GranuleInfo{BlockType: BlockShort}
	layout := layoutFor(h, g)

	var sub [576]float32
	for i := range sub {
		sub[i] = float32(i + 1)
	}
	reorder(sub[:], layout, 0)

	seen := map[float32]int{}
	for _, v := range sub {
		seen[v]++
	}
	for i := range 576 {
		if seen[float32(i+1)] != 1 {
			t.Fatalf("value %d appears %d times after reordering", i+1, seen[float32(i+1)])
		}
	}
}

// Every other subband comes out of the transform reversed in frequency, and the flip is its own
// inverse.
func TestFrequencyInversionIsItsOwnInverse(t *testing.T) {
	var sub, want [576]float32
	for i := range sub {
		sub[i] = float32(i%13) - 6
	}
	want = sub
	invertFrequencies(&sub)

	changed := 0
	for i := range sub {
		if sub[i] != want[i] {
			changed++
		}
	}
	// Nine samples of each of sixteen subbands, less those that were nothing to begin with.
	if changed == 0 || changed > 16*9 {
		t.Errorf("%d samples changed sign, want at most %d", changed, 16*9)
	}
	invertFrequencies(&sub)
	if sub != want {
		t.Error("flipping twice did not come back")
	}
}

// Only a mixed block keeps any subband long, and how many depends on the rate.
func TestLongBandsOnlyForMixedBlocks(t *testing.T) {
	for _, tc := range []struct {
		version Version
		rate    int
		mixed   bool
		want    int
	}{
		{MPEG1, 44100, false, 0},
		{MPEG1, 44100, true, 2},
		{MPEG2, 22050, true, 2},
		{MPEG25, 8000, true, 4},
		{MPEG25, 11025, true, 2},
		{MPEG25, 8000, false, 0},
	} {
		h := Header{Version: tc.version, Layer: Layer3, Mode: Mono, SampleRate: tc.rate}
		g := GranuleInfo{BlockType: BlockShort, Mixed: tc.mixed}
		if got := longBandsOf(h, g); got != tc.want {
			t.Errorf("%s at %d Hz mixed=%v: %d long subbands, want %d",
				tc.version, tc.rate, tc.mixed, got, tc.want)
		}
	}
}

// A granule laps onto the one before it, so a transform of nothing still emits whatever the previous
// granule left behind — and leaves nothing of its own.
func TestImdctGranuleLapsOntoTheOverlap(t *testing.T) {
	g := GranuleInfo{BlockType: BlockLong}
	var sub, overlap [576]float32
	for i := range overlap {
		overlap[i] = float32(i%11) - 5
	}
	want := overlap

	imdctGranule(&sub, &overlap, g, 0)
	for i := range sub {
		if sub[i] != want[i] {
			t.Fatalf("sample %d came out as %g, want the overlap's %g", i, sub[i], want[i])
		}
	}
	for i := range overlap {
		if overlap[i] != 0 {
			t.Fatalf("a silent granule left %g at %d", overlap[i], i)
		}
	}
}

// The transform of a single coefficient is a windowed cosine, and it has to span both halves: the
// first eighteen samples come out now and the second eighteen are held for the next granule.
func TestImdctGranuleSplitsAcrossGranules(t *testing.T) {
	g := GranuleInfo{BlockType: BlockLong}
	var sub, overlap [576]float32
	sub[1] = 1

	imdctGranule(&sub, &overlap, g, 0)
	var now, held float64
	for i := range 18 {
		now += math.Abs(float64(sub[i]))
		held += math.Abs(float64(overlap[i]))
	}
	if now == 0 || held == 0 {
		t.Errorf("the transform put %g out now and held %g, and it should do both", now, held)
	}
	// Nothing should reach the subbands that had no coefficient.
	for i := 18; i < 576; i++ {
		if sub[i] != 0 || overlap[i] != 0 {
			t.Fatalf("subband %d moved when only the first had anything", i/18)
		}
	}
}

// The transform computes eighteen of its thirty-six outputs and mirrors the rest, so the symmetry it
// mirrors by is load-bearing. Like the filterbank's, it is a property of the cosine rather than of
// the code, and nothing else would notice if the basis moved under it.
func TestImdctBasisSymmetryHoldsWhereTheTransformAssumesIt(t *testing.T) {
	// Any input will do: the symmetry is in the basis, so it holds for every one.
	var sub [18]float64
	for k := range sub {
		sub[k] = math.Sin(float64(k)*1.7) * 3
	}
	var raw [36]float64
	for n := range 36 {
		for k := range 18 {
			raw[n] += sub[k] * imdctLong[n][k]
		}
	}
	for n := range 9 {
		if math.Abs(raw[n]+raw[17-n]) > 1e-9 {
			t.Errorf("output %d is not the negative of %d: %g against %g", 17-n, n, raw[17-n], raw[n])
		}
		if math.Abs(raw[18+n]-raw[35-n]) > 1e-9 {
			t.Errorf("output %d does not repeat %d: %g against %g", 35-n, 18+n, raw[35-n], raw[18+n])
		}
	}
}
