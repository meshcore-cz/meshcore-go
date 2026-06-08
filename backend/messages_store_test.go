package backend

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestStateStoreMessages(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	store, err := OpenStateStore(strings.Repeat("a", 64))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()

	// Incoming direct message.
	in := MessageRecord{
		Direction: MessageIn, Kind: MessageDirect, Peer: "abc123", Text: "hi",
		Timestamp: time.Now(), SNR: 7.5, Status: StatusReceived,
	}
	if err := store.InsertMessage(ctx, &in); err != nil {
		t.Fatal(err)
	}
	if in.ID == 0 {
		t.Fatal("InsertMessage did not set ID")
	}

	// Outgoing message: queued -> sent (with ack) -> delivered.
	out := MessageRecord{
		Direction: MessageOut, Kind: MessageDirect, Peer: "abc123", Text: "yo",
		Timestamp: time.Now(), Status: StatusQueued, Read: true,
	}
	if err := store.InsertMessage(ctx, &out); err != nil {
		t.Fatal(err)
	}
	if err := store.SetMessageStatus(ctx, out.ID, StatusSent, "deadbeef"); err != nil {
		t.Fatal(err)
	}
	if err := store.SetMessageStatusByAck(ctx, "deadbeef", StatusDelivered); err != nil {
		t.Fatal(err)
	}

	// Unread incoming filter returns only the incoming message.
	unread, err := store.Messages(ctx, MessageFilter{Direction: MessageIn, UnreadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(unread) != 1 || unread[0].Text != "hi" || unread[0].SNR != 7.5 {
		t.Fatalf("unread incoming = %+v, want one 'hi' with snr 7.5", unread)
	}

	// Outgoing message reflects the delivered status.
	outgoing, err := store.Messages(ctx, MessageFilter{Direction: MessageOut})
	if err != nil {
		t.Fatal(err)
	}
	if len(outgoing) != 1 || outgoing[0].Status != StatusDelivered || outgoing[0].AckCode != "deadbeef" {
		t.Fatalf("outgoing = %+v, want one delivered with ack deadbeef", outgoing)
	}
	if outgoing[0].AcknowledgedAt.IsZero() {
		t.Fatal("delivered message has no acknowledged_at timestamp")
	}

	// Mark read, then the unread filter is empty.
	if err := store.MarkMessagesRead(ctx, []int64{in.ID}); err != nil {
		t.Fatal(err)
	}
	unread, err = store.Messages(ctx, MessageFilter{Direction: MessageIn, UnreadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(unread) != 0 {
		t.Fatalf("after MarkMessagesRead: %d unread, want 0", len(unread))
	}

	// All messages, oldest first.
	all, err := store.Messages(ctx, MessageFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("all messages = %d, want 2", len(all))
	}
}
