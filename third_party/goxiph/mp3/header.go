// Package mp3 decodes MPEG-1, MPEG-2 and MPEG-2.5 audio, layers I, II and III.
//
// Decoding only. There is no encoder here and none planned.
//
// MP3 is the one format in this module with no freely readable specification. ISO/IEC 11172-3 and
// 13818-3 are sold rather than published, and MPEG-2.5 — the half-rate extension almost every
// low-bitrate file uses — was never standardised at all. What is public is the conformance criteria
// in ISO/IEC 11172-4 annex A, which is what the gates here are set to.
package mp3

import (
	"errors"
	"fmt"
)

// ErrBadFrame reports a frame header that is not one, or describes something this decoder will not
// produce.
var ErrBadFrame = errors.New("mp3: invalid frame")

// ErrFreeFormat reports a stream whose bitrate is not one of the standard ones.
//
// Named rather than folded into ErrBadFrame because the stream is legal and this decoder does not do
// it. Free format sets the bitrate index to zero and leaves the frame length to be worked out by
// finding the next sync word, which takes three frames to settle. Almost nothing produces such files
// and almost nothing plays them.
var ErrFreeFormat = errors.New("mp3: free format is not supported")

// Version is which MPEG audio standard a frame belongs to.
//
// MPEG-2.5 is the odd one: an extension to a quarter of MPEG-1's sample rates that appears in no
// standard, recognised by a version field the original specification reserved.
type Version int

// The versions, in the order the header's two-bit field numbers them.
const (
	MPEG25 Version = iota
	versionReserved
	MPEG2
	MPEG1
)

func (v Version) String() string {
	switch v {
	case MPEG1:
		return "MPEG-1"
	case MPEG2:
		return "MPEG-2"
	case MPEG25:
		return "MPEG-2.5"
	}
	return "reserved"
}

// lsf reports whether a version uses the lower sampling frequencies, which halve a layer III frame
// and change how its scale factors are coded.
func (v Version) lsf() bool { return v == MPEG2 || v == MPEG25 }

// Layer is which of the three codings a frame uses. They share a header and a synthesis filterbank
// and differ in everything between.
type Layer int

// The layers, numbered downwards by the header's field: three is one, one is three.
const (
	layerReserved Layer = iota
	Layer3
	Layer2
	Layer1
)

func (l Layer) String() string {
	switch l {
	case Layer1:
		return "I"
	case Layer2:
		return "II"
	case Layer3:
		return "III"
	}
	return "reserved"
}

// Mode is how a frame's channels are coded.
type Mode int

// The channel modes, in the order the header's two-bit field numbers them.
const (
	Stereo Mode = iota
	JointStereo
	DualChannel
	Mono
)

func (m Mode) String() string {
	switch m {
	case Stereo:
		return "stereo"
	case JointStereo:
		return "joint stereo"
	case DualChannel:
		return "dual channel"
	case Mono:
		return "mono"
	}
	return "unknown"
}

// Channels is how many channels a mode carries. Dual channel is two unrelated programmes and decodes
// as two channels like any other pair.
func (m Mode) Channels() int {
	if m == Mono {
		return 1
	}
	return 2
}

// HeaderSize is the length of a frame header in bytes.
const HeaderSize = 4

// Header is one frame's header, the four bytes every frame begins with.
type Header struct {
	Version Version
	Layer   Layer
	// CRC reports that a sixteen-bit checksum follows the header. The bit in the stream is set when
	// there is no checksum, which is the wrong way round and worth stating once.
	CRC        bool
	Bitrate    int // bits per second
	SampleRate int // hertz
	Padding    bool
	Private    bool
	Mode       Mode
	// ModeExtension says which joint stereo couplings are in use. Its meaning differs by layer: in
	// layers I and II it is the band the intensity coupling starts at, in layer III it is two flags,
	// one for mid-side and one for intensity.
	ModeExtension int
	Copyright     bool
	Original      bool
	Emphasis      int
}

// bitrates are the standard bitrates in kilobits per second, indexed by the header's four-bit field.
//
// Index zero is free format and index fifteen is forbidden; both are held as zero here and rejected
// by ParseHeader. MPEG-2 and MPEG-2.5 share one table, and within each version layers II and III
// share another — so there are five distinct rows and not nine.
var bitrates = [2][4][16]int{
	// MPEG-2 and MPEG-2.5.
	{
		{}, // reserved layer
		// Layer III
		{0, 8, 16, 24, 32, 40, 48, 56, 64, 80, 96, 112, 128, 144, 160, 0},
		// Layer II
		{0, 8, 16, 24, 32, 40, 48, 56, 64, 80, 96, 112, 128, 144, 160, 0},
		// Layer I
		{0, 32, 48, 56, 64, 80, 96, 112, 128, 144, 160, 176, 192, 224, 256, 0},
	},
	// MPEG-1.
	{
		{},
		{0, 32, 40, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320, 0},
		{0, 32, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320, 384, 0},
		{0, 32, 64, 96, 128, 160, 192, 224, 256, 288, 320, 352, 384, 416, 448, 0},
	},
}

