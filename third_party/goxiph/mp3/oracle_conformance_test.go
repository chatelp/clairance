//go:build conformance

package mp3

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// The oracle is what the reference decoder does, granule by granule, recorded once and compared
// against ever after.
//
// It exists because the obvious check was wrong. A granule's side information gives it a length, and
// it seemed to follow that a correct decoder consumes exactly that — but encoders pad, sometimes by
// hundreds of bits, and the reference forces its own position to the granule's end rather than
// reading to it. Measuring against the declared length therefore says nothing about correctness: a
// granule that stops five hundred bits early may be perfectly decoded.
//
// What does say something is the shape of the decode. Four numbers are recorded, but only one of them
// is a fair comparison and it is worth being precise about which:
//
//   - **The big-value bins are compared.** Both decoders count the same thing — how many of the 576
//     the paired region filled — and it is the structural quantity. A decoder that chose the wrong
//     table, misjudged a region, or lost a codeword fills a different number of them.
//   - **The big-value bits are compared too.** The same pairs read at the same lengths means the same
//     codewords were taken from the same places, which is as close to bit-exactness as this stage
//     gets.
//   - The quadruple count and the end of the spectrum are recorded and not compared at all. The
//     reference advances its output pointer before deciding to stop, and counts the codeword it then
//     discards, so the two are measuring different moments.
//
// Each line is a file name and those four numbers. Rebuild with `mise run mp3:oracle`.
const oracleFile = "testdata/granules.txt"

type granuleShape struct {
	file                             string
	block                            BlockType
	mixed                            bool
	bigBits, bigBins, quads, endBins int
}

func readOracle(t *testing.T) []granuleShape {
	t.Helper()
	f, err := os.Open(oracleFile)
	if err != nil {
		t.Skipf("no oracle: %v", err)
	}
	defer f.Close()

	var out []granuleShape
	scan := bufio.NewScanner(f)
	for scan.Scan() {
		fields := strings.Fields(scan.Text())
		if len(fields) != 5 {
			continue
		}
		g := granuleShape{file: fields[0]}
		g.bigBits, _ = strconv.Atoi(fields[1])
		g.bigBins, _ = strconv.Atoi(fields[2])
		g.quads, _ = strconv.Atoi(fields[3])
		g.endBins, _ = strconv.Atoi(fields[4])
		out = append(out, g)
	}
	return out
}

// TestConformanceGranuleShapesMatchTheReference decodes every granule and compares its shape against
// what the reference made of the same one.
//
// This is the real gate on the Huffman pass. The bins are structural — a decoder that picked the
// wrong table or misjudged a region fills a different number of them — and the bit count is the
// sensitive part, since one codeword read at the wrong length moves every one after it.
func TestConformanceGranuleShapesMatchTheReference(t *testing.T) {
	oracle := readOracle(t)
	if len(oracle) == 0 {
		t.Skip("the oracle is empty")
	}
	byFile := map[string][]granuleShape{}
	for _, g := range oracle {
		byFile[g.file] = append(byFile[g.file], g)
	}

	var checked, matched, structural, bitsToo int
	var wellFormed, wellMatched int
	bitsOff := map[string]int{}
	offBy := map[string]int{}
	mismatch := map[string]int{}
	seen := map[string]int{}

	for _, path := range vectors(t) {
		name := strings.TrimSuffix(filepath.Base(path), ".bit")
		want := byFile[name]
		if len(want) == 0 {
			continue
		}
		for _, got := range decodeShapes(t, path) {
			if seen[name] >= len(want) {
				break
			}
			w := want[seen[name]]
			seen[name]++
			checked++
			ok := got.bigBits == w.bigBits && got.bigBins == w.bigBins
			if !malformed(name) {
				wellFormed++
				if ok {
					wellMatched++
				} else {
					bitsOff[name]++
					kind := got.block.String()
					if got.mixed {
						kind += " mixed"
					}
					offBy[kind]++
				}
			}
			if got.bigBins == w.bigBins {
				structural++
			}
			if got.bigBits == w.bigBits && got.bigBins == w.bigBins {
				bitsToo++
			}
			if got.bigBits == w.bigBits && got.bigBins == w.bigBins && got.endBins == w.endBins {
				matched++
			} else if got.bigBins != w.bigBins {
				if mismatch[name] < 1 && len(mismatch) < 3 {
					t.Logf("%s #%d: mine %d/%d/%d/%d, reference %d/%d/%d/%d",
						name, seen[name]-1, got.bigBits, got.bigBins, got.quads, got.endBins,
						w.bigBits, w.bigBins, w.quads, w.endBins)
				}
				mismatch[name]++
			}
		}
	}

	if checked == 0 {
		t.Fatal("no granules compared")
	}
	t.Logf("%d granules compared: %d agree on the big-value bins (%.1f%%), %d on their bits too (%.1f%%), %d on everything (%.1f%%)",
		checked, structural, 100*float64(structural)/float64(checked),
		bitsToo, 100*float64(bitsToo)/float64(checked),
		matched, 100*float64(matched)/float64(checked))
	t.Logf("of the %d granules in streams that are not malformed on purpose, %d agree on bins and bits (%.2f%%)",
		wellFormed, wellMatched, 100*float64(wellMatched)/float64(wellFormed))
	for kind, n := range offBy {
		t.Logf("  differing granules by block: %s, %d", kind, n)
	}
	for name, n := range bitsOff {
		t.Logf("  bits differ: %s, %d of %d", name, n, seen[name])
	}
	for name, n := range mismatch {
		t.Logf("  %s: %d of %d differ", name, n, seen[name])
	}

	// The bar is on the streams that are not malformed on purpose, over both the bins and the bits
	// the big values took. Those two together say the same codewords were read from the same places.
	const bar = 999 // per thousand
	if per := 1000 * wellMatched / wellFormed; per < bar {
		t.Errorf("%d granules per thousand agree with the reference, below the %d held", per, bar)
	}
}

