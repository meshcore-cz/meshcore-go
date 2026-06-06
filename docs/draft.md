# meshcore-go

A reusable Go library for interacting with MeshCore-compatible companion radios, with the `mc` command-line client included.

> **Status:** early concept and work in progress.
> Public APIs, package names, CLI commands, and configuration formats may change before the first stable release.

`meshcore-go` provides a transport-independent Go SDK for communicating with MeshCore-compatible companion radios.

The repository also includes `mc`, a modern terminal client built on top of the library:

```text
meshcore-go
├── reusable Go SDK
└── cmd/mc
    └── command-line client
```

The goal is to maintain one protocol implementation that can be reused by:

* command-line tools
* automations
* Home Assistant bridges
* MQTT gateways
* monitoring services
* web backends
* desktop applications
* experimental transports
* firmware development tools
* custom MeshCore-compatible integrations

The bundled `mc` command is the first reference application.

---

## Design principles

### Library first

The CLI is intentionally thin.

Anything related to the MeshCore companion protocol belongs in the reusable SDK:

```text
protocol framing
request sequencing
response parsing
event dispatch
contact synchronization
message handling
capability detection
transport abstractions
```

Anything related only to terminal usage belongs in `mc`:

```text
saved profiles
interactive discovery
configuration files
tables
JSON output
shell completion
diagnostics
```

External applications can import the SDK without depending on the CLI.

### Transport independent

Applications should use the same high-level API regardless of how a companion radio is reached.

```go
client, err := meshcore.Dial(ctx, "serial:///dev/ttyACM0")
```

```go
client, err := meshcore.Dial(ctx, "ble://C4:20:12:34:56:78")
```

Future transports should fit the same model:

```go
client, err := meshcore.Dial(ctx, "tcp://192.168.1.20:5000")
```

```go
client, err := meshcore.Dial(ctx, "ws://home-assistant.local/api/meshcore/handheld")
```

Once connected, the application works with the same client methods:

```go
info, err := client.DeviceInfo(ctx)
contacts, err := client.Contacts(ctx)
receipt, err := client.SendText(ctx, "alice", "hello")
```

### Compatible firmware, not one implementation

`meshcore-go` is intended for the broader MeshCore ecosystem rather than one exact firmware build.

The default implementation targets the standard MeshCore companion protocol.

Compatible firmware forks such as ZephCore should work through the same base client wherever they expose the same protocol behavior.

Optional firmware-specific features should be isolated behind capability detection and namespaced extensions.

### Extensible without forking

Custom applications should be able to add:

* transports
* endpoint schemes
* discovery providers
* protocol extensions
* firmware-specific capabilities
* experimental packet handlers

without modifying the core SDK.

---

## Project goals

### SDK goals

Provide a reusable Go package for:

* USB serial companion connections
* BLE companion connections
* TCP connections in a later release
* custom transport implementations
* endpoint discovery
* connection lifecycle management
* request queuing
* response matching
* asynchronous events
* context cancellation
* timeouts
* reconnection policies
* device information
* contacts
* channels
* messages
* acknowledgements
* advertisements
* path management
* telemetry
* repeater administration
* capability negotiation
* firmware extensions

### CLI goals

Provide a simple terminal experience:

```bash
mc connect
mc status
mc contacts
mc inbox
mc send alice "hello"
mc watch
```

After a one-time setup, daily commands should not require BLE addresses or serial paths.

### Automation goals

Support stable structured output:

```bash
mc status --json
mc contacts --json
mc trace repeater-kololec --json
```

Streaming commands should emit newline-delimited JSON:

```bash
mc watch --json
```

---

## Non-goals

The first releases do not aim to:

* reproduce every historical CLI alias
* provide a permanent background daemon by default
* provide a full-screen TUI immediately
* replace mobile companion applications
* couple the SDK to one firmware implementation
* couple the SDK to one transport
* make the SDK responsible for CLI configuration files
* hide all protocol behavior behind a remote HTTP API
* promise compatibility with arbitrary modified firmware automatically

The initial focus is a clean SDK, USB serial support, BLE support, and a practical terminal client.

---

# Architecture

The project separates four concerns:

```text
applications
    │
    ├── mc CLI
    ├── automations
    ├── gateways
    ├── Home Assistant bridges
    └── external Go applications
          │
          ▼
      meshcore.Client
          │
          ├── base companion protocol
          ├── capability detection
          └── optional extensions
          │
          ▼
      transport.PacketConn
          │
          ├── serial
          ├── BLE
          ├── TCP
          ├── WebSocket
          └── custom transports
```

## Layer 1: application API

Applications use high-level methods:

```go
info, err := client.DeviceInfo(ctx)
contacts, err := client.Contacts(ctx)
channels, err := client.Channels(ctx)

receipt, err := client.SendText(ctx, "alice", "hello")
```

Applications should not need to understand:

