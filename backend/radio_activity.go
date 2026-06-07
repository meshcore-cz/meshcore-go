package backend

import "time"

func (s *Server) lockRadio(method string) {
	s.trackRadioWaiter(1)
	s.radioMu.Lock()
	s.trackRadioWaiter(-1)
	s.mu.Lock()
	s.radioActive = true
	s.radioMethod = method
	s.radioSince = time.Now()
	s.mu.Unlock()
}

func (s *Server) unlockRadio() {
	s.mu.Lock()
	now := time.Now()
	if s.radioActive && !s.radioSince.IsZero() {
		s.radioLastMethod = s.radioMethod
		s.radioLastDurationMs = now.Sub(s.radioSince).Milliseconds()
	}
	s.radioActive = false
	s.radioMethod = ""
	s.radioSince = time.Time{}
	s.radioLastAt = now
	s.mu.Unlock()
	s.radioMu.Unlock()
}

func (s *Server) tryLockRadio(method string) bool {
	if !s.radioMu.TryLock() {
		return false
	}
	s.mu.Lock()
	s.radioActive = true
	s.radioMethod = method
	s.radioSince = time.Now()
	s.mu.Unlock()
	return true
}

func (s *Server) radioStatusLocked() radioStatus {
	if !s.radioActive {
		return radioStatus{
			Idle:           true,
			LastAt:         s.radioLastAt,
			LastMethod:     s.radioLastMethod,
			LastDurationMs: s.radioLastDurationMs,
		}
	}
	st := radioStatus{
		Active: true,
		Method: s.radioMethod,
		Since:  s.radioSince,
	}
	if !s.radioSince.IsZero() {
		st.DurationMs = time.Since(s.radioSince).Milliseconds()
	}
	return st
}
