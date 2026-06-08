// Command rf-watch connects to a BLE companion radio and prints raw RF packets.
//
//	go run ./examples/rf-watch ble://90d56c84-42ef-36f3-89ae-9e8f42231b00
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"

	meshcore "github.com/meshcore-cz/meshcore-go"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintf(os.Stderr, "usage: go run ./examples/rf-watch ble://<device-id-or-address>\n")
		os.Exit(2)
	}
	uri := os.Args[1]

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	client, err := meshcore.Dial(ctx, uri)
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-client.Events():
			if !ok {
				return
			}
			rf, ok := ev.(meshcore.RFPacketReceived)
			if !ok {
				continue
			}
			fmt.Printf("%s snr=%+.1f rssi=%d bytes=%s\n",
				rf.Timestamp.Format("2006-01-02T15:04:05.000Z07:00"),
				rf.SNR,
				rf.RSSI,
				hexLine(rf.Bytes))
		}
	}
}

func hexLine(b []byte) string {
	parts := make([]string, len(b))
	for i, v := range b {
		parts[i] = fmt.Sprintf("%02x", v)
	}
	return strings.Join(parts, " ")
}
