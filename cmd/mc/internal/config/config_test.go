package config

import (
	"testing"
	"time"
)

func TestSaveLoadRoundTrip(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	cfg := &Config{Version: 1, Devices: map[string]Device{}}
	cfg.Put("handheld", Device{
		Name:               "MeshCore-handheld",
		PublicKeyPrefix:    "0a53ef34",
		PreferredTransport: "ble",
		Transports:         []Endpoint{{URI: "ble://C4:20:12:34:56:78"}},
	}, true)
	if err := cfg.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Current != "handheld" {
		t.Errorf("current = %q, want handheld", loaded.Current)
	}
	dev, ok := loaded.Devices["handheld"]
	if !ok {
		t.Fatal("handheld profile missing")
	}
	if got := dev.PrimaryURI(); got != "ble://C4:20:12:34:56:78" {
		t.Errorf("PrimaryURI = %q", got)
	}
}

func TestBackendLogRequestsRoundTrip(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	cfg := &Config{Version: 1, Backend: Backend{LogRequests: true}}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if !loaded.Backend.LogRequests {
		t.Fatal("log_requests not persisted")
	}
}

func TestLoadMissingReturnsEmpty(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Version != 1 || len(cfg.Devices) != 0 {
		t.Errorf("unexpected default config %+v", cfg)
	}
}

func TestRemoveClearsCurrent(t *testing.T) {
	cfg := &Config{Version: 1, Devices: map[string]Device{"a": {}}, Current: "a"}
	if !cfg.Remove("a") {
		t.Fatal("Remove returned false")
	}
	if cfg.Current != "" {
		t.Errorf("current = %q, want empty", cfg.Current)
	}
	if cfg.Remove("missing") {
		t.Error("Remove of missing profile should be false")
	}
}

func TestPutRepeaterKeysByPublicKey(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	key := "252525ce5267abcd252525ce5267abcd252525ce5267abcd252525ce5267abcd"
	cfg := &Config{Version: 1, Repeaters: map[string]Repeater{}}
	cfg.PutRepeater(key, Repeater{Name: "mc.kololec.cz", Password: "secret"}, true)
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}

	loaded, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.CurrentRepeater != key {
		t.Fatalf("current = %q, want %q", loaded.CurrentRepeater, key)
	}
	rep, ok := loaded.RepeaterByKey(key)
	if !ok || rep.Name != "mc.kololec.cz" || rep.Password != "secret" {
		t.Fatalf("repeater = %#v, ok=%v", rep, ok)
	}
	if _, rep, ok := loaded.MatchRepeater("mc.kololec.cz"); !ok || rep.Name != "mc.kololec.cz" {
		t.Fatalf("match by name failed: %#v ok=%v", rep, ok)
	}
	if _, rep, ok := loaded.MatchRepeater("252525ce5267"); !ok || rep.Name != "mc.kololec.cz" {
		t.Fatalf("match by prefix failed: %#v ok=%v", rep, ok)
	}
}

func TestRemoveRepeaterClearsCurrent(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	key := "252525ce5267abcd252525ce5267abcd252525ce5267abcd252525ce5267abcd"
	cfg := &Config{Version: 1, Repeaters: map[string]Repeater{}}
	cfg.PutRepeater(key, Repeater{Name: "mc.kololec.cz"}, true)

	gotKey, rep, ok := cfg.RemoveRepeater("mc.kololec.cz")
	if !ok || gotKey != key || rep.Name != "mc.kololec.cz" {
		t.Fatalf("RemoveRepeater = %q %#v ok=%v", gotKey, rep, ok)
	}
	if cfg.CurrentRepeater != "" {
		t.Fatalf("current = %q, want empty", cfg.CurrentRepeater)
	}
	if _, ok := cfg.Repeaters[key]; ok {
		t.Fatal("repeater profile should be removed")
	}
	if _, _, ok := cfg.RemoveRepeater("missing"); ok {
		t.Fatal("RemoveRepeater of missing repeater should be false")
	}
}

func TestRepeaterSessionRoundTrip(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	sess := RepeaterSession{
		Name:        "mc.kololec.cz",
		PublicKey:   "252525ce5267abcd252525ce5267abcd252525ce5267abcd252525ce5267abcd",
		LoggedInAt:  time.Now().UTC().Truncate(time.Second),
		ExpiresAt:   time.Now().UTC().Add(30 * time.Minute).Truncate(time.Second),
		Permissions: 1,
		Tag:         42,
	}
	if err := SaveRepeaterSession(sess); err != nil {
		t.Fatal(err)
	}
	loaded, ok, err := LoadRepeaterSession(sess.PublicKey)
	if err != nil || !ok {
		t.Fatalf("LoadRepeaterSession: ok=%v err=%v", ok, err)
	}
	if loaded.Name != sess.Name || loaded.Tag != sess.Tag || !loaded.Active() {
		t.Fatalf("loaded %#v", loaded)
	}
	dir, err := SessionsDir()
	if err != nil || dir == "" {
		t.Fatalf("SessionsDir: %q err=%v", dir, err)
	}

	cfg := &Config{
		Repeaters: map[string]Repeater{
			sess.PublicKey: {Name: sess.Name},
		},
		CurrentRepeater: sess.PublicKey,
	}
	if got, ok := CachedRepeaterSession(cfg, sess.Name); !ok || got.Tag != sess.Tag {
		t.Fatalf("CachedRepeaterSession by name: got %#v ok=%v", got, ok)
	}
}

func TestPrimaryURIPrefersTransport(t *testing.T) {
	d := Device{
		PreferredTransport: "serial",
		Transports: []Endpoint{
			{URI: "ble://aa"},
			{URI: "serial:///dev/ttyACM0"},
		},
	}
	if got := d.PrimaryURI(); got != "serial:///dev/ttyACM0" {
		t.Errorf("PrimaryURI = %q", got)
	}
}

func TestRepeaterRoundTrip(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	cfg := &Config{Version: 1}
	cfg.PutRepeater("mc.kololec.cz", Repeater{Name: "mc.kololec.cz", Password: "secret"}, true)
	if err := cfg.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.CurrentRepeater != "mc.kololec.cz" {
		t.Errorf("current repeater = %q", loaded.CurrentRepeater)
	}
	if got := loaded.Repeaters["mc.kololec.cz"].Password; got != "secret" {
		t.Errorf("password = %q", got)
	}
}
