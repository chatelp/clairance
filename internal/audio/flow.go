package audio

import (
	"sync"
	"time"
)

// flowMeter counts the bytes ffmpeg hands over, second by second, over the
// last flowWindow. "connected" says only that ffmpeg started: from 16 to
// 18/10 every frequency read "connected" while ffmpeg, cut off from the
// station, produced nothing for a day and a half. The bytes say whether audio
// arrives (docs-fr/34, the launcher's health).
type flowMeter struct {
	mu    sync.Mutex
	slots [flowWindow]flowSlot
	last  time.Time // when the last byte arrived; zero before the first
}

type flowSlot struct {
	second int64 // the Unix second this slot counts; a slot from an older second is stale
	bytes  int64
}

// flowWindow is how far back Flow looks, one slot per second.
const flowWindow = 60

func (m *flowMeter) add(n int, now time.Time) {
	if n <= 0 {
		return
	}
	sec := now.Unix()
	m.mu.Lock()
	s := &m.slots[sec%flowWindow]
	if s.second != sec {
		s.second, s.bytes = sec, 0
	}
	s.bytes += int64(n)
	m.last = now
	m.mu.Unlock()
}

// flow returns the bytes of the last flowWindow seconds, the current one
// included, and when the last byte arrived.
func (m *flowMeter) flow(now time.Time) (int64, time.Time) {
	sec := now.Unix()
	m.mu.Lock()
	defer m.mu.Unlock()
	var total int64
	for _, s := range m.slots {
		if s.second > sec-flowWindow && s.second <= sec {
			total += s.bytes
		}
	}
	return total, m.last
}

// Flow is the audio that came out of ffmpeg over the last minute, in bytes,
// and when its last byte did; a zero time before the first.
func (p *CentralAudioProcessor) Flow() (int64, time.Time) {
	return p.flow.flow(time.Now())
}
