package mp3

import (
	"fmt"

	"github.com/yegors/co-atc/third_party/goxiph/internal/bitio"
)

// The spectrum is coded in three stretches, and the boundaries between them are not in the stream.
//
// The lowest stretch is the big values, pairs read from three Huffman tables over three regions of
// bands, where the loud low end needs an alphabet that reaches far. Above it is the count1 region,
// quadruples of values no larger than one, which is what most of the upper spectrum is. Above that
// is nothing: the rest of the 576 bins are zero, and how far up that begins is only known by running
// out of bits.
//
// Running out of bits is how the granule ends, and it is the ordinary case rather than an error. A
// granule is given a length in the side information and the coder fills it; the last codeword may be
// cut short by it, and what has been read so far still stands.

// spectrum is one granule's quantised values, before they are scaled or reordered.
type spectrum [576]int

// shape is what a granule's decode turned out to be: how far the big values reached and what the
// quadruples after them added.
//
// Reported because it is what a decode can be checked by. There is no specification to compare the
// values against and no reference output at this stage, but two decoders that fill the same bins
// with the same number of bits have read the same codewords from the same places.
type shape struct {
	// bigBits is how many bits the big values took and bigBins how many of the 576 they filled.
	bigBits, bigBins int
	// quads is how many quadruples were read and endBins where the spectrum stopped.
	quads, endBins int
}

// readSpectrum decodes one granule's spectrum from the main data.
//
// limit is the bit position the granule's data ends at, which the side information fixes. Nothing in
// the stream says where the count1 region starts or the zeros begin; the first is where the big
// values run out and the second is where the bits do.
func readSpectrum(r *bitio.MSBReader, h Header, g GranuleInfo, limit int) (spectrum, shape, error) {
	var out spectrum
	var sh shape
	begin := r.BitsRead()
	layout := layoutFor(h, g)

	// The three regions the big values are split across, as bin counts. A switched block fixes its
	// own boundaries; an unswitched one names them in bands.
	bounds := regionBounds(layout, g, h)

	at := 0
	pairs := g.BigValues
	for region := range 3 {
		end := min(bounds[region], pairs*2)
		table := &huffTables[g.TableSelect[region]]
		for at < end {
			if r.BitsRead() >= limit {
				sh.bigBits, sh.bigBins, sh.endBins = r.BitsRead()-begin, at, at
				return out, sh, nil
			}
			x, y, err := readPair(r, table, limit)
			if err != nil {
				return out, sh, err
			}
			out[at], out[at+1] = x, y
			at += 2
		}
	}

	sh.bigBits, sh.bigBins = r.BitsRead()-begin, at

	// The quadruples, which run until the bits do.
	table := g.Count1Table
	for at <= 572 && r.BitsRead() < limit {
		v, err := readCount1(r, table)
		if err != nil {
			return out, sh, err
		}
		sh.quads++
		for i := range 4 {
			if v>>(3-i)&1 == 0 {
				continue
			}
			s, _ := r.Read(1)
			out[at+i] = 1 - 2*int(s)
		}
		at += 4
	}
	sh.endBins = at
	return out, sh, nil
}

// regionBounds is where each of the three big-value regions ends, in bins.
//
// Always from the bands, for every block type. A switched block does not name its own boundaries —
// the side information fixes its region counts at seven, or eight for a short block that is not
// mixed — but they are still counted in bands of that block's own layout, and a short block's bands
// are four bins wide at some rates and eight at others. Fixing the boundary at a bin instead is
// right for one rate and wrong for the rest, which is quiet: the granule still decodes, with some of
// its pairs read from the wrong table.
func regionBounds(layout bandLayout, g GranuleInfo, h Header) [3]int {
	first := min(g.RegionCount[0]+1, len(layout.bounds)-1)
	second := min(g.RegionCount[0]+g.RegionCount[1]+2, len(layout.bounds)-1)
	if g.RegionCount[1] == 255 {
		second = len(layout.bounds) - 1
	}
	return [3]int{int(layout.bounds[first]), int(layout.bounds[second]), 576}
}

// readPair reads one pair of big values.
func readPair(r *bitio.MSBReader, table *huffTable, limit int) (x, y int, err error) {
	if len(table.tree) == 0 {
		return 0, 0, nil
	}
	leaf, err := walk(r, table.tree, limit)
	if err != nil {
		return 0, 0, err
	}
	x, y = leafValues(leaf)
	if x, err = escape(r, x, table.linbits); err != nil {
		return 0, 0, err
	}
	if y, err = escape(r, y, table.linbits); err != nil {
		return 0, 0, err
	}
	return x, y, nil
}

// escape extends a value that reached the top of its alphabet, and gives it a sign.
//
// Fifteen is not fifteen but "fifteen or more", and the tables that reach past it carry the rest in
// a fixed number of bits after the codeword. A value of nothing carries no sign, which is what keeps
// the commonest symbol to a single codeword.
func escape(r *bitio.MSBReader, v, linbits int) (int, error) {
	if v == 15 && linbits > 0 {
		extra, err := r.Read(uint(linbits))
		if err != nil {
			return 0, err
		}
		v += int(extra)
	}
	if v == 0 {
		return 0, nil
	}
	s, err := r.Read(1)
	if err != nil {
		return 0, err
	}
	if s != 0 {
		return -v, nil
	}
	return v, nil
}

// walk descends a table's tree one bit at a time until it reaches a leaf.
func walk(r *bitio.MSBReader, tree []int32, limit int) (int32, error) {
	at := 0
	for range maxCodewordBits {
		if r.BitsRead() >= limit {
			return 0, errTruncated
		}
		bit, err := r.Read(1)
		if err != nil {
			return 0, err
		}
		switch v := tree[at+int(bit)]; {
		case v < 0:
			return v, nil
		case v == 0:
			return 0, fmt.Errorf("%w: a codeword walked off the table", ErrBadFrame)
		default:
			at = int(v)
		}
	}
	return 0, fmt.Errorf("%w: a codeword ran past %d bits", ErrBadFrame, maxCodewordBits)
}

// maxCodewordBits is the longest a Huffman codeword can be, which bounds the walk.
const maxCodewordBits = 19

// errTruncated marks a granule whose data ran out mid-codeword, which is ordinary rather than wrong.
var errTruncated = fmt.Errorf("%w: the granule's data ended", ErrBadFrame)

// readCount1 reads one quadruple's codeword and returns which of its four values are set.
//
// Sixteen codewords of at most six bits do not repay a tree, so the bits are accumulated and matched.
func readCount1(r *bitio.MSBReader, table int) (int, error) {
	code, length := uint8(0), uint8(0)
	for length < 6 {
		bit, err := r.Read(1)
		if err != nil {
			return 0, err
		}
		code = code<<1 | uint8(bit)
		length++
		for _, c := range count1Tables[table] {
			if c.length == length && c.bits == code {
				return int(c.flags), nil
			}
		}
	}
	return 0, fmt.Errorf("%w: a quadruple walked off its table", ErrBadFrame)
}
