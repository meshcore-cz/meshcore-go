package cli

import (
	"context"
	"fmt"
	"time"

	meshcore "github.com/meshcore-cz/meshcore-go"
	localbackend "github.com/meshcore-cz/meshcore-go/backend"
	"github.com/meshcore-cz/meshcore-go/cmd/mc/internal/config"
	"github.com/meshcore-cz/meshcore-go/cmd/mc/internal/ui"
)

func cmdUse(e *env) error {
	name := e.restArg(0)
	if name == "" {
		return fmt.Errorf("usage: mc use <profile>")
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if _, ok := cfg.Devices[name]; !ok {
		return fmt.Errorf("unknown profile %q", name)
	}
	cfg.Current = name
	if err := cfg.Save(); err != nil {
		return err
	}
	e.out.Human("Now using %q.\n", name)
	return e.out.JSONValue(map[string]string{"current": name})
}

func cmdDevice(ctx context.Context, e *env) error {
	switch e.restArg(0) {
	case "", "list":
		return deviceList(ctx, e)
	case "show":
		return deviceShow(ctx, e)
	case "remove":
		return deviceRemove(e)
	default:
		return fmt.Errorf("unknown device subcommand %q", e.restArg(0))
	}
}

func deviceList(ctx context.Context, e *env) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	st, backendRunning := backendStatus(ctx)

	if e.out.JSON {
		return e.out.JSONValue(deviceListJSON(cfg, st, backendRunning))
	}

	if len(cfg.Devices) == 0 {
		e.out.Human("No saved profiles. Run `mc connect`.\n")
		return nil
	}

	data := deviceListData(cfg, st, backendRunning)
	data.Wide = e.args.has("wide")
	printer := ui.NewPrinter(e.out.Out)
	printer.Print(ui.RenderDeviceList(data, printer))
	return nil
}

func deviceShow(ctx context.Context, e *env) error {
	name := e.restArg(1)
	if name == "" {
		return deviceShowConnected(ctx, e)
	}
	return deviceShowProfile(e, name)
}

func deviceShowConnected(ctx context.Context, e *env) error {
	st, backendRunning := backendStatus(ctx)
	if backendRunning && st.Healthy && !e.args.has("direct") && st.Device.Available() {
		dev := st.Device
		dev.Transport = st.Transport
		return printDeviceShow(e, st, dev)
	}

	backend, err := openBackend(ctx, e)
	if err != nil {
		return err
	}
	defer backend.Close()

	info, err := backend.DeviceInfo(ctx)
	if err != nil {
		return err
	}
	dev := deviceStatusFromInfo(info, backend.Transport(), backendRunning)
	return printDeviceShow(e, st, dev)
}

func printDeviceShow(e *env, st localbackend.Status, dev localbackend.DeviceStatus) error {
	info := deviceInfoFromBackend(st, dev, meshcore.LocalStats{}, false, time.Time{})
	if e.out.JSON {
		return e.out.JSONValue(map[string]any{
			"name":             info.Name,
			"firmware":         info.Firmware,
			"firmware_version": info.FirmwareVersion,
			"protocol":         info.Protocol,
			"transport":        info.Transport,
			"endpoint":         info.TransportURI,
			"public_key":       info.PublicKey,
			"capabilities":     info.Capabilities,
		})
	}
	printer := ui.NewPrinter(e.out.Out)
	printer.Print(ui.RenderDeviceShow(info, printer))
	return nil
}

func deviceShowProfile(e *env, name string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	d, ok := cfg.Devices[name]
	if !ok {
		return fmt.Errorf("unknown profile %q", name)
	}
	if e.out.JSON {
		return e.out.JSONValue(d)
	}
	e.out.Human("Profile:     %s\n", name)
	e.out.Human("Name:        %s\n", orDash(d.Name))
	e.out.Human("Public key:  %s\n", orDash(d.PublicKeyPrefix))
	e.out.Human("Transport:   %s\n", orDash(d.PreferredTransport))
	for _, t := range d.Transports {
		e.out.Human("Endpoint:    %s\n", t.URI)
	}
	return nil
}

func deviceRemove(e *env) error {
	name := e.restArg(1)
	if name == "" {
		return fmt.Errorf("usage: mc device remove <name>")
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if !cfg.Remove(name) {
		return fmt.Errorf("unknown profile %q", name)
	}
	if err := cfg.Save(); err != nil {
		return err
	}
	e.out.Human("Removed profile %q.\n", name)
	return e.out.JSONValue(map[string]string{"removed": name})
}
