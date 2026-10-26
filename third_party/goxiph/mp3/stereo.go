package mp3

import "math"

// A stereo pair is rarely coded as two channels. Two couplings are available and a frame may use
// either, or both at once on different bands.
//
// Mid and side codes the sum and the difference, which costs nothing when the channels are alike and
// is undone by adding and subtracting again. Intensity codes only the sum above some band and gives
// each channel a share of it, so the upper spectrum keeps its loudness in each ear and loses its
// difference — which is most of what hearing takes from high frequencies anyway.

// intensityPan is the share of a band's energy each channel takes, for the eight positions MPEG-1
// allows.
//
// Computed rather than tabulated: the position is an angle in twelfths of a right angle, and the two
// shares are its sine and cosine normalised to sum to one. The last position is the whole of it to
// the left, which the formula gives without a special case.
func intensityPan(pos int) (left, right float64) {
	s, c := math.Sincos(float64(pos) * math.Pi / 12)
	return s / (s + c), c / (s + c)
}

// applyStereo undoes whichever couplings a frame used, in place over a pair of granules.
//
// left and right are one granule of each channel. The bands are walked together because the two
// couplings are decided band by band: intensity takes the bands above where the second channel fell
// silent, and mid-side takes whatever is left.
func applyStereo(left, right *[576]float32, h Header, g GranuleInfo, sf *scaleFactors, layout bandLayout, sideCompress int) {
	midSide := h.Mode == JointStereo && h.ModeExtension&2 != 0
	intensity := h.Mode == JointStereo && h.ModeExtension&1 != 0

	if !intensity {
		if midSide {
			for i := range 576 {
				a, b := left[i], right[i]
				left[i], right[i] = a+b, a-b
			}
		}
		return
	}

	// Where the second channel stops carrying anything. Above that band the pair is one signal
	// shared out, and below it whatever coupling the frame declared still applies.
	//
	// A short block has three windows interleaved band by band, and each keeps its own limit; a long
	// block has one, so the three collapse to the highest.
	top := topBands(right, layout)
	if layout.short == 0 {
		high := max(max(top[0], top[1]), top[2])
		top = [3]int{high, high, high}
	}

	// The topmost band has no position of its own to trust.
	//
	// It sits above everything the second channel carried, so whatever its scale factor says was
	// never meant as a position. Where the second channel did reach that far it takes a default, and
	// where it did not it takes the position of the band below it — the band three down for a short
	// block, one down for a long one, since a short block's three windows each keep their own run.
	blocks := 1
	if layout.short > 0 {
		blocks = 3
	}
	def := uint8(3)
	if h.Version.lsf() {
		def = 0
	}
	for i := range blocks {
		at := layout.bands() - blocks + i
		below := at - blocks
		if at < 0 || at >= len(sf.intensity) {
			continue
		}
		if below < 0 || top[i] >= below {
			sf.intensity[at] = def
		} else {
			sf.intensity[at] = sf.intensity[below]
		}
	}

	maxPos := 7
	if h.Version.lsf() {
		maxPos = 64
	}
	// A frame that couples both ways scales the shares, because the sum it is sharing out is a mid
	// channel rather than a left one.
	amplitude := 1.0
	if midSide {
		amplitude = math.Sqrt2
	}

	at := 0
	for band := range layout.bands() {
		end := min(int(layout.bounds[band+1]), 576)
		if end <= at {
			continue
		}
		pos := int(sf.intensity[band])
		switch {
		case band > top[band%3] && pos < maxPos:
			kl, kr := intensityShares(pos, h, sideCompress)
			for i := at; i < end; i++ {
				v := float64(left[i])
				left[i] = float32(v * kl * amplitude)
				right[i] = float32(v * kr * amplitude)
			}
		case midSide:
			for i := at; i < end; i++ {
				a, b := left[i], right[i]
				left[i], right[i] = a+b, a-b
			}
		}
		at = end
	}
}

// intensityShares is what each channel takes of a coupled band.
//
// MPEG-1 divides a right angle into twelfths. The lower sampling frequencies instead halve one
// channel repeatedly, by a step the second channel's own scale factor sizing chooses — which is the
// only place in the format where one channel's side information decides something about the other.
func intensityShares(pos int, h Header, sideCompress int) (left, right float64) {
	if !h.Version.lsf() {
		return intensityPan(pos)
	}
	shift := sideCompress & 1
	step := math.Exp2(-float64((pos+1)>>1<<shift) / 4)
	if pos&1 != 0 {
		return step, 1
	}
	return 1, step
}

// topBands is the highest band of each of the three windows in which the second channel still holds
// anything. A long block uses one window and the other two come back empty.
func topBands(right *[576]float32, layout bandLayout) [3]int {
	top := [3]int{-1, -1, -1}
	at := 0
	for band := range layout.bands() {
		end := min(int(layout.bounds[band+1]), 576)
		for i := at; i < end; i++ {
			if right[i] != 0 {
				top[band%3] = band
				break
			}
		}
		at = end
	}
	return top
}
