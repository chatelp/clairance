package audio

import (
	"testing"
	"time"
)

func TestFlowCountsTheLastMinute(t *testing.T) {
	var m flowMeter
	t0 := time.Unix(1_000_000, 0)

	if n, last := m.flow(t0); n != 0 || !last.IsZero() {
		t.Fatalf("before any byte: %d bytes, last %v", n, last)
	}

	// 32 kB a second for 90 s: the minute holds 60 of them.
	for i := 0; i < 90; i++ {
		m.add(32000, t0.Add(time.Duration(i)*time.Second))
	}
	now := t0.Add(89 * time.Second)
	if n, last := m.flow(now); n != 60*32000 || !last.Equal(now) {
		t.Fatalf("steady flow: %d bytes, last %v", n, last)
	}

	// The decoder stops: 30 s later half the minute is left, 60 s later nothing,
	// and the last byte keeps its time.
	if n, _ := m.flow(now.Add(30 * time.Second)); n != 30*32000 {
		t.Fatalf("30 s after the stop: %d bytes", n)
	}
	if n, last := m.flow(now.Add(60 * time.Second)); n != 0 || !last.Equal(now) {
		t.Fatalf("a minute after the stop: %d bytes, last %v", n, last)
	}

	// Long after, a slot is reused for a new second: the old count is gone.
	later := now.Add(10 * time.Minute)
	m.add(100, later)
	if n, _ := m.flow(later); n != 100 {
		t.Fatalf("after a silence: %d bytes", n)
	}
}
