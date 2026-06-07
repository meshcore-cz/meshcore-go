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

// commandAliases maps short command names to their canonical form.
var commandAliases = map[string]string{
	"add":  "connect",
	"b":    "backend",
	"c":    "contacts",
	"h":    "help",
	"log":  "backend",
	"rep":  "repeater",
	"s":    "status",
	"conf": "config",
	"ls":   "device",
	"list": "device",
}

// commandAliasSubcommand supplies a default subcommand for alias-only commands.
var commandAliasSubcommand = map[string]string{
	"log":  "log",
	"ls":   "list",
	"list": "list",
}

// resolveAlias returns the canonical command name for cmd, or cmd unchanged.
func resolveAlias(cmd string) string {
	if canonical, ok := commandAliases[cmd]; ok {
		return canonical
	}
	return cmd
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
	wantHelp := pa.has("help") || pa.has("h")

	// `mc help [command]`
	if cmd == "help" {
		if sub := pa.arg(1); sub != "" {
			return printCommandHelp(sub)
		}
		usage()
		return 0
	}
	if cmd == "" {
		usage()
		if wantHelp {
			return 0
		}
		return 2
	}
	// `mc <command> -h` / `--help`
	if wantHelp {
		return printCommandHelp(cmd)
	}

	ctx := context.Background()
	out := output.New(pa.has("json"))

	rest := pa.positionals[1:]
	if sub, ok := commandAliasSubcommand[originalCmd]; ok {
		rest = append([]string{sub}, rest...)
	}
	env := &env{args: pa, rest: rest, out: out, dbg: newDebug(pa)}

	var runErr error
	switch cmd {
	case "version":
		runErr = cmdVersion(env)
	case "connect":
		runErr = cmdConnect(ctx, env)
	case "status":
		runErr = cmdStatus(ctx, env)
	case "stats":
		runErr = cmdStats(ctx, env)
	case "doctor":
		runErr = cmdDoctor(ctx, env)
	case "backend":
		runErr = cmdBackend(ctx, env)
	case "contacts":
		runErr = cmdContacts(ctx, env)
	case "contact":
		runErr = cmdContact(ctx, env)
	case "inbox":
		runErr = cmdInbox(ctx, env)
	case "send":
		runErr = cmdSend(ctx, env)
	case "watch":
		runErr = cmdWatch(ctx, env)
	case "shell":
		runErr = cmdShell(ctx, env)
	case "trace":
		runErr = cmdTrace(ctx, env)
	case "channel":
		runErr = cmdChannel(ctx, env)
	case "advert":
		runErr = cmdAdvert(ctx, env)
	case "discover":
		runErr = cmdDiscover(ctx, env)
	case "repeater":
		runErr = cmdRepeater(ctx, env)
	case "use":
		runErr = cmdUse(env)
	case "device":
		runErr = cmdDevice(ctx, env)
	case "config":
		runErr = cmdConfig(env)
	case "raw":
		runErr = cmdRaw(ctx, env)
	default:
		fmt.Fprintf(os.Stderr, "mc: unknown command %q\n", cmd)
		usage()
		return 2
	}

	if runErr != nil {
		fmt.Fprintln(os.Stderr, "mc:", runErr)
		if errors.Is(runErr, serial.ErrBusy) {
			fmt.Fprintln(os.Stderr, "hint: another program is using the serial port "+
				"(serial monitor, firmware flasher, or another mc). Close it and retry.")
		}
		return 1
	}
	return 0
}

// env carries parsed arguments and the output printer to each command.
type env struct {
	args parsedArgs
	rest []string // positional args after the subcommand
	out  *output.Printer
	dbg  Debug
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
  advert             Broadcast this device's advert (--flood for mesh-wide)
  discover           Scan for nearby nodes (repeaters by default)
  repeater list      List saved repeaters
  repeater add <n>   Save/login to a repeater
  repeater del [n]   Remove a saved repeater
  use <profile>      Set the default device profile
  device list        List saved profiles
  device show        Show the connected device
  device show <name> Show a saved profile
  device remove <n>  Remove a profile
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
