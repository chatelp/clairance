package mp3

// Clairance's tests of the live reader (live.go), written to decide whether
// this decoder can replace ffmpeg for the station's streams (docs-fr/35-plan.md,
// B1; Q60). The recordings in testdata/clairance are synthetic, made once with
// ffmpeg and LAME, each with ffmpeg's own decoding beside it (.ref.s16le), so
// the comparison needs no ffmpeg when the tests run. station-8k-vbr.mp3 has the
// station's shape: MPEG-2.5, 8 kHz, mono, variable bitrate.

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"testing/iotest"
	"time"

	"github.com/yegors/co-atc/third_party/goxiph/audio"
)

var fixtures = []string{"station-8k-vbr", "mpeg2-16k-cbr", "liveatc-22k-tagged", "mpeg1-44k-stereo", "silence-8k"}

func fixture(t testing.TB, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "clairance", name+".mp3"))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// reference is ffmpeg's decoding of a fixture, mono, at its own rate.
func reference(t testing.TB, name string) []float64 {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "clairance", name+".ref.s16le"))
	if err != nil {
		t.Fatal(err)
	}
	out := make([]float64, len(b)/2)
	for i := range out {
		out[i] = float64(int16(binary.LittleEndian.Uint16(b[2*i:]))) / 32768
	}
	return out
}

// decodeLive decodes everything r gives, mono, and returns how it ended.
func decodeLive(r io.Reader) (pcm []float64, rate int, fr *FrameReader, end error) {
	d, fr, err := NewLiveDecoder(r)
	if err != nil {
		return nil, 0, fr, err
	}
	buf, err := audio.NewBuffer(d.Format(), 0)
	if err != nil {
		return nil, 0, fr, err
	}
	for {
		if err := d.Next(buf); err != nil {
			return pcm, d.Format().SampleRate, fr, err
		}
		for i := range buf.Data[0] {
			var v float64
			for c := range buf.Data {
				v += float64(buf.Data[c][i])
			}
			pcm = append(pcm, v/float64(len(buf.Data)))
		}
	}
}

// compare aligns test on ref (within maxLag samples, found on the loudest two
// seconds of ref) and returns the signal-to-noise ratio of their difference
// over the overlap, in dB, and the lag. Silence on both sides compares as
// identical.
func compare(ref, test []float64, maxLag int) (snr float64, lag int) {
	seg := min(16000, len(ref))
	from, loud := 0, -1.0
	for s := 0; s+seg <= len(ref); s += seg / 4 {
		var e float64
		for i := s; i < s+seg; i++ {
			e += ref[i] * ref[i]
		}
		if e > loud {
			from, loud = s, e
		}
	}
	best, top := 0, math.Inf(-1)
	for l := -maxLag; l <= maxLag; l++ {
		var c, n float64
		for i := from; i < from+seg; i++ {
			j := i + l
			if j < 0 || j >= len(test) {
				continue
			}
			c += ref[i] * test[j]
			n += test[j] * test[j]
		}
		if n > 0 && c/math.Sqrt(n) > top {
			best, top = l, c/math.Sqrt(n)
		}
	}
	var sig, noise float64
	for i := range ref {
		j := i + best
		if j < 0 || j >= len(test) {
			continue
		}
		d := ref[i] - test[j]
		sig += ref[i] * ref[i]
		noise += d * d
	}
	if noise == 0 {
		return math.Inf(1), best
	}
	if sig == 0 {
		return -10 * math.Log10(noise), best
	}
	return 10 * math.Log10(sig/noise), best
}

// as16bit rounds samples as ffmpeg writes them in 16 bits, clipped.
func as16bit(x []float64) []float64 {
	out := make([]float64, len(x))
	for i, v := range x {
		q := math.Round(v * 32768)
		out[i] = math.Max(-32768, math.Min(32767, q)) / 32768
	}
	return out
}

// window is the part of x from a to b seconds.
func window(x []float64, rate int, a, b float64) []float64 {
	i, j := int(a*float64(rate)), int(b*float64(rate))
	if j > len(x) {
		j = len(x)
	}
	if i > j {
		i = j
	}
	return x[i:j]
}

func ended(err error) bool { return errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) }

