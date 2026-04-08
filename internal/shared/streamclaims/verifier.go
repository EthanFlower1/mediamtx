package streamclaims

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/MicahParks/keyfunc/v3"
	"github.com/bluenviron/mediamtx/internal/shared/auth"
	"github.com/golang-jwt/jwt/v5"
)

// Verifier validates RS256 stream tokens produced by an [Issuer].
//
// Construct one via [NewVerifier]. The [Verifier] is safe for concurrent use
// by multiple goroutines. It does NOT perform nonce-uniqueness checks — that
// is the responsibility of KAI-257 (nonce bloom filter). Callers MUST chain
// nonce checks after a successful [Verifier.Verify] call.
//
// The [Verifier] fails closed: any parsing, signature, audience, issuer,
// expiry, or structural error results in a non-nil error and a nil result.
// There is no partial success path.
type Verifier struct {
	keyfunc  jwt.Keyfunc
	issuerID string
	audience string
	log      *slog.Logger
}

// VerifierOption is a functional option for [NewVerifier].
type VerifierOption func(*Verifier)

// WithVerifierLogger sets the structured logger used by the [Verifier].
func WithVerifierLogger(l *slog.Logger) VerifierOption {
	return func(v *Verifier) { v.log = l }
}

// NewVerifier constructs a [Verifier] from a raw JWKS JSON document.
//
// jwksJSON is the raw bytes of the /.well-known/jwks.json response (obtained
// from [Issuer.PublicKeySet] in tests, or via an HTTP fetch in production).
// issuerID and audience MUST match the values used by the [Issuer] that
// produced the tokens being verified.
//
// Key rotation: construct a new [Verifier] with the updated JWKS whenever the
// cloud rotates its signing key. The existing verifier continues to verify
// tokens signed with the old key until it is GC'd — callers should implement
// a brief overlap window during rotation.
func NewVerifier(jwksJSON json.RawMessage, issuerID, audience string, opts ...VerifierOption) (*Verifier, error) {
	if len(jwksJSON) == 0 {
		return nil, fmt.Errorf("streamclaims: NewVerifier: jwksJSON must not be empty")
	}
	if issuerID == "" {
		return nil, fmt.Errorf("streamclaims: NewVerifier: issuerID must not be empty")
	}
	if audience == "" {
		return nil, fmt.Errorf("streamclaims: NewVerifier: audience must not be empty")
	}

	jwkSet, err := keyfunc.NewJWKSetJSON(jwksJSON)
	if err != nil {
		return nil, fmt.Errorf("streamclaims: NewVerifier: failed to parse JWKS: %w", err)
	}

	v := &Verifier{
		keyfunc:  jwkSet.Keyfunc,
		issuerID: issuerID,
		audience: audience,
		log:      slog.Default(),
	}
	for _, o := range opts {
		o(v)
	}
	return v, nil
}

