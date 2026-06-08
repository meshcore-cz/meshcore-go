package meshcore

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strconv"
	"strings"

	"github.com/meshcore-cz/meshcore-go/protocol"
	"github.com/meshcore-cz/meshcore-go/protocol/companion"
)

// maxChannels bounds how many channel slots are probed when enumerating.
const maxChannels = 8

// ChannelSecretLen is the length of a channel pre-shared key.
const ChannelSecretLen = 16

// ChannelDisplayName returns a channel name for display, ensuring a single
// leading '#'. Hashtag channels are stored with the '#'; others are shown with
// one prepended.
func ChannelDisplayName(name string) string {
	if strings.HasPrefix(name, "#") {
		return name
	}
	return "#" + name
}

// DeriveChannelSecret derives the pre-shared key for a name-only ("hashtag")
// channel from its name: the first 16 bytes of SHA-256(name). This lets peers
// share a channel by agreeing only on a name.
//
// NOTE: this derivation is firmware-derived and not yet hardware-verified.
// Confirm interoperability with other MeshCore clients before relying on it.
func DeriveChannelSecret(name string) []byte {
	sum := sha256.Sum256([]byte(name))
	out := make([]byte, ChannelSecretLen)
	copy(out, sum[:ChannelSecretLen])
	return out
}

// Channel describes a channel slot on the device.
type Channel struct {
	Index int
	Name  string
	// Secret is the 16-byte pre-shared key for the channel; it defines the
	// channel's shared identity across devices. Excluded from JSON so the PSK is
	// not exposed over IPC or in command output.
	Secret []byte `json:"-"`
}

// Channels returns the configured (named) channel slots.
func (c *Client) Channels(ctx context.Context) ([]Channel, error) {
	if err := c.requireCapability(CapabilityChannels); err != nil {
		return nil, err
	}

	var channels []Channel
	for i := 0; i < maxChannels; i++ {
		ch, ok, err := c.channelAt(ctx, byte(i))
		if err != nil {
			return nil, err
		}
		if ok && ch.Name != "" {
			channels = append(channels, ch)
		}
	}
	return channels, nil
}

// Channel looks up a channel by name (case-insensitive, leading '#' optional)
// or by slot index.
func (c *Client) Channel(ctx context.Context, name string) (Channel, error) {
	query := strings.TrimPrefix(strings.TrimSpace(name), "#")

	if idx, err := strconv.Atoi(query); err == nil {
		ch, ok, err := c.channelAt(ctx, byte(idx))
		if err != nil {
			return Channel{}, err
		}
		if !ok {
			return Channel{}, fmt.Errorf("no channel at index %d", idx)
		}
		return ch, nil
	}

	channels, err := c.Channels(ctx)
	if err != nil {
		return Channel{}, err
	}
	for _, ch := range channels {
		if strings.EqualFold(strings.TrimPrefix(ch.Name, "#"), query) {
			return ch, nil
		}
	}
	return Channel{}, fmt.Errorf("no channel matching %q", name)
}

func (c *Client) channelAt(ctx context.Context, index byte) (Channel, bool, error) {
	msg, err := c.request(ctx, companion.GetChannel{Index: index})
	if err != nil {
		return Channel{}, false, err
	}
	info, ok := msg.(companion.ChannelInfo)
	if !ok {
		return Channel{}, false, nil
	}
	return Channel{Index: int(info.Index), Name: info.Name, Secret: info.Secret}, true, nil
}

// SendChannelText sends a text message to a channel (by name or index).
//
// Note: the send wire format is firmware-derived and not yet hardware-verified.
func (c *Client) SendChannelText(ctx context.Context, channel, text string) (Receipt, error) {
	if err := c.requireCapability(CapabilityMessages); err != nil {
		return Receipt{}, err
	}
	ch, err := c.Channel(ctx, channel)
	if err != nil {
		return Receipt{}, err
	}

	msg, err := c.request(ctx, companion.SendChannelTextMessage{
		Channel: byte(ch.Index),
		Text:    text,
	})
	if err != nil {
		return Receipt{}, err
	}
	return channelReceiptFrom(msg, ChannelDisplayName(ch.Name))
}

// SetChannelAt writes a channel slot by index. An empty name clears the slot.
func (c *Client) SetChannelAt(ctx context.Context, index byte, name string, secret []byte) error {
	if err := c.requireCapability(CapabilityChannels); err != nil {
		return err
	}
	msg, err := c.request(ctx, companion.SetChannel{Index: index, Name: name, Secret: secret})
	if err != nil {
		return err
	}
	switch m := msg.(type) {
	case companion.OK:
		return nil
	case companion.Err:
		return fmt.Errorf("meshcore: device rejected channel write (error code %d)", m.Code)
	default:
		return protocol.ErrUnexpectedResponse
	}
}

// AddChannel adds a channel by name with the given pre-shared key. If a channel
// with the same name already exists its slot is updated; otherwise the first
// free slot is used. Returns the resulting channel.
func (c *Client) AddChannel(ctx context.Context, name string, secret []byte) (Channel, error) {
	if err := c.requireCapability(CapabilityChannels); err != nil {
		return Channel{}, err
	}
	if name == "" {
		return Channel{}, fmt.Errorf("meshcore: channel name is required")
	}
	if len(secret) != ChannelSecretLen {
		return Channel{}, fmt.Errorf("meshcore: channel key must be %d bytes, got %d", ChannelSecretLen, len(secret))
	}

	existing, free := -1, -1
	for i := 0; i < maxChannels; i++ {
		ch, ok, err := c.channelAt(ctx, byte(i))
		if err != nil {
			return Channel{}, err
		}
		if !ok || ch.Name == "" {
			if free < 0 {
				free = i
			}
			continue
		}
		if strings.EqualFold(ch.Name, name) {
			existing = i
			break
		}
	}
	idx := existing
	if idx < 0 {
		idx = free
	}
	if idx < 0 {
		return Channel{}, fmt.Errorf("meshcore: no free channel slot")
	}
	if err := c.SetChannelAt(ctx, byte(idx), name, secret); err != nil {
		return Channel{}, err
	}
	return Channel{Index: idx, Name: name, Secret: secret}, nil
}

// RemoveChannel clears the channel slot matching query (name or index).
func (c *Client) RemoveChannel(ctx context.Context, query string) (Channel, error) {
	if err := c.requireCapability(CapabilityChannels); err != nil {
		return Channel{}, err
	}
	ch, err := c.Channel(ctx, query)
	if err != nil {
		return Channel{}, err
	}
	if err := c.SetChannelAt(ctx, byte(ch.Index), "", nil); err != nil {
		return Channel{}, err
	}
	return ch, nil
}
