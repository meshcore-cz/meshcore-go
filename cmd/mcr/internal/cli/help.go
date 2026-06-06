package cli

import (
	"fmt"
	"os"
)

// globalFlags is appended to the help of commands that open a connection.
const globalFlags = `
Global flags:
  --json           Machine-readable JSON output
  --uri <uri>      Use an explicit endpoint for this command only
  --device <name>  Use a saved profile for this command only
  --debug          Verbose logging to stderr
`

// commandHelp maps each command to its detailed help text. Connection commands
// end with the shared global-flags section.
var commandHelp = map[string]string{
	"connect": `Usage: mcr connect [uri] [flags]

Discover or connect to a companion radio, verify it with a handshake, and
(unless --no-save) save a profile and make it the default.

  mcr connect                       scan USB + BLE and choose interactively
  mcr connect --usb                 scan USB serial only
  mcr connect serial:///dev/ttyACM0 connect to an explicit endpoint
  mcr connect ble://C4:20:...       connect over BLE

Flags:
  --usb            Scan USB serial devices only
  --ble            Scan BLE devices only
  --as <name>      Save under this profile name
  --no-save        Connect without saving a profile
  --json           Machine-readable JSON output
`,

	"status": `Usage: mcr status [flags]

Show the connected device's identity, firmware, transport and capabilities.
` + globalFlags,

	"doctor": `Usage: mcr doctor [flags]

Run connection diagnostics: configuration, endpoint reachability, handshake,
firmware and clock difference.
` + globalFlags,

	"contacts": `Usage: mcr contacts [flags]

List the contacts stored on the device.
` + globalFlags,

	"contact": `Usage: mcr contact show <name> [flags]

Show details for a single contact, matched by name (case-insensitive) or by a
public-key hex prefix.
` + globalFlags,

	"inbox": `Usage: mcr inbox [flags]

Drain and print messages buffered on the device. Synced messages are removed
from the device buffer.
` + globalFlags,

	"send": `Usage: mcr send <recipient> <text> [flags]

Send a direct text message to a contact (by name or key prefix).

  mcr send alice "hello"
  mcr send alice "hello" --wait     wait for delivery acknowledgement

Flags:
  --wait           Block until the message is acknowledged
` + globalFlags,

	"watch": `Usage: mcr watch [flags]

Stream incoming messages and events until interrupted (Ctrl-C). With --json,
each event is emitted as a newline-delimited JSON object.
` + globalFlags,

	"trace": `Usage: mcr trace <target> [flags]

Trace the network route to a node and report per-link signal quality.

The target is either a hash path (one hex byte per hop) or a contact name /
key prefix whose node hash is used:

  mcr trace 25        trace the direct neighbour with hash 0x25
  mcr trace 25,a1     trace through 0x25 then 0xa1
  mcr trace alice     trace the contact "alice"

A single-hash target only reaches a direct neighbour; distant nodes need the
full multi-hop path.
` + globalFlags,

	"channel": `Usage: mcr channel <list|show|send> [args] [flags]

Work with channel slots.

  mcr channel list                  list configured channels
  mcr channel show Public           show a channel by name or index
  mcr channel send Public "hello"   send a message to a channel
` + globalFlags,

	"use": `Usage: mcr use <profile>

Set the default device profile used by subsequent commands.
`,

	"device": `Usage: mcr device <list|show|remove> [name]

Manage saved device profiles.

  mcr device list             list saved profiles
  mcr device show handheld    show a profile
  mcr device remove handheld  delete a profile

Flags:
  --json           Machine-readable JSON output
`,

	"config": `Usage: mcr config <path|show>

Inspect the CLI configuration.

  mcr config path    print the configuration file path
  mcr config show    print the current configuration

Flags:
  --json           Machine-readable JSON output
`,

	"version": `Usage: mcr version [--json]

Print version information.
`,
}

// printCommandHelp writes a command's help to stdout and returns an exit code.
func printCommandHelp(cmd string) int {
	h, ok := commandHelp[cmd]
	if !ok {
		fmt.Fprintf(os.Stderr, "mcr: no help available for %q\n", cmd)
		usage()
		return 2
	}
	fmt.Fprint(os.Stdout, h)
	return 0
}
