package backend

import (
	"context"
	"sync"
	"time"
)

// RadioExecutor serialises live radio I/O and records the currently active or
// most recent operation for status/diagnostics.
type RadioExecutor struct {
	mu          sync.Mutex
	activityMu  sync.RWMutex
	trackWaiter func(int)

	active         bool
	method         string
	since          time.Time
	lastAt         time.Time
	lastMethod     string
	lastDurationMs int64
}

func NewRadioExecutor(trackWaiter func(int)) *RadioExecutor {
	return &RadioExecutor{trackWaiter: trackWaiter}
}

func (r *RadioExecutor) Lock(method string) {
	if r.trackWaiter != nil {
		r.trackWaiter(1)
	}
	r.mu.Lock()
	if r.trackWaiter != nil {
		r.trackWaiter(-1)
	}
	r.start(method)
}

func (r *RadioExecutor) TryLock(method string) bool {
	if !r.mu.TryLock() {
		return false
	}
	r.start(method)
	return true
}

func (r *RadioExecutor) Unlock() {
	r.finish()
	r.mu.Unlock()
}

func (r *RadioExecutor) Do(ctx context.Context, method string, fn func(context.Context) error) error {
	r.Lock(method)
	defer r.Unlock()
	return fn(ctx)
}

func (r *RadioExecutor) Status() radioStatus {
	r.activityMu.RLock()
	defer r.activityMu.RUnlock()
	if !r.active {
		return radioStatus{
			Idle:           true,
			LastAt:         r.lastAt,
			LastMethod:     r.lastMethod,
			LastDurationMs: r.lastDurationMs,
		}
	}
	st := radioStatus{
		Active: true,
		Method: r.method,
		Since:  r.since,
	}
	if !r.since.IsZero() {
		st.DurationMs = time.Since(r.since).Milliseconds()
	}
	return st
}

func (r *RadioExecutor) start(method string) {
	r.activityMu.Lock()
	r.active = true
	r.method = method
	r.since = time.Now()
	r.activityMu.Unlock()
}

func (r *RadioExecutor) finish() {
	r.activityMu.Lock()
	now := time.Now()
	if r.active && !r.since.IsZero() {
		r.lastMethod = r.method
		r.lastDurationMs = now.Sub(r.since).Milliseconds()
	}
	r.active = false
	r.method = ""
	r.since = time.Time{}
	r.lastAt = now
	r.activityMu.Unlock()
}

func (s *DeviceSession) radioExecutor() *RadioExecutor {
	if s.radio == nil {
		s.radio = NewRadioExecutor(s.trackRadioWaiter)
	}
	return s.radio
}

func (s *DeviceSession) lockRadio(method string) {
	s.radioExecutor().Lock(method)
}

func (s *DeviceSession) unlockRadio() {
	s.radioExecutor().Unlock()
}

func (s *DeviceSession) tryLockRadio(method string) bool {
	return s.radioExecutor().TryLock(method)
}

func (s *DeviceSession) radioStatusLocked() radioStatus {
	return s.radioExecutor().Status()
}
