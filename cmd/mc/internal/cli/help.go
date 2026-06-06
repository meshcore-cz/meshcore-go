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
	"connect": `Usage: mc connect [uri] [flags]

Discover or connect to a companion radio, verify it with a handshake, and
(unless --no-save) save a profile and make it the default. In interactive mode,
mc then offers to start the local backend for that endpoint.

  mc connect                       scan USB + BLE and choose interactively
  mc connect --usb                 scan USB serial only
  mc connect serial:///dev/ttyACM0 connect to an explicit endpoint
  mc connect ble://C4:20:...       connect over BLE

Flags:
  --usb            Scan USB serial devices only
  --ble            Scan BLE devices only
  --as <name>      Save under this profile name
  --no-save        Connect without saving a profile
  --json           Machine-readable JSON output
`,

	"status": `Usage: mc status [flags]

Show the connected device's identity, firmware, transport and capabilities.
` + globalFlags,

	"doctor": `Usage: mc doctor [flags]

Run connection diagnostics: configuration, endpoint reachability, handshake,
firmware and clock difference.
` + globalFlags,

	"backend": `Usage: mc backend <start|restart|stop|status|log> [flags]

Manage the local backend process. When it is running, ordinary commands use it
automatically; when it is not running, commands dial the radio directly.

  mc backend start             start the backend for the selected profile
  mc backend restart           restart using the current backend endpoint
  mc backend restart --uri ... restart onto an explicit endpoint
  mc backend start --uri ...   start the backend for an explicit endpoint
  mc backend status            show backend socket, pid and endpoint
  mc backend log               show recent backend log output
  mc backend log --follow      stream new log lines
  mc backend stop              stop the running backend

Log flags:
  -n, --lines <count>          number of lines to show (default: 100)
  -f, --follow                 follow log output as it is written

Bridge listeners are configured in config.yaml:

backend:
  bridges:
    - enabled: true
      type: tcp
      listen: 127.0.0.1:4403
    - enabled: true
      type: pty

Note: on macOS, Chrome/Web Serial does not list PTY bridges. The pty bridge is
for native serial clients; use tcp only with clients that explicitly support a
TCP MeshCore bridge.
` + globalFlags,

	"contacts": `Usage: mc contacts [flags]

List the contacts stored on the device.

Flags:
  --cached         Read contacts from the backend's local replica only
  --refresh        Force a radio sync and update the local replica
` + globalFlags,

	"contact": `Usage: mc contact show <name> [flags]

Show details for a single contact, matched by name (case-insensitive) or by a
public-key hex prefix.
` + globalFlags,

	"inbox": `Usage: mc inbox [flags]

Drain and print messages buffered on the device. Synced messages are removed
from the device buffer.
` + globalFlags,

	"send": `Usage: mc send <recipient> <text> [flags]

Send a direct text message to a contact (by name or key prefix).

  mc send alice "hello"
  mc send alice "hello" --wait     wait for delivery acknowledgement

Flags:
  --wait           Block until the message is acknowledged
` + globalFlags,

	"watch": `Usage: mc watch [flags]

Stream incoming messages and events until interrupted (Ctrl-C). With --json,
each event is emitted as a newline-delimited JSON object.

  mc watch --raw                 stream every inbound packet as JSON lines

Flags:
  --raw           Stream all inbound companion packets as JSON lines
` + globalFlags,

	"shell": `Usage: mc shell [flags]

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

	"trace": `Usage: mc trace <target> [flags]

Trace the network route to a node and report per-link signal quality.

The target is either a hash path (one hex byte per hop) or a contact name /
key prefix whose node hash is used:

  mc trace 25        trace the direct neighbour with hash 0x25
  mc trace 25,a1     trace through 0x25 then 0xa1
  mc trace alice     trace the contact "alice"

A single-hash target only reaches a direct neighbour; distant nodes need the
full multi-hop path.
` + globalFlags,

	"channel": `Usage: mc channel <list|show|send> [args] [flags]

Work with channel slots.

  mc channel list                  list configured channels
  mc channel show Public           show a channel by name or index
  mc channel send Public "hello"   send a message to a channel

Channels are served from the backend's local replica. Use --refresh to force a
radio sync and update the replica.

Flags:
  --refresh        Force a radio sync and update the local replica
` + globalFlags,

	"advert": `Usage: mc advert [flags]

Broadcast the device's own advertisement so other nodes learn about it.

  mc advert            send a zero-hop advert (direct neighbours only)
  mc advert --flood    send a flood advert that propagates across the mesh

Flags:
  --flood          Send a flood advert instead of the default zero-hop one
` + globalFlags,

	"repeater": `Usage: mc repeater <list|add|del|status|neighbours|exec> [args] [flags]

Manage remote repeaters through the active companion radio.

  mc repeater list                 list saved repeaters
  mc repeater add mc.kololec.cz [password]
  mc repeater del [mc.kololec.cz]  remove saved repeater and session
  mc repeater status [mc.kololec.cz]
  mc repeater neighbours [mc.kololec.cz]
  mc repeater exec [mc.kololec.cz] "clock"

Saved repeaters are stored in the mc config. If no password is supplied to
add, mc prompts for one in interactive human mode.

Flags:
  --json           Machine-readable JSON output
` + globalFlags,

	"use": `Usage: mc use <profile>

Set the default device profile used by subsequent commands.
`,

	"device": `Usage: mc device <list|show|remove> [name]

Manage saved device profiles.

  mc device list             list saved profiles
  mc device show handheld    show a profile
  mc device remove handheld  delete a profile

Flags:
  --json           Machine-readable JSON output
`,

	"config": `Usage: mc config <path|show>

Inspect the CLI configuration.

  mc config path    print the configuration file path
  mc config show    print the current configuration

Flags:
  --json           Machine-readable JSON output
`,

	"raw": `Usage: mc raw <hex bytes...> [flags]

Send raw bytes directly to the device and print the decoded response.
Useful for protocol exploration and verifying undocumented commands.

The payload is the bare companion-protocol bytes (command byte + body).
Transport framing is added automatically; do not include it.
Bytes can be given as separate tokens or concatenated:

  mc raw 16 03              DeviceQuery  (cmd=0x16, app_ver=0x03)
  mc raw 14                 GetBatteryVoltage
  mc raw 05                 GetDeviceTime
  mc raw 04                 GetContacts (streaming — only first frame shown)
  mc raw 1f 00              GetChannel index 0
  mc raw ab cd ef           unknown three-byte command
  mc raw abcdef             same (concatenated)
  mc raw 0xab 0xcd          same with 0x prefix

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

With --debug, logs the resolved endpoint, outbound frame, and decoded response.
` + globalFlags,

	"version": `Usage: mc version [--json]

Print version information.
`,
}

// printCommandHelp writes a command's help to stdout and returns an exit code.
func printCommandHelp(cmd string) int {
	h, ok := commandHelp[cmd]
	if !ok {
		fmt.Fprintf(os.Stderr, "mc: no help available for %q\n", cmd)
		usage()
		return 2
	}
	fmt.Fprint(os.Stdout, h)
	return 0
}
