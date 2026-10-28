package mp3

import (
	"fmt"
	"math"

	"github.com/yegors/co-atc/third_party/goxiph/internal/bitio"
)

// Layers I and II, which are one decoder with three differences between them.
//
// Neither has a transform, a reservoir or a Huffman table. A frame says how many bits each subband
// was given, a scale factor to put the level back, and then the samples themselves — read straight
// out of the stream at whatever width the allocation named. The filterbank that turns those subbands
// back into samples is the same one layer III ends with.
//
// Layer II is three times as long, reads three samples from one codeword where the allocation is
// coarse, and carries up to three scale factors per subband rather than one.

// layer12 decodes one layer I or II frame into the decoder's scratch buffer.
//
// Nothing carries over between frames but the filterbank's delay line, which is what makes these
// layers seekable where layer III is not.
func (d *Decoder) layer12(fr Frame) error {
	h := fr.Header
	r := bitio.NewMSBReader(fr.Data)
	f, err := readLayer12Scales(r, h)
	if err != nil {
		return err
	}

	d.slots = [2][36][32]float64{}
	size := groupSize(h.Layer)
	for part := range layer12Parts {
		if err := readLayer12Part(r, h, f, part, &d.slots, part*4*size); err != nil {
			return err
		}
	}

	slots := slotsPerFrame(h.Layer)
	d.scratch.Format = d.Format()
	d.scratch.Resize(slots * 32)
	var pcm [32]float64
	for ch := range h.Channels() {
		for slot := range slots {
			d.synth[ch].synthesise(&d.slots[ch][slot], &pcm)
			for i := range 32 {
				d.scratch.Data[ch][slot*32+i] = float32(pcm[i])
			}
		}
	}
	return nil
}

// slotsPerFrame is how many time slots of thirty-two subbands a frame of each layer holds.
func slotsPerFrame(l Layer) int {
	if l == Layer1 {
		return 12
	}
	return 36
}

// layer12Parts is how many runs of samples a frame is read in. Both layers use three; layer II gives
// each its own scale factor and layer I gives all three the same one.
const layer12Parts = 3

// groupSize is how many samples a layer takes at once where the allocation is coarse enough to pack
// them into one codeword. Layer I never packs.
func groupSize(l Layer) int {
	if l == Layer1 {
		return 1
	}
	return 3
}

