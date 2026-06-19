package tcp

import (
	"bufio"
	"bytes"
	"context"
	"net"
	"testing"

	"github.com/meshcore-cz/meshcore-go/transport/internal/streamframe"
)

func TestConnReadWritePacket(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	conn := &Conn{
		uri:  "tcp://test",
		addr: "test",
		conn: client,
		r:    bufio.NewReader(client),
	}

	writeErr := make(chan error, 1)
	go func() {
		writeErr <- streamframe.Write(server, streamframe.ToHost, []byte{0x01, 0x02})
	}()
	got, err := conn.ReadPacket(context.Background())
	if err != nil {
		t.Fatalf("ReadPacket: %v", err)
	}
	if !bytes.Equal(got, []byte{0x01, 0x02}) {
		t.Fatalf("ReadPacket = %x, want 0102", got)
	}
	if err := <-writeErr; err != nil {
		t.Fatalf("server write: %v", err)
	}

	readErr := make(chan error, 1)
	readPacket := make(chan []byte, 1)
	go func() {
		pkt, err := streamframe.Read(bufio.NewReader(server), streamframe.ToDevice)
		if err != nil {
			readErr <- err
			return
		}
		readPacket <- pkt
	}()
	if err := conn.WritePacket(context.Background(), []byte{0x03, 0x04}); err != nil {
		t.Fatalf("WritePacket: %v", err)
	}
	select {
	case err := <-readErr:
		t.Fatalf("server read: %v", err)
	case pkt := <-readPacket:
		if !bytes.Equal(pkt, []byte{0x03, 0x04}) {
			t.Fatalf("server got %x, want 0304", pkt)
		}
	}
}
