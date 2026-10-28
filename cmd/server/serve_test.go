package main

import (
	"context"
	"io"
	"net"
	"net/http"
	"strings"
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

// The stop ends what is still being answered instead of waiting for it: a
// stream that ends only with its request's context lets the server stop at
// once; an answer that does not listen is named, so that a slow stop says
// what held it (27/10: 10 s, twice, unexplained).
func TestTheStopEndsWhatIsStillAnswered(t *testing.T) {
	a := newAnswering()
	release := make(chan struct{})
	mux := http.NewServeMux()
	started := make(chan string, 2)
	mux.HandleFunc("/stream", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		started <- "stream"
		<-r.Context().Done()
	})
	mux.HandleFunc("/deaf", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		started <- "deaf"
		<-release // listens to nothing the stop says
	})
	cfg := config.Defaults()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := answeringServer(&cfg, ln.Addr().String(), mux, a)
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { close(release); _ = srv.Close() })

	resp, err := http.Get("http://" + ln.Addr().String() + "/stream")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	<-started

	a.stop()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	start := time.Now()
	if err := srv.Shutdown(ctx); err != nil || time.Since(start) > time.Second {
		t.Errorf("stopped in %v (err %v) with a stream open, want at once", time.Since(start), err)
	}

	// Another server, the same way, with an answer that does not listen.
	b := newAnswering()
	ln2, _ := net.Listen("tcp", "127.0.0.1:0")
	srv2 := answeringServer(&cfg, ln2.Addr().String(), mux, b)
	go func() { _ = srv2.Serve(ln2) }()
	t.Cleanup(func() { _ = srv2.Close() })
	resp2, err := http.Get("http://" + ln2.Addr().String() + "/deaf")
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	<-started
	b.stop()
	still := b.still()
	if len(still) != 1 || !strings.HasPrefix(still[0], "GET /deaf from 127.0.0.1:") {
		t.Errorf("still answering: %q, want the deaf one named", still)
	}
}