* serial frame markers
* BLE characteristic UUIDs
* raw packet bytes
* request queues
* response correlation
* asynchronous push codes
* reconnect loops
* firmware-specific quirks

## Layer 2: protocol implementation

The protocol layer owns:

* command encoding
* response decoding
* notification decoding
* protocol initialization
* packet classification
* capability probing
* unknown-packet handling
* protocol-specific errors

## Layer 3: transports

A transport moves complete logical companion-protocol packets.

Each transport owns its own low-level framing:

```text
serial transport
    protocol packet
        ↓
    serial V3 framing
        ↓
    CDC-ACM byte stream

BLE transport
    protocol packet
        ↓
    characteristic write
        ↓
    GATT notification

WebSocket bridge
    protocol packet
        ↓
    binary WebSocket message
        ↓
    remote bridge
```

## Layer 4: CLI

The bundled `mc` client owns:

* configuration files
* saved device profiles
* active device selection
* interactive setup
* human-readable tables
* JSON output
* CLI flags
* exit codes
* shell completion
* diagnostics
* future interactive shell support

---

# Repository layout

```text
meshcore-go/
├── go.mod
├── README.md
├── LICENSE
│
├── client.go
├── dial.go
├── discover.go
├── capabilities.go
├── events.go
├── device.go
├── contacts.go
├── channels.go
├── messages.go
├── paths.go
├── repeaters.go
├── telemetry.go
│
├── protocol/
│   ├── protocol.go
│   ├── registry.go
│   ├── raw.go
│   └── companion/
│       ├── protocol.go
│       ├── commands.go
│       ├── responses.go
│       ├── notifications.go
│       ├── encode.go
│       └── decode.go
│
├── transport/
│   ├── transport.go
│   ├── endpoint.go
│   ├── registry.go
│   │
│   ├── serial/
│   │   ├── serial.go
│   │   ├── framing.go
│   │   └── discover.go
│   │
│   ├── ble/
│   │   ├── ble.go
│   │   ├── discover.go
│   │   └── uuids.go
│   │
│   ├── tcp/
│   │   └── tcp.go
│   │
│   └── websocket/
│       └── websocket.go
│
├── extensions/
│   └── zephcore/
│       ├── extension.go
│       ├── capabilities.go
│       └── commands.go
│
├── internal/
│   ├── queue/
│   ├── dispatcher/
│   └── testutil/
│
├── examples/
│   ├── device-info/
│   ├── discover/
│   ├── send-message/
│   ├── watch-events/
│   └── custom-transport/
│
└── cmd/
    └── mc/
        ├── main.go
        └── internal/
            ├── cli/
            ├── config/
            ├── discovery/
            ├── output/
            └── shell/
```

The root package is imported as:

```go
import meshcore "github.com/meshcore-cz/meshcore-go"
```

The CLI is installed from:

```bash
go install github.com/meshcore-cz/meshcore-go/cmd/mc@latest
```

---

# Transport model

## PacketConn interface

All transports implement a minimal packet-oriented interface:

```go
package transport

import "context"

type PacketConn interface {
	Open(ctx context.Context) error
	Close() error

	ReadPacket(ctx context.Context) ([]byte, error)
	WritePacket(ctx context.Context, packet []byte) error

	String() string
}
```

The SDK client works only with complete logical packets.

Transport-specific framing remains hidden behind each adapter.

## Built-in transports

Initial priorities:

```text
serial://
ble://
```

Planned built-in transports:

```text
tcp://
ws://
wss://
```

Potential external transports:

```text
ha://
mqtt://
unix://
ssh://
custom schemes
```

A scheme does not need to be bundled into the core repository to be usable.

---

## Endpoint URIs

Use explicit URIs consistently:

```text
serial:///dev/ttyACM0
serial:///dev/ttyUSB0
ble://C4:20:12:34:56:78
tcp://192.168.1.20:5000
ws://home-assistant.local/api/meshcore/handheld
```

Benefits:

* explicit transport selection
* easy storage in configuration files
* consistent CLI usage
* simple future extensions
* reusable SDK behavior
* straightforward debugging

Applications may connect through the convenience helper:

```go
client, err := meshcore.Dial(ctx, "serial:///dev/ttyACM0")
```

Advanced applications may instantiate a transport directly:

```go
conn := serial.New(
	"/dev/ttyACM0",
	serial.WithBaud(115200),
)

client := meshcore.New(conn)
```

---

## Transport registry

URI-based dialing should be extensible:

```go
type Dialer interface {
	Dial(ctx context.Context, uri *url.URL) (transport.PacketConn, error)
}

type Registry struct {
	dialers map[string]Dialer
}

func (r *Registry) Register(scheme string, dialer Dialer) {
	r.dialers[scheme] = dialer
}
```

Built-in registration:

