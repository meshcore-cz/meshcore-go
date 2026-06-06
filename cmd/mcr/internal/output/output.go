// Package output renders CLI results as either human-readable text or JSON.
package output

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
)

// Printer renders results to stdout in the selected format. Errors and
// diagnostics belong on stderr, which callers write to directly.
type Printer struct {
	JSON bool
	Out  io.Writer
}

// New returns a Printer writing to stdout.
func New(jsonMode bool) *Printer {
	return &Printer{JSON: jsonMode, Out: os.Stdout}
}

// Human prints text only in human mode.
func (p *Printer) Human(format string, args ...any) {
	if p.JSON {
		return
	}
	fmt.Fprintf(p.Out, format, args...)
}

// JSONValue prints v as indented JSON only in JSON mode.
func (p *Printer) JSONValue(v any) error {
	if !p.JSON {
		return nil
	}
	enc := json.NewEncoder(p.Out)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// Line writes a newline-delimited JSON object (for streaming commands).
func (p *Printer) Line(v any) error {
	enc := json.NewEncoder(p.Out)
	return enc.Encode(v)
}
