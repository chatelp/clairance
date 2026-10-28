package mp3

import (
	"github.com/yegors/co-atc/third_party/goxiph/internal/bitio"
)

// Scale factors are how a granule's bands are levelled against each other: the spectrum is quantised
// flat and each band is then scaled back by its own factor, so that the quantisation noise follows
// the signal rather than sitting at one level across the whole spectrum.
//
// The two versions share nothing but the purpose. MPEG-1 codes twenty-one bands with two widths,
// split at band eleven, and lets the second granule take a band group unchanged from the first. The
// lower sampling frequencies code four partitions whose widths come out of a mixed-radix division of
// one field, whose band counts depend on the block type, and which change again when the channel is
// coded as an intensity difference.

// bandLayout is how a granule's bands are laid out: the boundaries, and how many are long and short.
type bandLayout struct {
	// bounds are the bin each band begins at. A short block's are given once per window.
	bounds []int16
	// long and short are how many bands of each kind the granule has.
	long, short int
}

// layoutFor returns the band layout a granule uses.
//
// The row is chosen by the sample rate, in the order the header numbers them, with the lower
// sampling frequencies following MPEG-1's three. A mixed block keeps its lowest bands long — eight
// of them at MPEG-1 and six at the lower rates, which is a difference nothing announces.
func layoutFor(h Header, g GranuleInfo) bandLayout {
	row := rateRow(h)
	switch {
	case g.BlockType == BlockShort && g.Mixed:
		long := 8
		if h.Version.lsf() {
			long = 6
		}
		return bandLayout{bounds: scfBandMixed[row][:], long: long, short: 30}
	case g.BlockType == BlockShort:
		return bandLayout{bounds: scfBandShort[row][:], long: 0, short: 39}
	default:
		return bandLayout{bounds: scfBandLong[row][:], long: 22, short: 0}
	}
}

// rateRow is which row of the band tables a header selects.
//
// Not what it looks like. There are nine combinations of version and rate but only eight rows: the
// lowest two — the half-rate extension at 11 and 12 kHz — share one. So the rows run from the
// slowest rate upwards, the extension first and MPEG-1 last, and the first two collapse together.
//
// Getting this wrong is quiet. The bands are still bands, the regions still split somewhere, and the
// granule still decodes a plausible-looking spectrum; only the boundaries are in the wrong places, so
// some pairs are read from the wrong table. It shows up as the right number of values costing the
// wrong number of bits.
func rateRow(h Header) int {
	score := 0
	switch h.Version {
	case MPEG1:
		score = 2
	case MPEG2:
		score = 1
	}
	row := rateIndex(h) + score*3
	if row != 0 {
		row--
	}
	return row
}

// rateIndex is where a header's sample rate sits among the three its version allows.
func rateIndex(h Header) int {
	for i, hz := range sampleRates[h.Version] {
		if hz == h.SampleRate {
			return i
		}
	}
	return 0
}

// scaleFactors holds one granule's factors, and where the intensity positions were.
type scaleFactors struct {
	// value is the factor for each band, in the layout's order.
	value [40]uint8
	// intensity is the same reading, with the largest representable value marked as absent. Intensity
	// stereo uses it as a position rather than a level, and the largest value means "not coupled".
	intensity [40]uint8
}

// bands is how many bands a layout holds altogether.
func (l bandLayout) bands() int { return l.long + l.short }

