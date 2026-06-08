package meshkey

import (
	"strings"
	"testing"
)

func TestGenerate(t *testing.T) {
	kp, err := Generate()
	if err != nil {
		t.Fatal(err)
	}
	if len(kp.PublicKey) != 64 {
		t.Fatalf("public key hex len = %d, want 64", len(kp.PublicKey))
	}
	if len(kp.PrivateKey) != 64 {
		t.Fatalf("private key hex len = %d, want 64", len(kp.PrivateKey))
	}
	// Two calls must produce different keys.
	kp2, _ := Generate()
	if kp.PublicKey == kp2.PublicKey {
		t.Fatal("two Generate() calls returned the same public key")
	}
}

func TestPublicKeyFromPrivate(t *testing.T) {
	kp, err := Generate()
	if err != nil {
		t.Fatal(err)
	}
	got, err := PublicKeyFromPrivate(kp.PrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	if got != kp.PublicKey {
		t.Fatalf("derived public key %q != original %q", got, kp.PublicKey)
	}
}

func TestParsePublicKeyErrors(t *testing.T) {
	if _, err := ParsePublicKey("notahex"); err == nil {
		t.Fatal("expected error for non-hex input")
	}
	if _, err := ParsePublicKey(strings.Repeat("00", 16)); err == nil {
		t.Fatal("expected error for 16-byte key")
	}
}

func TestParsePrivateKeyErrors(t *testing.T) {
	if _, err := ParsePrivateKey("notahex"); err == nil {
		t.Fatal("expected error for non-hex input")
	}
	if _, err := ParsePrivateKey(strings.Repeat("00", 16)); err == nil {
		t.Fatal("expected error for 16-byte key")
	}
}

func TestRoundTrip(t *testing.T) {
	kp, _ := Generate()

	pub, err := ParsePublicKey(kp.PublicKey)
	if err != nil {
		t.Fatalf("ParsePublicKey: %v", err)
	}
	if len(pub) != 32 {
		t.Fatalf("parsed public key len = %d, want 32", len(pub))
	}

	priv, err := ParsePrivateKey(kp.PrivateKey)
	if err != nil {
		t.Fatalf("ParsePrivateKey: %v", err)
	}
	if len(priv) != 32 {
		t.Fatalf("parsed private key len = %d, want 32", len(priv))
	}
}
