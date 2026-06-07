package cli

import (
	"testing"

	meshcore "github.com/meshcore-cz/meshcore-go"
)

func testContact(name string, firstByte byte) meshcore.Contact {
	key := make([]byte, 32)
	key[0] = firstByte
	for i := 1; i < len(key); i++ {
		key[i] = byte(i)
	}
	return meshcore.Contact{
		Name:      name,
		PublicKey: formatTestKey(key),
	}
}

func testContactPrefix(name string, prefix []byte) meshcore.Contact {
	key := make([]byte, 32)
	copy(key, prefix)
	return meshcore.Contact{
		Name:      name,
		PublicKey: formatTestKey(key),
	}
}

func formatTestKey(key []byte) string {
	const hex = "0123456789abcdef"
	out := make([]byte, len(key)*2)
	for i, b := range key {
		out[i*2] = hex[b>>4]
		out[i*2+1] = hex[b&0x0f]
	}
	return string(out)
}

func TestTraceHopResolveExactWidth(t *testing.T) {
	idx := traceNameIndexFromContacts([]meshcore.Contact{
		testContact("Effik monitoring", 0x25),
		testContactPrefix("mc.kololec.cz", []byte{0x25, 0x25}),
	})

	node := idx.resolveNode([]byte{0x25, 0x25})
	if node.Ambiguous || node.PlainLabel() != "[2525] mc.kololec.cz" {
		t.Fatalf("2-byte match = %+v label=%q", node, node.PlainLabel())
	}

	node = idx.resolveNode([]byte{0x25})
	if !node.Ambiguous || node.PlainLabel() != "[25]" {
		t.Fatalf("1-byte match = %+v label=%q", node, node.PlainLabel())
	}

	if node := idx.resolveNode([]byte{0x25, 0x26}); node.Hash != "2526" || len(node.Names) != 0 {
		t.Fatalf("unknown hop = %+v", node)
	}
}

func TestTraceHopResolveAmbiguous(t *testing.T) {
	idx := traceNameIndexFromContacts([]meshcore.Contact{
		testContact("alice", 0x25),
		testContact("bob", 0x25),
	})

	node := idx.resolveNode([]byte{0x25})
	if !node.Ambiguous {
		t.Fatal("expected ambiguous prefix")
	}
	if node.PlainLabel() != "[25]" {
		t.Fatalf("label = %q", node.PlainLabel())
	}
}

func (n traceNode) PlainLabel() string {
	if n.Ambiguous || n.Name == "" {
		return traceHashLabel(n.Hash)
	}
	return traceHashLabel(n.Hash) + " " + n.Name
}

func TestTraceNodeFromKey(t *testing.T) {
	got := traceNodeFromKey("tree", "aabbccddeeff00112233445566778899aabbccddeeff00112233445566778899", 2)
	if got.LegacyJSONLabel() != "tree [aabb]" {
		t.Fatalf("legacy JSON = %q", got.LegacyJSONLabel())
	}
	if got.PlainLabel() != "[aabb] tree" {
		t.Fatalf("plain = %q", got.PlainLabel())
	}
}

func TestTracePrefixLabel(t *testing.T) {
	if got := tracePrefixLabel(4); got != "4B" {
		t.Fatalf("got %q", got)
	}
}

func TestTracePlanPathLabel(t *testing.T) {
	got := tracePlanPathLabel(meshcore.TracePlan{
		Path:     []byte{0x25, 0x25, 0xce, 0x52},
		HashSize: 4,
	})
	if got != "2525ce52" {
		t.Fatalf("got %q", got)
	}
}

func TestTraceDisplayPathLabel(t *testing.T) {
	got := traceDisplayPathLabel(meshcore.TracePlan{
		Path:     []byte{0xa9, 0x0d, 0x57, 0xdb, 0x3f, 0x18},
		HashSize: 2,
	})
	if got != "a90d → 57db → 3f18" {
		t.Fatalf("got %q", got)
	}
}

