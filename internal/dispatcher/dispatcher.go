package dispatcher

import "sync"

// Dispatcher fans asynchronous events out to subscribers without blocking the
// producer (the client read loop).
type Dispatcher[T any] struct {
	mu        sync.Mutex
	subs      map[uint64]chan T
	nextID    uint64
	closed    bool
	closeOnce sync.Once
}

// New returns a Dispatcher. Subscribers are created with Subscribe.
func New[T any](buffer int) *Dispatcher[T] {
	_ = buffer // per-subscriber buffer is passed to Subscribe
	return &Dispatcher[T]{subs: make(map[uint64]chan T)}
}

// Subscribe registers a new subscriber with its own buffered channel. The
// returned cancel function removes the subscriber. Slow subscribers may drop
// their oldest buffered event to make room.
func (d *Dispatcher[T]) Subscribe(buffer int) (<-chan T, func()) {
	if buffer < 1 {
		buffer = 1
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		ch := make(chan T)
		close(ch)
		return ch, func() {}
	}
	id := d.nextID
	d.nextID++
	ch := make(chan T, buffer)
	d.subs[id] = ch
	return ch, func() { d.unsubscribe(id) }
}

// Events returns a receive-only channel for the first subscription pattern.
// Prefer Subscribe when multiple consumers are needed.
func (d *Dispatcher[T]) Events() <-chan T {
	ch, _ := d.Subscribe(64)
	return ch
}

func (d *Dispatcher[T]) unsubscribe(id uint64) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if ch, ok := d.subs[id]; ok {
		delete(d.subs, id)
		close(ch)
	}
}

// Emit queues an event to all subscribers, dropping the oldest buffered event
// per slow subscriber if necessary. It is a no-op after Close.
func (d *Dispatcher[T]) Emit(ev T) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return
	}
	for _, ch := range d.subs {
		select {
		case ch <- ev:
		default:
			select {
			case <-ch:
			default:
			}
			select {
			case ch <- ev:
			default:
			}
		}
	}
}

// Close closes all subscriber channels. It is safe to call multiple times.
func (d *Dispatcher[T]) Close() {
	d.closeOnce.Do(func() {
		d.mu.Lock()
		defer d.mu.Unlock()
		d.closed = true
		for id, ch := range d.subs {
			close(ch)
			delete(d.subs, id)
		}
	})
}
