package meshcore

import (
	"errors"
	"testing"

	"github.com/meshcore-cz/meshcore-go/protocol"
	"github.com/meshcore-cz/meshcore-go/protocol/companion"
)

func TestChannelReceiptFrom(t *testing.T) {
	// Broadcast channel send: device replies OK (no ack code).
	r, err := channelReceiptFrom(companion.OK{}, "#rem-ha")
	if err != nil {
		t.Fatalf("OK response: %v", err)
	}
	if r.To != "#rem-ha" || r.AckCode != 0 {
		t.Fatalf("OK receipt = %+v, want to=#rem-ha ack=0", r)
	}

	// Some firmware echoes SENT with an ack code.
	r, err = channelReceiptFrom(companion.Sent{ExpectedAck: 0x1234}, "#rem-ha")
	if err != nil {
		t.Fatalf("SENT response: %v", err)
	}
	if r.AckCode != 0x1234 {
		t.Fatalf("SENT receipt ack = %x, want 1234", r.AckCode)
	}

	// Device rejection.
	if _, err := channelReceiptFrom(companion.Err{Code: 3}, "#rem-ha"); err == nil {
		t.Fatal("Err response should return an error")
	}

	// Anything else is unexpected.
	if _, err := channelReceiptFrom(companion.NoMoreMessages{}, "#rem-ha"); !errors.Is(err, protocol.ErrUnexpectedResponse) {
		t.Fatalf("unexpected response err = %v, want ErrUnexpectedResponse", err)
	}
}
