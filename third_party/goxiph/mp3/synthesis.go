package mp3

import "math"

// The synthesis filterbank is the last stage and the only one shared by all three layers.
//
// Thirty-two subbands go in and thirty-two samples come out, but not independently: the bank holds
// sixteen blocks of history, so each output sample draws on half a second of subband values through a
// five-hundred-and-twelve-tap window. That is what undoes the analysis filterbank the encoder ran,
// and it is why a decoder cannot start cleanly part way through a stream.

// synthMatrix is the cosine matrix the subbands are spread across, computed once.
//
// Sixty-four outputs from thirty-two inputs, the classic modulation of the prototype window. A
// closed form, so it is calculated rather than carried.
var synthMatrix = func() *[64][32]float64 {
	var m [64][32]float64
	for i := range 64 {
		for k := range 32 {
			m[i][k] = math.Cos(float64((16+i)*(2*k+1)) * math.Pi / 64)
		}
	}
	return &m
}()

// synthesiser holds one channel's delay line.
type synthesiser struct {
	// fifo is sixteen blocks of sixty-four, newest first.
	fifo [1024]float64
}

// reset clears the delay line, which is what starting a stream or seeking into one calls for.
func (s *synthesiser) reset() { s.fifo = [1024]float64{} }

// synthesise turns one time slot of thirty-two subband samples into thirty-two output samples.
func (s *synthesiser) synthesise(sub *[32]float64, out *[32]float64) {
	// Shift the delay line by one block and spread the new subbands across the front of it.
	copy(s.fifo[64:], s.fifo[:len(s.fifo)-64])

	// Half the block comes out of the other half. Of the sixty-four rows only thirty-one need the
	// multiply: row sixteen is zero throughout, row forty-eight is minus one throughout, rows
	// seventeen to thirty-two are the negatives of rows fifteen down to zero, and rows forty-nine to
	// sixty-three repeat rows forty-seven down to thirty-three. That is a property of the cosine and
	// it is checked against the matrix itself in the tests.
	for i := range 16 {
		var v float64
		row := &synthMatrix[i]
		for k := range 32 {
			v += row[k] * sub[k]
		}
		s.fifo[i] = v
		s.fifo[32-i] = -v
	}
	for i := 33; i < 48; i++ {
		var v float64
		row := &synthMatrix[i]
		for k := range 32 {
			v += row[k] * sub[k]
		}
		s.fifo[i] = v
		s.fifo[96-i] = v
	}

	s.fifo[16] = 0
	var total float64
	for k := range 32 {
		total += sub[k]
	}
	s.fifo[48] = -total

	// The window does not run down the delay line in order. Each half of a block is taken from a
	// different depth, which is what interleaves the overlapping transforms back together.
	var u [512]float64
	for i := range 8 {
		for j := range 32 {
			u[i*64+j] = s.fifo[i*128+j]
			u[i*64+32+j] = s.fifo[i*128+96+j]
		}
	}

	for j := range 32 {
		var v float64
		for i := range 16 {
			v += u[j+32*i] * synthWindow[j+32*i]
		}
		out[j] = v
	}
}
