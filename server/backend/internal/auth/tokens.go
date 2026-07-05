// Package auth holds the token and password primitives shared by the device,
// MCP, and dashboard auth paths.
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"encoding/hex"
	"strings"
)

// lowerBase32 is base32 without padding, lowercased — url/log friendly ids.
var lowerBase32 = base32.StdEncoding.WithPadding(base32.NoPadding)

// randToken returns n bytes of randomness as a lowercase base32 string.
func randToken(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic("auth: crypto/rand failed: " + err.Error())
	}
	return strings.ToLower(lowerBase32.EncodeToString(b))
}

// NewID makes a short unique identifier with the given prefix, e.g.
// NewID("dev") -> "dev_ktp4h2n9...". Used for account/device/session ids.
func NewID(prefix string) string {
	return prefix + "_" + randToken(10)
}

// NewSecret makes a longer, high-entropy secret token with the given prefix,
// e.g. NewSecret("abd_mcp") -> "abd_mcp_<32 base32 chars>". Returned to the user
// once; only its hash is stored.
func NewSecret(prefix string) string {
	return prefix + "_" + randToken(20)
}

// HashToken is the one-way function applied to every secret before storage and
// on every lookup. Tokens are high-entropy random strings, so a fast hash is
// sufficient (unlike passwords) and lets us index by hash for O(1) lookup.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
