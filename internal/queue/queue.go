package queue

import (
	"context"
	"sync"
)

// Queue allows a single active request and correlates the next delivered
// response with the waiting caller.
type Queue struct {
	sendMu sync.Mutex // ensures only one request is active at a time

	mu     sync.Mutex
	waiter *streamWaiter
}

type streamWaiter struct {
	responses chan any
	overflow  chan error
}

// New returns an empty Queue.
func New() *Queue { return &Queue{} }

// streamBuffer bounds how many undelivered responses may queue for a single
// in-flight request. It must comfortably exceed the largest multi-frame
// response (e.g. a full contact list).
const streamBuffer = 2048

// Do transmits a command via send and waits for a single response delivered
// through Deliver, honoring ctx (which the caller typically bounds with a
// timeout). Only one Do/DoStream call runs at a time.
func (q *Queue) Do(ctx context.Context, send func() error) (any, error) {
	var result any
	err := q.run(ctx, send, func(v any) bool {
		result = v
		return true
	})
	return result, err
}

// DoStream transmits a command and feeds each delivered response to collect
// until collect returns true (stream complete) or ctx is done.
func (q *Queue) DoStream(ctx context.Context, send func() error, collect func(any) bool) error {
	return q.run(ctx, send, collect)
}

func (q *Queue) run(ctx context.Context, send func() error, collect func(any) bool) error {
	q.sendMu.Lock()
	defer q.sendMu.Unlock()

	w := &streamWaiter{
		responses: make(chan any, streamBuffer),
		overflow:  make(chan error, 1),
	}
	q.mu.Lock()
	q.waiter = w
	q.mu.Unlock()

	defer func() {
		q.mu.Lock()
		q.waiter = nil
		q.mu.Unlock()
	}()

	if err := send(); err != nil {
		return err
	}

	for {
		select {
		case v := <-w.responses:
			if collect(v) {
				return nil
			}
		case err := <-w.overflow:
			return err
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// Deliver routes a response to the current waiter. Unmatched responses (no
// active request) return (false, nil). Returns ErrResponseOverflow when the
// response buffer is full.
func (q *Queue) Deliver(v any) (bool, error) {
	q.mu.Lock()
	w := q.waiter
	q.mu.Unlock()
	if w == nil {
		return false, nil
	}
	select {
	case w.responses <- v:
		return true, nil
	default:
		select {
		case w.overflow <- ErrResponseOverflow:
		default:
		}
		return false, ErrResponseOverflow
	}
}
