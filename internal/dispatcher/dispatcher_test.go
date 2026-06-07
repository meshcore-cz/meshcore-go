package dispatcher

import "testing"

func TestDispatcherEmitReceive(t *testing.T) {
	d := New[int](4)
	ch, cancel := d.Subscribe(4)
	defer cancel()
	d.Emit(1)
	d.Emit(2)
	if v := <-ch; v != 1 {
		t.Errorf("got %d, want 1", v)
	}
	if v := <-ch; v != 2 {
		t.Errorf("got %d, want 2", v)
	}
}

func TestDispatcherDropsOldest(t *testing.T) {
	d := New[int](2)
	ch, cancel := d.Subscribe(2)
	defer cancel()
	d.Emit(1)
	d.Emit(2)
	d.Emit(3) // buffer full: drops oldest (1)

	first := <-ch
	second := <-ch
	if first == 1 {
		t.Errorf("expected oldest event to be dropped, got %d first", first)
	}
	if second != 3 {
		t.Errorf("expected newest event 3, got %d", second)
	}
}

func TestDispatcherFanOut(t *testing.T) {
	d := New[int](4)
	a, cancelA := d.Subscribe(4)
	b, cancelB := d.Subscribe(4)
	defer cancelA()
	defer cancelB()
	d.Emit(7)
	if v := <-a; v != 7 {
		t.Errorf("subscriber A got %d, want 7", v)
	}
	if v := <-b; v != 7 {
		t.Errorf("subscriber B got %d, want 7", v)
	}
}

func TestDispatcherClose(t *testing.T) {
	d := New[int](1)
	ch, cancel := d.Subscribe(1)
	defer cancel()
	d.Close()
	d.Emit(99) // no panic, no-op
	if _, ok := <-ch; ok {
		t.Error("channel should be closed")
	}
	d.Close() // idempotent
}
