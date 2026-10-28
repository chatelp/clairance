package mp3

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// Finding frames is most of the work of reading an MP3 file. There is no container: a stream is
// frames laid end to end, with tags of three different formats permitted before and after them, and
// nothing that says where the audio starts. A decoder syncs by looking for a frame header and
// checking that the next one lands where this one says it will.

// ErrNoSync reports that no frame header was found where one was looked for.
var ErrNoSync = errors.New("mp3: no frame found")

// scanLimit is how many bytes to search for a sync word before giving up.
//
// Generous, because a file can open with an ID3v2 tag holding cover art of several hundred kilobytes
// and the tag's own length field is the thing being distrusted.
const scanLimit = 1 << 20

// Frame is one frame of a stream: its header and the bytes after it.
type Frame struct {
	Header Header
	// Data is the frame's body, the header and any checksum removed.
	Data []byte
	// Offset is where the frame began in the stream.
	Offset int64
}

// Scanner walks the frames of an MP3 stream.
//
// It syncs on the first header whose successor lands where it should, which is what tells a frame
// from four bytes of audio or of cover art that happen to look like one. Tags are skipped: ID3v2 at
// the front by its own length, ID3v1 and APE at the back by never being synced to.
type Scanner struct {
	buf  []byte
	at   int
	base int64

	synced bool
	first  Header
	// skipped is how many bytes were passed over to find the frames, tags included, and trailer what
	// was cut from the end.
	skipped int64
	trailer []byte
}

// NewScanner reads the whole stream and prepares to walk it.
//
// Whole, because MP3 needs to look ahead to sync and to look back for the bit reservoir, and a file
// is small enough that the alternative buys nothing but complexity.
func NewScanner(r io.Reader) (*Scanner, error) {
	buf, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	trimmed := trimTrailers(buf)
	s := &Scanner{buf: trimmed, trailer: buf[len(trimmed):]}
	s.at = skipTags(s.buf)
	s.skipped = int64(s.at) + int64(len(buf)-len(s.buf))
	return s, nil
}

// trimTrailers cuts the tags that sit after the audio rather than before it.
//
// They have to go before the scan and not during it. A frame is believed when the next one lands
// where it says, and the last frame of a tagged file is followed by "TAG" or "APETAGEX" instead —
// so with the trailer still attached the final frame never corroborates, and a file holding one
// frame and a tag will not sync at all.
func trimTrailers(buf []byte) []byte {
	for {
		switch {
		case len(buf) >= 128 && string(buf[len(buf)-128:len(buf)-125]) == "TAG":
			buf = buf[:len(buf)-128]
		case len(buf) >= 32 && string(buf[len(buf)-32:len(buf)-24]) == "APETAGEX":
			// The footer holds the tag's size without itself, so both come off together.
			size := int(binary.LittleEndian.Uint32(buf[len(buf)-20:]))
			if size <= 0 || size+32 > len(buf) {
				return buf
			}
			buf = buf[:len(buf)-size-32]
		default:
			return buf
		}
	}
}

// skipTags returns the offset past any ID3v2 tag at the start of a stream.
//
// ID3v2 declares its own length in a syncsafe integer — seven bits per byte, so that no length can
// contain a byte that looks like a sync word. Trusting it is right when it is right and harmless
// when it is not, since the scan that follows will resync anyway.
func skipTags(buf []byte) int {
	at := 0
	for {
		if len(buf) < at+10 || string(buf[at:at+3]) != "ID3" {
			return at
		}
		size := syncsafe(buf[at+6 : at+10])
		next := at + 10 + size
		// A footer, present when the tag says so, adds ten more.
		if buf[at+5]&0x10 != 0 {
			next += 10
		}
		if next <= at || next > len(buf) {
			return at
		}
		at = next
	}
}

// syncsafe reads the four-byte integer ID3v2 uses for lengths, seven bits to the byte.
func syncsafe(b []byte) int {
	return int(b[0]&0x7F)<<21 | int(b[1]&0x7F)<<14 | int(b[2]&0x7F)<<7 | int(b[3]&0x7F)
}

// Next returns the next frame, or io.EOF at the end of the stream.
func (s *Scanner) Next() (Frame, error) {
	// Too little left to hold a header, so the stream is over rather than broken. What remains is
	// whatever a writer left after the last frame.
	if s.at+HeaderSize > len(s.buf) {
		s.skipped += int64(len(s.buf) - s.at)
		s.at = len(s.buf)
		return Frame{}, io.EOF
	}
	h, at, err := s.sync()
	if err != nil {
		return Frame{}, err
	}

	end := at + h.FrameSize()
	if end > len(s.buf) {
		// A last frame cut short by the end of the file. Nothing can be decoded from it.
		s.at = len(s.buf)
		return Frame{}, io.EOF
	}
	body := at + HeaderSize + h.CRCSize()
	s.at = end
	return Frame{Header: h, Data: s.buf[body:end], Offset: s.base + int64(at)}, nil
}

// sync finds the next frame header at or after the cursor.
//
// The first one has to be corroborated: a header is believed only when the frame it describes ends
// on another header of the same stream. After that the stream is trusted, because a file that has
// synced once and then fails is corrupt rather than misaligned, and resyncing on every frame would
// hide that.
func (s *Scanner) sync() (Header, int, error) {
	limit := min(s.at+scanLimit, len(s.buf))
	for at := s.at; at+HeaderSize <= len(s.buf); at++ {
		if at >= limit {
			break
		}
		h, err := ParseHeader(s.buf[at:])
		if err != nil {
			if errors.Is(err, ErrFreeFormat) && s.synced {
				return h, 0, err
			}
			continue
		}
		if s.synced {
			if !s.first.sameStream(h) {
				continue
			}
			s.skipped += int64(at - s.at)
			return h, at, nil
		}
		if !s.corroborated(h, at) {
			continue
		}
		s.synced, s.first = true, h
		s.skipped += int64(at - s.at)
		return h, at, nil
	}
	return Header{}, 0, fmt.Errorf("%w: searched %d bytes from %d", ErrNoSync, limit-s.at, s.at)
}

// corroborated reports whether a candidate header is followed by another of the same stream where it
// says it should be. A stream of one frame is taken at its word, since there is nothing to check it
// against.
func (s *Scanner) corroborated(h Header, at int) bool {
	next := at + h.FrameSize()
	if next+HeaderSize > len(s.buf) {
		return next <= len(s.buf)
	}
	o, err := ParseHeader(s.buf[next:])
	return err == nil && h.sameStream(o)
}

// Prefix is the bytes before the first frame, which is where a leading tag sits. Empty when the
// stream begins with audio.
func (s *Scanner) Prefix() []byte {
	at := skipTags(s.buf)
	return s.buf[:at]
}

// Trailer is the bytes cut from the end of the stream, which is where the older tags sit.
func (s *Scanner) Trailer() []byte { return s.trailer }

// Remaining is how many bytes of the stream are still ahead of the cursor.
func (s *Scanner) Remaining() int64 { return int64(len(s.buf) - s.at) }

// Skipped is how many bytes were passed over rather than decoded: tags, padding between frames, and
// anything else that was not a frame.
func (s *Scanner) Skipped() int64 { return s.skipped }
