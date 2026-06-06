package cli

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	meshcore "github.com/meshcore-dev/meshcore-go"
	"github.com/meshcore-dev/meshcore-go/cmd/mcr/internal/config"
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
	defer client.Close()

	info, err := client.DeviceInfo(ctx)
	if err != nil {
		return err
	}
	e.out.Human("Connected successfully.\n")

	if e.args.has("no-save") {
		return e.out.JSONValue(map[string]string{"uri": uri, "name": info.Name, "saved": "false"})
	}

	name := e.args.flag("as")
	if name == "" {
		name = profileName(info.Name)
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
		PublicKeyPrefix:    keyPrefix(info.PublicKey),
		PreferredTransport: schemeOf(uri),
		Transports:         []config.Endpoint{{URI: uri}},
	}, true)
	if err := cfg.Save(); err != nil {
		return err
	}

	e.out.Human("Saved profile %q.\n", name)
	e.out.Human("Using %q as the default device.\n", name)
	return e.out.JSONValue(map[string]string{"uri": uri, "name": info.Name, "profile": name, "saved": "true"})
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

func profileName(deviceName string) string {
	n := strings.TrimPrefix(deviceName, "MeshCore-")
	n = strings.ToLower(strings.TrimSpace(n))
	if n == "" {
		return "radio"
	}
	return n
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
