package mp3

import (
	"errors"
	"testing"

	"github.com/yegors/co-atc/third_party/goxiph/internal/bitio"
)

// The two layouts differ by more than their lengths, and the differences are where a decoder written
// from one of them silently misreads the other.
func TestSideInfoSizesByVersionAndChannels(t *testing.T) {
	for _, tc := range []struct {
		version  Version
		mode     Mode
		size     int
		granules int
	}{
		{MPEG1, Mono, 17, 2},
		{MPEG1, Stereo, 32, 2},
		{MPEG1, JointStereo, 32, 2},
		{MPEG2, Mono, 9, 1},
		{MPEG2, Stereo, 17, 1},
		{MPEG25, Mono, 9, 1},
		{MPEG25, Stereo, 17, 1},
	} {
		h := Header{Version: tc.version, Layer: Layer3, Mode: tc.mode}
		if got := sideInfoSize(h); got != tc.size {
			t.Errorf("%s %s: side information is %d bytes, want %d", tc.version, tc.mode, got, tc.size)
		}
		if got := granules(tc.version); got != tc.granules {
			t.Errorf("%s: %d granules, want %d", tc.version, got, tc.granules)
		}
	}
}

// A side information block built field by field, so a test can state what it expects to be read back
// rather than assert against bytes nobody can check.
type sideInfoBuilder struct{ w *bitio.MSBWriter }

func newSideInfo() *sideInfoBuilder { return &sideInfoBuilder{w: bitio.NewMSBWriter()} }

func (b *sideInfoBuilder) put(v uint64, n uint) *sideInfoBuilder {
	_ = b.w.Write(v, n)
	return b
}

func (b *sideInfoBuilder) bytes(size int) []byte {
	b.w.AlignByte()
	out := append([]byte(nil), b.w.Bytes()...)
	for len(out) < size {
		out = append(out, 0)
	}
	return out[:size]
}

// granule writes one unswitched granule with recognisable values in every field.
func (b *sideInfoBuilder) granule(lsf bool, part23, big, gain, compress int) *sideInfoBuilder {
	b.put(uint64(part23), 12).put(uint64(big), 9).put(uint64(gain), 8)
	if lsf {
		b.put(uint64(compress), 9)
	} else {
		b.put(uint64(compress), 4)
	}
	b.put(0, 1)                     // no window switching
	b.put(1, 5).put(2, 5).put(3, 5) // three tables
	b.put(4, 4).put(2, 3)           // region counts
	if !lsf {
		b.put(1, 1) // preflag, read only at MPEG-1
	}
	b.put(1, 1).put(0, 1) // scalefac_scale, count1 table
	return b
}

// MPEG-1 packs a nine-bit pointer, then private bits, then four selection flags per channel. Reading
// the pointer as eight bits, which is what the lower sampling frequencies use, shifts everything.
func TestSideInfoMPEG1Layout(t *testing.T) {
	h := Header{Version: MPEG1, Layer: Layer3, Mode: Stereo}
	b := newSideInfo()
	b.put(511, 9)  // main data begins as far back as the field reaches
	b.put(5, 3)    // private bits
	b.put(0xA5, 8) // four selection flags per channel
	for range 4 {  // two granules, two channels
		b.granule(false, 100, 200, 210, 9)
	}

	si, err := ParseSideInfo(h, b.bytes(sideInfoSize(h)))
	if err != nil {
		t.Fatal(err)
	}
	if si.MainDataBegin != 511 {
		t.Errorf("main data begins %d bytes back, want 511", si.MainDataBegin)
	}
	if si.Private != 5 {
		t.Errorf("private bits are %d, want 5", si.Private)
	}
	// The flags belong to the second granule. The first has nothing behind it to share with, so it
	// must read none however the field was set.
	if got := si.Granule[0][0].Scfsi; got != 0 {
		t.Errorf("the first granule carries selection %x, and it can share nothing", got)
	}
	if got := si.Granule[1][0].Scfsi; got != 0xA {
		t.Errorf("channel 0 selection is %x, want a", got)
	}
	if got := si.Granule[1][1].Scfsi; got != 0x5 {
		t.Errorf("channel 1 selection is %x, want 5", got)
	}
	g := si.Granule[1][1]
	if g.Part23Length != 100 || g.BigValues != 200 || g.GlobalGain != 210 || g.ScalefacCompress != 9 {
		t.Errorf("the last granule read back as %+v", g)
	}
	if !g.Preflag {
		t.Error("MPEG-1 reads the preemphasis flag from the stream, and it was set")
	}
}

// The lower sampling frequencies code one granule, a nine-bit scale factor sizing, and no
// preemphasis flag — it follows from the sizing instead.
func TestSideInfoLSFLayout(t *testing.T) {
	for _, version := range []Version{MPEG2, MPEG25} {
		h := Header{Version: version, Layer: Layer3, Mode: Mono}
		b := newSideInfo()
		b.put(255, 8).put(1, 1)
		b.granule(true, 300, 100, 180, 500)

		si, err := ParseSideInfo(h, b.bytes(sideInfoSize(h)))
		if err != nil {
			t.Fatalf("%s: %v", version, err)
		}
		if si.MainDataBegin != 255 {
			t.Errorf("%s: main data begins %d bytes back, want 255", version, si.MainDataBegin)
		}
		g := si.Granule[0][0]
		if g.ScalefacCompress != 500 {
			t.Errorf("%s: scale factor sizing is %d, want 500", version, g.ScalefacCompress)
		}
		if !g.Preflag {
			t.Errorf("%s: a sizing of 500 or more carries the preemphasis, and it was not reported", version)
		}
		if g.Scfsi != 0 {
			t.Errorf("%s: there is no selection field at the lower sampling frequencies, got %x", version, g.Scfsi)
		}
	}
}

