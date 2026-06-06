package cli

import (
	"context"
	"fmt"

	meshcore "github.com/meshcore-cz/meshcore-go"
	"github.com/meshcore-cz/meshcore-go/cmd/mc/internal/config"
)

// resolveURI determines which endpoint a command should use, in priority order:
//
//  1. --uri <uri>            explicit temporary endpoint
//  2. --device <name>        a named saved profile
//  3. the current profile    from the config file
func resolveURI(e *env) (uri, profile string, err error) {
	if u := e.args.flag("uri"); u != "" {
		return u, "", nil
	}

	cfg, err := config.Load()
	if err != nil {
		return "", "", err
	}

	name := e.args.flag("device")
	if name == "" {
		name = cfg.Current
	}
	if name == "" {
		return "", "", fmt.Errorf("no device selected; run `mc connect` or pass --uri")
	}

	dev, ok := cfg.Devices[name]
	if !ok {
		return "", "", fmt.Errorf("unknown profile %q", name)
	}
	uri = dev.PrimaryURI()
	if uri == "" {
		return "", "", fmt.Errorf("profile %q has no endpoint", name)
	}
	return uri, name, nil
}

// connect resolves the endpoint and dials a connected client.
func connect(ctx context.Context, e *env) (*meshcore.Client, string, error) {
	uri, _, err := resolveURI(e)
	if err != nil {
		return nil, "", err
	}
	client, err := meshcore.Dial(ctx, uri, e.dbg.DialOptions()...)
	if err != nil {
		return nil, uri, fmt.Errorf("connecting to %s: %w", uri, err)
	}
	return client, uri, nil
}
