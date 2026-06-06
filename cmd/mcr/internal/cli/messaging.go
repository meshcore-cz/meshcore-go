package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"

	meshcore "github.com/meshcore-dev/meshcore-go"
	localbackend "github.com/meshcore-dev/meshcore-go/backend"
)

// cmdContacts implements `mcr contacts`.
func cmdContacts(ctx context.Context, e *env) error {
	backend, err := openBackend(ctx, e)
	if err != nil {
		return err
	}
	defer backend.Close()

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

// cmdContact implements `mcr contact show <name>`.
func cmdContact(ctx context.Context, e *env) error {
	if e.restArg(0) != "show" {
		return fmt.Errorf("usage: mcr contact show <name>")
	}
	name := e.restArg(1)
	if name == "" {
		return fmt.Errorf("usage: mcr contact show <name>")
	}

	backend, err := openBackend(ctx, e)
	if err != nil {
		return err
	}
	defer backend.Close()

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

// cmdInbox implements `mcr inbox`: drain buffered messages.
func cmdInbox(ctx context.Context, e *env) error {
	backend, err := openBackend(ctx, e)
	if err != nil {
		return err
	}
	defer backend.Close()

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

// contactNames returns a best-effort map from 12-char key prefix to contact
// name, used to render sender names for received messages.
func contactNames(ctx context.Context, client *meshcore.Client) map[string]string {
	contacts, err := client.Contacts(ctx)
	if err != nil {
		return nil
	}
	names := make(map[string]string, len(contacts))
	for _, ct := range contacts {
		if len(ct.PublicKey) >= 12 {
			names[ct.PublicKey[:12]] = ct.Name
		}
	}
	return names
}

func backendContactNames(ctx context.Context, backend Backend) map[string]string {
	contacts, err := backend.Contacts(ctx)
	if err != nil {
		return nil
	}
	names := make(map[string]string, len(contacts))
	for _, ct := range contacts {
		if len(ct.PublicKey) >= 12 {
			names[ct.PublicKey[:12]] = ct.Name
		}
	}
	return names
}

// resolveName maps a sender key prefix to a contact name when known.
func resolveName(names map[string]string, from string) string {
	if n, ok := names[from]; ok {
		return n
	}
	return from
}

// cmdSend implements `mcr send <recipient> <text> [--wait]`.
func cmdSend(ctx context.Context, e *env) error {
	recipient := e.restArg(0)
	text := e.restArg(1)
	if recipient == "" || text == "" {
		return fmt.Errorf("usage: mcr send <recipient> <text> [--wait]")
	}

	backend, err := openBackend(ctx, e)
	if err != nil {
		return err
	}
	defer backend.Close()

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

// cmdTrace implements `mcr trace <target>`.
func cmdTrace(ctx context.Context, e *env) error {
	target := e.restArg(0)
	if target == "" {
		return fmt.Errorf("usage: mcr trace <target>")
	}

	backend, err := openBackend(ctx, e)
	if err != nil {
		return err
	}
	defer backend.Close()

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

func printTrace(e *env, trace meshcore.Trace) {
	e.out.Human("Trace to %s  ·  %s  ·  tag %08x\n",
		trace.Target, trace.RoundTrip.Round(1e6), trace.Tag)

	// Build the full node chain: you → path[0] → … → path[N-1] → you
	nodes := make([]string, len(trace.Path)+2)
	nodes[0] = "you"
	for i, h := range trace.Path {
		nodes[i+1] = fmt.Sprintf("%02x", h)
	}
	nodes[len(nodes)-1] = "you"

	if len(trace.SNRs) == 0 {
		e.out.Human("  no signal data\n")
		return
	}

	// Column width: longest node name (minimum 3 for "you").
	col := 3
	for _, n := range nodes {
		if len(n) > col {
			col = len(n)
		}
	}

	e.out.Human("\n")
	e.out.Human("  %-4s  %-*s    %-*s  %s\n", "hop", col, "from", col, "to", "SNR")
	e.out.Human("  %s\n", strings.Repeat("─", 4+2+col+4+col+2+12))

	for i, snr := range trace.SNRs {
		if i+1 >= len(nodes) {
			break
		}
		flag := ""
		if snr < 0 {
			flag = "  ← weak"
		}
		e.out.Human("  %-4d  %-*s  → %-*s  %+.2f dB%s\n",
			i+1, col, nodes[i], col, nodes[i+1], snr, flag)
	}
}

// cmdWatch implements `mcr watch`: stream asynchronous events.
func cmdWatch(ctx context.Context, e *env) error {
	if e.args.has("raw") {
		return cmdWatchRaw(ctx, e)
	}

	if !e.args.has("direct") {
		err := cmdWatchBackend(ctx, e)
		if err == nil {
			return nil
		}
		if errors.Is(err, errBackendDegraded) {
			return err
		}
	}

	uri, _, err := resolveURI(e)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(ctx, os.Interrupt)
	defer stop()

	opts := append(e.dbg.DialOptions(), meshcore.WithClientOptions(meshcore.WithMessageSync()))
	client, err := meshcore.Dial(ctx, uri, opts...)
	if err != nil {
		return fmt.Errorf("connecting to %s: %w", uri, err)
	}
	defer client.Close()

	names := contactNames(ctx, client)
	e.out.Human("Watching %s (Ctrl-C to stop)...\n", uri)
	for {
		select {
		case <-ctx.Done():
			return nil
		case ev, ok := <-client.Events():
			if !ok {
				return nil
			}
			printEvent(e, ev, names)
		}
	}
}

func cmdWatchRaw(ctx context.Context, e *env) error {
	e.out.JSON = true
	if !e.args.has("direct") {
		err := cmdWatchRawBackend(ctx, e)
		if err == nil {
			return nil
		}
		if errors.Is(err, errBackendDegraded) {
			return err
		}
	}

	uri, _, err := resolveURI(e)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(ctx, os.Interrupt)
	defer stop()

	client, err := meshcore.Dial(ctx, uri, e.dbg.DialOptions()...)
	if err != nil {
		return fmt.Errorf("connecting to %s: %w", uri, err)
	}
	defer client.Close()

	e.out.Human("Watching raw packets from %s (Ctrl-C to stop)...\n", uri)
	for {
		select {
		case <-ctx.Done():
			return nil
		case pkt, ok := <-client.RawPackets():
			if !ok {
				return nil
			}
			printRawPacket(e, pkt)
		}
	}
}

func cmdWatchRawBackend(ctx context.Context, e *env) error {
	client := localbackend.NewClient("")
	st, err := client.Status(ctx)
	if err != nil {
		return err
	}
	if !st.Healthy {
		return fmt.Errorf("%w: current active device is %s: %s", errBackendDegraded, st.State, st.LastError)
	}

	ctx, stop := signal.NotifyContext(ctx, os.Interrupt)
	defer stop()

	packets, err := client.WatchRaw(ctx)
	if err != nil {
		return err
	}
	e.out.Human("Watching raw packets from backend %s (Ctrl-C to stop)...\n", st.URI)
	for pkt := range packets {
		printRawPacket(e, pkt)
	}
	return nil
}

func cmdWatchBackend(ctx context.Context, e *env) error {
	client := localbackend.NewClient("")
	st, err := client.Status(ctx)
	if err != nil {
		return err
	}
	if !st.Healthy {
		return fmt.Errorf("%w: current active device is %s: %s", errBackendDegraded, st.State, st.LastError)
	}

	ctx, stop := signal.NotifyContext(ctx, os.Interrupt)
	defer stop()

	events, err := client.Watch(ctx)
	if err != nil {
		return err
	}
	e.out.Human("Watching backend %s (Ctrl-C to stop)...\n", st.URI)
	for ev := range events {
		printBackendEvent(e, ev)
	}
	return nil
}

func printEvent(e *env, ev meshcore.Event, names map[string]string) {
	switch m := ev.(type) {
	case meshcore.MessageReceived:
		from := resolveName(names, m.From.Name)
		if m.Channel != "" {
			from = "#" + m.Channel
		}
		if e.out.JSON {
			_ = e.out.Line(map[string]any{"type": "message", "from": from, "text": m.Text, "timestamp": m.Timestamp})
			return
		}
		e.out.Human("%s: %s\n", from, m.Text)
	case meshcore.MessageAcknowledged:
		if e.out.JSON {
			_ = e.out.Line(map[string]any{"type": "ack", "code": m.Code, "rtt_ms": m.RTT.Milliseconds()})
			return
		}
		e.out.Human("ack %s\n", m.Code)
	case meshcore.AdvertisementReceived:
		if e.out.JSON {
			_ = e.out.Line(map[string]any{"type": "advert", "name": m.Contact.Name, "public_key": m.Contact.PublicKey})
			return
		}
		e.out.Human("advert: %s\n", m.Contact.Name)
	case meshcore.Disconnected:
		if e.out.JSON {
			_ = e.out.Line(map[string]any{"type": "disconnected"})
			return
		}
		e.out.Human("disconnected: %v\n", m.Err)
	}
}

func printBackendEvent(e *env, ev localbackend.Event) {
	if e.out.JSON {
		_ = e.out.Line(ev)
		return
	}
	switch ev.Type {
	case "message":
		from := ev.From
		if from == "" && ev.Channel != "" {
			from = "#" + ev.Channel
		}
		e.out.Human("%s: %s\n", from, ev.Text)
	case "ack":
		e.out.Human("ack %s\n", ev.Code)
	case "advert":
		e.out.Human("advert: %s\n", ev.Name)
	case "disconnected":
		e.out.Human("disconnected: %s\n", ev.Error)
	}
}

func printRawPacket(e *env, pkt meshcore.RawPacket) {
	row := map[string]any{
		"timestamp":    pkt.Timestamp,
		"direction":    pkt.Direction,
		"type":         fmt.Sprintf("0x%02x", pkt.Type),
		"async":        pkt.Async,
		"decoded_type": pkt.DecodedType,
		"length":       len(pkt.Bytes),
		"bytes":        hexLine(pkt.Bytes),
	}
	if pkt.DecodeError != "" {
		row["decode_error"] = pkt.DecodeError
	}
	if e.out.JSON {
		_ = e.out.Line(row)
		return
	}
	e.out.Human("%s %-3s type=0x%02x async=%t len=%d %s\n",
		pkt.Timestamp.Format("15:04:05.000"), pkt.Direction, pkt.Type, pkt.Async, len(pkt.Bytes), hexLine(pkt.Bytes))
	if pkt.DecodeError != "" {
		e.out.Human("  decode error: %s\n", pkt.DecodeError)
	}
}
