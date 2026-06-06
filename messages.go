package meshcore

import (
	"context"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/meshcore-dev/meshcore-go/protocol"
	"github.com/meshcore-dev/meshcore-go/protocol/companion"
)

// Receipt is returned after queuing an outbound message. It carries the ack
// code the device expects the recipient to echo, used by WaitForAcknowledgement.
type Receipt struct {
	To       string        // recipient name or channel
	AckCode  uint32        // expected acknowledgement code
	Timeout  time.Duration // device-suggested wait for the ack
	QueuedAt time.Time
}

// ID returns a short hex identifier for the receipt (its ack code).
func (r Receipt) ID() string { return fmt.Sprintf("%08x", r.AckCode) }

// Ack reports a confirmed message delivery.
type Ack struct {
	Code string
	RTT  time.Duration
}

// Message is a received text message (direct or channel).
type Message struct {
	From      string // sender name, key prefix, or channel
	Channel   string // non-empty for channel messages
	Text      string
	TxtType   byte
	Timestamp time.Time
	SNR       float64
}

// SendText sends a direct text message to a contact (by name or key prefix).
//
// Note: the send wire format is firmware-derived and not yet hardware-verified.
func (c *Client) SendText(ctx context.Context, recipient, text string) (Receipt, error) {
	if err := c.requireCapability(CapabilityMessages); err != nil {
		return Receipt{}, err
	}
	contact, err := c.Contact(ctx, recipient)
	if err != nil {
		return Receipt{}, err
	}
	return c.SendTextToContact(ctx, contact, text)
}

// SendTextToContact sends a direct text message to an already-resolved contact.
func (c *Client) SendTextToContact(ctx context.Context, contact Contact, text string) (Receipt, error) {
	if err := c.requireCapability(CapabilityMessages); err != nil {
		return Receipt{}, err
	}
	key, err := decodeKey(contact.PublicKey)
	if err != nil {
		return Receipt{}, err
	}

	msg, err := c.request(ctx, companion.SendTextMessage{
		DestPublicKey: key,
		Text:          text,
	})
	if err != nil {
		return Receipt{}, err
	}
	return receiptFrom(msg, contact.Name)
}

// SyncMessages drains and returns all messages currently buffered on the
// device, oldest first.
func (c *Client) SyncMessages(ctx context.Context) ([]Message, error) {
	var out []Message
	for {
		msg, more, err := c.syncNext(ctx)
		if err != nil {
			return out, err
		}
		if !more {
			return out, nil
		}
		out = append(out, msg)
	}
}

// syncNext drains a single buffered message. more is false once the buffer is
// empty.
func (c *Client) syncNext(ctx context.Context) (Message, bool, error) {
	msg, err := c.request(ctx, companion.SyncNextMessage{})
	if err != nil {
		return Message{}, false, err
	}
	switch m := msg.(type) {
	case companion.NoMoreMessages:
		return Message{}, false, nil
	case companion.ContactMessage:
		return Message{
			From:      m.SenderPrefix,
			Text:      m.Text,
			TxtType:   m.TxtType,
			Timestamp: m.Timestamp,
			SNR:       m.SNR,
		}, true, nil
	case companion.ChannelMessage:
		return Message{
			From:      fmt.Sprintf("channel %d", m.Channel),
			Channel:   fmt.Sprintf("%d", m.Channel),
			Text:      m.Text,
			TxtType:   m.TxtType,
			Timestamp: m.Timestamp,
			SNR:       m.SNR,
		}, true, nil
	default:
		return Message{}, false, protocol.ErrUnexpectedResponse
	}
}

// WaitForAcknowledgement blocks until a SendConfirmed push matching the
// receipt's ack code arrives, or the context (or receipt timeout) expires.
func (c *Client) WaitForAcknowledgement(ctx context.Context, receipt Receipt) (Ack, error) {
	if err := c.requireCapability(CapabilityAcknowledgements); err != nil {
		return Ack{}, err
	}

	timeout := receipt.Timeout
	if timeout <= 0 {
		timeout = c.timeout
	}
	wctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	for {
		select {
		case <-wctx.Done():
			return Ack{}, wctx.Err()
		case <-c.closed:
			return Ack{}, fmt.Errorf("meshcore: connection closed")
		case code := <-c.acks:
			if code == receipt.AckCode {
				return Ack{
					Code: fmt.Sprintf("%08x", code),
					RTT:  time.Since(receipt.QueuedAt),
				}, nil
			}
		}
	}
}

// decodeKey hex-decodes a contact public key.
func decodeKey(hexKey string) ([]byte, error) {
	b, err := hex.DecodeString(hexKey)
	if err != nil {
		return nil, fmt.Errorf("meshcore: invalid public key %q: %w", hexKey, err)
	}
	if len(b) < 6 {
		return nil, fmt.Errorf("meshcore: public key too short")
	}
	return b, nil
}

// receiptFrom builds a Receipt from a SENT response.
func receiptFrom(msg protocol.Message, to string) (Receipt, error) {
	sent, ok := msg.(companion.Sent)
	if !ok {
		return Receipt{}, protocol.ErrUnexpectedResponse
	}
	return Receipt{
		To:       to,
		AckCode:  sent.ExpectedAck,
		Timeout:  sent.SuggestedTimeout,
		QueuedAt: time.Now(),
	}, nil
}
