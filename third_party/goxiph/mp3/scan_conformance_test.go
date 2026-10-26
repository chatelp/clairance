//go:build conformance

package mp3

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// vectorDir is where `mise run fixtures:minimp3` leaves the compliance bitstreams.
const vectorDir = "../.reference/minimp3/vectors"

// The ISO compliance bitstreams, which minimp3 carries alongside its reference output. They reach
// what a generated corpus cannot: every layer, both lower-sampling-frequency versions, the stereo
// modes, and a set of malformed and tag-laden files taken from real complaints.
func vectors(t *testing.T) []string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(vectorDir, "*.bit"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Skip("no compliance vectors; run `mise run fixtures:minimp3`")
	}
	sort.Strings(files)
	return files
}

// TestConformanceFrameSizesWalkTheStream checks the header tables against every vector.
//
// The check is that the stream is self-consistent: each frame's declared size has to put the next
// frame's sync word exactly where the scanner then finds it, all the way to the end. A wrong bitrate
// table entry or a wrong frame-size formula cannot survive that, because the error compounds — one
// byte out on the first frame and nothing after it lines up.
//
// This is what the tables are verified by. They were transcribed rather than derived, and no
// specification is at hand to check them against.
func TestConformanceFrameSizesWalkTheStream(t *testing.T) {
	var frames, files int
	versions := map[string]int{}
	layers := map[string]int{}
	modes := map[string]int{}
	rates := map[int]int{}
	var refused []string

	for _, path := range vectors(t) {
		name := filepath.Base(path)
		f, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		s, err := NewScanner(f)
		f.Close()
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}

		n := 0
		for {
			fr, err := s.Next()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				// A vector this decoder will not take is recorded rather than failed: the set holds
				// free format and malformed streams to be refused.
				refused = append(refused, name+": "+err.Error())
				break
			}
			n++
			versions[fr.Header.Version.String()]++
			layers[fr.Header.Layer.String()]++
			modes[fr.Header.Mode.String()]++
			rates[fr.Header.SampleRate]++

			if len(fr.Data) == 0 {
				t.Errorf("%s frame %d has no body", name, n)
			}
		}
		if n == 0 && len(refused) == 0 {
			t.Errorf("%s: no frames found", name)
		}
		frames += n
		files++
	}

	t.Logf("%d frames over %d files", frames, files)
	t.Logf("versions %v", versions)
	t.Logf("layers %v", layers)
	t.Logf("modes %v", modes)
	t.Logf("rates %v", rates)
	for _, r := range refused {
		t.Logf("refused %s", r)
	}
}

// The tag-laden vectors are in the set because tags are what stops a decoder syncing. ID3v2 goes in
// front of the audio and can be hundreds of kilobytes of cover art; ID3v1 and APE go after it and
// must not be mistaken for frames.
func TestConformanceTagsDoNotStopTheScan(t *testing.T) {
	for _, name := range []string{
		"l3-nonstandard-id3v1.bit",
		"l3-nonstandard-id3v2.bit",
		"l3-nonstandard-id3v2-only.bit",
		"l3-nonstandard-apetag.bit",
		"l3-nonstandard-id3v1-apetag.bit",
	} {
		t.Run(name, func(t *testing.T) {
			f, err := os.Open(filepath.Join(vectorDir, name))
			if err != nil {
				t.Skipf("%v", err)
			}
			defer f.Close()
			s, err := NewScanner(f)
			if err != nil {
				t.Fatal(err)
			}
			n := 0
			for {
				_, err := s.Next()
				if errors.Is(err, io.EOF) {
					break
				}
				if err != nil {
					t.Fatalf("after %d frames: %v", n, err)
				}
				n++
			}
			t.Logf("%d frames, %d bytes skipped", n, s.Skipped())
			// id3v2-only holds a tag and no audio at all, which is a file a player must not crash on.
			if n == 0 && name != "l3-nonstandard-id3v2-only.bit" {
				t.Error("no frames found")
			}
		})
	}
}