// Each fixture decodes as ffmpeg decodes it, to within rounding: 90 dB is
// below what 16-bit audio can carry; B1 measured about 100 dB on the station's
// own streams.
func TestLiveDecodesLikeFFmpeg(t *testing.T) {
	for _, name := range fixtures {
		t.Run(name, func(t *testing.T) {
			pcm, rate, fr, err := decodeLive(bytes.NewReader(fixture(t, name)))
			if !ended(err) {
				t.Fatalf("ended with %v", err)
			}
			ref := reference(t, name)
			// Length: ffmpeg trims the encoder's priming when a tag says how
			// much; a live stream has no tag. Within two frames of 1152.
			if d := len(pcm) - len(ref); d < -2304 || d > 2304 {
				t.Errorf("%d samples, ffmpeg %d", len(pcm), len(ref))
			}
			snr, lag := compare(ref, as16bit(pcm), 2400)
			// Stereo is a known gap: on joint stereo the left channel is
			// 52 dB from ffmpeg, LAME and Apple, which agree among themselves
			// at 79 to 85 dB (measured 26/10, ../README.md). Inaudible, and the
			// station is mono; held here so that it cannot get worse unseen.
			want := 90.0
			if strings.Contains(name, "stereo") {
				want = 50
			}
			if snr < want {
				t.Errorf("SNR %.1f dB at lag %d, want %.0f dB or more", snr, lag, want)
			}
			t.Logf("%d Hz, %d samples, SNR %.1f dB at lag %+d, %d bytes skipped, %d resyncs", rate, len(pcm), snr, lag, fr.Skipped, fr.Resyncs)
		})
	}
}

// The live reader decodes exactly what the whole-file reader does, on a
// stream without a tag: the same decoder, fed frame by frame.
func TestLiveEqualsWholeFile(t *testing.T) {
	b := fixture(t, "station-8k-vbr")
	live, _, _, _ := decodeLive(bytes.NewReader(b))
	d, err := NewDecoder(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	buf, _ := audio.NewBuffer(d.Format(), 0)
	var whole []float64
	for d.Next(buf) == nil {
		for _, v := range buf.Data[0] {
			whole = append(whole, float64(v))
		}
	}
	if len(live) != len(whole) {
		t.Fatalf("live %d samples, whole file %d", len(live), len(whole))
	}
	for i := range live {
		if live[i] != whole[i] {
			t.Fatalf("sample %d: live %v, whole file %v", i, live[i], whole[i])
		}
	}
}

// A radio stream is joined mid-flight, at any byte. The reader finds the next
// whole frame; after a few frames (the bit reservoir and the filterbank fill
// again) the sound is the stream's own.
func TestJoinedAtAnyByte(t *testing.T) {
	b := fixture(t, "station-8k-vbr")
	full, rate, _, _ := decodeLive(bytes.NewReader(b))
	for _, cut := range []int{1, 3, 4, 50, 97, 143, 500, 1001, 5000} {
		pcm, _, fr, err := decodeLive(bytes.NewReader(b[cut:]))
		if !ended(err) {
			t.Errorf("joined at byte %d: ended with %v", cut, err)
			continue
		}
		// From one second after the join: the same sound as from the start.
		snr, _ := compare(window(full, rate, float64(cut)/2000+2, 18), pcm, 4*rate)
		if snr < 90 {
			t.Errorf("joined at byte %d: SNR %.1f dB after the first seconds, want 90", cut, snr)
		}
		if fr.Skipped >= 1000 {
			t.Errorf("joined at byte %d: %d bytes skipped to find a frame", cut, fr.Skipped)
		}
	}
}

// Bytes lost or damaged in transit: the reader resyncs on the next whole
// frame and keeps going. What is far from the damage is untouched, and the
// stream does not end.
func TestDamageMidStream(t *testing.T) {
	b := fixture(t, "station-8k-vbr")
	full, rate, _, _ := decodeLive(bytes.NewReader(b))
	mid := len(b) / 2
	rng := rand.New(rand.NewSource(26))
	garbage := make([]byte, 300)
	rng.Read(garbage)
	cases := map[string][]byte{
		"20 bytes flipped": func() []byte {
			c := append([]byte(nil), b...)
			for i := 0; i < 20; i++ {
				c[mid+rng.Intn(2000)] ^= byte(1 + rng.Intn(255))
			}
			return c
		}(),
		"500 bytes zeroed": func() []byte {
			c := append([]byte(nil), b...)
			for i := mid; i < mid+500; i++ {
				c[i] = 0
			}
			return c
		}(),
		"300 bytes inserted": append(append(append([]byte(nil), b[:mid]...), garbage...), b[mid:]...),
		"100 bytes dropped":  append(append([]byte(nil), b[:mid]...), b[mid+100:]...),
		"a frame's sync word broken": func() []byte {
			c := append([]byte(nil), b...)
			for i := mid; i < len(c)-1; i++ {
				if c[i] == 0xFF && c[i+1]&0xE0 == 0xE0 {
					c[i] = 0x00
					break
				}
			}
			return c
		}(),
	}
	for name, damaged := range cases {
		t.Run(name, func(t *testing.T) {
			pcm, _, fr, err := decodeLive(bytes.NewReader(damaged))
			if !ended(err) {
				t.Fatalf("ended with %v before the end of the stream", err)
			}
			if float64(len(pcm)) < 0.97*float64(len(full)) {
				t.Errorf("%d samples, %d undamaged: more than 3%% lost", len(pcm), len(full))
			}
			// The first five seconds, before the damage, and the last five,
			// well after it, sound as the undamaged stream does.
			if snr, _ := compare(window(full, rate, 0.5, 5), window(pcm, rate, 0, 6), rate); snr < 90 {
				t.Errorf("before the damage: SNR %.1f dB", snr)
			}
			if snr, _ := compare(window(full, rate, 15, 19), window(pcm, rate, 13, 20), 2*rate); snr < 90 {
				t.Errorf("after the damage: SNR %.1f dB", snr)
			}
			t.Logf("%d samples (undamaged %d), %d resyncs, %d bytes skipped", len(pcm), len(full), fr.Resyncs, fr.Skipped)
		})
	}
}

// A stream cut in the middle of a frame (a dropped connection) ends with
// io.ErrUnexpectedEOF, after everything before the cut.
func TestCutMidFrame(t *testing.T) {
	b := fixture(t, "station-8k-vbr")
	full, rate, _, _ := decodeLive(bytes.NewReader(b))
	pcm, _, _, err := decodeLive(bytes.NewReader(b[:len(b)/2+37]))
	if !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		t.Fatalf("ended with %v", err)
	}
	if snr, _ := compare(window(full, rate, 0.5, 9), window(pcm, rate, 0.5, 9), 100); snr < 90 {
		t.Errorf("what came before the cut: SNR %.1f dB", snr)
	}
}

