package protocol

import (
	"errors"
	"sync"
)

// ErrStreamClosed is returned when reading from or writing to a closed stream.
var ErrStreamClosed = errors.New("stream closed")

// StreamReader provides a pull-based interface for consuming a stream of values.
type StreamReader[T any] struct {
	ch   chan T
	done chan struct{}
	once sync.Once
	err  error
	mu   sync.Mutex
}

// Recv blocks until the next value is available, the stream ends (io.EOF), or the stream is closed.
func (r *StreamReader[T]) Recv() (T, error) {
	select {
	case v, ok := <-r.ch:
		if !ok {
			var zero T
			r.mu.Lock()
			err := r.err
			r.mu.Unlock()
			if err != nil {
				return zero, err
			}
			return zero, errors.New("EOF")
		}
		return v, nil
	case <-r.done:
		var zero T
		return zero, ErrStreamClosed
	}
}

// Close signals the producer to stop. Safe to call multiple times.
func (r *StreamReader[T]) Close() {
	r.once.Do(func() { close(r.done) })
}

// Copy creates n independent readers that each receive all subsequent values.
func (r *StreamReader[T]) Copy(n int) []*StreamReader[T] {
	readers := make([]*StreamReader[T], n)
	writers := make([]*StreamWriter[T], n)
	for i := 0; i < n; i++ {
		readers[i], writers[i] = Pipe[T](cap(r.ch))
	}
	go func() {
		for {
			v, err := r.Recv()
			if err != nil {
				for _, w := range writers {
					w.Finish(err)
				}
				return
			}
			for _, w := range writers {
				_ = w.Send(v)
			}
		}
	}()
	return readers
}

// StreamWriter provides a push-based interface for producing a stream of values.
type StreamWriter[T any] struct {
	ch   chan T
	done chan struct{}
	once sync.Once
	errp *error
	mu   *sync.Mutex
}

// Send pushes a value into the stream. Returns ErrStreamClosed if the reader has closed.
func (w *StreamWriter[T]) Send(v T) error {
	select {
	case w.ch <- v:
		return nil
	case <-w.done:
		return ErrStreamClosed
	}
}

// Finish closes the stream with an optional error. Safe to call multiple times.
func (w *StreamWriter[T]) Finish(err error) {
	w.once.Do(func() {
		if err != nil {
			w.mu.Lock()
			*w.errp = err
			w.mu.Unlock()
		}
		close(w.ch)
	})
}

// Pipe creates a connected StreamReader/StreamWriter pair with the given buffer capacity.
func Pipe[T any](cap int) (*StreamReader[T], *StreamWriter[T]) {
	ch := make(chan T, cap)
	done := make(chan struct{})
	r := &StreamReader[T]{ch: ch, done: done}
	w := &StreamWriter[T]{ch: ch, done: done, errp: &r.err, mu: &r.mu}
	return r, w
}

// ReaderFromSlice creates a StreamReader that yields all items then closes.
func ReaderFromSlice[T any](items []T) *StreamReader[T] {
	r, w := Pipe[T](len(items))
	go func() {
		for _, item := range items {
			if err := w.Send(item); err != nil {
				w.Finish(err)
				return
			}
		}
		w.Finish(nil)
	}()
	return r
}

// CollectAll reads all values from a StreamReader into a slice.
func CollectAll[T any](r *StreamReader[T]) ([]T, error) {
	var result []T
	for {
		v, err := r.Recv()
		if err != nil {
			if err.Error() == "EOF" {
				return result, nil
			}
			return result, err
		}
		result = append(result, v)
	}
}

// MapReader transforms a stream by applying fn to each element.
func MapReader[T any, U any](r *StreamReader[T], fn func(T) (U, error)) *StreamReader[U] {
	out, w := Pipe[U](cap(r.ch))
	go func() {
		for {
			v, err := r.Recv()
			if err != nil {
				w.Finish(err)
				return
			}
			u, err := fn(v)
			if err != nil {
				w.Finish(err)
				return
			}
			if err := w.Send(u); err != nil {
				w.Finish(err)
				return
			}
		}
	}()
	return out
}
