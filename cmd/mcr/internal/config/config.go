// Package config loads and saves the mcr CLI configuration, including saved
// device profiles and the active device selection.
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config is the on-disk CLI configuration.
type Config struct {
	Version         int                 `yaml:"version"`
	Current         string              `yaml:"current,omitempty"`
	CurrentRepeater string              `yaml:"current_repeater,omitempty"`
	Devices         map[string]Device   `yaml:"devices,omitempty"`
	Repeaters       map[string]Repeater `yaml:"repeaters,omitempty"`
}

// Device is a saved profile. A logical device may carry multiple endpoints.
type Device struct {
	Name               string     `yaml:"name,omitempty"`
	PublicKeyPrefix    string     `yaml:"public_key_prefix,omitempty"`
	PreferredTransport string     `yaml:"preferred_transport,omitempty"`
	Transports         []Endpoint `yaml:"transports,omitempty"`
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
		if d.PreferredTransport == "" || schemeOf(t.URI) == d.PreferredTransport {
			return t.URI
		}
	}
	if len(d.Transports) > 0 {
		return d.Transports[0].URI
	}
	return ""
}

func schemeOf(uri string) string {
	for i := 0; i < len(uri); i++ {
		if uri[i] == ':' {
			return uri[:i]
		}
	}
	return ""
}

// Path returns the configuration file path, honouring $XDG_CONFIG_HOME.
func Path() (string, error) {
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		return filepath.Join(x, "mcr", "config.yaml"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "mcr", "config.yaml"), nil
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

// PutRepeater adds or replaces a repeater profile and optionally marks it current.
func (c *Config) PutRepeater(name string, rep Repeater, makeCurrent bool) {
	if c.Repeaters == nil {
		c.Repeaters = map[string]Repeater{}
	}
	if rep.Name == "" {
		rep.Name = name
	}
	c.Repeaters[name] = rep
	if makeCurrent || c.CurrentRepeater == "" {
		c.CurrentRepeater = name
	}
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
