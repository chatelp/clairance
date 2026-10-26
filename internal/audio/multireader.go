package audio

import (
	"context"
	"io"
	"sync"
	"time"

	"github.com/yegors/co-atc/pkg/logger"
)

// MultiReader implements a reader that can be consumed by multiple goroutines
//
// Positions are counted from the first byte ever written and never wrap; only
// the storage does. Upstream kept wrapped indices and 64 KB, 1.3 s at 24 kHz:
// a writer that ran more than that ahead of a reader -- the decoder reading an
// Icecast burst of 50 to 65 s in a fraction of a second -- overwrote what the
// reader had not read, and the reader could not tell. A reader that falls
// behind now skips to the oldest data still held, and the loss is logged.
type MultiReader struct {
	buffer     []byte // Circular buffer for audio data
	bufferSize int    // Size of the circular buffer
	written    int64  // Bytes written since the start; the next one goes at written % bufferSize
	readers    map[string]*readerState
	mu         sync.RWMutex // Mutex for thread safety
	ctx        context.Context
	cancel     context.CancelFunc
	logger     *logger.Logger
	closed     bool
}

// readerState tracks the state of each reader
type readerState struct {
	pos      int64      // Next byte to read, counted like MultiReader.written
	readCond *sync.Cond // Condition variable for signaling new data
	closed   bool
	skipped  int64 // bytes lost to falling behind, for the log
}

// multiReaderBytes holds 87 s of 24 kHz mono s16le, so that a whole Icecast
// burst reaches a reader that reads a little slower than the decoder writes.
const multiReaderBytes = 4 << 20

// NewMultiReader creates a new multi-reader
func NewMultiReader(ctx context.Context, logger *logger.Logger) *MultiReader {
	bufferSize := multiReaderBytes
	readerCtx, readerCancel := context.WithCancel(ctx)

	mr := &MultiReader{
		buffer:     make([]byte, bufferSize),
		bufferSize: bufferSize,
		readers:    make(map[string]*readerState),
		ctx:        readerCtx,
		cancel:     readerCancel,
		logger:     logger,
		closed:     false,
	}

	return mr
}

// Write writes data to the buffer and notifies all readers
func (mr *MultiReader) Write(p []byte) (n int, err error) {
	mr.mu.Lock()
	defer mr.mu.Unlock()

	if mr.closed {
		return 0, io.ErrClosedPipe
	}

	// Copy data to the circular buffer
	n = len(p)
	for done := 0; done < n; {
		at := int(mr.written % int64(mr.bufferSize))
		c := copy(mr.buffer[at:], p[done:])
		done += c
		mr.written += int64(c)
	}

	// Notify all readers that new data is available. Broadcast, not Signal: a
	// read that timed out leaves a waiter behind, and Signal could wake that one
	// instead of the reader actually waiting.
	for _, reader := range mr.readers {
		if !reader.closed && reader.readCond != nil {
			reader.readCond.Broadcast()
		}
	}

	return n, nil
}

// CreateReader creates a new reader for the multi-reader
func (mr *MultiReader) CreateReader(id string) io.ReadCloser {
	mr.mu.Lock()
	defer mr.mu.Unlock()

	// Check if reader already exists
	if reader, exists := mr.readers[id]; exists {
		if !reader.closed {
			return newMultiReaderClient(mr, id)
		}
		// If it exists but is closed, remove it and create a new one
		delete(mr.readers, id)
	}

	// Create a new reader state
	readerMutex := &sync.Mutex{}
	reader := &readerState{
		pos:      mr.written,                // Start reading from current write position
		readCond: sync.NewCond(readerMutex), // Condition variable for signaling
		closed:   false,
	}

	mr.readers[id] = reader
	mr.logger.Debug("Created new reader", logger.String("reader_id", id))

	return newMultiReaderClient(mr, id)
}

