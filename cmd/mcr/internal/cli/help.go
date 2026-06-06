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
  --direct         Do not use the local backend for this command
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

	"backend": `Usage: mcr backend <start|restart|stop|status> [flags]

Manage the local backend process. When it is running, ordinary commands use it
automatically; when it is not running, commands dial the radio directly.

  mcr backend start             start the backend for the selected profile
  mcr backend restart           restart using the current backend endpoint
  mcr backend restart --uri ... restart onto an explicit endpoint
  mcr backend start --uri ...   start the backend for an explicit endpoint
  mcr backend status            show backend socket, pid and endpoint
  mcr backend stop              stop the running backend
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

	"shell": `Usage: mcr shell [flags]

Open an interactive foreground session that keeps one radio connection alive.

Available shell commands:
  status
  contacts
  contact show <name>
  inbox
  send <recipient> <text> [--wait]
  trace <target>
  channel list|show|send
  watch
  help
  exit
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

	"raw": `Usage: mcr raw <hex bytes...> [flags]

Send raw bytes directly to the device and print the decoded response.
Useful for protocol exploration and verifying undocumented commands.

The payload is the bare companion-protocol bytes (command byte + body).
Transport framing is added automatically; do not include it.
Bytes can be given as separate tokens or concatenated:

  mcr raw 16 03              DeviceQuery  (cmd=0x16, app_ver=0x03)
  mcr raw 14                 GetBatteryVoltage
  mcr raw 05                 GetDeviceTime
  mcr raw 04                 GetContacts (streaming — only first frame shown)
  mcr raw 1f 00              GetChannel index 0
  mcr raw ab cd ef           unknown three-byte command
  mcr raw abcdef             same (concatenated)
  mcr raw 0xab 0xcd          same with 0x prefix

Common command bytes (host → device):
  0x01  AppStart          0x0a  SyncNextMessage
  0x02  SendTxtMsg        0x0b  SetRadioParams
  0x03  SendChannelTxt    0x0c  SetTxPower
  0x04  GetContacts       0x0d  ResetPath
  0x05  GetDeviceTime     0x13  Reboot
  0x06  SetDeviceTime     0x14  GetBatteryVoltage
  0x07  SendSelfAdvert    0x16  DeviceQuery
  0x09  AddUpdateContact  0x1f  GetChannel
  0x0f  RemoveContact     0x24  SendTracePath

Output shows the decoded response type and fields for known opcodes,
or a hex dump for unrecognised ones.
` + globalFlags,

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
