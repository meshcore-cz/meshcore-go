// Package queue serialises protocol requests so only one is in flight at a
// time and routes the matching response back to the caller. The SDK owns
// request sequencing; applications never manage command timing manually.
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
	waiter chan any
}

// New returns an empty Queue.
func New() *Queue { return &Queue{} }

// Do transmits a command via send and waits for a response delivered through
// Deliver, honoring ctx (which the caller typically bounds with a timeout).
// Only one Do call runs at a time.
func (q *Queue) Do(ctx context.Context, send func() error) (any, error) {
	q.sendMu.Lock()
	defer q.sendMu.Unlock()

	ch := make(chan any, 1)
	q.mu.Lock()
	q.waiter = ch
	q.mu.Unlock()

	defer func() {
		q.mu.Lock()
		q.waiter = nil
		q.mu.Unlock()
	}()

	if err := send(); err != nil {
		return nil, err
	}

	select {
	case v := <-ch:
		return v, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// Deliver routes a response to the current waiter. It reports whether a waiter
// accepted the value; unmatched responses (e.g. when no request is pending)
// return false so the caller can treat them as stray.
func (q *Queue) Deliver(v any) bool {
	q.mu.Lock()
	ch := q.waiter
	q.mu.Unlock()
	if ch == nil {
		return false
	}
	select {
	case ch <- v:
		return true
	default:
		return false
	}
}
