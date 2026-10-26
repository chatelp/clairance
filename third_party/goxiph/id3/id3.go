// Package id3 reads the tags wrapped around an MP3 file.
//
// Not part of any codec and not part of MP3 either. A tag is a block of text bolted onto the front or
// the back of a stream, and a decoder's only interest in one is skipping it — which the mp3 package
// does without this. This is for the other use: knowing what a file holds without decoding it.
//
// Three formats appear in the wild and all three are read here. ID3v2 sits in front of the audio and
// is the one that matters; ID3v1 is a hundred and twenty-eight bytes at the end, from before there
// was anything better; APE is a third that some encoders write. A file may carry more than one, and
// where they disagree the richer one wins.
//
// Reading only. Writing tags is a different job with different hazards, and nothing here needs it.
package id3

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

// ErrMalformed reports a tag that does not parse. A file with a broken tag still holds audio, so
// callers that only want to play it can carry on.
var ErrMalformed = errors.New("id3: malformed tag")

// Tags is what a file says about itself.
//
// The named fields are the ones a listener would recognise and the ones a caller naming files by
// pattern reaches for. Everything the tag held is in Frames as well, under the identifier the format
// gave it, because no fixed set of fields covers what people put in these.
type Tags struct {
	Title       string
	Artist      string
	Album       string
	AlbumArtist string
	Composer    string
	Genre       string
	Comment     string

	// Year is the recording year, or zero. Date holds whatever the tag said, which may be a full date
	// or a year or something else entirely.
	Year int
	Date string

	// Track and Disc are the position, and Tracks and Discs the total where the tag gave one. A tag
	// that says "3/12" fills both; one that says "3" fills only the first.
	Track, Tracks int
	Disc, Discs   int

	// Version names the tag this came from: "ID3v2.4", "ID3v1", "APEv2" and so on. Empty when a file
	// carried no tag at all.
	Version string

	// Frames is every text field the tag held, keyed by its identifier — "TIT2" and the like for
	// ID3v2, or the key itself for APE. Repeated fields keep their order.
	Frames map[string][]string
}

// Get returns the first value of a frame, or the empty string.
func (t Tags) Get(name string) string {
	if v := t.Frames[name]; len(v) > 0 {
		return v[0]
	}
	return ""
}

// Empty reports a file that said nothing about itself.
func (t Tags) Empty() bool { return t.Version == "" }

// maxTagSize is how large a tag may be before it is refused.
//
// Generous, because cover art is routinely a megabyte and occasionally much more, and mean enough
// that a corrupt length field cannot ask for the whole address space.
const maxTagSize = 64 << 20

// Read reads whatever tags a stream carries.
//
// The whole stream, because ID3v1 and APE sit at the end of it and there is no other way to reach
// them. A caller that only wants the leading tag and does not want to read a hundred megabytes to get
// it should use ReadPrefix.
func Read(r io.Reader) (Tags, error) {
	data, err := io.ReadAll(io.LimitReader(r, maxTagSize+1))
	if err != nil {
		return Tags{}, err
	}
	if len(data) > maxTagSize {
		return Tags{}, fmt.Errorf("%w: the stream is longer than %d bytes", ErrMalformed, maxTagSize)
	}

	// The leading tag first, then the trailing ones, so that a richer tag at the front is not lost to
	// a sparser one at the back.
	tags, _ := ReadPrefix(data)
	for _, trailing := range []func([]byte) (Tags, bool){readAPE, readV1} {
		if t, ok := trailing(data); ok {
			tags = tags.Merge(t)
		}
	}
	return tags, nil
}

// ReadPrefix reads the ID3v2 tag at the start of a buffer, and reports whether there was one.
//
// For a caller holding the first part of a file and no more, which is the ordinary case when the
// point is to name the file rather than play it.
func ReadPrefix(data []byte) (Tags, bool) {
	return readV2(data)
}

// Merge fills in whatever this tag does not say from another, poorer one.
func (t Tags) Merge(o Tags) Tags {
	if t.Version == "" {
		return o
	}
	for _, pair := range []struct {
		dst *string
		src string
	}{
		{&t.Title, o.Title}, {&t.Artist, o.Artist}, {&t.Album, o.Album},
		{&t.AlbumArtist, o.AlbumArtist}, {&t.Composer, o.Composer},
		{&t.Genre, o.Genre}, {&t.Comment, o.Comment}, {&t.Date, o.Date},
	} {
		if *pair.dst == "" {
			*pair.dst = pair.src
		}
	}
	if t.Year == 0 {
		t.Year = o.Year
	}
	if t.Track == 0 {
		t.Track, t.Tracks = o.Track, o.Tracks
	}
	if t.Disc == 0 {
		t.Disc, t.Discs = o.Disc, o.Discs
	}
	for k, v := range o.Frames {
		if _, seen := t.Frames[k]; !seen {
			if t.Frames == nil {
				t.Frames = map[string][]string{}
			}
			t.Frames[k] = v
		}
	}
	return t
}

