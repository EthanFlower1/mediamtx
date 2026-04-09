package tokenverify

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/MicahParks/keyfunc/v3"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/time/rate"
)

// SkewTolerance is the permitted clock skew applied to both exp and nbf
// checks. It is fixed rather than configurable so every role in the
// system has identical skew semantics (one of the goals of hoisting
// token verification into this shared package).
const SkewTolerance = 60 * time.Second

// DefaultCacheTTL is the default background refresh interval for the
// JWKS cache, used when [VerifierConfig.CacheTTL] is zero.
const DefaultCacheTTL = 5 * time.Minute

// allowedAlgs is the signature algorithm allowlist. `none` and `HS256`
// are intentionally excluded: Kaivue accepts only asymmetric signatures
// and the JWKS we consume is a public-key set.
var allowedAlgs = []string{
	jwt.SigningMethodRS256.Alg(),
	jwt.SigningMethodRS384.Alg(),
	jwt.SigningMethodRS512.Alg(),
	jwt.SigningMethodES256.Alg(),
	jwt.SigningMethodES384.Alg(),
}

// Clock is the time source the verifier consults for exp/nbf checks.
// It is injectable so tests can pin time deterministically.
type Clock interface {
	Now() time.Time
}

type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

// VerifierConfig configures a [TokenVerifier]. All fields except
// JWKSURL, Issuer and Audience have sensible defaults.
type VerifierConfig struct {
	// JWKSURL is the absolute URL of the JSON Web Key Set the
	// verifier fetches and caches. For Zitadel this is the
	// /oauth/v2/keys endpoint on the IdP host.
	JWKSURL string

	// Issuer is the expected value of the `iss` claim. Must match
	// exactly (no trailing-slash normalization).
	Issuer string

	// Audience is the expected value of the `aud` claim. For v1
	// there is exactly one audience per verifier; if a token has
	// multiple audiences it passes as long as this one is present.
	Audience string

	// CacheTTL is the background refresh interval for the JWKS
	// cache. Defaults to DefaultCacheTTL (5 minutes).
	CacheTTL time.Duration

	// HTTPClient is used for JWKS fetches. Defaults to
	// http.DefaultClient when nil.
	HTTPClient *http.Client

	// Clock is the time source. Defaults to the real wall clock.
	Clock Clock
}

// VerifiedToken is the result of a successful [TokenVerifier.Verify]
// call. It is intentionally named differently from the parent
// package's [auth.Claims] — this primitive operates on raw JWTs and
// returns the full decoded claims set, while the parent Claims type is
// the higher-level tenant-scoped projection an IdP adapter produces.
type VerifiedToken struct {
	// Subject is the `sub` claim.
	Subject string
	// Issuer is the `iss` claim (already verified to match the
	// configured issuer).
	Issuer string
	// Audience is the full list of audiences on the token.
	Audience []string
	// ExpiresAt is the `exp` claim as a time.Time.
	ExpiresAt time.Time
	// IssuedAt is the `iat` claim, or the zero value if absent.
	IssuedAt time.Time
	// NotBefore is the `nbf` claim, or the zero value if absent.
	NotBefore time.Time
	// JTI is the `jti` claim, or the empty string if absent.
	JTI string
	// Custom holds every claim that was on the token, including the
	// registered ones above. Adapter code projects this into a
	// tenant-scoped Claims struct.
	Custom map[string]any
}

// TokenVerifier validates JWTs against a cached JWKS. It is safe for
// concurrent use from multiple goroutines. Construct one via
// [NewTokenVerifier] and release it with [TokenVerifier.Close].
type TokenVerifier struct {
	cfg VerifierConfig

	kf     keyfunc.Keyfunc
	cancel context.CancelFunc

	closeOnce sync.Once
}

// NewTokenVerifier constructs a [TokenVerifier] from the given config.
// It does NOT perform the initial JWKS fetch synchronously — the first
// [TokenVerifier.Verify] call triggers a lazy fetch. This keeps
// startup fast even when the IdP is briefly unreachable; a verifier
// that cannot reach the IdP still fails closed at Verify time.
func NewTokenVerifier(cfg VerifierConfig) (*TokenVerifier, error) {
	if cfg.JWKSURL == "" {
		return nil, errors.New("tokenverify: VerifierConfig.JWKSURL must not be empty")
	}
	if cfg.Issuer == "" {
		return nil, errors.New("tokenverify: VerifierConfig.Issuer must not be empty")
	}
	if cfg.Audience == "" {
		return nil, errors.New("tokenverify: VerifierConfig.Audience must not be empty")
	}
	if cfg.CacheTTL <= 0 {
		cfg.CacheTTL = DefaultCacheTTL
	}
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = http.DefaultClient
	}
	if cfg.Clock == nil {
		cfg.Clock = realClock{}
	}

	ctx, cancel := context.WithCancel(context.Background())

	override := keyfunc.Override{
		Client:          cfg.HTTPClient,
		RefreshInterval: cfg.CacheTTL,
		// RefreshUnknownKID triggers a synchronous JWKS refresh
		// on the keyfunc.KeyRead path when the kid is not in the
		// cached set. The rate.Limiter bounds that: at most one
		// forced refresh every CacheTTL/5, burst of 1. This is
		// the defense against a flood of unknown-kid tokens
		// turning into an IdP DDOS.
		RefreshUnknownKID: rate.NewLimiter(rate.Every(cfg.CacheTTL/5), 1),
	}

	kf, err := keyfunc.NewDefaultOverrideCtx(ctx, []string{cfg.JWKSURL}, override)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("tokenverify: build JWKS keyfunc: %w", err)
	}

	return &TokenVerifier{
		cfg:    cfg,
		kf:     kf,
		cancel: cancel,
	}, nil
}

