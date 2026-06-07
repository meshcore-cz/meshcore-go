package cli

import (
	"context"
	"fmt"

	localbackend "github.com/meshcore-cz/meshcore-go/backend"
	"github.com/meshcore-cz/meshcore-go/cmd/mc/internal/config"
)

// cmdSession manages device sessions (live radio connections) inside the
// running backend daemon. Profiles themselves are managed with `mc device`.
func cmdSession(ctx context.Context, e *env) error {
	switch e.restArg(0) {
	case "", "list":
		return deviceList(ctx, e)
	case "start":
		return sessionStart(ctx, e)
	case "stop":
		return sessionStop(ctx, e)
	case "restart":
		return sessionRestart(ctx, e)
	default:
		return fmt.Errorf("unknown session subcommand %q", e.restArg(0))
	}
}

// ensureBackend starts the backend daemon (supervisor) if it is not already
// running. The temporary shell backend is left untouched.
func ensureBackend(ctx context.Context, e *env) error {
	if _, ok := daemonRunning(ctx, e); ok {
		return nil
	}
	if e.exec.TemporaryShellBackend {
		return fmt.Errorf("backend is not running")
	}
	e.out.Human("Backend not running; starting it...\n")
	if _, err := spawnDaemon(ctx, e); err != nil {
		return err
	}
	e.out.Human("Backend started.\n")
	return nil
}

// sessionTarget resolves the device a session subcommand targets: an explicit
// argument, else --device, else the current profile.
func sessionTarget(e *env) (string, error) {
	name := e.restArg(1)
	if name == "" {
		name = e.args.flag("device")
	}
	if name == "" {
		cfg, err := config.Load()
		if err != nil {
			return "", err
		}
		name = cfg.Current
	}
	if name == "" {
		return "", fmt.Errorf("no device selected; pass a device name")
	}
	return name, nil
}

func sessionStart(ctx context.Context, e *env) error {
	name, err := sessionTarget(e)
	if err != nil {
		return err
	}
	if err := ensureBackend(ctx, e); err != nil {
		return err
	}
	res, err := localbackend.NewClient(resolveBackendSocket(e)).DeviceStart(ctx, name)
	if err != nil {
		return err
	}
	if res.Changed {
		e.out.Human("Started session for %q.\n", name)
	} else {
		e.out.Human("Session for %q is already running.\n", name)
	}
	return e.out.JSONValue(map[string]any{"device": name, "running": res.Running, "changed": res.Changed})
}

func sessionStop(ctx context.Context, e *env) error {
	name, err := sessionTarget(e)
	if err != nil {
		return err
	}
	if _, ok := daemonRunning(ctx, e); !ok {
		return fmt.Errorf("backend is not running")
	}
	res, err := localbackend.NewClient(resolveBackendSocket(e)).DeviceStop(ctx, name)
	if err != nil {
		return err
	}
	if res.Changed {
		e.out.Human("Stopped session for %q.\n", name)
	} else {
		e.out.Human("Session for %q is not running.\n", name)
	}
	return e.out.JSONValue(map[string]any{"device": name, "running": false, "changed": res.Changed})
}

func sessionRestart(ctx context.Context, e *env) error {
	name, err := sessionTarget(e)
	if err != nil {
		return err
	}
	if err := ensureBackend(ctx, e); err != nil {
		return err
	}
	res, err := localbackend.NewClient(resolveBackendSocket(e)).DeviceRestart(ctx, name)
	if err != nil {
		return err
	}
	e.out.Human("Restarted session for %q.\n", name)
	return e.out.JSONValue(map[string]any{"device": name, "running": res.Running, "changed": res.Changed})
}
