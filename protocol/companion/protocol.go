package companion

import (
	"context"
	"fmt"

	"github.com/meshcore-dev/meshcore-go/protocol"
	"github.com/meshcore-dev/meshcore-go/transport"
)

// ProtocolVersion is reported in SessionInfo for the standard companion wire
// format implemented here.
const ProtocolVersion = "companion-v3"

// Protocol implements protocol.Protocol for the standard MeshCore companion
// protocol.
type Protocol struct {
	// AppName overrides the identifier reported during the handshake.
	AppName string
}

// New returns a default companion protocol driver.
func New() *Protocol { return &Protocol{} }

// Compile-time assertion that Protocol satisfies the interface.
var _ protocol.Protocol = (*Protocol)(nil)

// Encode implements protocol.Protocol.
func (p *Protocol) Encode(cmd protocol.Command) ([]byte, error) {
	return encode(cmd)
}

// Decode implements protocol.Protocol.
func (p *Protocol) Decode(packet []byte) (protocol.Message, error) {
	return decode(packet)
}

// Capabilities reports the features the base companion protocol exposes.
func (p *Protocol) Capabilities() protocol.Capabilities {
	return protocol.Capabilities{
		protocol.CapabilityContacts:         true,
		protocol.CapabilityChannels:         true,
		protocol.CapabilityMessages:         true,
		protocol.CapabilityAcknowledgements: true,
		protocol.CapabilityAdvertisements:   true,
		protocol.CapabilityTracing:          true,
		protocol.CapabilityRepeaterLogin:    true,
		protocol.CapabilityRepeaterCommands: true,
	}
}

// Initialize performs the APP_START handshake, reads the SelfInfo identity and
// best-effort device details, and returns the resulting SessionInfo.
func (p *Protocol) Initialize(ctx context.Context, conn transport.PacketConn) (protocol.SessionInfo, error) {
	var si protocol.SessionInfo

	start, err := encode(AppStart{Name: p.AppName})
	if err != nil {
		return si, err
	}
	if err := conn.WritePacket(ctx, start); err != nil {
		return si, fmt.Errorf("companion: app start: %w", err)
	}

	msg, err := p.readNext(ctx, conn)
	if err != nil {
		return si, fmt.Errorf("companion: reading self info: %w", err)
	}
	self, ok := msg.(SelfInfo)
	if !ok {
		return si, fmt.Errorf("companion: %w: expected self info, got %T", protocol.ErrUnexpectedResponse, msg)
	}

	si = protocol.SessionInfo{
		Name:            self.Name,
		PublicKey:       self.PublicKey,
		FirmwareName:    "MeshCore",
		ProtocolVersion: ProtocolVersion,
		Capabilities:    p.Capabilities(),
	}

	// Device query is best-effort: older firmware may not implement it.
	if dev, err := p.deviceQuery(ctx, conn); err == nil {
		if dev.FirmwareName != "" {
			si.FirmwareName = dev.FirmwareName
		}
		si.FirmwareVersion = firmwareVersion(dev)
	}

	return si, nil
}

// deviceQuery issues CMD_DEVICE_QUERY and returns the parsed DeviceInfo.
func (p *Protocol) deviceQuery(ctx context.Context, conn transport.PacketConn) (DeviceInfo, error) {
	q, err := encode(DeviceQuery{})
	if err != nil {
		return DeviceInfo{}, err
	}
	if err := conn.WritePacket(ctx, q); err != nil {
		return DeviceInfo{}, err
	}
	msg, err := p.readNext(ctx, conn)
	if err != nil {
		return DeviceInfo{}, err
	}
	dev, ok := msg.(DeviceInfo)
	if !ok {
		return DeviceInfo{}, protocol.ErrUnexpectedResponse
	}
	return dev, nil
}

// readNext reads packets until a non-push message arrives, so a stray
// notification during the handshake does not derail response matching.
func (p *Protocol) readNext(ctx context.Context, conn transport.PacketConn) (protocol.Message, error) {
	for {
		pkt, err := conn.ReadPacket(ctx)
		if err != nil {
			return nil, err
		}
		msg, err := p.Decode(pkt)
		if err != nil {
			return nil, err
		}
		if !msg.Async() {
			return msg, nil
		}
	}
}

func firmwareVersion(d DeviceInfo) string {
	if d.FirmwareBuild != "" {
		return d.FirmwareBuild
	}
	if d.FirmwareVersion != 0 {
		return fmt.Sprintf("v%d", d.FirmwareVersion)
	}
	return ""
}
