package backend

import (
	"context"
	"time"

	meshcore "github.com/meshcore-cz/meshcore-go"
)

func (s *DeviceSession) refreshDeviceStatsCache(ctx context.Context, client *meshcore.Client) {
	if client == nil {
		return
	}
	stats, err := client.Stats(ctx)
	if err != nil {
		return
	}
	s.storeDeviceStats(stats)
}

func (s *DeviceSession) refreshStartupCaches(ctx context.Context, client *meshcore.Client) {
	s.refreshDeviceInfoCache(ctx, client)
	s.refreshDeviceStatsCache(ctx, client)
}

func (s *DeviceSession) storeDeviceStats(stats meshcore.LocalStats) {
	s.mu.Lock()
	s.deviceStats = stats
	s.deviceStatsOK = true
	s.deviceStatsAt = time.Now()
	s.mu.Unlock()
}

func (s *DeviceSession) deviceStatsSnapshot() (meshcore.LocalStats, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if !s.deviceStatsOK {
		return meshcore.LocalStats{}, false
	}
	return s.deviceStats, true
}

func (s *DeviceSession) deviceStatsSnapshotLocked() *meshcore.LocalStats {
	if !s.deviceStatsOK {
		return nil
	}
	stats := s.deviceStats
	return &stats
}
