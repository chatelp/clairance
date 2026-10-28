package id3

import (
	"encoding/binary"
	"strconv"
	"strings"
)

// The three tag formats, each read here and none of them related to the others beyond sitting in the
// same files.

// readV2 reads an ID3v2 tag from the start of a buffer.
//
// The header gives a version, some flags and a length, and then frames follow until the length runs
// out. The length is syncsafe — seven bits to the byte — so that no length can contain a byte that
// looks like a frame header's sync word, which is the one thoughtful thing about the format.
func readV2(data []byte) (Tags, bool) {
	if len(data) < 10 || string(data[:3]) != "ID3" {
		return Tags{}, false
	}
	major := data[3]
	if major < 2 || major > 4 {
		return Tags{}, false
	}
	flags := data[5]
	size := syncsafe(data[6:10])
	if size <= 0 || 10+size > len(data) {
		size = len(data) - 10
	}
	body := data[10 : 10+size]

	// Unsynchronisation replaces every 0xFF 0x00 pair that could look like a frame sync. Version four
	// applies it per frame instead, and says so in each frame's own flags.
	if flags&0x80 != 0 && major < 4 {
		body = resynchronise(body)
	}
	// An extended header, whose only use here is knowing how far to step over it.
	if flags&0x40 != 0 && len(body) >= 4 {
		var skip int
		if major == 3 {
			skip = 4 + int(binary.BigEndian.Uint32(body))
		} else {
			skip = syncsafe(body[:4])
		}
		if skip > 0 && skip < len(body) {
			body = body[skip:]
		}
	}

	tags := Tags{Version: "ID3v2." + strconv.Itoa(int(major))}
	if major == 2 {
		readFrames(&tags, body, 3, 3, false)
	} else {
		readFrames(&tags, body, 4, 4, major == 4)
	}
	if tags.Frames == nil {
		// A tag that held nothing readable is still a tag, and saying so is what tells a caller the
		// file was tagged rather than bare.
		tags.Frames = map[string][]string{}
	}
	return tags, true
}

// readFrames walks the frames of a tag body.
//
// idLen and sizeLen differ by version: three bytes each in version two, four in the later ones. And
// version four made the frame sizes syncsafe where version three left them plain, which plenty of
// version four files get wrong — so a size that does not fit is retried the other way.
func readFrames(t *Tags, body []byte, idLen, sizeLen int, syncsafeSizes bool) {
	header := idLen + sizeLen
	if sizeLen == 4 {
		header += 2 // two bytes of flags, which version two does not have
	}

	for at := 0; at+header <= len(body); {
		id := strings.TrimRight(string(body[at:at+idLen]), "\x00")
		if id == "" || !plausibleID(id) {
			return // padding, which is what the rest of a tag usually is
		}

		var size int
		switch {
		case sizeLen == 3:
			size = int(body[at+3])<<16 | int(body[at+4])<<8 | int(body[at+5])
		case syncsafeSizes:
			size = syncsafe(body[at+4 : at+8])
			// Written plainly by a writer that did not read the change. A size that overruns the tag
			// but works read the other way is the giveaway.
			if plain := int(binary.BigEndian.Uint32(body[at+4:])); at+header+size > len(body) &&
				at+header+plain <= len(body) {
				size = plain
			}
		default:
			size = int(binary.BigEndian.Uint32(body[at+4:]))
		}

		var frameFlags uint16
		if sizeLen == 4 {
			frameFlags = binary.BigEndian.Uint16(body[at+8:])
		}
		if size < 0 || at+header+size > len(body) {
			return
		}
		payload := body[at+header : at+header+size]
		at += header + size

		// Compressed or encrypted frames are skipped: neither carries anything a name is made of and
		// both need machinery this does not have.
		if frameFlags&0x000C != 0 {
			continue
		}
		if frameFlags&0x0002 != 0 {
			payload = resynchronise(payload)
		}
		// A data length indicator sits in front of the payload when the frame says so.
		if frameFlags&0x0001 != 0 && len(payload) >= 4 {
			payload = payload[4:]
		}

		switch {
		case strings.HasPrefix(id, "T"):
			// A text frame may hold several values separated by nulls, which is how version four
			// spells more than one artist.
			for _, v := range splitText(payload) {
				t.set(id, v)
			}
		case id == "COMM" || id == "COM":
			// A language and a short description come first, both of which a caller wanting the
			// comment does not want.
			t.set(id, comment(payload))
		}
	}
}

// plausibleID reports an identifier made of the letters and digits the format allows. Anything else
// is padding, or the tag has run out and what follows is audio.
func plausibleID(id string) bool {
	for _, c := range id {
		if (c < 'A' || c > 'Z') && (c < '0' || c > '9') {
			return false
		}
	}
	return true
}

// splitText breaks a text frame into its values, keeping the encoding byte with each.
func splitText(payload []byte) []string {
	if len(payload) == 0 {
		return nil
	}
	encoding := payload[0]
	// Only the eight-bit encodings can be split on a single null; a UTF-16 value is full of them.
	if encoding != 0 && encoding != 3 {
		return []string{decodeText(payload)}
	}
	var out []string
	for _, part := range strings.Split(string(payload[1:]), "\x00") {
		if part != "" {
			out = append(out, decodeText(append([]byte{encoding}, part...)))
		}
	}
	return out
}

