package streamclaims

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/MicahParks/jwkset"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// Issuer signs StreamClaims JWTs with an RSA private key (RS256).
//
// Construct one via [NewIssuer]. The [Issuer] holds only public state after
// construction and is safe for concurrent use by multiple goroutines.
//
// Architectural note (§9.1): signing happens cloud-side only. Never embed an
// [Issuer] in Recorder or Gateway code. Recorders use [Verifier] instead.
type Issuer struct {
	privateKey *rsa.PrivateKey
	signer     crypto.Signer
	issuerID   string // "iss" claim — typically the cloud Directory service URL
	audience   string // "aud" claim — typically "kaivue-recorder"
	keyID      string // "kid" in JWK and JWT header — rotatable
	log        *slog.Logger
}

// IssuerOption is a functional option for [NewIssuer].
type IssuerOption func(*Issuer)

// WithIssuerLogger sets the structured logger used by the [Issuer].
func WithIssuerLogger(l *slog.Logger) IssuerOption {
	return func(i *Issuer) { i.log = l }
}

// WithKeyID sets the key identifier embedded in the JWT header ("kid").
// This MUST match the key ID used when publishing the JWKS via
// [PublicKeySet] so that Verifiers can select the correct key during rotation.
// Defaults to a random UUID set at construction time.
func WithKeyID(kid string) IssuerOption {
	return func(i *Issuer) { i.keyID = kid }
}

// NewIssuer constructs an [Issuer] from an RSA private key.
//
// issuerID should be the stable URL of the signing Directory service (e.g.
// "https://us-east-2.api.kaivue.com"). audience should be the expected token
// consumer ("kaivue-recorder").
//
// The signer parameter accepts any [crypto.Signer] backed by an RSA key. In
// production this will typically be a KMS-backed signer (via AWS KMS or
// Cloud HSM); in tests pass rsa.GenerateKey output directly.
func NewIssuer(signer crypto.Signer, issuerID, audience string, opts ...IssuerOption) (*Issuer, error) {
	if _, isRSA := signer.Public().(*rsa.PublicKey); !isRSA {
		return nil, fmt.Errorf("streamclaims: NewIssuer: signer must be backed by an RSA key (got %T)", signer.Public())
	}

	// Extract the concrete *rsa.PrivateKey if the signer is one, so we can
	// include the public key in the JWKS. For HSM-backed signers the caller
	// must supply the public key via the JWKS helper separately.
	var priv *rsa.PrivateKey
	if rsaPriv, isRSAPriv := signer.(*rsa.PrivateKey); isRSAPriv {
		priv = rsaPriv
	}

	kid := uuid.NewString()
	iss := &Issuer{
		privateKey: priv,
		signer:     signer,
		issuerID:   issuerID,
		audience:   audience,
		keyID:      kid,
		log:        slog.Default(),
	}
	for _, o := range opts {
		o(iss)
	}
	return iss, nil
}

// jwtClaims is the internal type used when signing. It embeds
// jwt.RegisteredClaims and adds the StreamClaims fields as a flat map so
// that the signed JWT payload is both standard-compliant and self-describing.
type jwtClaims struct {
	jwt.RegisteredClaims

	// Application claims — kept as a nested object under "sc" to avoid
	// collisions with any standard or well-known JWT claim names.
	SC streamClaimsPayload `json:"sc"`
}

// streamClaimsPayload is the JSON-serialisable form of StreamClaims.
// All times are Unix epoch seconds (int64) to avoid floating-point ambiguity.
type streamClaimsPayload struct {
	UserID        string     `json:"uid"`
	TenantType    string     `json:"tnt_type"`
	TenantID      string     `json:"tnt_id"`
	CameraID      string     `json:"cam"`
	RecorderID    string     `json:"rec"`
	DirectoryID   string     `json:"dir"`
	Kind          uint32     `json:"kind"`
	Protocol      string     `json:"proto"`
	PlaybackRange *TimeRange `json:"pbr,omitempty"`
	Nonce         string     `json:"nonce"`
}

