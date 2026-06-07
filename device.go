package meshcore

import (
	"context"
	"time"

	"github.com/meshcore-cz/meshcore-go/protocol"
	"github.com/meshcore-cz/meshcore-go/protocol/companion"
)

// DeviceInfo describes the connected radio's identity and capabilities.
type DeviceInfo struct {
	Name            string
	PublicKey       string
	FirmwareName    string
	FirmwareVersion string
	ProtocolVersion string
	RadioFreqKHz    uint32
	RadioBWKHz      uint32
	RadioSF         byte
	RadioCR         byte
	TxPowerDBm      byte
	Latitude        float64
	Longitude       float64

	Capabilities Capabilities
	Extensions   map[string]ExtensionInfo
}

// ExtensionInfo describes a firmware-specific extension (Phase 6).
type ExtensionInfo struct {
	Name    string
	Version string
}

// Transport returns the underlying transport's endpoint description.
func (c *Client) Transport() string { return c.conn.String() }

// DeviceInfo returns the identity learned during the handshake, refreshing
// firmware details from the device when the initial connect-time query failed.
func (c *Client) DeviceInfo(ctx context.Context) (DeviceInfo, error) {
	c.mu.RLock()
	s := c.session
	c.mu.RUnlock()

	info := DeviceInfo{
		Name:            s.Name,
		PublicKey:       s.PublicKey,
		FirmwareName:    s.FirmwareName,
		FirmwareVersion: s.FirmwareVersion,
		ProtocolVersion: s.ProtocolVersion,
		RadioFreqKHz:    s.RadioFreqKHz,
		RadioBWKHz:      s.RadioBWKHz,
		RadioSF:         s.RadioSF,
		RadioCR:         s.RadioCR,
		TxPowerDBm:      s.TxPowerDBm,
		Latitude:        s.Latitude,
		Longitude:       s.Longitude,
		Capabilities:    s.Capabilities,
		Extensions:      map[string]ExtensionInfo{},
	}
	if info.FirmwareVersion != "" {
		return info, nil
	}

	dev, err := c.queryDeviceInfo(ctx)
	if err != nil {
		return info, nil
	}
	info.FirmwareVersion = companion.FirmwareVersion(dev)
	if dev.Model != "" {
		info.FirmwareName = "MeshCore (" + dev.Model + ")"
	}

	c.mu.Lock()
	if c.session.FirmwareVersion == "" {
		c.session.FirmwareVersion = info.FirmwareVersion
		if dev.Model != "" {
			c.session.FirmwareName = info.FirmwareName
		}
	}
	c.mu.Unlock()

	return info, nil
}

func (c *Client) queryDeviceInfo(ctx context.Context) (companion.DeviceInfo, error) {
	msg, err := c.request(ctx, companion.DeviceQuery{})
	if err != nil {
		return companion.DeviceInfo{}, err
	}
	dev, ok := msg.(companion.DeviceInfo)
	if !ok {
		return companion.DeviceInfo{}, protocol.ErrUnexpectedResponse
	}
	return dev, nil
}

// FirmwareVersion returns the firmware version string reported by the device.
func (c *Client) FirmwareVersion(ctx context.Context) (string, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.session.FirmwareVersion, nil
}

// DeviceTime queries the device's current clock.
func (c *Client) DeviceTime(ctx context.Context) (time.Time, error) {
	msg, err := c.request(ctx, companion.GetDeviceTime{})
	if err != nil {
		return time.Time{}, err
	}
	t, ok := msg.(companion.CurrentTime)
	if !ok {
		return time.Time{}, protocol.ErrUnexpectedResponse
	}
	return t.Time, nil
}

// SyncClock sets the device clock to the host's current time.
func (c *Client) SyncClock(ctx context.Context) error {
	_, err := c.request(ctx, companion.SetDeviceTime{Time: time.Now()})
	return err
}

// Advertise broadcasts the device's own advertisement. When flood is false the
// device sends a zero-hop advert (neighbours only); when true it sends a flood
// advert that propagates across the mesh.
func (c *Client) Advertise(ctx context.Context, flood bool) error {
	if err := c.requireCapability(CapabilityAdvertisements); err != nil {
		return err
	}
	_, err := c.request(ctx, companion.SendSelfAdvert{Flood: flood})
	return err
}

// Reboot asks the device to restart. The device may drop the connection
// immediately, so a timeout error here is not necessarily a failure.
func (c *Client) Reboot(ctx context.Context) error {
	_, err := c.request(ctx, companion.Reboot{})
	return err
}

// BatteryMillivolts reads the battery voltage in millivolts.
func (c *Client) BatteryMillivolts(ctx context.Context) (uint16, error) {
	msg, err := c.request(ctx, companion.GetBatteryVoltage{})
	if err != nil {
		return 0, err
	}
	b, ok := msg.(companion.BatteryVoltage)
	if !ok {
		return 0, protocol.ErrUnexpectedResponse
	}
	return b.Millivolts, nil
}
