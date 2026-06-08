package backend

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
)

// publicChannelSecret is the well-known pre-shared key of MeshCore's default
// public channel. A channel whose secret differs from this is treated as
// private. This is the documented default; if a firmware uses a different
// public key the private flag is the only thing affected.
var publicChannelSecret, _ = base64.StdEncoding.DecodeString("izOH6cXN6mrJ5e26oRXNcg==")

// channelKey derives a stable, collision-resistant universal identifier for a
// channel from its pre-shared key. Two devices configured for the same channel
// share the same secret and therefore the same key, independent of the local
// slot index or name. Returns "" when no secret is available.
func channelKey(secret []byte) string {
	if len(secret) == 0 {
		return ""
	}
	sum := sha256.Sum256(secret)
	return hex.EncodeToString(sum[:])
}

// isPrivateChannel reports whether a channel secret differs from the well-known
// public channel key. Unknown (empty) secrets are treated as not private.
func isPrivateChannel(secret []byte) bool {
	if len(secret) == 0 {
		return false
	}
	return !bytes.Equal(secret, publicChannelSecret)
}
