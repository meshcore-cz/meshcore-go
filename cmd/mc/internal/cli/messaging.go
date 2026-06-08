package cli

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"sort"
	"strings"
	"time"

	meshcore "github.com/meshcore-cz/meshcore-go"
	localbackend "github.com/meshcore-cz/meshcore-go/backend"
	"github.com/meshcore-cz/meshcore-go/cmd/mc/internal/ui"
	"github.com/meshcore-cz/meshcore-go/protocol/pathhash"
)

// cmdContacts implements `mc contacts`.
func cmdContacts(ctx context.Context, e *env) error {
	backend, err := openContactsBackend(ctx, e)
	if err != nil {
		return err
	}
	defer backend.Close()

	if e.args.has("refresh") && !e.args.has("wait") {
		result, err := backend.StartContactRefresh(ctx, e.args.has("full"))
		if err != nil {
			return err
		}
		if e.out.JSON {
			return e.out.JSONValue(result)
		}
		if result.Started {
			e.out.Human("Contact synchronization started.\n")
		} else if result.Running {
			e.out.Human("Contact synchronization already running.\n")
		}
		return nil
	}

	contacts, err := backend.ContactsWithOptions(ctx, e.args.has("cached"), e.args.has("refresh"), e.args.has("wait"), e.args.has("full"))
	if err != nil {
		return err
	}

	query, err := contactListQueryFromEnv(e)
	if err != nil {
		return err
	}
	originLat, originLon := localOriginFromBackend(ctx, backend)
	contacts, err = filterContacts(contacts, query, originLat, originLon)
	if err != nil {
		return err
	}

	if e.out.JSON {
		return e.out.JSONValue(contacts)
	}
	if len(contacts) == 0 {
		if query.filtered() {
			e.out.Human("No contacts matching filters.\n")
		} else if e.args.has("cached") {
			e.out.Human("No contacts in device-local state.\n")
		} else {
			e.out.Human("No contacts.\n")
		}
		return nil
	}
	printContactsHuman(ctx, e, backend, contacts, e.args.has("wide"))
	return nil
}

