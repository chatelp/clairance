package mp3

import (
	"testing"

	"github.com/yegors/co-atc/third_party/goxiph/internal/bitio"
)

// codewordFor finds a table's codeword for a pair, by walking the tree for it. Tests state the pair
// they mean and let this find the bits, so that a wrong table shows up as a wrong pair rather than as
// a hexadecimal constant nobody can check.
func codewordFor(tree []int32, x, y int) (bits uint64, length uint, ok bool) {
	var walk func(at int, code uint64, depth uint) bool
	walk = func(at int, code uint64, depth uint) bool {
		for bit := range 2 {
			next := code<<1 | uint64(bit)
			switch v := tree[at+bit]; {
			case v < 0:
				if gx, gy := leafValues(v); gx == x && gy == y {
					bits, length = next, depth+1
					return true
				}
			case v > 0:
				if walk(int(v), next, depth+1) {
					return true
				}
			}
		}
		return false
	}
	return bits, length, walk(0, 0, 0)
}

// A pair read back from the table it was written with. Table 1 is the smallest, four codewords over
// the pairs of nought and one, so every one of them can be checked.
func TestHuffmanPairsRoundTrip(t *testing.T) {
	table := &huffTables[1]
	for x := range 2 {
		for y := range 2 {
			w := bitio.NewMSBWriter()
			bits, length, ok := codewordFor(table.tree, x, y)
			if !ok {
				t.Fatalf("table 1 has no codeword for (%d, %d)", x, y)
			}
			_ = w.Write(bits, length)
			// A value of nought carries no sign; anything else carries one, and a set bit is negative.
			if x != 0 {
				_ = w.Write(1, 1)
			}
			if y != 0 {
				_ = w.Write(0, 1)
			}
			w.AlignByte()

			r := bitio.NewMSBReader(w.Bytes())
			gx, gy, err := readPair(r, table, 1000)
			if err != nil {
				t.Fatalf("(%d, %d): %v", x, y, err)
			}
			wantX, wantY := -x, y
			if gx != wantX || gy != wantY {
				t.Errorf("(%d, %d) came back as (%d, %d), want (%d, %d)", x, y, gx, gy, wantX, wantY)
			}
		}
	}
}

// The escape is what lets a table of sixteen values reach past fifteen: the top value is not a value
// but a signal that more bits follow. A decoder that treats it as fifteen loses every loud bin and
// reads everything after it from the wrong place.
func TestHuffmanEscapeExtendsTheValue(t *testing.T) {
	// Table 16 carries one escape bit, so fifteen reaches sixteen.
	table := &huffTables[16]
	bits, length, ok := codewordFor(table.tree, 15, 0)
	if !ok {
		t.Fatal("table 16 has no codeword for (15, 0)")
	}
	w := bitio.NewMSBWriter()
	_ = w.Write(bits, length)
	_ = w.Write(1, 1) // the escape bit, adding one
	_ = w.Write(0, 1) // the sign
	w.AlignByte()

	x, y, err := readPair(bitio.NewMSBReader(w.Bytes()), table, 1000)
	if err != nil {
		t.Fatal(err)
	}
	if x != 16 || y != 0 {
		t.Errorf("an escaped fifteen came back as (%d, %d), want (16, 0)", x, y)
	}
}

// Every codeword of the quadruple tables decodes to the flags it was written for. Sixteen symbols
// each, and both tables, so this is exhaustive.
func TestCount1TablesRoundTrip(t *testing.T) {
	for table := range 2 {
		for _, c := range count1Tables[table] {
			w := bitio.NewMSBWriter()
			_ = w.Write(uint64(c.bits), uint(c.length))
			w.AlignByte()
			got, err := readCount1(bitio.NewMSBReader(w.Bytes()), table)
			if err != nil {
				t.Fatalf("table %d, codeword %d of %d bits: %v", table, c.bits, c.length, err)
			}
			if got != int(c.flags) {
				t.Errorf("table %d, codeword %d of %d bits: flags %d, want %d",
					table, c.bits, c.length, got, c.flags)
			}
		}
	}
}

// A granule ends when its bits run out, and the last codeword may be cut short by that. What has been
// read stands; nothing after it is invented.
func TestSpectrumStopsAtTheLimit(t *testing.T) {
	h := Header{Version: MPEG1, Layer: Layer3, Mode: Mono, SampleRate: 44100}
	g := GranuleInfo{BigValues: 100, TableSelect: [3]int{1, 1, 1}, RegionCount: [3]int{7, 7, 255}}

	// Twenty bytes of ones, and a limit of eight bits. Only what fits may be read.
	data := make([]byte, 20)
	for i := range data {
		data[i] = 0xFF
	}
	r := bitio.NewMSBReader(data)
	if _, _, err := readSpectrum(r, h, g, 8); err != nil {
		t.Fatal(err)
	}
	if got := r.BitsRead(); got > 8+maxCodewordBits {
		t.Errorf("read %d bits against a limit of 8", got)
	}
}

// An unswitched granule takes its region boundaries from the bands it names; a short block that is
// not mixed has them fixed, because its bands are windows rather than a rising spectrum.
func TestRegionBoundsByBlockType(t *testing.T) {
	h := Header{Version: MPEG1, Layer: Layer3, Mode: Mono, SampleRate: 44100}

	long := GranuleInfo{RegionCount: [3]int{7, 6, 255}}
	got := regionBounds(layoutFor(h, long), long, h)
	if got[0] >= got[1] || got[1] > 576 || got[2] != 576 {
		t.Errorf("a long granule's regions came out %v", got)
	}

	short := GranuleInfo{BlockType: BlockShort, RegionCount: [3]int{8, 255, 255}}
	if got := regionBounds(layoutFor(h, short), short, h); got != [3]int{36, 576, 576} {
		t.Errorf("a short granule's regions came out %v, want [36 576 576]", got)
	}
}
