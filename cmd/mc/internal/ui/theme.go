package ui

import (
	"io"
	"os"

	"golang.org/x/term"
)

// Health is a semantic status level for styled output.
type Health int

const (
	HealthUnknown Health = iota
	HealthOK
	HealthWarning
	HealthError
)

const (
	ansiReset      = "\x1b[0m"
	ansiDim        = "\x1b[2m"
	ansiBoldGreen  = "\x1b[1;32m"
	ansiBoldYellow = "\x1b[1;33m"
	ansiBoldRed    = "\x1b[1;31m"
)

// Theme provides minimal terminal styling for mc commands.
type Theme struct {
	enabled bool
}

// NewTheme builds a theme for the given output stream.
func NewTheme(out io.Writer) Theme {
	return Theme{enabled: colorEnabled(out)}
}

func colorEnabled(out io.Writer) bool {
	if _, ok := os.LookupEnv("NO_COLOR"); ok {
		return false
	}
	return IsTerminal(out)
}

// IsTerminal reports whether out is connected to a terminal (TTY).
func IsTerminal(out io.Writer) bool {
	f, ok := out.(*os.File)
	if !ok {
		return false
	}
	return term.IsTerminal(int(f.Fd()))
}

// ColorEnabled reports whether colored ANSI output is appropriate for out:
// it must be a terminal and NO_COLOR must be unset.
func ColorEnabled(out io.Writer) bool {
	return colorEnabled(out)
}

// StatusWord colors a single status token when color is enabled.
func (t Theme) StatusWord(health Health, word string) string {
	if !t.enabled {
		return word
	}
	switch health {
	case HealthOK:
		return ansiBoldGreen + word + ansiReset
	case HealthWarning:
		return ansiBoldYellow + word + ansiReset
	case HealthError:
		return ansiBoldRed + word + ansiReset
	default:
		return word
	}
}

// Dim renders text with reduced intensity when color is enabled.
func (t Theme) Dim(text string) string {
	if !t.enabled || text == "" {
		return text
	}
	return ansiDim + text + ansiReset
}
