package dispatcher

import "testing"

func TestDispatcherEmitReceive(t *testing.T) {
	d := New[int](4)
	d.Emit(1)
	d.Emit(2)
	if v := <-d.Events(); v != 1 {
		t.Errorf("got %d, want 1", v)
	}
	if v := <-d.Events(); v != 2 {
		t.Errorf("got %d, want 2", v)
	}
}

func TestDispatcherDropsOldest(t *testing.T) {
	d := New[int](2)
	d.Emit(1)
	d.Emit(2)
	d.Emit(3) // buffer full: drops oldest (1)

	first := <-d.Events()
	second := <-d.Events()
	if first == 1 {
		t.Errorf("expected oldest event to be dropped, got %d first", first)
	}
	if second != 3 {
		t.Errorf("expected newest event 3, got %d", second)
	}
}

func TestDispatcherClose(t *testing.T) {
	d := New[int](1)
	d.Close()
	d.Emit(99) // no panic, no-op
	if _, ok := <-d.Events(); ok {
		t.Error("channel should be closed")
	}
	d.Close() // idempotent
}
