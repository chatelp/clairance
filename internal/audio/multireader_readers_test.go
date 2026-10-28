package audio

import (
	"bytes"
	"errors"
	"io"
	"testing"
	"time"
)

// readResult is what a Read returned, for a Read run on its own goroutine.
type readResult struct {
	data []byte
	err  error
}

func readAsync(r io.Reader, size int) <-chan readResult {
	out := make(chan readResult, 1)
	go func() {
		buf := make([]byte, size)
		n, err := r.Read(buf)
		out <- readResult{buf[:n], err}
	}()
	return out
}

func within(t *testing.T, ch <-chan readResult, d time.Duration, what string) readResult {
	t.Helper()
	select {
	case r := <-ch:
		return r
	case <-time.After(d):
		t.Fatalf("%s: no answer within %v", what, d)
		return readResult{}
	}
}

// Writing never waits for a reader: a browser that stops reading must not
// stall the transcription reading the same stream, nor the decoder behind them.
func TestASlowReaderDoesNotHoldBackTheOthers(t *testing.T) {
	mr := newTestMultiReader(t)
	fast := mr.CreateReader("transcription")
	slow := mr.CreateReader("browser") // never read until the end
	const chunk = 64 << 10
	total := 2*multiReaderBytes + chunk

	done := make(chan struct{})
	go func() {
		defer close(done)
		for off := 0; off < total; off += chunk {
			if _, err := mr.Write(pattern(off, chunk)); err != nil {
				t.Errorf("write at %d: %v", off, err)
				return
			}
			got := make([]byte, chunk)
			if _, err := io.ReadFull(fast, got); err != nil || !bytes.Equal(got, pattern(off, chunk)) {
				t.Errorf("the fast reader lost data at %d: %v", off, err)
				return
			}
		}
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("writing stalled behind a reader that does not read")
	}

	if got := readAll(t, slow, multiReaderBytes); !bytes.Equal(got, pattern(total-multiReaderBytes, multiReaderBytes)) {
		t.Error("the slow reader should get the newest buffer's worth")
	}
}

// A read waiting for audio returns as soon as some is written.
func TestAWaitingReadWakesOnNewData(t *testing.T) {
	mr := newTestMultiReader(t)
	r := mr.CreateReader("transcription")
	pending := readAsync(r, 960)
	time.Sleep(20 * time.Millisecond) // let it wait
	mr.Write(pattern(0, 500))
	got := within(t, pending, 2*time.Second, "a waiting read after a write")
	if got.err != nil || !bytes.Equal(got.data, pattern(0, 500)) {
		t.Errorf("got %d bytes, %v; want the 500 written", len(got.data), got.err)
	}
}

// Removing a reader, or closing it, ends a read that is waiting: the HTTP
// handler of a browser that left must not hang on its last read.
func TestRemovingAReaderEndsItsWaitingRead(t *testing.T) {
	for name, end := range map[string]func(mr *MultiReader, r io.ReadCloser){
		"RemoveReader": func(mr *MultiReader, _ io.ReadCloser) { mr.RemoveReader("browser") },
		"Close":        func(_ *MultiReader, r io.ReadCloser) { r.Close() },
	} {
		t.Run(name, func(t *testing.T) {
			mr := newTestMultiReader(t)
			r := mr.CreateReader("browser")
			other := mr.CreateReader("transcription")
			pending := readAsync(r, 960)
			time.Sleep(20 * time.Millisecond)
			end(mr, r)
			if got := within(t, pending, 2*time.Second, "a waiting read after its reader ended"); !errors.Is(got.err, io.EOF) {
				t.Errorf("want EOF, got %d bytes, %v", len(got.data), got.err)
			}
			if _, err := r.Read(make([]byte, 10)); !errors.Is(err, io.EOF) {
				t.Errorf("a read after the end: %v, want EOF", err)
			}
			mr.Write(pattern(0, 100))
			if got := readAll(t, other, 100); !bytes.Equal(got, pattern(0, 100)) {
				t.Error("the other reader should be unaffected")
			}
		})
	}
}

// Closing the stream ends every reader, refuses further writes, and can be
// done twice.
func TestClosingTheStreamEndsEveryReader(t *testing.T) {
	mr := newTestMultiReader(t)
	a := readAsync(mr.CreateReader("a"), 960)
	b := readAsync(mr.CreateReader("b"), 960)
	time.Sleep(20 * time.Millisecond)
	if err := mr.Close(); err != nil {
		t.Fatal(err)
	}
	for name, ch := range map[string]<-chan readResult{"a": a, "b": b} {
		if got := within(t, ch, 2*time.Second, "reader "+name+" after Close"); !errors.Is(got.err, io.EOF) {
			t.Errorf("reader %s: %v, want EOF", name, got.err)
		}
	}
	if _, err := mr.Write([]byte{1, 2}); !errors.Is(err, io.ErrClosedPipe) {
		t.Errorf("write after Close: %v, want ErrClosedPipe", err)
	}
	if err := mr.Close(); err != nil {
		t.Errorf("a second Close: %v", err)
	}
	if _, err := mr.CreateReader("late").Read(make([]byte, 10)); !errors.Is(err, io.EOF) {
		t.Errorf("a reader created after Close: %v, want EOF", err)
	}
}

// An id still open is the same reader, at the same position; an id closed and
// created again starts live.
func TestAReaderIDIsReusedOnlyWhileOpen(t *testing.T) {
	mr := newTestMultiReader(t)
	first := mr.CreateReader("browser")
	mr.Write(pattern(0, 1000))
	if _, err := io.ReadFull(first, make([]byte, 400)); err != nil {
		t.Fatal(err)
	}

	same := mr.CreateReader("browser")
	got := make([]byte, 600)
	if _, err := io.ReadFull(same, got); err != nil || !bytes.Equal(got, pattern(400, 600)) {
		t.Error("an id still open should continue where its reader stood")
	}

	same.Close()
	mr.Write(pattern(1000, 300))
	again := mr.CreateReader("browser")
	mr.Write(pattern(1300, 200))
	if got := readAll(t, again, 200); !bytes.Equal(got, pattern(1300, 200)) {
		t.Error("an id created again after Close should start live")
	}
}

// A reader overtaken by an odd number of bytes skips an even number, so that
// it stays on 16-bit sample boundaries.
func TestAnOvertakenReaderStaysOnSampleBoundaries(t *testing.T) {
	mr := newTestMultiReader(t)
	r := mr.CreateReader("slow")
	mr.Write(pattern(0, multiReaderBytes+1))
	got := readAll(t, r, multiReaderBytes-1)
	if !bytes.Equal(got, pattern(2, multiReaderBytes-1)) {
		t.Error("one byte too many should cost the oldest sample, two bytes, and keep the rest in order")
	}
}
