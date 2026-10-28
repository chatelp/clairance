package mp3

import (
	"encoding/binary"
	"strings"
)

// The first frame of most files is not audio but a tag describing the whole stream: how many frames
// it holds, how many bytes, and where in the first and last frames the real signal begins and ends.
//
// Nothing in the format requires it and nothing marks the frame as special — it is an ordinary frame
// whose audio happens to be silence, recognised by a magic word sitting where the side information
// ends. A decoder that misses it plays the silence and reports the wrong length; one that mistakes
// an ordinary frame for it loses a frame of audio.

// VBRTag is what the leading tag frame declares, where there is one.
type VBRTag struct {
	// Frames is how many frames follow, this one excluded, and Bytes how many bytes the stream
	// occupies. Either may be absent, in which case it is zero.
	Frames uint32
	Bytes  uint32

	// Encoder is what wrote the file, from the LAME extension: "LAME3.100", "Lavf60.16.100" and so
	// on. Empty when the extension is absent.
	Encoder string

	// Delay is the samples of priming at the start of the stream and Padding the samples of silence
	// at the end, both from the LAME extension. They are what makes gapless playback possible, and
	// without them a decoder cannot know where the audio truly starts.
	Delay   int
	Padding int

	// CBR reports a tag written as "Info" rather than "Xing", which marks a constant bitrate stream
	// carrying the tag for its length and seek table alone.
	CBR bool
}

// Skip is how many samples to throw away at the start of the stream.
//
// The delay field alone, which is not what most accounts of the format say. They add 529 for the
// priming the synthesis filterbank holds back, and that is worth stating because it was tried: on
// the conformance vector written for this, the reference decoder's own output and ffmpeg's agree
// with each other to the sample and with the delay taken alone. Both come out 529 samples longer
// than the sum would give. Two independent references beat a widely repeated number.
func (t VBRTag) Skip() int {
	if t.Delay == 0 && t.Padding == 0 {
		return 0
	}
	return t.Delay
}

// Samples is how many samples per channel the stream holds, or zero where the tag does not say.
//
// The frame count covers the whole stream including the priming and the trailing silence, so both
// come off. The tag frame itself is not counted: its own frame is excluded from Frames.
func (t VBRTag) Samples(perFrame int) int64 {
	if t.Frames == 0 {
		return 0
	}
	n := int64(t.Frames) * int64(perFrame)
	if t.Delay != 0 || t.Padding != 0 {
		n -= int64(t.Skip()) + int64(t.Padding)
	}
	return max(n, 0)
}

// sideInfoSize is how many bytes of side information sit between the header and the main data.
//
// Fixed by the version and the channel count, and it is where the VBR tag is looked for: the tag
// begins exactly where the main data would.
func sideInfoSize(h Header) int {
	if h.Version == MPEG1 {
		if h.Channels() == 1 {
			return 17
		}
		return 32
	}
	if h.Channels() == 1 {
		return 9
	}
	return 17
}

// ParseVBRTag reads the leading tag out of a frame, and reports whether the frame was one.
//
// data is the frame body with the header and any checksum already removed, which is what Scanner
// hands over.
func ParseVBRTag(h Header, data []byte) (VBRTag, bool) {
	if h.Layer != Layer3 {
		return VBRTag{}, false
	}
	at := sideInfoSize(h)
	if len(data) < at+8 {
		return VBRTag{}, false
	}

	switch string(data[at : at+4]) {
	case "Xing", "Info":
		return parseXing(data[at:], string(data[at:at+4]) == "Info")
	}
	// Fraunhofer's own tag, which sits at a fixed offset rather than after the side information and
	// counts frames in a fixed-size table.
	const vbriOffset = 32
	if len(data) >= vbriOffset+26 && string(data[vbriOffset:vbriOffset+4]) == "VBRI" {
		return VBRTag{
			Bytes:  binary.BigEndian.Uint32(data[vbriOffset+10:]),
			Frames: binary.BigEndian.Uint32(data[vbriOffset+14:]),
		}, true
	}
	return VBRTag{}, false
}

// printable reports whether a string is a plausible encoder name: some text, and nothing in it that
// a name would not hold.
func printable(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < 0x20 || r > 0x7E {
			return false
		}
	}
	return true
}

// parseXing reads a Xing or Info tag, starting at its magic word.
//
// Every field after the flags is optional and present only when its flag is set, so the fields have
// to be walked in order rather than read at fixed offsets. A tag whose flags claim more than the
// frame holds is truncated rather than trusted — the vector set carries several such files, written
// to catch exactly that read.
func parseXing(b []byte, cbr bool) (VBRTag, bool) {
	t := VBRTag{CBR: cbr}
	if len(b) < 8 {
		return t, false
	}
	flags := binary.BigEndian.Uint32(b[4:])
	at := 8

	if flags&1 != 0 {
		if len(b) < at+4 {
			return t, true
		}
		t.Frames = binary.BigEndian.Uint32(b[at:])
		at += 4
	}
	if flags&2 != 0 {
		if len(b) < at+4 {
			return t, true
		}
		t.Bytes = binary.BigEndian.Uint32(b[at:])
		at += 4
	}
	if flags&4 != 0 {
		// A hundred bytes of seek table, one per hundredth of the stream. Skipped: it is an
		// approximation for seeking and says nothing this decoder needs.
		if len(b) < at+100 {
			return t, true
		}
		at += 100
	}
	if flags&8 != 0 {
		if len(b) < at+4 {
			return t, true
		}
		at += 4
	}

	// The LAME extension, thirty-six bytes beginning with the encoder's name. Other encoders write
	// it too, so the name is reported rather than required to say LAME.
	const lameSize = 36
	if len(b) < at+lameSize {
		return t, true
	}
	name := strings.TrimRight(string(b[at:at+9]), "\x00 ")
	if !printable(name) {
		// Not an encoder name, so what follows it is not a delay and a padding either. One vector
		// carries a run of zero bytes here that reads as a delay of 546 and a padding of 1062, which
		// would take a thousand samples off a stream that has none to spare.
		return t, true
	}
	t.Encoder = name

	// Delay and padding share three bytes, twelve bits each.
	d := b[at+21:]
	t.Delay = int(d[0])<<4 | int(d[1])>>4
	t.Padding = int(d[1]&0x0F)<<8 | int(d[2])
	return t, true
}