// A switched granule names two tables rather than three and fixes its own region boundaries, so a
// parser that reads three walks off into the main data.
func TestSideInfoSwitchedGranule(t *testing.T) {
	h := Header{Version: MPEG1, Layer: Layer3, Mode: Mono}
	b := newSideInfo()
	b.put(0, 9).put(0, 5).put(0, 4)
	for range 2 {
		b.put(50, 12).put(10, 9).put(200, 8).put(3, 4)
		b.put(1, 1) // window switching
		b.put(uint64(BlockShort), 2)
		b.put(0, 1)            // not mixed
		b.put(7, 5).put(11, 5) // two tables
		b.put(1, 3).put(2, 3).put(3, 3)
		b.put(0, 1).put(0, 1).put(0, 1)
	}

	si, err := ParseSideInfo(h, b.bytes(sideInfoSize(h)))
	if err != nil {
		t.Fatal(err)
	}
	g := si.Granule[0][0]
	if g.BlockType != BlockShort {
		t.Errorf("block type is %v, want short", g.BlockType)
	}
	if g.TableSelect[0] != 7 || g.TableSelect[1] != 11 || g.TableSelect[2] != 0 {
		t.Errorf("tables read back as %v", g.TableSelect)
	}
	if g.SubblockGain != [3]int{1, 2, 3} {
		t.Errorf("subblock gains read back as %v", g.SubblockGain)
	}
	// A short block that is not mixed covers eight regions before the fixed boundary.
	if g.RegionCount[0] != 8 {
		t.Errorf("region count is %d, want 8 for an unmixed short block", g.RegionCount[0])
	}
}

// A start or stop block may set the mixed flag too, and it means the same thing there: keep the
// lowest subbands long. Dropping it because the block is not a short one leaves those subbands
// transformed the wrong length against a mixed neighbour, which is worth about seventy decibels.
func TestSideInfoMixedFlagSurvivesEveryBlockType(t *testing.T) {
	h := Header{Version: MPEG1, Layer: Layer3, Mode: Mono}
	for _, block := range []BlockType{BlockShort, BlockStart, BlockStop} {
		b := newSideInfo()
		b.put(0, 9).put(0, 5).put(0, 4)
		for range 2 {
			b.put(50, 12).put(10, 9).put(200, 8).put(3, 4)
			b.put(1, 1) // window switching
			b.put(uint64(block), 2)
			b.put(1, 1) // mixed
			b.put(7, 5).put(11, 5)
			b.put(1, 3).put(2, 3).put(3, 3)
			b.put(0, 1).put(0, 1).put(0, 1)
		}

		si, err := ParseSideInfo(h, b.bytes(sideInfoSize(h)))
		if err != nil {
			t.Fatal(err)
		}
		g := si.Granule[0][0]
		if !g.Mixed {
			t.Errorf("%v: the mixed flag was dropped", block)
		}
		if got := longBandsOf(Header{Version: MPEG1, Layer: Layer3, SampleRate: 44100}, g); got != 2 {
			t.Errorf("%v: %d long subbands, want 2", block, got)
		}
	}
}

// Two shapes the stream can hold that describe nothing, both of which the vector set carries.
func TestSideInfoRefusals(t *testing.T) {
	h := Header{Version: MPEG1, Layer: Layer3, Mode: Mono}

	tooMany := newSideInfo()
	tooMany.put(0, 9).put(0, 5).put(0, 4)
	tooMany.put(50, 12).put(400, 9) // 400 pairs, the spectrum holds 288
	if _, err := ParseSideInfo(h, tooMany.bytes(sideInfoSize(h))); !errors.Is(err, ErrBadFrame) {
		t.Errorf("a granule claiming 400 big values gave %v", err)
	}

	longSwitch := newSideInfo()
	longSwitch.put(0, 9).put(0, 5).put(0, 4)
	longSwitch.put(50, 12).put(10, 9).put(200, 8).put(3, 4)
	longSwitch.put(1, 1).put(uint64(BlockLong), 2) // switching, and then the unswitched type
	if _, err := ParseSideInfo(h, longSwitch.bytes(sideInfoSize(h))); !errors.Is(err, ErrBadFrame) {
		t.Errorf("a switched granule naming the long block gave %v", err)
	}
}

// The reservoir is what makes frames dependent on each other, and reaching further back than has
// been collected is normal at the start of a file rather than a fault in it.
func TestReservoirRefusesWhatItDoesNotHold(t *testing.T) {
	var res reservoir
	if _, err := res.open([]byte{1, 2, 3, 4}, 100); !errors.Is(err, ErrReservoir) {
		t.Errorf("reaching back 100 bytes into an empty reservoir gave %v", err)
	}
	// That frame's own data is still kept, so the next frame can reach into it.
	r, err := res.open([]byte{5, 6}, 4)
	if err != nil {
		t.Fatalf("reaching back 4 bytes after 4 were pushed: %v", err)
	}
	got, _ := r.Read(8)
	if got != 1 {
		t.Errorf("the reservoir opened at %d, want the first byte pushed", got)
	}
}
