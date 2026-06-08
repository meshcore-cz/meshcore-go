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

	// Incoming direct message, heard twice over different paths.
	msgTime := time.Now()
	in := MessageRecord{
		Direction: MessageIn, Kind: MessageDirect, Peer: "abc123", Text: "hi",
		Timestamp: msgTime, Status: StatusReceived,
	}
	created, err := store.RecordReceivedMessage(ctx, &in, Reception{SNR: 7.5, PathLen: 0})
	if err != nil {
		t.Fatal(err)
	}
	if !created {
		t.Fatal("first hearing should create a new record")
	}
	// Same message heard again over a 2-hop path appends a reception, no new row.
	dup := MessageRecord{Direction: MessageIn, Kind: MessageDirect, Peer: "abc123", Text: "hi", Timestamp: msgTime}
	if created, err := store.RecordReceivedMessage(ctx, &dup, Reception{SNR: 4.0, PathLen: 2}); err != nil {
		t.Fatal(err)
	} else if created {
		t.Fatal("second hearing should not create a new record")
	}

	// Outgoing message: queued -> sent (with ack) -> acked twice (two paths).
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
	if err := store.AppendAck(ctx, "deadbeef", Reception{At: time.Now().UTC(), RTTMs: 1200}); err != nil {
		t.Fatal(err)
	}
	if err := store.AppendAck(ctx, "deadbeef", Reception{At: time.Now().UTC(), RTTMs: 1500}); err != nil {
		t.Fatal(err)
	}

	// Unread incoming filter returns one record with two receptions.
	unread, err := store.Messages(ctx, MessageFilter{Direction: MessageIn, UnreadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(unread) != 1 || unread[0].Text != "hi" || unread[0].SNR != 7.5 {
		t.Fatalf("unread incoming = %+v, want one 'hi' with snr 7.5", unread)
	}
	if len(unread[0].Receptions) != 2 {
		t.Fatalf("incoming receptions = %d, want 2", len(unread[0].Receptions))
	}
	if unread[0].Receptions[0].PathLen != 0 || unread[0].Receptions[1].PathLen != 2 {
		t.Fatalf("reception path lengths = %d,%d, want 0,2",
			unread[0].Receptions[0].PathLen, unread[0].Receptions[1].PathLen)
	}

	// Outgoing reflects delivered status with both acks recorded.
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
	if len(outgoing[0].Receptions) != 2 {
		t.Fatalf("outgoing acks = %d, want 2", len(outgoing[0].Receptions))
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

	// A channel message in a different conversation.
	chMsg := MessageRecord{
		Direction: MessageIn, Kind: MessageChannel, Peer: "chankey", Channel: "0",
		Text: "ahoj", Timestamp: time.Now(), Status: StatusReceived,
	}
	if _, err := store.RecordReceivedMessage(ctx, &chMsg, Reception{}); err != nil {
		t.Fatal(err)
	}

	// Per-target filters isolate each conversation.
	direct, err := store.Messages(ctx, MessageFilter{Kind: MessageDirect, Peer: "abc123"})
	if err != nil {
		t.Fatal(err)
	}
	if len(direct) != 2 {
		t.Fatalf("direct conversation = %d, want 2", len(direct))
	}
	for _, rec := range direct {
		if rec.Kind != MessageDirect || rec.Peer != "abc123" {
			t.Fatalf("direct filter leaked %+v", rec)
		}
	}

	channel, err := store.Messages(ctx, MessageFilter{Kind: MessageChannel, Channel: "0"})
	if err != nil {
		t.Fatal(err)
	}
	if len(channel) != 1 || channel[0].Text != "ahoj" {
		t.Fatalf("channel conversation = %+v, want one 'ahoj'", channel)
	}
}
