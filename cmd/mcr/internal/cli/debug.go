package cli

import (
	"fmt"
	"io"
	"log/slog"
	"os"

	localbackend "github.com/meshcore-dev/meshcore-go/backend"
	meshcore "github.com/meshcore-dev/meshcore-go"
)

var discardLogger = slog.New(slog.NewTextHandler(io.Discard, nil))

// Debug is the CLI's unified --debug interface. When debug mode is off, calls
// are discarded so commands can log unconditionally.
type Debug struct {
	enabled bool
	log     *slog.Logger
}

func newDebug(pa parsedArgs) Debug {
	if pa.has("debug") {
		return Debug{
			enabled: true,
			log: slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
				Level: slog.LevelDebug,
			})),
		}
	}
	return Debug{log: discardLogger}
}

// Enabled reports whether --debug was set.
func (d Debug) Enabled() bool { return d.enabled }

// Log writes a debug message to stderr when --debug is set.
func (d Debug) Log(msg string, args ...any) {
	d.log.Debug(msg, args...)
}

// DialOptions returns meshcore dial options that forward protocol debug output
// to stderr when --debug is set.
func (d Debug) DialOptions() []meshcore.DialOption {
	if !d.enabled {
		return nil
	}
	return []meshcore.DialOption{meshcore.WithClientOptions(meshcore.WithLogger(d.log))}
}

// Backend logs the opened backend endpoint.
func (d Debug) Backend(mode string, b Backend) {
	d.Log("backend opened", "mode", mode, "uri", b.URI(), "transport", b.Transport())
}

// Contact logs a resolved device contact.
func (d Debug) Contact(ct meshcore.Contact) {
	d.Log("contact resolved",
		"name", ct.Name,
		"type", ct.Type,
		"public_key", shortKey(ct.PublicKey),
		"has_path", ct.HasPath,
	)
}

// RawSend logs an outbound raw companion frame.
func (d Debug) RawSend(payload []byte) {
	args := []any{"bytes", len(payload), "hex", hexLine(payload)}
	if len(payload) > 0 {
		args = append(args, "cmd", fmt.Sprintf("0x%02x", payload[0]))
	}
	d.Log("raw send", args...)
}

// RawResult logs a decoded raw response.
func (d Debug) RawResult(result localbackend.RawResult) {
	if result.Type == "raw" {
		frame := append([]byte{result.Code}, result.Payload...)
		d.Log("raw response",
			"type", "raw",
			"code", fmt.Sprintf("0x%02x", result.Code),
			"push", result.Push,
			"bytes", len(frame),
			"hex", hexLine(frame),
		)
		return
	}
	d.Log("raw response", "type", result.Type, "decoded", result.Decoded)
}