```go
registry := transport.NewRegistry()

registry.Register("serial", serial.NewDialer())
registry.Register("ble", ble.NewDialer())
registry.Register("tcp", tcp.NewDialer())
registry.Register("ws", websocket.NewDialer())
registry.Register("wss", websocket.NewDialer())
```

A third-party integration can register its own scheme:

```go
registry.Register("ha", hatransport.NewDialer(token))
```

Usage:

```go
client, err := meshcore.Dial(
	ctx,
	"ha://home-assistant.local/handheld",
	meshcore.WithTransportRegistry(registry),
)
```

---

## Custom transport example

A custom transport can be implemented outside this repository:

```go
type HomeAssistantConn struct {
	BaseURL string
	Token   string
}

func (c *HomeAssistantConn) Open(ctx context.Context) error {
	// Establish a session with the remote bridge.
	return nil
}

func (c *HomeAssistantConn) ReadPacket(
	ctx context.Context,
) ([]byte, error) {
	// Read one complete companion-protocol packet.
	panic("implement me")
}

func (c *HomeAssistantConn) WritePacket(
	ctx context.Context,
	packet []byte,
) error {
	// Forward one complete companion-protocol packet.
	return nil
}

func (c *HomeAssistantConn) Close() error {
	return nil
}

func (c *HomeAssistantConn) String() string {
	return "ha://home-assistant.local/handheld"
}
```

Use it directly:

```go
conn := &HomeAssistantConn{
	BaseURL: "https://home-assistant.local",
	Token:   token,
}

client := meshcore.New(conn)

if err := client.Connect(ctx); err != nil {
	return err
}
defer client.Close()
```

No core SDK changes are required.

---

# Remote bridges

A bridge allows a companion radio attached to one machine to be used from another machine.

Example:

```text
mc
 ↓
WebSocket packet tunnel
 ↓
Home Assistant integration
 ↓
USB / BLE / TCP
 ↓
companion radio
```

The recommended bridge design is a bidirectional packet tunnel.

Each binary WebSocket message should contain one complete logical companion-protocol packet.

```text
client → bridge
    one binary packet

bridge → client
    one binary packet
```

This keeps the bridge simple and future-proof:

* protocol handling stays in `meshcore-go`
* new protocol commands do not require bridge changes
* compatible firmware variants remain usable
* asynchronous events flow naturally
* remote debugging stays transparent

A higher-level HTTP or JSON API may also be useful, but it should be treated as a separate backend abstraction rather than forced into `PacketConn`.

---

# Protocol model

> **Source of truth:** the companion protocol is defined by the firmware, not by
> this repository. Every command code, response code, push-notification code,
> packet layout, field offset and framing detail implemented here must be
> cross-verified against the current firmware implementation:
>
> **https://github.com/meshcore-dev/MeshCore**
>
> When the firmware and this SDK disagree, the firmware wins. Wire-format details
> derived by inspection or hardware capture should be confirmed against the
> firmware source and annotated with the firmware version they were checked
> against. Tolerant decoding (see below) exists precisely because firmware
> evolves ahead of this library.

## Protocol interface

The default client should use the standard companion protocol.

For larger deviations, applications may provide a custom protocol implementation:

```go
type Protocol interface {
	Initialize(
		ctx context.Context,
		conn transport.PacketConn,
	) (SessionInfo, error)

	Encode(command Command) ([]byte, error)
	Decode(packet []byte) (Message, error)

	Capabilities() Capabilities
}
```

Normal usage:

```go
client := meshcore.New(
	conn,
	meshcore.WithProtocol(companion.New()),
)
```

Modified firmware with a diverging wire protocol:

```go
client := meshcore.New(
	conn,
	meshcore.WithProtocol(myfirmware.NewProtocol()),
)
```

A custom protocol driver should be used only where the wire protocol genuinely differs.

Compatible firmware forks should normally reuse the default companion protocol.

---

## Request queue

Only one protocol request should be active at a time.

The SDK owns request sequencing:

```text
application request
        ↓
internal request queue
        ↓
single active request
        ↓
transport write
        ↓
incoming packet dispatcher
        ├── expected response → waiting request
        └── notification      → event stream
```

Conceptual client structure:

```go
type Client struct {
	conn     transport.PacketConn
	protocol protocol.Protocol

	requests chan request
	events   chan Event

	done chan struct{}
}
```

Applications must not need to manage command timing manually.

---

## Tolerant decoding

Modified firmware may add fields, packet types, or notifications.

The decoder should degrade gracefully.

Recommended behavior:

| Situation                       | Behavior                                     |
| ------------------------------- | -------------------------------------------- |
| Known packet type               | Decode into a typed structure                |
| Known packet with trailing data | Decode known fields and retain remainder     |
| Unknown notification            | Emit `RawEvent`                              |
| Unknown response                | Return typed raw response                    |
| Extension response              | Offer packet to registered extension decoder |
| Malformed packet                | Return parsing error                         |
| Unsupported capability          | Return `ErrUnsupportedCapability`            |