// Close stops the background JWKS refresh goroutine and releases any
// resources held by the verifier. It is safe to call Close multiple
// times; subsequent calls are no-ops. Close never returns an error in
// the current implementation but the signature is reserved for future
// use (e.g. waiting for in-flight HTTP requests to drain).
func (v *TokenVerifier) Close() error {
	v.closeOnce.Do(func() {
		if v.cancel != nil {
			v.cancel()
		}
	})
	return nil
}

// Verify parses, validates, and returns the decoded [VerifiedToken]
// for a raw JWT. Any verification failure — malformed token, bad
// signature, wrong issuer, wrong audience, expired, future nbf,
// disallowed alg, unknown kid — returns a non-nil error and nil
// token. Verify is safe for concurrent use.
//
// The ctx parameter is honored by the JWKS storage layer for fetches
// and refreshes. Cancelling ctx does NOT abort an in-flight signature
// check; the signature check is CPU-bound and returns quickly.
func (v *TokenVerifier) Verify(ctx context.Context, rawJWT string) (*VerifiedToken, error) {
	if rawJWT == "" {
		return nil, errors.New("tokenverify: empty token")
	}

	now := v.cfg.Clock.Now()

	parser := jwt.NewParser(
		jwt.WithValidMethods(allowedAlgs),
		jwt.WithIssuer(v.cfg.Issuer),
		jwt.WithAudience(v.cfg.Audience),
		jwt.WithLeeway(SkewTolerance),
		jwt.WithExpirationRequired(),
		// We pin time ourselves via [jwt.Parser.TimeFunc] below to
		// honor the injected clock.
		jwt.WithTimeFunc(func() time.Time { return now }),
	)

	claims := jwt.MapClaims{}
	token, err := parser.ParseWithClaims(rawJWT, claims, v.kf.KeyfuncCtx(ctx))
	if err != nil {
		return nil, fmt.Errorf("tokenverify: verify: %w", err)
	}
	if !token.Valid {
		return nil, errors.New("tokenverify: verify: token reported invalid")
	}

	// Defense in depth: even though jwt.WithValidMethods enforces
	// the allowlist, re-check the alg header here. If an attacker
	// finds a way past jwt.Parser's method gate (spec-confusion,
	// library bug), we still refuse to trust the token.
	algHeader, _ := token.Header["alg"].(string)
	if !algAllowed(algHeader) {
		return nil, fmt.Errorf("tokenverify: verify: alg %q not permitted", algHeader)
	}

	out := &VerifiedToken{
		Custom: map[string]any(claims),
	}

	if sub, ok := claims["sub"].(string); ok {
		out.Subject = sub
	}
	if iss, ok := claims["iss"].(string); ok {
		out.Issuer = iss
	}
	out.Audience = extractAudience(claims["aud"])
	if jti, ok := claims["jti"].(string); ok {
		out.JTI = jti
	}
	if exp, ok := extractNumericDate(claims["exp"]); ok {
		out.ExpiresAt = exp
	}
	if iat, ok := extractNumericDate(claims["iat"]); ok {
		out.IssuedAt = iat
	}
	if nbf, ok := extractNumericDate(claims["nbf"]); ok {
		out.NotBefore = nbf
	}

	return out, nil
}

// algAllowed reports whether alg is on the allowlist. Case-sensitive
// match: JWT headers MUST use the exact registered names.
func algAllowed(alg string) bool {
	for _, a := range allowedAlgs {
		if a == alg {
			return true
		}
	}
	return false
}

// extractAudience normalizes a JWT `aud` claim into a []string. RFC
// 7519 allows either a single string or an array of strings.
func extractAudience(v any) []string {
	switch a := v.(type) {
	case string:
		return []string{a}
	case []string:
		return append([]string(nil), a...)
	case []any:
		out := make([]string, 0, len(a))
		for _, s := range a {
			if str, ok := s.(string); ok {
				out = append(out, str)
			}
		}
		return out
	}
	return nil
}

// extractNumericDate parses a JWT NumericDate claim (seconds since
// epoch) from the map-claims representation. Returns the zero value
// and false if the claim is absent or of an unexpected type.
func extractNumericDate(v any) (time.Time, bool) {
	switch n := v.(type) {
	case float64:
		return time.Unix(int64(n), 0), true
	case int64:
		return time.Unix(n, 0), true
	case json.Number:
		i, err := n.Int64()
		if err != nil {
			return time.Time{}, false
		}
		return time.Unix(i, 0), true
	}
	return time.Time{}, false
}
