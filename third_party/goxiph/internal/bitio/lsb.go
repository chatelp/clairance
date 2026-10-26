// Package bitio provides the bit-level readers and writers the codecs are built from.
//
// Vorbis and Opus pack the least significant bit first; FLAC packs the most significant bit first.
// The two conventions are separate types rather than one type with a direction flag, because the
// choice sits in the hottest loop in the library and a runtime branch there is not free.
package bitio

import (
	"errors"
	"fmt"
)

// ErrEndOfPacket reports a read that ran past the end of the data.
//
// Vorbis treats this as a normal condition in several places: a truncated final packet is decoded
// as far as it goes rather than rejected, so callers check for it instead of failing outright.
var ErrEndOfPacket = errors.New("bitio: end of packet")

// LSBReader reads fields least significant bit first, the Vorbis and Opus convention.
//
// Within a byte, bit 0 is read first. A field spanning bytes takes its low bits from the earlier
// byte, so the second byte contributes the more significant bits.
type LSBReader struct {
	data []byte
	pos  int  // byte offset of the next bit
	bit  uint // bit offset within data[pos], 0..7
	eop  bool // a read has already run past the end
}

// NewLSBReader reads from data.
func NewLSBReader(data []byte) *LSBReader {
	return &LSBReader{data: data}
}

// Reset points the reader at data, reusing the receiver.
func (r *LSBReader) Reset(data []byte) {
	r.data = data
	r.pos = 0
	r.bit = 0
	r.eop = false
}

// EndOfPacket reports whether a read has run past the end.
func (r *LSBReader) EndOfPacket() bool { return r.eop }

// BitsRead reports how many bits have been consumed.
func (r *LSBReader) BitsRead() int { return r.pos*8 + int(r.bit) }

// BitsLeft reports how many bits remain.
func (r *LSBReader) BitsLeft() int { return len(r.data)*8 - r.BitsRead() }

// Read consumes n bits, 0 <= n <= 32, and returns them right-aligned.
func (r *LSBReader) Read(n uint) (uint32, error) {
	v, err := r.Look(n)
	if err != nil {
		return 0, err
	}
	r.skip(n)
	return v, nil
}

// Look returns the next n bits without consuming them.
func (r *LSBReader) Look(n uint) (uint32, error) {
	if n > 32 {
		return 0, fmt.Errorf("bitio: read of %d bits, max 32", n)
	}
	// Vorbis I 2.1.9: once end-of-packet is set every later read fails, including a zero-width
	// one. Before that, a zero-width read succeeds and does not move the cursor.
	if r.eop {
		return 0, ErrEndOfPacket
	}
	if n == 0 {
		return 0, nil
	}
	if int(n) > r.BitsLeft() {
		r.eop = true
		return 0, ErrEndOfPacket
	}

	var v uint32
	var got uint
	pos, bit := r.pos, r.bit
	for got < n {
		avail := 8 - bit
		take := min(n-got, avail)
		chunk := (uint32(r.data[pos]) >> bit) & (1<<take - 1)
		v |= chunk << got
		got += take
		bit += take
		if bit == 8 {
			bit = 0
			pos++
		}
	}
	return v, nil
}

// LookPartial returns up to n bits without consuming them, zero-padding past the end of the data
// and reporting how many bits were real.
//
// Huffman decoding needs this: a code near the end of a packet is shorter than the lookahead
// window, so a short read there is ordinary rather than an error.
func (r *LSBReader) LookPartial(n uint) (v uint32, valid uint) {
	if n > 32 {
		n = 32
	}
	left := r.BitsLeft()
	if left < 0 {
		left = 0
	}
	valid = n
	if int(valid) > left {
		valid = uint(left)
	}
	v, _ = r.Look(valid)
	return v, valid
}

// ReadBit consumes a single bit.
func (r *LSBReader) ReadBit() (uint32, error) { return r.Read(1) }

