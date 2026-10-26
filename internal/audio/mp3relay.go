package audio

import (
	"io"
	"sync"
)

// The station's own MP3, relayed to browsers (docs-fr/35-plan.md, D2 bis).
//
// The page played each frequency as an endless WAV stream, which Safari, on
// the Mac as on the iPhone, refuses: it wants to know a file's length (26/10,
// tested by the owner: sound in Dia, none in Safari). An endless MP3 stream
// is what every web radio sends and every browser plays, and it is the
// station's own audio at about 15 kb/s, where the WAV took 384. The Go
// decoder already reads these frames; the relay hands them on, whole.

// mp3BurstFrames is how much a new listener gets at once, so that its player
// starts at once instead of waiting to fill its buffer: about two seconds of
// the station's 72 ms frames, as Icecast sends a burst at connection.
const mp3BurstFrames = 28

// mp3ClientFrames is how many frames a listener may fall behind before it
// loses some: about eighteen seconds. A listener that slow loses frames; the
// others, and the decoder, never wait for it.
const mp3ClientFrames = 256

type mp3Relay struct {
	mu      sync.Mutex
	burst   [][]byte
	format  [2]int // sample rate and channels of the frames being relayed
	clients map[*mp3Reader]struct{}
}

func newMP3Relay() *mp3Relay {
	return &mp3Relay{clients: map[*mp3Reader]struct{}{}}
}

// setFormat is said at each connection to the stream. Frames of another rate
// or channel count cannot follow the earlier ones in a player: its listeners
// are let go, and come back to the new stream.
func (m *mp3Relay) setFormat(rate, channels int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	f := [2]int{rate, channels}
	if f == m.format {
		return
	}
	m.format = f
	m.burst = nil
	for c := range m.clients {
		c.end()
		delete(m.clients, c)
	}
}

// frame hands one whole frame, header included, to every listener.
func (m *mp3Relay) frame(raw []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.burst = append(m.burst, raw)
	if len(m.burst) > mp3BurstFrames {
		m.burst = m.burst[len(m.burst)-mp3BurstFrames:]
	}
	for c := range m.clients {
		select {
		case c.frames <- raw:
		default: // too far behind: this frame is lost to it
		}
	}
}

// reader is a new listener: the burst first, then the frames as they come.
// ok is false while no frame has been seen.
func (m *mp3Relay) reader() (io.ReadCloser, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.burst) == 0 {
		return nil, false
	}
	c := &mp3Reader{relay: m, frames: make(chan []byte, mp3ClientFrames), done: make(chan struct{})}
	for _, f := range m.burst {
		c.frames <- f
	}
	m.clients[c] = struct{}{}
	return c, true
}

// close lets every listener go: the stream has stopped.
func (m *mp3Relay) close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for c := range m.clients {
		c.end()
		delete(m.clients, c)
	}
	m.burst = nil
	m.format = [2]int{}
}

type mp3Reader struct {
	relay  *mp3Relay
	frames chan []byte
	done   chan struct{}
	once   sync.Once
	rest   []byte
}

func (c *mp3Reader) end() { c.once.Do(func() { close(c.done) }) }

// Read gives the frames in order; io.EOF once the listener is let go and
// every frame it was given has been read.
func (c *mp3Reader) Read(p []byte) (int, error) {
	if len(c.rest) == 0 {
		select {
		case f := <-c.frames:
			c.rest = f
		case <-c.done:
			// Let go: what is already given still comes first.
			select {
			case f := <-c.frames:
				c.rest = f
			default:
				return 0, io.EOF
			}
		}
	}
	n := copy(p, c.rest)
	c.rest = c.rest[n:]
	return n, nil
}

// Close leaves the relay.
func (c *mp3Reader) Close() error {
	c.relay.mu.Lock()
	delete(c.relay.clients, c)
	c.relay.mu.Unlock()
	c.end()
	return nil
}

// MP3Reader is a listener to the stream's own MP3, frame-aligned, while the
// stream is decoded in Go and has given frames; false otherwise (the stream
// goes through ffmpeg, or is not MP3), and the caller serves the WAV stream.
func (p *CentralAudioProcessor) MP3Reader() (io.ReadCloser, bool) {
	if p.mp3 == nil {
		return nil, false
	}
	return p.mp3.reader()
}
