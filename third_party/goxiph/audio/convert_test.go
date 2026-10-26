package audio

import (
	"math"
	"testing"
)

func mustBuffer(t *testing.T, f Format, frames int) *Buffer {
	t.Helper()
	b, err := NewBuffer(f, frames)
	if err != nil {
		t.Fatalf("NewBuffer: %v", err)
	}
	return b
}

// TestIntRoundTripIsBitExact is the property the lossless FLAC and WAV paths rest on. It holds only
// because scaleForDepth uses 2^(depth-1) rather than 2^(depth-1)-1.
func TestIntRoundTripIsBitExact(t *testing.T) {
	for _, depth := range []int{8, 16, 20, MaxIntDepth} {
		t.Run(depthName(depth), func(t *testing.T) {
			lo := -(int32(1) << (depth - 1))
			hi := (int32(1) << (depth - 1)) - 1
			src := []int32{lo, lo + 1, -1, 0, 1, hi - 1, hi}

			b := mustBuffer(t, Format{SampleRate: 44100, Channels: 1}, 0)
			if err := FromInt(src, depth, b); err != nil {
				t.Fatalf("FromInt: %v", err)
			}
			got := make([]int32, len(src))
			if err := ToInt(b, depth, got); err != nil {
				t.Fatalf("ToInt: %v", err)
			}
			for i := range src {
				if got[i] != src[i] {
					t.Errorf("sample %d: %d -> %v -> %d", i, src[i], b.Data[0][i], got[i])
				}
			}
		})
	}
}

func depthName(d int) string {
	return string(rune('0'+d/10)) + string(rune('0'+d%10)) + "bit"
}

func TestInt16RoundTripIsBitExact(t *testing.T) {
	src := []int16{math.MinInt16, -32767, -1, 0, 1, 32766, math.MaxInt16}
	b := mustBuffer(t, Format{SampleRate: 48000, Channels: 1}, 0)
	if err := FromInt16(src, b); err != nil {
		t.Fatalf("FromInt16: %v", err)
	}
	got := make([]int16, len(src))
	if err := ToInt16(b, got); err != nil {
		t.Fatalf("ToInt16: %v", err)
	}
	for i := range src {
		if got[i] != src[i] {
			t.Errorf("sample %d: %d -> %v -> %d", i, src[i], b.Data[0][i], got[i])
		}
	}
}

func TestToIntClamps(t *testing.T) {
	b := mustBuffer(t, Format{SampleRate: 44100, Channels: 1}, 4)
	copy(b.Data[0], []float32{-4, -1.5, 1.5, 4})

	got := make([]int32, 4)
	if err := ToInt(b, 16, got); err != nil {
		t.Fatalf("ToInt: %v", err)
	}
	want := []int32{-32768, -32768, 32767, 32767}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("sample %d = %d, want %d", i, got[i], want[i])
		}
	}
}

func TestInterleaveRoundTrip(t *testing.T) {
	for _, ch := range []int{1, 2, 6} {
		b := mustBuffer(t, Format{SampleRate: 44100, Channels: ch}, 5)
		for c := range b.Data {
			for j := range b.Data[c] {
				b.Data[c][j] = float32(c*100+j) / 1000
			}
		}

		flat := make([]float32, ch*5)
		if err := Interleave(b, flat); err != nil {
			t.Fatalf("Interleave: %v", err)
		}
		back := mustBuffer(t, Format{SampleRate: 44100, Channels: ch}, 0)
		if err := Deinterleave(flat, back); err != nil {
			t.Fatalf("Deinterleave: %v", err)
		}
		for c := range b.Data {
			for j := range b.Data[c] {
				if back.Data[c][j] != b.Data[c][j] {
					t.Fatalf("ch %d frame %d: %v, want %v", c, j, back.Data[c][j], b.Data[c][j])
				}
			}
		}
	}
}

func TestInterleaveChannelOrder(t *testing.T) {
	b := mustBuffer(t, Format{SampleRate: 44100, Channels: 2}, 3)
	copy(b.Data[0], []float32{1, 2, 3})
	copy(b.Data[1], []float32{-1, -2, -3})

	flat := make([]float32, 6)
	if err := Interleave(b, flat); err != nil {
		t.Fatalf("Interleave: %v", err)
	}
	want := []float32{1, -1, 2, -2, 3, -3}
	for i := range want {
		if flat[i] != want[i] {
			t.Fatalf("got %v, want %v", flat, want)
		}
	}
}

func TestRejectsBadInput(t *testing.T) {
	b := mustBuffer(t, Format{SampleRate: 44100, Channels: 2}, 4)

	if err := Deinterleave(make([]float32, 5), b); err == nil {
		t.Error("Deinterleave accepted a length not divisible by the channel count")
	}
	if err := Interleave(b, make([]float32, 3)); err == nil {
		t.Error("Interleave accepted an undersized destination")
	}
	if err := ToInt(b, 1, make([]int32, 8)); err == nil {
		t.Error("ToInt accepted bit depth 1")
	}
}

// TestRejectsDepthsFloat32CannotHold guards the reason for MaxIntDepth. Silently accepting 32-bit
// would corrupt the low bits of a stream that is supposed to be lossless.
func TestRejectsDepthsFloat32CannotHold(t *testing.T) {
	b := mustBuffer(t, Format{SampleRate: 44100, Channels: 1}, 4)
	for _, depth := range []int{25, 32} {
		if err := ToInt(b, depth, make([]int32, 4)); err == nil {
			t.Errorf("ToInt accepted bit depth %d", depth)
		}
		if err := FromInt(make([]int32, 4), depth, b); err == nil {
			t.Errorf("FromInt accepted bit depth %d", depth)
		}
	}
}

func TestBufferPlanesShareNoOverlap(t *testing.T) {
	b := mustBuffer(t, Format{SampleRate: 44100, Channels: 2}, 4)
	// Planes are slices of one backing array; appending must not reach into the next channel.
	b.Data[0] = append(b.Data[0], 999)
	for _, s := range b.Data[1] {
		if s == 999 {
			t.Fatal("append to channel 0 wrote into channel 1")
		}
	}
}

func TestBufferValidate(t *testing.T) {
	b := mustBuffer(t, Format{SampleRate: 44100, Channels: 2}, 4)
	if err := b.Validate(); err != nil {
		t.Fatalf("fresh buffer invalid: %v", err)
	}
	b.Data[1] = b.Data[1][:2]
	if err := b.Validate(); err == nil {
		t.Error("Validate accepted ragged channels")
	}
}

func BenchmarkToInt16(b *testing.B) {
	buf, _ := NewBuffer(Format{SampleRate: 48000, Channels: 2}, 4096)
	dst := make([]int16, 2*4096)
	b.ReportAllocs()
	for range b.N {
		_ = ToInt16(buf, dst)
	}
}
