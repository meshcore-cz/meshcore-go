// Package config loads and saves the mc CLI configuration, including saved
// device profiles and the active device selection.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/meshcore-cz/meshcore-go/transport"
	"gopkg.in/yaml.v3"
)

// Config is the on-disk CLI configuration.
type Config struct {
	Version         int                 `yaml:"version"`
	Current         string              `yaml:"current,omitempty"`
	CurrentRepeater string              `yaml:"current_repeater,omitempty"`
	Backend         Backend             `yaml:"backend,omitempty"`
	Devices         map[string]Device   `yaml:"devices,omitempty"`
	Repeaters       map[string]Repeater `yaml:"repeaters,omitempty"`
}

// Backend configures the local backend process.
type Backend struct {
	LogRequests bool       `yaml:"log_requests,omitempty"`
	Bridges     []Bridge   `yaml:"bridges,omitempty"`
	HTTP        HTTPConfig `yaml:"http,omitempty"`
}

// HTTPConfig configures the optional embedded web dashboard served by the
// backend daemon.
type HTTPConfig struct {
	Enabled bool   `yaml:"enabled"`
	Port    int    `yaml:"port,omitempty"` // listen port, default 8080 when enabled
	Host    string `yaml:"host,omitempty"` // bind address, default 127.0.0.1
}

// DefaultHTTPHost is the loopback bind address used when HTTPConfig.Host is
// unset. Set Host to 0.0.0.0 to expose the dashboard on the LAN.
const DefaultHTTPHost = "127.0.0.1"

// DefaultHTTPPort is the listen port used when HTTPConfig.Port is unset.
const DefaultHTTPPort = 8080

// Addr returns the host:port the dashboard should bind, applying defaults.
func (h HTTPConfig) Addr() string {
	host := h.Host
	if host == "" {
		host = DefaultHTTPHost
	}
	port := h.Port
	if port == 0 {
		port = DefaultHTTPPort
	}
	return fmt.Sprintf("%s:%d", host, port)
}

// Bridge configures one local bridge listener exposed by the backend.
type Bridge struct {
	Enabled bool   `yaml:"enabled"`
	Type    string `yaml:"type"`             // tcp or pty
	Listen  string `yaml:"listen,omitempty"` // tcp listen address
	Name    string `yaml:"name,omitempty"`
}

// Device is a saved profile. A logical device may carry multiple endpoints.
type Device struct {
	Name               string        `yaml:"name,omitempty"`
	PublicKey          string        `yaml:"public_key,omitempty"`
	PublicKeyPrefix    string        `yaml:"public_key_prefix,omitempty"`
	PreferredTransport string        `yaml:"preferred_transport,omitempty"`
	Transports         []Endpoint    `yaml:"transports,omitempty"`
	Backend            DeviceBackend `yaml:"backend,omitempty"`
}

// DeviceBackend controls how the daemon manages this device's session.
type DeviceBackend struct {
	// Autostart connects the device when the daemon starts and keeps it
	// reconnecting; otherwise the session starts on `mc session start`.
	Autostart bool `yaml:"autostart,omitempty"`
	// Bridges are local bridge listeners exposing this device's radio.
	Bridges []Bridge `yaml:"bridges,omitempty"`
}

// Endpoint is a single saved transport URI plus optional transport options.
type Endpoint struct {
	URI     string         `yaml:"uri"`
	Options map[string]any `yaml:"options,omitempty"`
}

// Repeater is a saved remote repeater profile.
type Repeater struct {
	Name     string `yaml:"name,omitempty"`
	Password string `yaml:"password,omitempty"`
}

// PrimaryURI returns the device's preferred endpoint URI.
func (d Device) PrimaryURI() string {
	for _, t := range d.Transports {
		if d.PreferredTransport == "" || transport.Scheme(t.URI) == d.PreferredTransport {
			return t.URI
		}
	}
	if len(d.Transports) > 0 {
		return d.Transports[0].URI
	}
	return ""
}

// Path returns the configuration file path, honouring $XDG_CONFIG_HOME.
func Path() (string, error) {
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		return filepath.Join(x, "mc", "config.yaml"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "mc", "config.yaml"), nil
}

