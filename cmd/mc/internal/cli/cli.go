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

// Execute parses arguments and runs a single command.
func Execute(ctx context.Context, args []string, opts ExecuteOptions) error {
	root := NewRoot(&App{Options: opts})
	root.SetArgs(normalizeArgs(args))
	return root.ExecuteContext(ctx)
}

// Run parses arguments and dispatches to a subcommand. It returns a process
// exit code.
func Run(args []string) int {
	err := Execute(context.Background(), args, ExecuteOptions{})
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

// parsedArgs is the small flag/argument view consumed by existing command
// handlers while Cobra owns parsing and dispatch.
type parsedArgs struct {
	flags       map[string]string
	bools       map[string]bool
	positionals []string
}

func (p parsedArgs) flag(name string) string { return p.flags[name] }
func (p parsedArgs) has(name string) bool    { return p.bools[name] }
func (p parsedArgs) arg(i int) string {
	if i < len(p.positionals) {
		return p.positionals[i]
	}
	return ""
}

func (e *env) restArg(i int) string {
	if i < len(e.rest) {
		return e.rest[i]
	}
	return ""
}