// ReadBool consumes a single bit as a flag.
func (r *LSBReader) ReadBool() (bool, error) {
	v, err := r.Read(1)
	return v != 0, err
}

// ReadSigned consumes n bits and sign-extends them from bit n-1.
func (r *LSBReader) ReadSigned(n uint) (int32, error) {
	v, err := r.Read(n)
	if err != nil {
		return 0, err
	}
	if n == 0 || n == 32 {
		return int32(v), nil
	}
	if v&(1<<(n-1)) != 0 {
		v |= ^uint32(0) << n
	}
	return int32(v), nil
}

// Skip advances past n bits.
func (r *LSBReader) Skip(n uint) error {
	if r.eop {
		return ErrEndOfPacket
	}
	if int(n) > r.BitsLeft() {
		r.pos = len(r.data)
		r.bit = 0
		r.eop = true
		return ErrEndOfPacket
	}
	r.skip(n)
	return nil
}

func (r *LSBReader) skip(n uint) {
	total := r.bit + n
	r.pos += int(total / 8)
	r.bit = total % 8
}

// AlignByte advances to the next byte boundary.
func (r *LSBReader) AlignByte() {
	if r.bit != 0 {
		r.bit = 0
		r.pos++
	}
}

// ReadBytes consumes n whole bytes. The reader must be byte-aligned.
func (r *LSBReader) ReadBytes(n int) ([]byte, error) {
	if r.bit != 0 {
		return nil, fmt.Errorf("bitio: ReadBytes at bit offset %d, must be byte-aligned", r.bit)
	}
	if r.eop {
		return nil, ErrEndOfPacket
	}
	if n < 0 || r.pos+n > len(r.data) {
		r.eop = true
		return nil, ErrEndOfPacket
	}
	out := r.data[r.pos : r.pos+n]
	r.pos += n
	return out, nil
}

// LSBWriter packs fields least significant bit first.
type LSBWriter struct {
	data []byte
	bit  uint // bits already used in the final byte, 0..7
}

// NewLSBWriter returns a writer with an empty buffer.
func NewLSBWriter() *LSBWriter { return &LSBWriter{} }

// Reset empties the buffer, keeping its capacity.
func (w *LSBWriter) Reset() {
	w.data = w.data[:0]
	w.bit = 0
}

// BitsWritten reports the number of bits packed so far.
func (w *LSBWriter) BitsWritten() int {
	if w.bit == 0 {
		return len(w.data) * 8
	}
	return (len(w.data)-1)*8 + int(w.bit)
}

// Bytes returns the packed data. Trailing bits of the final byte are zero.
func (w *LSBWriter) Bytes() []byte { return w.data }

// Write packs the low n bits of v, 0 <= n <= 32.
func (w *LSBWriter) Write(v uint32, n uint) error {
	if n > 32 {
		return fmt.Errorf("bitio: write of %d bits, max 32", n)
	}
	if n < 32 {
		v &= 1<<n - 1
	}
	for n > 0 {
		if w.bit == 0 {
			w.data = append(w.data, 0)
		}
		space := 8 - w.bit
		take := min(n, space)
		w.data[len(w.data)-1] |= byte(v&(1<<take-1)) << w.bit
		v >>= take
		n -= take
		w.bit = (w.bit + take) % 8
	}
	return nil
}

// WriteBit packs a single bit.
func (w *LSBWriter) WriteBit(b uint32) error { return w.Write(b, 1) }

// AlignByte pads with zero bits to the next byte boundary.
func (w *LSBWriter) AlignByte() {
	if w.bit != 0 {
		w.bit = 0
	}
}

// WriteBytes appends whole bytes. The writer must be byte-aligned.
func (w *LSBWriter) WriteBytes(p []byte) error {
	if w.bit != 0 {
		return fmt.Errorf("bitio: WriteBytes at bit offset %d, must be byte-aligned", w.bit)
	}
	w.data = append(w.data, p...)
	return nil
}
