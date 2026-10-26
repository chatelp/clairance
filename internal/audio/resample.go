package audio

import "math"

// resampler converts a stream of samples from one rate to another, as they
// arrive, by band-limited interpolation: a sinc kernel of taps input samples on
// each side, under a Blackman window, its cutoff at the lower of the two
// Nyquist frequencies. The rates are in a rational ratio, so every output
// sample falls on one of a fixed number of phases between input samples, and
// the kernel is computed once per phase.
//
// Measured against ffmpeg's own resampler on the station's streams (8 to
// 24 kHz, 26/10, docs-fr/05 Q60): 72 to 80 dB apart, for 5.6 s of CPU per
// hour of audio.
type resampler struct {
	up, down, taps int
	phases         [][]float64

	hist []float64 // input samples from absolute index base on
	base int64
	next int64 // the next output sample's absolute index
}

func newResampler(from, to, taps int) *resampler {
	g := gcd(from, to)
	r := &resampler{up: to / g, down: from / g, taps: taps}
	if r.up == r.down {
		return r
	}
	cutoff := math.Min(1, float64(r.up)/float64(r.down)) // of the input's Nyquist
	width := 2 * taps
	r.phases = make([][]float64, r.up)
	for ph := range r.phases {
		frac := float64(ph) / float64(r.up)
		k := make([]float64, width)
		var norm float64
		for t := range k {
			x := (frac - float64(t-taps+1)) * cutoff
			k[t] = windowedSinc(x, float64(taps)*cutoff)
			norm += k[t]
		}
		for t := range k {
			k[t] /= norm
		}
		r.phases[ph] = k
	}
	return r
}

// process takes the next input samples, in [-1, 1], and appends to out the
// 16-bit output samples they complete. An output sample waits for taps input
// samples after it: taps/from seconds, 4 ms for 32 taps at 8 kHz.
func (r *resampler) process(in []float64, out []int16) []int16 {
	if r.up == r.down {
		for _, v := range in {
			out = append(out, toInt16(v))
		}
		return out
	}
	r.hist = append(r.hist, in...)
	end := r.base + int64(len(r.hist))
	up, down, taps := int64(r.up), int64(r.down), int64(r.taps)
	for {
		pos := r.next * down
		c, ph := pos/up, pos%up
		if c+taps >= end {
			break
		}
		k := r.phases[ph]
		var sum float64
		first := c - taps + 1
		for t, w := range k {
			if idx := first + int64(t); idx >= r.base {
				sum += r.hist[idx-r.base] * w
			}
		}
		out = append(out, toInt16(sum))
		r.next++
	}
	// Keep only what the next output sample can still reach.
	if keep := (r.next*down)/up - taps + 1; keep > r.base {
		drop := int(keep - r.base)
		r.hist = append(r.hist[:0], r.hist[drop:]...)
		r.base = keep
	}
	return out
}

// windowedSinc is a sinc at x under a Blackman window of half-width half.
func windowedSinc(x, half float64) float64 {
	if math.Abs(x) >= half {
		return 0
	}
	s := 1.0
	if x != 0 {
		s = math.Sin(math.Pi*x) / (math.Pi * x)
	}
	u := (x/half + 1) / 2
	return s * (0.42 - 0.5*math.Cos(2*math.Pi*u) + 0.08*math.Cos(4*math.Pi*u))
}

// toInt16 writes a sample in [-1, 1] as ffmpeg does: rounded, clipped.
func toInt16(v float64) int16 {
	q := math.Round(v * 32768)
	if q > math.MaxInt16 {
		return math.MaxInt16
	}
	if q < math.MinInt16 {
		return math.MinInt16
	}
	return int16(q)
}

func gcd(a, b int) int {
	for b != 0 {
		a, b = b, a%b
	}
	return a
}