// malformed names the vectors written to be broken rather than to be decoded. Their side information
// describes granules that cannot exist, so two decoders make different sense of them and neither is
// wrong. They are still run — a decoder must not crash on them — but not counted.
func malformed(name string) bool {
	return strings.Contains(name, "compl-sideinfo")
}

// decodeShapes runs one file through and reports the shape of each granule it decodes.
func decodeShapes(t *testing.T, path string) []granuleShape {
	t.Helper()
	name := strings.TrimSuffix(filepath.Base(path), ".bit")
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	s, err := NewScanner(f)
	f.Close()
	if err != nil {
		return nil
	}

	var out []granuleShape
	var res reservoir
	var previous [2]scaleFactors
	for {
		fr, err := s.Next()
		if errors.Is(err, io.EOF) || err != nil {
			return out
		}
		if fr.Header.Layer != Layer3 {
			continue
		}
		si, err := ParseSideInfo(fr.Header, fr.Data)
		if err != nil {
			continue
		}
		main := fr.Data[sideInfoSize(fr.Header):]
		r, err := res.open(main, si.MainDataBegin)
		if err != nil {
			continue
		}
		for g := range granules(fr.Header.Version) {
			for ch := range fr.Header.Channels() {
				gi := si.Granule[g][ch]
				start := r.BitsRead()
				sf := readScaleFactors(r, fr.Header, gi, ch, &previous[ch])
				previous[ch] = sf
				_, sh, err := readSpectrum(r, fr.Header, gi, start+gi.Part23Length)
				if err != nil {
					return out
				}
				out = append(out, granuleShape{
					file: name, block: gi.BlockType, mixed: gi.Mixed,
					bigBits: sh.bigBits, bigBins: sh.bigBins,
					quads: sh.quads, endBins: sh.endBins,
				})
				// The granule's data ends where it was told to, whatever was read.
				if err := skipTo(r, start+gi.Part23Length); err != nil {
					return out
				}
			}
		}
	}
}

// skipTo moves the reader to a bit position, which is what a decoder does at a granule's end: the
// coder pads and the padding is not read.
func skipTo(r interface{ BitsRead() int }, to int) error {
	rd, ok := r.(interface {
		BitsRead() int
		Read(uint) (uint64, error)
	})
	if !ok {
		return fmt.Errorf("mp3: the reader cannot be advanced")
	}
	for rd.BitsRead() < to {
		n := min(uint(to-rd.BitsRead()), 32)
		if _, err := rd.Read(n); err != nil {
			return err
		}
	}
	return nil
}
