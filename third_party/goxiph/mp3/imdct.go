package mp3

import "math"

// What stands between the spectrum and the filterbank: putting a short block's values back in order,
// undoing the aliasing the encoder's own filterbank left, transforming each subband back into time,
// and flipping the sign of every other one.
//
// All of it is closed form except eight numbers, and those eight are the only thing here that had to
// be taken from anywhere.

// aliasC are the eight coefficients the butterflies are built from.
//
// The specification's, and the one table in this file with no formula behind it. Everything derived
// from them is computed.
var aliasC = [8]float64{-0.6, -0.535, -0.33, -0.185, -0.095, -0.041, -0.0142, -0.0037}

// aliasCS and aliasCA are the butterfly's two coefficients, a cosine and a sine of the same angle.
var aliasCS, aliasCA = func() (cs, ca [8]float64) {
	for i, c := range aliasC {
		norm := math.Sqrt(1 + c*c)
		cs[i], ca[i] = 1/norm, -c/norm
	}
	return cs, ca
}()

// imdctBasis is the inverse transform's cosine matrix for a block of n outputs, computed once.
func imdctBasis(n int) [][]float64 {
	half := n / 2
	m := make([][]float64, n)
	for i := range m {
		m[i] = make([]float64, half)
		for k := range half {
			m[i][k] = math.Cos(math.Pi / float64(2*n) * float64(2*i+1+half) * float64(2*k+1))
		}
	}
	return m
}

var (
	imdctLong  = imdctBasis(36)
	imdctShort = imdctBasis(12)
)

// blockWindows are the four shapes a granule's transform is windowed by.
//
// The long one is half a sine period. The start and stop shapes are that on one side and a short
// block's own window on the other, with a flat stretch between: they are what lets a granule change
// transform length without a step at the join.
var blockWindows = func() *[4][36]float64 {
	var w [4][36]float64
	long := func(i int) float64 { return math.Sin(math.Pi / 36 * (float64(i) + 0.5)) }
	short := func(i int) float64 { return math.Sin(math.Pi / 12 * (float64(i) + 0.5)) }

	for i := range 36 {
		w[BlockLong][i] = long(i)
		w[BlockShort][i] = long(i) // unused; a short granule windows its three sub-blocks itself
	}
	for i := range 18 {
		w[BlockStart][i] = long(i)
		w[BlockStop][18+i] = long(18 + i)
	}
	for i := range 6 {
		w[BlockStart][18+i] = 1
		w[BlockStart][24+i] = short(6 + i)
		w[BlockStop][6+i] = short(i)
		w[BlockStop][12+i] = 1
	}
	return &w
}()

// reorder puts a short block's values back into the order the transform wants.
//
// A short granule codes its three windows band by band rather than window by window, because that is
// how the scale factors group them. The transform needs the opposite, so the values are interleaved
// back: three windows of one band become one band of three windows.
func reorder(sub []float32, layout bandLayout, from int) {
	var scratch [576]float32
	src, dst := from, 0
	for band := layout.long; band+2 < layout.bands(); band += 3 {
		width := int(layout.bounds[band+1]) - int(layout.bounds[band])
		if width <= 0 || src+3*width > len(sub) {
			break
		}
		for i := range width {
			scratch[dst] = sub[src+i]
			scratch[dst+1] = sub[src+i+width]
			scratch[dst+2] = sub[src+i+2*width]
			dst += 3
		}
		src += 3 * width
	}
	copy(sub[from:from+dst], scratch[:dst])
}

// antialias undoes the overlap the encoder's filterbank left between neighbouring subbands.
//
// Eight butterflies across each boundary, each mixing a value from the top of one subband with its
// mirror at the bottom of the next. The encoder introduced that overlap on purpose and it has to come
// out in the same places.
func antialias(sub *[576]float32, bands int) {
	for b := range bands {
		at := b * 18
		for i := range 8 {
			u := float64(sub[at+18+i])
			d := float64(sub[at+17-i])
			sub[at+18+i] = float32(u*aliasCS[i] - d*aliasCA[i])
			sub[at+17-i] = float32(u*aliasCA[i] + d*aliasCS[i])
		}
	}
}

// imdctGranule transforms every subband back into time, windows it, and laps it onto what the granule
// before left behind.
//
// overlap carries eighteen samples per subband from the previous granule. A mixed block transforms
// its lowest subbands long and the rest short, which is the whole point of it: the low end keeps its
// frequency resolution while the top follows a transient.
func imdctGranule(sub *[576]float32, overlap *[576]float32, g GranuleInfo, longBands int) {
	for b := range 32 {
		at := b * 18
		var raw [36]float64

		if b < longBands || g.BlockType != BlockShort {
			shape := g.BlockType
			if b < longBands {
				shape = BlockLong
			}
			// Half the transform comes out of the other half. Of the thirty-six sums only eighteen
			// need computing: the first half mirrors about its centre with a sign change and the
			// second half mirrors about its own without one. Both are properties of the cosine and
			// the tests pin them. Each position still takes its own window tap.
			for n := range 9 {
				var v float64
				row := imdctLong[n]
				for k := range 18 {
					v += float64(sub[at+k]) * row[k]
				}
				raw[n] = v * blockWindows[shape][n]
				raw[17-n] = -v * blockWindows[shape][17-n]
			}
			for n := 18; n < 27; n++ {
				var v float64
				row := imdctLong[n]
				for k := range 18 {
					v += float64(sub[at+k]) * row[k]
				}
				raw[n] = v * blockWindows[shape][n]
				raw[53-n] = v * blockWindows[shape][53-n]
			}
		} else {
			// Three transforms of a third the length, each windowed and laid down six samples along
			// from the one before.
			for w := range 3 {
				for n := range 12 {
					var v float64
					row := imdctShort[n]
					for k := range 6 {
						v += float64(sub[at+3*k+w]) * row[k]
					}
					raw[6*w+6+n] += v * math.Sin(math.Pi/12*(float64(n)+0.5))
				}
			}
		}

		for i := range 18 {
			sub[at+i] = float32(raw[i]) + overlap[at+i]
			overlap[at+i] = float32(raw[18+i])
		}
	}
}

// invertFrequencies flips the sign of every other sample of every other subband.
//
// The filterbank that follows expects its odd subbands reversed in frequency, which the encoder's
// left them as. It costs nothing and is easy to leave out, and leaving it out sounds like a different
// piece of music.
func invertFrequencies(sub *[576]float32) {
	for b := 1; b < 32; b += 2 {
		for i := 1; i < 18; i += 2 {
			sub[b*18+i] = -sub[b*18+i]
		}
	}
}

// longBandsOf is how many of a granule's subbands keep the long transform.
//
// None unless the granule is mixed, and then two — or four at the slowest rate, where a subband
// covers so little that two would not reach far enough down.
func longBandsOf(h Header, g GranuleInfo) int {
	if !g.Mixed {
		return 0
	}
	if h.Version == MPEG25 && rateIndex(h) == 2 {
		return 4
	}
	return 2
}
