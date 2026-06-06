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

// streamBuffer bounds how many undelivered responses may queue for a single
// in-flight request. It must comfortably exceed the largest multi-frame
// response (e.g. a full contact list).
const streamBuffer = 512

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

	ch := make(chan any, streamBuffer)
	q.mu.Lock()
	q.waiter = ch
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
		case v := <-ch:
			if collect(v) {
				return nil
			}
		case <-ctx.Done():
			return ctx.Err()
		}
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
