package pathhash

import "testing"

func TestIsHexTraceTarget(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want bool
	}{
		{"25", true},
		{"252525", true},
		{"a1b2,c3d4", true},
		{"mc.kololec.cz", false},
		{"25,abcd", true},
		{"25g", false},
		{"25a", false},
	} {
		if got := IsHexTraceTarget(tc.in); got != tc.want {
			t.Fatalf("IsHexTraceTarget(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestParsePathRejectsUnsupportedWidth(t *testing.T) {
	if _, _, err := ParsePath("252525"); err == nil {
		t.Fatal("expected error for 3-byte path")
	}
}

func TestParsePath(t *testing.T) {
	path, sz, err := ParsePath("25")
	if err != nil || sz != 1 || len(path) != 1 || path[0] != 0x25 {
		t.Fatalf("single hop = %x size=%d err=%v", path, sz, err)
	}

	path, sz, err = ParsePath("a1b2,c3d4")
	if err != nil || sz != 2 || len(path) != 4 {
		t.Fatalf("two-hop 2-byte = %x size=%d err=%v", path, sz, err)
	}

	if _, _, err := ParsePath("25,abcd"); err == nil {
		t.Fatal("expected mixed-width error")
	}
}

func TestTraceFlagsRoundTrip(t *testing.T) {
	for _, sz := range []int{1, 2, 4, 8} {
		flags, err := TraceFlagsFromHashSize(sz)
		if err != nil {
			t.Fatal(err)
		}
		if got := HashSizeFromTraceFlags(flags); got != sz {
			t.Fatalf("size %d: flags=%d got=%d", sz, flags, got)
		}
	}
}

func TestPathBytes(t *testing.T) {
	raw := []byte{0x25, 0x45, 0x00, 0x00}
	got := PathBytes(0x02, raw) // 2 hops, 1-byte hashes
	if len(got) != 2 || got[0] != 0x25 || got[1] != 0x45 {
		t.Fatalf("got %x", got)
	}
}

func TestNearestTraceHashSize(t *testing.T) {
	for _, tc := range []struct {
		in, want int
	}{
		{1, 1}, {2, 2}, {3, 4}, {4, 4}, {5, 8}, {8, 8},
	} {
		if got := NearestTraceHashSize(tc.in); got != tc.want {
			t.Fatalf("NearestTraceHashSize(%d) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestHexPrefixByteLen(t *testing.T) {
	if got := HexPrefixByteLen("252525"); got != 3 {
		t.Fatalf("HexPrefixByteLen(252525) = %d, want 3", got)
	}
	if got := HexPrefixByteLen("25,a1"); got != 0 {
		t.Fatalf("HexPrefixByteLen(25,a1) = %d, want 0", got)
	}
}

func TestOutPathDirectAndStored(t *testing.T) {
	if !IsDirectOutPath(0x00) {
		t.Fatal("0x00 should be direct")
	}
	if !IsDirectOutPath(0x40) {
		t.Fatal("0x40 should be direct with 2-byte hash size")
	}
	if IsDirectOutPath(0xff) || IsDirectOutPath(0x41) {
		t.Fatal("flood and multi-hop should not be direct")
	}
	if !HasStoredOutPath(0x41, []byte{0xaa, 0xbb}) {
		t.Fatal("0x41 with path bytes should be stored")
	}
	if HasStoredOutPath(0x00, make([]byte, 64)) {
		t.Fatal("direct path should not be stored multi-hop")
	}
}

func TestTracePathFromOutPathThreeByte(t *testing.T) {
	raw := []byte{0xaa, 0xbb, 0xcc, 0x11, 0x22, 0x33}
	path, size, flags, err := TracePathFromOutPath(0x82, raw) // 2 hops, 3-byte hashes
	if err != nil {
		t.Fatal(err)
	}
	if size != 2 || flags != 1 {
		t.Fatalf("size=%d flags=%d", size, flags)
	}
	if len(path) != 4 || path[0] != 0xaa || path[1] != 0xbb || path[2] != 0x11 || path[3] != 0x22 {
		t.Fatalf("path = %x", path)
	}
}
