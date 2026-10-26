package audio

import (
	"bytes"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/yegors/co-atc/third_party/goxiph/mp3"
)

func frameOf(b byte) []byte { return []byte{0xFF, 0xE3, b, b} }

// readFrames reads n four-byte frames from r, or fails after a second.
func readFrames(t *testing.T, r io.Reader, n int) []byte {
	t.Helper()
	got := make(chan []byte, 1)
	go func() {
		b := make([]byte, 4*n)
		_, err := io.ReadFull(r, b)
		if err != nil {
			got <- nil
			return
		}
		got <- b
	}()
	select {
	case b := <-got:
		if b == nil {
			t.Fatal("the reader ended")
		}
		return b
	case <-time.After(time.Second):
		t.Fatal("no frames within a second")
	}
	return nil
}

// A new listener gets the last frames at once, then the live ones in order;
// none is offered before the first frame.
func TestTheRelayGivesTheBurstThenTheLiveFrames(t *testing.T) {
	m := newMP3Relay()
	if _, ok := m.reader(); ok {
		t.Fatal("a listener before any frame")
	}
	m.setFormat(8000, 1)
	for i := 0; i < mp3BurstFrames+5; i++ {
		m.frame(frameOf(byte(i)))
	}
	r, ok := m.reader()
	if !ok {
		t.Fatal("no listener after frames")
	}
	burst := readFrames(t, r, mp3BurstFrames)
	if burst[2] != 5 || burst[len(burst)-2] != byte(mp3BurstFrames+4) {
		t.Errorf("burst from frame %d to %d, want the last %d", burst[2], burst[len(burst)-2], mp3BurstFrames)
	}
	m.frame(frameOf(200))
	if live := readFrames(t, r, 1); live[2] != 200 {
		t.Errorf("live frame %d, want 200", live[2])
	}
	r.Close()
}

// A listener that does not read loses frames; the others and the decoder
// never wait for it.
func TestASlowListenerDoesNotHoldTheRelay(t *testing.T) {
	m := newMP3Relay()
	m.setFormat(8000, 1)
	m.frame(frameOf(0))
	slow, _ := m.reader()
	fast, _ := m.reader()
	readFrames(t, fast, 1)
	done := make(chan struct{})
	go func() {
		for i := 0; i < 3*mp3ClientFrames; i++ {
			m.frame(frameOf(byte(i)))
			if i%10 == 0 {
				readFrames(t, fast, 1)
			}
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("the relay waited for a listener that does not read")
	}
	slow.Close()
	fast.Close()
}

// Another format (the station switched to a stream of another rate) lets the
// listeners go, and so does the stream's stop: they come back to the new one.
func TestListenersAreLetGoOnAFormatChangeAndAStop(t *testing.T) {
	m := newMP3Relay()
	m.setFormat(8000, 1)
	m.frame(frameOf(1))
	r, _ := m.reader()
	readFrames(t, r, 1)
	m.setFormat(16000, 1)
	if _, err := r.Read(make([]byte, 4)); !errors.Is(err, io.EOF) {
		t.Errorf("after a format change: %v, want EOF", err)
	}
	m.frame(frameOf(2))
	r2, ok := m.reader()
	if !ok {
		t.Fatal("no listener on the new format")
	}
	readFrames(t, r2, 1)
	m.close()
	if _, err := r2.Read(make([]byte, 4)); !errors.Is(err, io.EOF) {
		t.Errorf("after the stop: %v, want EOF", err)
	}
	if _, ok := m.reader(); ok {
		t.Error("a listener after the stop")
	}
}

// What the relay sends is the stream's own MP3, whole frames from the first
// byte: a player decodes it from the start, as goxiph does here.
func TestTheRelayedMP3DecodesFromItsFirstByte(t *testing.T) {
	b := mp3Fixture(t, "station-8k-vbr")
	m := newMP3Relay()
	var pcm bytes.Buffer
	listener := make(chan io.ReadCloser, 1)
	go func() {
		for {
			if r, ok := m.reader(); ok {
				listener <- r
				return
			}
			time.Sleep(time.Millisecond)
		}
	}()
	// At a stream's pace, in pieces, so that the listener joins early on.
	if err := decodeMP3(&paced{b: b[777:]}, 8000, &pcm, m, nil, quietLog(t)); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("decoding: %v", err)
	}
	r := <-listener
	m.close()
	relayed, _ := io.ReadAll(r)
	if len(relayed) < len(b)/2 || relayed[0] != 0xFF || relayed[1]&0xE0 != 0xE0 {
		t.Fatalf("%d bytes relayed, starting %x", len(relayed), relayed[:min(4, len(relayed))])
	}
	if _, _, err := mp3.NewLiveDecoder(bytes.NewReader(relayed)); err != nil {
		t.Errorf("the relayed bytes do not decode: %v", err)
	}
	fr := mp3.NewFrameReader(bytes.NewReader(relayed))
	if _, err := fr.Header(); err != nil || fr.Skipped != 0 {
		t.Errorf("the relayed stream does not start on a frame: %d bytes skipped, %v", fr.Skipped, err)
	}
}

// paced hands its bytes over 500 at a time, a millisecond apart.
type paced struct{ b []byte }

func (p *paced) Read(q []byte) (int, error) {
	if len(p.b) == 0 {
		return 0, io.EOF
	}
	time.Sleep(time.Millisecond)
	n := copy(q[:min(len(q), 500)], p.b)
	p.b = p.b[n:]
	return n, nil
}
