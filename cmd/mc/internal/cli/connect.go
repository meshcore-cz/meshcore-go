package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	meshcore "github.com/meshcore-cz/meshcore-go"
	localbackend "github.com/meshcore-cz/meshcore-go/backend"
	"github.com/meshcore-cz/meshcore-go/cmd/mc/internal/config"
)

// cmdConnect discovers or connects to a radio, verifies it via the handshake
// and (unless --no-save) saves a profile and marks it the default.
func cmdConnect(ctx context.Context, e *env) error {
	uri := e.restArg(0)
	if uri == "" {
		var err error
		uri, err = discoverInteractive(ctx, e)
		if err != nil {
			return err
		}
		if uri == "" {
			return fmt.Errorf("no device selected")
		}
	}

	e.out.Human("Connecting to %s ...\n", uri)
	client, err := meshcore.Dial(ctx, uri, e.dbg.DialOptions()...)
	if err != nil {
		return fmt.Errorf("connecting to %s: %w", uri, err)
	}
	info, err := client.DeviceInfo(ctx)
	if err != nil {
		client.Close()
		return err
	}
	client.Close()
	if schemeOf(uri) == "ble" {
		// Let the OS release the BLE connection before the backend dials again.
		time.Sleep(time.Second)
	}

	e.out.Human("Connected successfully.\n")

	if e.args.has("no-save") {
		if err := maybeOfferBackendStart(ctx, e, uri); err != nil {
			return err
		}
		return e.out.JSONValue(map[string]string{"uri": uri, "name": info.Name, "saved": "false"})
	}

	name := e.args.flag("as")
	if name == "" {
		name = defaultProfileName(info.PublicKey, uri)
		if !e.out.JSON {
			name = promptDefault("Profile name", name)
		}
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	cfg.Put(name, config.Device{
		Name:               info.Name,
		PublicKey:          strings.ToLower(strings.TrimSpace(info.PublicKey)),
		PublicKeyPrefix:    keyPrefix(info.PublicKey),
		PreferredTransport: schemeOf(uri),
		Transports:         []config.Endpoint{{URI: uri}},
	}, true)
	if err := cfg.Save(); err != nil {
		return err
	}

	e.out.Human("Saved profile %q.\n", name)
	e.out.Human("Using %q as the default device.\n", name)
	if err := maybeOfferBackendStart(ctx, e, uri); err != nil {
		return err
	}
	return e.out.JSONValue(map[string]string{"uri": uri, "name": info.Name, "profile": name, "saved": "true"})
}

type sessionStartChoice int

const (
	sessionStartAuto sessionStartChoice = iota // connect now and reconnect on every backend start
	sessionStartYes                            // connect now, this run only
	sessionStartNo                             // leave disconnected
)

func maybeOfferBackendStart(ctx context.Context, e *env, uri string) error {
	if e.out.JSON {
		return nil
	}
	cfg, err := config.Load()
	if err != nil || cfg.Current == "" {
		return nil
	}
	name := cfg.Current

	switch promptSessionStart(fmt.Sprintf("Start a backend session for %q?", name)) {
	case sessionStartNo:
		e.out.Human("Not connected. Start it later with `mc session start %s`.\n", name)
		return nil
	case sessionStartAuto:
		if err := setDeviceAutostart(name, true); err != nil {
			return err
		}
		e.out.Human("Auto-connect enabled for %q.\n", name)
	}

	// Ensure the supervisor is running before connecting the session.
	if _, ok := daemonRunning(ctx, e); !ok {
		if _, err := spawnDaemon(ctx, e); err != nil {
			return err
		}
		e.out.Human("Backend started.\n")
	}

	e.out.Human("Connecting %s...\n", name)
	if _, err := localbackend.NewClient(resolveBackendSocket(e)).DeviceStart(ctx, name); err != nil {
		e.out.Human("Could not start session: %v\n", err)
		return nil
	}
	e.out.Human("Session ready.\n")
	return nil
}

// promptSessionStart asks whether to connect a device's backend session, with
// AUTO (persist autostart + connect now) as the default.
func promptSessionStart(label string) sessionStartChoice {
	fmt.Fprintf(os.Stderr, "%s [AUTO/y/n]: ", label)
	r := bufio.NewReader(os.Stdin)
	line, _ := r.ReadString('\n')
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return sessionStartYes
	case "n", "no":
		return sessionStartNo
	default: // "", "a", "auto", or anything unexpected
		return sessionStartAuto
	}
}

// setDeviceAutostart persists the backend.autostart flag for a saved profile.
func setDeviceAutostart(name string, v bool) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	dev, ok := cfg.Devices[name]
	if !ok {
		return fmt.Errorf("unknown profile %q", name)
	}
	dev.Backend.Autostart = v
	cfg.Devices[name] = dev
	return cfg.Save()
}

// discoverInteractive scans for endpoints and prompts the user to pick one.
func discoverInteractive(ctx context.Context, e *env) (string, error) {
	opts := []meshcore.DiscoverOption{}
	switch {
	case e.args.has("usb"):
		opts = append(opts, meshcore.WithSerialDiscovery())
	case e.args.has("ble"):
		opts = append(opts, meshcore.WithBLEDiscovery())
	default:
		opts = append(opts, meshcore.WithSerialDiscovery(), meshcore.WithBLEDiscovery())
	}

	e.out.Human("Scanning for MeshCore companion radios...\n\n")
	endpoints, err := meshcore.Discover(ctx, opts...)
	if err != nil {
		return "", err
	}
	if len(endpoints) == 0 {
		return "", fmt.Errorf("no companion radios found")
	}

	for i, ep := range endpoints {
		e.out.Human("  %d. %-5s %-22s %s\n", i+1, strings.ToUpper(ep.Transport), ep.Address, ep.Name)
	}
	e.out.Human("\n")

	if len(endpoints) == 1 {
		return endpoints[0].URI, nil
	}
	choice := promptDefault("Select device", "1")
	var idx int
	if _, err := fmt.Sscanf(choice, "%d", &idx); err != nil || idx < 1 || idx > len(endpoints) {
		return "", fmt.Errorf("invalid selection %q", choice)
	}
	return endpoints[idx-1].URI, nil
}

func promptDefault(label, def string) string {
	fmt.Fprintf(os.Stderr, "%s [%s]: ", label, def)
	r := bufio.NewReader(os.Stdin)
	line, _ := r.ReadString('\n')
	line = strings.TrimSpace(line)
	if line == "" {
		return def
	}
	return line
}

func promptYes(label string, defaultYes bool) bool {
	def := "Y/n"
	if !defaultYes {
		def = "y/N"
	}
	fmt.Fprintf(os.Stderr, "%s [%s]: ", label, def)
	r := bufio.NewReader(os.Stdin)
	line, _ := r.ReadString('\n')
	line = strings.ToLower(strings.TrimSpace(line))
	if line == "" {
		return defaultYes
	}
	return line == "y" || line == "yes"
}

func defaultProfileName(publicKey, uri string) string {
	prefix := strings.ToLower(keyPrefix(publicKey))
	if prefix == "" {
		prefix = "radio"
	}
	if transport := schemeOf(uri); transport != "" {
		return transport + ":" + prefix
	}
	return prefix
}

func schemeOf(uri string) string {
	if i := strings.IndexByte(uri, ':'); i >= 0 {
		return uri[:i]
	}
	return ""
}

func keyPrefix(key string) string {
	if len(key) >= 8 {
		return key[:8]
	}
	return key
}
