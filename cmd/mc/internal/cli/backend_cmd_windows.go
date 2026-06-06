//go:build windows

package cli

import (
	"fmt"
	"os/exec"
)

func startBackground(cmd *exec.Cmd) error {
	return fmt.Errorf("background backend is not implemented on Windows")
}
