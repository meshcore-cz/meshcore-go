package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	meshcore "github.com/meshcore-dev/meshcore-go"
)

func cmdShell(ctx context.Context, e *env) error {
	backend, err := openShellBackend(ctx, e)
	if err != nil {
		return err
	}
	defer backend.Close()

	e.out.Human("Connected to %s. Type help for commands, exit to quit.\n", backend.URI())
	return runShell(ctx, e, backend, os.Stdin)
}

func openShellBackend(ctx context.Context, e *env) (Backend, error) {
	if !e.args.has("direct") {
		b, err := openIPCBackend(ctx)
		if err == nil {
			return b, nil
		}
		if errors.Is(err, errBackendDegraded) {
			return nil, err
		}
	}
	uri, _, err := resolveURI(e)
	if err != nil {
		return nil, err
	}
	opts := append(dialOptions(e), meshcore.WithClientOptions(meshcore.WithMessageSync()))
	client, err := meshcore.Dial(ctx, uri, opts...)
	if err != nil {
		return nil, fmt.Errorf("connecting to %s: %w", uri, err)
	}
	return newDirectBackend(uri, client), nil
}

func runShell(ctx context.Context, e *env, backend Backend, in io.Reader) error {
	scanner := bufio.NewScanner(in)
	for {
		e.out.Human("mcr> ")
		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				return err
			}
			e.out.Human("\n")
			return nil
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		fields, err := splitShellFields(line)
		if err != nil {
			fmt.Fprintln(os.Stderr, "mcr:", err)
			continue
		}
		if len(fields) == 0 {
			continue
		}
		if shellShouldExit(fields[0]) {
			return nil
		}
		if err := runShellCommand(ctx, e, backend, fields); err != nil {
			fmt.Fprintln(os.Stderr, "mcr:", err)
		}
	}
}

func shellShouldExit(cmd string) bool {
	switch cmd {
	case "exit", "quit":
		return true
	default:
		return false
	}
}

func runShellCommand(ctx context.Context, parent *env, backend Backend, fields []string) error {
	pa, err := parseArgs(fields)
	if err != nil {
		return err
	}
	inheritShellFlags(&pa, parent.args)
	cmd := pa.arg(0)
	e := &env{args: pa, rest: pa.positionals[1:], out: parent.out}

	switch cmd {
	case "help", "?":
		printShellHelp(e)
	case "status":
		return shellStatus(ctx, e, backend)
	case "contacts":
		return shellContacts(ctx, e, backend)
	case "contact":
		return shellContact(ctx, e, backend)
	case "inbox":
		return shellInbox(ctx, e, backend)
	case "send":
		return shellSend(ctx, e, backend)
	case "trace":
		return shellTrace(ctx, e, backend)
	case "channel":
		return shellChannel(ctx, e, backend)
	case "watch":
		return shellWatch(ctx, e, backend)
	default:
		return fmt.Errorf("unknown shell command %q", cmd)
	}
	return nil
}

func inheritShellFlags(child *parsedArgs, parent parsedArgs) {
	for _, name := range []string{"debug", "json"} {
		if parent.has(name) {
			child.bools[name] = true
		}
	}
	for _, name := range []string{"uri", "device"} {
		if child.flag(name) == "" && parent.flag(name) != "" {
			child.flags[name] = parent.flag(name)
		}
	}
}

func printShellHelp(e *env) {
	e.out.Human(`Commands:
  status
  contacts
  contact show <name>
  inbox
  send <recipient> <text> [--wait]
  trace <target>
  channel list
  channel show <name|index>
  channel send <name|index> <text>
  watch
  exit
`)
}

func shellStatus(ctx context.Context, e *env, backend Backend) error {
	info, err := backend.DeviceInfo(ctx)
	if err != nil {
		return err
	}
	if e.out.JSON {
		return e.out.JSONValue(map[string]any{
			"name":             info.Name,
			"firmware":         info.FirmwareName,
			"firmware_version": info.FirmwareVersion,
			"protocol":         info.ProtocolVersion,
			"transport":        backend.Transport(),
			"public_key":       info.PublicKey,
			"capabilities":     info.Capabilities.List(),
		})
	}
	e.out.Human("Device:       %s\n", orDash(info.Name))
	e.out.Human("Firmware:     %s %s\n", info.FirmwareName, info.FirmwareVersion)
	e.out.Human("Protocol:     %s\n", orDash(info.ProtocolVersion))
	e.out.Human("Transport:    %s\n", backend.URI())
	e.out.Human("Public key:   %s\n", shortKey(info.PublicKey))
	e.out.Human("Capabilities: %s\n", info.Capabilities.String())
	return nil
}

