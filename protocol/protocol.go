// Package protocol defines the transport-independent contract between the
// meshcore-go client and a concrete companion-protocol implementation. The
// default implementation lives in protocol/companion; alternative drivers may
// be supplied for firmware with a genuinely diverging wire protocol.
package protocol

import (
	"context"
	"errors"

	"github.com/meshcore-dev/meshcore-go/transport"
)

// Command is a protocol-agnostic handle for an outbound request. Concrete
// command types are defined by each protocol implementation; the encoder
// type-switches on them.
type Command any

// Message is a decoded inbound packet: either a response to a request or an
// unsolicited push notification.
type Message interface {
	// Async reports whether the device emitted this message without a
	// corresponding request (a push notification).
	Async() bool
}

// SessionInfo captures the device identity learned during the protocol
// handshake.
type SessionInfo struct {
	Name            string
	PublicKey       string
	FirmwareName    string
	FirmwareVersion string
	ProtocolVersion string
	Capabilities    Capabilities
}

// Protocol encodes commands, decodes packets and performs the initial
// handshake for a connection.
type Protocol interface {
	// Initialize performs the protocol handshake on a freshly opened
	// connection and reads the device identity. It runs before the client's
	// read loop starts, so it may use conn directly.
	Initialize(ctx context.Context, conn transport.PacketConn) (SessionInfo, error)

	// Encode serialises a command into a complete logical packet.
	Encode(cmd Command) ([]byte, error)

	// Decode parses a complete logical packet into a Message. Unknown packets
	// should degrade to a RawMessage rather than an error.
	Decode(packet []byte) (Message, error)

	// Capabilities reports the features this protocol exposes.
	Capabilities() Capabilities
}

// ErrUnsupportedCapability is returned when a feature is requested that the
// connected device does not advertise.
var ErrUnsupportedCapability = errors.New("meshcore: unsupported capability")

// ErrUnexpectedResponse is returned when the device replies with a message
// that does not match the issued command.
var ErrUnexpectedResponse = errors.New("meshcore: unexpected response")
