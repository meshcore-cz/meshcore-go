// Command device-info connects to a companion radio and prints its identity.
//
//	go run ./examples/device-info serial:///dev/ttyACM0
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	meshcore "github.com/meshcore-cz/meshcore-go"
)

func main() {
	uri := "serial:///dev/ttyACM0"
	if len(os.Args) > 1 {
		uri = os.Args[1]
	}

	ctx := context.Background()
	client, err := meshcore.Dial(ctx, uri)
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	info, err := client.DeviceInfo(ctx)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Name:         %s\n", info.Name)
	fmt.Printf("Public key:   %s\n", info.PublicKey)
	fmt.Printf("Firmware:     %s %s\n", info.FirmwareName, info.FirmwareVersion)
	fmt.Printf("Protocol:     %s\n", info.ProtocolVersion)
	fmt.Printf("Capabilities: %s\n", info.Capabilities)
}
