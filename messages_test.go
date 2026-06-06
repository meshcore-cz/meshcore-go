package meshcore_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"testing"
	"time"

	meshcore "github.com/meshcore-dev/meshcore-go"
	"github.com/meshcore-dev/meshcore-go/internal/testutil"
)

// packet builders for messaging responses.

func contactsStartPacket(n uint32) []byte { return append([]byte{2}, le32(n)...) }
func endOfContactsPacket() []byte         { return append([]byte{4}, le32(0)...) }

func contactPacket(name string, key []byte, typ byte) []byte {
	body := make([]byte, 147)
	copy(body[0:32], key)
	body[32] = typ
	body[34] = 0xff // out_path_len = -1
	copy(body[99:131], []byte(name))
	return append([]byte{3}, body...)
}

func channelInfoPacket(index byte, name string) []byte {
	body := make([]byte, 49)
	body[0] = index
	copy(body[1:33], []byte(name))
	return append([]byte{18}, body...)
}

func sentPacket(ack uint32, timeoutMS uint32) []byte {
	p := []byte{6, 0}
	p = append(p, le32(ack)...)
	return append(p, le32(timeoutMS)...)
}

func sendConfirmedPacket(ack uint32) []byte {
	return append([]byte{0x82}, le32(ack)...)
}

func contactMessagePacket(prefix []byte, text string, ts time.Time) []byte {
	p := append([]byte{7}, prefix[:6]...)
	p = append(p, 0, 0) // path_len, txt_type
	p = append(p, le32(uint32(ts.Unix()))...)
	return append(p, []byte(text)...)
}

func noMoreMessagesPacket() []byte { return []byte{10} }

func msgWaitingPacket() []byte { return []byte{0x83} }

func okPacket() []byte { return []byte{0} }

func traceDataPacket(tag uint32, hashes []byte, snrs []int8) []byte {
	// [code][flags][path_len][rsvd][tag(4)][auth(4)][hashes…][snrs…]
	p := []byte{0x89, 0, byte(len(hashes)), 0}
	p = append(p, le32(tag)...)
	p = append(p, le32(0)...) // auth
	p = append(p, hashes...)
	for _, s := range snrs {
		p = append(p, byte(s))
	}
	return p
}

func TestClientTrace(t *testing.T) {
	client, ft := newConnectedClient(t)
	defer client.Close()

	go func() {
		// Target "25" parses directly as a hash path, so no contact lookup.
		raw := <-ft.WrittenPackets // SendTracePath
		tag := binary.LittleEndian.Uint32(raw[1:5])
		ft.ReadPackets <- okPacket()
		ft.ReadPackets <- traceDataPacket(tag, []byte{0x25}, []int8{48, 46})
	}()

	trace, err := client.Trace(context.Background(), "25")
	if err != nil {
		t.Fatal(err)
	}
	if len(trace.Path) != 1 || trace.Path[0] != 0x25 {
		t.Fatalf("path = %x", trace.Path)
	}
	if len(trace.SNRs) != 2 || trace.SNRs[0] != 12 {
		t.Errorf("snrs = %v", trace.SNRs)
	}
}

