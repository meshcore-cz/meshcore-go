package backend

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// TestSessionMessagesDispatch verifies the "messages" IPC method serves stored
// history filtered per conversation, without touching the radio.
func TestSessionMessagesDispatch(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	store, err := OpenStateStore(strings.Repeat("d", 64))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()

	now := time.Now()
	in := MessageRecord{Direction: MessageIn, Kind: MessageDirect, Peer: "alicekey", Text: "hi", Timestamp: now, Status: StatusReceived}
	if _, err := store.RecordReceivedMessage(ctx, &in, Reception{}); err != nil {
		t.Fatal(err)
	}
	out := MessageRecord{Direction: MessageOut, Kind: MessageDirect, Peer: "alicekey", Text: "yo", Timestamp: now.Add(time.Second), Status: StatusSent, Read: true}
	if err := store.InsertMessage(ctx, &out); err != nil {
		t.Fatal(err)
	}
	chMsg := MessageRecord{Direction: MessageIn, Kind: MessageChannel, Peer: "chankey", Channel: "0", Text: "ahoj", Timestamp: now, Status: StatusReceived}
	if _, err := store.RecordReceivedMessage(ctx, &chMsg, Reception{}); err != nil {
		t.Fatal(err)
	}

	s := &DeviceSession{store: store}

	// Direct conversation with alice returns both directions, oldest first.
	params, _ := json.Marshal(messagesParams{Kind: MessageDirect, Peer: "alicekey"})
	res, err := s.dispatch(ctx, "messages", params)
	if err != nil {
		t.Fatalf("dispatch messages: %v", err)
	}
	recs, ok := res.([]MessageRecord)
	if !ok {
		t.Fatalf("result type = %T, want []MessageRecord", res)
	}
	if len(recs) != 2 || recs[0].Text != "hi" || recs[1].Text != "yo" {
		t.Fatalf("direct messages = %+v, want hi then yo", recs)
	}

	// Channel conversation is isolated.
	params, _ = json.Marshal(messagesParams{Kind: MessageChannel, Channel: "0"})
	res, err = s.dispatch(ctx, "messages", params)
	if err != nil {
		t.Fatalf("dispatch channel messages: %v", err)
	}
	recs = res.([]MessageRecord)
	if len(recs) != 1 || recs[0].Text != "ahoj" {
		t.Fatalf("channel messages = %+v, want one ahoj", recs)
	}
}

// TestSessionMessagesNoStore verifies a clear error when no store is configured.
func TestSessionMessagesNoStore(t *testing.T) {
	s := &DeviceSession{}
	if _, err := s.dispatch(context.Background(), "messages", nil); err == nil {
		t.Fatal("expected error when store is unavailable")
	}
}
