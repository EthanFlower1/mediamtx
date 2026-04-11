package federation

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/hkdf"
)

// PeerEnrollmentTokenTTL is the default lifetime of a peer enrollment token.
const PeerEnrollmentTokenTTL = 30 * time.Minute

// hkdfPeerTokenInfo is the domain separator for the sub-key used to sign
// peer enrollment tokens. Scoped to the federation-peer-token domain.
const hkdfPeerTokenInfo = "kaivue-federation-peer-token-v1"

// PeerEnrollmentToken is a one-time, TTL-bound credential that a founding
// Directory issues to a joining peer Directory. It contains everything the
// peer needs to establish trust and join the federation.
//
// This is the contract type that KAI-269 (Federation pairing token) builds on.
type PeerEnrollmentToken struct {
	// TokenID is a stable UUID for lookup, audit, and deduplication.
	TokenID string `json:"token_id"`

	// FederationCAFingerprint is the lowercase hex SHA-256 of the federation
	// root CA's DER encoding. The joining peer uses this for trust-on-first-use.
	FederationCAFingerprint string `json:"federation_ca_fingerprint"`

	// FederationCARootPEM is the PEM-encoded federation root certificate.
	// The peer installs this into its trust store on first enrollment.
	FederationCARootPEM string `json:"federation_ca_root_pem"`

	// FoundingDirectoryEndpoint is the URL the joining peer should contact
	// to complete enrollment, e.g. "https://dir.site-a.kaivue.local:8443".
	FoundingDirectoryEndpoint string `json:"founding_directory_endpoint"`

	// PeerSiteID is the site ID assigned to the joining peer. The founding
	// Directory pre-allocates this so the peer's cert CN is deterministic.
	PeerSiteID string `json:"peer_site_id"`

	// ExpiresAt is the absolute UTC time after which the token MUST be rejected.
	ExpiresAt time.Time `json:"expires_at"`

	// IssuedBy identifies which site/admin issued the token.
	IssuedBy string `json:"issued_by"`
}

// MintPeerEnrollmentToken creates a signed peer enrollment token containing
// the federation CA fingerprint and root PEM. The joining Directory uses
// this to trust-on-first-use the federation root and enroll.
//
// This method is the primary contract surface for KAI-269.
func (c *FederationCA) MintPeerEnrollmentToken(
	foundingEndpoint string,
	peerSiteID string,
	issuedBy string,
	ttl time.Duration,
) (string, error) {
	if foundingEndpoint == "" {
		return "", errors.New("federation: MintPeerEnrollmentToken: founding endpoint required")
	}
	if peerSiteID == "" {
		return "", errors.New("federation: MintPeerEnrollmentToken: peer site ID required")
	}
	if ttl <= 0 {
		ttl = PeerEnrollmentTokenTTL
	}

	c.mu.RLock()
	fp := c.fingerprint
	rootPEM := string(c.rootCertPEM)
	rootKey := c.rootKey
	siteID := c.cfg.SiteID
	c.mu.RUnlock()

	if rootKey == nil {
		return "", errors.New("federation: MintPeerEnrollmentToken: CA not initialized")
	}
	if issuedBy == "" {
		issuedBy = siteID
	}

	signingKey, err := derivePeerTokenSigningKey(rootKey)
	if err != nil {
		return "", err
	}

	now := c.now().UTC()
	tok := PeerEnrollmentToken{
		TokenID:                   fmt.Sprintf("fed-%s", peerSiteID),
		FederationCAFingerprint:   fp,
		FederationCARootPEM:       rootPEM,
		FoundingDirectoryEndpoint: foundingEndpoint,
		PeerSiteID:                peerSiteID,
		ExpiresAt:                 now.Add(ttl),
		IssuedBy:                  issuedBy,
	}

	return tok.Encode(signingKey)
}

// DecodePeerEnrollmentToken parses and verifies a peer enrollment token.
// The verifyKey should be the public key from DerivePeerTokenVerifyKey.
func DecodePeerEnrollmentToken(token string, verifyKey ed25519.PublicKey) (*PeerEnrollmentToken, error) {
	parts := strings.SplitN(token, ".", 2)
	if len(parts) != 2 {
		return nil, errors.New("federation: decode: malformed token (expected payload.sig)")
	}
	rawPayload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, fmt.Errorf("federation: decode: payload base64: %w", err)
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("federation: decode: sig base64: %w", err)
	}

	if !ed25519.Verify(verifyKey, []byte(parts[0]), sig) {
		return nil, errors.New("federation: decode: signature verification failed")
	}

	var pet PeerEnrollmentToken
	if err := json.Unmarshal(rawPayload, &pet); err != nil {
		return nil, fmt.Errorf("federation: decode: unmarshal: %w", err)
	}
	if pet.ExpiresAt.Before(time.Now().UTC()) {
		return nil, fmt.Errorf("federation: decode: token expired at %s", pet.ExpiresAt.Format(time.RFC3339))
	}
	return &pet, nil
}

// Encode serializes and signs the token using the supplied ed25519 private key.
// Output format: base64url(JSON) + "." + base64url(Ed25519 signature).
func (t *PeerEnrollmentToken) Encode(signingKey ed25519.PrivateKey) (string, error) {
	if len(signingKey) == 0 {
		return "", errors.New("federation: encode: nil signing key")
	}
	payload, err := json.Marshal(t)
	if err != nil {
		return "", fmt.Errorf("federation: encode marshal: %w", err)
	}
	encodedPayload := base64.RawURLEncoding.EncodeToString(payload)
	sig := ed25519.Sign(signingKey, []byte(encodedPayload))
	encodedSig := base64.RawURLEncoding.EncodeToString(sig)
	return encodedPayload + "." + encodedSig, nil
}

// derivePeerTokenSigningKey derives a domain-scoped ed25519 signing key from
// the federation root private key via HKDF-SHA256.
func derivePeerTokenSigningKey(rootKey ed25519.PrivateKey) (ed25519.PrivateKey, error) {
	if len(rootKey) == 0 {
		return nil, errors.New("federation: derivePeerTokenSigningKey: nil root key")
	}
	seed := []byte(rootKey[:32])
	h := hkdf.New(sha256.New, seed, nil, []byte(hkdfPeerTokenInfo))
	derived := make([]byte, ed25519.SeedSize)
	if _, err := h.Read(derived); err != nil {
		return nil, fmt.Errorf("federation: derivePeerTokenSigningKey: hkdf: %w", err)
	}
	return ed25519.NewKeyFromSeed(derived), nil
}

// DerivePeerTokenVerifyKey returns the ed25519 public key that can verify
// peer enrollment tokens signed by a federation root key. This is what the
// joining peer uses to verify the token before trusting the federation CA.
func DerivePeerTokenVerifyKey(rootKey ed25519.PrivateKey) (ed25519.PublicKey, error) {
	signingKey, err := derivePeerTokenSigningKey(rootKey)
	if err != nil {
		return nil, err
	}
	return signingKey.Public().(ed25519.PublicKey), nil
}
