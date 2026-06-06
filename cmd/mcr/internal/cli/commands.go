package cli

import (
	"context"
	"runtime"
	"strings"
	"time"

	meshcore "github.com/meshcore-dev/meshcore-go"
)

func clockDelta(deviceTime time.Time) string {
	d := time.Since(deviceTime)
	if d < 0 {
		d = -d
	}
	return d.Round(time.Second).String()
}

func cmdVersion(e *env) error {
	info := map[string]string{
		"mcr":      Version,
		"meshcore": Version,
		"commit":   Commit,
		"go":       runtime.Version(),
		"os":       runtime.GOOS,
		"arch":     runtime.GOARCH,
	}
	if err := e.out.JSONValue(info); err != nil {
		return err
	}
	e.out.Human("mcr        %s\n", Version)
	e.out.Human("meshcore   %s\n", Version)
	e.out.Human("commit     %s\n", Commit)
	e.out.Human("go         %s\n", runtime.Version())
	e.out.Human("os         %s\n", runtime.GOOS)
	e.out.Human("arch       %s\n", runtime.GOARCH)
	return nil
}

func cmdStatus(ctx context.Context, e *env) error {
	client, uri, err := connect(ctx, e)
	if err != nil {
		return err
	}
	defer client.Close()

	info, err := client.DeviceInfo(ctx)
	if err != nil {
		return err
	}

	type statusJSON struct {
		Name            string   `json:"name"`
		Firmware        string   `json:"firmware"`
		FirmwareVersion string   `json:"firmware_version"`
		Protocol        string   `json:"protocol"`
		Transport       string   `json:"transport"`
		PublicKey       string   `json:"public_key"`
		Capabilities    []string `json:"capabilities"`
	}
	if e.out.JSON {
		return e.out.JSONValue(statusJSON{
			Name:            info.Name,
			Firmware:        info.FirmwareName,
			FirmwareVersion: info.FirmwareVersion,
			Protocol:        info.ProtocolVersion,
			Transport:       client.Transport(),
			PublicKey:       info.PublicKey,
			Capabilities:    info.Capabilities.List(),
		})
	}

	e.out.Human("Device:       %s\n", orDash(info.Name))
	e.out.Human("Firmware:     %s %s\n", info.FirmwareName, info.FirmwareVersion)
	e.out.Human("Protocol:     %s\n", orDash(info.ProtocolVersion))
	e.out.Human("Transport:    %s\n", uri)
	e.out.Human("Public key:   %s\n", shortKey(info.PublicKey))
	e.out.Human("Capabilities: %s\n", info.Capabilities.String())
	return nil
}

func cmdDoctor(ctx context.Context, e *env) error {
	type check struct {
		Name   string `json:"name"`
		Result string `json:"result"`
		OK     bool   `json:"ok"`
	}
	var checks []check
	add := func(name, result string, ok bool) {
		checks = append(checks, check{Name: name, Result: result, OK: ok})
		e.out.Human("%-34s %s\n", name, result)
	}

	uri, profile, err := resolveURI(e)
	if err != nil {
		add("Configuration", err.Error(), false)
		return finishDoctor(e, checks)
	}
	add("Configuration file", "ok", true)
	if profile != "" {
		add("Default profile", profile, true)
	}
	add("Endpoint", uri, true)

	client, err := meshcore.Dial(ctx, uri)
	if err != nil {
		add("Companion radio", "unreachable: "+err.Error(), false)
		return finishDoctor(e, checks)
	}
	defer client.Close()
	add("Companion radio", "reachable", true)
	add("Protocol handshake", "ok", true)

	info, _ := client.DeviceInfo(ctx)
	add("Firmware", strings.TrimSpace(info.FirmwareName+" "+info.FirmwareVersion), true)
	add("Protocol", orDash(info.ProtocolVersion), true)

	if t, err := client.DeviceTime(ctx); err == nil {
		add("Clock difference", clockDelta(t), true)
	}

	return finishDoctor(e, checks)
}

func finishDoctor(e *env, checks any) error {
	return e.out.JSONValue(checks)
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func shortKey(k string) string {
	if len(k) > 12 {
		return k[:12] + "..."
	}
	return orDash(k)
}
