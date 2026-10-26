// Package vorbiscomment implements the Vorbis comment metadata block.
//
// The format is shared unchanged by Vorbis, Opus and FLAC, so it lives outside all three.
package vorbiscomment

import (
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
)

// ErrMalformed reports a comment block that cannot be parsed.
var ErrMalformed = errors.New("vorbiscomment: malformed")

// maxField bounds a single length-prefixed field. The format allows 32-bit lengths, so an
// untrusted stream can otherwise ask for a 4 GiB allocation before the data proves it is there.
const maxField = 1 << 24

// Tags is a parsed comment block.
//
// Comments are held as raw "NAME=value" strings because the format permits repeated names and
// preserves their order, neither of which a map would survive.
type Tags struct {
	Vendor   string
	Comments []string
}

// Get returns the values whose field name matches name, compared without case per the
// specification. Field names are ASCII, so simple case folding is correct here.
func (t Tags) Get(name string) []string {
	var out []string
	for _, c := range t.Comments {
		k, v, ok := strings.Cut(c, "=")
		if ok && strings.EqualFold(k, name) {
			out = append(out, v)
		}
	}
	return out
}

// First returns the first value for name, or "" when absent.
func (t Tags) First(name string) string {
	if v := t.Get(name); len(v) > 0 {
		return v[0]
	}
	return ""
}

// Add appends a name/value pair.
func (t *Tags) Add(name, value string) {
	t.Comments = append(t.Comments, name+"="+value)
}

// Unmarshal parses a comment block. framing reports whether a trailing framing bit is expected,
// which Vorbis requires and FLAC and Opus do not.
//
// The returned strings copy their input; the caller may reuse data.
func Unmarshal(data []byte, framing bool) (Tags, error) {
	var t Tags
	p := data

	vendorLen, p, err := readLen(p, "vendor length")
	if err != nil {
		return t, err
	}
	if len(p) < int(vendorLen) {
		return t, fmt.Errorf("%w: vendor string wants %d bytes, %d left", ErrMalformed, vendorLen, len(p))
	}
	t.Vendor = string(p[:vendorLen])
	p = p[vendorLen:]

	count, p, err := readLen(p, "comment count")
	if err != nil {
		return t, err
	}
	// Each comment costs at least its own 4-byte length, so a count larger than the remaining
	// bytes allows is malformed and must not drive an allocation.
	if int(count) > len(p)/4 {
		return t, fmt.Errorf("%w: %d comments will not fit in %d bytes", ErrMalformed, count, len(p))
	}

	t.Comments = make([]string, 0, count)
	for i := range count {
		var n uint32
		n, p, err = readLen(p, "comment length")
		if err != nil {
			return t, fmt.Errorf("comment %d: %w", i, err)
		}
		if len(p) < int(n) {
			return t, fmt.Errorf("%w: comment %d wants %d bytes, %d left", ErrMalformed, i, n, len(p))
		}
		t.Comments = append(t.Comments, string(p[:n]))
		p = p[n:]
	}

	if framing {
		if len(p) < 1 {
			return t, fmt.Errorf("%w: missing framing bit", ErrMalformed)
		}
		if p[0]&1 == 0 {
			return t, fmt.Errorf("%w: framing bit clear", ErrMalformed)
		}
	}
	return t, nil
}

// AppendTo appends the encoded block to dst.
func (t Tags) AppendTo(dst []byte, framing bool) []byte {
	dst = binary.LittleEndian.AppendUint32(dst, uint32(len(t.Vendor)))
	dst = append(dst, t.Vendor...)
	dst = binary.LittleEndian.AppendUint32(dst, uint32(len(t.Comments)))
	for _, c := range t.Comments {
		dst = binary.LittleEndian.AppendUint32(dst, uint32(len(c)))
		dst = append(dst, c...)
	}
	if framing {
		dst = append(dst, 1)
	}
	return dst
}

// readLen consumes a 32-bit little-endian length.
func readLen(p []byte, what string) (uint32, []byte, error) {
	if len(p) < 4 {
		return 0, p, fmt.Errorf("%w: truncated %s", ErrMalformed, what)
	}
	n := binary.LittleEndian.Uint32(p)
	if n > maxField {
		return 0, p, fmt.Errorf("%w: %s of %d bytes exceeds the %d-byte limit", ErrMalformed, what, n, maxField)
	}
	return n, p[4:], nil
}