// allocClasses map an allocation code to the quantiser it names.
//
// A subband's allocation is a few bits whose meaning depends on where the subband sits: the low ones
// may be given any of sixteen quantisers and the high ones only a few, so each run of subbands reads
// a different width and looks the code up in a different row. Zero means the subband was given
// nothing. Codes from seventeen up name the packed quantisers, which layer II alone has.
//
// The rows are read as prefixes where a run uses fewer bits than the row is long.
var allocClasses = [7][16]uint8{
	{0, 17, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
	{0, 17, 18, 3, 19, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 16},
	{0, 17, 18, 3, 19, 4, 5, 16},
	{0, 17, 18, 16},
	{0, 17, 18, 19, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15},
	{0, 17, 18, 3, 19, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14},
	{0, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
}

// allocRun is a run of subbands whose allocations are read the same way.
type allocRun struct {
	row   int
	bits  uint
	bands int
}

// allocPlan is how a frame's allocations are laid out, and how many subbands carry anything at all.
//
// Layer I gives every subband four bits. Layer II gives the low subbands four, the middle three and
// the top two, and how far up the subbands go at all depends on the rate: a stream with little to
// spend stops well short of the top rather than spread itself thin.
func allocPlan(h Header) ([]allocRun, int) {
	if h.Layer == Layer1 {
		return []allocRun{{6, 4, 32}}, 32
	}
	if h.Version.lsf() {
		return []allocRun{{5, 4, 4}, {4, 3, 7}, {4, 2, 19}}, 30
	}

	// The rate each channel gets, which is what decides how far up the subbands reach. A free format
	// stream names no rate, and the one the vector set carries is a generous one.
	kbps := h.Bitrate / 1000
	if h.Mode != Mono {
		kbps /= 2
	}
	if kbps == 0 {
		kbps = 192
	}

	switch {
	case kbps < 56:
		bands := 8
		if rateIndex(h) == 2 {
			bands = 12
		}
		return []allocRun{{4, 4, 2}, {4, 3, 10}}, bands
	case kbps >= 96 && rateIndex(h) != 1:
		return []allocRun{{0, 4, 3}, {1, 4, 8}, {2, 3, 12}, {3, 2, 7}}, 30
	default:
		return []allocRun{{0, 4, 3}, {1, 4, 8}, {2, 3, 12}, {3, 2, 7}}, 27
	}
}

// quantiser is what an allocation code resolves to: how many steps the subband was divided into and
// how the codewords are read.
//
// A plain quantiser spends one codeword per sample, as many bits as it takes. A packed one spends one
// codeword on three samples, which is what makes three, five and nine steps worth having: three steps
// cost five bits for three samples rather than six.
type quantiser struct {
	levels int
	bits   uint
	packed bool
}

// quantiserFor resolves an allocation code. The second result is false where the subband was given
// nothing.
func quantiserFor(class uint8) (quantiser, bool) {
	switch {
	case class == 0:
		return quantiser{}, false
	case class < 17:
		return quantiser{levels: 1<<class - 1, bits: uint(class)}, true
	default:
		levels := 2<<(class-17) + 1
		bits := uint(math.Ceil(3 * math.Log2(float64(levels))))
		return quantiser{levels: levels, bits: bits, packed: true}, true
	}
}

// layer12Scale is the level a scale factor stands for.
//
// Sixty-four positions two decibels apart, which the format tabulates and this computes. The last is
// out of range and appears only in streams written to see what a decoder does with it; the formula
// gives it a value like any other, which is all that is needed to keep decoding.
func layer12Scale(index int) float64 { return math.Exp2(1 - float64(index)/3) }

// layer12Frame is one frame's worth of allocations and scale factors.
type layer12Frame struct {
	bands   int
	shared  int // the subband from which both channels read one set of samples
	classes [2][32]uint8
	scale   [2][32][3]float64
}

// stereoBands is the subband from which a joint stereo frame stops coding the two channels apart.
//
// Above it one set of samples serves both, each channel keeping its own scale factor — the coarsest
// stereo coupling in the family, and the only one either layer has.
func stereoBands(h Header, bands int) int {
	switch h.Mode {
	case Mono:
		return 0
	case JointStereo:
		return min(h.ModeExtension<<2+4, bands)
	default:
		return bands
	}
}

// readLayer12Scales reads a frame's allocations and scale factors, which come before any of its
// samples.
func readLayer12Scales(r *bitio.MSBReader, h Header) (layer12Frame, error) {
	runs, bands := allocPlan(h)
	channels := h.Channels()

	f := layer12Frame{bands: bands, shared: stereoBands(h, bands)}
	at, run := 0, 0
	for band := range bands {
		for at+runs[run].bands <= band {
			at += runs[run].bands
			run++
			if run >= len(runs) {
				return f, fmt.Errorf("%w: the allocation runs out at subband %d", ErrBadFrame, band)
			}
		}
		row, bits := runs[run].row, runs[run].bits

		code, err := r.Read(bits)
		if err != nil {
			return f, err
		}
		f.classes[0][band] = allocClasses[row][code]
		if channels == 2 {
			// Above the shared bound the second channel takes the first's allocation rather than one
			// of its own, because from there up it takes the first's samples too.
			if band < f.shared {
				code, err = r.Read(bits)
				if err != nil {
					return f, err
				}
			}
			f.classes[1][band] = allocClasses[row][code]
		}
	}

	// Which of the three parts share a scale factor. Layer I has one part and needs none of this.
	var selection [2][32]uint8
	if h.Layer != Layer1 {
		for band := range bands {
			for ch := range channels {
				if f.classes[ch][band] == 0 {
					continue
				}
				v, err := r.Read(2)
				if err != nil {
					return f, err
				}
				selection[ch][band] = uint8(v)
			}
		}
	}

	for band := range bands {
		for ch := range channels {
			if f.classes[ch][band] == 0 {
				continue
			}
			scales, err := readLayer12Scale(r, h, selection[ch][band])
			if err != nil {
				return f, err
			}
			f.scale[ch][band] = scales
		}
	}
	return f, nil
}

// scaleReads says which of a subband's three scale factors are in the stream, for each of the four
// ways a frame may share them: all three given, the first two the same, all three the same, the last
// two the same. One that is not given repeats the one before it.
var scaleReads = [4][3]bool{
	{true, true, true},
	{true, false, true},
	{true, false, false},
	{true, true, false},
}

// readLayer12Scale reads one subband's scale factors. Layer I gives one and holds it for the whole
// frame; layer II gives up to three and says which of them it left out.
func readLayer12Scale(r *bitio.MSBReader, h Header, selection uint8) ([3]float64, error) {
	reads := scaleReads[2]
	if h.Layer != Layer1 {
		reads = scaleReads[selection]
	}

	var out [3]float64
	for i, given := range reads {
		if !given {
			out[i] = out[i-1]
			continue
		}
		v, err := r.Read(6)
		if err != nil {
			return out, err
		}
		out[i] = layer12Scale(int(v))
	}
	return out, nil
}

// readLayer12Part reads one part of a frame into slots of thirty-two subbands, starting at slot at.
//
// The samples come subband by subband and channel by channel, a few at a time, so that a decoder can
// fill its filterbank as it reads rather than hold the frame.
func readLayer12Part(r *bitio.MSBReader, h Header, f layer12Frame, part int,
	slots *[2][36][32]float64, at int,
) error {
	channels := h.Channels()
	size := groupSize(h.Layer)

	// Four passes either way: layer I takes one sample of each subband per pass and layer II three.
	for pass := range 4 {
		for band := range f.bands {
			for ch := range channels {
				// Above the shared bound only the first channel's samples are in the stream, and the
				// second takes them as they are read.
				if ch == 1 && band >= f.shared {
					continue
				}
				q, coded := quantiserFor(f.classes[ch][band])
				if !coded {
					continue
				}
				values, err := readQuantised(r, q, size)
				if err != nil {
					return err
				}
				for k := range size {
					slot := at + pass*size + k
					slots[ch][slot][band] = values[k] * f.scale[ch][band][part]
					if ch == 0 && channels == 2 && band >= f.shared {
						slots[1][slot][band] = values[k] * f.scale[1][band][part]
					}
				}
			}
		}
	}
	return nil
}

// readQuantised reads one group of samples and puts them back on the scale they were taken from.
//
// A packed quantiser holds all three in one codeword, each digit of the codeword in a base of as many
// steps as the quantiser has. Either way a sample is a step counted from the middle, divided by the
// number of steps to land between minus one and one.
func readQuantised(r *bitio.MSBReader, q quantiser, size int) ([3]float64, error) {
	var out [3]float64
	if q.packed {
		code, err := r.Read(q.bits)
		if err != nil {
			return out, err
		}
		for k := range size {
			out[k] = float64(int(code%uint64(q.levels))-q.levels/2) / float64(q.levels)
			code /= uint64(q.levels)
		}
		return out, nil
	}
	for k := range size {
		v, err := r.Read(q.bits)
		if err != nil {
			return out, err
		}
		out[k] = float64(int(v)-q.levels/2) / float64(q.levels)
	}
	return out, nil
}
