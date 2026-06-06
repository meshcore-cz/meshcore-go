# mc

**A practical command-line client for MeshCore companion radios.**

Connect a radio once, save it as a profile, and use the mesh directly from your terminal.

```sh
mc connect
mc status
mc contacts
mc send alice "hello"
```

`mc` is powered by [`meshcore-go`](../../README.md), the reusable Go SDK for MeshCore-compatible companion radios.

> [!WARNING]
> `mc` is under active development. Commands, configuration formats, and output may change before the first stable release.

## Features

* USB serial and Bluetooth Low Energy connections
* interactive radio discovery
* saved device profiles
* optional persistent local backend
* direct messages and channel messages
* contact and channel synchronization
* buffered inbox draining
* live event streaming
* delivery acknowledgements
* route tracing
* nearby-node discovery
* self-advertisements
* remote repeater administration
* JSON output for scripts
* raw protocol exploration
* connection diagnostics

## Install

`mc` currently requires Go 1.25 or newer.

```sh
go install github.com/meshcore-cz/meshcore-go/cmd/mc@latest
```

Make sure the Go binary directory is in your `PATH`. It is usually:

```sh
export PATH="$PATH:$(go env GOPATH)/bin"
```

Verify the installation:

```sh
mc version
```

### Build from source

From the repository root:

```sh
make build
./bin/mc version
```

Install the locally built version into `$GOBIN`:

```sh
make install
```

## Quick start

### 1. Connect a radio

Run:

```sh
mc connect
```

`mc` scans USB serial and BLE devices, verifies compatible radios with a protocol handshake, and lets you select one interactively.

To scan only one transport:

```sh
mc connect --usb
mc connect --ble
```

To connect to an explicit endpoint:

```sh
mc connect serial:///dev/ttyACM0
mc connect serial:///dev/ttyUSB0
mc connect ble://C4:20:12:34:56:78
```

The selected radio is saved as a profile and becomes the default for future commands.

### 2. Check the connection

```sh
mc status
```

Typical output has this shape:

```text
Device:        a1b2c3d4
Firmware:      MeshCore <version>
Protocol:      companion
Transport:     serial:///dev/ttyACM0
Public key:    <public-key>

Backend:       ready (pid <pid>)
Replica:       fresh
Contacts:      24, updated moments ago
Channels:      3, updated moments ago
```

### 3. Start using the mesh

```sh
mc contacts
mc inbox
mc send alice "hello"
mc watch
```

## Everyday commands

### Device status

```sh
mc status
```

Shows the active radio, firmware, protocol, transport, public key, backend state, and replica freshness.

For more detailed device information, including capabilities:

```sh
mc device show
```

### Contacts

List contacts stored on the radio:

```sh
mc contacts
```

Show one contact by name or public-key prefix:

```sh
mc contact show alice
mc contact show 0a53ef
```

When the local backend is running, contacts are replicated locally. Use the cached copy without contacting the radio:

```sh
mc contacts --cached
```

Force a fresh radio sync:

```sh
mc contacts --refresh
```

### Direct messages

Send a direct message:

```sh
mc send alice "hello"
```

Recipients can be resolved by contact name or public-key prefix.

Wait for a delivery acknowledgement:

```sh
mc send alice "hello" --wait
```

### Inbox

Drain messages buffered on the radio:

```sh
mc inbox
```

> [!NOTE]
> Synced messages are removed from the device buffer after they are printed.

### Live events

Stream incoming messages and events until interrupted:

```sh
mc watch
```

For newline-delimited JSON:

```sh
mc watch --json
```

For low-level protocol inspection, stream every inbound packet as JSON lines:

```sh
mc watch --raw
```

### Channels

List configured channel slots:

```sh
mc channel list
```

Show a channel by name or index:

```sh
mc channel show Public
mc channel show 0
```

Send a channel message:

```sh
mc channel send Public "hello everyone"
```

Force a channel refresh from the radio:

```sh
mc channel list --refresh
```

## Local backend

By default, `mc` remains useful as a normal one-shot command-line tool. Commands connect directly to the selected radio, perform an operation, and exit.

For frequent use, `mc` can also run a local background backend that keeps the radio connection open and maintains local replicas of contacts and channels.

```text
mc command
    │
    ├── backend available ──▶ local socket ──▶ persistent radio connection
    │
    └── backend absent ─────▶ direct radio connection
```

The backend is optional. Ordinary commands automatically use it when it is running and fall back to a direct connection when it is not.

### Start the backend

```sh
mc backend start
```

### Inspect its status

```sh
mc backend status
```

### Restart or stop it

```sh
mc backend restart
mc backend stop
```

### Read backend logs

```sh
mc backend log
mc backend log --lines 50
mc backend log --follow
```

### Force a direct connection

Bypass the backend for one command:

```sh
mc status --direct
mc contacts --direct
```

### Backend bridges

The backend can optionally expose local bridge listeners for other applications.

Configure bridges in `config.yaml`:

