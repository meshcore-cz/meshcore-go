package ui

import (
	"fmt"
	"strings"

	"rsc.io/qr"
)

// qrQuietZone is the white border, in QR modules, drawn around the symbol. The
// QR spec requires at least 4 modules of quiet zone for reliable scanning.
const qrQuietZone = 4

const (
	qrBlack = "0;0;0"
	qrWhite = "255;255;255"
)

// RenderQR encodes text as a QR code drawn with Unicode half-block characters,
// two QR rows per terminal line.
//
// When color is true it emits true-color ANSI so the symbol is painted as black
// modules on a white quiet zone; this scans regardless of the terminal's own
// color scheme. When color is false it falls back to a monochrome block
// rendering using the terminal's default colors.
func RenderQR(text string, color bool) (string, error) {
	code, err := qr.Encode(text, qr.M)
	if err != nil {
		return "", err
	}
	if color {
		return renderQRColor(code), nil
	}
	return renderQRMono(code), nil
}

// qrModule reports whether the module at (x, y) — measured in the padded grid
// that includes the quiet zone — is dark. Coordinates inside the quiet zone are
// always light.
func qrModule(code *qr.Code, x, y int) bool {
	x -= qrQuietZone
	y -= qrQuietZone
	if x < 0 || y < 0 || x >= code.Size || y >= code.Size {
		return false
	}
	return code.Black(x, y)
}

func renderQRColor(code *qr.Code) string {
	dim := code.Size + 2*qrQuietZone
	var b strings.Builder
	for y := 0; y < dim; y += 2 {
		for x := 0; x < dim; x++ {
			fg := qrWhite
			if qrModule(code, x, y) {
				fg = qrBlack
			}
			bg := qrWhite
			if qrModule(code, x, y+1) { // y+1 == dim on a final odd row -> light
				bg = qrBlack
			}
			// Upper-half block: foreground paints the top module, background the
			// bottom one.
			fmt.Fprintf(&b, "\x1b[38;2;%sm\x1b[48;2;%sm▀", fg, bg)
		}
		b.WriteString("\x1b[0m\n")
	}
	return b.String()
}

func renderQRMono(code *qr.Code) string {
	dim := code.Size + 2*qrQuietZone
	var b strings.Builder
	for y := 0; y < dim; y++ {
		for x := 0; x < dim; x++ {
			if qrModule(code, x, y) {
				b.WriteString("██") // two cells keep modules ~square
			} else {
				b.WriteString("  ")
			}
		}
		b.WriteByte('\n')
	}
	return b.String()
}
