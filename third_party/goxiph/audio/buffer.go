package audio

import "fmt"

// Buffer holds planar PCM normalised to [-1, 1]. Planar because every transform codec works one
// channel at a time; interleaving is a boundary concern handled in convert.go.
type Buffer struct {
	Format Format
	Data   [][]float32 // Data[channel][frame]
}

// NewBuffer allocates a buffer of frames per channel.
func NewBuffer(f Format, frames int) (*Buffer, error) {
	if err := f.Validate(); err != nil {
		return nil, err
	}
	if frames < 0 {
		return nil, fmt.Errorf("%w: frame count %d", ErrInvalidFormat, frames)
	}
	b := &Buffer{Format: f, Data: make([][]float32, f.Channels)}
	// One backing array; codecs iterate per channel but allocate per stream.
	flat := make([]float32, f.Channels*frames)
	for ch := range b.Data {
		b.Data[ch] = flat[ch*frames : (ch+1)*frames : (ch+1)*frames]
	}
	return b, nil
}

// Frames reports the length of one channel, or 0 for an empty buffer.
func (b *Buffer) Frames() int {
	if len(b.Data) == 0 {
		return 0
	}
	return len(b.Data[0])
}

// Validate checks that Data matches Format and that channels are equal length.
func (b *Buffer) Validate() error {
	if err := b.Format.Validate(); err != nil {
		return err
	}
	if len(b.Data) != b.Format.Channels {
		return fmt.Errorf("%w: %d planes for %d channels", ErrInvalidFormat, len(b.Data), b.Format.Channels)
	}
	n := b.Frames()
	for ch, plane := range b.Data {
		if len(plane) != n {
			return fmt.Errorf("%w: channel %d has %d frames, channel 0 has %d", ErrInvalidFormat, ch, len(plane), n)
		}
	}
	return nil
}

// Resize grows or shrinks every channel to frames, reusing capacity where it exists, and gives the
// buffer one plane per channel of its format.
//
// The planes are brought into line with the format rather than left to the caller. A buffer that has
// never been used has none at all, and one carried over from another stream may have the wrong
// count; both are ordinary, since a decoder fills whatever it is handed.
func (b *Buffer) Resize(frames int) {
	if len(b.Data) != b.Format.Channels {
		planes := make([][]float32, b.Format.Channels)
		copy(planes, b.Data)
		b.Data = planes
	}
	for ch, plane := range b.Data {
		if cap(plane) >= frames {
			b.Data[ch] = plane[:frames]
			continue
		}
		// Twice what is there, at least. A caller accumulating a stream block by block resizes once
		// per block, and growing to exactly what is asked makes that quadratic: every block copies
		// everything decoded so far.
		grown := make([]float32, frames, max(frames, 2*cap(plane)))
		copy(grown, plane)
		b.Data[ch] = grown
	}
}

// Zero clears every sample without reallocating.
func (b *Buffer) Zero() {
	for _, plane := range b.Data {
		clear(plane)
	}
}
