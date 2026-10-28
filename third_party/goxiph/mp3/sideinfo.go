package mp3

import (
	"fmt"

	"github.com/yegors/co-atc/third_party/goxiph/internal/bitio"
)

// The side information is the frame's index: how the main data that follows is to be read, and where
// that main data begins — which is often not in this frame at all.
//
// Layer III lets a frame borrow room from the ones before it. A frame's own bytes are a fixed budget
// set by the bitrate, but the audio in it needs whatever it needs, so a frame that needs less leaves
// its unused bytes for a later one to reach back into. The side information sits in the frame it
// belongs to and points backwards; everything after it is read from the pooled data.

// Granules is how many granules a frame of the given version holds. MPEG-1 codes two, the lower
// sampling frequencies one.
func granules(v Version) int {
	if v.lsf() {
		return 1
	}
	return 2
}

// BlockType is which transform a granule uses.
type BlockType int

// The block types, as the two-bit field numbers them. Type zero is the ordinary long block, which is
// what a granule has when it does no switching at all.
const (
	BlockLong BlockType = iota
	BlockStart
	BlockShort
	BlockStop
)

// GranuleInfo is one granule of one channel: everything needed to read its main data.
type GranuleInfo struct {
	// Part23Length is how many bits this granule's scale factors and Huffman data occupy.
	Part23Length int
	// BigValues is how many pairs of the spectrum are coded by the big-value tables. The count1
	// region follows it and the rest is zero.
	BigValues int
	// GlobalGain is the granule's overall level, and ScalefacCompress how its scale factors are
	// sized.
	GlobalGain       int
	ScalefacCompress int

	BlockType BlockType
	// Mixed reports a granule whose lowest bands are long and the rest short.
	//
	// The stream carries the flag for every switched granule, start and stop blocks included, where
	// it means nothing — the vector set has a file whose whole purpose is to set it there. It is
	// reported here only where it applies, so that nothing downstream has to remember to ask the
	// block type first.
	Mixed bool
	// TableSelect names a Huffman table for each of the three big-value regions, and RegionCount
	// where those regions end. A switched block has two regions rather than three.
	TableSelect  [3]int
	RegionCount  [3]int
	SubblockGain [3]int

	// Preflag adds a fixed boost to the upper scale factors. MPEG-1 reads it; the lower sampling
	// frequencies derive it from the scale factor sizing instead.
	Preflag bool
	// ScalefacScale is the step the scale factors are applied in, and Count1Table which of the two
	// tables codes the quadruples above the big values.
	ScalefacScale bool
	Count1Table   int

	// Scfsi says which scale factor band groups this granule takes from the granule before it rather
	// than coding again. Four flags, and only the second granule of an MPEG-1 frame can have them:
	// the first has nothing behind it.
	Scfsi int
}

// SideInfo is a frame's side information.
type SideInfo struct {
	// MainDataBegin is how many bytes before the end of this frame's header the main data starts.
	// Zero means it starts here; anything else reaches back into frames already read.
	MainDataBegin int
	// Private is the bits the format sets aside and says nothing about.
	Private int
	// Granule is indexed by granule then channel.
	Granule [2][2]GranuleInfo
}

