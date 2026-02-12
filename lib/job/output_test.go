package job

import (
	"bytes"
	"context"
	"io"
	"sync"
	"testing"
	"time"
)

func TestOutputBuffer_Write(t *testing.T) {
	t.Parallel()

	t.Run("single", func(t *testing.T) {
		t.Parallel()

		ob := newOutputBuffer()
		data := []byte("hello world")
		count, err := ob.Write(data)
		if err != nil {
			t.Fatalf("Write (got=%v, want=nil)", err)
		}

		if count != len(data) {
			t.Fatalf("Write count (got=%d, want=%d)", count, len(data))
		}

		ob.mu.RLock()
		got := ob.mu.buf
		ob.mu.RUnlock()

		if !bytes.Equal(got, data) {
			t.Fatalf("buffer data (got=%q, want=%q)", got, data)
		}
	})

	t.Run("multiple", func(t *testing.T) {
		t.Parallel()

		ob := newOutputBuffer()
		_, _ = ob.Write([]byte("one"))
		_, _ = ob.Write([]byte("two"))
		_, _ = ob.Write([]byte("three"))

		ob.mu.RLock()
		got := ob.mu.buf
		ob.mu.RUnlock()

		want := []byte("onetwothree")
		if !bytes.Equal(got, want) {
			t.Fatalf("buffer data (got=%q, want=%q)", got, want)
		}
	})

	t.Run("empty", func(t *testing.T) {
		t.Parallel()

		ob := newOutputBuffer()
		count, err := ob.Write(nil)
		if count != 0 || err != nil {
			t.Fatalf("Write nil (got=(%d, %v), want=(0, nil))", count, err)
		}

		count, err = ob.Write([]byte{})
		if count != 0 || err != nil {
			t.Fatalf("Write empty (got=(%d, %v), want=(0, nil))", count, err)
		}

		ob.mu.RLock()
		got := len(ob.mu.buf)
		ob.mu.RUnlock()

		if got != 0 {
			t.Fatalf("buffer length after empty writes (got=%d, want=0)", got)
		}
	})

	t.Run("after_close", func(t *testing.T) {
		t.Parallel()

		ob := newOutputBuffer()
		ob.Close()

		count, err := ob.Write([]byte("hello"))
		if count != 0 || err != io.ErrClosedPipe {
			t.Fatalf("Write after Close (got=(%d, %v), want=(0, io.ErrClosedPipe))", count, err)
		}
	})
}

func TestOutputBuffer_CloseIdempotent(t *testing.T) {
	t.Parallel()

	ob := newOutputBuffer()
	if err := ob.Close(); err != nil {
		t.Fatalf("first Close (got=%v, want=nil)", err)
	}

	if err := ob.Close(); err != nil {
		t.Fatalf("second Close (got=%v, want=nil)", err)
	}
}

func TestOutputReader_Read(t *testing.T) {
	t.Parallel()

	t.Run("new_data", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		ob := newOutputBuffer()
		or := newOutputReader(ctx, ob)

		// Read and then write the data to the done channel
		done := make(chan []byte, 1)
		go func() {
			buf := make([]byte, 16)
			count, _ := or.Read(buf)
			done <- buf[:count]
		}()

		// Read should be blocked since there is no new data
		select {
		case <-done:
			t.Fatal("Read returned before Write")
		case <-time.After(50 * time.Millisecond):
		}

		data := []byte("hello world")
		_, _ = ob.Write(data)

		// Read goroutine should have returned since there was new data
		select {
		case got := <-done:
			if !bytes.Equal(got, data) {
				t.Fatalf("data mismatch (got=%q, want=%q)", got, data)
			}
		case <-time.After(time.Second):
			t.Fatal("Read did not unblock after Write")
		}
	})

	t.Run("eof", func(t *testing.T) {
		t.Parallel()

		ob := newOutputBuffer()
		or := newOutputReader(context.Background(), ob)
		ob.Close()

		buf := make([]byte, 16)
		count, err := or.Read(buf)
		if count != 0 || err != io.EOF {
			t.Fatalf("Read on closed empty buffer (got=(%d, %v), want=(0, EOF))", count, err)
		}
	})
}

