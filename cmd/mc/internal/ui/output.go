package ui

import (
	"fmt"
	"io"
)

// Printer writes CLI output.
type Printer struct {
	Out io.Writer
}

// NewPrinter builds a printer for the given writer.
func NewPrinter(out io.Writer) Printer {
	return Printer{Out: out}
}

// Print writes text to the configured output.
func (p Printer) Print(text string) {
	fmt.Fprint(p.Out, text) //nolint:errcheck
}
