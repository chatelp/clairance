package mp3

import "testing"

// leaves counts the codewords in a table and the longest of them, by walking the tree.
func leaves(tree []int32) (count, longest int) {
	var walk func(at, depth int)
	walk = func(at, depth int) {
		for bit := range 2 {
			switch v := tree[at+bit]; {
			case v < 0:
				count++
				longest = max(longest, depth+1)
			case v > 0:
				walk(int(v), depth+1)
			}
		}
	}
	if len(tree) > 0 {
		walk(0, 0)
	}
	return count, longest
}

// The tables were recovered by walking another decoder rather than transcribed from a specification,
// so their shape is what says the recovery worked. These counts are the standard's: a table over
// pairs drawn from an alphabet of n holds n squared codewords, and the alphabets are 2, 3, 4, 6, 8
// and 16. Three tables are absent because the format does not use them.
//
// A regeneration that reads the reference wrongly does not produce a table of the wrong shape by
// accident — it produces one that is short, or long, or not a prefix code at all.
func TestHuffmanTableShapes(t *testing.T) {
	want := map[int]int{
		0: 0, 4: 0, 14: 0,
		1: 4,
		2: 9, 3: 9,
		5: 16, 6: 16,
		7: 36, 8: 36, 9: 36,
		10: 64, 11: 64, 12: 64,
		13: 256, 15: 256,
	}
	for n := 16; n < 32; n++ {
		want[n] = 256
	}

	total := 0
	for n, table := range huffTables {
		count, longest := leaves(table.tree)
		if count != want[n] {
			t.Errorf("table %d holds %d codewords, want %d", n, count, want[n])
		}
		if count > 0 && (longest == 0 || longest > 19) {
			t.Errorf("table %d has a codeword of %d bits", n, longest)
		}
		total += count
	}
	if total != 4962 {
		t.Errorf("%d codewords in all, want 4962", total)
	}
}

// The escape widths say how many extra bits a value of fifteen carries, which is how the tables with
// a small alphabet reach large values. They come in two runs of eight, and a table without them must
// not claim any.
func TestEscapeWidths(t *testing.T) {
	want := map[int]int{
		16: 1, 17: 2, 18: 3, 19: 4, 20: 6, 21: 8, 22: 10, 23: 13,
		24: 4, 25: 5, 26: 6, 27: 7, 28: 8, 29: 9, 30: 11, 31: 13,
	}
	for n, table := range huffTables {
		if got := table.linbits; got != want[n] {
			t.Errorf("table %d carries %d escape bits, want %d", n, got, want[n])
		}
	}
}

// A leaf holds both values of a pair, and the packing has to survive the pair (0, 0) — which is the
// commonest thing any of these tables codes.
func TestLeafPackingRoundTrips(t *testing.T) {
	for x := range 16 {
		for y := range 16 {
			leaf := -(int32(x)<<4 | int32(y)) - 1
			if leaf >= 0 {
				t.Fatalf("(%d, %d) packs to a non-negative leaf", x, y)
			}
			gx, gy := leafValues(leaf)
			if gx != x || gy != y {
				t.Errorf("(%d, %d) came back as (%d, %d)", x, y, gx, gy)
			}
		}
	}
}

// The band layouts are boundaries into a spectrum of 576, rising, and each row ends where its bands
// run out. A layout that does not rise would put a band's end before its start.
func TestBandLayoutsRise(t *testing.T) {
	check := func(name string, rows [][]int16) {
		for r, row := range rows {
			for i := 1; i < len(row); i++ {
				if row[i] < row[i-1] {
					t.Errorf("%s row %d falls from %d to %d at %d", name, r, row[i-1], row[i], i)
					break
				}
			}
			if last := row[len(row)-1]; last < 192 {
				t.Errorf("%s row %d reaches only %d of 576", name, r, last)
			}
		}
	}
	toRows := func(a [8][24]int16) [][]int16 {
		out := make([][]int16, len(a))
		for i := range a {
			out[i] = a[i][:]
		}
		return out
	}
	toRows41 := func(a [8][41]int16) [][]int16 {
		out := make([][]int16, len(a))
		for i := range a {
			out[i] = a[i][:]
		}
		return out
	}
	check("long", toRows(scfBandLong))
	check("short", toRows41(scfBandShort))
	check("mixed", toRows41(scfBandMixed))
}

// Every layout spans the whole granule of 576 bins, the short ones included: their bands are listed
// once per window, so three windows of a third each come to the same total. Getting the span wrong
// puts every scale factor on the wrong bins.
//
// The final entry repeats the one before it, which is where the widths ran out — the reference marks
// the end with a zero width and that becomes a boundary that does not move.
func TestBandLayoutsSpanTheSpectrum(t *testing.T) {
	for r, row := range scfBandLong {
		if got := row[22]; got != 576 {
			t.Errorf("long row %d spans %d, want 576", r, got)
		}
	}
	for name, rows := range map[string][8][41]int16{"short": scfBandShort, "mixed": scfBandMixed} {
		for r, row := range rows {
			last := row[len(row)-1]
			if last != 576 {
				t.Errorf("%s row %d spans %d, want 576", name, r, last)
			}
			if row[len(row)-2] != last {
				t.Errorf("%s row %d does not end on a repeated boundary", name, r)
			}
		}
	}
}
