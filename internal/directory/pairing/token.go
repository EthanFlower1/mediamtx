package pairing

import (
	"crypto"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/hkdf"
)

// TokenTTL is the fixed lifetime of a freshly issued PairingToken.
const TokenTTL = 15 * time.Minute

// hkdfInfo is the domain separator for the sub-key derived from the root
// ed25519 signing key. This scopes signatures to the pairing domain.
const hkdfInfo = "kaivue-pairing-token-v1"

// UserID is the opaque identifier of the admin that generated the token.
// It matches the shape used across the cloud auth stack.
type UserID string

// PairingToken is a one-time, TTL-bound credential bundling everything a new
// Recorder needs to join a Directory's cluster. It is distinct from
// StreamClaims (KAI-256) and is never a JWT.
//
// Token blast radius: a leaked token grants only "join as one Recorder."
// It is single-use and expires in ~15 minutes.
type PairingToken struct {
	// TokenID is the stable UUID used for lookup, audit, and deduplication.
	TokenID string `json:"token_id"`

	// DirectoryEndpoint is the base URL the Recorder should dial for all
	// subsequent management traffic, e.g. "https://dir.customer.local:8443".
	DirectoryEndpoint string `json:"directory_endpoint"`

	// HeadscalePreAuthKey is the single-use tailnet pre-auth key minted by
	// the embedded Headscale coordinator (KAI-240). The Recorder presents it
	// during tsnet.Server startup so it joins the site mesh automatically.
	HeadscalePreAuthKey string `json:"headscale_pre_auth_key"`

	// StepCAFingerprint is the lowercase hex SHA-256 of the site root CA DER.
	// The Recorder uses it for trust-on-first-use (TOFU) when contacting
	// step-ca during its own certificate enrollment.
	StepCAFingerprint string `json:"step_ca_fingerprint"`

	// StepCAEnrollToken is the JWK provisioner one-time enrollment token that
	// allows the Recorder to obtain its first mutual-TLS certificate from the
	// embedded step-ca (KAI-241).
	StepCAEnrollToken string `json:"step_ca_enroll_token"`

	// DirectoryFingerprint is the lowercase hex SHA-256 of the Directory's
	// own TLS serving certificate. The Recorder uses it to verify the
	// Directory before sending its enrollment payload.
	DirectoryFingerprint string `json:"directory_fingerprint"`

	// SuggestedRoles is the hint set the Directory proposes for this
	// Recorder, e.g. ["recorder"]. The final assignment is always made by
	// an admin after check-in.
	SuggestedRoles []string `json:"suggested_roles"`

	// ExpiresAt is the absolute UTC time after which the token MUST be
	// rejected even if status is still 'pending'.
	ExpiresAt time.Time `json:"expires_at"`

	// SignedBy is the UserID of the admin that triggered token generation.
	SignedBy UserID `json:"signed_by"`

	// CloudTenantBinding is an optional opaque handle that ties this
	// enrollment to a cloud-managed tenant. Empty for fully air-gapped sites.
	CloudTenantBinding string `json:"cloud_tenant_binding,omitempty"`
}

// Encode serializes and signs the token using the supplied ed25519 private key.
//
// The output is:
//
//	base64url(JSON payload) + "." + base64url(Ed25519 signature over the payload bytes)
//
// The signing key is typically a sub-key derived via deriveSigningKey from the
// site root private key — callers should NOT pass the raw root key directly;
// use NewSigningKey instead.
func (t *PairingToken) Encode(signingKey ed25519.PrivateKey) (string, error) {
	if len(signingKey) == 0 {
		return "", errors.New("pairing: encode: nil signing key")
	}
	payload, err := json.Marshal(t)
	if err != nil {
		return "", fmt.Errorf("pairing: encode marshal: %w", err)
	}
	encodedPayload := base64.RawURLEncoding.EncodeToString(payload)

	sig, err := signingKey.Sign(rand.Reader, []byte(encodedPayload), crypto.Hash(0))
	if err != nil {
		return "", fmt.Errorf("pairing: encode sign: %w", err)
	}
	encodedSig := base64.RawURLEncoding.EncodeToString(sig)
	return encodedPayload + "." + encodedSig, nil
}

// Decode parses and verifies a token string produced by Encode.
//
// It rejects:
//   - Malformed blobs (wrong number of segments, bad base64, bad JSON).
//   - Tokens with invalid signatures (cross-directory rejection).
//   - Expired tokens (ExpiresAt before now).
//
// Callers MUST also check the database status (pending/redeemed/expired) via
// the Store — Decode only validates the cryptographic integrity and TTL.
func Decode(token string, verifyKey crypto.PublicKey) (*PairingToken, error) {
	parts := strings.SplitN(token, ".", 2)
	if len(parts) != 2 {
		return nil, errors.New("pairing: decode: malformed token (expected payload.sig)")
	}
	rawPayload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, fmt.Errorf("pairing: decode: payload base64: %w", err)
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("pairing: decode: sig base64: %w", err)
	}

	edPub, ok := verifyKey.(ed25519.PublicKey)
	if !ok {
		return nil, fmt.Errorf("pairing: decode: verifyKey must be ed25519.PublicKey, got %T", verifyKey)
	}

	// Verify signature over the raw payload segment (the encoded bytes), not
	// the decoded JSON. This prevents canonicalization attacks.
	if !ed25519.Verify(edPub, []byte(parts[0]), sig) {
		return nil, errors.New("pairing: decode: signature verification failed")
	}

	var pt PairingToken
	if err := json.Unmarshal(rawPayload, &pt); err != nil {
		return nil, fmt.Errorf("pairing: decode: unmarshal: %w", err)
	}
	if pt.ExpiresAt.Before(time.Now().UTC()) {
		return nil, fmt.Errorf("pairing: decode: token expired at %s", pt.ExpiresAt.Format(time.RFC3339))
	}
	return &pt, nil
}

// NewSigningKey derives a domain-scoped ed25519 signing key from the site
// root private key via HKDF-SHA256. The output is deterministic for a given
// root key so it requires no additional key-material storage.
//
// This is the preferred way to obtain the key passed to PairingToken.Encode.
// Pass the corresponding public key to Decode.
func NewSigningKey(rootKey ed25519.PrivateKey) (ed25519.PrivateKey, error) {
	if len(rootKey) == 0 {
		return nil, errors.New("pairing: NewSigningKey: nil root key")
	}
	// Use the raw 32-byte seed (first half of the 64-byte ed25519 private key)
	// as HKDF input keying material.
	seed := []byte(rootKey[:32])
	h := hkdf.New(sha256.New, seed, nil, []byte(hkdfInfo))
	derived := make([]byte, ed25519.SeedSize)
	if _, err := h.Read(derived); err != nil {
		return nil, fmt.Errorf("pairing: NewSigningKey: hkdf: %w", err)
	}
	return ed25519.NewKeyFromSeed(derived), nil
}

// VerifyPublicKey returns the ed25519.PublicKey from a signing key produced by
// NewSigningKey. This is the key to pass to Decode for a specific Directory.
func VerifyPublicKey(signingKey ed25519.PrivateKey) ed25519.PublicKey {
	return signingKey.Public().(ed25519.PublicKey)
}