// RemoveReader removes a reader
func (mr *MultiReader) RemoveReader(id string) {
	mr.mu.Lock()
	defer mr.mu.Unlock()

	if reader, exists := mr.readers[id]; exists {
		// Mark as closed
		reader.closed = true

		// Signal the reader in case it's waiting
		if reader.readCond != nil {
			reader.readCond.Signal()
		}

		// Remove from map
		delete(mr.readers, id)
		mr.logger.Debug("Removed reader", logger.String("reader_id", id))
	}
}

// Close closes the multi-reader and all readers
func (mr *MultiReader) Close() error {
	mr.mu.Lock()
	defer mr.mu.Unlock()

	if mr.closed {
		return nil
	}

	mr.closed = true
	mr.cancel()

	// Close all readers
	for id, reader := range mr.readers {
		// Mark as closed
		reader.closed = true

		// Signal the reader in case it's waiting
		if reader.readCond != nil {
			reader.readCond.Signal()
		}

		mr.logger.Debug("Closed reader during shutdown", logger.String("reader_id", id))
	}

	// Clear readers map
	mr.readers = make(map[string]*readerState)

	return nil
}

// multiReaderClient is a ReadCloser that reads from a MultiReader
type multiReaderClient struct {
	mr *MultiReader
	id string
	mu sync.Mutex // Mutex for thread safety
}

// newMultiReaderClient creates a new client for the multi-reader
func newMultiReaderClient(mr *MultiReader, id string) io.ReadCloser {
	return &multiReaderClient{
		mr: mr,
		id: id,
	}
}

// Read reads data from the multi-reader
func (mrc *multiReaderClient) Read(p []byte) (n int, err error) {
	// Lock to prevent concurrent reads from the same client
	mrc.mu.Lock()
	defer mrc.mu.Unlock()

	// Get reader state
	mrc.mr.mu.RLock()
	reader, exists := mrc.mr.readers[mrc.id]
	if !exists || reader.closed || mrc.mr.closed {
		mrc.mr.mu.RUnlock()
		return 0, io.EOF
	}

	// Get current read position
	pos := reader.pos
	written := mrc.mr.written
	readCond := reader.readCond
	mrc.mr.mu.RUnlock()

	// If there's no data available, wait for it
	if pos == written {
		// Wait for data with a timeout
		waitChan := make(chan struct{})

		go func() {
			readCond.L.Lock()
			defer readCond.L.Unlock()

			// Wait for signal or timeout
			readCond.Wait()
			close(waitChan)
		}()

		// Wait for either data or context cancellation
		select {
		case <-waitChan:
			// Data is available, continue
		case <-mrc.mr.ctx.Done():
			return 0, io.EOF
		case <-time.After(30 * time.Second):
			// Longer timeout, and return EOF to signal connection should be reestablished
			return 0, io.EOF
		}
	}

	mrc.mr.mu.Lock()
	defer mrc.mr.mu.Unlock()
	reader, exists = mrc.mr.readers[mrc.id]
	if !exists || reader.closed || mrc.mr.closed {
		return 0, io.EOF
	}
	size := int64(mrc.mr.bufferSize)
	if behind := mrc.mr.written - reader.pos; behind > size {
		// Overwritten before it was read: skip to the oldest byte still held,
		// keeping sample alignment.
		skip := (behind - size + 1) &^ 1
		reader.pos += skip
		reader.skipped += skip
		mrc.mr.logger.Warn("Audio reader fell behind, oldest audio dropped",
			logger.String("reader_id", mrc.id), logger.Int("bytes", int(skip)),
			logger.Int("bytes_dropped_so_far", int(reader.skipped)))
	}

	// Copy what is available, in at most two pieces around the end of storage
	copied := 0
	for copied < len(p) && reader.pos < mrc.mr.written {
		at := int(reader.pos % size)
		end := mrc.mr.bufferSize
		if left := mrc.mr.written - reader.pos; int64(end-at) > left {
			end = at + int(left)
		}
		c := copy(p[copied:], mrc.mr.buffer[at:end])
		copied += c
		reader.pos += int64(c)
	}

	return copied, nil
}

// Close closes the reader
func (mrc *multiReaderClient) Close() error {
	mrc.mr.RemoveReader(mrc.id)
	return nil
}
