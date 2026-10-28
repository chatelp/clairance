//go:build conformance

package mp3

import (
	"errors"
	"io"
	"os"
	"testing"

	"github.com/yegors/co-atc/third_party/goxiph/audio"
)

// TestConformanceGaplessTrimming checks the decoder against the length the tag declares.
//
// A stream carries no granule position and no length of its own, so the tag's delay and padding are
// the only things that say where the audio starts and stops. Getting it wrong is the trap the Opus
// pre-skip was: the audio sounds right and lines up with nothing.
func TestConformanceGaplessTrimming(t *testing.T) {
	const name = "l3-nonstandard-sin1k0db_lame_vbrtag.bit"
	f, err := os.Open(vectorDir + "/" + name)
	if err != nil {
		t.Skipf("%v", err)
	}
	defer f.Close()

	d, err := NewDecoder(f)
	if err != nil {
		t.Fatal(err)
	}
	tag, ok := d.Stream().Tag()
	if !ok {
		t.Fatal("no tag in a file named for having one")
	}
	want := tag.Samples(d.Stream().Header().Samples())

	buf, err := audio.NewBuffer(d.Format(), 0)
	if err != nil {
		t.Fatal(err)
	}
	var got int64
	for {
		err := d.Next(buf)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("after %d samples: %v", got, err)
		}
		got += int64(buf.Frames())
	}

	t.Logf("%d samples decoded, the tag declares %d (delay %d, padding %d)",
		got, want, tag.Delay, tag.Padding)
	if got != want {
		t.Errorf("decoded %d samples, the tag declares %d", got, want)
	}
}
