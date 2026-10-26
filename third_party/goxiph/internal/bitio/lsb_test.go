package bitio

import (
	"errors"
	"math/rand/v2"
	"testing"
)

// TestLSBOrderAgainstSpecExample pins the packing direction. The Vorbis specification packs bit 0
// of a byte first, so 0x0A read as two 4-bit fields yields 10 then 0, not 0 then 10.
func TestLSBOrderAgainstSpecExample(t *testing.T) {
	r := NewLSBReader([]byte{0x0A})

	first, err := r.Read(4)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	second, err := r.Read(4)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if first != 10 || second != 0 {
		t.Errorf("got %d then %d, want 10 then 0 (bits are packed low first)", first, second)
	}
}

// TestFieldSpanningBytes checks that the earlier byte supplies the low bits.
func TestFieldSpanningBytes(t *testing.T) {
	// 0xCD 0xAB read as one 16-bit field is 0xABCD: the first byte is least significant.
	r := NewLSBReader([]byte{0xCD, 0xAB})
	v, err := r.Read(16)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if v != 0xABCD {
		t.Errorf("got %#04x, want 0xabcd", v)
	}
}

func TestReadWidths(t *testing.T) {
	for n := uint(1); n <= 32; n++ {
		w := NewLSBWriter()
		want := uint32(0xDEADBEEF)
		if n < 32 {
			want &= 1<<n - 1
		}
		if err := w.Write(want, n); err != nil {
			t.Fatalf("n=%d: Write: %v", n, err)
		}
		got, err := NewLSBReader(w.Bytes()).Read(n)
		if err != nil {
			t.Fatalf("n=%d: Read: %v", n, err)
		}
		if got != want {
			t.Errorf("n=%d: got %#x, want %#x", n, got, want)
		}
	}
}

// TestRoundTripRandomFields is the property that matters: any sequence of widths survives a write
// followed by a read.
func TestRoundTripRandomFields(t *testing.T) {
	rng := rand.New(rand.NewPCG(42, 7))
	type field struct {
		v uint32
		n uint
	}

	for range 200 {
		fields := make([]field, rng.IntN(60)+1)
		w := NewLSBWriter()
		for i := range fields {
			n := uint(rng.IntN(32) + 1)
			v := rng.Uint32()
			if n < 32 {
				v &= 1<<n - 1
			}
			fields[i] = field{v, n}
			if err := w.Write(v, n); err != nil {
				t.Fatalf("Write: %v", err)
			}
		}

		r := NewLSBReader(w.Bytes())
		for i, f := range fields {
			got, err := r.Read(f.n)
			if err != nil {
				t.Fatalf("field %d (%d bits): %v", i, f.n, err)
			}
			if got != f.v {
				t.Fatalf("field %d (%d bits): got %#x, want %#x", i, f.n, got, f.v)
			}
		}
	}
}

func TestLookDoesNotConsume(t *testing.T) {
	r := NewLSBReader([]byte{0xFF, 0x00, 0xFF})
	first, err := r.Look(12)
	if err != nil {
		t.Fatalf("Look: %v", err)
	}
	if r.BitsRead() != 0 {
		t.Errorf("Look consumed %d bits", r.BitsRead())
	}
	second, _ := r.Look(12)
	if first != second {
		t.Error("two Looks returned different values")
	}
	got, _ := r.Read(12)
	if got != first {
		t.Errorf("Read returned %#x after Look returned %#x", got, first)
	}
}

func TestReadPastEnd(t *testing.T) {
	r := NewLSBReader([]byte{0xFF})
	if _, err := r.Read(8); err != nil {
		t.Fatalf("Read: %v", err)
	}
	if _, err := r.Read(1); !errors.Is(err, ErrEndOfPacket) {
		t.Errorf("got %v, want ErrEndOfPacket", err)
	}
	if _, err := r.Read(9); !errors.Is(err, ErrEndOfPacket) {
		t.Errorf("oversized read: got %v, want ErrEndOfPacket", err)
	}
}

func TestReadTooWide(t *testing.T) {
	r := NewLSBReader(make([]byte, 8))
	if _, err := r.Read(33); err == nil {
		t.Error("accepted a 33-bit read")
	}
	if err := NewLSBWriter().Write(0, 33); err == nil {
		t.Error("accepted a 33-bit write")
	}
}

// TestLookPartialPadsAtEnd covers the Huffman case: a code near the end of a packet is shorter than
// the lookahead window, which is ordinary rather than an error.
func TestLookPartialPadsAtEnd(t *testing.T) {
	r := NewLSBReader([]byte{0x0F})
	if err := r.Skip(4); err != nil {
		t.Fatalf("Skip: %v", err)
	}

	v, valid := r.LookPartial(16)
	if valid != 4 {
		t.Errorf("valid = %d, want 4", valid)
	}
	if v != 0 {
		t.Errorf("v = %#x, want 0 (the high nibble of 0x0f)", v)
	}
	if r.BitsRead() != 4 {
		t.Error("LookPartial consumed bits")
	}
}

func TestLookPartialOnEmpty(t *testing.T) {
	r := NewLSBReader(nil)
	v, valid := r.LookPartial(8)
	if v != 0 || valid != 0 {
		t.Errorf("got (%#x, %d), want (0, 0)", v, valid)
	}
}

