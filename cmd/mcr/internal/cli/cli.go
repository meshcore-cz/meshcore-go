// Package cli implements the mcr command-line client. It is intentionally thin:
// anything related to the companion protocol lives in the reusable SDK.
package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/meshcore-dev/meshcore-go/cmd/mcr/internal/output"
)

// version metadata, overridable at build time with -ldflags.
var (
	Version = "v0.1.0"
	Commit  = "unknown"
)

// Run parses arguments and dispatches to a subcommand. It returns a process
// exit code.
func Run(args []string) int {
	pa, err := parseArgs(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, "mcr:", err)
		return 2
	}

	cmd := pa.arg(0)
	if cmd == "" || pa.has("help") || pa.has("h") {
		usage()
		if cmd == "" {
			return 2
		}
		return 0
	}

	ctx := context.Background()
	out := output.New(pa.has("json"))

	rest := pa.positionals[1:]
	env := &env{args: pa, rest: rest, out: out}

	var runErr error
	switch cmd {
	case "version":
		runErr = cmdVersion(env)
	case "connect":
		runErr = cmdConnect(ctx, env)
	case "status":
		runErr = cmdStatus(ctx, env)
	case "doctor":
		runErr = cmdDoctor(ctx, env)
	case "use":
		runErr = cmdUse(env)
	case "device":
		runErr = cmdDevice(ctx, env)
	case "config":
		runErr = cmdConfig(env)
	default:
		fmt.Fprintf(os.Stderr, "mcr: unknown command %q\n", cmd)
		usage()
		return 2
	}

	if runErr != nil {
		fmt.Fprintln(os.Stderr, "mcr:", runErr)
		return 1
	}
	return 0
}

// env carries parsed arguments and the output printer to each command.
type env struct {
	args parsedArgs
	rest []string // positional args after the subcommand
	out  *output.Printer
}

func (e *env) restArg(i int) string {
	if i < len(e.rest) {
		return e.rest[i]
	}
	return ""
}

func usage() {
	fmt.Fprint(os.Stderr, `mcr - MeshCore companion radio client

Usage:
  mcr <command> [flags]

Commands:
  connect [uri]      Discover or connect to a radio and save a profile
  status             Show device status
  doctor             Run connection diagnostics
  use <profile>      Set the default device profile
  device list        List saved profiles
  device show <name> Show a profile
  device remove <n>  Remove a profile
  config path        Print the config file path
  config show        Print the current configuration
  version            Show version information

Global flags:
  --json             Machine-readable output
  --uri <uri>        Use an explicit endpoint for one command
  --device <name>    Use a saved profile for one command
  --debug            Verbose logging
`)
}