Example raw event:

```go
type RawEvent struct {
	Type    byte
	Payload []byte
}
```

Applications can inspect unknown events:

```go
for event := range client.Events() {
	switch event := event.(type) {
	case meshcore.MessageReceived:
		fmt.Println(event.Text)

	case meshcore.RawEvent:
		log.Printf(
			"unknown event type=0x%02x payload=%x",
			event.Type,
			event.Payload,
		)
	}
}
```

---

# Capabilities and firmware variants

## Capability negotiation

Feature detection should be based on capabilities rather than firmware names.

Avoid:

```go
if info.FirmwareName == "zephcore" {
	// Enable telemetry.
}
```

Prefer:

```go
if client.Capabilities().Has(meshcore.CapabilityTelemetry) {
	data, err := client.RequestTelemetry(ctx, target)
}
```

This matters because a capability may appear in:

* upstream MeshCore
* ZephCore
* another compatible firmware fork
* one build profile but not another
* future protocol versions
* experimental firmware branches

Firmware identity remains useful for diagnostics:

```go
type DeviceInfo struct {
	Name            string
	PublicKey       string
	FirmwareName    string
	FirmwareVersion string
	ProtocolVersion string

	Capabilities Capabilities
	Extensions   map[string]ExtensionInfo
}
```

Example:

```go
info, err := client.DeviceInfo(ctx)
if err != nil {
	return err
}

fmt.Println(info.FirmwareName)
fmt.Println(info.FirmwareVersion)
fmt.Println(info.Capabilities)
```

---

## Capability representation

```go
type Capability string

const (
	CapabilityContacts          Capability = "contacts"
	CapabilityChannels          Capability = "channels"
	CapabilityMessages          Capability = "messages"
	CapabilityAcknowledgements  Capability = "acknowledgements"
	CapabilityAdvertisements    Capability = "advertisements"
	CapabilityTelemetry         Capability = "telemetry"
	CapabilityTracing           Capability = "tracing"
	CapabilityRepeaterLogin     Capability = "repeater.login"
	CapabilityRepeaterCommands  Capability = "repeater.commands"
	CapabilityPrivateKeyExport  Capability = "private-key-export"
)
```

```go
type Capabilities map[Capability]bool

func (c Capabilities) Has(cap Capability) bool {
	return c[cap]
}
```

Unsupported features should return a predictable error:

```go
var ErrUnsupportedCapability = errors.New("unsupported capability")
```

---

## Firmware extensions

Fork-specific features should not pollute the base client API.

Avoid:

```go
client.SetZephCoreBackoffMultiplier(...)
client.SetExperimentalForkOption(...)
client.EnableSomeCustomFirmwareMode(...)
```

Use namespaced extension packages:

```text
extensions/
└── zephcore/
    ├── extension.go
    ├── capabilities.go
    └── commands.go
```

Usage:

```go
ext, err := zephcore.From(client)
if err != nil {
	return err
}

status, err := ext.BackoffStatus(ctx)
if err != nil {
	return err
}
```

Extension packages may expose firmware-specific methods:

```go
status, err := ext.BackoffStatus(ctx)
err := ext.SetBackoffMultiplier(ctx, 0.75)
```

The base client remains stable:

```go
client.DeviceInfo(ctx)
client.Contacts(ctx)
client.SendText(ctx, "alice", "hello")
client.Events()
```

---

## ZephCore compatibility goal

ZephCore support is a first-class design goal.

The intended model is:

```text
standard operations
    ↓
default MeshCore companion protocol

ZephCore-specific operations
    ↓
extensions/zephcore
```

The library should not require a separate ZephCore client where the standard companion protocol is sufficient.

Where ZephCore adds optional behavior, expose it through:

* capabilities
* namespaced extensions
* tolerant parsing
* optional custom command decoders

---

# Discovery

## Discovery API

Device discovery should be reusable outside the CLI:

```go
endpoints, err := meshcore.Discover(
	ctx,
	meshcore.WithSerialDiscovery(),
	meshcore.WithBLEDiscovery(),
)
```

Potential endpoint structure:

```go
type Endpoint struct {
	URI       string
	Transport string
	Name      string
	Address   string
	Metadata  map[string]string
}
```

Example result:

```text
USB   serial:///dev/ttyACM0         MeshCore desk radio
BLE   ble://C4:20:12:34:56:78       MeshCore handheld
```

Discovery and protocol verification are separate.

```text
discover endpoint
        ↓
open connection
        ↓
perform handshake
        ↓
read identity
        ↓
treat as verified companion radio
```

A discovered serial port should not automatically be assumed to be a MeshCore device.

---

## Discovery providers

Custom discovery mechanisms should be pluggable:

```go
type Discoverer interface {
	Discover(ctx context.Context) ([]transport.Endpoint, error)
}
```

