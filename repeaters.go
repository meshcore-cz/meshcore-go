package meshcore

import (
	"context"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/meshcore-dev/meshcore-go/protocol"
	"github.com/meshcore-dev/meshcore-go/protocol/companion"
)

// RepeaterResponse is a text response returned by a remote repeater CLI.
type RepeaterResponse struct {
	Repeater string
	Command  string
	Text     string
	Received time.Time
}

// RepeaterLogin logs in to a remote repeater using its saved contact key.
func (c *Client) RepeaterLogin(ctx context.Context, repeater, password string) error {
	if err := c.requireCapability(CapabilityRepeaterLogin); err != nil {
		return err
	}
	ct, key, err := c.repeaterKey(ctx, repeater)
	if err != nil {
		return err
	}
	msg, err := c.request(ctx, companion.SendLogin{
		PublicKey: key,
		Password:  password,
	})
	if err != nil {
		return err
	}
	if _, ok := msg.(companion.OK); !ok {
		return protocol.ErrUnexpectedResponse
	}
	_ = ct
	return nil
}

// RepeaterStatus requests the repeater's CLI status.
func (c *Client) RepeaterStatus(ctx context.Context, repeater string) (RepeaterResponse, error) {
	return c.RepeaterExec(ctx, repeater, "status")
}

// RepeaterNeighbours requests the repeater's CLI neighbour list.
func (c *Client) RepeaterNeighbours(ctx context.Context, repeater string) (RepeaterResponse, error) {
	return c.RepeaterExec(ctx, repeater, "neighbours")
}

// RepeaterExec sends a command to a remote repeater CLI and waits for a text
// response from the same contact.
func (c *Client) RepeaterExec(ctx context.Context, repeater, command string) (RepeaterResponse, error) {
	if err := c.requireCapability(CapabilityRepeaterCommands); err != nil {
		return RepeaterResponse{}, err
	}
	ct, key, err := c.repeaterKey(ctx, repeater)
	if err != nil {
		return RepeaterResponse{}, err
	}
	command = strings.TrimSpace(command)
	if command == "" {
		return RepeaterResponse{}, fmt.Errorf("repeater command is empty")
	}

	msg, err := c.request(ctx, companion.SendTextMessage{
		DestPublicKey: key,
		Text:          command,
		TxtType:       1, // remote CLI command
	})
	if err != nil {
		return RepeaterResponse{}, err
	}
	if _, err := receiptFrom(msg, ct.Name); err != nil {
		return RepeaterResponse{}, err
	}

	timeout := c.timeout * 3
	if timeout < 30*time.Second {
		timeout = 30 * time.Second
	}
	wctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	prefix := ""
	if len(ct.PublicKey) >= 12 {
		prefix = ct.PublicKey[:12]
	}
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-wctx.Done():
			return RepeaterResponse{}, fmt.Errorf("waiting for repeater %s response to %q: %w", ct.Name, command, wctx.Err())
		case <-c.closed:
			return RepeaterResponse{}, fmt.Errorf("meshcore: connection closed")
		case ev, ok := <-c.events.Events():
			if !ok {
				return RepeaterResponse{}, fmt.Errorf("meshcore: event stream closed")
			}
			if msg, ok := ev.(MessageReceived); ok && messageMatchesRepeater(msg, ct, prefix) {
				return RepeaterResponse{Repeater: ct.Name, Command: command, Text: msg.Text, Received: msg.Timestamp}, nil
			}
		case <-ticker.C:
			msgs, err := c.SyncMessages(wctx)
			if err != nil {
				continue
			}
			for _, msg := range msgs {
				if messageMatchesRepeaterMessage(msg, ct, prefix) {
					return RepeaterResponse{Repeater: ct.Name, Command: command, Text: msg.Text, Received: msg.Timestamp}, nil
				}
			}
		}
	}
}

func (c *Client) repeaterKey(ctx context.Context, repeater string) (Contact, []byte, error) {
	ct, err := c.Contact(ctx, repeater)
	if err != nil {
		return Contact{}, nil, err
	}
	if ct.Type != ContactRepeater && ct.Type != ContactRoom && ct.Type != ContactSensor {
		return Contact{}, nil, fmt.Errorf("%q is %s, not a repeater/room/sensor", ct.Name, ct.Type)
	}
	key, err := hex.DecodeString(ct.PublicKey)
	if err != nil || len(key) < 32 {
		return Contact{}, nil, fmt.Errorf("repeater %q has no usable public key", ct.Name)
	}
	return ct, key, nil
}

func messageMatchesRepeater(msg MessageReceived, ct Contact, prefix string) bool {
	from := msg.From.Name
	return strings.EqualFold(from, ct.Name) || (prefix != "" && strings.EqualFold(from, prefix))
}

func messageMatchesRepeaterMessage(msg Message, ct Contact, prefix string) bool {
	return strings.EqualFold(msg.From, ct.Name) || (prefix != "" && strings.EqualFold(msg.From, prefix))
}