// sampleRates are the three rates each version allows, indexed by the header's two-bit field. The
// fourth index is reserved and rejected.
var sampleRates = [4][4]int{
	MPEG25:          {11025, 12000, 8000, 0},
	MPEG2:           {22050, 24000, 16000, 0},
	MPEG1:           {44100, 48000, 32000, 0},
	versionReserved: {},
}

// Samples is how many samples per channel one frame of this header holds.
//
// Layer III halves at the lower sampling frequencies: the same 1152 samples would span too long a
// window at 8 kHz, so MPEG-2 codes one granule per frame where MPEG-1 codes two.
func (h Header) Samples() int {
	switch h.Layer {
	case Layer1:
		return 384
	case Layer2:
		return 1152
	case Layer3:
		if h.Version.lsf() {
			return 576
		}
		return 1152
	}
	return 0
}

// FrameSize is the whole frame in bytes, header and padding included.
//
// The rate and the frame length rarely divide evenly, so a padding byte is added to some frames to
// keep the average right. Layer I pads in four-byte slots rather than single bytes, because its
// frame is a whole number of thirty-two-bit words.
func (h Header) FrameSize() int {
	if h.SampleRate == 0 {
		return 0
	}
	pad := 0
	if h.Padding {
		pad = 1
	}
	if h.Layer == Layer1 {
		return (12*h.Bitrate/h.SampleRate + pad) * 4
	}
	return h.Samples()/8*h.Bitrate/h.SampleRate + pad
}

// CRCSize is how many bytes of checksum follow the header, zero or two.
func (h Header) CRCSize() int {
	if h.CRC {
		return 2
	}
	return 0
}

// Channels is how many channels the frame carries.
func (h Header) Channels() int { return h.Mode.Channels() }

// ParseHeader reads the four bytes a frame begins with.
func ParseHeader(b []byte) (Header, error) {
	var h Header
	if len(b) < HeaderSize {
		return h, fmt.Errorf("%w: %d bytes, need %d", ErrBadFrame, len(b), HeaderSize)
	}
	// Eleven bits of sync. The twelfth was ones in MPEG-1 too, which is why so much code looks for
	// twelve and then cannot read MPEG-2.5 — its version field begins with a zero there.
	if b[0] != 0xFF || b[1]&0xE0 != 0xE0 {
		return h, fmt.Errorf("%w: no sync word", ErrBadFrame)
	}

	h.Version = Version(b[1] >> 3 & 3)
	h.Layer = Layer(b[1] >> 1 & 3)
	h.CRC = b[1]&1 == 0
	if h.Version == versionReserved {
		return h, fmt.Errorf("%w: reserved version", ErrBadFrame)
	}
	if h.Layer == layerReserved {
		return h, fmt.Errorf("%w: reserved layer", ErrBadFrame)
	}

	rateIndex := int(b[2] >> 2 & 3)
	if rateIndex == 3 {
		return h, fmt.Errorf("%w: reserved sample rate", ErrBadFrame)
	}
	h.SampleRate = sampleRates[h.Version][rateIndex]

	version := 0
	if h.Version == MPEG1 {
		version = 1
	}
	bitrateIndex := int(b[2] >> 4)
	if bitrateIndex == 15 {
		return h, fmt.Errorf("%w: forbidden bitrate", ErrBadFrame)
	}
	if bitrateIndex == 0 {
		return h, ErrFreeFormat
	}
	h.Bitrate = bitrates[version][h.Layer][bitrateIndex] * 1000

	h.Padding = b[2]>>1&1 == 1
	h.Private = b[2]&1 == 1
	h.Mode = Mode(b[3] >> 6 & 3)
	h.ModeExtension = int(b[3] >> 4 & 3)
	h.Copyright = b[3]>>3&1 == 1
	h.Original = b[3]>>2&1 == 1
	h.Emphasis = int(b[3] & 3)
	if h.Emphasis == 2 {
		return h, fmt.Errorf("%w: reserved emphasis", ErrBadFrame)
	}
	return h, nil
}

// sameStream reports whether two headers could belong to one stream, which is what tells a real sync
// word from four bytes of audio that happen to look like one.
func (h Header) sameStream(o Header) bool {
	return h.Version == o.Version && h.Layer == o.Layer &&
		h.SampleRate == o.SampleRate && h.Mode.Channels() == o.Mode.Channels()
}