Built-in providers:

```text
serial discovery
BLE discovery
```

Possible future providers:

```text
mDNS gateway discovery
Home Assistant bridge discovery
static configuration discovery
LAN broadcast discovery
custom fleet-management discovery
```

---

# Events

MeshCore companion radios may emit asynchronous events independently of an application request.

The SDK exposes a typed event stream:

```go
events := client.Events()

for event := range events {
	switch event := event.(type) {
	case meshcore.MessageReceived:
		fmt.Printf("%s: %s\n", event.From.Name, event.Text)

	case meshcore.MessageAcknowledged:
		fmt.Printf("ack: %s\n", event.Code)

	case meshcore.AdvertisementReceived:
		fmt.Printf("advert: %s\n", event.Contact.Name)

	case meshcore.TelemetryReceived:
		fmt.Printf("telemetry: %+v\n", event.Data)

	case meshcore.Disconnected:
		fmt.Printf("disconnected: %v\n", event.Err)
	}
}
```

Initial event types may include:

```go
type Event interface {
	isMeshCoreEvent()
}

type MessageReceived struct {
	From      Contact
	Text      string
	Timestamp time.Time
}

type MessageAcknowledged struct {
	Code string
	RTT  time.Duration
}

type AdvertisementReceived struct {
	Contact Contact
}

type TelemetryReceived struct {
	From Contact
	Data Telemetry
}

type Disconnected struct {
	Err error
}

type RawEvent struct {
	Type    byte
	Payload []byte
}
```

---

# SDK installation

Add the module:

```bash
go get github.com/meshcore-cz/meshcore-go
```

Import it:

```go
import meshcore "github.com/meshcore-cz/meshcore-go"
```

---

# SDK examples

## Connect and print device information

```go
package main

import (
	"context"
	"fmt"
	"log"

	meshcore "github.com/meshcore-cz/meshcore-go"
)

func main() {
	ctx := context.Background()

	client, err := meshcore.Dial(
		ctx,
		"serial:///dev/ttyACM0",
	)
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	info, err := client.DeviceInfo(ctx)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Name: %s\n", info.Name)
	fmt.Printf("Public key: %s\n", info.PublicKey)
	fmt.Printf("Firmware: %s\n", info.FirmwareVersion)
}
```

BLE usage only changes the URI:

```go
client, err := meshcore.Dial(
	ctx,
	"ble://C4:20:12:34:56:78",
)
```

---

## Discover devices

```go
endpoints, err := meshcore.Discover(
	ctx,
	meshcore.WithSerialDiscovery(),
	meshcore.WithBLEDiscovery(),
)
if err != nil {
	log.Fatal(err)
}

for _, endpoint := range endpoints {
	fmt.Printf(
		"%s\t%s\t%s\n",
		endpoint.Transport,
		endpoint.URI,
		endpoint.Name,
	)
}
```

---

## List contacts

```go
contacts, err := client.Contacts(ctx)
if err != nil {
	log.Fatal(err)
}

for _, contact := range contacts {
	fmt.Printf(
		"%s\t%s\n",
		contact.Name,
		contact.PublicKey,
	)
}
```

---

## Send a message

```go
receipt, err := client.SendText(
	ctx,
	"alice",
	"hello",
)
if err != nil {
	log.Fatal(err)
}

fmt.Printf("Queued message: %s\n", receipt.ID)
```

Optionally wait for acknowledgement:

```go
ack, err := client.WaitForAcknowledgement(
	ctx,
	receipt,
)
if err != nil {
	log.Fatal(err)
}

fmt.Printf("Acknowledged after %s\n", ack.RTT)
```

---

## Watch events

```go
for event := range client.Events() {
	switch event := event.(type) {
	case meshcore.MessageReceived:
		fmt.Printf(
			"%s: %s\n",
			event.From.Name,
			event.Text,
		)

	case meshcore.Disconnected:
		log.Printf(
			"disconnected: %v",
			event.Err,
		)
	}
}
```

---

# High-level SDK API

The exact API may evolve during early development.

## Device information

```go
info, err := client.DeviceInfo(ctx)
version, err := client.FirmwareVersion(ctx)
err := client.SyncClock(ctx)
err := client.Reboot(ctx)
err := client.Advertise(ctx)
```

## Contacts

```go
contacts, err := client.Contacts(ctx)
contact, err := client.Contact(ctx, "alice")
uri, err := client.ExportContact(ctx, "alice")
err := client.ImportContact(ctx, uri)
err := client.RemoveContact(ctx, "alice")
```

## Messages

```go
receipt, err := client.SendText(ctx, "alice", "hello")
messages, err := client.SyncMessages(ctx)
ack, err := client.WaitForAcknowledgement(ctx, receipt)
```

## Channels