// set files a frame under its identifier and into whichever named field it belongs to.
func (t *Tags) set(id, value string) {
	value = strings.TrimRight(value, "\x00")
	if value == "" {
		return
	}
	if t.Frames == nil {
		t.Frames = map[string][]string{}
	}
	t.Frames[id] = append(t.Frames[id], value)

	switch id {
	case "TIT2", "TT2", "Title":
		assign(&t.Title, value)
	case "TPE1", "TP1", "Artist":
		assign(&t.Artist, value)
	case "TALB", "TAL", "Album":
		assign(&t.Album, value)
	case "TPE2", "TP2", "Album Artist":
		assign(&t.AlbumArtist, value)
	case "TCOM", "TCM", "Composer":
		assign(&t.Composer, value)
	case "TCON", "TCO", "Genre":
		assign(&t.Genre, genreName(value))
	case "COMM", "COM", "Comment":
		assign(&t.Comment, value)
	case "TDRC", "TYER", "TYE", "TDAT", "Year":
		if t.Date == "" {
			t.Date = value
		}
		if t.Year == 0 {
			t.Year = leadingYear(value)
		}
	case "TRCK", "TRK", "Track":
		if t.Track == 0 {
			t.Track, t.Tracks = position(value)
		}
	case "TPOS", "TPA", "Disc":
		if t.Disc == 0 {
			t.Disc, t.Discs = position(value)
		}
	}
}

func assign(dst *string, v string) {
	if *dst == "" {
		*dst = v
	}
}

// position reads a "3" or a "3/12".
func position(v string) (at, total int) {
	num, den, _ := strings.Cut(v, "/")
	at, _ = strconv.Atoi(strings.TrimSpace(num))
	total, _ = strconv.Atoi(strings.TrimSpace(den))
	return at, total
}

// leadingYear reads the year out of whatever the tag called a date. ID3v2.4 allows a full timestamp
// and plenty of files hold something else again.
func leadingYear(v string) int {
	for i := 0; i+4 <= len(v); i++ {
		if n, err := strconv.Atoi(v[i : i+4]); err == nil && n > 1000 && n < 3000 {
			return n
		}
	}
	return 0
}

// decodeText turns a frame's bytes into a string, in whichever of the four encodings the tag names.
//
// The first byte says which. Nothing else in the format is as often got wrong: a tag written in
// UTF-16 and read as Latin-1 comes out as every other character being a null, which looks like a
// truncated string rather than a wrong encoding.
func decodeText(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	encoding, body := b[0], b[1:]
	switch encoding {
	case 0: // Latin-1
		return latin1(body)
	case 1: // UTF-16 with a byte order mark
		return utf16String(body, true)
	case 2: // UTF-16 big endian, no mark
		return utf16String(body, false)
	case 3: // UTF-8
		if utf8.Valid(body) {
			return strings.TrimRight(string(body), "\x00")
		}
		return latin1(body)
	}
	// An encoding this format does not have. Whatever it is, it is not a length, so the whole frame
	// is text of some kind.
	return latin1(b)
}

func latin1(b []byte) string {
	var out strings.Builder
	for _, c := range b {
		if c == 0 {
			continue
		}
		out.WriteRune(rune(c))
	}
	return out.String()
}

// utf16String decodes UTF-16, taking the byte order from a mark where there is one.
func utf16String(b []byte, mark bool) string {
	big := true
	if mark && len(b) >= 2 {
		switch {
		case b[0] == 0xFF && b[1] == 0xFE:
			big, b = false, b[2:]
		case b[0] == 0xFE && b[1] == 0xFF:
			big, b = true, b[2:]
		}
	}
	units := make([]uint16, 0, len(b)/2)
	for i := 0; i+1 < len(b); i += 2 {
		if big {
			units = append(units, binary.BigEndian.Uint16(b[i:]))
		} else {
			units = append(units, binary.LittleEndian.Uint16(b[i:]))
		}
	}
	return strings.TrimRight(string(utf16.Decode(units)), "\x00")
}