// comment pulls the text out of a comment frame, past the language and the description.
func comment(payload []byte) string {
	if len(payload) < 4 {
		return ""
	}
	encoding, rest := payload[0], payload[4:] // three bytes of language
	if encoding == 0 || encoding == 3 {
		if _, after, found := strings.Cut(string(rest), "\x00"); found {
			return decodeText(append([]byte{encoding}, after...))
		}
		return ""
	}
	// UTF-16: the description ends at a null pair.
	for i := 0; i+1 < len(rest); i += 2 {
		if rest[i] == 0 && rest[i+1] == 0 {
			return decodeText(append([]byte{encoding}, rest[i+2:]...))
		}
	}
	return ""
}

// resynchronise undoes the escaping that keeps a tag from looking like audio.
func resynchronise(b []byte) []byte {
	out := make([]byte, 0, len(b))
	for i := 0; i < len(b); i++ {
		out = append(out, b[i])
		if b[i] == 0xFF && i+1 < len(b) && b[i+1] == 0 {
			i++
		}
	}
	return out
}

// syncsafe reads the four-byte integer the format uses for lengths, seven bits to the byte.
func syncsafe(b []byte) int {
	if len(b) < 4 {
		return 0
	}
	return int(b[0]&0x7F)<<21 | int(b[1]&0x7F)<<14 | int(b[2]&0x7F)<<7 | int(b[3]&0x7F)
}

// readV1 reads the hundred and twenty-eight bytes at the end of a file.
//
// Fixed-width fields with no encoding and no room: thirty characters for a title, four for a year, a
// genre named by a number out of a list someone wrote in 1994. It is read because plenty of files
// have nothing else.
func readV1(data []byte) (Tags, bool) {
	if len(data) < 128 {
		return Tags{}, false
	}
	tag := data[len(data)-128:]
	if string(tag[:3]) != "TAG" {
		return Tags{}, false
	}

	t := Tags{Version: "ID3v1"}
	t.set("Title", latin1(trimField(tag[3:33])))
	t.set("Artist", latin1(trimField(tag[33:63])))
	t.set("Album", latin1(trimField(tag[63:93])))
	t.set("Year", latin1(trimField(tag[93:97])))

	// Version one and a half took the last two bytes of the comment for a track number, which is why
	// a comment of exactly thirty characters loses its end.
	comment := tag[97:127]
	if comment[28] == 0 && comment[29] != 0 {
		t.set("Track", strconv.Itoa(int(comment[29])))
		comment = comment[:28]
	}
	t.set("Comment", latin1(trimField(comment)))
	if g := int(tag[127]); g < len(v1Genres) {
		t.set("Genre", v1Genres[g])
	}
	return t, true
}

func trimField(b []byte) []byte {
	if i := indexByte(b, 0); i >= 0 {
		b = b[:i]
	}
	return []byte(strings.TrimRight(string(b), " "))
}

func indexByte(b []byte, c byte) int {
	for i, v := range b {
		if v == c {
			return i
		}
	}
	return -1
}

// readAPE reads an APE tag, which some encoders write after the audio.
//
// A footer at the very end names the tag's size and how many items it holds, and each item is a
// length, some flags, a key, a null, and the value.
func readAPE(data []byte) (Tags, bool) {
	if len(data) < 32 || string(data[len(data)-32:len(data)-24]) != "APETAGEX" {
		return Tags{}, false
	}
	footer := data[len(data)-32:]
	size := int(binary.LittleEndian.Uint32(footer[12:]))
	count := int(binary.LittleEndian.Uint32(footer[16:]))
	if size < 32 || size > len(data) || count <= 0 || count > 1024 {
		return Tags{}, false
	}
	body := data[len(data)-size : len(data)-32]

	t := Tags{Version: "APEv2"}
	for at := 0; at+8 < len(body) && count > 0; count-- {
		length := int(binary.LittleEndian.Uint32(body[at:]))
		flags := binary.LittleEndian.Uint32(body[at+4:])
		at += 8
		end := indexByte(body[at:], 0)
		if end < 0 {
			break
		}
		key := string(body[at : at+end])
		at += end + 1
		if length < 0 || at+length > len(body) {
			break
		}
		value := body[at : at+length]
		at += length
		// The item type sits in two bits of the flags; anything but text is a picture or a file.
		if flags&0x06 == 0 {
			t.set(canonicalAPEKey(key), string(value))
		}
	}
	if t.Frames == nil {
		return Tags{}, false
	}
	return t, true
}

// canonicalAPEKey puts an APE key into the spelling the field mapping expects. The format leaves case
// to the writer and writers disagree.
func canonicalAPEKey(key string) string {
	switch strings.ToLower(key) {
	case "title":
		return "Title"
	case "artist":
		return "Artist"
	case "album":
		return "Album"
	case "album artist", "albumartist":
		return "Album Artist"
	case "composer":
		return "Composer"
	case "genre":
		return "Genre"
	case "comment":
		return "Comment"
	case "year", "date":
		return "Year"
	case "track":
		return "Track"
	case "disc":
		return "Disc"
	}
	return key
}

// genreName turns the parenthesised number an older tag uses into the name it stands for, and leaves
// anything else alone.
func genreName(v string) string {
	if !strings.HasPrefix(v, "(") {
		return v
	}
	num, rest, found := strings.Cut(v[1:], ")")
	if !found {
		return v
	}
	if rest != "" {
		return rest
	}
	if n, err := strconv.Atoi(num); err == nil && n >= 0 && n < len(v1Genres) {
		return v1Genres[n]
	}
	return v
}