```go
channels, err := client.Channels(ctx)
channel, err := client.Channel(ctx, "#kololec")
receipt, err := client.SendChannelText(
	ctx,
	"#kololec",
	"hello",
)
```

## Paths and traces

```go
path, err := client.ContactPath(ctx, "alice")
err := client.ResetContactPath(ctx, "alice")
path, err := client.DiscoverContactPath(ctx, "alice")
trace, err := client.Trace(ctx, "repeater-kololec")
```

## Telemetry

```go
telemetry, err := client.RequestTelemetry(
	ctx,
	"sensor-kololec",
)
```

## Repeaters

```go
err := client.RepeaterLogin(
	ctx,
	"repeater-kololec",
	password,
)

status, err := client.RepeaterStatus(
	ctx,
	"repeater-kololec",
)

neighbours, err := client.RepeaterNeighbours(
	ctx,
	"repeater-kololec",
)

response, err := client.RepeaterExec(
	ctx,
	"repeater-kololec",
	"clock",
)
```

---

# mc command-line client

`mc` is the terminal client bundled with `meshcore-go`.

It is intended to be:

* easy to install
* easy to use interactively
* predictable in scripts
* independent of the selected transport
* useful without a background daemon
* a reference implementation for SDK consumers

Install:

```bash
go install github.com/meshcore-cz/meshcore-go/cmd/mc@latest
```

Verify installation:

```bash
mc version
```

---

## First-time setup

Run:

```bash
mc connect
```

The CLI scans for USB serial and BLE companion radios:

```text
Scanning for MeshCore companion radios...

  1. USB   /dev/ttyACM0           MeshCore desk radio
  2. BLE   C4:20:12:34:56:78      MeshCore handheld

Select device [1]: 2
Profile name [handheld]: handheld

Connected successfully.
Saved profile "handheld".
Using "handheld" as the default device.
```

After setup, ordinary commands automatically use the selected profile:

```bash
mc status
mc contacts
mc inbox
mc send alice "hello"
```

---

## Meaning of `mc connect`

`mc connect` is primarily a setup command.

It:

1. discovers available endpoints
2. opens the selected transport
3. performs a protocol handshake
4. reads device identity
5. asks for a profile name
6. saves the profile
7. marks the profile as active

It does not keep a permanent background process running.

A normal command:

```bash
mc status
```

performs:

```text
load active profile
        ↓
open transport
        ↓
initialize session
        ↓
execute operation
        ↓
print result
        ↓
close transport
```

Commands that require a persistent session stay connected until interrupted:

```bash
mc watch
mc shell
```

---

## Connecting devices

Interactive discovery:

```bash
mc connect
```

Scan only USB serial devices:

```bash
mc connect --usb
```

Scan only BLE devices:

```bash
mc connect --ble
```

Connect directly:

```bash
mc connect serial:///dev/ttyACM0
mc connect ble://C4:20:12:34:56:78
```

Save a custom profile name:

```bash
mc connect serial:///dev/ttyACM0 --as desk-radio
mc connect ble://C4:20:12:34:56:78 --as handheld
```

Test without saving:

```bash
mc connect serial:///dev/ttyUSB0 --no-save
```

Run one command against a temporary endpoint:

```bash
mc --uri serial:///dev/ttyUSB0 status
mc --uri ble://C4:20:12:34:56:78 contacts
```

Future gateway usage:

```bash
mc connect tcp://192.168.1.20:5000 --as home-gateway
```

Future bridge usage:

```bash
mc connect ws://home-assistant.local/api/meshcore/handheld \
    --as remote-handheld
```

---

## Device profiles

List saved devices:

```bash
mc device list
```

Example:

```text
NAME             TRANSPORT   ENDPOINT                           DEFAULT
handheld         ble         C4:20:12:34:56:78                  *
desk-radio       serial      /dev/ttyACM0
remote-handheld  ws          home-assistant.local/... 
```

Select a default device:

```bash
mc use handheld
```

Use another profile for one command:

```bash
mc --device desk-radio status
```

Show profile details:

```bash
mc device show handheld
```

Remove a profile:

```bash
mc device remove handheld
```

---

## CLI configuration

The CLI stores configuration in the standard per-user configuration directory.

On Linux:

```text
~/.config/mc/config.yaml
```

If `$XDG_CONFIG_HOME` is defined:

```text
$XDG_CONFIG_HOME/mc/config.yaml
```

Useful commands:

```bash
mc config path
mc config show
mc config edit
```

Example:

```yaml
version: 1

current: handheld

devices:
  handheld:
    name: MeshCore-handheld
    public_key_prefix: "0a53ef34"
    preferred_transport: ble
    transports:
      - uri: ble://C4:20:12:34:56:78

  desk-radio:
    name: MeshCore-desk
    public_key_prefix: "c42012ab"
    preferred_transport: serial
    transports:
      - uri: serial:///dev/ttyACM0
        options:
          baud: 115200

  remote-handheld:
    name: MeshCore-handheld
    public_key_prefix: "0a53ef34"
    preferred_transport: ws
    transports:
      - uri: ws://home-assistant.local/api/meshcore/handheld
```