func TestReadSigned(t *testing.T) {
	cases := []struct {
		bits uint
		raw  uint32
		want int32
	}{
		{4, 0b0111, 7},
		{4, 0b1000, -8},
		{4, 0b1111, -1},
		{8, 0xFF, -1},
		{8, 0x80, -128},
		{8, 0x7F, 127},
		{1, 1, -1},
		{1, 0, 0},
		{32, 0xFFFFFFFF, -1},
	}
	for _, c := range cases {
		w := NewLSBWriter()
		if err := w.Write(c.raw, c.bits); err != nil {
			t.Fatalf("Write: %v", err)
		}
		got, err := NewLSBReader(w.Bytes()).ReadSigned(c.bits)
		if err != nil {
			t.Fatalf("ReadSigned: %v", err)
		}
		if got != c.want {
			t.Errorf("%d bits of %#x: got %d, want %d", c.bits, c.raw, got, c.want)
		}
	}
}

func TestSkipAndAlign(t *testing.T) {
	r := NewLSBReader([]byte{0xAA, 0xBB, 0xCC})
	if err := r.Skip(3); err != nil {
		t.Fatalf("Skip: %v", err)
	}
	r.AlignByte()
	if r.BitsRead() != 8 {
		t.Errorf("after align, BitsRead = %d, want 8", r.BitsRead())
	}
	r.AlignByte()
	if r.BitsRead() != 8 {
		t.Error("AlignByte moved an already-aligned reader")
	}

	got, err := r.Read(8)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if got != 0xBB {
		t.Errorf("got %#x, want 0xbb", got)
	}
}

func TestSkipPastEnd(t *testing.T) {
	r := NewLSBReader([]byte{0x00})
	if err := r.Skip(9); !errors.Is(err, ErrEndOfPacket) {
		t.Errorf("got %v, want ErrEndOfPacket", err)
	}
	if r.BitsLeft() != 0 {
		t.Errorf("BitsLeft = %d after overrun, want 0", r.BitsLeft())
	}
}

func TestReadBytes(t *testing.T) {
	r := NewLSBReader([]byte{1, 2, 3, 4})
	got, err := r.ReadBytes(2)
	if err != nil {
		t.Fatalf("ReadBytes: %v", err)
	}
	if len(got) != 2 || got[0] != 1 || got[1] != 2 {
		t.Errorf("got %v, want [1 2]", got)
	}
	if _, err := r.ReadBytes(3); !errors.Is(err, ErrEndOfPacket) {
		t.Errorf("got %v, want ErrEndOfPacket", err)
	}

	r2 := NewLSBReader([]byte{1, 2})
	_ = r2.Skip(1)
	if _, err := r2.ReadBytes(1); err == nil {
		t.Error("ReadBytes succeeded while not byte-aligned")
	}
}

func TestBitsLeftAndRead(t *testing.T) {
	r := NewLSBReader(make([]byte, 4))
	if r.BitsLeft() != 32 {
		t.Errorf("BitsLeft = %d, want 32", r.BitsLeft())
	}
	_ = r.Skip(5)
	if r.BitsRead() != 5 || r.BitsLeft() != 27 {
		t.Errorf("after 5 bits: read %d, left %d", r.BitsRead(), r.BitsLeft())
	}
}

func TestWriterBytesAndReset(t *testing.T) {
	w := NewLSBWriter()
	_ = w.Write(0b101, 3)
	if w.BitsWritten() != 3 {
		t.Errorf("BitsWritten = %d, want 3", w.BitsWritten())
	}
	if len(w.Bytes()) != 1 || w.Bytes()[0] != 0b101 {
		t.Errorf("got %08b, want 00000101", w.Bytes()[0])
	}

	w.Reset()
	if w.BitsWritten() != 0 || len(w.Bytes()) != 0 {
		t.Error("Reset left state behind")
	}
}

func TestWriteBytesRequiresAlignment(t *testing.T) {
	w := NewLSBWriter()
	if err := w.WriteBytes([]byte{1, 2}); err != nil {
		t.Fatalf("WriteBytes: %v", err)
	}
	_ = w.Write(1, 1)
	if err := w.WriteBytes([]byte{3}); err == nil {
		t.Error("WriteBytes succeeded while not byte-aligned")
	}
}

func TestReaderReset(t *testing.T) {
	r := NewLSBReader([]byte{0xFF})
	_ = r.Skip(4)
	r.Reset([]byte{0x0F, 0xF0})
	if r.BitsRead() != 0 || r.BitsLeft() != 16 {
		t.Errorf("Reset left state behind: read %d, left %d", r.BitsRead(), r.BitsLeft())
	}
}

func FuzzLSBRoundTrip(f *testing.F) {
	f.Add([]byte{0x0A}, uint8(4))
	f.Add([]byte{0xCD, 0xAB}, uint8(16))

	f.Fuzz(func(t *testing.T, data []byte, width uint8) {
		n := uint(width%32) + 1
		r := NewLSBReader(data)
		w := NewLSBWriter()

		for {
			v, err := r.Read(n)
			if err != nil {
				break
			}
			if err := w.Write(v, n); err != nil {
				t.Fatalf("Write: %v", err)
			}
		}
		// Every field that was read must come back identically.
		back := NewLSBReader(w.Bytes())
		check := NewLSBReader(data)
		for {
			want, err := check.Read(n)
			if err != nil {
				break
			}
			got, err := back.Read(n)
			if err != nil {
				t.Fatalf("re-read failed after %d bits: %v", back.BitsRead(), err)
			}
			if got != want {
				t.Fatalf("got %#x, want %#x", got, want)
			}
		}
	})
}

func BenchmarkLSBRead(b *testing.B) {
	data := make([]byte, 4096)
	for i := range data {
		data[i] = byte(i)
	}
	r := NewLSBReader(data)
	b.ReportAllocs()
	b.SetBytes(int64(len(data)))
	for range b.N {
		r.Reset(data)
		for {
			if _, err := r.Read(5); err != nil {
				break
			}
		}
	}
}
