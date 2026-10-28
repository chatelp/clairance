package audio

import (
	"math"
	"testing"
)

// ulawDecode is G.711's decoding, the one the page applies (live-player.js).
func ulawDecode(b byte) float64 {
	u := ^b
	sign := u & 0x80
	exp := int(u>>4) & 0x07
	mant := int(u & 0x0F)
	s := ((mant << 3) + 0x84) << exp
	s -= 0x84
	if sign != 0 {
		s = -s
	}
	return float64(s) / 32768
}

// The coding agrees with G.711's reference values, and a sample comes back
// within the step of its segment: about 1 % of its value, 24 dB under it at
// worst, what the telephone has always had.
func TestMuLawFollowsG711(t *testing.T) {
	for _, c := range []struct {
		in   float64
		want byte
	}{{0, 0xFF}, {1, 0x80}, {-1, 0x00}, {0.5, 0x8F}, {-0.5, 0x0F}} {
		if got := ulawOf(c.in); got != c.want {
			t.Errorf("ulaw(%v) = %#x, want %#x", c.in, got, c.want)
		}
	}
	worst := 0.0
	for i := -32000; i <= 32000; i += 37 {
		v := float64(i) / 32768
		back := ulawDecode(ulawOf(v))
		err := math.Abs(back - v)
		if math.Abs(v) > 0.01 {
			worst = math.Max(worst, err/math.Abs(v))
		} else if err > 0.002 {
			t.Fatalf("near zero: %v came back %v", v, back)
		}
	}
	if worst > 0.07 {
		t.Errorf("relative error up to %.3f, want under 7 %%", worst)
	}
	t.Logf("worst relative error %.3f", worst)
}