A logical device may contain multiple endpoints:

```yaml
devices:
  handheld:
    name: MeshCore-handheld
    public_key_prefix: "0a53ef34"
    preferred_transport: ble
    transports:
      - uri: ble://C4:20:12:34:56:78
      - uri: serial:///dev/ttyACM0
        options:
          baud: 115200
```

The CLI may optionally try fallback transports in order.

---

## Common commands

### Status

```bash
mc status
```

Example:

```text
Device:       MeshCore-handheld
Firmware:     MeshCore 1.x.x
Protocol:     companion-v3
Transport:    BLE
Address:      C4:20:12:34:56:78
Public key:   0a53ef34...
Capabilities: contacts, channels, messages, telemetry
```

### Contacts

```bash
mc contacts
mc contact show alice
mc contact import 'meshcore://...'
mc contact export alice
mc contact remove alice
```

### Messaging

```bash
mc send alice "hello"
mc send alice "hello" --wait
mc inbox
mc watch
```

### Channels

```bash
mc channel list
mc channel show '#kololec'
mc channel send '#kololec' "hello"
```

### Paths

```bash
mc contact path alice
mc contact path reset alice
mc contact path discover alice
```

### Traces

```bash
mc trace repeater-kololec
```

### Telemetry

```bash
mc telemetry sensor-kololec
```

### Repeaters

```bash
mc repeater login repeater-kololec
mc repeater logout repeater-kololec
mc repeater status repeater-kololec
mc repeater neighbours repeater-kololec
mc repeater exec repeater-kololec "clock"
```

### Device management

```bash
mc device time
mc device time sync
mc device advertise
mc device reboot
```

---

## Interactive shell

A persistent shell is planned:

```bash
mc shell
```

Example:

```text
Connected to handheld over BLE.
Type "help" to list commands or "exit" to quit.

handheld> contacts
handheld> send alice "hello"
handheld> inbox
handheld> trace repeater-kololec
handheld> exit
```

Potential shell features:

* persistent connection
* command history
* tab completion
* contact-name completion
* channel-name completion
* repeater-name completion
* readable asynchronous notifications
* quick recipient switching

A full-screen TUI may be added later:

```bash
mc tui
```

---

## JSON output

Human-readable output is the default:

```bash
mc status
```

Structured output:

```bash
mc status --json
mc contacts --json
mc trace repeater-kololec --json
```

Streaming commands emit newline-delimited JSON:

```bash
mc watch --json
```

Example:

```json
{"type":"message","from":"alice","text":"hello","timestamp":"2026-06-06T12:00:00+02:00"}
{"type":"ack","contact":"alice","rtt_ms":1832}
{"type":"advert","name":"repeater-kololec"}
```

Errors should be written to stderr.

Commands should return meaningful exit codes.

---

## Diagnostics

Run checks:

```bash
mc doctor
```

Example:

```text
Configuration file                 ok
Default profile                    handheld
Transport                          BLE
Companion radio                    reachable
Protocol handshake                 ok
Firmware                           ZephCore
Protocol                           companion-v3
Clock difference                   3s
```

Verbose logging:

```bash
mc --debug status
```

---

## Planned CLI overview

```text
mc connect
mc status
mc contacts
mc inbox
mc send
mc watch
mc trace
mc telemetry
mc shell
mc tui

mc use

mc device list
mc device show
mc device remove
mc device advertise
mc device reboot
mc device time
mc device time sync

mc contact show
mc contact import
mc contact export
mc contact remove
mc contact path
mc contact path reset
mc contact path discover

mc channel list
mc channel show
mc channel send

mc repeater login
mc repeater logout
mc repeater status
mc repeater neighbours
mc repeater exec

mc config path
mc config show
mc config edit

mc doctor
mc version
mc completion
```

---

# CLI design rules

## Prefer clear commands

Use:

```bash
mc repeater neighbours repeater-kololec
```

instead of opaque abbreviations.

Use:

```bash
mc device time sync
```

instead of memorized aliases.

## Keep shortcuts for daily workflows

Common actions deserve concise top-level commands:

```bash
mc status
mc contacts
mc inbox
mc send
mc watch
mc trace
```

## Use flags consistently

Use:

```bash
mc status --json
```

for structured output.

Use:

```bash
mc --device handheld status
```

for a saved profile.

Use:

```bash
mc --uri serial:///dev/ttyUSB0 status
```

for an explicit temporary endpoint.

## Hide transport details during normal usage

Users should usually run:

```bash
mc status
```

not:

```bash
mc --ble-address C4:20:12:34:56:78 status
```

---

# Testing strategy

