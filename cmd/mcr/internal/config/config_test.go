package config

import (
	"testing"
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
