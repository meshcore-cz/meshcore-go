// Package pathhash implements MeshCore path hash encoding helpers.
package pathhash

import (
	"encoding/hex"
	"fmt"
	"strings"
)

const (
	OutPathUnknown = 0xff
	MaxPathSize    = 64
)

// HashSizeFromTraceFlags returns the path hash size in bytes for a trace flags
// byte (lower two bits: hash size = 1 << bits).
func HashSizeFromTraceFlags(flags byte) int {
	return 1 << (flags & 0x03)
}

// TraceFlagsFromHashSize returns the trace flags byte for a supported hash
// size (1, 2, 4, or 8 bytes per hop).
func TraceFlagsFromHashSize(size int) (byte, error) {
	switch size {
	case 1:
		return 0, nil
	case 2:
		return 1, nil
	case 4:
		return 2, nil
	case 8:
		return 3, nil
	default:
		return 0, fmt.Errorf("unsupported trace hash size %d bytes (use 1, 2, 4, or 8)", size)
	}
}

// HashSizeFromPathMeta returns the hash size encoded in a contact/path metadata
// byte (bits 6-7 store hash_size - 1).
func HashSizeFromPathMeta(encoded byte) int {
	return int(encoded>>6) + 1
}

// HopCountFromPathMeta returns the hop count encoded in a metadata byte.
func HopCountFromPathMeta(encoded byte) int {
	return int(encoded & 0x3f)
}

// PathBytes trims raw path storage to the byte length implied by encoded.
func PathBytes(encoded byte, raw []byte) []byte {
	n := HopCountFromPathMeta(encoded) * HashSizeFromPathMeta(encoded)
	if n <= 0 {
		return nil
	}
	if n > len(raw) {
		n = len(raw)
	}
	return append([]byte(nil), raw[:n]...)
}

// NearestTraceHashSize rounds a desired hash width up to the nearest size
// supported by the trace companion protocol (1, 2, 4, or 8 bytes).
func NearestTraceHashSize(n int) int {
	switch {
	case n <= 1:
		return 1
	case n <= 2:
		return 2
	case n <= 4:
		return 4
	default:
		return 8
	}
}

// HexPrefixByteLen returns the byte length of s when it is a single hex prefix
// (even-length hex, no comma/space separators). Otherwise it returns 0.
func HexPrefixByteLen(s string) int {
	s = strings.TrimSpace(s)
	if s == "" || strings.ContainsAny(s, ", ") {
		return 0
	}
	for _, r := range s {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') && (r < 'A' || r > 'F') {
			return 0
		}
	}
	if len(s)%2 != 0 {
		return 0
	}
	return len(s) / 2
}

// IsDirectOutPath reports whether encoded metadata describes a known direct
// (zero-hop) route to the contact.
func IsDirectOutPath(encoded byte) bool {
	return encoded != OutPathUnknown && HopCountFromPathMeta(encoded) == 0
}

// HasStoredOutPath reports whether encoded metadata contains a multi-hop path.
func HasStoredOutPath(encoded byte, raw []byte) bool {
	if encoded == OutPathUnknown || HopCountFromPathMeta(encoded) == 0 {
		return false
	}
	return len(PathBytes(encoded, raw)) > 0
}

// TracePathFromOutPath converts a contact out-path to a trace payload path.
// Packet routing supports 1/2/3-byte hops; trace uses 1/2/4/8-byte hops, so
// stored 3-byte out-paths are down-converted to 2-byte hops.
func TracePathFromOutPath(encoded byte, raw []byte) ([]byte, int, byte, error) {
	pathHashSize := HashSizeFromPathMeta(encoded)
	hopCount := HopCountFromPathMeta(encoded)
	path := PathBytes(encoded, raw)
	if len(path) == 0 {
		return nil, 0, 0, fmt.Errorf("empty out-path")
	}

	switch pathHashSize {
	case 1, 2, 4, 8:
		flags, err := TraceFlagsFromHashSize(pathHashSize)
		if err != nil {
			return nil, 0, 0, err
		}
		return path, pathHashSize, flags, nil
	case 3:
		tracePath := make([]byte, 0, hopCount*2)
		for i := 0; i < hopCount; i++ {
			off := i * 3
			if off+2 > len(path) {
				break
			}
			tracePath = append(tracePath, path[off:off+2]...)
		}
		if len(tracePath) == 0 {
			return nil, 0, 0, fmt.Errorf("empty 3-byte out-path")
		}
		flags, err := TraceFlagsFromHashSize(2)
		if err != nil {
			return nil, 0, 0, err
		}
		return tracePath, 2, flags, nil
	default:
		return nil, 0, 0, fmt.Errorf("unsupported out-path hash size %d bytes", pathHashSize)
	}
}

// TraceFlagsFromPathMeta maps contact out-path metadata to trace flags.
func TraceFlagsFromPathMeta(encoded byte) (byte, error) {
	hashSize := HashSizeFromPathMeta(encoded)
	if hashSize == 3 {
		hashSize = 2
	}
	return TraceFlagsFromHashSize(hashSize)
}

// IsHexTraceTarget reports whether s looks like a hex trace path rather than a
// contact name. Comma- or space-separated hops are allowed.
func IsHexTraceTarget(s string) bool {
	fields := strings.FieldsFunc(strings.TrimSpace(s), func(r rune) bool { return r == ',' || r == ' ' })
	if len(fields) == 0 {
		return false
	}
	for _, f := range fields {
		if f == "" || len(f)%2 != 0 {
			return false
		}
		for _, r := range f {
			if (r < '0' || r > '9') && (r < 'a' || r > 'f') && (r < 'A' || r > 'F') {
				return false
			}
		}
	}
	return true
}

// ParsePath parses a comma- or space-separated list of hex hashes. Every hop
// must use the same hash width (2, 4, 8, or 16 hex digits for 1/2/4/8 bytes).
func ParsePath(s string) ([]byte, int, error) {
	fields := strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ' ' })
	if len(fields) == 0 {
		return nil, 0, fmt.Errorf("empty path")
	}

	hashSize := len(fields[0]) / 2
	if hashSize == 0 || len(fields[0])%2 != 0 {
		return nil, 0, fmt.Errorf("invalid path hash %q", fields[0])
	}
	if _, err := TraceFlagsFromHashSize(hashSize); err != nil {
		return nil, 0, err
	}

	path := make([]byte, 0, len(fields)*hashSize)
	for _, f := range fields {
		if len(f) != hashSize*2 {
			return nil, 0, fmt.Errorf("path hash %q has width %d hex digits, expected %d", f, len(f), hashSize*2)
		}
		b, err := hex.DecodeString(f)
		if err != nil {
			return nil, 0, fmt.Errorf("invalid path hash %q: %w", f, err)
		}
		path = append(path, b...)
	}
	if len(path) > MaxPathSize {
		return nil, 0, fmt.Errorf("path exceeds %d bytes", MaxPathSize)
	}
	return path, hashSize, nil
}

// Split splits raw path bytes into per-hop hashes.
func Split(path []byte, hashSize int) [][]byte {
	if hashSize <= 0 || len(path) == 0 || len(path)%hashSize != 0 {
		return nil
	}
	hops := make([][]byte, 0, len(path)/hashSize)
	for i := 0; i < len(path); i += hashSize {
		hop := append([]byte(nil), path[i:i+hashSize]...)
		hops = append(hops, hop)
	}
	return hops
}

// FormatHop renders a hop hash as lowercase hex.
func FormatHop(hop []byte) string {
	return hex.EncodeToString(hop)
}
