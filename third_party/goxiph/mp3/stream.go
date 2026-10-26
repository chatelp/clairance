package mp3

import (
	"bytes"
	"errors"
	"io"
	"time"

	"github.com/yegors/co-atc/third_party/goxiph/id3"
)

// Stream is an MP3 file: its frames, and what the leading tag says about them.
//
// The tag frame is consumed here rather than handed on. It holds no audio, and a caller that decoded
// it would get a frame of silence at the head of every file written by any common encoder.
type Stream struct {
	scan *Scanner
	tag  VBRTag
	// first is the first audio frame, read to learn the stream's shape and kept for the caller.
	first Frame
	// pending says the first frame has not been handed over yet.
	pending bool
	haveTag bool
}

// NewStream reads far enough into an MP3 file to know what it is.
func NewStream(r io.Reader) (*Stream, error) {
	scan, err := NewScanner(r)
	if err != nil {
		return nil, err
	}
	s := &Stream{scan: scan}

	f, err := scan.Next()
	if err != nil {
		return nil, err
	}
	if tag, ok := ParseVBRTag(f.Header, f.Data); ok {
		s.tag, s.haveTag = tag, true
		next, err := scan.Next()
		if err != nil {
			// A file holding a tag and no audio. Legal, and something a player must not crash on.
			// The tag frame's own header still describes the stream the tag is describing, so it is
			// kept for that and not handed on as audio.
			if errors.Is(err, io.EOF) {
				s.first = f
				return s, nil
			}
			return nil, err
		}
		f = next
	}
	s.first, s.pending = f, true
	return s, nil
}

// Header is the first audio frame's header, which describes the stream as a whole.
func (s *Stream) Header() Header { return s.first.Header }

// Tag is the leading VBR tag, and whether there was one.
func (s *Stream) Tag() (VBRTag, bool) { return s.tag, s.haveTag }

// Tags are what the file says about itself, from whatever ID3 or APE tag wraps the audio.
//
// Read here rather than by a caller opening the file again, because the scanner has already found
// where the audio starts and ends and therefore where the tags are.
func (s *Stream) Tags() id3.Tags {
	prefix, trailer := s.scan.Prefix(), s.scan.Trailer()
	tags, _ := id3.ReadPrefix(prefix)
	if t, err := id3.Read(bytes.NewReader(trailer)); err == nil && !t.Empty() {
		tags = tags.Merge(t)
	}
	return tags
}

// Next returns the next audio frame, or io.EOF at the end.
func (s *Stream) Next() (Frame, error) {
	if s.pending {
		s.pending = false
		return s.first, nil
	}
	return s.scan.Next()
}

// Duration is how long the stream plays.
//
// From the tag where there is one, which is exact and costs nothing. Otherwise from the bitrate and
// what is left of the file, which is right for a constant bitrate stream and an estimate for a
// variable one — the alternative is reading every frame header in the file, and a caller wanting a
// progress bar wants an answer now.
func (s *Stream) Duration() time.Duration {
	rate := s.first.Header.SampleRate
	if rate <= 0 {
		return 0
	}
	if n := s.tag.Samples(s.first.Header.Samples()); n > 0 {
		return time.Duration(n) * time.Second / time.Duration(rate)
	}
	return s.estimate()
}

// Exact reports whether Duration is the stream's own figure rather than an estimate from its
// bitrate. A caller drawing a progress bar wants to know which it has.
func (s *Stream) Exact() bool {
	return s.tag.Samples(s.first.Header.Samples()) > 0
}

// estimate is the length from the bitrate and the bytes remaining.
func (s *Stream) estimate() time.Duration {
	h := s.first.Header
	if h.Bitrate <= 0 {
		return 0
	}
	bytes := s.scan.Remaining() + int64(h.FrameSize())
	return time.Duration(bytes) * 8 * time.Second / time.Duration(h.Bitrate)
}
