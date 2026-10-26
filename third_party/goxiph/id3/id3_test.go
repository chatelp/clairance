package id3

import (
	"bytes"
	"encoding/binary"
	"strings"
	"testing"
)

// v2 builds an ID3v2 tag out of frames, so a test can say what it means rather than spell out bytes
// nobody can read back.
func v2(major byte, frames ...[2]string) []byte {
	var body bytes.Buffer
	for _, f := range frames {
		id, value := f[0], f[1]
		payload := append([]byte{3}, value...) // UTF-8
		body.WriteString(id)
		if major == 2 {
			body.Write([]byte{byte(len(payload) >> 16), byte(len(payload) >> 8), byte(len(payload))})
		} else {
			var size [4]byte
			if major == 4 {
				n := len(payload)
				size = [4]byte{byte(n >> 21 & 0x7F), byte(n >> 14 & 0x7F), byte(n >> 7 & 0x7F), byte(n & 0x7F)}
			} else {
				binary.BigEndian.PutUint32(size[:], uint32(len(payload)))
			}
			body.Write(size[:])
			body.Write([]byte{0, 0})
		}
		body.Write(payload)
	}

	n := body.Len()
	head := []byte{'I', 'D', '3', major, 0, 0,
		byte(n >> 21 & 0x7F), byte(n >> 14 & 0x7F), byte(n >> 7 & 0x7F), byte(n & 0x7F)}
	return append(head, body.Bytes()...)
}

// The three versions differ in how long an identifier is and how a frame's size is written, and a
// reader that gets either wrong walks off into the audio.
func TestReadsEveryVersionOfID3v2(t *testing.T) {
	for _, tc := range []struct {
		major         byte
		title, artist string
	}{
		{2, "TT2", "TP1"},
		{3, "TIT2", "TPE1"},
		{4, "TIT2", "TPE1"},
	} {
		tags, ok := ReadPrefix(v2(tc.major, [2]string{tc.title, "Sea Song"}, [2]string{tc.artist, "Wyatt"}))
		if !ok {
			t.Fatalf("version %d: no tag found", tc.major)
		}
		if tags.Title != "Sea Song" || tags.Artist != "Wyatt" {
			t.Errorf("version %d read %q by %q", tc.major, tags.Title, tags.Artist)
		}
		if want := "ID3v2." + string('0'+tc.major); tags.Version != want {
			t.Errorf("version reported as %q, want %q", tags.Version, want)
		}
	}
}

// A track number may carry a total, and a caller naming files by pattern wants both.
func TestTrackAndDiscPositions(t *testing.T) {
	tags, _ := ReadPrefix(v2(4, [2]string{"TRCK", "3/12"}, [2]string{"TPOS", "1/2"}))
	if tags.Track != 3 || tags.Tracks != 12 {
		t.Errorf("track read as %d of %d, want 3 of 12", tags.Track, tags.Tracks)
	}
	if tags.Disc != 1 || tags.Discs != 2 {
		t.Errorf("disc read as %d of %d, want 1 of 2", tags.Disc, tags.Discs)
	}

	// And one without a total leaves the total alone rather than inventing one.
	bare, _ := ReadPrefix(v2(4, [2]string{"TRCK", "7"}))
	if bare.Track != 7 || bare.Tracks != 0 {
		t.Errorf("a bare track read as %d of %d, want 7 of 0", bare.Track, bare.Tracks)
	}
}

// The year has to come out of whatever the tag called a date. Version four allows a full timestamp
// and files hold all sorts.
func TestYearFromAnyDate(t *testing.T) {
	for _, tc := range []struct {
		date string
		want int
	}{
		{"1974", 1974},
		{"1974-09-07", 1974},
		{"1974-09-07T12:00:00", 1974},
		{"not a date", 0},
		{"", 0},
	} {
		tags, _ := ReadPrefix(v2(4, [2]string{"TDRC", tc.date}))
		if tags.Year != tc.want {
			t.Errorf("%q gave year %d, want %d", tc.date, tags.Year, tc.want)
		}
	}
}

// Four text encodings, and reading one as another is the commonest way to mangle a tag. UTF-16 read
// as Latin-1 comes back with a null between every character, which looks like a short string rather
// than a wrong guess.
func TestTextEncodings(t *testing.T) {
	const want = "Émile"
	for _, tc := range []struct {
		name    string
		payload []byte
	}{
		{"latin-1", append([]byte{0}, 0xC9, 'm', 'i', 'l', 'e')},
		{"utf-8", append([]byte{3}, []byte(want)...)},
		{"utf-16 with a mark", append([]byte{1, 0xFF, 0xFE},
			0xC9, 0, 'm', 0, 'i', 0, 'l', 0, 'e', 0)},
		{"utf-16 big endian", append([]byte{2},
			0, 0xC9, 0, 'm', 0, 'i', 0, 'l', 0, 'e')},
	} {
		if got := decodeText(tc.payload); got != want {
			t.Errorf("%s decoded to %q, want %q", tc.name, got, want)
		}
	}
}

