package audio

import (
	"fmt"
	"math"
)

// Interleave writes b as interleaved frames into dst, which must hold Frames()*Channels samples.
func Interleave(b *Buffer, dst []float32) error {
	n, ch := b.Frames(), b.Format.Channels
	if len(dst) != n*ch {
		return fmt.Errorf("audio: interleave: dst holds %d samples, need %d", len(dst), n*ch)
	}
	for c, plane := range b.Data {
		i := c
		for _, s := range plane {
			dst[i] = s
			i += ch
		}
	}
	return nil
}

// Deinterleave splits src into b, resizing b to fit.
func Deinterleave(src []float32, b *Buffer) error {
	ch := b.Format.Channels
	if ch == 0 || len(src)%ch != 0 {
		return fmt.Errorf("audio: deinterleave: %d samples across %d channels", len(src), ch)
	}
	b.Resize(len(src) / ch)
	for c, plane := range b.Data {
		i := c
		for j := range plane {
			plane[j] = src[i]
			i += ch
		}
	}
	return nil
}

// MaxIntDepth is the deepest integer sample that survives a round trip through the float32 buffer.
//
// float32 carries a 24-bit mantissa, so integers wider than 24 bits lose their low bits on
// conversion. FLAC permits up to 32; those streams need an integer path that never touches
// audio.Buffer.
const MaxIntDepth = 24

// scaleForDepth is the divisor mapping an integer sample of the given bit depth onto [-1, 1).
//
// The scale is 2^(depth-1), not 2^(depth-1)-1. Only the power of two makes int -> float -> int
// bit-exact, which the lossless FLAC and WAV paths depend on.
func scaleForDepth(depth int) (float32, error) {
	if depth < 2 || depth > MaxIntDepth {
		return 0, fmt.Errorf("%w: bit depth %d, want 2..%d", ErrInvalidFormat, depth, MaxIntDepth)
	}
	return float32(int64(1) << (depth - 1)), nil
}

// FromInt writes interleaved integer samples of the given bit depth into b, resizing it to fit.
// Samples are sign-extended values in the low bits of each int32, which is how FLAC presents them.
func FromInt(src []int32, depth int, b *Buffer) error {
	scale, err := scaleForDepth(depth)
	if err != nil {
		return err
	}
	ch := b.Format.Channels
	if ch == 0 || len(src)%ch != 0 {
		return fmt.Errorf("audio: fromint: %d samples across %d channels", len(src), ch)
	}
	b.Resize(len(src) / ch)
	for c, plane := range b.Data {
		i := c
		for j := range plane {
			plane[j] = float32(src[i]) / scale
			i += ch
		}
	}
	return nil
}

// ToInt writes b into dst as interleaved integers of the given bit depth, clamping out-of-range
// samples. dst must hold Frames()*Channels values.
func ToInt(b *Buffer, depth int, dst []int32) error {
	scale, err := scaleForDepth(depth)
	if err != nil {
		return err
	}
	n, ch := b.Frames(), b.Format.Channels
	if len(dst) != n*ch {
		return fmt.Errorf("audio: toint: dst holds %d samples, need %d", len(dst), n*ch)
	}
	lo, hi := -int32(scale), int32(scale)-1
	for c, plane := range b.Data {
		i := c
		for _, s := range plane {
			v := int32(math.Round(float64(s * scale)))
			dst[i] = min(max(v, lo), hi)
			i += ch
		}
	}
	return nil
}

// FromInt16 writes interleaved 16-bit samples into b, resizing it to fit.
func FromInt16(src []int16, b *Buffer) error {
	ch := b.Format.Channels
	if ch == 0 || len(src)%ch != 0 {
		return fmt.Errorf("audio: fromint16: %d samples across %d channels", len(src), ch)
	}
	b.Resize(len(src) / ch)
	for c, plane := range b.Data {
		i := c
		for j := range plane {
			plane[j] = float32(src[i]) / (1 << 15)
			i += ch
		}
	}
	return nil
}

// ToInt16 writes b into dst as interleaved 16-bit samples, clamping out-of-range samples.
func ToInt16(b *Buffer, dst []int16) error {
	n, ch := b.Frames(), b.Format.Channels
	if len(dst) != n*ch {
		return fmt.Errorf("audio: toint16: dst holds %d samples, need %d", len(dst), n*ch)
	}
	for c, plane := range b.Data {
		i := c
		for _, s := range plane {
			v := int32(math.Round(float64(s) * (1 << 15)))
			dst[i] = int16(min(max(v, math.MinInt16), math.MaxInt16))
			i += ch
		}
	}
	return nil
}
