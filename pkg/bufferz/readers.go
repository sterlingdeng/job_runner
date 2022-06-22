package bufferz

import (
	"context"
	"errors"
	"io"
	"sync"
	"sync/atomic"
)

// MultiReader enables multiple readers to stream from a single writer.
// Readers are cleaned up when their context is cancelled or the Close method is called.
// If the Close method is called, new readers will read the data from the beginning and return an io.EOF
// when it reaches the end
//
// When Close is called, the Write call will return an error.
type MultiReader struct {
	mu   sync.Mutex
	cond *sync.Cond

	data []byte

	// signals the writers to return an io.EOF
	// signals to current and future readers to return io.EOF when each reader has finished reading data
	closeChan chan struct{}
	closed    uint32 // closeCalled if non-zero
}

func NewMultiReaderBuffer() *MultiReader {
	ret := &MultiReader{
		mu:        sync.Mutex{},
		closeChan: make(chan struct{}),
	}
	ret.cond = sync.NewCond(&ret.mu)
	return ret
}

// Write must be used by a single writer. It is invalid to call Write after Close.
func (m *MultiReader) Write(p []byte) (int, error) {
	if m.closeCalled() {
		return 0, errors.New("multireader: close called")
	}
	m.mu.Lock()
	m.data = append(m.data, p...)
	m.mu.Unlock()
	m.cond.Broadcast()
	return len(p), nil
}

// GetReader returns independent io.Readers to the underlying data. Each individual reader returned by
// calling GetReader() is not safe to use concurrently. For each goroutine, get a new Reader.
// When the reader gets to the end of the data and Close is called, the reader will return io.EOF.
func (m *MultiReader) GetReader(ctx context.Context) io.Reader {
	var pos int

	// we spin up a goroutine here to mainly listen for context cancellation.
	// the goroutine is cleaned up when context cancellation occurs or when Close() is called.
	go func() {
		select {
		case <-ctx.Done():
			m.mu.Lock()
			m.cond.Broadcast()
			m.mu.Unlock()
			return
		case <-m.closeChan:
			return
		}
	}()

	return reader(func(p []byte) (int, error) {
		m.mu.Lock()
		defer m.mu.Unlock()

	loop:
		for {
			// Readers exit when...
			select {
			case <-ctx.Done():
				// 1) Their context is cancelled: we return the ctx.Err() here
				return 0, ctx.Err()
			case <-m.closeChan:
				// 2) Close is called.
				// We check if there is new data to read even is close is called.
				// If new readers are created after the writer has finished,
				// we still want the new readers to stream data from the beginning.
				break loop
			default:
			}

			// If there is new data to read
			if pos < len(m.data) {
				break loop
			}

			// When reader is at the end and the writer has not closed yet, we wait for more writes here.
			//
			// This wait is unblocked by broadcast which occurs by the following conditions:
			// 1) A new write has been made.
			// 2) Close is called.
			// 3) When any reader's context is cancelled. However, we only break the loop if the reader
			// who's passed context has cancelled.
			m.cond.Wait()
		}

		var n int
		// only call copy if there is new data to read
		if pos < len(m.data) {
			n = copy(p, m.data[pos:])
			pos += n
		}

		// if we reached the end of the stream and no more writes will occur, we reached EOF
		if m.closeCalled() && pos == len(m.data) {
			return n, io.EOF
		}
		return n, nil
	})
}

func (m *MultiReader) closeCalled() bool {
	return atomic.LoadUint32(&m.closed) > 0
}

// Close must be called when writing is complete. This will unblock readers waiting for writes
// and causes any current or future readers to return io.EOF when it reads all the data
// Multireader may leak resources if Close is not called
func (m *MultiReader) Close() error {
	if m.closeCalled() {
		return nil
	}
	atomic.StoreUint32(&m.closed, 1)

	m.mu.Lock()
	defer m.mu.Unlock()
	close(m.closeChan)
	m.cond.Broadcast()
	return nil
}

type reader func([]byte) (int, error)

func (r reader) Read(p []byte) (int, error) { return r(p) }
