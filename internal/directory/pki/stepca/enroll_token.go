package stepca

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// EnrollmentTokenTTL is the lifetime of a JWK provisioner enrollment token.
// It is deliberately short because the token is one-time and embedded in a
// PairingToken which itself expires in 15 minutes.
const EnrollmentTokenTTL = 10 * time.Minute

// enrollmentClaims are the JWT claims for a step-ca JWK provisioner
// one-shot enrollment token (ACME-like, not RFC 8555).
//
// The required set for step-ca's /1.0/sign endpoint (JWK provisioner path):
//
//	iss  — must match the provisioner name configured on the CA
//	aud  — must be the full CA /sign URL (e.g. https://ca.site.local:9000/1.0/sign)
//	sub  — the SANS to embed in the issued cert (first entry used as CN)
//	jti  — one-time nonce; step-ca rejects replays
//	iat / exp — standard
//	sans — extension claim listing the requested SANs
type enrollmentClaims struct {
	SANS []string `json:"sans,omitempty"`
	jwt.RegisteredClaims
}

// MintEnrollmentToken produces a short-lived, signed JWK provisioner enrollment
// token that a new Recorder can present to step-ca's /1.0/sign endpoint to
// obtain its first mTLS leaf certificate.
//
// audience is the full CA sign URL, e.g. "https://ca.site.local:9000/1.0/sign".
// issuer should be the JWK provisioner name configured on the step-ca instance
// (typically "kaivue-pairing").
// sans are the SAN DNS names / IPs to embed in the issued cert.
//
// The token is signed with the site root ed25519 private key. Step-ca can
// verify it using the corresponding public JWK (which it derives from the same
// root during bootstrap). In a production step-ca deployment the signing key
// would be a dedicated provisioner key; for our embedded CA the root key
// serves this role directly since we own and manage the CA.
func (c *ClusterCA) MintEnrollmentToken(audience, issuer string, sans []string, ttl time.Duration) (string, error) {
	if ttl <= 0 {
		ttl = EnrollmentTokenTTL
	}

	c.mu.RLock()
	key := c.rootKey
	c.mu.RUnlock()

	if key == nil {
		return "", fmt.Errorf("stepca: MintEnrollmentToken: CA not initialized")
	}
	if audience == "" {
		return "", fmt.Errorf("stepca: MintEnrollmentToken: audience required")
	}
	if issuer == "" {
		issuer = "kaivue-pairing"
	}

	now := c.now().UTC()
	claims := enrollmentClaims{
		SANS: sans,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    issuer,
			Subject:   firstOrEmpty(sans),
			Audience:  jwt.ClaimStrings{audience},
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
			// jti prevents replay; step-ca caches seen jti values within
			// the token's validity window.
			ID: uuid.NewString(),
		},
	}

	tok := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)
	signed, err := tok.SignedString(key)
	if err != nil {
		return "", fmt.Errorf("stepca: MintEnrollmentToken: sign: %w", err)
	}
	return signed, nil
}

func firstOrEmpty(ss []string) string {
	if len(ss) == 0 {
		return ""
	}
	return ss[0]
}
