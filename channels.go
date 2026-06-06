package meshcore

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/meshcore-dev/meshcore-go/protocol/companion"
)

// maxChannels bounds how many channel slots are probed when enumerating.
const maxChannels = 8

// Channel describes a channel slot on the device.
type Channel struct {
	Index int
	Name  string
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
		if strings.EqualFold(ch.Name, query) {
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
	return Channel{Index: int(info.Index), Name: info.Name}, true, nil
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
	return receiptFrom(msg, "#"+ch.Name)
}