func TestOutputReader_DrainsBeforeEOF(t *testing.T) {
	t.Parallel()

	data := []byte("hello world")
	readSize := 4
	ob := newOutputBuffer()
	_, _ = ob.Write(data)
	ob.Close()

	or := newOutputReader(context.Background(), ob)
	chunk := make([]byte, readSize)

	count, err := or.Read(chunk)
	if err != nil {
		t.Fatalf("first Read (got=%v, want=nil)", err)
	}

	if !bytes.Equal(chunk[:count], data[:readSize]) {
		t.Fatalf("first Read data (got=%q, want=%q)", chunk[:count], data[:readSize])
	}

	remaining, err := io.ReadAll(or)
	if err != nil {
		t.Fatalf("ReadAll (got=%v, want=nil)", err)
	}

	got := append(chunk[:count], remaining...)
	if !bytes.Equal(got, data) {
		t.Fatalf("full data (got=%q, want=%q)", got, data)
	}
}

func TestOutputReader_ZeroLengthBuffer(t *testing.T) {
	t.Parallel()

	ob := newOutputBuffer()
	or := newOutputReader(context.Background(), ob)
	_, _ = ob.Write([]byte("hello world"))

	count, err := or.Read([]byte{})
	if count != 0 || err != nil {
		t.Fatalf("Read with empty buffer (got=(%d, %v), want=(0, nil))", count, err)
	}
}

func TestOutputReader_ContextCancel(t *testing.T) {
	t.Parallel()

	t.Run("before", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithCancel(context.Background())

		ob := newOutputBuffer()
		or := newOutputReader(ctx, ob)

		cancel()

		buf := make([]byte, 16)
		count, err := or.Read(buf)
		if count != 0 || err != context.Canceled {
			t.Fatalf("Read with cancelled context (got=(%d, %v), want=(0, context.Canceled))", count, err)
		}
	})

	t.Run("during", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithCancel(context.Background())

		ob := newOutputBuffer()
		or := newOutputReader(ctx, ob)

		// Read and then write the error to the done channel
		done := make(chan error, 1)
		go func() {
			buf := make([]byte, 16)
			_, err := or.Read(buf)
			done <- err
		}()

		cancel()

		// Read goroutine should have returned since the context was cancelled
		select {
		case err := <-done:
			if err != context.Canceled {
				t.Fatalf("Read after cancel (got=%v, want=context.Canceled)", err)
			}
		case <-time.After(time.Second):
			t.Fatal("Read did not unblock after context cancellation")
		}
	})
}

func TestOutputReader_MultipleReaders(t *testing.T) {
	t.Parallel()

	data := []byte("hello world")
	ob := newOutputBuffer()
	_, _ = ob.Write(data)
	ob.Close()

	var wg sync.WaitGroup
	for range 3 {
		wg.Go(func() {
			or := newOutputReader(context.Background(), ob)
			got, err := io.ReadAll(or)
			if err != nil {
				t.Errorf("ReadAll (got=%v, want=nil)", err)
			}

			if !bytes.Equal(got, data) {
				t.Errorf("data mismatch (got=%q, want=%q)", got, data)
			}
		})
	}

	wg.Wait()
}

func TestOutputReader_ConcurrentWriteAndRead(t *testing.T) {
	t.Parallel()

	ob := newOutputBuffer()
	or := newOutputReader(context.Background(), ob)

	// Write 100 sequential bytes
	go func() {
		defer ob.Close()

		for i := range 100 {
			_, _ = ob.Write([]byte{byte(i)})
		}
	}()

	got, err := io.ReadAll(or)
	if err != nil {
		t.Fatalf("ReadAll (got=%v, want=nil)", err)
	}

	if len(got) != 100 {
		t.Fatalf("byte count (got=%d, want=100)", len(got))
	}

	for i, b := range got {
		if b != byte(i) {
			t.Fatalf("byte %d (got=%d, want=%d)", i, b, i)
		}
	}
}
