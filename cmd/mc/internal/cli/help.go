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

Show radio status, persistent local state, and backend session state.

  --all            Show compact status for all saved devices
  --live           Refresh radio stats before showing status (may block on radio I/O)
` + globalFlags,

	"stats": `Usage: mc stats [flags]

Show local core, radio and packet statistics from the active companion radio.
When the backend is running, mc asks the backend first; otherwise it dials the
selected device directly.
` + globalFlags,

	"doctor": `Usage: mc doctor [flags]

Run connection diagnostics: configuration, endpoint reachability, handshake,
firmware and clock difference.
` + globalFlags,

	"backend": `Usage: mc backend <start|restart|stop|status|log|reset> [flags]

Manage the local backend process. When it is running, ordinary commands use it
automatically; when it is not running, commands dial the radio directly.

  mc backend start             start the backend for the selected profile
  mc backend restart           restart using the current backend endpoint
  mc backend restart --reset   restart and delete local replica state first
  mc backend restart --uri ... restart onto an explicit endpoint
  mc backend start --uri ...   start the backend for an explicit endpoint
  mc backend status            show backend daemon, radio and diagnostics
  mc backend status --verbose  include poll config, requests and log path
  mc backend log               show recent backend log output
  mc backend log --follow      stream new log lines
  mc backend stop              stop the running backend
  mc backend reset             stop the backend and delete local replica state

Log flags:
  -n, --lines <count>          number of lines to show (default: 100)
  -f, --follow                 follow log output as it is written

Bridge listeners are configured in config.yaml:

backend:
  log_requests: true
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

List contacts from the backend's local replica.

Flags:
  --cached         Same as the default: read the local replica
  --refresh        Start a background radio sync (returns immediately)
  --wait           With --refresh, block until synchronization finishes
  --full           With --refresh, rebuild the full contact list (ignore cursor)
  --wide           Show full public keys, stored advert paths and coordinates
  --type <kind>    Filter by contact type (companion, repeater, room, sensor)
  --route <kind>   Filter by route (direct, flood, static)
  --within <dist>  Filter by distance from local companion (e.g. 10km, 500m)
  --sort <field>   Sort contacts (default: name)
                   Fields: name, type, age, adv, route, key, distance
` + globalFlags,

	"contact": `Usage: mc contact show <name> [flags]

Show details for a single contact, matched by name (case-insensitive) or by a
public-key hex prefix.
` + globalFlags,

	"inbox": `Usage: mc inbox [flags]

Print unread incoming messages and mark them read. When the backend is running
it drains the radio inbox itself, persists every message to device-local state,
and broadcasts it to ` + "`mc watch`" + `; mc inbox then reads the stored unread
messages. Without a backend, mc inbox drains the radio directly.
` + globalFlags,

	"send": `Usage: mc send <recipient> <text> [flags]
       mc send --channel <name|index> <text>

Send a direct text message to a contact (by name or key prefix), or a message to
a channel with --channel.

  mc send alice "hello"
  mc send alice "hello" --wait     wait for delivery acknowledgement
  mc send --channel rem-ha "ahoj!" send to a channel (same as mc channel send)

Flags:
  --wait             Block until the message is acknowledged (direct only)
  --channel <name>   Send to a channel by name or slot index
` + globalFlags,

	"watch": `Usage: mc watch [flags]

Stream incoming messages and events until interrupted (Ctrl-C). With --json,
each event is emitted as a newline-delimited JSON object.

  mc watch --raw                 stream every inbound packet as JSON lines
  mc watch --rf                  stream RF packet log frames as JSON lines

Flags:
  --rf            Stream typed RF packet log data (0x88) as JSON lines
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

Trace the network route to a node and report per-leg signal quality.

The target is either a hash path or a contact name:

  mc trace 25               direct neighbour, 1-byte trace hash
  mc trace 25,a1            multi-hop path, 1 byte per leg
  mc trace a1b2,c3d4        multi-hop path, 2 bytes per leg
  mc trace 252525ce         explicit 4-byte trace hash
  mc trace 2525,5153,0455 --return
                            trace out and back: 2525,5153,0455,5153,2525
  mc trace alice            use the stored contact route when available