// Verify parses and validates a signed stream token string. On success it
// returns the decoded [StreamClaims]; on any failure it returns nil and a
// descriptive error.
//
// Checks performed (in order; fail on first failure — fail closed):
//  1. JWT signature valid (RS256, key from JWKS)
//  2. Issuer ("iss") matches
//  3. Audience ("aud") contains the expected value
//  4. Token is not expired ("exp")
//  5. Not-before ("nbf") is not in the future
//  6. Required StreamClaims fields are non-empty
//  7. Kind bitfield is non-zero
//  8. Nonce is non-empty (uniqueness check delegated to KAI-257)
//  9. PlaybackRange present iff Kind includes PLAYBACK
//  10. ExpiresAt is within [MaxTTL] of IssuedAt (guards against forgery of
//     over-long TTLs even if the signature is valid)
func (v *Verifier) Verify(tokenString string) (*StreamClaims, error) {
	if tokenString == "" {
		return nil, fmt.Errorf("streamclaims: Verify: token must not be empty")
	}

	var jc jwtClaims
	token, err := jwt.ParseWithClaims(
		tokenString,
		&jc,
		v.keyfunc,
		jwt.WithIssuer(v.issuerID),
		jwt.WithAudience(v.audience),
		jwt.WithLeeway(0), // no clock-skew tolerance — fail closed
		jwt.WithValidMethods([]string{jwt.SigningMethodRS256.Alg()}),
		jwt.WithExpirationRequired(),
	)
	if err != nil {
		return nil, fmt.Errorf("streamclaims: Verify: %w", err)
	}
	if !token.Valid {
		return nil, fmt.Errorf("streamclaims: Verify: token is not valid")
	}

	sc := jc.SC

	// Structural checks — fail closed on missing required fields.
	switch {
	case sc.UserID == "":
		return nil, fmt.Errorf("streamclaims: Verify: missing uid claim")
	case sc.TenantID == "" || sc.TenantType == "":
		return nil, fmt.Errorf("streamclaims: Verify: missing or incomplete tnt claim")
	case sc.CameraID == "":
		return nil, fmt.Errorf("streamclaims: Verify: missing cam claim")
	case sc.RecorderID == "":
		return nil, fmt.Errorf("streamclaims: Verify: missing rec claim")
	case sc.DirectoryID == "":
		return nil, fmt.Errorf("streamclaims: Verify: missing dir claim")
	case sc.Protocol == "":
		return nil, fmt.Errorf("streamclaims: Verify: missing proto claim")
	case sc.Nonce == "":
		return nil, fmt.Errorf("streamclaims: Verify: missing nonce claim")
	case sc.Kind == 0:
		return nil, fmt.Errorf("streamclaims: Verify: kind bitfield must not be zero")
	}

	kind := StreamKind(sc.Kind)

	// PlaybackRange consistency.
	if kind.Has(StreamKindPlayback) && sc.PlaybackRange == nil {
		return nil, fmt.Errorf("streamclaims: Verify: PLAYBACK kind requires PlaybackRange")
	}
	if !kind.Has(StreamKindPlayback) && sc.PlaybackRange != nil {
		return nil, fmt.Errorf("streamclaims: Verify: PlaybackRange present but kind does not include PLAYBACK")
	}

	// TTL guard — even a validly signed token cannot have a TTL > MaxTTL.
	if jc.IssuedAt != nil && jc.ExpiresAt != nil {
		actualTTL := jc.ExpiresAt.Sub(jc.IssuedAt.Time)
		if actualTTL > MaxTTL {
			return nil, fmt.Errorf("streamclaims: Verify: token TTL %s exceeds MaxTTL %s", actualTTL, MaxTTL)
		}
	}

	claims := &StreamClaims{
		UserID: UserID(sc.UserID),
		TenantRef: TenantRef{
			Type: auth.TenantType(sc.TenantType),
			ID:   sc.TenantID,
		},
		CameraID:      sc.CameraID,
		RecorderID:    RecorderID(sc.RecorderID),
		DirectoryID:   sc.DirectoryID,
		Kind:          kind,
		Protocol:      Protocol(sc.Protocol),
		PlaybackRange: sc.PlaybackRange,
		Nonce:         sc.Nonce,
		ExpiresAt:     jc.ExpiresAt.Time,
	}

	v.log.Debug("stream token verified",
		slog.String("jti", jc.ID),
		slog.String("cam", claims.CameraID),
		slog.String("rec", string(claims.RecorderID)),
		slog.String("tenant_id", claims.TenantRef.ID),
		slog.String("nonce", claims.Nonce),
		slog.Time("expires_at", claims.ExpiresAt),
	)

	return claims, nil
}

// HasKind is a convenience function for callers that have a verified
// [StreamClaims] and want to check scope before serving a request.
// It mirrors [StreamKind.Has] but operates on the claims struct directly.
//
// Usage:
//
//	if !HasKind(claims, StreamKindLive) {
//	    return errors.New("permission.insufficient_scope")
//	}
func HasKind(c *StreamClaims, want StreamKind) bool {
	return c != nil && c.Kind.Has(want)
}

// RemainingTTL returns how long until the token expires. Returns a negative
// duration if the token is already expired — callers should treat negative
// values as expired rather than panicking.
func RemainingTTL(c *StreamClaims) time.Duration {
	return time.Until(c.ExpiresAt)
}
