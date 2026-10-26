package transcription

import (
	"math"
	"testing"
	"time"
)

// A simulated listener. Audio sample k was live at liveOf(k); the listener
// reads it at some later wall-clock time. Dates are checked against liveOf.
type listener struct {
	t     *testing.T
	c     *streamClock
	rate  int
	now   time.Time
	k     int // samples read so far
	live0 time.Time
	gapAt int       // first sample after a reconnection, 0 if none
	live1 time.Time // live time of that sample
	drops []int     // sample index at which the mixer dropped 125 ms, before it
}

const tick = 20 * time.Millisecond

// newListener connects to a stream whose burst of `burst` seconds takes
// `delivery` to arrive. The server sends the burst and then whatever was
// produced meanwhile, so the burst's last sample is live when it arrives.
func newListener(t *testing.T, rate int, burst, delivery time.Duration) *listener {
	now := time.Date(2026, 9, 25, 22, 0, 0, 0, time.UTC)
	return &listener{t: t, c: newStreamClock(rate), rate: rate, now: now, live0: now.Add(delivery - burst)}
}

func (l *listener) liveOf(k int) time.Time {
	base, from := l.live0, 0
	if l.gapAt > 0 && k >= l.gapAt {
		base, from = l.live1, l.gapAt
	}
	d := time.Duration(float64(k-from) / float64(l.rate) * float64(time.Second))
	for _, at := range l.drops {
		if k >= at && at >= from {
			d += 125 * time.Millisecond // audio after a drop was live later than its position says
		}
	}
	return base.Add(d)
}

// read delivers `audio` worth of samples over one tick of wall clock.
func (l *listener) read(audio time.Duration) {
	l.now = l.now.Add(tick)
	n := int(audio.Seconds() * float64(l.rate))
	l.k += n
	l.c.received(n, l.now)
}

func (l *listener) steady(d time.Duration) {
	for i := 0; i < int(d/tick); i++ {
		l.read(tick)
	}
}

// burst delivers `audio` in one second of wall clock.
func (l *listener) burst(audio time.Duration) {
	for i := 0; i < 50; i++ {
		l.read(audio / 50)
	}
}

func (l *listener) check(m leadAt, k int, tol time.Duration, what string) {
	l.t.Helper()
	got, want := l.c.date(m), l.liveOf(k)
	if err := got.Sub(want); time.Duration(math.Abs(float64(err))) > tol {
		l.t.Errorf("%s: dated %s off (got %s, live %s)", what, err, got.Format("15:04:05.000"), want.Format("15:04:05.000"))
	}
}

// The case that matters: a new listener receives a minute of old audio in a
// second. Transmissions in it must be dated when they were on the air, not when
// the connection was made.
func TestAConnectionBurstIsDatedInThePast(t *testing.T) {
	l := newListener(t, 8000, 60*time.Second, 2*time.Second)
	l.burst(15 * time.Second)
	early, kEarly := l.c.mark(l.now), l.k // a transmission cut 15 s into the burst
	if l.c.settled() {
		t.Fatal("in the middle of a burst the clock must not claim to know dates")
	}
	l.burst(45 * time.Second)
	l.steady(3 * time.Second)
	if !l.c.settled() {
		t.Fatal("after the burst the clock should settle")
	}
	l.check(early, kEarly, 100*time.Millisecond, "cut during the burst")
	l.steady(10 * time.Second)
	l.check(l.c.mark(l.now), l.k, 100*time.Millisecond, "cut after the burst")
}

// Without a burst (raw PCM over UDP, a server that sends none) nothing changes:
// arrival is the date.
func TestSteadyDeliveryIsDatedOnArrival(t *testing.T) {
	l := newListener(t, 16000, 0, 0)
	l.steady(5 * time.Second)
	m := l.c.mark(l.now)
	if !l.c.settled() {
		t.Fatal("steady delivery should settle")
	}
	if got := l.c.date(m); !got.Equal(m.at) {
		t.Errorf("dated %s, want the arrival %s", got, m.at)
	}
}

// A reconnection brings a new burst, and a new offset from the live signal.
func TestAReconnectionIsDatedFromItsOwnBurst(t *testing.T) {
	l := newListener(t, 8000, 60*time.Second, time.Second)
	l.burst(60 * time.Second)
	l.steady(5 * time.Minute)
	l.now = l.now.Add(7 * time.Second) // the stream drops, and comes back 7 s later
	l.gapAt, l.live1 = l.k, l.now.Add(time.Second-4*time.Second)
	l.burst(4 * time.Second) // a small burst, as on the station's per-channel mounts
	m, k := l.c.mark(l.now), l.k
	l.steady(3 * time.Second)
	l.check(m, k, 150*time.Millisecond, "cut in the reconnection burst")
	l.steady(time.Minute)
	l.check(l.c.mark(l.now), l.k, 150*time.Millisecond, "a minute after reconnecting")
}

// A stall on the network: audio that arrives late is dated when it was live.
func TestLateAudioIsDatedWhenItWasLive(t *testing.T) {
	l := newListener(t, 8000, 4*time.Second, time.Second)
	l.burst(4 * time.Second)
	l.steady(time.Minute)
	l.now = l.now.Add(800 * time.Millisecond) // nothing for 0.8 s, then it all comes
	l.read(800*time.Millisecond + tick)
	l.check(l.c.mark(l.now), l.k, 100*time.Millisecond, "delivered after a stall")
}

// The station's mixer can drop 125 ms when its output thread falls behind. The
// clock follows within its window, and is never off by more than the drop.
func TestAMixerDropIsFollowed(t *testing.T) {
	l := newListener(t, 8000, 60*time.Second, time.Second)
	l.burst(60 * time.Second)
	l.steady(time.Minute)
	l.drops = append(l.drops, l.k)
	l.now = l.now.Add(125 * time.Millisecond) // 125 ms of clock with no audio for it
	l.steady(10 * time.Second)
	l.check(l.c.mark(l.now), l.k, 130*time.Millisecond, "just after the drop")
	l.steady(3 * time.Minute)
	l.check(l.c.mark(l.now), l.k, 30*time.Millisecond, "once the window has passed")
}

// After a reconnection the server's burst repeats what was already delivered:
// dated by the clock, it falls on or before the last transmission sent.
func TestAReplayedTransmissionIsDroppedAndANewOneKept(t *testing.T) {
	var f replayFilter
	t0 := time.Date(2026, 9, 25, 22, 0, 0, 0, time.UTC)
	for i, c := range []struct {
		at   time.Duration
		keep bool
	}{
		{0, true},
		{4 * time.Second, true},
		{4*time.Second + 300*time.Millisecond, false}, // the same one, replayed, dated a little late
		{2 * time.Second, false},                      // an older one, replayed
		{5*time.Second + 100*time.Millisecond, true},  // the next one: new
	} {
		if got := f.fresh(t0.Add(c.at)); got != c.keep {
			t.Errorf("%d: fresh(+%s) = %v, want %v", i, c.at, got, c.keep)
		}
	}
}
