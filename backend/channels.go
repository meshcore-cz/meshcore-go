package backend

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"

	meshcore "github.com/meshcore-cz/meshcore-go"
)

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

// isPrivateChannel reports whether a channel's key is NOT derivable from its
// name. Public and hashtag channels derive their key from the name (the name is
// effectively the key, so anyone who knows the name can read them); a channel
// whose key does not match the name derivation is private (a random or
// out-of-band key). Unknown (empty) secrets are treated as not private.
func isPrivateChannel(name string, secret []byte) bool {
	if len(secret) == 0 {
		return false
	}
	return !bytes.Equal(secret, meshcore.DeriveChannelSecret(name))
}
