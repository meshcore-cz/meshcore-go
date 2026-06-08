// Package cli implements the mc command-line client. It is intentionally thin:
// anything related to the companion protocol lives in the reusable SDK.
package cli

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/meshcore-cz/meshcore-go/cmd/mc/internal/output"
	"github.com/meshcore-cz/meshcore-go/transport/serial"
)

// version metadata, overridable at build time with -ldflags.
var (
	Version = "v0.1.0"
	Commit  = "unknown"
)

// commandAliasSubcommand supplies a default subcommand for alias-only commands.
var commandAliasSubcommand = map[string]string{
	"?":    "",
	"d":    "list",
	"h":    "",
	"log":  "log",
	"ls":   "list",
	"list": "list",
}

// ExecuteOptions configures shared command execution.
type ExecuteOptions struct {
	// BackendSocket overrides the normal per-device backend socket.
	// Used by mc shell when it starts a private temporary backend.
	BackendSocket string

	// RequireIPC prevents silent fallback to a new direct radio connection.
	// A shell session should keep using its session backend.
	RequireIPC bool

	// InShell allows commands to adapt behavior where necessary.
	InShell bool

	// TemporaryShellBackend marks the session backend as owned by mc shell.
	TemporaryShellBackend bool
}

// resolveAlias returns the canonical command name for cmd, or cmd unchanged.
func resolveAlias(cmd string) string {
	if spec, ok := findCommandSpec(commandRegistry, cmd); ok {
		return spec.Name
	}
	return cmd
}

// Execute parses arguments and runs a single command.
func Execute(ctx context.Context, args []string, opts ExecuteOptions) error {
	pa, err := parseArgs(args)
	if err != nil {
		return err
	}

	originalCmd := pa.arg(0)
	cmd := resolveAlias(originalCmd)
	wantHelp := pa.has("help") || pa.has("h")

	if cmd == "help" {
		if sub := pa.arg(1); sub != "" {
			return writeCommandHelp(sub)
		}
		usage()
		if opts.InShell {
			printShellHelpFooter()
		}
		return nil
	}

	if cmd == "" {
		return fmt.Errorf("missing command")
	}

	if wantHelp {
		return writeCommandHelp(cmd)
	}

	rest := pa.positionals[1:]
	if sub, ok := commandAliasSubcommand[originalCmd]; ok {
		rest = append([]string{sub}, rest...)
	}

	e := &env{
		args: pa,
		rest: rest,
		out:  output.New(pa.has("json")),
		dbg:  newDebug(pa),
		exec: opts,
	}

	return dispatch(ctx, cmd, e)
}

func dispatch(ctx context.Context, cmd string, e *env) error {
	if cmd == "shell" {
		if e.exec.InShell {
			return fmt.Errorf("already running inside mc shell")
		}
		return cmdShell(ctx, e)
	}
	spec, ok := findCommandSpec(commandRegistry, cmd)
	if !ok || spec.Run == nil {
		return fmt.Errorf("unknown command %q", cmd)
	}
	return spec.Run(ctx, e)
}

// Run parses arguments and dispatches to a subcommand. It returns a process
// exit code.
func Run(args []string) int {
	pa, err := parseArgs(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, "mc:", err)
		return 2
	}

	originalCmd := pa.arg(0)
	cmd := resolveAlias(originalCmd)
	if cmd == "" {
		usage()
		if pa.has("help") || pa.has("h") {
			return 0
		}
		return 2
	}

	err = Execute(context.Background(), args, ExecuteOptions{})
	if err == nil {
		return 0
	}

	fmt.Fprintln(os.Stderr, "mc:", err)

	if errors.Is(err, serial.ErrBusy) {
		fmt.Fprintln(os.Stderr, "hint: another program is using the serial port "+
			"(serial monitor, firmware flasher, or another mc). Close it and retry.")
	}

	return 1
}

// env carries parsed arguments and the output printer to each command.
type env struct {
	args parsedArgs
	rest []string // positional args after the subcommand
	out  *output.Printer
	dbg  Debug
	exec ExecuteOptions
}

func (e *env) restArg(i int) string {
	if i < len(e.rest) {
		return e.rest[i]
	}
	return ""
}

func usage() {
	fmt.Fprint(os.Stderr, `mc - MeshCore companion radio client

Usage:
  mc <command> [flags]

Commands:
  connect [uri]      Discover or connect to a radio and save a profile
  status             Show device status
  stats              Show local radio statistics
  doctor             Run connection diagnostics
  backend start      Start/restart/stop/reset/log/inspect the local backend
  contacts           List contacts
  contact show <n>   Show a contact
  inbox              Drain buffered incoming messages
  send <to> <text>   Send a direct message (--wait for ack)
  watch              Stream incoming messages and events
  shell              Open an interactive persistent session
  trace <target>     Trace the route to a node
  channel list       List channels
  channel send <c> <text>  Send a channel message
  channel add <n> [key]    Add a channel (key, or #name to derive)
  channel remove <c> Remove a channel
  advert             Broadcast this device's advert (--flood for mesh-wide)
  discover           Scan for nearby nodes (repeaters by default)
  repeater list      List saved repeaters
  repeater add <n>   Save/login to a repeater
  repeater del [n]   Remove a saved repeater
  use <profile>      Set the default device profile
  device list        List saved profiles and session state
  device show        Show the connected device
  device show <name> Show a saved profile
  device remove <n>  Remove a profile
  session start <n>  Connect a device's radio session
  session stop <n>   Disconnect a device's radio session
  session restart <n> Reconnect a device's radio session
  state list         List per-device local-state databases
  state show <d>     Show one device's local state
  state purge <d>    Delete one device's local state
  state prune        Delete state older than --older-than
  config path        Print the config file path
  config show        Print the current configuration
  raw <hex>          Send raw bytes and print the decoded response
  version            Show version information

Global flags:
  --json             Machine-readable output
  --uri <uri>        Use an explicit endpoint for one command
  --device <name>    Use a saved profile for one command
  --debug            Verbose logging

Run "mc <command> -h" or "mc help <command>" for command-specific help.
`)
}

func printShellHelpFooter() {
	fmt.Fprint(os.Stderr, `
Shell commands:
  exit, quit    Close the shell
`)
}
