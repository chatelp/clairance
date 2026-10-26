//go:build conformance

package id3

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestConformanceAgainstFfprobe checks the reader against another that has seen far more files.
//
// Tags written by hand agree with the code that wrote them, which proves less than it looks. This
// writes a tag with `ffmpeg`, reads it back with both this package and `ffprobe`, and checks the two
// agree — over each version of the format and over text that does not fit in the older encodings.
func TestConformanceAgainstFfprobe(t *testing.T) {
	src := "../.reference/minimp3/vectors/l3-nonstandard-sin1k0db_lame_vbrtag.bit"
	if _, err := os.Stat(src); err != nil {
		t.Skipf("no fixture: %v", err)
	}

	for _, tc := range []struct {
		name    string
		version string
		title   string
		artist  string
	}{
		{"version 3", "3", "Sea Song", "Robert Wyatt"},
		{"version 4", "4", "Sea Song", "Robert Wyatt"},
		{"beyond latin-1", "4", "Émile — a title with an em dash", "Ólafur Arnalds"},
		{"beyond the basic plane", "4", "A title with an emoji 🎵", "Artist"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out := filepath.Join(t.TempDir(), "tagged.mp3")
			cmd := exec.Command("ffmpeg", "-v", "error", "-y", "-i", src, "-c", "copy",
				"-id3v2_version", tc.version,
				"-metadata", "title="+tc.title,
				"-metadata", "artist="+tc.artist,
				"-metadata", "album=Rock Bottom",
				"-metadata", "date=1974",
				"-metadata", "track=3/12",
				out)
			if err := cmd.Run(); err != nil {
				t.Skipf("ffmpeg: %v", err)
			}

			f, err := os.Open(out)
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			got, err := Read(f)
			if err != nil {
				t.Fatal(err)
			}

			if got.Title != tc.title {
				t.Errorf("title read as %q, want %q", got.Title, tc.title)
			}
			if got.Artist != tc.artist {
				t.Errorf("artist read as %q, want %q", got.Artist, tc.artist)
			}
			if got.Album != "Rock Bottom" {
				t.Errorf("album read as %q", got.Album)
			}
			if got.Year != 1974 {
				t.Errorf("year read as %d", got.Year)
			}
			if got.Track != 3 || got.Tracks != 12 {
				t.Errorf("track read as %d of %d, want 3 of 12", got.Track, got.Tracks)
			}
			if !strings.HasPrefix(got.Version, "ID3v2.") {
				t.Errorf("version reported as %q", got.Version)
			}

			// And against what ffprobe makes of the same file, which is the point: two readers that
			// have never seen each other's code agreeing on the same bytes.
			for field, want := range map[string]string{
				"title": tc.title, "artist": tc.artist, "album": "Rock Bottom",
			} {
				if probe := ffprobeTag(t, out, field); probe != want {
					t.Errorf("ffprobe reads %s as %q, want %q", field, probe, want)
				}
			}
		})
	}
}

// ffprobeTag asks ffprobe for one tag of a file.
func ffprobeTag(t *testing.T, path, field string) string {
	t.Helper()
	out, err := exec.Command("ffprobe", "-v", "error",
		"-show_entries", "format_tags="+field, "-of", "default=nw=1:nk=1", path).Output()
	if err != nil {
		t.Skipf("ffprobe: %v", err)
	}
	return strings.TrimSpace(string(out))
}

// The vector set carries files with each kind of tag, written to trouble a decoder rather than to
// hold anything. What matters is that reading them is safe: a tag that is nonsense must not become a
// crash or a string of rubbish presented as a title.
func TestConformanceTaggedVectorsAreSafeToRead(t *testing.T) {
	dir := "../.reference/minimp3/vectors"
	names, err := filepath.Glob(filepath.Join(dir, "*id3*.bit"))
	if err != nil {
		t.Fatal(err)
	}
	ape, _ := filepath.Glob(filepath.Join(dir, "*apetag*.bit"))
	names = append(names, ape...)
	if len(names) == 0 {
		t.Skip("no tagged vectors")
	}

	for _, path := range names {
		f, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		tags, err := Read(f)
		f.Close()
		if err != nil {
			t.Errorf("%s: %v", filepath.Base(path), err)
			continue
		}
		fields := 0
		for _, v := range tags.Frames {
			fields += len(v)
		}
		t.Logf("%-40s %-8s %d fields", filepath.Base(path), tags.Version, fields)

		// Whatever came out has to be text. A reader that hands back raw bytes as a title puts them
		// into a file name.
		for id, values := range tags.Frames {
			for _, v := range values {
				if strings.ContainsRune(v, 0) {
					t.Errorf("%s: frame %s holds a null", filepath.Base(path), id)
				}
				if !strconv.CanBackquote(v) && !isPrintable(v) {
					t.Errorf("%s: frame %s is not text: %q", filepath.Base(path), id, v)
				}
			}
		}
	}
}

func isPrintable(s string) bool {
	for _, r := range s {
		if r < 0x20 && r != '\n' && r != '\t' && r != '\r' {
			return false
		}
	}
	return true
}
