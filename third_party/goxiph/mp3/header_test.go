package mp3

import (
	"errors"
	"testing"
)

// header builds the four bytes from the fields, so a test can say what it means rather than spell
// out a hexadecimal constant nobody can read back.
func header(version Version, layer Layer, crc bool, bitrateIndex, rateIndex int, pad bool, mode Mode) []byte {
	b := []byte{0xFF, 0xE0, 0, 0}
	b[1] |= byte(version) << 3
	b[1] |= byte(layer) << 1
	if !crc {
		b[1] |= 1
	}
	b[2] = byte(bitrateIndex)<<4 | byte(rateIndex)<<2
	if pad {
		b[2] |= 2
	}
	b[3] = byte(mode) << 6
	return b
}

// The published frame sizes, which are what everything downstream indexes by. A wrong size does not
// fail to decode; it reads the next frame from the wrong place and the whole stream slides.
func TestFrameSizeMatchesTheKnownCases(t *testing.T) {
	for _, tc := range []struct {
		name         string
		version      Version
		layer        Layer
		bitrateIndex int
		rateIndex    int
		pad          bool
		want         int
		samples      int
	}{
		{"128k 44.1k layer III", MPEG1, Layer3, 9, 0, false, 417, 1152},
		{"128k 44.1k layer III padded", MPEG1, Layer3, 9, 0, true, 418, 1152},
		{"128k 48k layer III", MPEG1, Layer3, 9, 1, false, 384, 1152},
		{"320k 44.1k layer III", MPEG1, Layer3, 14, 0, false, 1044, 1152},
		{"64k 22.05k layer III", MPEG2, Layer3, 8, 0, false, 208, 576},
		{"64k 11.025k layer III", MPEG25, Layer3, 8, 0, false, 417, 576},
		{"8k 8k layer III", MPEG25, Layer3, 1, 2, false, 72, 576},
		{"224k 44.1k layer II", MPEG1, Layer2, 11, 0, false, 731, 1152},
		{"192k 44.1k layer I", MPEG1, Layer1, 6, 0, false, 208, 384},
		{"32k 32k layer I", MPEG1, Layer1, 1, 2, false, 48, 384},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h, err := ParseHeader(header(tc.version, tc.layer, false, tc.bitrateIndex, tc.rateIndex, tc.pad, Stereo))
			if err != nil {
				t.Fatal(err)
			}
			if got := h.FrameSize(); got != tc.want {
				t.Errorf("frame is %d bytes, want %d", got, tc.want)
			}
			if got := h.Samples(); got != tc.samples {
				t.Errorf("frame holds %d samples, want %d", got, tc.samples)
			}
		})
	}
}

// The version field is two bits and one of its values was reserved in MPEG-1 and later taken for the
// half-rate extension. Reading it as a single bit is the common fault and it makes every MPEG-2.5
// file unreadable.
func TestVersionsAndRates(t *testing.T) {
	for _, tc := range []struct {
		version Version
		rates   [3]int
	}{
		{MPEG1, [3]int{44100, 48000, 32000}},
		{MPEG2, [3]int{22050, 24000, 16000}},
		{MPEG25, [3]int{11025, 12000, 8000}},
	} {
		for i, want := range tc.rates {
			h, err := ParseHeader(header(tc.version, Layer3, false, 9, i, false, Stereo))
			if err != nil {
				t.Fatalf("%s rate %d: %v", tc.version, i, err)
			}
			if h.SampleRate != want {
				t.Errorf("%s rate index %d is %d Hz, want %d", tc.version, i, h.SampleRate, want)
			}
			if h.Version != tc.version {
				t.Errorf("read the version as %s, want %s", h.Version, tc.version)
			}
		}
	}
}

// What must be refused, and refused distinctly: free format is a legal stream this decoder does not
// do, and the rest are malformed.
func TestHeaderRefusals(t *testing.T) {
	for _, tc := range []struct {
		name string
		b    []byte
		want error
	}{
		{"no sync", []byte{0x00, 0x00, 0x00, 0x00}, ErrBadFrame},
		{"short", []byte{0xFF, 0xFB}, ErrBadFrame},
		{"reserved version", header(versionReserved, Layer3, false, 9, 0, false, Stereo), ErrBadFrame},
		{"reserved layer", header(MPEG1, layerReserved, false, 9, 0, false, Stereo), ErrBadFrame},
		{"reserved rate", header(MPEG1, Layer3, false, 9, 3, false, Stereo), ErrBadFrame},
		{"forbidden bitrate", header(MPEG1, Layer3, false, 15, 0, false, Stereo), ErrBadFrame},
		{"free format", header(MPEG1, Layer3, false, 0, 0, false, Stereo), ErrFreeFormat},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := ParseHeader(tc.b); !errors.Is(err, tc.want) {
				t.Errorf("got %v, want %v", err, tc.want)
			}
		})
	}
}

// The checksum bit is set when there is no checksum. Reading it the natural way puts every field
// after the header two bytes out.
func TestCRCBitIsInverted(t *testing.T) {
	with, err := ParseHeader(header(MPEG1, Layer3, true, 9, 0, false, Stereo))
	if err != nil {
		t.Fatal(err)
	}
	if !with.CRC || with.CRCSize() != 2 {
		t.Error("a frame with a checksum did not report one")
	}
	without, err := ParseHeader(header(MPEG1, Layer3, false, 9, 0, false, Stereo))
	if err != nil {
		t.Fatal(err)
	}
	if without.CRC || without.CRCSize() != 0 {
		t.Error("a frame without a checksum reported one")
	}
}

func TestModesAndChannelCounts(t *testing.T) {
	for mode, want := range map[Mode]int{Stereo: 2, JointStereo: 2, DualChannel: 2, Mono: 1} {
		h, err := ParseHeader(header(MPEG1, Layer3, false, 9, 0, false, mode))
		if err != nil {
			t.Fatal(err)
		}
		if h.Mode != mode {
			t.Errorf("read the mode as %s, want %s", h.Mode, mode)
		}
		if h.Channels() != want {
			t.Errorf("%s carries %d channels, want %d", mode, h.Channels(), want)
		}
	}
}
