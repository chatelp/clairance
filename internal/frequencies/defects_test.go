package frequencies

import (
	"os"
	"sync"
	"testing"
	"time"

	cfg "github.com/yegors/co-atc/internal/config"
)

// skipDefect marks a test that shows a defect found while writing these tests.
// It is skipped so the suite stays green until the fix is made on the station's
// side; ATC_DEFECTS=1 runs it, to see the failure.
func skipDefect(t *testing.T, what string) {
	t.Helper()
	if os.Getenv("ATC_DEFECTS") == "" {
		t.Skip("defect: " + what)
	}
}

// bytesWithin reads r for d and reports how much it got: what was waiting for
// the reader, plus what arrived meanwhile.
func bytesWithin(r interface{ Read([]byte) (int, error) }, d time.Duration) int {
	got := make(chan int, 1024)
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := r.Read(buf)
			if err != nil {
				close(got)
				return
			}
			got <- n
		}
	}()
	total := 0
	deadline := time.After(d)
	for {
		select {
		case n, ok := <-got:
			if !ok {
				return total
			}
			total += n
		case <-deadline:
			return total
		}
	}
}

// The page keeps one client id for its life and reuses it each time it
// listens to a frequency again. ClientStreamReader.Close only closes a
// NonClosingReader, whose Close does nothing, so the reader AddClient created
// in the shared MultiReader is never removed; when the id comes back,
// MultiReader.CreateReader finds it still open and hands back its old
// position. The listener starts as far behind live as they were away, up to
// the whole buffer (4 MiB: 87 s at the 24 kHz of the day, 131 s at 16 kHz).
//
// Measured here on the tone: a new client gets 2 to 4 KB in its first 100 ms,
// the live rate; the same id back after 1.5 s gets 64 KB at once, 2.0 s of
// audio -- the time away, plus what the first reader had not yet read.
func TestAListenerWhoComesBackHearsLiveAudio(t *testing.T) {
	sp := newToneProcessor(t)
	first := sp.AddClient("page-1")
	readSome(t, first, 44+3200)
	first.Close() // the stream handler's defer, when the page stops listening
	sp.removeInactiveClients()

	time.Sleep(1500 * time.Millisecond) // away; the stream goes on
	again := sp.AddClient("page-1")
	readSome(t, again, 44)
	// 0.5 s of audio at 16 kHz mono s16le; live delivers well under that in 100 ms.
	if n := bytesWithin(again, 100*time.Millisecond); n > 16000 {
		t.Errorf("back after 1.5 s, the listener got %d bytes (%.1f s of audio) at once: it starts that far behind live", n, float64(n)/32000)
	}
}

// buildStreamInfo advances streamPortIndex, and GetAllFrequencies and
// GetFrequencyByID call it holding only sourcesMu's read lock: two requests
// for the list at once write the index together. The race detector says so;
// without -race this test passes.
func TestTheFrequencyListCanBeReadConcurrently(t *testing.T) {
	c := toneConfig()
	c.Server.AdditionalPorts = []int{8001, 8002}
	c.Frequencies.Sources = []cfg.FrequencyConfig{{ID: "a", Name: "A", URL: listedOnly}, {ID: "b", Name: "B", URL: listedOnly}}
	s := NewService(c, quietLog(t), nil, nil, nil, nil)
	t.Cleanup(s.Stop)

	var wg sync.WaitGroup
	for g := 0; g < 4; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				s.GetAllFrequencies()
				s.GetFrequencyByID("a")
			}
		}()
	}
	wg.Wait()
}