func openContactsBackend(ctx context.Context, e *env) (Backend, error) {
	if e.args.has("cached") {
		return openIPCBackendAllowDegraded(ctx, e)
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
	printContactDetailHuman(ctx, e, backend, ct)
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
	if channel := e.args.flag("channel"); channel != "" {
		return sendChannel(ctx, e, channel)
	}

	recipient := e.restArg(0)
	text := e.restArg(1)
	if recipient == "" || text == "" {
		return fmt.Errorf("usage: mc send <recipient> <text> [--wait]  |  mc send --channel <name> <text>")
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

// sendChannel handles `mc send --channel <name> <text>`. Channel messages are
// broadcast and not individually acknowledged, so --wait does not apply.
func sendChannel(ctx context.Context, e *env, channel string) error {
	text := e.restArg(0)
	if text == "" {
		return fmt.Errorf("usage: mc send --channel <name|index> <text>")
	}

	backend, err := openBackend(ctx, e)
	if err != nil {
		return err
	}
	defer backend.Close()

	receipt, err := backend.SendChannelText(ctx, channel, text)
	if err != nil {
		return err
	}
	e.out.Human("Sent to %s.\n", receipt.To)
	return e.out.JSONValue(map[string]any{"to": receipt.To, "id": receipt.ID()})
}

// cmdTrace implements `mc trace <target>`.
func cmdTrace(ctx context.Context, e *env) error {
	target := e.restArg(0)
	if target == "" {
		return fmt.Errorf("usage: mc trace <target>")
	}
	if e.args.has("return") {
		var err error
		target, err = traceReturnTarget(target)
		if err != nil {
			return err
		}
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
	origin := traceOriginNode(ctx, backend, trace)
	var planPtr *meshcore.TracePlan
	if planErr == nil {
		planPtr = &plan
	}
	if e.out.JSON {
		return e.out.JSONValue(traceJSON(trace, hopIndex, origin, planPtr))
	}
	printTrace(e, trace, hopIndex, origin, planPtr)
	return nil
}

func traceReturnTarget(target string) (string, error) {
	if !pathhash.IsHexTraceTarget(target) {
		return "", fmt.Errorf("--return requires an explicit trace path")
	}
	path, hashSize, err := pathhash.ParsePath(target)
	if err != nil {
		return "", err
	}
	hops := pathhash.Split(path, hashSize)
	if len(hops) < 2 {
		return target, nil
	}
	parts := make([]string, 0, len(hops)*2-1)
	for _, hop := range hops {
		parts = append(parts, pathhash.FormatHop(hop))
	}
	for i := len(hops) - 2; i >= 0; i-- {
		parts = append(parts, pathhash.FormatHop(hops[i]))
	}
	return strings.Join(parts, ","), nil
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

func traceJSON(trace meshcore.Trace, idx traceNameIndex, origin traceNode, plan *meshcore.TracePlan) map[string]any {
	hops := traceHops(trace)
	path := make([]map[string]any, len(hops))
	for i, hop := range hops {
		node := idx.resolveNode(hop)
		item := map[string]any{"hash": node.Hash}
		if node.Ambiguous {
			item["ambiguous"] = true
			item["names"] = node.Names
		} else if node.Name != "" {
			item["name"] = node.Name
		}
		path[i] = item
	}
	hashSize := tracePathHashSize(trace, plan)
	out := map[string]any{
		"target":          trace.Target,
		"tag":             fmt.Sprintf("%08x", trace.Tag),
		"origin":          traceOriginJSON(origin),
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

func traceOriginJSON(origin traceNode) map[string]string {
	out := map[string]string{"label": origin.LegacyJSONLabel()}
	if origin.Name != "" {
		out["name"] = origin.Name
	}
	if origin.Hash != "" {
		out["hash"] = origin.Hash
	}
	return out
}

func printTrace(e *env, trace meshcore.Trace, idx traceNameIndex, origin traceNode, plan *meshcore.TracePlan) {
	data := buildTraceData(trace, idx, origin, plan)
	printer := ui.NewPrinter(e.out.Out)
	printer.Print(ui.RenderTrace(data, printer))
}

func buildTraceData(trace meshcore.Trace, idx traceNameIndex, origin traceNode, plan *meshcore.TracePlan) ui.TraceData {
	hashSize := tracePathHashSize(trace, plan)
	hops := traceHops(trace)

	pathNodes := make([]traceNode, len(hops))
	ambiguous := make([]traceNode, 0)
	for i, hop := range hops {
		node := idx.resolveNode(hop)
		pathNodes[i] = node
		if node.Ambiguous {
			ambiguous = append(ambiguous, node)
		}
	}

	nodes := make([]traceNode, 0, len(pathNodes)+2)
	nodes = append(nodes, origin)
	nodes = append(nodes, pathNodes...)
	nodes = append(nodes, origin)

	legs := make([]ui.TraceLeg, 0, len(trace.SNRs))
	for i, snr := range trace.SNRs {
		if i+1 >= len(nodes) {
			break
		}
		legs = append(legs, ui.TraceLeg{
			Number: i + 1,
			From:   toUITraceNode(nodes[i]),
			To:     toUITraceNode(nodes[i+1]),
			SNRDB:  snr,
		})
	}

	target := traceTargetNode(trace, plan, pathNodes, idx)
	if target.Ambiguous {
		ambiguous = append(ambiguous, target)
	}

	return ui.TraceData{
		Target:      toUITraceNode(target),
		Request:     traceRequestLabel(plan),
		PrefixBytes: hashSize,
		Tag:         fmt.Sprintf("%08x", trace.Tag),
		RoundTrip:   trace.RoundTrip,
		Legs:        legs,
		Ambiguous:   toUITraceNodes(ambiguous),
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
		return "1B"
	case 2:
		return "2B"
	case 4:
		return "4B"
	case 8:
		return "8B"
	default:
		return fmt.Sprintf("%dB", bytes)
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
	case "contact_out_path", "contact_direct_path":
		return "contact route"
	case "contact_key_fallback":
		return "contact key fallback"
	default:
		return source
	}
}

func traceRequestLabel(plan *meshcore.TracePlan) string {
	if plan == nil {
		return ""
	}
	source := traceSourceLabel(plan.Source)
	if plan.Source == "contact_direct_path" {
		return source + " · direct"
	}
	if path := traceDisplayPathLabel(*plan); path != "" {
		return source + " · " + path
	}
	return source
}

func traceDisplayPathLabel(plan meshcore.TracePlan) string {
	hops := pathhash.Split(plan.Path, plan.HashSize)
	if len(hops) == 0 {
		return ""
	}
	parts := make([]string, len(hops))
	for i, hop := range hops {
		parts[i] = pathhash.FormatHop(hop)
	}
	return strings.Join(parts, " → ")
}

func traceTargetNode(trace meshcore.Trace, plan *meshcore.TracePlan, pathNodes []traceNode, idx traceNameIndex) traceNode {
	if len(pathNodes) > 0 {
		return pathNodes[len(pathNodes)-1]
	}
	if plan != nil && len(plan.Path) > 0 {
		hops := pathhash.Split(plan.Path, plan.HashSize)
		if len(hops) > 0 {
			return idx.resolveNode(hops[len(hops)-1])
		}
	}
	if trace.Target != "" {
		return traceNode{Hash: trace.Target}
	}
	return traceNode{}
}

type traceNode struct {
	Hash      string
	Name      string
	Names     []string
	Ambiguous bool
}

func (n traceNode) LegacyJSONLabel() string {
	if n.Name != "" {
		return n.Name + " " + traceHashLabel(n.Hash)
	}
	return traceHashLabel(n.Hash)
}

func toUITraceNode(n traceNode) ui.TraceNode {
	return ui.TraceNode{
		Hash:      n.Hash,
		Name:      n.Name,
		Names:     n.Names,
		Ambiguous: n.Ambiguous,
	}
}

func toUITraceNodes(nodes []traceNode) []ui.TraceNode {
	out := make([]ui.TraceNode, len(nodes))
	for i, node := range nodes {
		out[i] = toUITraceNode(node)
	}
	return out
}

type traceNameIndex struct {
	byPrefix map[string][]string
}

func traceHopIndex(ctx context.Context, backend Backend) traceNameIndex {
	contacts, err := backend.ContactsWithOptions(ctx, true, false, false, false)
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

func (idx traceNameIndex) resolveNode(hop []byte) traceNode {
	hash := pathhash.FormatHop(hop)
	names := append([]string(nil), idx.byPrefix[hash]...)
	sort.Strings(names)
	switch len(names) {
	case 0:
		return traceNode{Hash: hash}
	case 1:
		return traceNode{Hash: hash, Name: names[0], Names: names}
	default:
		return traceNode{Hash: hash, Names: names, Ambiguous: true}
	}
}

func traceHashLabel(hash string) string {
	return fmt.Sprintf("[%s]", hash)
}

func containsString(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func traceOriginNode(ctx context.Context, backend Backend, trace meshcore.Trace) traceNode {
	info, err := backend.DeviceInfo(ctx)
	if err != nil {
		return traceNode{Name: "you"}
	}
	hashSize := trace.PathHashSize
	if hashSize <= 0 {
		hashSize = 1
	}
	return traceNodeFromKey(info.Name, info.PublicKey, hashSize)
}

func traceNodeFromKey(name, pubKeyHex string, hashSize int) traceNode {
	key, err := hex.DecodeString(pubKeyHex)
	if err != nil || len(key) == 0 {
		if name != "" {
			return traceNode{Name: name}
		}
		return traceNode{Name: "you"}
	}
	if hashSize <= 0 {
		hashSize = 1
	}
	if hashSize > len(key) {
		hashSize = len(key)
	}
	hash := pathhash.FormatHop(key[:hashSize])
	if name != "" {
		return traceNode{Hash: hash, Name: name}
	}
	return traceNode{Hash: hash}
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
	if e.args.has("rf") {
		return cmdWatchRF(ctx, e)
	}

	if !e.args.has("direct") {
		err := cmdWatchBackend(ctx, e)
		if err == nil {
			return nil
		}
		if errors.Is(err, errBackendDegraded) || e.exec.RequireIPC {
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

func cmdWatchRF(ctx context.Context, e *env) error {
	e.out.JSON = true
	if !e.args.has("direct") {
		err := cmdWatchRFBackend(ctx, e)
		if err == nil {
			return nil
		}
		if errors.Is(err, errBackendDegraded) || e.exec.RequireIPC {
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

	e.out.Human("Watching RF packets from %s (Ctrl-C to stop)...\n", uri)
	for {
		select {
		case <-ctx.Done():
			return nil
		case ev, ok := <-client.Events():
			if !ok {
				return nil
			}
			if rf, ok := rfPacketFromEvent(ev); ok {
				printRFPacket(e, rf)
			}
		}
	}
}

func cmdWatchRFBackend(ctx context.Context, e *env) error {
	client := backendClientForEnv(e)
	st, err := client.Status(ctx)
	if err != nil {
		return err
	}
	if !st.Healthy {
		return fmt.Errorf("%w: current active device is %s: %s", errBackendDegraded, st.State, st.LastError)
	}

	ctx, stop := signal.NotifyContext(ctx, os.Interrupt)
	defer stop()

	packets, err := client.WatchRF(ctx)
	if err != nil {
		return err
	}
	e.out.Human("Watching RF packets from backend %s (Ctrl-C to stop)...\n", st.URI)
	for pkt := range packets {
		printRFPacket(e, pkt)
	}
	return nil
}

func cmdWatchRaw(ctx context.Context, e *env) error {
	e.out.JSON = true
	if !e.args.has("direct") {
		err := cmdWatchRawBackend(ctx, e)
		if err == nil {
			return nil
		}
		if errors.Is(err, errBackendDegraded) || e.exec.RequireIPC {
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
	client := backendClientForEnv(e)
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
	client := backendClientForEnv(e)
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

func printRFPacket(e *env, pkt meshcore.RFPacketReceived) {
	_ = e.out.Line(map[string]any{
		"timestamp": pkt.Timestamp,
		"snr":       pkt.SNR,
		"rssi":      pkt.RSSI,
		"bytes":     hexLine(pkt.Bytes),
	})
}

func rfPacketFromEvent(ev meshcore.Event) (meshcore.RFPacketReceived, bool) {
	rf, ok := ev.(meshcore.RFPacketReceived)
	return rf, ok
}
