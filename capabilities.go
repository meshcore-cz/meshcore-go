package meshcore

import "github.com/meshcore-cz/meshcore-go/protocol"

// Capability and Capabilities are re-exported from the protocol package so
// applications can refer to them as meshcore.Capability* without importing the
// protocol package directly.
type (
	Capability   = protocol.Capability
	Capabilities = protocol.Capabilities
)

// Re-exported capability constants.
const (
	CapabilityContacts         = protocol.CapabilityContacts
	CapabilityChannels         = protocol.CapabilityChannels
	CapabilityMessages         = protocol.CapabilityMessages
	CapabilityAcknowledgements = protocol.CapabilityAcknowledgements
	CapabilityAdvertisements   = protocol.CapabilityAdvertisements
	CapabilityTelemetry        = protocol.CapabilityTelemetry
	CapabilityTracing          = protocol.CapabilityTracing
	CapabilityRepeaterLogin    = protocol.CapabilityRepeaterLogin
	CapabilityRepeaterCommands = protocol.CapabilityRepeaterCommands
	CapabilityPrivateKeyExport = protocol.CapabilityPrivateKeyExport
)

// ErrUnsupportedCapability is returned when a requested feature is not
// advertised by the connected device.
var ErrUnsupportedCapability = protocol.ErrUnsupportedCapability
