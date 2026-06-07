package cli

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"time"

	meshcore "github.com/meshcore-cz/meshcore-go"
	localbackend "github.com/meshcore-cz/meshcore-go/backend"
	"github.com/meshcore-cz/meshcore-go/protocol/pathhash"
)

// cmdContacts implements `mc contacts`.
func cmdContacts(ctx context.Context, e *env) error {
	backend, err := openContactsBackend(ctx, e)
	if err != nil {
		return err
	}
	defer backend.Close()

	contacts, err := backend.ContactsWithOptions(ctx, e.args.has("cached"), e.args.has("refresh"))
	if err != nil {
		return err
	}
	if e.out.JSON {
		return e.out.JSONValue(contacts)
	}
	if len(contacts) == 0 {
		if e.args.has("cached") {
			e.out.Human("No contacts in local replica.\n")
		} else {
			e.out.Human("No contacts.\n")
		}
		return nil
	}
	if e.args.has("cached") {
		e.out.Human("Local replica:\n")
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

func openContactsBackend(ctx context.Context, e *env) (Backend, error) {
	if e.args.has("cached") {
		return openIPCBackendAllowDegraded(ctx)
	}
	return openBackend(ctx, e)
}

// cmdContact implements `mc contact show <name>`.
func cmdContact(ctx context.Context, e *env) error {
	if e.restArg(0) != "show" {
		return fmt.Errorf("usage: mc contact show <name>")
	}
	name := e.restArg(1)
	if name == "" {
		return fmt.Errorf("usage: mc contact show <name>")
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

// cmdInbox implements `mc inbox`: drain buffered messages.
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

// cmdSend implements `mc send <recipient> <text> [--wait]`.
func cmdSend(ctx context.Context, e *env) error {
	recipient := e.restArg(0)
	text := e.restArg(1)
	if recipient == "" || text == "" {
		return fmt.Errorf("usage: mc send <recipient> <text> [--wait]")
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

// cmdTrace implements `mc trace <target>`.
func cmdTrace(ctx context.Context, e *env) error {
	target := e.restArg(0)
	if target == "" {
		return fmt.Errorf("usage: mc trace <target>")
	}

	backend, err := openBackend(ctx, e)
	if err != nil {
		return err
	}
	defer backend.Close()

	start := time.Now()
	e.dbg.TraceStarted(target, start)

	plan, planErr := planTrace(ctx, backend, target)
	if planErr == nil {
		e.dbg.TracePlan(plan)
		if plan.Contact != "" {
			if ct, err := backend.Contact(ctx, target); err == nil {
				e.dbg.Contact(ct)
			}
		}
	} else {
		e.dbg.Log("trace plan failed", "target", target, "error", planErr)
	}

	trace, err := backend.Trace(ctx, target)
	e.dbg.TraceDone(start, trace, err)
	if err != nil {
		return err
	}
	hopIndex := traceHopIndex(ctx, backend)
	selfLabel := traceSelfLabel(ctx, backend, trace)
	var planPtr *meshcore.TracePlan
	if planErr == nil {
		planPtr = &plan
	}
	if e.out.JSON {
		return e.out.JSONValue(traceJSON(trace, hopIndex, selfLabel, planPtr))
	}
	printTrace(e, trace, hopIndex, selfLabel, planPtr)
	return nil
}

func planTrace(ctx context.Context, backend Backend, target string) (meshcore.TracePlan, error) {
	if pathhash.IsHexTraceTarget(target) {
		path, hashSize, err := pathhash.ParsePath(target)
		if err != nil {
			return meshcore.TracePlan{}, err
		}
		flags, err := pathhash.TraceFlagsFromHashSize(hashSize)
		if err != nil {
			return meshcore.TracePlan{}, err
		}
		return meshcore.TracePlan{
			Query:    target,
			Name:     target,
			Source:   "explicit_path",
			Path:     path,
			HashSize: hashSize,
			Flags:    flags,
			HopCount: len(path) / hashSize,
		}, nil
	}
	ct, err := backend.Contact(ctx, target)
	if err != nil {
		return meshcore.TracePlan{}, err
	}
	plan, err := meshcore.PlanTraceContact(ct, 0)
	if err != nil {
		return meshcore.TracePlan{}, err
	}
	plan.Query = target
	return plan, nil
}

func traceJSON(trace meshcore.Trace, idx traceNameIndex, selfLabel string, plan *meshcore.TracePlan) map[string]any {
	hops := traceHops(trace)
	path := make([]map[string]any, len(hops))
	for i, hop := range hops {
		match := idx.resolve(hop)
		item := map[string]any{"hash": match.hash}
		if match.ambiguous {
			item["ambiguous"] = true
			item["names"] = match.names
		} else if len(match.names) == 1 {
			item["name"] = match.names[0]
		}
		path[i] = item
	}
	hashSize := tracePathHashSize(trace, plan)
	out := map[string]any{
		"target":          trace.Target,
		"tag":             fmt.Sprintf("%08x", trace.Tag),
		"origin":          traceOriginJSON(selfLabel),
		"path":            path,
		"prefix_bytes":    hashSize,
		"prefix":          tracePrefixLabel(hashSize),
		"path_hash_bytes": hashSize,
		"snr_db":          trace.SNRs,
		"round_trip_ms":   trace.RoundTrip.Milliseconds(),
	}
	if plan != nil {
		out["sent_path"] = tracePlanPathLabel(*plan)
		out["source"] = plan.Source
		if plan.Contact != "" {
			out["contact"] = plan.Contact
		}
	}
	return out
}

func traceOriginJSON(selfLabel string) map[string]string {
	out := map[string]string{"label": selfLabel}
	if name, hashPart, ok := strings.Cut(selfLabel, " ["); ok && strings.HasSuffix(hashPart, "]") {
		out["name"] = name
		out["hash"] = strings.TrimSuffix(hashPart, "]")
		return out
	}
	if strings.HasPrefix(selfLabel, "[") && strings.HasSuffix(selfLabel, "]") {
		out["hash"] = strings.TrimPrefix(strings.TrimSuffix(selfLabel, "]"), "[")
	}
	return out
}

func printTrace(e *env, trace meshcore.Trace, idx traceNameIndex, selfLabel string, plan *meshcore.TracePlan) {
	hashSize := tracePathHashSize(trace, plan)
	e.out.Human("Trace to %s  ·  %s  ·  %s  ·  tag %08x\n",
		trace.Target, tracePrefixLabel(hashSize), trace.RoundTrip.Round(1e6), trace.Tag)
	if plan != nil && len(plan.Path) > 0 {
		e.out.Human("  sent %s  ·  %s\n", tracePlanPathLabel(*plan), traceSourceLabel(plan.Source))
	}

	hops := traceHops(trace)
	nodes := make([]string, len(hops)+2)
	matches := make([]traceHopMatch, 0, len(hops))
	nodes[0] = selfLabel
	for i, hop := range hops {
		match := idx.resolve(hop)
		matches = append(matches, match)
		nodes[i+1] = match.label
	}
	nodes[len(nodes)-1] = selfLabel

	if len(trace.SNRs) == 0 {
		e.out.Human("  no signal data\n")
		return
	}

	col := len(selfLabel)
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
	printTraceAmbiguous(e, matches)
}

func printTraceAmbiguous(e *env, matches []traceHopMatch) {
	seen := map[string]bool{}
	for _, match := range matches {
		if !match.ambiguous || seen[match.hash] {
			continue
		}
		seen[match.hash] = true
		e.out.Human("\n  ambiguous prefix %s: %s\n", traceHashLabel(match.hash), strings.Join(match.names, ", "))
	}
}

func traceHops(trace meshcore.Trace) [][]byte {
	size := trace.PathHashSize
	if size <= 0 {
		size = 1
	}
	return pathhash.Split(trace.Path, size)
}

func tracePathHashSize(trace meshcore.Trace, plan *meshcore.TracePlan) int {
	if trace.PathHashSize > 0 {
		return trace.PathHashSize
	}
	if plan != nil && plan.HashSize > 0 {
		return plan.HashSize
	}
	return 1
}

func tracePrefixLabel(bytes int) string {
	switch bytes {
	case 1:
		return "1-byte prefix"
	case 2:
		return "2-byte prefix"
	case 4:
		return "4-byte prefix"
	case 8:
		return "8-byte prefix"
	default:
		return fmt.Sprintf("%d-byte prefix", bytes)
	}
}

func tracePlanPathLabel(plan meshcore.TracePlan) string {
	hops := pathhash.Split(plan.Path, plan.HashSize)
	if len(hops) == 0 {
		return ""
	}
	parts := make([]string, len(hops))
	for i, hop := range hops {
		parts[i] = pathhash.FormatHop(hop)
	}
	return strings.Join(parts, ",")
}

func traceSourceLabel(source string) string {
	switch source {
	case "explicit_path":
		return "explicit path"
	case "contact_out_path":
		return "contact out-path"
	case "contact_direct_path":
		return "contact direct"
	case "contact_key_fallback":
		return "contact key fallback"
	default:
		return source
	}
}

type traceHopMatch struct {
	label     string
	hash      string
	names     []string
	ambiguous bool
}

type traceNameIndex struct {
	byPrefix map[string][]string
}

func traceHopIndex(ctx context.Context, backend Backend) traceNameIndex {
	contacts, err := backend.ContactsWithOptions(ctx, true, false)
	if err != nil {
		return traceNameIndex{byPrefix: map[string][]string{}}
	}
	return traceNameIndexFromContacts(contacts)
}

func traceNameIndexFromContacts(contacts []meshcore.Contact) traceNameIndex {
	idx := traceNameIndex{byPrefix: make(map[string][]string, len(contacts))}
	for _, ct := range contacts {
		key, ok := contactKeyBytes(ct)
		if !ok || len(key) == 0 || ct.Name == "" {
			continue
		}
		for _, width := range []int{8, 4, 2, 1} {
			if len(key) < width {
				continue
			}
			prefix := pathhash.FormatHop(key[:width])
			if !containsString(idx.byPrefix[prefix], ct.Name) {
				idx.byPrefix[prefix] = append(idx.byPrefix[prefix], ct.Name)
			}
		}
	}
	return idx
}

func (idx traceNameIndex) resolve(hop []byte) traceHopMatch {
	hash := pathhash.FormatHop(hop)
	names := idx.byPrefix[hash]
	switch len(names) {
	case 0:
		return traceHopMatch{label: traceHashLabel(hash), hash: hash}
	case 1:
		return traceHopMatch{
			label: traceNamedLabel(names[0], hash),
			hash:  hash,
			names: names,
		}
	default:
		return traceHopMatch{
			label:     traceHashLabel(hash),
			hash:      hash,
			names:     names,
			ambiguous: true,
		}
	}
}

func traceHashLabel(hash string) string {
	return fmt.Sprintf("[%s]", hash)
}

func traceNamedLabel(name, hash string) string {
	return fmt.Sprintf("%s %s", name, traceHashLabel(hash))
}

func containsString(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func traceSelfLabel(ctx context.Context, backend Backend, trace meshcore.Trace) string {
	info, err := backend.DeviceInfo(ctx)
	if err != nil {
		return "you"
	}
	hashSize := trace.PathHashSize
	if hashSize <= 0 {
		hashSize = 1
	}
	return traceNodeLabel(info.Name, info.PublicKey, hashSize)
}

func traceNodeLabel(name, pubKeyHex string, hashSize int) string {
	key, err := hex.DecodeString(pubKeyHex)
	if err != nil || len(key) == 0 {
		if name != "" {
			return name
		}
		return "you"
	}
	if hashSize <= 0 {
		hashSize = 1
	}
	if hashSize > len(key) {
		hashSize = len(key)
	}
	hash := pathhash.FormatHop(key[:hashSize])
	if name != "" {
		return traceNamedLabel(name, hash)
	}
	return traceHashLabel(hash)
}

func contactKeyBytes(ct meshcore.Contact) ([]byte, bool) {
	key, err := hex.DecodeString(ct.PublicKey)
	if err != nil || len(key) == 0 {
		return nil, false
	}
	return key, true
}

// cmdWatch implements `mc watch`: stream asynchronous events.
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