func TestTraceRequestLabel(t *testing.T) {
	tests := []struct {
		source string
		path   []byte
		size   int
		want   string
	}{
		{"explicit_path", []byte{0x25, 0x25}, 2, "explicit path · 2525"},
		{"explicit_path", []byte{0xa9, 0x0d, 0x57, 0xdb, 0x3f, 0x18}, 2, "explicit path · a90d → 57db → 3f18"},
		{"contact_out_path", []byte{0xa9, 0x0d, 0x57, 0xdb, 0x3f, 0x18}, 2, "contact route · a90d → 57db → 3f18"},
		{"contact_direct_path", []byte{0x25, 0x25}, 2, "contact route · direct"},
		{"contact_direct_path", nil, 2, "contact route · direct"},
		{"contact_key_fallback", []byte{0x25, 0x25}, 2, "contact key fallback · 2525"},
	}
	for _, tc := range tests {
		plan := meshcore.TracePlan{Source: tc.source, Path: tc.path, HashSize: tc.size}
		if got := traceRequestLabel(&plan); got != tc.want {
			t.Fatalf("source=%s got %q want %q", tc.source, got, tc.want)
		}
	}
}

func TestTraceOriginJSONStructured(t *testing.T) {
	origin := traceNode{Hash: "eff0", Name: "EFF01EF2"}
	got := traceOriginJSON(origin)
	if got["label"] != "EFF01EF2 [eff0]" {
		t.Fatalf("label = %q", got["label"])
	}
	if got["name"] != "EFF01EF2" || got["hash"] != "eff0" {
		t.Fatalf("origin = %#v", got)
	}
}

func TestTraceJSONCompatibility(t *testing.T) {
	idx := traceNameIndexFromContacts([]meshcore.Contact{
		testContactPrefix("mc.kololec.cz", []byte{0x25, 0x25}),
	})
	origin := traceNode{Hash: "eff0", Name: "EFF01EF2"}
	trace := meshcore.Trace{
		Target:       "2525",
		Tag:          0xb7e46d9a,
		Path:         []byte{0x25, 0x25},
		PathHashSize: 2,
		SNRs:         []float64{12.5, 11.5},
		RoundTrip:    687_000_000,
	}
	plan := meshcore.TracePlan{
		Query:    "2525",
		Source:   "explicit_path",
		Path:     []byte{0x25, 0x25},
		HashSize: 2,
	}
	out := traceJSON(trace, idx, origin, &plan)

	for _, key := range []string{
		"target", "tag", "origin", "path", "prefix_bytes", "prefix",
		"path_hash_bytes", "snr_db", "round_trip_ms", "sent_path", "source",
	} {
		if _, ok := out[key]; !ok {
			t.Fatalf("missing key %q in %#v", key, out)
		}
	}
	if out["target"] != "2525" {
		t.Fatalf("target = %v", out["target"])
	}
	if out["tag"] != "b7e46d9a" {
		t.Fatalf("tag = %v", out["tag"])
	}
	originMap, ok := out["origin"].(map[string]string)
	if !ok {
		t.Fatalf("origin type = %T", out["origin"])
	}
	if originMap["label"] != "EFF01EF2 [eff0]" {
		t.Fatalf("origin label = %q", originMap["label"])
	}
	path, ok := out["path"].([]map[string]any)
	if !ok || len(path) != 1 {
		t.Fatalf("path = %#v", out["path"])
	}
	if path[0]["hash"] != "2525" || path[0]["name"] != "mc.kololec.cz" {
		t.Fatalf("path item = %#v", path[0])
	}
	if out["prefix"] != "2B" || out["prefix_bytes"] != 2 {
		t.Fatalf("prefix = %#v bytes=%v", out["prefix"], out["prefix_bytes"])
	}
	if out["sent_path"] != "2525" || out["source"] != "explicit_path" {
		t.Fatalf("plan fields = %#v", out)
	}
}

func TestTracePathHashSizeThreeByteDownconvert(t *testing.T) {
	trace := meshcore.Trace{PathHashSize: 2}
	plan := &meshcore.TracePlan{
		OutPathEnc: 0x82,
		HashSize:   2,
	}
	if got := tracePathHashSize(trace, plan); got != 2 {
		t.Fatalf("hash size = %d, want 2", got)
	}
	if got := tracePrefixLabel(tracePathHashSize(trace, plan)); got != "2B" {
		t.Fatalf("prefix = %q, want 2B", got)
	}
}
