package streamclaims

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
)

// nonceBytes is the length in bytes of the raw random nonce before
// base64url encoding. 128 bits of entropy is sufficient to make collisions
// cosmologically improbable even at sustained high-volume minting.
const nonceBytes = 16

// GenerateNonce returns a cryptographically random, base64url-encoded nonce
// string suitable for use in [StreamClaims.Nonce].
//
// The returned string is URL-safe and has no padding characters, making it
// safe to embed in query parameters and HTTP headers without further encoding.
//
// Callers MUST NOT cache or re-use a nonce. Each call to [GenerateNonce] is
// guaranteed to return a distinct value with overwhelming probability (2^128
// collision space).
func GenerateNonce() (string, error) {
	b := make([]byte, nonceBytes)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("streamclaims: generate nonce: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
