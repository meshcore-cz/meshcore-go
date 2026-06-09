// Command send-channel-msg sends one text message to a public (hashtag) channel
// via the local backend daemon using Client.SendMeshPacket (CMD_SEND_RAW_PACKET).
//
// A fresh Curve25519 keypair and a fake identity are generated on each run.
//
// Edit the variables below, then run:
//
//	go run ./examples/send-channel-msg
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/brianvoe/gofakeit/v6"
	meshcore "github.com/meshcore-cz/meshcore-go"
	"github.com/meshcore-cz/meshcore-go/backend"
	"github.com/meshcore-cz/meshpkt"
)

var (
	publicChannel = "#test"
	backendDevice = "" // leave empty to use the daemon's default device
)

func main() {
	gofakeit.Seed(0) // 0 = random seed each run

	kp, err := meshpkt.Generate()
	if err != nil {
		log.Fatalf("generate key: %v", err)
	}

	name := gofakeit.Name()
	age := gofakeit.Number(18, 80)
	animal := gofakeit.Animal()

	message := fmt.Sprintf(
		"Ahoj! Jsem %s (%s), je mi %d let a mám doma %s.",
		name, kp.PublicKey[:12], age, animal,
	)

	fmt.Println("=== generated identity ===")
	fmt.Printf("public key:  %s\n", kp.PublicKey)
	fmt.Printf("private key: %s\n", kp.PrivateKey)
	fmt.Printf("name:        %s\n", name)
	fmt.Println("==========================")
	fmt.Printf("message: %s\n\n", message)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	bc := backend.NewClientForDevice("", backendDevice)

	secret := meshcore.DeriveChannelSecret(publicChannel)
	fmt.Printf("channel: %s (hash %02x)\n", publicChannel, meshcore.ChannelHash(secret))

	pkt, err := meshpkt.GroupTextPacket(secret, name, message, time.Now(), meshpkt.WithPathHashSize(2))
	if err != nil {
		log.Fatalf("build packet: %v", err)
	}

	if err := bc.SendMeshPacket(ctx, 0, pkt); err != nil {
		log.Fatalf("send: %v", err)
	}

	fmt.Printf("sent: [%s] %s: %s\n", publicChannel, name, message)
}
