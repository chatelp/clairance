package mp3

import (
	"testing"

	"github.com/yegors/co-atc/third_party/goxiph/internal/bitio"
)

// MPEG-1 sizes its scale factors from a packed pair of widths and splits twenty-one bands four ways.
// The widths are two bits each, so a sizing that reads them the wrong way round still reads a
// plausible number of bits and gets every factor wrong.
func TestScaleFactorsMPEG1Widths(t *testing.T) {
	h := Header{Version: MPEG1, Layer: Layer3, Mode: Mono, SampleRate: 44100}
	// The widths come from the table rather than from arithmetic here: the packing is the thing
	// under test, so stating it twice would only agree with itself.
	const sizing = 14
	g := GranuleInfo{ScalefacCompress: sizing}
	first := int(scfCompress[sizing]) >> 2
	second := int(scfCompress[sizing]) & 3
	want := 6*first + 5*first + 5*second + 5*second

	w := bitio.NewMSBWriter()
	for i := range 21 {
		width := first
		if i >= 11 {
			width = second
		}
		_ = w.Write(uint64(i%2), uint(width))
	}
	w.AlignByte()

	r := bitio.NewMSBReader(w.Bytes())
	sf := readScaleFactors(r, h, g, 0, &scaleFactors{})
	if got := r.BitsRead(); got != want {
		t.Errorf("read %d bits of scale factors, want %d", got, want)
	}
	for i := range 21 {
		if int(sf.value[i]) != i%2 {
			t.Errorf("band %d came back as %d, want %d", i, sf.value[i], i%2)
		}
	}
}

// A group the second granule takes from the first reads no bits at all, which is the whole point of
// the field: a stretch of steady sound codes its scale factors once.
func TestScaleFactorsSharedGroupsReadNothing(t *testing.T) {
	h := Header{Version: MPEG1, Layer: Layer3, Mode: Mono, SampleRate: 44100}
	var previous scaleFactors
	for i := range 21 {
		previous.value[i] = uint8(i)
	}

	// Every one of the four groups taken from the granule before.
	g := GranuleInfo{ScalefacCompress: 14, Scfsi: 0xF}
	r := bitio.NewMSBReader(make([]byte, 8))
	sf := readScaleFactors(r, h, g, 0, &previous)
	if got := r.BitsRead(); got != 0 {
		t.Errorf("a fully shared granule read %d bits, want none", got)
	}
	for i := range 21 {
		if sf.value[i] != previous.value[i] {
			t.Errorf("band %d came back as %d, want %d", i, sf.value[i], previous.value[i])
		}
	}
}

// A short block's three windows each carry a gain that lifts every band of that window, and the lift
// depends on the step the factors are applied in.
func TestSubblockGainsLiftTheirWindows(t *testing.T) {
	h := Header{Version: MPEG1, Layer: Layer3, Mode: Mono, SampleRate: 44100}
	g := GranuleInfo{BlockType: BlockShort, ScalefacCompress: 0, SubblockGain: [3]int{1, 2, 3}}

	r := bitio.NewMSBReader(make([]byte, 32))
	sf := readScaleFactors(r, h, g, 0, &scaleFactors{})
	// A sizing of nought gives widths of nought, so every factor is the gain alone, shifted by two.
	for i := 0; i < 36; i += 3 {
		for w := range 3 {
			if want := uint8((w + 1) << 2); sf.value[i+w] != want {
				t.Errorf("band %d window %d is %d, want %d", i, w, sf.value[i+w], want)
			}
		}
	}
}

// The layouts a granule can take, and the counts that go with them. A mixed block keeps its lowest
// bands long, and how many depends on the version — a difference nothing in the stream announces.
func TestBandLayoutsByBlockType(t *testing.T) {
	for _, tc := range []struct {
		version     Version
		block       BlockType
		mixed       bool
		long, short int
	}{
		{MPEG1, BlockLong, false, 22, 0},
		{MPEG1, BlockShort, false, 0, 39},
		{MPEG1, BlockShort, true, 8, 30},
		{MPEG2, BlockShort, true, 6, 30},
		{MPEG25, BlockShort, true, 6, 30},
	} {
		h := Header{Version: tc.version, Layer: Layer3, Mode: Mono, SampleRate: sampleRates[tc.version][0]}
		l := layoutFor(h, GranuleInfo{BlockType: tc.block, Mixed: tc.mixed})
		if l.long != tc.long || l.short != tc.short {
			t.Errorf("%s %v mixed=%v: %d long and %d short, want %d and %d",
				tc.version, tc.block, tc.mixed, l.long, l.short, tc.long, tc.short)
		}
		if l.bands() != tc.long+tc.short {
			t.Errorf("%s: bands() is %d", tc.version, l.bands())
		}
	}
}
