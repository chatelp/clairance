package mp3

import (
	"fmt"

	"github.com/yegors/co-atc/third_party/goxiph/internal/bitio"
)

// The bit reservoir is what a frame's audio is read out of.
//
// A frame's own length is fixed by the bitrate, but what it has to say varies: a passage of silence
// needs almost nothing and a transient needs more than its share. So a frame that finishes early
// leaves its remaining bytes behind, and a later frame may begin its main data that far back. The
// side information says how far — up to 511 bytes at MPEG-1, 255 at the lower sampling frequencies.
//
// The consequence for a decoder is that frames cannot be decoded independently. A frame whose
// pointer reaches back further than has been collected cannot be decoded at all, which is what makes
// the first frames after a seek or a truncation unplayable rather than merely wrong.

// reservoirSize is how much history to keep. The largest pointer is 511 bytes and a frame's own main
// data can be longer than that again, so this is generous rather than exact.
const reservoirSize = 2048

// reservoir accumulates main data across frames.
type reservoir struct {
	buf []byte
}

// push appends a frame's main data, keeping the buffer bounded.
func (res *reservoir) push(main []byte) {
	res.buf = append(res.buf, main...)
	if len(res.buf) > reservoirSize {
		res.buf = res.buf[len(res.buf)-reservoirSize:]
	}
}

// ErrReservoir reports a frame whose main data begins further back than has been collected.
//
// Not a fault in the stream: it is what the first frames of a file say when they were written to
// follow frames that are not there, and what every frame says after a seek. A decoder skips them and
// carries on.
var ErrReservoir = fmt.Errorf("%w: main data begins before the stream does", ErrBadFrame)

// open returns a reader over the main data a frame's granules are to be read from.
//
// begin is how many bytes before this frame's main data the granules start. The frame's own main
// data is pushed first, so the reader spans the history and the frame together.
func (res *reservoir) open(main []byte, begin int) (*bitio.MSBReader, error) {
	if begin > len(res.buf) {
		res.push(main)
		return nil, fmt.Errorf("%w: %d bytes back, %d held", ErrReservoir, begin, len(res.buf))
	}
	// Push first and measure from the end afterwards. Pushing can trim the front of the buffer to
	// keep it bounded, and an offset taken before that would point somewhere else entirely.
	res.push(main)
	at := len(res.buf) - begin - len(main)
	if at < 0 {
		return nil, fmt.Errorf("%w: %d bytes back, %d held", ErrReservoir, begin, len(res.buf))
	}
	return bitio.NewMSBReader(res.buf[at:]), nil
}