// Sign mints a signed RS256 JWT carrying claims. The token string is returned
// on success; any error means no token was produced.
//
// Sign enforces [MaxTTL]: if claims.ExpiresAt is more than MaxTTL from now,
// Sign returns an error instead of producing an over-privileged token.
//
// Sign sets the standard JWT claims (iss, aud, iat, nbf, jti, exp)
// automatically from the Issuer configuration and from claims.ExpiresAt.
// The caller only needs to populate the StreamClaims fields.
func (i *Issuer) Sign(claims StreamClaims) (string, error) {
	now := time.Now().UTC()

	// Enforce TTL cap — fail closed.
	if claims.ExpiresAt.IsZero() {
		return "", fmt.Errorf("streamclaims: Sign: ExpiresAt must be set")
	}
	ttl := claims.ExpiresAt.Sub(now)
	if ttl <= 0 {
		return "", fmt.Errorf("streamclaims: Sign: ExpiresAt is in the past")
	}
	if ttl > MaxTTL {
		return "", fmt.Errorf("streamclaims: Sign: ExpiresAt exceeds MaxTTL (%s > %s)", ttl, MaxTTL)
	}

	// Require at least one StreamKind bit.
	if claims.Kind == 0 {
		return "", fmt.Errorf("streamclaims: Sign: Kind must have at least one bit set")
	}

	// Playback range consistency.
	if claims.Kind.Has(StreamKindPlayback) && claims.PlaybackRange == nil {
		return "", fmt.Errorf("streamclaims: Sign: PlaybackRange required for PLAYBACK kind")
	}
	if !claims.Kind.Has(StreamKindPlayback) && claims.PlaybackRange != nil {
		return "", fmt.Errorf("streamclaims: Sign: PlaybackRange must be nil when Kind does not include PLAYBACK")
	}

	// Required string fields.
	if claims.Nonce == "" {
		return "", fmt.Errorf("streamclaims: Sign: Nonce must not be empty")
	}
	if claims.TenantRef.IsZero() {
		return "", fmt.Errorf("streamclaims: Sign: TenantRef must be set")
	}
	if claims.CameraID == "" {
		return "", fmt.Errorf("streamclaims: Sign: CameraID must not be empty")
	}
	if claims.RecorderID == "" {
		return "", fmt.Errorf("streamclaims: Sign: RecorderID must not be empty")
	}
	if claims.DirectoryID == "" {
		return "", fmt.Errorf("streamclaims: Sign: DirectoryID must not be empty")
	}
	if claims.UserID == "" {
		return "", fmt.Errorf("streamclaims: Sign: UserID must not be empty")
	}
	if claims.Protocol == "" {
		return "", fmt.Errorf("streamclaims: Sign: Protocol must not be empty")
	}

	jti := uuid.NewString()
	jc := jwtClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    i.issuerID,
			Audience:  jwt.ClaimStrings{i.audience},
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(claims.ExpiresAt),
			ID:        jti,
		},
		SC: streamClaimsPayload{
			UserID:        string(claims.UserID),
			TenantType:    string(claims.TenantRef.Type),
			TenantID:      claims.TenantRef.ID,
			CameraID:      claims.CameraID,
			RecorderID:    string(claims.RecorderID),
			DirectoryID:   claims.DirectoryID,
			Kind:          uint32(claims.Kind),
			Protocol:      string(claims.Protocol),
			PlaybackRange: claims.PlaybackRange,
			Nonce:         claims.Nonce,
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, jc)
	token.Header["kid"] = i.keyID

	signed, err := token.SignedString(i.signer)
	if err != nil {
		return "", fmt.Errorf("streamclaims: Sign: jwt signing failed: %w", err)
	}

	i.log.Debug("stream token signed",
		slog.String("jti", jti),
		slog.String("cam", claims.CameraID),
		slog.String("rec", string(claims.RecorderID)),
		slog.String("tenant_id", claims.TenantRef.ID),
		slog.String("nonce", claims.Nonce),
		slog.Duration("ttl", ttl),
	)

	return signed, nil
}

// PublicKeySet returns a JSON-serialisable [jwkset.JWKSMarshal] representing
// the Issuer's public key. This is the value served by the
// /.well-known/jwks.json endpoint (handler: KAI-255).
//
// Returns an error if the [Issuer] was constructed with an opaque HSM-backed
// [crypto.Signer] that does not expose its private key (in that case the
// JWKS must be built externally and passed to the handler directly).
func (i *Issuer) PublicKeySet() (json.RawMessage, error) {
	if i.privateKey == nil {
		return nil, fmt.Errorf(
			"streamclaims: PublicKeySet: issuer has no concrete private key (HSM-backed signer); build JWKS externally",
		)
	}

	jwkOptions := jwkset.JWKOptions{
		Marshal: jwkset.JWKMarshalOptions{
			Private: false,
		},
		Metadata: jwkset.JWKMetadataOptions{
			KID: i.keyID,
			ALG: jwkset.AlgRS256,
			USE: jwkset.UseSig,
		},
	}

	jwkData, err := jwkset.NewJWKFromKey(i.privateKey, jwkOptions)
	if err != nil {
		return nil, fmt.Errorf("streamclaims: PublicKeySet: failed to build JWK: %w", err)
	}

	store := jwkset.NewMemoryStorage()
	ctx := contextBackground()
	if writeErr := store.KeyWrite(ctx, jwkData); writeErr != nil {
		return nil, fmt.Errorf("streamclaims: PublicKeySet: failed to write JWK to store: %w", writeErr)
	}

	raw, err := store.JSONPublic(ctx)
	if err != nil {
		return nil, fmt.Errorf("streamclaims: PublicKeySet: failed to marshal JWKS: %w", err)
	}

	return raw, nil
}

// KeyID returns the key identifier ("kid") used in JWT headers and JWK entries.
func (i *Issuer) KeyID() string { return i.keyID }

// Audience returns the audience string embedded in tokens minted by this Issuer.
func (i *Issuer) Audience() string { return i.audience }

// IssuerID returns the issuer URL embedded in tokens minted by this Issuer.
func (i *Issuer) IssuerID() string { return i.issuerID }

// GenerateTestKey is a convenience helper for tests and local development.
// It generates an ephemeral 2048-bit RSA key pair. Production code must use
// a KMS-managed key instead.
func GenerateTestKey() (*rsa.PrivateKey, error) {
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("streamclaims: GenerateTestKey: %w", err)
	}
	return k, nil
}
