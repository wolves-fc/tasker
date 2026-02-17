package job

import (
	"context"
	"io"
	"sync"
)

var (
	// Compile time verification that outputBuffer implements io.WriteCloser.
	_ io.WriteCloser = (*outputBuffer)(nil)
	// Compile time verification that outputReader implements io.Reader.
	_ io.Reader = (*outputReader)(nil)
)

// outputBuffer is a byte buffer that notifies readers on change.
type outputBuffer struct {
	mu struct {
		sync.RWMutex
		buf        []byte
		closed     bool
		notifyChan chan struct{}
	}
}

func newOutputBuffer() *outputBuffer {
	ob := &outputBuffer{}
	ob.mu.notifyChan = make(chan struct{})
	return ob
}

// Write appends data to the buffer and notifies readers.
func (ob *outputBuffer) Write(data []byte) (int, error) {
	if len(data) == 0 {
		return 0, nil
	}

	ob.mu.Lock()
	defer ob.mu.Unlock()

	if ob.mu.closed {
		return 0, io.ErrClosedPipe
	}

	ob.mu.buf = append(ob.mu.buf, data...)
	close(ob.mu.notifyChan)
	ob.mu.notifyChan = make(chan struct{})

	return len(data), nil
}

// Close marks the buffer as closed and notifies readers.
func (ob *outputBuffer) Close() error {
	ob.mu.Lock()
	defer ob.mu.Unlock()

	if ob.mu.closed {
		return nil
	}

	ob.mu.closed = true
	close(ob.mu.notifyChan)

	return nil
}

// outputReader reads from the beginning of an outputBuffer.
type outputReader struct {
	ctx context.Context

	ob     *outputBuffer
	offset int
}

func newOutputReader(ctx context.Context, ob *outputBuffer) *outputReader {
	return &outputReader{ctx: ctx, ob: ob}
}

// Read blocks on the outputBuffer until notified of new data, EOF or context cancellation.
func (or *outputReader) Read(buf []byte) (int, error) {
	if len(buf) == 0 {
		return 0, nil
	}

	for {
		or.ob.mu.RLock()
		// new data
		if or.offset < len(or.ob.mu.buf) {
			count := copy(buf, or.ob.mu.buf[or.offset:])
			or.offset += count
			or.ob.mu.RUnlock()

			return count, nil
		}

		// EOF
		if or.ob.mu.closed {
			or.ob.mu.RUnlock()
			return 0, io.EOF
		}

		notifyChan := or.ob.mu.notifyChan
		or.ob.mu.RUnlock()

		select {
		case <-notifyChan:
			// loop back to check what the notify was for
		case <-or.ctx.Done():
			return 0, or.ctx.Err()
		}
	}
}
