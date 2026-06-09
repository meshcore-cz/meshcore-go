package meshpkt

import "crypto/sha256"

// ChannelSecretLen is the length of a channel pre-shared key.
const ChannelSecretLen = 16

// DeriveChannelSecret derives the pre-shared key for a name-only ("hashtag")
// channel from its name: the first 16 bytes of SHA-256(name). This lets
// peers share a channel by agreeing only on a name.
//
// NOTE: this derivation is firmware-derived and not yet hardware-verified.
// Confirm interoperability with other MeshCore clients before relying on it.
func DeriveChannelSecret(name string) []byte {
	sum := sha256.Sum256([]byte(name))
	out := make([]byte, ChannelSecretLen)
	copy(out, sum[:ChannelSecretLen])
	return out
}

// ChannelHash returns the 1-byte routing hash the firmware uses to match
// incoming packets to a channel slot without decrypting every payload:
// SHA256(secret[:16])[0]. Pass the output of DeriveChannelSecret or the
// Secret field from a Channel returned by Client.Channels().
func ChannelHash(secret []byte) byte {
	sum := sha256.Sum256(secret[:ChannelSecretLen])
	return sum[0]
}
