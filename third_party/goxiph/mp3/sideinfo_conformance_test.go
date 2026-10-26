//go:build conformance

package mp3

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// TestConformanceSideInfoIsConsistent parses the side information of every layer III frame in the
// vector set and checks it against itself.
//
// There is no reference to compare the fields to, so what is checked is that they describe something
// possible: the granules claim no more bits than the pooled data holds, the regions fall inside the
// spectrum, and every table named exists. A layout read wrongly — a field one bit out, the lower
// sampling frequencies read as MPEG-1 — puts every field after it out of range, and over six
// thousand frames it cannot go unnoticed.
func TestConformanceSideInfoIsConsistent(t *testing.T) {
	var frames, granuleCount int
	blocks := map[BlockType]int{}
	mixed, switched, preflag, reservoirUsed := 0, 0, 0, 0
	var deepest int

	for _, path := range vectors(t) {
		name := filepath.Base(path)
		f, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		s, err := NewScanner(f)
		f.Close()
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}

		var res reservoir
		for {
			fr, err := s.Next()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				break // free format and the malformed vectors, already accounted for elsewhere
			}
			if fr.Header.Layer != Layer3 {
				continue
			}

			si, err := ParseSideInfo(fr.Header, fr.Data)
			if err != nil {
				// The set holds frames written to be refused. What matters is that they are refused
				// rather than read into something absurd.
				continue
			}
			frames++

			limit := 511
			if fr.Header.Version.lsf() {
				limit = 255
			}
			if si.MainDataBegin > limit {
				t.Errorf("%s: main data begins %d bytes back, the field holds at most %d",
					name, si.MainDataBegin, limit)
			}
			if si.MainDataBegin > 0 {
				reservoirUsed++
			}
			deepest = max(deepest, si.MainDataBegin)

			main := fr.Data[sideInfoSize(fr.Header):]
			if _, err := res.open(main, si.MainDataBegin); err != nil {
				continue // the first frames of a file legitimately reach back before it
			}

			for g := range granules(fr.Header.Version) {
				for ch := range fr.Header.Channels() {
					gi := si.Granule[g][ch]
					granuleCount++
					blocks[gi.BlockType]++
					if gi.Mixed {
						mixed++
					}
					if gi.BlockType != BlockLong {
						switched++
					}
					if gi.Preflag {
						preflag++
					}

					if gi.BigValues > 288 {
						t.Errorf("%s: %d big values", name, gi.BigValues)
					}
					for i, tbl := range gi.TableSelect {
						if tbl > 31 {
							t.Errorf("%s: region %d names table %d", name, i, tbl)
						}
					}
					// The flag is carried for every switched granule and only an unswitched one has
					// no business setting it. It reaches start and stop blocks too, where it keeps
					// their lowest subbands long; six granules of the set do that and this test used
					// to call all six a fault.
					if gi.Mixed && gi.BlockType == BlockLong {
						t.Errorf("%s: an unswitched block was reported mixed", name)
					}
					if gi.BlockType == BlockLong && (gi.SubblockGain != [3]int{}) {
						t.Errorf("%s: an unswitched granule carries subblock gains", name)
					}
				}
			}

			// The granules together must not claim more bits than the frame and its history hold.
			bits := 0
			for g := range granules(fr.Header.Version) {
				for ch := range fr.Header.Channels() {
					bits += si.Granule[g][ch].Part23Length
				}
			}
			if bits > (len(main)+si.MainDataBegin)*8 {
				t.Errorf("%s: granules claim %d bits, %d bytes are available",
					name, bits, len(main)+si.MainDataBegin)
			}
		}
	}

	if frames == 0 {
		t.Fatal("no layer III frames parsed")
	}
	t.Logf("%d frames, %d granules", frames, granuleCount)
	t.Logf("block types: long %d, start %d, short %d, stop %d",
		blocks[BlockLong], blocks[BlockStart], blocks[BlockShort], blocks[BlockStop])
	t.Logf("%d switched granules, %d of them mixed, %d with preemphasis", switched, mixed, preflag)
	t.Logf("%d frames reach into the reservoir, deepest %d bytes", reservoirUsed, deepest)

	// A corpus that never switches blocks or never uses the reservoir would leave most of the
	// decoder unexercised, and it would be worth knowing before trusting a gate built on it.
	if blocks[BlockShort] == 0 {
		t.Error("no short blocks in the whole set")
	}
	if reservoirUsed == 0 {
		t.Error("no frame uses the bit reservoir")
	}
}
