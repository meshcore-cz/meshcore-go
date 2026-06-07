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

	deadline := time.After(time.Second)
	for {
		if ok, err := q.Deliver("response"); ok {
			break
		} else if err != nil {
			t.Fatalf("Deliver: %v", err)
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
	if ok, err := q.Deliver("stray"); ok || err != nil {
		t.Errorf("Deliver without waiter = (%v, %v), want (false, nil)", ok, err)
	}
}

func TestQueueOverflow(t *testing.T) {
	q := New()
	block := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- q.DoStream(context.Background(), func() error { return nil }, func(any) bool {
			<-block
			return false
		})
	}()

	deadline := time.After(time.Second)
	for {
		q.mu.Lock()
		waiting := q.waiter != nil
		q.mu.Unlock()
		if waiting {
			break
		}
		select {
		case <-deadline:
			t.Fatal("waiter never registered")
		default:
			time.Sleep(time.Millisecond)
		}
	}

	if ok, err := q.Deliver(0); !ok || err != nil {
		t.Fatalf("first Deliver = (%v, %v)", ok, err)
	}
	for i := 1; i <= streamBuffer; i++ {
		if ok, err := q.Deliver(i); !ok || err != nil {
			t.Fatalf("Deliver(%d) = (%v, %v)", i, ok, err)
		}
	}
	if ok, err := q.Deliver(streamBuffer + 1); ok || !errors.Is(err, ErrResponseOverflow) {
		t.Fatalf("overflow Deliver = (%v, %v), want (_, ErrResponseOverflow)", ok, err)
	}
	close(block)
	if err := <-done; !errors.Is(err, ErrResponseOverflow) {
		t.Fatalf("DoStream err = %v, want ErrResponseOverflow", err)
	}
}