// Load reads the configuration. A missing file yields an empty default config.
func Load() (*Config, error) {
	path, err := Path()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &Config{Version: 1, Devices: map[string]Device{}, Repeaters: map[string]Repeater{}}, nil
	}
	if err != nil {
		return nil, err
	}
	cfg := &Config{}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("config: parse %s: %w", path, err)
	}
	if cfg.Devices == nil {
		cfg.Devices = map[string]Device{}
	}
	if cfg.Repeaters == nil {
		cfg.Repeaters = map[string]Repeater{}
	}
	if cfg.Version == 0 {
		cfg.Version = 1
	}
	return cfg, nil
}

// NormalizePublicKey lowercases a contact public key for use as a map key.
func NormalizePublicKey(key string) string {
	return strings.ToLower(strings.TrimSpace(key))
}

// IsPublicKey reports whether s looks like a full 32-byte hex public key.
func IsPublicKey(s string) bool {
	s = NormalizePublicKey(s)
	if len(s) != 64 {
		return false
	}
	for _, c := range s {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

// PutRepeater adds or replaces a repeater profile keyed by public key and
// optionally marks it current.
func (c *Config) PutRepeater(publicKey string, rep Repeater, makeCurrent bool) {
	if c.Repeaters == nil {
		c.Repeaters = map[string]Repeater{}
	}
	publicKey = NormalizePublicKey(publicKey)
	c.Repeaters[publicKey] = rep
	if makeCurrent || c.CurrentRepeater == "" {
		c.CurrentRepeater = publicKey
	}
}

// RepeaterByKey returns a saved repeater by full public key.
func (c *Config) RepeaterByKey(publicKey string) (Repeater, bool) {
	publicKey = NormalizePublicKey(publicKey)
	rep, ok := c.Repeaters[publicKey]
	return rep, ok
}

// MatchRepeater finds a saved repeater by full public key, key prefix, saved
// name, or a legacy name-keyed entry.
func (c *Config) MatchRepeater(query string) (publicKey string, rep Repeater, ok bool) {
	query = strings.TrimSpace(query)
	if query == "" {
		return "", Repeater{}, false
	}
	q := strings.ToLower(query)
	for key, rep := range c.Repeaters {
		nk := NormalizePublicKey(key)
		if strings.EqualFold(nk, query) ||
			strings.EqualFold(rep.Name, query) ||
			strings.EqualFold(key, query) {
			return key, rep, true
		}
		if len(q) >= 6 && strings.HasPrefix(nk, q) {
			return key, rep, true
		}
	}
	return "", Repeater{}, false
}

// RemoveRepeater deletes a saved repeater profile and clears the current
// selection when it pointed there.
func (c *Config) RemoveRepeater(query string) (publicKey string, rep Repeater, removed bool) {
	publicKey, rep, ok := c.MatchRepeater(query)
	if !ok {
		return "", Repeater{}, false
	}
	delete(c.Repeaters, publicKey)
	if NormalizePublicKey(c.CurrentRepeater) == NormalizePublicKey(publicKey) {
		c.CurrentRepeater = ""
	}
	return publicKey, rep, true
}

// CurrentRepeaterName returns the saved display name for the current repeater.
func (c *Config) CurrentRepeaterName() string {
	if c.CurrentRepeater == "" {
		return ""
	}
	if rep, ok := c.Repeaters[c.CurrentRepeater]; ok && rep.Name != "" {
		return rep.Name
	}
	if !IsPublicKey(c.CurrentRepeater) {
		return c.CurrentRepeater
	}
	return ""
}

// Save writes the configuration, creating the directory if necessary.
func (c *Config) Save() error {
	path, err := Path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

// Put adds or replaces a profile and optionally marks it current.
func (c *Config) Put(name string, dev Device, makeCurrent bool) {
	if c.Devices == nil {
		c.Devices = map[string]Device{}
	}
	c.Devices[name] = dev
	if makeCurrent || c.Current == "" {
		c.Current = name
	}
}

// Remove deletes a profile, clearing the current selection if it pointed there.
func (c *Config) Remove(name string) bool {
	if _, ok := c.Devices[name]; !ok {
		return false
	}
	delete(c.Devices, name)
	if c.Current == name {
		c.Current = ""
	}
	return true
}
