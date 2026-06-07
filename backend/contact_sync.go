package backend

import (
	"context"
	"time"
)

func (s *DeviceSession) startContactRefresh(full bool) ContactRefreshResult {
	s.contactSyncMu.Lock()
	if s.contactSyncing {
		s.contactSyncMu.Unlock()
		return ContactRefreshResult{Started: false, Running: true}
	}
	s.contactSyncMu.Unlock()

	go s.runContactRefresh(full)
	return ContactRefreshResult{Started: true, Running: true}
}

func (s *DeviceSession) runContactRefresh(full bool) {
	s.lockRadio("contacts")
	defer s.unlockRadio()

	ctx, cancel := context.WithTimeout(context.Background(), initialContactSyncTimeout)
	s.setContactRefreshCancel(cancel)
	defer s.clearContactRefreshCancel()

	client := s.clientSnapshot()
	if client == nil || !s.healthy() {
		return
	}
	_, err := s.syncContacts(ctx, client, full)
	if err != nil {
		Logf("contact sync failed: %v", err)
	}
}

func (s *DeviceSession) setContactRefreshCancel(cancel context.CancelFunc) {
	s.refreshMu.Lock()
	defer s.refreshMu.Unlock()
	s.refreshCancel = cancel
}

func (s *DeviceSession) clearContactRefreshCancel() {
	s.refreshMu.Lock()
	defer s.refreshMu.Unlock()
	s.refreshCancel = nil
}

func (s *DeviceSession) interruptContactRefresh() {
	s.refreshMu.Lock()
	cancel := s.refreshCancel
	s.refreshCancel = nil
	s.refreshMu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (s *DeviceSession) scheduleContactRefreshAfterInteractive() {
	s.contactSyncMu.Lock()
	running := s.contactSyncing
	s.contactSyncMu.Unlock()
	if running {
		return
	}
	go func() {
		time.Sleep(2 * time.Second)
		s.startContactRefresh(false)
	}()
}