```yaml
backend:
  bridges:
    - enabled: true
      type: tcp
      listen: 127.0.0.1:4403

    - enabled: true
      type: pty
```

Supported bridge types:

| Type  | Purpose                                            |
| ----- | -------------------------------------------------- |
| `tcp` | Expose the backend through a local TCP listener    |
| `pty` | Expose a pseudo-terminal for native serial clients |

> [!NOTE]
> On macOS, Chrome Web Serial does not list PTY bridges. The PTY bridge is intended for native serial clients. Use TCP only with clients that explicitly support a TCP MeshCore bridge.

## Interactive shell

For a foreground session that keeps a single radio connection alive:

```sh
mc shell
```

The shell is useful for experimentation without repeatedly reopening the connection.

```text
mc> status
mc> contacts
mc> send alice hello
mc> trace alice
mc> channel list
mc> inbox
mc> watch
mc> exit
```

Available shell commands:

| Command                            | Purpose                 |
| ---------------------------------- | ----------------------- |
| `status`                           | Show radio status       |
| `contacts`                         | List contacts           |
| `contact show <name>`              | Show one contact        |
| `inbox`                            | Drain buffered messages |
| `send <recipient> <text> [--wait]` | Send a direct message   |
| `trace <target>`                   | Trace a route           |
| `channel list\|show\|send`         | Work with channels      |
| `watch`                            | Stream events           |
| `help`                             | Show shell help         |
| `exit`                             | Close the session       |

## Mesh tools

### Advertise this node

Send a zero-hop advertisement to direct neighbours:

```sh
mc advert
```

Flood the advertisement across the mesh:

```sh
mc advert --flood
```

### Discover nearby nodes

Scan for nearby repeaters:

```sh
mc discover
```

Without type flags, discovery scans repeaters only.

Scan every node type:

```sh
mc discover --all
```

Select specific types:

```sh
mc discover --repeater
mc discover --companion
mc discover --room
mc discover --sensor
mc discover --room --sensor
```

Request full public keys and wait longer for replies:

```sh
mc discover --all --full --timeout 10
```

Each reply reports round-trip signal information:

```text
↑ how the remote node heard the request
↓ how the local radio heard the reply
```

> [!TIP]
> `mc connect` discovers radios attached to your computer.
> `mc discover` sends a request over the mesh to discover remote nodes.

### Trace a route

Trace a direct neighbour by node hash:

```sh
mc trace 25
```

Trace a multi-hop path:

```sh
mc trace 25,a1
```

Trace a saved contact:

```sh
mc trace alice
```

A single-byte target only reaches a direct neighbour. Distant nodes require the full comma-separated path.

## Remote repeaters

`mc` can save and administer remote repeaters through the active companion radio.

The shorter `rep` alias is also available:

```sh
mc rep list
```

### Add a repeater

```sh
mc repeater add mc.kololec.cz
```

When running interactively, `mc` prompts for a password when none is passed explicitly.

You can also pass it directly:

```sh
mc repeater add mc.kololec.cz "<password>"
```

### List saved repeaters

```sh
mc repeater list
```

### Query repeater status

```sh
mc repeater status mc.kololec.cz
```

When a current repeater is already selected, the argument can be omitted:

```sh
mc repeater status
```

### Inspect repeater neighbours

```sh
mc repeater neighbours mc.kololec.cz
```

### Execute a repeater command

```sh
mc repeater exec mc.kololec.cz "clock"
```

### Remove a repeater

```sh
mc repeater del mc.kololec.cz
```

> [!CAUTION]
> Saved repeater profiles can contain passwords. Protect your local `mc` configuration file accordingly.

## Device profiles

`mc connect` saves radios as named profiles, so daily commands do not need repeated device addresses or serial paths.

### Name a profile during setup

```sh
mc connect --as handheld
mc connect serial:///dev/ttyACM0 --as desk-radio
```

### List profiles

```sh
mc device list
```

The aliases `mc ls` and `mc list` are also available:

```sh
mc ls
```

### Select the default profile

```sh
mc use handheld
```

### Override the profile for one command

```sh
mc status --device handheld
mc contacts --device desk-radio
```

### Show or remove a profile

```sh
mc device show handheld
mc device remove handheld
```

### Use an endpoint without saving it

```sh
mc connect serial:///dev/ttyACM0 --no-save
```

## Configuration

Print the active configuration path:

```sh
mc config path
```

Print the current configuration:

```sh
mc config show
```

The default path is:

```text
~/.config/mc/config.yaml
```

When `$XDG_CONFIG_HOME` is set, `mc` uses:

```text
$XDG_CONFIG_HOME/mc/config.yaml
```

A configuration file may contain:

* the selected device profile
* saved device endpoints
* preferred transports
* saved repeater profiles
* backend bridge listeners

## Global flags

Connection-oriented commands support these flags:

| Flag              | Purpose                                  |
| ----------------- | ---------------------------------------- |
| `--json`          | Print machine-readable JSON              |
| `--uri <uri>`     | Use an explicit endpoint for one command |
| `--device <name>` | Use a saved profile for one command      |
| `--direct`        | Bypass the local backend                 |
| `--debug`         | Print verbose logs to stderr             |