func shellContacts(ctx context.Context, e *env, backend Backend) error {
	contacts, err := backend.Contacts(ctx)
	if err != nil {
		return err
	}
	if e.out.JSON {
		return e.out.JSONValue(contacts)
	}
	if len(contacts) == 0 {
		e.out.Human("No contacts.\n")
		return nil
	}
	e.out.Human("%-26s %-9s %-5s %s\n", "NAME", "TYPE", "PATH", "PUBLIC KEY")
	for _, ct := range contacts {
		path := "-"
		if ct.HasPath {
			path = "yes"
		}
		e.out.Human("%-26s %-9s %-5s %s\n", ct.Name, ct.Type, path, shortKey(ct.PublicKey))
	}
	return nil
}

func shellContact(ctx context.Context, e *env, backend Backend) error {
	if e.restArg(0) != "show" {
		return fmt.Errorf("usage: contact show <name>")
	}
	name := e.restArg(1)
	if name == "" {
		return fmt.Errorf("usage: contact show <name>")
	}
	ct, err := backend.Contact(ctx, name)
	if err != nil {
		return err
	}
	if e.out.JSON {
		return e.out.JSONValue(ct)
	}
	e.out.Human("Name:       %s\n", ct.Name)
	e.out.Human("Type:       %s\n", ct.Type)
	e.out.Human("Public key: %s\n", ct.PublicKey)
	e.out.Human("Has path:   %t\n", ct.HasPath)
	if !ct.LastAdvert.IsZero() {
		e.out.Human("Last advert: %s\n", ct.LastAdvert.Format("2006-01-02 15:04:05"))
	}
	if ct.Latitude != 0 || ct.Longitude != 0 {
		e.out.Human("Location:   %.6f, %.6f\n", ct.Latitude, ct.Longitude)
	}
	return nil
}

func shellInbox(ctx context.Context, e *env, backend Backend) error {
	msgs, err := backend.Inbox(ctx)
	if err != nil {
		return err
	}
	if e.out.JSON {
		return e.out.JSONValue(msgs)
	}
	if len(msgs) == 0 {
		e.out.Human("No new messages.\n")
		return nil
	}
	names := backendContactNames(ctx, backend)
	for _, m := range msgs {
		ts := m.Timestamp.Format("15:04:05")
		e.out.Human("[%s] %s: %s\n", ts, resolveName(names, m.From), m.Text)
	}
	return nil
}

func shellSend(ctx context.Context, e *env, backend Backend) error {
	recipient := e.restArg(0)
	text := e.restArg(1)
	if recipient == "" || text == "" {
		return fmt.Errorf("usage: send <recipient> <text> [--wait]")
	}
	receipt, err := backend.SendText(ctx, recipient, text)
	if err != nil {
		return err
	}
	e.out.Human("Queued message %s to %s.\n", receipt.ID(), receipt.To)
	if !e.args.has("wait") {
		return e.out.JSONValue(map[string]any{"id": receipt.ID(), "to": receipt.To})
	}
	ack, err := backend.WaitForAcknowledgement(ctx, receipt)
	if err != nil {
		return fmt.Errorf("waiting for acknowledgement: %w", err)
	}
	e.out.Human("Acknowledged after %s.\n", ack.RTT.Round(1e6))
	return e.out.JSONValue(map[string]any{"id": receipt.ID(), "to": receipt.To, "rtt_ms": ack.RTT.Milliseconds()})
}

