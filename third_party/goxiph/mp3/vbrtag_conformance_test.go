//go:build conformance

package mp3

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// The vector set carries five files aimed at the VBR tag alone: one empty, one claiming no frames,
// one holding nothing but the tag, one corrupted, and one whose flags claim more than the frame
// holds. They are there because a tag parser that trusts its own length fields reads past the end of
// the frame, which is where the last of them gets its name.
//
// What is asserted is that none of them fails to open and none of them reports a length it cannot
// support. Which of them carries a usable tag is reported rather than fixed, since the set does not
// say and guessing would be inventing a rule.
func TestConformanceVBRTagsAreSurvived(t *testing.T) {
	for _, name := range []string{
		"l3-nonstandard-vbrtag-empty.bit",
		"l3-nonstandard-vbrtag-noframes.bit",
		"l3-nonstandard-vbrtag-only.bit",
		"l3-nonstandard-vbrtag-corrupted.bit",
		"l3-nonstandard-vbrtag-oob-read.bit",
		"l3-nonstandard-sin1k0db_lame_vbrtag.bit",
	} {
		t.Run(name, func(t *testing.T) {
			f, err := os.Open(filepath.Join(vectorDir, name))
			if err != nil {
				t.Skipf("%v", err)
			}
			defer f.Close()

			s, err := NewStream(f)
			if err != nil {
				t.Fatalf("opening: %v", err)
			}
			tag, ok := s.Tag()
			d := s.Duration()
			t.Logf("tag=%v frames=%d bytes=%d encoder=%q delay=%d padding=%d exact=%v duration=%s",
				ok, tag.Frames, tag.Bytes, tag.Encoder, tag.Delay, tag.Padding, s.Exact(), d)

			if d < 0 {
				t.Errorf("negative duration %s", d)
			}
			// Walking to the end must not fail on any of them.
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
			t.Logf("%d audio frames", n)
		})
	}
}

// TestConformanceTaggedLengthMatchesTheReference pins the sample count against the conformance set's
// own reference output.
//
// This is the number the whole tag exists to produce, and getting it wrong is the trap the Opus
// pre-skip was: the audio sounds right and lines up with nothing. It is checked against the
// reference PCM rather than against ffprobe, which reports the container's length and knows nothing
// about the delay and padding that the tag declares.
func TestConformanceTaggedLengthMatchesTheReference(t *testing.T) {
	const name = "l3-nonstandard-sin1k0db_lame_vbrtag"
	f, err := os.Open(filepath.Join(vectorDir, name+".bit"))
	if err != nil {
		t.Skipf("%v", err)
	}
	defer f.Close()

	ref, err := os.Stat(filepath.Join(vectorDir, name+".pcm"))
	if err != nil {
		t.Skipf("%v", err)
	}

	s, err := NewStream(f)
	if err != nil {
		t.Fatal(err)
	}
	tag, ok := s.Tag()
	if !ok {
		t.Fatal("no tag")
	}
	h := s.Header()
	// Sixteen bits a sample, one sample per channel per frame.
	want := ref.Size() / 2 / int64(h.Channels())
	if got := tag.Samples(h.Samples()); got != want {
		t.Errorf("the tag gives %d samples per channel, the reference holds %d", got, want)
	}
}

// A tag frame is an ordinary frame whose audio is silence, so a decoder that does not recognise it
// plays that silence and counts it. This checks it is taken out of the audio rather than handed on.
func TestConformanceTagFrameIsNotAudio(t *testing.T) {
	const name = "l3-nonstandard-sin1k0db_lame_vbrtag.bit"
	f, err := os.Open(filepath.Join(vectorDir, name))
	if err != nil {
		t.Skipf("%v", err)
	}
	defer f.Close()

	s, err := NewStream(f)
	if err != nil {
		t.Fatal(err)
	}
	tag, ok := s.Tag()
	if !ok {
		t.Fatal("no tag found in a file named for having one")
	}

	audio := 0
	for {
		if _, err := s.Next(); errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			t.Fatal(err)
		}
		audio++
	}
	// The tag counts the frames after itself, so the audio frames should agree with it.
	if tag.Frames != 0 && int(tag.Frames) != audio {
		t.Errorf("the tag declares %d frames, %d were found", tag.Frames, audio)
	}
	if tag.Encoder == "" {
		t.Error("no encoder name, so no delay or padding either")
	}
}