Hex targets are always interpreted as explicit paths.
Use a contact name to trace using a stored contact route.

  --return        Append the reverse path (explicit hex paths only)
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

	"discover": `Usage: mc discover [flags]

Broadcast a node-discovery request and print nodes as they reply. Each reply
reports the round-trip signal: ↑ is how the remote node heard the request, ↓ is
how this device heard the reply. With no type flag, only repeaters are scanned.

  mc discover                  scan for nearby repeaters
  mc discover --all            scan for every node type
  mc discover --room --sensor  scan for rooms and sensors
  mc discover --timeout 10     listen for 10 seconds

Flags:
  --all            Discover every node type
  --repeater       Discover repeaters
  --companion      Discover companion (client) nodes
  --room           Discover room servers
  --sensor         Discover sensors
  --full           Request full public keys instead of 8-byte prefixes
  --timeout <sec>  How long to listen for replies (default ~6s)
` + globalFlags,

	"repeater": `Usage: mc repeater <list|add|del|status|neighbours|exec> [args] [flags]

Manage remote repeaters through the active companion radio. Aliased as "mc rep".

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

Manage saved device profiles. To connect or disconnect a device's live radio
session, use ` + "`mc session`" + `.

  mc device list             list saved profiles and their session state
  mc device show             show the connected device and capabilities
  mc device show handheld    show a saved profile
  mc device remove handheld  stop the session and delete the profile

Flags:
  --wide           Show transport endpoints
  --json           Machine-readable JSON output
`,

	"session": `Usage: mc session <list|start|stop|restart> [name]

Manage device sessions: the live radio connections held by the running backend.
A session connects one saved profile's radio; profiles are managed with
` + "`mc device`" + `. These require a running backend (` + "`mc backend start`" + `).

  mc session list               show each device's session state
  mc session start handheld     connect this device's radio in the backend
  mc session stop handheld      disconnect the radio (keeps the saved profile)
  mc session restart handheld   reconnect the radio

With no name, the current default device is used. Each session is isolated: the
radio connection, queue, and replica for other devices are unaffected. Set
` + "`backend.autostart: true`" + ` on a profile to connect it automatically when the
backend starts.

Flags:
  --json           Machine-readable JSON output
`,

	"state": `Usage: mc state <list|show|purge|prune> [device]

Inspect and manage per-device local state. Each connected device keeps its own
SQLite database of contacts, channels, and repeater sessions at
~/.local/state/mc/devices/<public-key-prefix>.db. This is device-local state,
not a cache: it may be stale, incomplete, or locally enriched.

  mc state list                       list every device's local-state database
  mc state show handheld              show one device's state (name, key, or prefix)
  mc state purge handheld             delete one device's local-state database
  mc state prune --older-than 30d     delete state not updated within a duration

A device may be named by saved profile, full public key, or key prefix.
Durations accept d (days), w (weeks), and Go units (h, m, s).

Flags:
  --older-than <dur>   Age threshold for prune (e.g. 30d, 2w, 48h)
  --json               Machine-readable JSON output
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
  0x38  GetStats

Output shows the decoded response type and fields for known opcodes,
or a hex dump for unrecognised ones.

With --debug, logs the resolved endpoint, outbound frame, and decoded response.
` + globalFlags,

	"version": `Usage: mc version [--json]

Print version information.
`,
}

// writeCommandHelp writes a command's help to stdout.
func writeCommandHelp(cmd string) error {
	h, ok := commandHelp[resolveAlias(cmd)]
	if !ok {
		fmt.Fprintf(os.Stderr, "mc: no help available for %q\n", cmd)
		usage()
		return fmt.Errorf("no help available for %q", cmd)
	}
	fmt.Fprint(os.Stdout, h)
	return nil
}

// printCommandHelp writes a command's help to stdout and returns an exit code.
func printCommandHelp(cmd string) int {
	if err := writeCommandHelp(cmd); err != nil {
		return 2
	}
	return 0
}
