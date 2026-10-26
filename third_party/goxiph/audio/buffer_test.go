package audio

import "testing"

// A decoder fills whatever buffer it is handed, so resizing has to establish the shape the format
// names rather than assume the caller already did. A fresh buffer has no planes and one carried over
// from another stream may have the wrong number; both used to index past the end.
func TestResizeGivesOnePlanePerChannel(t *testing.T) {
	var b Buffer
	b.Format = Format{Channels: 2, SampleRate: 44100}
	b.Resize(128)
	if err := b.Validate(); err != nil {
		t.Fatalf("a resized zero buffer does not validate: %v", err)
	}
	if b.Frames() != 128 {
		t.Errorf("resized to %d frames, want 128", b.Frames())
	}

	// And carried from a stereo stream to a mono one, and back.
	b.Format = Format{Channels: 1, SampleRate: 44100}
	b.Resize(64)
	if err := b.Validate(); err != nil {
		t.Fatalf("after narrowing: %v", err)
	}
	b.Format = Format{Channels: 6, SampleRate: 44100}
	b.Resize(64)
	if err := b.Validate(); err != nil {
		t.Fatalf("after widening: %v", err)
	}
}
