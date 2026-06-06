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

// packet builders matching the companion wire layout.

func selfInfoPacket(name string, key []byte) []byte {
	var b bytes.Buffer
	b.WriteByte(5) // respSelfInfo
	b.Write([]byte{1, 22, 30})
	b.Write(key)                 // 32-byte public key
	b.Write(make([]byte, 4+4+4)) // lat, lon, reserved
	b.Write(le32(868000))        // freq
	b.Write(le32(250))           // bw
	b.Write([]byte{11, 5})       // sf, cr
	b.WriteString(name)
	return b.Bytes()
}

func deviceInfoPacket(model, date, version string) []byte {
	var b bytes.Buffer
	b.WriteByte(13)                      // respDeviceInfo
	b.Write([]byte{0x0b, 0xaf, 0x28, 0}) // binary header
	b.Write([]byte{0, 0, 0})             // padding
	b.WriteString(date)
	b.WriteByte(0)
	b.WriteString(model)
	b.WriteByte(0)
	b.WriteString(version)
	b.WriteByte(0)
	return b.Bytes()
}

func currentTimePacket(t time.Time) []byte {
	return append([]byte{9}, le32(uint32(t.Unix()))...)
}

func advertPacket(name string, key []byte) []byte {
	pkt := append([]byte{0x80}, key...)
	return append(pkt, []byte(name)...)
}

func le16(v uint16) []byte { b := make([]byte, 2); binary.LittleEndian.PutUint16(b, v); return b }
func le32(v uint32) []byte { b := make([]byte, 4); binary.LittleEndian.PutUint32(b, v); return b }

// newConnectedClient wires a client to a fake transport, preloads the handshake
// responses and returns the connected client plus the transport for the test to
// drive.
func newConnectedClient(t *testing.T) (*meshcore.Client, *testutil.FakeTransport) {
	t.Helper()
	ft := testutil.NewFakeTransport(32)
	key := bytes.Repeat([]byte{0xab}, 32)

	// Handshake reads: SelfInfo then DeviceInfo.
	ft.ReadPackets <- selfInfoPacket("MeshCore-desk", key)
	ft.ReadPackets <- deviceInfoPacket("Heltec V3", "19-Apr-2026", "v1.2.3")

	client := meshcore.New(ft, meshcore.WithTimeout(2*time.Second))
	if err := client.Connect(context.Background()); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	// Drain the handshake writes (APP_START, DEVICE_QUERY).
	<-ft.WrittenPackets
	<-ft.WrittenPackets
	return client, ft
}

func TestClientHandshake(t *testing.T) {
	client, _ := newConnectedClient(t)
	defer client.Close()

	info, err := client.DeviceInfo(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if info.Name != "MeshCore-desk" {
		t.Errorf("name = %q", info.Name)
	}
	if info.FirmwareName != "MeshCore (Heltec V3)" {
		t.Errorf("firmware name = %q, want MeshCore (Heltec V3)", info.FirmwareName)
	}
	if info.FirmwareVersion != "v1.2.3" {
		t.Errorf("firmware version = %q", info.FirmwareVersion)
	}
	if info.ProtocolVersion != "companion-v3" {
		t.Errorf("protocol = %q", info.ProtocolVersion)
	}
	if !info.Capabilities.Has(meshcore.CapabilityMessages) {
		t.Error("expected messages capability")
	}
}

func TestClientRequestResponse(t *testing.T) {
	client, ft := newConnectedClient(t)
	defer client.Close()

	want := time.Unix(1717675200, 0)
	// Respond to the next written request with a CurrentTime packet.
	go func() {
		<-ft.WrittenPackets
		ft.ReadPackets <- currentTimePacket(want)
	}()

	got, err := client.DeviceTime(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !got.Equal(want) {
		t.Errorf("DeviceTime = %v, want %v", got, want)
	}
}

func TestClientRequestTimeout(t *testing.T) {
	client, _ := newConnectedClient(t)
	defer client.Close()

	// No responder: the request must time out.
	_, err := client.DeviceTime(context.Background())
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestClientEventDispatch(t *testing.T) {
	client, ft := newConnectedClient(t)
	defer client.Close()

	key := bytes.Repeat([]byte{0x11}, 32)
	ft.ReadPackets <- advertPacket("repeater-kololec", key)

	select {
	case ev := <-client.Events():
		adv, ok := ev.(meshcore.AdvertisementReceived)
		if !ok {
			t.Fatalf("event type %T, want AdvertisementReceived", ev)
		}
		if adv.Contact.Name != "repeater-kololec" {
			t.Errorf("advert name = %q", adv.Contact.Name)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no event received")
	}
}

func TestClientDisconnectEvent(t *testing.T) {
	client, ft := newConnectedClient(t)

	// Closing the transport ends the read loop and emits Disconnected.
	ft.Close()

	select {
	case ev := <-client.Events():
		if _, ok := ev.(meshcore.Disconnected); !ok {
			t.Fatalf("event type %T, want Disconnected", ev)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no disconnect event")
	}
	client.Close()
}
