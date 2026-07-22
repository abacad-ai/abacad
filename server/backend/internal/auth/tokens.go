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

// randLetters returns n random lowercase letters (a-z only, no digits). It uses
// rejection sampling — bytes >= 234 (the largest multiple of 26 below 256) are
// discarded — so every letter is equally likely, with no modulo bias.
func randLetters(n int) string {
	out := make([]byte, 0, n)
	buf := make([]byte, n) // refilled as needed; usually enough in one pass
	for len(out) < n {
		if _, err := rand.Read(buf); err != nil {
			panic("auth: crypto/rand failed: " + err.Error())
		}
		for _, x := range buf {
			if x >= 234 {
				continue
			}
			out = append(out, 'a'+x%26)
			if len(out) == n {
				break
			}
		}
	}
	return string(out)
}

// NewID makes a short unique identifier with the given prefix, e.g.
// NewID("acc") -> "acc_ktp4h2n9...". Used for account, session, and similar
// prefixed ids. Devices are the exception: they use NewDeviceID (no prefix).
func NewID(prefix string) string {
	return prefix + "_" + randToken(10)
}

// NewDeviceID makes a bare, prefix-free identifier: a random string of lowercase
// letters only — no type tag, no dashes, no digits (e.g. "ktphznmqvxbdfgwr").
// Devices use this because their id shows up in URLs (/devices/<id>), in ssh
// hostnames, and in the agent's device selection — places where a clean,
// letters-only token reads best. 16 letters carries ~75 bits of entropy, on par
// with NewID's 10 base32 bytes.
func NewDeviceID() string {
	return randLetters(16)
}

// userCodeAlphabet is 30 characters with the visually ambiguous ones removed
// (no 0/O, 1/I/L) so a human can read a pairing code off a terminal and type it
// into the browser without confusion.
const userCodeAlphabet = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"

// NewUserCode returns a short, human-typable device-pairing code like
// "WXYZ-2K7M": two dash-separated groups of four from an ambiguity-free 30-char
// alphabet (~39 bits). It is not a secret on its own — approval also requires an
// authenticated browser session — but is high-entropy enough that a pending code
// can't be guessed within its short lifetime. Rejection sampling (drop bytes
// >= 240, the largest multiple of 30 below 256) keeps every character equally
// likely, matching randLetters' approach.
func NewUserCode() string {
	const groups, per = 2, 4
	out := make([]byte, 0, groups*per+1)
	buf := make([]byte, groups*per)
	for len(out) < groups*per+1 {
		if _, err := rand.Read(buf); err != nil {
			panic("auth: crypto/rand failed: " + err.Error())
		}
		for _, x := range buf {
			if x >= 240 {
				continue
			}
			if len(out) == per {
				out = append(out, '-') // group separator after the first four
			}
			out = append(out, userCodeAlphabet[x%30])
			if len(out) == groups*per+1 {
				break
			}
		}
	}
	return string(out)
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
