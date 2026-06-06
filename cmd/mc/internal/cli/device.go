package cli

import (
	"context"
	"fmt"
	"sort"

	"github.com/meshcore-cz/meshcore-go/cmd/mc/internal/config"
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
		return deviceShow(e)
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
	names := make([]string, 0, len(cfg.Devices))
	for n := range cfg.Devices {
		names = append(names, n)
	}
	sort.Strings(names)

	if e.out.JSON {
		type row struct {
			Name      string `json:"name"`
			Transport string `json:"transport"`
			Endpoint  string `json:"endpoint"`
			Default   bool   `json:"default"`
			Backend   bool   `json:"backend"`
		}
		rows := make([]row, 0, len(names))
		for _, n := range names {
			d := cfg.Devices[n]
			uri := d.PrimaryURI()
			rows = append(rows, row{n, d.PreferredTransport, uri, n == cfg.Current, backendRunning && uri == st.URI})
		}
		return e.out.JSONValue(rows)
	}

	if len(names) == 0 {
		e.out.Human("No saved profiles. Run `mc connect`.\n")
		return nil
	}
	e.out.Human("%-16s %-10s %-34s %-7s %s\n", "NAME", "TRANSPORT", "ENDPOINT", "BACKEND", "DEFAULT")
	for _, n := range names {
		d := cfg.Devices[n]
		def := ""
		if n == cfg.Current {
			def = "*"
		}
		uri := d.PrimaryURI()
		active := ""
		if backendRunning && uri == st.URI {
			active = "running"
		}
		e.out.Human("%-16s %-10s %-34s %-7s %s\n", n, d.PreferredTransport, uri, active, def)
	}
	return nil
}

func deviceShow(e *env) error {
	name := e.restArg(1)
	if name == "" {
		return fmt.Errorf("usage: mc device show <name>")
	}
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
