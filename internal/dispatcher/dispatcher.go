// Package dispatcher fans asynchronous events out to a single consumer channel
// without blocking the producer (the client read loop).
package dispatcher

import "sync"

// Dispatcher delivers events of type T over a buffered channel. Emit never
// blocks: if the buffer is full the oldest event is dropped to keep the read
// loop responsive.
type Dispatcher[T any] struct {
	ch        chan T
	mu        sync.Mutex
	closed    bool
	closeOnce sync.Once
}

// New returns a Dispatcher with the given buffer size.
func New[T any](buffer int) *Dispatcher[T] {
	if buffer < 1 {
		buffer = 1
	}
	return &Dispatcher[T]{ch: make(chan T, buffer)}
}

// Events returns the receive-only event channel. It is closed by Close.
func (d *Dispatcher[T]) Events() <-chan T { return d.ch }

// Emit queues an event, dropping the oldest buffered event if necessary. It is
// a no-op after Close.
func (d *Dispatcher[T]) Emit(ev T) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return
	}
	select {
	case d.ch <- ev:
	default:
		// Buffer full: drop the oldest to make room, then enqueue.
		select {
		case <-d.ch:
		default:
		}
		select {
		case d.ch <- ev:
		default:
		}
	}
}

// Close closes the event channel. It is safe to call multiple times.
func (d *Dispatcher[T]) Close() {
	d.closeOnce.Do(func() {
		d.mu.Lock()
		defer d.mu.Unlock()
		d.closed = true
		close(d.ch)
	})
}
