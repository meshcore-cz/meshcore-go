package cli

import (
	"context"
	"encoding/hex"
	"fmt"
	"strings"

	localbackend "github.com/meshcore-dev/meshcore-go/backend"
)

// cmdRaw implements `mcr raw <hex bytes...>`.
func cmdRaw(ctx context.Context, e *env) error {
	payload, err := parseHexArgs(e.rest)
	if err != nil {
		return err
	}
	if len(payload) == 0 {
		return fmt.Errorf("usage: mcr raw <hex bytes>")
	}

	backend, err := openBackend(ctx, e)
	if err != nil {
		return err
	}
	defer backend.Close()

	e.out.Human("→ %d bytes: %s\n\n", len(payload), hexLine(payload))

	result, err := backend.RawSend(ctx, payload)
	if err != nil {
		return err
	}

	if e.out.JSON {
		return e.out.JSONValue(rawMsgJSON(result))
	}
	printRawMsg(e, result)
	return nil
}

// parseHexArgs joins and decodes space- or comma-separated hex tokens.
// Each token may be a full byte string ("0103") or a single byte ("01" "03").
// A leading "0x" prefix is stripped.
func parseHexArgs(args []string) ([]byte, error) {
	var out []byte
	for _, a := range args {
		a = strings.TrimPrefix(a, "0x")
		a = strings.TrimPrefix(a, "0X")
		a = strings.ReplaceAll(a, ",", "")
		a = strings.ReplaceAll(a, " ", "")
		if len(a) == 0 {
			continue
		}
		if len(a)%2 != 0 {
			a = "0" + a // treat "3" as "03"
		}
		b, err := hex.DecodeString(a)
		if err != nil {
			return nil, fmt.Errorf("invalid hex %q: %w", a, err)
		}
		out = append(out, b...)
	}
	return out, nil
}

// hexLine formats bytes as "aa bb cc …".
func hexLine(b []byte) string {
	parts := make([]string, len(b))
	for i, v := range b {
		parts[i] = fmt.Sprintf("%02x", v)
	}
	return strings.Join(parts, " ")
}

// hexDump prints an xxd-style hex+ASCII dump indented by a prefix.
func hexDump(e *env, b []byte, indent string) {
	for off := 0; off < len(b); off += 16 {
		end := off + 16
		if end > len(b) {
			end = len(b)
		}
		row := b[off:end]
		hexCols := make([]string, 16)
		ascii := make([]byte, 16)
		for i := range ascii {
			ascii[i] = ' '
		}
		for i, v := range row {
			hexCols[i] = fmt.Sprintf("%02x", v)
			if v >= 0x20 && v < 0x7f {
				ascii[i] = v
			} else {
				ascii[i] = '.'
			}
		}
		// Blank out unused columns.
		for i := len(row); i < 16; i++ {
			hexCols[i] = "  "
		}
		e.out.Human("%s%08x  %s %s  |%s|\n", indent, off,
			strings.Join(hexCols[:8], " "),
			strings.Join(hexCols[8:], " "),
			ascii[:len(row)])
	}
}

func printRawMsg(e *env, msg localbackend.RawResult) {
	if msg.Type == "raw" {
		e.out.Human("← code=0x%02x  push=%t  %d bytes\n", msg.Code, msg.Push, len(msg.Payload))
		if len(msg.Payload) > 0 {
			hexDump(e, append([]byte{msg.Code}, msg.Payload...), "   ")
		}
		return
	}
	e.out.Human("← %s\n   %s\n", msg.Type, msg.Decoded)
}

func rawMsgJSON(msg localbackend.RawResult) map[string]any {
	if msg.Type == "raw" {
		return map[string]any{
			"type":    fmt.Sprintf("0x%02x", msg.Code),
			"push":    msg.Push,
			"payload": hexLine(msg.Payload),
		}
	}
	return map[string]any{
		"type":    msg.Type,
		"decoded": msg.Decoded,
	}
}