func shellTrace(ctx context.Context, e *env, backend Backend) error {
	target := e.restArg(0)
	if target == "" {
		return fmt.Errorf("usage: trace <target>")
	}
	trace, err := backend.Trace(ctx, target)
	if err != nil {
		return err
	}
	if e.out.JSON {
		path := make([]string, len(trace.Path))
		for i, h := range trace.Path {
			path[i] = fmt.Sprintf("%02x", h)
		}
		return e.out.JSONValue(map[string]any{
			"target":        trace.Target,
			"tag":           fmt.Sprintf("%08x", trace.Tag),
			"path":          path,
			"snr_db":        trace.SNRs,
			"round_trip_ms": trace.RoundTrip.Milliseconds(),
		})
	}
	printTrace(e, trace)
	return nil
}

func shellChannel(ctx context.Context, e *env, backend Backend) error {
	switch e.restArg(0) {
	case "", "list":
		return shellChannelList(ctx, e, backend)
	case "show":
		return shellChannelShow(ctx, e, backend)
	case "send":
		return shellChannelSend(ctx, e, backend)
	default:
		return fmt.Errorf("unknown channel subcommand %q", e.restArg(0))
	}
}

func shellChannelList(ctx context.Context, e *env, backend Backend) error {
	channels, err := backend.Channels(ctx)
	if err != nil {
		return err
	}
	if e.out.JSON {
		return e.out.JSONValue(channels)
	}
	if len(channels) == 0 {
		e.out.Human("No channels.\n")
		return nil
	}
	e.out.Human("%-6s %s\n", "INDEX", "NAME")
	for _, ch := range channels {
		e.out.Human("%-6d %s\n", ch.Index, ch.Name)
	}
	return nil
}

func shellChannelShow(ctx context.Context, e *env, backend Backend) error {
	name := e.restArg(1)
	if name == "" {
		return fmt.Errorf("usage: channel show <name|index>")
	}
	ch, err := backend.Channel(ctx, name)
	if err != nil {
		return err
	}
	if e.out.JSON {
		return e.out.JSONValue(ch)
	}
	e.out.Human("Index: %d\n", ch.Index)
	e.out.Human("Name:  %s\n", ch.Name)
	return nil
}

func shellChannelSend(ctx context.Context, e *env, backend Backend) error {
	channel := e.restArg(1)
	text := e.restArg(2)
	if channel == "" || text == "" {
		return fmt.Errorf("usage: channel send <name|index> <text>")
	}
	receipt, err := backend.SendChannelText(ctx, channel, text)
	if err != nil {
		return err
	}
	e.out.Human("Sent to %s.\n", receipt.To)
	return e.out.JSONValue(map[string]any{"to": receipt.To, "id": receipt.ID()})
}

func shellWatch(ctx context.Context, e *env, backend Backend) error {
	if b, ok := backend.(*ipcBackend); ok {
		events, err := b.client.Watch(ctx)
		if err != nil {
			return err
		}
		e.out.Human("Watching backend %s. Press Ctrl-C to leave the shell.\n", backend.URI())
		for ev := range events {
			printBackendEvent(e, ev)
		}
		return nil
	}

	e.out.Human("Watching %s. Press Ctrl-C to leave the shell.\n", backend.URI())
	names := backendContactNames(ctx, backend)
	for {
		select {
		case <-ctx.Done():
			return nil
		case ev, ok := <-backend.Events():
			if !ok {
				return nil
			}
			printEvent(e, ev, names)
		}
	}
}

func splitShellFields(s string) ([]string, error) {
	var fields []string
	var b strings.Builder
	var quote rune
	escaped := false
	inField := false

	for _, r := range s {
		switch {
		case escaped:
			b.WriteRune(r)
			escaped = false
			inField = true
		case r == '\\':
			escaped = true
			inField = true
		case quote != 0:
			if r == quote {
				quote = 0
				continue
			}
			b.WriteRune(r)
		case r == '\'' || r == '"':
			quote = r
			inField = true
		case r == ' ' || r == '\t':
			if inField {
				fields = append(fields, b.String())
				b.Reset()
				inField = false
			}
		default:
			b.WriteRune(r)
			inField = true
		}
	}
	if escaped {
		return nil, errors.New("unfinished escape")
	}
	if quote != 0 {
		return nil, errors.New("unterminated quote")
	}
	if inField {
		fields = append(fields, b.String())
	}
	return fields, nil
}