// ParseSideInfo reads the side information from the start of a frame body.
//
// The layouts differ by more than their lengths. MPEG-1 codes two granules and gives each a
// four-bit scale factor sizing and a read flag for the preemphasis; the lower sampling frequencies
// code one granule with a nine-bit sizing and derive the preemphasis from it.
func ParseSideInfo(h Header, data []byte) (SideInfo, error) {
	var si SideInfo
	if len(data) < sideInfoSize(h) {
		return si, fmt.Errorf("%w: %d bytes of side information, need %d",
			ErrBadFrame, len(data), sideInfoSize(h))
	}
	r := bitio.NewMSBReader(data[:sideInfoSize(h)])
	channels := h.Channels()
	gr := granules(h.Version)

	// The private bits and the scale factor selection are packed against the main data pointer, so
	// they are read as one field and taken apart.
	var scfsi int
	if h.Version == MPEG1 {
		v, _ := r.Read(9)
		si.MainDataBegin = int(v)
		// Five private bits and four selection flags for a mono frame; three and eight for a pair.
		privateBits := uint(5)
		if channels == 2 {
			privateBits = 3
		}
		p, _ := r.Read(privateBits)
		si.Private = int(p)
		s, _ := r.Read(uint(4 * channels))
		scfsi = int(s)
	} else {
		v, _ := r.Read(8)
		si.MainDataBegin = int(v)
		p, _ := r.Read(uint(channels))
		si.Private = int(p)
	}

	for g := range gr {
		for ch := range channels {
			info, err := parseGranule(r, h)
			if err != nil {
				return si, err
			}
			// The flags are packed channel by channel, four to a channel, most significant first.
			// They belong to the second granule alone: the first has nothing behind it to take a
			// band group from, so it always codes its own.
			if h.Version == MPEG1 && g > 0 {
				info.Scfsi = scfsi >> (4 * (channels - 1 - ch)) & 0xF
			}
			si.Granule[g][ch] = info
		}
	}

	// A channel with a short block anywhere shares nothing.
	//
	// Sharing copies a band group from the granule before, and a short block's bands are not the
	// same bands — they are three windows of a third of the spectrum where a long block has one run
	// of it. So a short granule takes none, and a channel whose first granule is short leaves its
	// second nothing to take. The flags are still in the stream; they are ignored.
	if h.Version == MPEG1 {
		for ch := range channels {
			if si.Granule[0][ch].BlockType == BlockShort || si.Granule[1][ch].BlockType == BlockShort {
				si.Granule[1][ch].Scfsi = 0
			}
		}
	}
	return si, nil
}

// parseGranule reads one granule of one channel.
func parseGranule(r *bitio.MSBReader, h Header) (GranuleInfo, error) {
	var g GranuleInfo
	v, _ := r.Read(12)
	g.Part23Length = int(v)
	v, _ = r.Read(9)
	g.BigValues = int(v)
	// Two coefficients per big value, over a spectrum of 576. Anything more would read past the end
	// of the granule, and the vector set carries a file that tries.
	if g.BigValues > 288 {
		return g, fmt.Errorf("%w: %d big values, the spectrum holds 288 pairs", ErrBadFrame, g.BigValues)
	}
	v, _ = r.Read(8)
	g.GlobalGain = int(v)

	compressBits := uint(4)
	if h.Version.lsf() {
		compressBits = 9
	}
	v, _ = r.Read(compressBits)
	g.ScalefacCompress = int(v)

	switching, _ := r.ReadBool()
	if switching {
		v, _ = r.Read(2)
		g.BlockType = BlockType(v)
		if g.BlockType == BlockLong {
			// A granule that says it is switching and then names the unswitched block is describing
			// nothing. The vector set carries one.
			return g, fmt.Errorf("%w: a switched granule named the long block type", ErrBadFrame)
		}
		// Kept whatever block type the granule named, though only a short one can be mixed in the
		// ordinary sense. A start or stop block that sets it keeps its lowest subbands long as well,
		// so that they meet a mixed neighbour's long ones squarely; ignoring it there leaves those
		// subbands transformed the wrong length. The vector set carries a stream that does it.
		mixed, _ := r.ReadBool()
		g.Mixed = mixed

		// A switched granule has two regions rather than three, so it names two tables and the
		// region boundaries are fixed.
		for i := range 2 {
			v, _ = r.Read(5)
			g.TableSelect[i] = int(v)
		}
		g.RegionCount[0], g.RegionCount[1], g.RegionCount[2] = 7, 255, 255
		if g.BlockType == BlockShort && !g.Mixed {
			g.RegionCount[0] = 8
		}
		for i := range 3 {
			v, _ = r.Read(3)
			g.SubblockGain[i] = int(v)
		}
	} else {
		for i := range 3 {
			v, _ = r.Read(5)
			g.TableSelect[i] = int(v)
		}
		v, _ = r.Read(4)
		g.RegionCount[0] = int(v)
		v, _ = r.Read(3)
		g.RegionCount[1] = int(v)
		g.RegionCount[2] = 255
	}

	if h.Version == MPEG1 {
		g.Preflag, _ = r.ReadBool()
	} else {
		// Not in the stream at the lower sampling frequencies. It follows from the scale factor
		// sizing, which is nine bits there and carries the case in its upper range.
		g.Preflag = g.ScalefacCompress >= 500
	}
	g.ScalefacScale, _ = r.ReadBool()
	v, err := r.Read(1)
	g.Count1Table = int(v)
	return g, err
}

// String names a block type, for reporting.
func (b BlockType) String() string {
	switch b {
	case BlockStart:
		return "start"
	case BlockShort:
		return "short"
	case BlockStop:
		return "stop"
	}
	return "long"
}
