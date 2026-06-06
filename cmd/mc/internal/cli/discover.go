package cli

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	meshcore "github.com/meshcore-cz/meshcore-go"
)

// cmdDiscover implements `mc discover`: broadcast a node-discovery request and
// print nodes as they reply. With no type flags it scans for repeaters.
func cmdDiscover(ctx context.Context, e *env) error {
	opts := meshcore.NodeDiscoverOptions{
		Filter:     discoverFilter(e),
		PrefixOnly: !e.args.has("full"),
	}
	if secs := e.args.flag("timeout"); secs != "" {
		n, err := strconv.Atoi(secs)
		if err != nil || n <= 0 {
			return fmt.Errorf("invalid --timeout %q (expected whole seconds)", secs)
		}
		opts.Timeout = time.Duration(n) * time.Second
	}

	backend, err := openBackend(ctx, e)
	if err != nil {
		return err
	}
	defer backend.Close()

	e.out.Human("Discovering %s...\n", discoverFilterLabel(e))

	count := 0
	if _, err := backend.DiscoverNodes(ctx, opts, func(n meshcore.DiscoveredNode) {
		count++
		printDiscoveredNode(e, n)
	}); err != nil {
		return err
	}
	e.out.Human("Discovered %d node(s).\n", count)
	return nil
}

func printDiscoveredNode(e *env, n meshcore.DiscoveredNode) {
	if e.out.JSON {
		_ = e.out.Line(n)
		return
	}
	e.out.Human("%-9s %-18s  ↑ %.2fdB  ↓ %.2fdB  rssi %ddBm\n",
		n.Type, shortKey(n.PublicKey), n.SNRUp, n.SNRDown, n.RSSI)
}

// discoverFilter builds the node-type bitmask from flags. With no type flags it
// defaults to repeaters.
func discoverFilter(e *env) byte {
	if e.args.has("all") {
		return meshcore.NodeFilterAll
	}
	var f byte
	if e.args.has("companion") || e.args.has("client") {
		f |= meshcore.NodeFilterCompanion
	}
	if e.args.has("repeater") {
		f |= meshcore.NodeFilterRepeater
	}
	if e.args.has("room") {
		f |= meshcore.NodeFilterRoom
	}
	if e.args.has("sensor") {
		f |= meshcore.NodeFilterSensor
	}
	if f == 0 {
		return meshcore.NodeFilterRepeater
	}
	return f
}

func discoverFilterLabel(e *env) string {
	if e.args.has("all") {
		return "all nodes"
	}
	var parts []string
	if e.args.has("companion") || e.args.has("client") {
		parts = append(parts, "companions")
	}
	if e.args.has("repeater") {
		parts = append(parts, "repeaters")
	}
	if e.args.has("room") {
		parts = append(parts, "rooms")
	}
	if e.args.has("sensor") {
		parts = append(parts, "sensors")
	}
	if len(parts) == 0 {
		return "repeaters"
	}
	return strings.Join(parts, ", ")
}
