package ui

import "testing"

func TestDisplayWidthEmoji(t *testing.T) {
	tests := []struct {
		name string
		s    string
		want int
	}{
		{"plain", "OK2USB_RPT_Brno", 15},
		{"biohazard vs16", "OK5TVR☣️", 8},
		{"sun vs16", "OKROU.SOLAR☀️", 13},
		{"multi emoji", "OVA-FUTURUM 🗼🔋☀️", 18},
		{"accents and emoji", "Ovenecká ☀️🔋🗼", 15},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := DisplayWidth(tc.s); got != tc.want {
				t.Fatalf("DisplayWidth(%q) = %d, want %d", tc.s, got, tc.want)
			}
		})
	}
}