// What is not an MP3 stream is refused, in bounded time and memory: random
// bytes, an HTML error page, an empty answer.
func TestNotAStream(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	random := make([]byte, 200<<10)
	rng.Read(random)
	for name, b := range map[string][]byte{
		"random bytes": random,
		"an HTML page": []byte(strings.Repeat("<html><body>404 Not Found</body></html>\n", 4000)),
		"nothing":      nil,
	} {
		start := time.Now()
		_, _, _, err := decodeLive(bytes.NewReader(b))
		if err == nil || (!errors.Is(err, ErrNoSync) && !ended(err)) {
			t.Errorf("%s: %v, want no sync or the end", name, err)
		}
		if d := time.Since(start); d > 5*time.Second {
			t.Errorf("%s: %v to refuse", name, d)
		}
	}
}

// A stream that changes shape mid-way (another rate) stops with
// ErrFormatChanged, after the audio of the first, so that the caller starts
// a new decoder rather than decode one format as another.
func TestFormatChange(t *testing.T) {
	a, b := fixture(t, "station-8k-vbr"), fixture(t, "mpeg2-16k-cbr")
	full, _, _, _ := decodeLive(bytes.NewReader(a))
	pcm, _, _, err := decodeLive(io.MultiReader(bytes.NewReader(a), bytes.NewReader(b)))
	if !errors.Is(err, ErrFormatChanged) {
		t.Fatalf("ended with %v, want ErrFormatChanged", err)
	}
	if len(pcm) < len(full)-1152 {
		t.Errorf("%d samples before the change, the first stream has %d", len(pcm), len(full))
	}
}

