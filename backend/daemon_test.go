package backend

import (
	"strings"
	"testing"
)

// TestDaemonNoAutostartStaysStopped verifies that registering a device without
// autostart does not create a running session, and that the daemon-level status
// and device-list views report it as stopped while the daemon itself is up.
//
// This exercises the routing/status logic directly (no Unix listener), so it is
// safe in sandboxes that block socket I/O in test binaries.
func TestDaemonNoAutostartStaysStopped(t *testing.T) {
	d, err := NewDaemon(DaemonOptions{Socket: "/tmp/unused.sock"})
	if err != nil {
		t.Fatalf("NewDaemon: %v", err)
	}
	d.Register(SessionProfile{ID: "deskradio", URI: "serial:///dev/ttyNONEXISTENT", Autostart: false})
	d.SetDefault("deskradio")

	// No Serve and no autostart: nothing should have dialed.
	st := d.backendStatus()
	if !st.Running {
		t.Fatal("backendStatus.Running = false")
	}
	if len(st.Devices) != 1 {
		t.Fatalf("devices = %d, want 1", len(st.Devices))
	}
	if e := st.Devices[0]; e.ID != "deskradio" || e.Session != "stopped" || e.Connected {
		t.Fatalf("entry = %+v, want stopped/disconnected deskradio", e)
	}
	if st.DefaultID != "deskradio" {
		t.Fatalf("default = %q, want deskradio", st.DefaultID)
	}

	// Routing to a registered-but-stopped device must report that it is not
	// running (so the CLI falls back to a direct dial), not "unknown".
	_, err = d.session("deskradio")
	if err == nil {
		t.Fatal("session(deskradio) = nil error, want not-running error")
	}
	if !strings.Contains(err.Error(), "not running") {
		t.Fatalf("session error = %v, want 'not running'", err)
	}

	// An unregistered device is a distinct error.
	if _, err := d.session("ghost"); err == nil || !strings.Contains(err.Error(), "unknown device") {
		t.Fatalf("session(ghost) = %v, want 'unknown device'", err)
	}
}

// TestDaemonStopSessionReportsChange verifies that stopping a session that is
// not running reports no change (so the CLI can say "not running" instead of
// "stopped"), and that unknown devices error.
func TestDaemonStopSessionReportsChange(t *testing.T) {
	d, err := NewDaemon(DaemonOptions{Socket: "/tmp/unused.sock"})
	if err != nil {
		t.Fatalf("NewDaemon: %v", err)
	}
	d.Register(SessionProfile{ID: "deskradio", URI: "serial:///dev/x", Autostart: false})

	stopped, err := d.stopSession("deskradio")
	if err != nil {
		t.Fatalf("stopSession(deskradio) error = %v", err)
	}
	if stopped {
		t.Fatal("stopSession reported a change for a session that was not running")
	}

	if _, err := d.stopSession("ghost"); err == nil || !strings.Contains(err.Error(), "unknown device") {
		t.Fatalf("stopSession(ghost) = %v, want 'unknown device'", err)
	}

	if d.runningSession("deskradio") {
		t.Fatal("runningSession(deskradio) = true, want false")
	}
}

// TestDaemonAutostartProfilesSelected verifies that only autostart profiles are
// chosen for connection at serve time.
func TestDaemonAutostartProfilesSelected(t *testing.T) {
	d, err := NewDaemon(DaemonOptions{Socket: "/tmp/unused.sock"})
	if err != nil {
		t.Fatalf("NewDaemon: %v", err)
	}
	d.Register(SessionProfile{ID: "handheld", URI: "ble://x", Autostart: false})
	d.Register(SessionProfile{ID: "deskradio", URI: "serial:///dev/x", Autostart: true})

	var autostart []string
	for _, id := range d.profileIDs() {
		if p, ok := d.profile(id); ok && p.Autostart {
			autostart = append(autostart, id)
		}
	}
	if len(autostart) != 1 || autostart[0] != "deskradio" {
		t.Fatalf("autostart profiles = %v, want [deskradio]", autostart)
	}
}