All protocol fixtures and golden tests must reflect the current firmware
implementation at **https://github.com/meshcore-dev/MeshCore**. Captured packet
fixtures should record the firmware version they were taken from, and decoders
should be re-checked against firmware source whenever they are added or changed.

## Unit tests

Test:

* packet encoding
* packet decoding
* malformed packets
* unknown packets
* serial framing
* URI parsing
* registry behavior
* capability detection
* event dispatch
* request timeouts
* request queues
* config parsing
* JSON output

## Golden tests

Use known packet fixtures:

```text
packet bytes
    ↓
decoder
    ↓
expected Go structure
```

And encoding fixtures:

```text
Go command structure
    ↓
encoder
    ↓
expected packet bytes
```

## Fake transport

Provide an internal fake transport:

```go
type FakeTransport struct {
	ReadPackets    chan []byte
	WrittenPackets chan []byte
}
```

This allows deterministic testing of:

* request queues
* response matching
* event dispatch
* timeouts
* reconnect behavior
* unknown packets
* extension decoders

without physical hardware.

## Hardware tests

Test separately against real devices:

```text
upstream MeshCore companion over USB serial
upstream MeshCore companion over BLE
ZephCore companion over USB serial
ZephCore companion over BLE
multiple firmware versions
reconnect scenarios
offline message synchronization
acknowledgements
advertisements
telemetry
repeaters
remote packet bridges
```

Hardware tests should remain separate from the default unit-test suite.

---

# Development roadmap

## Phase 1: foundation

Implement:

```text
PacketConn interface
transport registry
endpoint URI parsing
protocol interface
default companion protocol
request queue
response matching
event dispatcher
context cancellation
timeouts
fake transport
packet fixtures
```

## Phase 2: serial

Implement:

```text
serial transport
serial V3 framing
serial discovery
device handshake
device information
mc connect --usb
mc status
mc doctor
```

## Phase 3: BLE

Implement:

```text
BLE scanning
BLE connection lifecycle
service filtering
notification subscriptions
device handshake
mc connect --ble
```

USB serial and BLE are both first-release priorities.

## Phase 4: messaging

Implement:

```text
contacts
channels
pending-message synchronization
direct messages
channel messages
acknowledgements
event stream
mc contacts
mc inbox
mc send
mc watch
```

## Phase 5: advanced operations

Implement:

```text
contact import and export
path management
route tracing
telemetry
repeater administration
clock synchronization
advertisements
JSON schemas
shell completion
```

## Phase 6: compatibility

Implement:

```text
capability negotiation
RawEvent handling
extension registry
ZephCore extension package
firmware compatibility tests
```

## Phase 7: remote transports

Implement:

```text
TCP transport
WebSocket packet transport
bridge protocol documentation
Home Assistant bridge prototype
custom dialer examples
```

## Phase 8: persistent workflows

Implement:

```text
interactive shell
optional reconnect policies
optional TUI
optional daemon experiments
```

---

# Versioning

The SDK and bundled CLI are released together:

```text
meshcore-go v0.1.0
mc         v0.1.0
```

Version output:

```bash
mc version
```

Example:

```text
mc        v0.1.0
meshcore   v0.1.0
commit     a12bc34
go         go1.x
os         linux
arch       amd64
```

Until `v1.0.0`, public APIs may change as protocol coverage and real-world testing improve.

---

# Relation to existing projects

## Existing MeshCore CLI

The existing Python CLI remains useful and feature-rich.

`meshcore-go` is not intended as a line-by-line port.

The new architecture is:

```text
reusable SDK
      ↓
thin CLI
      ↓
saved profiles
      ↓
transport-independent commands
      ↓
capabilities and extensions
```

Useful workflows can be added incrementally without copying every historical alias.

## meshcore-ha

A future goal is to support remote access through bridges such as a Home Assistant integration.

The recommended approach is a raw bidirectional packet tunnel over WebSocket.

This keeps the bridge small and allows `meshcore-go` to own the protocol behavior.

## ZephCore

ZephCore support is a first-class compatibility goal.

Standard operations should use the default companion-protocol implementation.

ZephCore-specific behavior should remain isolated under:

```text
extensions/zephcore
```

---

# Contributing

Early feedback and contributions are welcome.

Useful areas include:

* protocol coverage
* packet fixtures
* BLE behavior across operating systems
* serial discovery
* reconnect behavior
* capability detection
* compatible-firmware testing
* ZephCore integration
* custom transports
* WebSocket bridges
* Home Assistant bridges
* JSON schemas
* CLI ergonomics
* examples
* documentation

Before adding CLI-only behavior, consider whether the underlying feature belongs in the reusable SDK.

Before adding firmware-specific behavior, consider whether it belongs in a namespaced extension.

Before adding a new connection type, consider whether it can be implemented as `transport.PacketConn`.

---

# License

To be decided.

A permissive license such as MIT would make the SDK easy to reuse across open-source MeshCore integrations.