func TestClientContacts(t *testing.T) {
	client, ft := newConnectedClient(t)
	defer client.Close()

	go func() {
		<-ft.WrittenPackets // GetContacts
		ft.ReadPackets <- contactsStartPacket(2)
		ft.ReadPackets <- contactPacket("liba.meshcore.cz", bytes.Repeat([]byte{0xbb}, 32), 2)
		ft.ReadPackets <- contactPacket("JB/mobile", bytes.Repeat([]byte{0xf8}, 32), 1)
		ft.ReadPackets <- endOfContactsPacket()
	}()

	contacts, err := client.Contacts(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(contacts) != 2 {
		t.Fatalf("got %d contacts, want 2", len(contacts))
	}
	if contacts[0].Name != "liba.meshcore.cz" || contacts[0].Type != meshcore.ContactRepeater {
		t.Errorf("contact 0 = %+v", contacts[0])
	}
	if contacts[1].Type != meshcore.ContactChat {
		t.Errorf("contact 1 type = %v, want chat", contacts[1].Type)
	}
}

func TestClientChannelByIndex(t *testing.T) {
	client, ft := newConnectedClient(t)
	defer client.Close()

	go func() {
		<-ft.WrittenPackets // GetChannel 0
		ft.ReadPackets <- channelInfoPacket(0, "Public")
	}()

	ch, err := client.Channel(context.Background(), "0")
	if err != nil {
		t.Fatal(err)
	}
	if ch.Index != 0 || ch.Name != "Public" {
		t.Errorf("channel = %+v", ch)
	}
}

func TestClientSyncMessages(t *testing.T) {
	client, ft := newConnectedClient(t)
	defer client.Close()

	ts := time.Unix(1717675200, 0)
	go func() {
		<-ft.WrittenPackets // sync 1
		ft.ReadPackets <- contactMessagePacket(bytes.Repeat([]byte{0xbb}, 6), "hello", ts)
		<-ft.WrittenPackets // sync 2
		ft.ReadPackets <- noMoreMessagesPacket()
	}()

	msgs, err := client.SyncMessages(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 || msgs[0].Text != "hello" {
		t.Fatalf("messages = %+v", msgs)
	}
}

func TestClientSendTextAndAck(t *testing.T) {
	client, ft := newConnectedClient(t)
	defer client.Close()

	ackCode := uint32(0xdeadbeef)
	go func() {
		// SendText first fetches contacts to resolve the recipient.
		<-ft.WrittenPackets // GetContacts
		ft.ReadPackets <- contactsStartPacket(1)
		ft.ReadPackets <- contactPacket("alice", bytes.Repeat([]byte{0x11}, 32), 1)
		ft.ReadPackets <- endOfContactsPacket()
		<-ft.WrittenPackets // SendTextMessage
		ft.ReadPackets <- sentPacket(ackCode, 3000)
		// Acknowledgement push arrives shortly after.
		ft.ReadPackets <- sendConfirmedPacket(ackCode)
	}()

	receipt, err := client.SendText(context.Background(), "alice", "hi there")
	if err != nil {
		t.Fatal(err)
	}
	if receipt.AckCode != ackCode {
		t.Errorf("ack code = %08x, want %08x", receipt.AckCode, ackCode)
	}

	ack, err := client.WaitForAcknowledgement(context.Background(), receipt)
	if err != nil {
		t.Fatal(err)
	}
	if ack.Code != "deadbeef" {
		t.Errorf("ack = %q", ack.Code)
	}
}

func TestClientAutoSyncEmitsMessages(t *testing.T) {
	ft := testutil.NewFakeTransport(32)
	key := bytes.Repeat([]byte{0xab}, 32)
	ft.ReadPackets <- selfInfoPacket("MeshCore-desk", key)
	ft.ReadPackets <- deviceInfoPacket("Heltec V3", "19-Apr-2026", "v1.2.3")

	client := meshcore.New(ft, meshcore.WithTimeout(2*time.Second), meshcore.WithMessageSync())
	if err := client.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	<-ft.WrittenPackets // APP_START
	<-ft.WrittenPackets // DEVICE_QUERY

	ts := time.Unix(1717675200, 0)
	go func() {
		// Respond to the sync loop triggered by MSG_WAITING.
		<-ft.WrittenPackets // SyncNextMessage
		ft.ReadPackets <- contactMessagePacket(bytes.Repeat([]byte{0xcc}, 6), "ping", ts)
		<-ft.WrittenPackets // SyncNextMessage
		ft.ReadPackets <- noMoreMessagesPacket()
	}()

	// Inject the MSG_WAITING push that drives auto-sync.
	ft.ReadPackets <- msgWaitingPacket()

	select {
	case ev := <-client.Events():
		m, ok := ev.(meshcore.MessageReceived)
		if !ok {
			t.Fatalf("event type %T, want MessageReceived", ev)
		}
		if m.Text != "ping" {
			t.Errorf("text = %q", m.Text)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no message event from auto-sync")
	}
}
