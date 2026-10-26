package mp3

// Added in Clairance's copy of goxiph (see ../README.md): a live stream. The
// Scanner reads its whole input before the first frame, which is right for a
// file and never returns on a radio stream that has no end. FrameReader reads
// one frame at a time instead, and NewLiveDecoder decodes from it with the
// same Decoder, unchanged.

import (
	"bufio"
	"errors"
	"fmt"
	"io"

	"github.com/yegors/co-atc/third_party/goxiph/audio"
)

// ErrFormatChanged reports a stream whose frames stop matching the first one
// (another rate, layer or channel count) for longer than a corrupt patch
// would: the stream itself has changed and a decoder must start over.
var ErrFormatChanged = errors.New("mp3: stream format changed")

// liveScanLimit is how many bytes FrameReader searches for a frame before
// giving up, at the start and after losing sync. A live stream that has not
// shown a frame in this much is not an MP3 stream, or not one any more.
const liveScanLimit = 64 << 10

// FrameReader walks the frames of a live MP3 stream as they arrive.
//
// It syncs as the Scanner does, on a header whose successor lands where it
// should. Unlike the Scanner it resyncs after a frame that does not parse: a
// radio stream joined mid-flight starts inside a frame, and one carried over a
// network can lose bytes; skipping to the next corroborated frame costs a
// frame or two of sound, where giving up would cost a reconnection.
type FrameReader struct {
	r      *bufio.Reader
	synced bool
	first  Header
	have   bool

	// Skipped is how many bytes were passed over to find frames, and Resyncs
	// how many times sync was lost after the first.
	Skipped int64
	Resyncs int
}

// NewFrameReader reads frames from r.
func NewFrameReader(r io.Reader) *FrameReader {
	return &FrameReader{r: bufio.NewReaderSize(r, 16<<10)}
}

// Header is the first frame's header, syncing on it if that has not happened.
func (f *FrameReader) Header() (Header, error) {
	if !f.have {
		if err := f.sync(); err != nil {
			return Header{}, err
		}
	}
	return f.first, nil
}

// Next returns the next frame, or the reader's error: io.EOF at a clean end,
// io.ErrUnexpectedEOF inside a frame, ErrNoSync when no frame shows within
// liveScanLimit bytes, ErrFormatChanged when frames of another kind follow.
func (f *FrameReader) Next() (Frame, error) {
	for {
		if !f.synced {
			if err := f.sync(); err != nil {
				return Frame{}, err
			}
		}
		hb, err := f.r.Peek(HeaderSize)
		if err != nil {
			return Frame{}, eof(err, len(hb) > 0)
		}
		h, err := ParseHeader(hb)
		if err != nil || h.FrameSize() < HeaderSize+h.CRCSize() || !f.first.sameStream(h) {
			// Lost: skip a byte and look for the next corroborated frame.
			f.synced = false
			f.Resyncs++
			if _, err := f.r.Discard(1); err != nil {
				return Frame{}, eof(err, false)
			}
			f.Skipped++
			continue
		}
		buf := make([]byte, h.FrameSize())
		if n, err := io.ReadFull(f.r, buf); err != nil {
			return Frame{}, eof(err, n > 0)
		}
		return Frame{Header: h, Data: buf[HeaderSize+h.CRCSize():]}, nil
	}
}

// sync finds a header whose frame ends on another of the same stream. After
// the first sync, a frame of another kind is not taken: a run of them means
// the stream changed.
func (f *FrameReader) sync() error {
	other := 0
	for searched := 0; searched < liveScanLimit; searched++ {
		hb, err := f.r.Peek(HeaderSize)
		if err != nil {
			return eof(err, false)
		}
		if h, err := ParseHeader(hb); err == nil && h.FrameSize() >= HeaderSize+h.CRCSize() {
			n := h.FrameSize()
			nb, err := f.r.Peek(n + HeaderSize)
			if err != nil && !errors.Is(err, bufio.ErrBufferFull) {
				return eof(err, true)
			}
			if len(nb) >= n+HeaderSize {
				if o, err := ParseHeader(nb[n:]); err == nil && h.sameStream(o) {
					if !f.have {
						f.first, f.have, f.synced = h, true, true
						return nil
					}
					if f.first.sameStream(h) {
						f.synced = true
						return nil
					}
					// Two frames of another kind in a row: count them, and
					// after a few the stream has changed.
					if other++; other >= 3 {
						return fmt.Errorf("%w: %d Hz, now %d Hz", ErrFormatChanged, f.first.SampleRate, h.SampleRate)
					}
				}
			}
		}
		if _, err := f.r.Discard(1); err != nil {
			return eof(err, false)
		}
		f.Skipped++
	}
	return fmt.Errorf("%w: searched %d bytes", ErrNoSync, liveScanLimit)
}

// eof says io.ErrUnexpectedEOF when the stream ended inside a frame.
func eof(err error, inside bool) error {
	if errors.Is(err, io.EOF) && inside {
		return io.ErrUnexpectedEOF
	}
	return err
}

// NewLiveDecoder decodes a live stream frame by frame. There is no tag to read
// on a live stream, so nothing is trimmed, and Duration, Comments and Tags
// are empty.
func NewLiveDecoder(r io.Reader) (*Decoder, *FrameReader, error) {
	fr := NewFrameReader(r)
	h, err := fr.Header()
	if err != nil {
		return nil, fr, err
	}
	d := &Decoder{live: fr, channels: h.Channels(), rate: h.SampleRate}
	if d.scratch, err = audio.NewBuffer(d.Format(), 0); err != nil {
		return nil, fr, err
	}
	return d, fr, nil
}

// nextFrame is the Decoder's source of frames: the live reader when there is
// one, the whole-file stream otherwise.
func (d *Decoder) nextFrame() (Frame, error) {
	if d.live != nil {
		return d.live.Next()
	}
	return d.stream.Next()
}
