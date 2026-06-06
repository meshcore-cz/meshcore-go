package protocol

import (
	"sort"
	"strings"
)

// Capability identifies an optional protocol feature. Feature detection should
// be based on capabilities rather than firmware names so that the same code
// works across upstream MeshCore, ZephCore and other compatible forks.
type Capability string

// Known capabilities. This list is open-ended: extensions may define more.
const (
	CapabilityContacts         Capability = "contacts"
	CapabilityChannels         Capability = "channels"
	CapabilityMessages         Capability = "messages"
	CapabilityAcknowledgements Capability = "acknowledgements"
	CapabilityAdvertisements   Capability = "advertisements"
	CapabilityTelemetry        Capability = "telemetry"
	CapabilityTracing          Capability = "tracing"
	CapabilityRepeaterLogin    Capability = "repeater.login"
	CapabilityRepeaterCommands Capability = "repeater.commands"
	CapabilityPrivateKeyExport Capability = "private-key-export"
)

// Capabilities is a set of advertised features.
type Capabilities map[Capability]bool

// Has reports whether the capability is present.
func (c Capabilities) Has(cap Capability) bool {
	return c[cap]
}

// List returns the present capabilities as a sorted string slice.
func (c Capabilities) List() []string {
	out := make([]string, 0, len(c))
	for cap, ok := range c {
		if ok {
			out = append(out, string(cap))
		}
	}
	sort.Strings(out)
	return out
}

// String renders the capability set as a comma-separated list.
func (c Capabilities) String() string {
	return strings.Join(c.List(), ", ")
}
