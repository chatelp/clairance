// Package audio defines the PCM representation shared by every container and codec in goxiph.
package audio

import (
	"errors"
	"fmt"
)

// ErrInvalidFormat reports a Format that no codec can accept.
var ErrInvalidFormat = errors.New("audio: invalid format")

// Format describes an uncompressed stream. Bit depth is not part of it: samples are float32
// everywhere inside the library, and depth is a property of the source or sink only.
type Format struct {
	SampleRate int
	Channels   int
}

// Validate rejects formats that cannot describe a real stream.
func (f Format) Validate() error {
	if f.SampleRate <= 0 {
		return fmt.Errorf("%w: sample rate %d", ErrInvalidFormat, f.SampleRate)
	}
	if f.Channels <= 0 {
		return fmt.Errorf("%w: channel count %d", ErrInvalidFormat, f.Channels)
	}
	return nil
}

func (f Format) String() string {
	return fmt.Sprintf("%d Hz, %d ch", f.SampleRate, f.Channels)
}
