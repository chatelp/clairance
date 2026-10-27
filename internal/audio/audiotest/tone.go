// Package audiotest gives the tests of every package a live source of sound
// that needs nothing installed: a tone sent as raw PCM over UDP on the
// loopback, as an SDR program sends its audio. It replaced ffmpeg's generated
// tone (-f lavfi) when ffmpeg was removed (docs-fr/35-plan.md, B1 ter).
package audiotest

import (
	"encoding/binary"
	"math"
	"net"
	"sync"
	"testing"
	"time"
)

// ToneHz is the tone's frequency.
const ToneHz = 440

// FreeUDPPort is a UDP port free on the loopback when asked.
func FreeUDPPort(t testing.TB) int {
	t.Helper()
	c, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	return c.LocalAddr().(*net.UDPAddr).Port
}

// UDPTone sends a tone at half scale as raw 16-bit little-endian mono PCM at
// rate, in real time, 20 ms a packet, to a free port of the loopback, until
// the test ends. It returns the source's address, udp://127.0.0.1:port.
// Packets sent before the source listens are lost, as they would be.
func UDPTone(t testing.TB, rate int) string {
	t.Helper()
	port := FreeUDPPort(t)
	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	dst := &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: port}
	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		per := rate / 50
		pkt := make([]byte, 2*per)
		n := 0
		tick := time.NewTicker(20 * time.Millisecond)
		defer tick.Stop()
		for {
			for i := 0; i < per; i++ {
				v := int16(16384 * math.Sin(2*math.Pi*ToneHz*float64(n)/float64(rate)))
				binary.LittleEndian.PutUint16(pkt[2*i:], uint16(v))
				n++
			}
			_, _ = conn.WriteTo(pkt, dst)
			select {
			case <-done:
				return
			case <-tick.C:
			}
		}
	}()
	t.Cleanup(func() {
		close(done)
		wg.Wait()
		conn.Close()
	})
	return "udp://" + dst.String()
}
