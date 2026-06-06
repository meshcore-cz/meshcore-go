package queue

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestQueueDoDeliver(t *testing.T) {
	q := New()

	var wg sync.WaitGroup
	wg.Add(1)
	var got any
	go func() {
		defer wg.Done()
		v, err := q.Do(context.Background(), func() error { return nil })
		if err != nil {
			t.Errorf("Do: %v", err)
		}
		got = v
	}()

	// Wait until the waiter is registered, then deliver.
	deadline := time.After(time.Second)
	for {
		if q.Deliver("response") {
			break
		}
		select {
		case <-deadline:
			t.Fatal("Deliver never matched a waiter")
		default:
			time.Sleep(time.Millisecond)
		}
	}
	wg.Wait()
	if got != "response" {
		t.Errorf("got %v, want response", got)
	}
}

func TestQueueTimeout(t *testing.T) {
	q := New()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	_, err := q.Do(ctx, func() error { return nil })
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("err = %v, want DeadlineExceeded", err)
	}
}

func TestQueueSendError(t *testing.T) {
	q := New()
	sentinel := errors.New("boom")
	_, err := q.Do(context.Background(), func() error { return sentinel })
	if !errors.Is(err, sentinel) {
		t.Errorf("err = %v, want %v", err, sentinel)
	}
}

func TestDeliverWithoutWaiter(t *testing.T) {
	q := New()
	if q.Deliver("stray") {
		t.Error("Deliver should report false when no waiter is registered")
	}
}
