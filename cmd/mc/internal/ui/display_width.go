package ui

import "github.com/clipperhouse/displaywidth"

// DisplayWidth returns the terminal column width of s.
func DisplayWidth(s string) int {
	return displaywidth.String(s)
}
