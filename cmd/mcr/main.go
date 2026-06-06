// Command mcr is the reference terminal client for meshcore-go.
package main

import (
	"os"

	"github.com/meshcore-dev/meshcore-go/cmd/mcr/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:]))
}
