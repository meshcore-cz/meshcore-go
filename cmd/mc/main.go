// Command mc is the reference terminal client for meshcore-go.
package main

import (
	"os"

	"github.com/meshcore-cz/meshcore-go/cmd/mc/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:]))
}
