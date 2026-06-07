package cli

import (
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"os"
	"time"

	meshcore "github.com/meshcore-cz/meshcore-go"
	localbackend "github.com/meshcore-cz/meshcore-go/backend"
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

func debugTimestamp(t time.Time) string {
	return t.Format("2006-01-02 15:04:05.000")
}

// Started logs the start of a command with a wall-clock timestamp.
func (d Debug) Started(op string, start time.Time, args ...any) {
	fields := []any{"op", op, "started_at", debugTimestamp(start)}
	d.Log("started", append(fields, args...)...)
}

// Phase logs progress within a command relative to its start time.
func (d Debug) Phase(op, phase string, start time.Time, args ...any) {
	fields := []any{"op", op, "phase", phase, "at", debugTimestamp(time.Now()), "elapsed", time.Since(start).Round(time.Millisecond)}
	d.Log("phase", append(fields, args...)...)
}

// SendCommand logs an outbound radio/backend operation when --debug is set.
func (d Debug) SendCommand(op string, wireCmd byte, args ...any) {
	fields := []any{"op", op, "at", debugTimestamp(time.Now())}
	if wireCmd != 0 {
		fields = append(fields, "wire_cmd", fmt.Sprintf("0x%02x", wireCmd))
	}
	d.Log("radio send", append(fields, args...)...)
}

// CommandDone logs completion of a radio/backend operation with elapsed time.
func (d Debug) CommandDone(op string, start time.Time, args ...any) {
	now := time.Now()
	fields := []any{
		"op", op,
		"started_at", debugTimestamp(start),
		"finished_at", debugTimestamp(now),
		"elapsed", now.Sub(start).Round(time.Millisecond),
	}
	d.Log("radio done", append(fields, args...)...)
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

// TracePlan logs how a trace target was resolved before sending.
func (d Debug) TracePlan(plan meshcore.TracePlan) {
	args := []any{
		"query", plan.Query,
		"resolved_as", plan.Name,
		"source", plan.Source,
		"path_hex", hex.EncodeToString(plan.Path),
		"path_bytes", len(plan.Path),
		"hop_count", plan.HopCount,
		"hash_size", plan.HashSize,
		"flags", fmt.Sprintf("0x%02x", plan.Flags),
	}
	if plan.Contact != "" {
		args = append(args, "contact", plan.Contact)
	}
	if plan.OutPathEnc != 0 && plan.OutPathEnc != 0xff {
		args = append(args, "out_path_enc", fmt.Sprintf("0x%02x", plan.OutPathEnc))
	}
	if plan.PrefixHint > 0 {
		args = append(args, "prefix_hint_bytes", plan.PrefixHint)
	}
	d.Log("trace plan", args...)
}

// TraceStarted logs the beginning of a trace operation.
func (d Debug) TraceStarted(target string, start time.Time) {
	d.Started("trace", start, "target", target)
}

// TraceDone logs trace completion or failure.
func (d Debug) TraceDone(start time.Time, trace meshcore.Trace, err error) {
	if err != nil {
		d.CommandDone("trace", start, "error", err)
		return
	}
	d.CommandDone("trace", start,
		"target", trace.Target,
		"tag", fmt.Sprintf("%08x", trace.Tag),
		"hop_count", traceHopCount(trace),
		"snr_count", len(trace.SNRs),
		"round_trip", trace.RoundTrip.Round(time.Millisecond),
	)
}

func traceHopCount(trace meshcore.Trace) int {
	size := trace.PathHashSize
	if size <= 0 {
		size = 1
	}
	return len(trace.Path) / size
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
