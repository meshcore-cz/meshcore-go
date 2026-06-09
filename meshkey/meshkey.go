// Package meshkey provides Curve25519 (X25519) key generation and parsing for
// MeshCore node identities. Key functionality has moved to meshpkt; this package
// re-exports it for backward compatibility.
package meshkey

import "github.com/meshcore-cz/meshcore-go/meshpkt"

// KeyPair holds a Curve25519 identity keypair.
type KeyPair = meshpkt.KeyPair

// Generate creates a fresh random Curve25519 keypair suitable for use as a
// MeshCore node identity.
func Generate() (meshpkt.KeyPair, error) { return meshpkt.Generate() }

// ParsePublicKey validates and decodes a hex-encoded 32-byte X25519 public key.
func ParsePublicKey(hexKey string) ([]byte, error) { return meshpkt.ParsePublicKey(hexKey) }

// ParsePrivateKey validates and decodes a hex-encoded 32-byte X25519 private scalar.
func ParsePrivateKey(hexKey string) ([]byte, error) { return meshpkt.ParsePrivateKey(hexKey) }

// PublicKeyFromPrivate derives the public key from a hex-encoded private scalar.
func PublicKeyFromPrivate(hexPriv string) (string, error) {
	return meshpkt.PublicKeyFromPrivate(hexPriv)
}

// SharedSecret performs X25519 ECDH and returns the 32-byte shared secret.
// Callers typically use only the first 16 bytes as an AES-128 key.
func SharedSecret(privHex, pubHex string) ([]byte, error) {
	return meshpkt.SharedSecret(privHex, pubHex)
}
