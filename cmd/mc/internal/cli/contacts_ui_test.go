package cli

import (
	"testing"

	meshcore "github.com/meshcore-cz/meshcore-go"
	localbackend "github.com/meshcore-cz/meshcore-go/backend"
	"github.com/meshcore-cz/meshcore-go/cmd/mc/internal/ui"
)

func TestDeviceStatusFromInfoPreservesCoordinates(t *testing.T) {
	info := meshcore.DeviceInfo{
		Name:      "MeshCore",
		Latitude:  50.479238,
		Longitude: 13.990112,
	}
	dev := deviceStatusFromInfo(info, "serial", true)
	if dev.Latitude != info.Latitude || dev.Longitude != info.Longitude {
		t.Fatalf("device status coords = %v,%v, want %v,%v", dev.Latitude, dev.Longitude, info.Latitude, info.Longitude)
	}
}

func TestContactListMetaFromIPCBackend(t *testing.T) {
	backend := &ipcBackend{
		status: localbackend.Status{
			Healthy: true,
			Contacts: localbackend.ContactStatus{
				Count:        449,
				Syncing:      true,
				SyncReceived: 183,
				SyncTotal:    449,
			},
		},
	}
	meta, warning := contactListMetaFromBackend(backend, 449)
	if warning != "" {
		t.Fatalf("unexpected warning: %q", warning)
	}
	if !meta.Syncing || meta.SyncReceived != 183 {
		t.Fatalf("meta = %+v", meta)
	}

	degraded := &ipcBackend{
		status: localbackend.Status{Healthy: false},
	}
	_, warning = contactListMetaFromBackend(degraded, 10)
	if warning == "" {
		t.Fatal("expected degraded warning")
	}
}

func TestLocalOriginFromIPCBackend(t *testing.T) {
	backend := &ipcBackend{
		status: localbackend.Status{
			Device: localbackend.DeviceStatus{
				Latitude:  50.1,
				Longitude: 14.2,
			},
		},
	}
	lat, lon := localOriginFromBackend(t.Context(), backend)
	if lat != 50.1 || lon != 14.2 {
		t.Fatalf("origin = %v,%v", lat, lon)
	}
	if !ui.HasCoordinates(lat, lon) {
		t.Fatal("expected valid coordinates")
	}
}