// However the network hands the bytes over -- one at a time, in halves, in
// random chunks -- the sound is the same.
func TestAnyChunking(t *testing.T) {
	b := fixture(t, "station-8k-vbr")
	want, _, _, _ := decodeLive(bytes.NewReader(b))
	rng := rand.New(rand.NewSource(7))
	readers := map[string]io.Reader{
		"one byte at a time": iotest.OneByteReader(bytes.NewReader(b)),
		"halves":             iotest.HalfReader(bytes.NewReader(b)),
		"random chunks":      &chunked{b: b, rng: rng},
	}
	for name, r := range readers {
		got, _, _, err := decodeLive(r)
		if !ended(err) {
			t.Errorf("%s: ended with %v", name, err)
			continue
		}
		if len(got) != len(want) {
			t.Errorf("%s: %d samples, want %d", name, len(got), len(want))
			continue
		}
		for i := range got {
			if got[i] != want[i] {
				t.Errorf("%s: sample %d differs", name, i)
				break
			}
		}
	}
}

type chunked struct {
	b   []byte
	rng *rand.Rand
}

func (c *chunked) Read(p []byte) (int, error) {
	if len(c.b) == 0 {
		return 0, io.EOF
	}
	n := 1 + c.rng.Intn(700)
	n = min(n, len(p), len(c.b))
	copy(p, c.b[:n])
	c.b = c.b[n:]
	return n, nil
}

// An hour of the station's stream, without keeping the sound: memory stays
// flat, and decoding takes a small fraction of real time.
func TestAnHourOfStream(t *testing.T) {
	if testing.Short() {
		t.Skip("an hour of audio")
	}
	b := fixture(t, "station-8k-vbr")
	// 180 loops of 20 s; the loop seams are format-identical, which the
	// reader takes as one stream.
	r := &looped{b: b, left: 180}
	d, _, err := NewLiveDecoder(r)
	if err != nil {
		t.Fatal(err)
	}
	buf, _ := audio.NewBuffer(d.Format(), 0)
	var before, after runtime.MemStats
	samples := 0
	start := time.Now()
	for i := 0; ; i++ {
		if i == 2000 {
			runtime.GC()
			runtime.ReadMemStats(&before)
		}
		if err := d.Next(buf); err != nil {
			if !ended(err) {
				t.Fatal(err)
			}
			break
		}
		samples += len(buf.Data[0])
	}
	elapsed := time.Since(start)
	runtime.GC()
	runtime.ReadMemStats(&after)
	seconds := float64(samples) / 8000
	if seconds < 3500 {
		t.Fatalf("%.0f s decoded, want an hour", seconds)
	}
	growth := int64(after.HeapAlloc) - int64(before.HeapAlloc)
	if growth > 4<<20 {
		t.Errorf("heap grew by %d KB over the hour", growth>>10)
	}
	if elapsed > time.Duration(seconds/100*float64(time.Second)) {
		t.Errorf("%v for %.0f s of audio: slower than 1/100 of real time", elapsed, seconds)
	}
	t.Logf("%.0f s of audio in %v (%.0f× real time), heap %+d KB", seconds, elapsed.Round(time.Millisecond), seconds/elapsed.Seconds(), growth>>10)
}

type looped struct {
	b    []byte
	at   int
	left int
}

func (l *looped) Read(p []byte) (int, error) {
	if l.at == len(l.b) {
		if l.left--; l.left <= 0 {
			return 0, io.EOF
		}
		l.at = 0
	}
	n := copy(p, l.b[l.at:])
	l.at += n
	return n, nil
}

// Whatever bytes arrive, the decoder neither panics nor runs away: it ends
// with an error or at the end of the input.
func FuzzLiveDecoder(f *testing.F) {
	for _, name := range fixtures {
		b := fixture(f, name)
		f.Add(b)
		f.Add(b[len(b)/3 : len(b)/2])
	}
	f.Fuzz(func(t *testing.T, b []byte) {
		d, _, err := NewLiveDecoder(bytes.NewReader(b))
		if err != nil {
			return
		}
		buf, err := audio.NewBuffer(d.Format(), 0)
		if err != nil {
			return
		}
		for i := 0; i < len(b)+10; i++ {
			if d.Next(buf) != nil {
				return
			}
		}
		t.Fatal("more frames than bytes")
	})
}