// readScaleFactors decodes one granule's scale factors.
//
// previous carries the granule before it, which MPEG-1 may take band groups from unchanged. It is
// ignored at the lower sampling frequencies, which have no such field.
func readScaleFactors(r *bitio.MSBReader, h Header, g GranuleInfo, ch int, previous *scaleFactors) scaleFactors {
	var sf scaleFactors
	layout := layoutFor(h, g)

	var widths [4]int
	var counts [4]int
	share := g.Scfsi

	if h.Version == MPEG1 {
		// Two widths, packed two bits each, covering bands nought to ten and eleven to twenty.
		part := int(scfCompress[g.ScalefacCompress&0xF])
		widths[0], widths[1] = part>>2, part>>2
		widths[2], widths[3] = part&3, part&3
		for i := range 4 {
			counts[i] = int(scfPartitions[partitionRow(g)][i])
		}
	} else {
		widths, counts = lsfSizing(h, g, ch)
		// No sharing at the lower sampling frequencies, and the largest value in a partition marks
		// an uncoupled band rather than a level.
		share = -1
	}

	at := 0
	for i := range 4 {
		count := counts[i]
		if count == 0 {
			break
		}
		switch {
		case share > 0 && share&8 != 0:
			// This group is the one the granule before it read. Nothing is in the stream for it.
			copy(sf.value[at:at+count], previous.value[at:at+count])
			copy(sf.intensity[at:at+count], previous.intensity[at:at+count])
		case widths[i] == 0:
			// A width of nothing means every factor in the group is nothing.
			for k := range count {
				sf.value[at+k], sf.intensity[at+k] = 0, 0
			}
		default:
			largest := -1
			if share < 0 {
				largest = 1<<widths[i] - 1
			}
			for k := range count {
				v, _ := r.Read(uint(widths[i]))
				sf.value[at+k] = uint8(v)
				if int(v) == largest {
					sf.intensity[at+k] = intensityAbsent
				} else {
					sf.intensity[at+k] = uint8(v)
				}
			}
		}
		at += count
		if share > 0 {
			share *= 2
		}
	}

	applyGains(&sf, layout, g, h)
	return sf
}

// intensityAbsent marks a band that intensity stereo does not couple.
const intensityAbsent = 0xFF

// partitionRow is which of the three band-count layouts a granule uses: long, mixed, or short.
func partitionRow(g GranuleInfo) int {
	switch {
	case g.BlockType == BlockShort && g.Mixed:
		return 1
	case g.BlockType == BlockShort:
		return 2
	default:
		return 0
	}
}

// lsfSizing decides the four partition widths and band counts at the lower sampling frequencies.
//
// The widths are a mixed-radix division: the sizing field is divided by four moduli in turn, and
// which four moduli depends on how far the field reaches. Which set of band counts goes with them
// falls out of the same walk, which is why the two are decided together.
func lsfSizing(h Header, g GranuleInfo, ch int) (widths [4]int, counts [4]int) {
	layout := partitionRow(g)

	// An intensity-coded second channel halves the field and reads its widths from the second half
	// of the modulus table.
	intensity := 0
	if h.Mode == JointStereo && h.ModeExtension&1 != 0 && ch == 1 {
		intensity = 1
	}
	sizing := g.ScalefacCompress >> intensity

	// Walk the modulus groups until the sizing is used up. Each group is a radix for the four
	// widths, and the group that consumes the remainder is the one that describes them.
	k := intensity * 12
	for {
		product := 1
		for i := 3; i >= 0; i-- {
			widths[i] = sizing / product % int(scfModulus[k+i])
			product *= int(scfModulus[k+i])
		}
		sizing -= product
		k += 4
		if sizing < 0 || k+3 >= len(scfModulus) {
			break
		}
	}

	// The band counts come from the group after the one that gave the widths. The two tables are
	// offset against each other, which is not a symmetry to assume: the counts table has seven groups
	// where the moduli have six.
	if k+3 >= len(scfPartitions[layout]) {
		k = len(scfPartitions[layout]) - 4
	}
	for i := range 4 {
		counts[i] = int(scfPartitions[layout][k+i])
	}
	return widths, counts
}

// applyGains folds the per-window gains and the preemphasis into the factors.
//
// A short block's three windows each carry their own gain, applied to every band of that window. A
// long block may instead ask for preemphasis, a fixed boost to its upper ten bands. No granule has
// both: one is for switched blocks and the other is not.
func applyGains(sf *scaleFactors, layout bandLayout, g GranuleInfo, h Header) {
	shift := 1
	if g.ScalefacScale {
		shift = 2
	}
	if layout.short > 0 {
		up := 3 - shift
		for i := 0; i < layout.short; i += 3 {
			for w := range 3 {
				sf.value[layout.long+i+w] += uint8(g.SubblockGain[w] << up)
			}
		}
		return
	}
	if g.Preflag {
		for i := range 10 {
			sf.value[11+i] += scfPreamp[i]
		}
	}
}
