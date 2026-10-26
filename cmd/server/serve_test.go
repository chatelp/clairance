package main

import (
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/yegors/co-atc/internal/config"
)

// A client that trickles its headers is cut once readHeaderTimeout has
// passed, however steadily it trickles; an answer that lasts longer than
// that -- an audio stream -- still arrives whole. The timeouts are the
// defaults otherwise: no read or write timeout, an hour of idle.
func TestASlowClientIsCutAndALongAnswerIsNot(t *testing.T) {
	saved := readHeaderTimeout
	readHeaderTimeout = 300 * time.Millisecond
	t.Cleanup(func() { readHeaderTimeout = saved })

	const chunks, every = 10, 100 * time.Millisecond // a second of "audio"
	stream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for i := 0; i < chunks; i++ {
			time.Sleep(every)
			_, _ = w.Write(make([]byte, 100))
			w.(http.Flusher).Flush()
		}
	})
	cfg := config.Defaults()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := httpServer(&cfg, ln.Addr().String(), stream)
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })

	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	start := time.Now()
	go func() {
		for _, b := range []byte("GET / HTTP/1.1\r\nHost: station\r\nX-Slow: ") {
			if _, err := conn.Write([]byte{b}); err != nil {
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
	}()
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, err = io.ReadAll(conn)
	if took := time.Since(start); err != nil || took > 2*time.Second {
		t.Errorf("a client trickling its headers: closed after %v (err %v), want within a second or so", took, err)
	}

	start = time.Now()
	resp, err := http.Get("http://" + ln.Addr().String() + "/")
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if took := time.Since(start); err != nil || len(body) != chunks*100 || took < 3*readHeaderTimeout {
		t.Errorf("a long answer: %d of %d bytes in %v (err %v)", len(body), chunks*100, took, err)
	}
}
