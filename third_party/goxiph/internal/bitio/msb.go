package bitio

import "fmt"

// MSBReader reads fields most significant bit first, the FLAC convention.
//
// Within a byte, bit 7 is read first. A field spanning bytes takes its high bits from the earlier
// byte, which is the opposite of LSBReader in every respect; the two are separate types so that
// neither pays for a direction test in its inner loop.
type MSBReader struct {
	data []byte
	pos  int  // byte offset of the next bit
	bit  uint // bits already consumed from data[pos], 0..7
}

// NewMSBReader reads from data.
func NewMSBReader(data []byte) *MSBReader {
	return &MSBReader{data: data}
}

// Reset points the reader at data, reusing the receiver.
func (r *MSBReader) Reset(data []byte) {
	r.data = data
	r.pos = 0
	r.bit = 0
}

// BitsRead reports how many bits have been consumed.
func (r *MSBReader) BitsRead() int { return r.pos*8 + int(r.bit) }

// BitsLeft reports how many bits remain.
func (r *MSBReader) BitsLeft() int { return len(r.data)*8 - r.BitsRead() }

// ByteAligned reports whether the cursor sits on a byte boundary.
func (r *MSBReader) ByteAligned() bool { return r.bit == 0 }

// Read consumes n bits, 0 <= n <= 64, and returns them right-aligned.
func (r *MSBReader) Read(n uint) (uint64, error) {
	if n > 64 {
		return 0, fmt.Errorf("bitio: read of %d bits, max 64", n)
	}
	if n == 0 {
		return 0, nil
	}
	if int(n) > r.BitsLeft() {
		return 0, ErrEndOfPacket
	}

	var v uint64
	for n > 0 {
		avail := 8 - r.bit
		take := min(n, avail)
		chunk := (uint64(r.data[r.pos]) >> (avail - take)) & (1<<take - 1)
		v = v<<take | chunk
		n -= take
		r.bit += take
		if r.bit == 8 {
			r.bit = 0
			r.pos++
		}
	}
	return v, nil
}

// ReadSigned consumes n bits and sign-extends them from bit n-1.
func (r *MSBReader) ReadSigned(n uint) (int64, error) {
	v, err := r.Read(n)
	if err != nil {
		return 0, err
	}
	if n == 0 || n == 64 {
		return int64(v), nil
	}
	if v&(1<<(n-1)) != 0 {
		v |= ^uint64(0) << n
	}
	return int64(v), nil
}

// ReadBit consumes a single bit.
func (r *MSBReader) ReadBit() (uint64, error) { return r.Read(1) }

// ReadBool consumes a single bit as a flag.
func (r *MSBReader) ReadBool() (bool, error) {
	v, err := r.Read(1)
	return v == 1, err
}

// ReadUnary counts zero bits up to and including the terminating one bit.
//
// FLAC uses this for the high part of a Rice code and for the wasted-bits count. limit bounds the
// count so a run of zeros in corrupt input cannot spin.
func (r *MSBReader) ReadUnary(limit int) (int, error) {
	n := 0
	for {
		b, err := r.Read(1)
		if err != nil {
			return 0, err
		}
		if b == 1 {
			return n, nil
		}
		n++
		if n > limit {
			return 0, fmt.Errorf("bitio: unary run exceeds %d bits", limit)
		}
	}
}

// AlignByte advances to the next byte boundary, discarding the padding bits.
func (r *MSBReader) AlignByte() {
	if r.bit != 0 {
		r.bit = 0
		r.pos++
	}
}

// ReadBytes consumes n whole bytes. The reader must be byte-aligned.
func (r *MSBReader) ReadBytes(n int) ([]byte, error) {
	if r.bit != 0 {
		return nil, fmt.Errorf("bitio: ReadBytes at bit offset %d, must be byte-aligned", r.bit)
	}
	if n < 0 || r.pos+n > len(r.data) {
		return nil, ErrEndOfPacket
	}
	out := r.data[r.pos : r.pos+n]
	r.pos += n
	return out, nil
}

// MSBWriter packs fields most significant bit first.
type MSBWriter struct {
	data []byte
	bit  uint // bits already used in the final byte, 0..7
}

// NewMSBWriter returns a writer with an empty buffer.
func NewMSBWriter() *MSBWriter { return &MSBWriter{} }

// Reset empties the buffer, keeping its capacity.
func (w *MSBWriter) Reset() {
	w.data = w.data[:0]
	w.bit = 0
}

// Bytes returns the packed data. Trailing bits of the final byte are zero.
func (w *MSBWriter) Bytes() []byte { return w.data }

// BitsWritten reports the number of bits packed so far.
func (w *MSBWriter) BitsWritten() int {
	if w.bit == 0 {
		return len(w.data) * 8
	}
	return (len(w.data)-1)*8 + int(w.bit)
}

// Write packs the low n bits of v, 0 <= n <= 64.
func (w *MSBWriter) Write(v uint64, n uint) error {
	if n > 64 {
		return fmt.Errorf("bitio: write of %d bits, max 64", n)
	}
	if n < 64 {
		v &= 1<<n - 1
	}
	for n > 0 {
		if w.bit == 0 {
			w.data = append(w.data, 0)
		}
		space := 8 - w.bit
		take := min(n, space)
		chunk := byte(v>>(n-take)) & byte(1<<take-1)
		w.data[len(w.data)-1] |= chunk << (space - take)
		n -= take
		w.bit = (w.bit + take) % 8
	}
	return nil
}

// WriteUnary packs n zero bits followed by a one bit.
func (w *MSBWriter) WriteUnary(n int) error {
	for range n {
		if err := w.Write(0, 1); err != nil {
			return err
		}
	}
	return w.Write(1, 1)
}

// AlignByte pads with zero bits to the next byte boundary.
func (w *MSBWriter) AlignByte() {
	if w.bit != 0 {
		w.bit = 0
	}
}