// Version four can hold several values in one frame, separated by nulls.
func TestMultipleValuesInOneFrame(t *testing.T) {
	tags, _ := ReadPrefix(v2(4, [2]string{"TPE1", "Wyatt\x00Oldfield"}))
	if got := tags.Frames["TPE1"]; len(got) != 2 || got[0] != "Wyatt" || got[1] != "Oldfield" {
		t.Errorf("read %q, want two artists", got)
	}
	// The named field takes the first, which is what a file name wants.
	if tags.Artist != "Wyatt" {
		t.Errorf("artist is %q, want the first of them", tags.Artist)
	}
}

// The escaping that keeps a tag from looking like audio has to come out again, or every 0xFF in a
// string gains a null after it.
func TestUnsynchronisationIsUndone(t *testing.T) {
	if got := string(resynchronise([]byte{0xFF, 0x00, 'a', 0xFF, 0x00, 0xFF, 0x00})); got != "\xffa\xff\xff" {
		t.Errorf("resynchronised to %q", got)
	}
	// And a lone 0xFF is left as it is.
	if got := resynchronise([]byte{0xFF, 'a'}); string(got) != "\xffa" {
		t.Errorf("a lone byte became %q", got)
	}
}

// v1 builds the hundred and twenty-eight bytes that go at the end of a file.
func v1(title, artist, album, year, comment string, track, genre byte) []byte {
	pad := func(s string, n int) []byte {
		b := make([]byte, n)
		copy(b, s)
		return b
	}
	tag := []byte("TAG")
	tag = append(tag, pad(title, 30)...)
	tag = append(tag, pad(artist, 30)...)
	tag = append(tag, pad(album, 30)...)
	tag = append(tag, pad(year, 4)...)
	c := pad(comment, 30)
	if track != 0 {
		c[28], c[29] = 0, track
	}
	tag = append(tag, c...)
	return append(tag, genre)
}

// The old tag is fixed-width and has no encoding, and its track number lives in the last two bytes of
// the comment — which is why a comment of exactly thirty characters loses its end.
func TestReadsID3v1(t *testing.T) {
	data := append(make([]byte, 4096), v1("Sea Song", "Wyatt", "Rock Bottom", "1974", "hello", 3, 17)...)
	tags, err := Read(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if tags.Version != "ID3v1" {
		t.Errorf("version is %q", tags.Version)
	}
	if tags.Title != "Sea Song" || tags.Artist != "Wyatt" || tags.Album != "Rock Bottom" {
		t.Errorf("read %+v", tags)
	}
	if tags.Year != 1974 || tags.Track != 3 {
		t.Errorf("year %d track %d, want 1974 and 3", tags.Year, tags.Track)
	}
	if tags.Genre != "Rock" {
		t.Errorf("genre is %q, want Rock", tags.Genre)
	}
	if tags.Comment != "hello" {
		t.Errorf("comment is %q", tags.Comment)
	}
}

// A file with both tags takes the richer one and fills the gaps from the other, rather than letting
// whichever is read last win.
func TestTheRicherTagWins(t *testing.T) {
	head := v2(4, [2]string{"TIT2", "Sea Song"})
	data := append(head, make([]byte, 2048)...)
	data = append(data, v1("Wrong Title", "Wyatt", "", "1974", "", 0, 255)...)

	tags, err := Read(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if tags.Title != "Sea Song" {
		t.Errorf("title is %q; the newer tag should win", tags.Title)
	}
	if tags.Artist != "Wyatt" {
		t.Errorf("artist is %q; the older tag should fill the gap", tags.Artist)
	}
	if tags.Version != "ID3v2.4" {
		t.Errorf("version is %q", tags.Version)
	}
}

// An older tag names a genre by a number in brackets, and a caller wants the name.
func TestGenreNumbersBecomeNames(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"(17)", "Rock"},
		{"(17)Hard Rock", "Hard Rock"},
		{"Rock", "Rock"},
		{"(999)", "(999)"},
		{"(", "("},
	} {
		if got := genreName(tc.in); got != tc.want {
			t.Errorf("%q became %q, want %q", tc.in, got, tc.want)
		}
	}
}

// A file with no tag at all is not an error. Most of the world's audio has nothing on it.
func TestUntaggedFileIsNotAnError(t *testing.T) {
	tags, err := Read(strings.NewReader(strings.Repeat("\x00", 1000)))
	if err != nil {
		t.Fatal(err)
	}
	if !tags.Empty() {
		t.Errorf("an untagged file reported %+v", tags)
	}
	if tags.Get("TIT2") != "" {
		t.Error("an untagged file returned a title")
	}
}

// Whatever a tag holds is reachable by its own name, because no fixed set of fields covers what
// people put in these.
func TestEveryFrameIsReachable(t *testing.T) {
	tags, _ := ReadPrefix(v2(4, [2]string{"TENC", "LAME"}, [2]string{"TSSE", "-V0"}))
	if got := tags.Get("TENC"); got != "LAME" {
		t.Errorf("TENC is %q", got)
	}
	if got := tags.Get("TSSE"); got != "-V0" {
		t.Errorf("TSSE is %q", got)
	}
}
