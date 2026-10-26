package mp3

import "math"

// Requantisation is where the integers become a spectrum.
//
// The Huffman pass gives whole numbers, quantised with a power law rather than evenly: the coder took
// the three-quarter power of each value before rounding, so the decoder takes the four-thirds power
// to get back. That puts the quantisation error in proportion to the value, which is what a spectrum
// spanning ninety decibels needs.
//
// Each band is then scaled by its own factor, and the whole granule by a gain. Both are powers of two
// in quarter steps, so they combine into one exponent per band.

// dequantiserOffset is what the granule's gain is measured from.
//
// The 210 is the format's; the extra four takes the output down an octave, which is the scale the
// reference decoder works in and therefore the scale its output is compared against.
const dequantiserOffset = 210 + 4

// requantise turns one granule's quantised values into the scaled spectrum.
//
// The bands are walked in the layout's order, each carrying its own factor, and stop where the layout
// runs out — the bins above the last band are zero and stay zero. midSide says the frame's channels
// are coded as a sum and a difference, which costs the granule two quarter steps of gain: the pair
// will be recombined and a sum of two channels is twice one.
func requantise(sp *spectrum, sf *scaleFactors, layout bandLayout, g GranuleInfo, midSide bool, out *[576]float32) {
	shift := 1
	if g.ScalefacScale {
		shift = 2
	}
	gain := g.GlobalGain - dequantiserOffset
	if midSide {
		gain -= 2
	}

	at := 0
	for band := range layout.bands() {
		end := int(layout.bounds[band+1])
		if end > 576 {
			end = 576
		}
		if end <= at {
			continue
		}
		scale := math.Exp2(float64(gain-int(sf.value[band])<<shift) / 4)
		for i := at; i < end; i++ {
			out[i] = float32(scale * pow43(sp[i]))
		}
		at = end
	}
	for i := at; i < 576; i++ {
		out[i] = 0
	}
}

// pow43 is the four-thirds power, keeping the sign.
//
// The value carries its sign and the power does not, so the two are taken apart and put back. Zero is
// the commonest value by a long way and is worth not calling into the library for.
func pow43(v int) float64 {
	switch {
	case v == 0:
		return 0
	case v < 0:
		return -math.Pow(float64(-v), 4.0/3)
	default:
		return math.Pow(float64(v), 4.0/3)
	}
}
