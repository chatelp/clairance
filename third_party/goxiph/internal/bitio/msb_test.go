package bitio

import (
	"errors"
	"math/rand/v2"
	"testing"
)

// TestMSBOrderIsOppositeToLSB pins the direction. The same byte read four bits at a time gives the
// nibbles in the other order from LSBReader, which is the whole reason the two are separate types.
func TestMSBOrderIsOppositeToLSB(t *testing.T) {
	const b = 0x0A // 0000 1010

	msb := NewMSBReader([]byte{b})
	first, _ := msb.Read(4)
	second, _ := msb.Read(4)
	if first != 0 || second != 10 {
		t.Errorf("MSB gave %d then %d, want 0 then 10", first, second)
	}

	lsb := NewLSBReader([]byte{b})
	lf, _ := lsb.Read(4)
	ls, _ := lsb.Read(4)
	if lf != 10 || ls != 0 {
		t.Errorf("LSB gave %d then %d, want 10 then 0", lf, ls)
	}
}

// TestMSBFieldSpanningBytes checks that the earlier byte supplies the high bits.
func TestMSBFieldSpanningBytes(t *testing.T) {
	r := NewMSBReader([]byte{0xAB, 0xCD})
	v, err := r.Read(16)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if v != 0xABCD {
		t.Errorf("got %#04x, want 0xabcd", v)
	}
}

func TestMSBRoundTripRandomFields(t *testing.T) {
	rng := rand.New(rand.NewPCG(31, 37))
	type field struct {
		v uint64
		n uint
	}

	for range 200 {
		fields := make([]field, rng.IntN(60)+1)
		w := NewMSBWriter()
		for i := range fields {
			n := uint(rng.IntN(64) + 1)
			v := rng.Uint64()
			if n < 64 {
				v &= 1<<n - 1
			}
			fields[i] = field{v, n}
			if err := w.Write(v, n); err != nil {
				t.Fatalf("Write: %v", err)
			}
		}

		r := NewMSBReader(w.Bytes())
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

func TestMSBReadSigned(t *testing.T) {
	cases := []struct {
		bits uint
		raw  uint64
		want int64
	}{
		{4, 0b0111, 7},
		{4, 0b1000, -8},
		{4, 0b1111, -1},
		{5, 0b11111, -1}, // the prediction shift field is s(5)
		{5, 0b01111, 15},
		{16, 0xFFFF, -1},
		{16, 0x8000, -32768},
		{32, 0xFFFFFFFF, -1},
		{1, 1, -1},
	}
	for _, c := range cases {
		w := NewMSBWriter()
		if err := w.Write(c.raw, c.bits); err != nil {
			t.Fatalf("Write: %v", err)
		}
		got, err := NewMSBReader(w.Bytes()).ReadSigned(c.bits)
		if err != nil {
			t.Fatalf("ReadSigned: %v", err)
		}
		if got != c.want {
			t.Errorf("%d bits of %#x: got %d, want %d", c.bits, c.raw, got, c.want)
		}
	}
}

// TestReadUnary covers the Rice code's high part and the wasted-bits count.
func TestReadUnary(t *testing.T) {
	for _, n := range []int{0, 1, 2, 7, 8, 9, 31, 100} {
		w := NewMSBWriter()
		if err := w.WriteUnary(n); err != nil {
			t.Fatalf("WriteUnary: %v", err)
		}
		got, err := NewMSBReader(w.Bytes()).ReadUnary(1000)
		if err != nil {
			t.Fatalf("ReadUnary(%d): %v", n, err)
		}
		if got != n {
			t.Errorf("got %d, want %d", got, n)
		}
	}
}

// TestReadUnaryStopsRunaway guards against a long zero run in corrupt input.
func TestReadUnaryStopsRunaway(t *testing.T) {
	r := NewMSBReader(make([]byte, 1024)) // all zeros, no terminator
	if _, err := r.ReadUnary(64); err == nil {
		t.Error("ReadUnary accepted a run past its limit")
	}
}

func TestReadUnaryHitsEndOfData(t *testing.T) {
	r := NewMSBReader([]byte{0x00})
	if _, err := r.ReadUnary(1000); !errors.Is(err, ErrEndOfPacket) {
		t.Errorf("got %v, want ErrEndOfPacket", err)
	}
}

func TestMSBReadPastEnd(t *testing.T) {
	r := NewMSBReader([]byte{0xFF})
	if _, err := r.Read(8); err != nil {
		t.Fatalf("Read: %v", err)
	}
	if _, err := r.Read(1); !errors.Is(err, ErrEndOfPacket) {
		t.Errorf("got %v, want ErrEndOfPacket", err)
	}
	if _, err := r.Read(65); err == nil {
		t.Error("accepted a 65-bit read")
	}
}

func TestMSBAlignAndBytes(t *testing.T) {
	r := NewMSBReader([]byte{0xAA, 0xBB, 0xCC})
	if !r.ByteAligned() {
		t.Error("a fresh reader is not byte-aligned")
	}
	_, _ = r.Read(3)
	if r.ByteAligned() {
		t.Error("reader reports aligned after 3 bits")
	}
	r.AlignByte()
	if r.BitsRead() != 8 {
		t.Errorf("BitsRead = %d, want 8", r.BitsRead())
	}

	got, err := r.ReadBytes(2)
	if err != nil {
		t.Fatalf("ReadBytes: %v", err)
	}
	if got[0] != 0xBB || got[1] != 0xCC {
		t.Errorf("got %v, want [bb cc]", got)
	}

	r2 := NewMSBReader([]byte{1, 2})
	_, _ = r2.Read(1)
	if _, err := r2.ReadBytes(1); err == nil {
		t.Error("ReadBytes succeeded while not byte-aligned")
	}
}

func TestMSBWriterBytes(t *testing.T) {
	w := NewMSBWriter()
	_ = w.Write(0b101, 3)
	if w.BitsWritten() != 3 {
		t.Errorf("BitsWritten = %d, want 3", w.BitsWritten())
	}
	// Packed from the top of the byte, not the bottom.
	if w.Bytes()[0] != 0b10100000 {
		t.Errorf("got %08b, want 10100000", w.Bytes()[0])
	}
}

func FuzzMSBRoundTrip(f *testing.F) {
	f.Add([]byte{0xAB, 0xCD}, uint8(16))
	f.Add([]byte{0x0A}, uint8(4))

	f.Fuzz(func(t *testing.T, data []byte, width uint8) {
		n := uint(width%64) + 1
		r := NewMSBReader(data)
		w := NewMSBWriter()
		for {
			v, err := r.Read(n)
			if err != nil {
				break
			}
			if err := w.Write(v, n); err != nil {
				t.Fatalf("Write: %v", err)
			}
		}
		back := NewMSBReader(w.Bytes())
		check := NewMSBReader(data)
		for {
			want, err := check.Read(n)
			if err != nil {
				break
			}
			got, err := back.Read(n)
			if err != nil {
				t.Fatalf("re-read failed: %v", err)
			}
			if got != want {
				t.Fatalf("got %#x, want %#x", got, want)
			}
		}
	})
}

func BenchmarkMSBRead(b *testing.B) {
	data := make([]byte, 4096)
	for i := range data {
		data[i] = byte(i)
	}
	r := NewMSBReader(data)
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
