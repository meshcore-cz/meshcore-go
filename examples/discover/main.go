// Command discover lists candidate companion radio endpoints.
//
//	go run ./examples/discover
package main

import (
	"context"
	"fmt"
	"log"

	meshcore "github.com/meshcore-cz/meshcore-go"
)

func main() {
	ctx := context.Background()
	endpoints, err := meshcore.Discover(
		ctx,
		meshcore.WithSerialDiscovery(),
		meshcore.WithBLEDiscovery(),
	)
	if err != nil {
		log.Fatal(err)
	}

	if len(endpoints) == 0 {
		fmt.Println("No companion radios found.")
		return
	}
	for _, ep := range endpoints {
		fmt.Printf("%-6s %-30s %s\n", ep.Transport, ep.URI, ep.Name)
	}
}
