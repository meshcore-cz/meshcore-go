package backend

import (
	"testing"
	"time"
)

func TestRadioStatusLocked(t *testing.T) {
	s := &Server{}
	s.lockRadio("trace")
	st := func() radioStatus {
		s.mu.RLock()
		defer s.mu.RUnlock()
		return s.radioStatusLocked()
	}()
	if !st.Active || st.Idle {
		t.Fatalf("expected active radio status, got %+v", st)
	}
	if st.Method != "trace" {
		t.Fatalf("method = %q", st.Method)
	}
	if st.Since.IsZero() {
		t.Fatal("expected since timestamp")
	}
	if st.DurationMs < 0 {
		t.Fatalf("duration_ms = %d", st.DurationMs)
	}

	time.Sleep(5 * time.Millisecond)
	s.unlockRadio()
	st = func() radioStatus {
		s.mu.RLock()
		defer s.mu.RUnlock()
		return s.radioStatusLocked()
	}()
	if st.Active || !st.Idle {
		t.Fatalf("expected idle radio status, got %+v", st)
	}
	if st.Method != "" {
		t.Fatalf("method = %q", st.Method)
	}
	if st.LastAt.IsZero() {
		t.Fatal("expected last activity timestamp")
	}
	if st.LastMethod != "trace" {
		t.Fatalf("last method = %q", st.LastMethod)
	}
	if st.LastDurationMs <= 0 {
		t.Fatalf("last duration_ms = %d", st.LastDurationMs)
	}
}

func TestTryLockRadio(t *testing.T) {
	s := &Server{}
	if !s.tryLockRadio("stats") {
		t.Fatal("expected try lock to succeed")
	}
	if s.tryLockRadio("trace") {
		t.Fatal("expected second try lock to fail while held")
	}
	s.unlockRadio()
	if !s.tryLockRadio("trace") {
		t.Fatal("expected try lock to succeed after unlock")
	}
	s.unlockRadio()
}
