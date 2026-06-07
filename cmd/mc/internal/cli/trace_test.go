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

	match := idx.resolve([]byte{0x25, 0x25})
	if match.ambiguous || match.label != "mc.kololec.cz [2525]" {
		t.Fatalf("2-byte match = %+v", match)
	}

	match = idx.resolve([]byte{0x25})
	if !match.ambiguous || match.label != "[25]" {
		t.Fatalf("1-byte match = %+v", match)
	}

	if match := idx.resolve([]byte{0x25, 0x26}); match.label != "[2526]" || len(match.names) != 0 {
		t.Fatalf("unknown hop = %+v", match)
	}
}

func TestTraceHopResolveAmbiguous(t *testing.T) {
	idx := traceNameIndexFromContacts([]meshcore.Contact{
		testContact("alice", 0x25),
		testContact("bob", 0x25),
	})

	match := idx.resolve([]byte{0x25})
	if !match.ambiguous {
		t.Fatal("expected ambiguous prefix")
	}
	if match.label != "[25]" {
		t.Fatalf("label = %q", match.label)
	}
}

func TestTraceNodeLabel(t *testing.T) {
	got := traceNodeLabel("tree", "aabbccddeeff00112233445566778899aabbccddeeff00112233445566778899", 2)
	want := "tree [aabb]"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestTracePrefixLabel(t *testing.T) {
	if got := tracePrefixLabel(4); got != "4-byte prefix" {
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