Flags can appear before or after a command:

```sh
mc --device handheld status
mc status --device handheld
```

## JSON output

Use `--json` for scripts and automations:

```sh
mc status --json
mc contacts --json
mc channel list --json
mc repeater status mc.kololec.cz --json
mc config show --json
```

Streaming commands emit newline-delimited JSON objects:

```sh
mc watch --json
mc watch --raw
```

Example with `jq`:

```sh
mc contacts --json | jq
```

## Diagnostics

Run connection diagnostics:

```sh
mc doctor
```

This checks configuration, endpoint reachability, the protocol handshake, firmware information, and the device-clock difference.

Enable verbose stderr logs for any connection command:

```sh
mc status --debug
mc doctor --debug
```

When a serial port is busy, close other applications that may be using it, such as a serial monitor, firmware flasher, or another `mc` process.

## Raw protocol access

For protocol development and undocumented-command exploration, send bare companion-protocol bytes directly to the radio:

```sh
mc raw 16 03
mc raw 14
mc raw 05
mc raw 1f 00
```

Bytes can be written separately, concatenated, or with a `0x` prefix:

```sh
mc raw ab cd ef
mc raw abcdef
mc raw 0xab 0xcd 0xef
```

Transport framing is added automatically. Do not include serial or BLE framing bytes.

Known responses are decoded into structured fields. Unknown responses are shown as a hex dump.

Use debug output to inspect the resolved endpoint, outbound frame, and decoded response:

```sh
mc raw 14 --debug
```

## Command reference

| Command                                                  | Purpose                                           |
| -------------------------------------------------------- | ------------------------------------------------- |
| `mc connect [uri]`                                       | Discover or connect to a radio and save a profile |
| `mc status`                                              | Show device and backend status                    |
| `mc doctor`                                              | Run connection diagnostics                        |
| `mc backend <start\|restart\|stop\|status\|log>`         | Manage the local backend                          |
| `mc contacts`                                            | List contacts                                     |
| `mc contact show <name>`                                 | Show one contact                                  |
| `mc inbox`                                               | Drain buffered incoming messages                  |
| `mc send <recipient> <text>`                             | Send a direct message                             |
| `mc watch`                                               | Stream incoming events                            |
| `mc shell`                                               | Open an interactive persistent session            |
| `mc trace <target>`                                      | Trace the route to a node                         |
| `mc channel <list\|show\|send>`                          | Work with channel slots                           |
| `mc advert`                                              | Broadcast this device's advertisement             |
| `mc discover`                                            | Discover remote mesh nodes                        |
| `mc repeater <list\|add\|del\|status\|neighbours\|exec>` | Manage remote repeaters                           |
| `mc use <profile>`                                       | Select the default device profile                 |
| `mc device <list\|show\|remove>`                         | Manage saved profiles                             |
| `mc config <path\|show>`                                 | Inspect CLI configuration                         |
| `mc raw <hex bytes...>`                                  | Send raw companion-protocol bytes                 |
| `mc version`                                             | Print version information                         |

Detailed help is available directly in the terminal:

```sh
mc help
mc help connect
mc help backend
mc help discover
mc help repeater
mc help raw
```

## Aliases

A few short aliases are available for frequently used commands:

| Alias     | Equivalent command |
| --------- | ------------------ |
| `mc add`  | `mc connect`       |
| `mc rep`  | `mc repeater`      |
| `mc conf` | `mc config`        |
| `mc ls`   | `mc device list`   |
| `mc list` | `mc device list`   |
| `mc h`    | `mc help`          |

## Troubleshooting

### No radio is found

Limit the scan to one transport:

```sh
mc connect --usb
mc connect --ble
```

Or specify the endpoint explicitly:

```sh
mc connect serial:///dev/ttyACM0
```

### Serial port is busy

Another program may already hold the device open. Close serial monitors, firmware flashers, other MeshCore clients, and other `mc` processes.

### Backend state looks stale

Inspect the backend and its logs:

```sh
mc backend status
mc backend log
```

Force a direct connection to compare behavior:

```sh
mc status --direct
```

Refresh local replicas:

```sh
mc contacts --refresh
mc channel list --refresh
```

### Check the resolved configuration

```sh
mc config path
mc config show
```

### Enable verbose logs

```sh
mc doctor --debug
```

## Project relationship

`mc` is the reference terminal application for [`meshcore-go`](../../README.md).

The reusable SDK owns:

* companion-protocol behavior
* transport abstractions
* packet framing
* contact and message handling
* repeater operations
* asynchronous events

The CLI owns:

* terminal UX
* saved profiles
* local configuration
* human-readable output
* JSON output
* backend lifecycle management
* interactive shell behavior

This separation makes it possible to reuse the same MeshCore implementation in automations, gateways, monitoring services, Home Assistant bridges, and other applications.
