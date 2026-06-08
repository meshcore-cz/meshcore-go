// Package meshkey provides Curve25519 (X25519) key generation and parsing for
// MeshCore node identities. MeshCore uses X25519 for ECDH: the 32-byte private
// scalar derives the 32-byte public key, and both are represented as lowercase
// hex strings throughout the companion protocol and SDK.
package meshkey

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

// KeyPair holds a Curve25519 identity keypair.
type KeyPair struct {
	// PublicKey is the hex-encoded 32-byte X25519 public key.
	PublicKey string
	// PrivateKey is the hex-encoded 32-byte X25519 private scalar.
	PrivateKey string
}

// Generate creates a fresh random Curve25519 keypair suitable for use as a
// MeshCore node identity.
func Generate() (KeyPair, error) {
	priv, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return KeyPair{}, fmt.Errorf("meshkey: generate: %w", err)
	}
	return KeyPair{
		PublicKey:  hex.EncodeToString(priv.PublicKey().Bytes()),
		PrivateKey: hex.EncodeToString(priv.Bytes()),
	}, nil
}

// ParsePublicKey validates and decodes a hex-encoded 32-byte X25519 public key.
func ParsePublicKey(hexKey string) ([]byte, error) {
	b, err := hex.DecodeString(hexKey)
	if err != nil {
		return nil, fmt.Errorf("meshkey: invalid public key: %w", err)
	}
	if len(b) != 32 {
		return nil, fmt.Errorf("meshkey: public key must be 32 bytes, got %d", len(b))
	}
	if _, err := ecdh.X25519().NewPublicKey(b); err != nil {
		return nil, fmt.Errorf("meshkey: invalid public key: %w", err)
	}
	return b, nil
}

// ParsePrivateKey validates and decodes a hex-encoded 32-byte X25519 private scalar.
func ParsePrivateKey(hexKey string) ([]byte, error) {
	b, err := hex.DecodeString(hexKey)
	if err != nil {
		return nil, fmt.Errorf("meshkey: invalid private key: %w", err)
	}
	if len(b) != 32 {
		return nil, fmt.Errorf("meshkey: private key must be 32 bytes, got %d", len(b))
	}
	if _, err := ecdh.X25519().NewPrivateKey(b); err != nil {
		return nil, fmt.Errorf("meshkey: invalid private key: %w", err)
	}
	return b, nil
}

// PublicKeyFromPrivate derives the public key from a hex-encoded private scalar.
func PublicKeyFromPrivate(hexPriv string) (string, error) {
	b, err := ParsePrivateKey(hexPriv)
	if err != nil {
		return "", err
	}
	priv, err := ecdh.X25519().NewPrivateKey(b)
	if err != nil {
		return "", fmt.Errorf("meshkey: %w", err)
	}
	return hex.EncodeToString(priv.PublicKey().Bytes()), nil
}
