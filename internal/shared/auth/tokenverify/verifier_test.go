package tokenverify_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/MicahParks/jwkset"
	"github.com/golang-jwt/jwt/v5"

	"github.com/bluenviron/mediamtx/internal/shared/auth/tokenverify"
)

// --- Test fixtures ---------------------------------------------------

const (
	testIssuer   = "https://id.kaivue.test/oauth/v2"
	testAudience = "kaivue-directory"
)

type testKey struct {
	kid  string
	priv *rsa.PrivateKey
}

func newTestKey(t *testing.T, kid string) testKey {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate rsa key: %v", err)
	}
	return testKey{kid: kid, priv: priv}
}

// jwksServer serves a JWKS document that can be rotated at runtime via
// setKeys. It is safe for concurrent use.
type jwksServer struct {
	t    *testing.T
	mu   sync.Mutex
	keys []testKey
	srv  *httptest.Server
	hits int64
}

func newJWKSServer(t *testing.T, initial ...testKey) *jwksServer {
	t.Helper()
	js := &jwksServer{t: t, keys: initial}
	js.srv = httptest.NewServer(http.HandlerFunc(js.serve))
	t.Cleanup(js.srv.Close)
	return js
}

func (j *jwksServer) URL() string {
	return j.srv.URL
}

func (j *jwksServer) Hits() int64 {
	return atomic.LoadInt64(&j.hits)
}

func (j *jwksServer) setKeys(keys ...testKey) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.keys = keys
}

func (j *jwksServer) serve(w http.ResponseWriter, _ *http.Request) {
	atomic.AddInt64(&j.hits, 1)

	j.mu.Lock()
	keys := append([]testKey(nil), j.keys...)
	j.mu.Unlock()

	store := jwkset.NewMemoryStorage()
	ctx := context.Background()
	for _, k := range keys {
		jwk, err := jwkset.NewJWKFromKey(k.priv.Public(), jwkset.JWKOptions{
			Metadata: jwkset.JWKMetadataOptions{
				KID: k.kid,
				ALG: jwkset.AlgRS256,
				USE: jwkset.UseSig,
			},
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if err := store.KeyWrite(ctx, jwk); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	raw, err := store.JSONPublic(ctx)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/jwk-set+json")
	_, _ = w.Write(raw)
}

// signToken builds a signed JWT with the given key, algorithm, header
// tweaks, and claims.
func signToken(t *testing.T, key testKey, method jwt.SigningMethod, claims jwt.MapClaims) string {
	t.Helper()
	tok := jwt.NewWithClaims(method, claims)
	tok.Header["kid"] = key.kid
	signed, err := tok.SignedString(key.priv)
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return signed
}

// fakeClock is a Clock for deterministic tests.
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func (f *fakeClock) Now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.now
}

func (f *fakeClock) set(t time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.now = t
}

// --- Positive path ---------------------------------------------------

func TestVerify_Valid(t *testing.T) {
	key := newTestKey(t, "kid-1")
	jsrv := newJWKSServer(t, key)

	now := time.Date(2026, 4, 8, 12, 0, 0, 0, time.UTC)
	clk := &fakeClock{now: now}

	v, err := tokenverify.NewTokenVerifier(tokenverify.VerifierConfig{
		JWKSURL:  jsrv.URL(),
		Issuer:   testIssuer,
		Audience: testAudience,
		Clock:    clk,
	})
	if err != nil {
		t.Fatalf("new verifier: %v", err)
	}
	defer v.Close()

	tok := signToken(t, key, jwt.SigningMethodRS256, jwt.MapClaims{
		"iss":      testIssuer,
		"aud":      testAudience,
		"sub":      "user-42",
		"jti":      "nonce-abc",
		"iat":      now.Add(-1 * time.Minute).Unix(),
		"nbf":      now.Add(-1 * time.Minute).Unix(),
		"exp":      now.Add(5 * time.Minute).Unix(),
		"tid":      "tenant-xyz",
		"role_set": []any{"admin", "viewer"},
	})

	got, err := v.Verify(context.Background(), tok)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if got.Subject != "user-42" {
		t.Errorf("subject: got %q", got.Subject)
	}
	if got.Issuer != testIssuer {
		t.Errorf("issuer: got %q", got.Issuer)
	}
	if len(got.Audience) != 1 || got.Audience[0] != testAudience {
		t.Errorf("audience: got %v", got.Audience)
	}
	if got.JTI != "nonce-abc" {
		t.Errorf("jti: got %q", got.JTI)
	}
	if got.ExpiresAt.Unix() != now.Add(5*time.Minute).Unix() {
		t.Errorf("exp: got %v", got.ExpiresAt)
	}
	if got.Custom["tid"] != "tenant-xyz" {
		t.Errorf("custom tid: got %v", got.Custom["tid"])
	}
}

// --- Negative paths --------------------------------------------------

func TestVerify_Negative(t *testing.T) {
	key := newTestKey(t, "kid-1")
	otherKey := newTestKey(t, "kid-other")
	jsrv := newJWKSServer(t, key)

	now := time.Date(2026, 4, 8, 12, 0, 0, 0, time.UTC)
	clk := &fakeClock{now: now}

	v, err := tokenverify.NewTokenVerifier(tokenverify.VerifierConfig{
		JWKSURL:  jsrv.URL(),
		Issuer:   testIssuer,
		Audience: testAudience,
		Clock:    clk,
	})
	if err != nil {
		t.Fatalf("new verifier: %v", err)
	}
	defer v.Close()

	good := func() jwt.MapClaims {
		return jwt.MapClaims{
			"iss": testIssuer,
			"aud": testAudience,
			"sub": "user-42",
			"iat": now.Add(-1 * time.Minute).Unix(),
			"exp": now.Add(5 * time.Minute).Unix(),
		}
	}

	cases := []struct {
		name  string
		token string
	}{
		{
			name: "wrong issuer",
			token: signToken(t, key, jwt.SigningMethodRS256, jwt.MapClaims{
				"iss": "https://evil.example",
				"aud": testAudience,
				"sub": "user-42",
				"exp": now.Add(5 * time.Minute).Unix(),
			}),
		},
		{
			name: "wrong audience",
			token: signToken(t, key, jwt.SigningMethodRS256, jwt.MapClaims{
				"iss": testIssuer,
				"aud": "some-other-service",
				"sub": "user-42",
				"exp": now.Add(5 * time.Minute).Unix(),
			}),
		},
		{
			name: "expired exp",
			token: signToken(t, key, jwt.SigningMethodRS256, jwt.MapClaims{
				"iss": testIssuer,
				"aud": testAudience,
				"sub": "user-42",
				"exp": now.Add(-5 * time.Minute).Unix(),
			}),
		},
		{
			name: "future nbf",
			token: signToken(t, key, jwt.SigningMethodRS256, jwt.MapClaims{
				"iss": testIssuer,
				"aud": testAudience,
				"sub": "user-42",
				"nbf": now.Add(10 * time.Minute).Unix(),
				"exp": now.Add(20 * time.Minute).Unix(),
			}),
		},
		{
			name: "bad signature (signed with other key, claims same kid)",
			token: func() string {
				// Sign with otherKey but advertise kid-1 → signature check fails.
				ck := testKey{kid: "kid-1", priv: otherKey.priv}
				return signToken(t, ck, jwt.SigningMethodRS256, good())
			}(),
		},
		{
			name: "unknown kid",
			token: func() string {
				// Advertise kid-zzz but sign with a key that is not in
				// the JWKS (and the JWKS has no such kid).
				k := testKey{kid: "kid-zzz", priv: otherKey.priv}
				return signToken(t, k, jwt.SigningMethodRS256, good())
			}(),
		},
		{
			name:  "alg: none",
			token: makeNoneToken(t, key.kid, good()),
		},
		{
			name: "alg: HS256",
			token: func() string {
				tok := jwt.NewWithClaims(jwt.SigningMethodHS256, good())
				tok.Header["kid"] = key.kid
				s, err := tok.SignedString([]byte("supersecret"))
				if err != nil {
					t.Fatalf("sign hs256: %v", err)
				}
				return s
			}(),
		},
		{
			name:  "malformed two parts",
			token: "header.payload",
		},
		{
			name:  "non-base64 payload",
			token: "aaa.!!!.bbb",
		},
		{
			name: "tampered payload",
			token: func() string {
				valid := signToken(t, key, jwt.SigningMethodRS256, good())
				// flip a byte in the middle part (payload)
				b := []byte(valid)
				for i := 0; i < len(b); i++ {
					if b[i] == '.' {
						// change the next char
						if i+3 < len(b) {
							if b[i+3] == 'a' {
								b[i+3] = 'b'
							} else {
								b[i+3] = 'a'
							}
						}
						break
					}
				}
				return string(b)
			}(),
		},
		{
			name:  "empty",
			token: "",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got, err := v.Verify(context.Background(), tc.token)
			if err == nil {
				t.Fatalf("expected error, got token=%+v", got)
			}
			if got != nil {
				t.Fatalf("expected nil token on error, got %+v", got)
			}
		})
	}
}

// makeNoneToken hand-builds a JWT with alg=none so we can test that
// the verifier refuses it. We hand-craft the bytes because golang-jwt
// requires an explicit opt-in to sign with SigningMethodNone.
func makeNoneToken(t *testing.T, kid string, claims jwt.MapClaims) string {
	t.Helper()
	header := map[string]any{"alg": "none", "typ": "JWT", "kid": kid}
	hdrJSON, _ := json.Marshal(header)
	clJSON, _ := json.Marshal(claims)
	b64 := func(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }
	return b64(hdrJSON) + "." + b64(clJSON) + "."
}

// --- Clock skew tolerance --------------------------------------------

func TestVerify_SkewTolerance(t *testing.T) {
	key := newTestKey(t, "kid-1")
	jsrv := newJWKSServer(t, key)

	now := time.Date(2026, 4, 8, 12, 0, 0, 0, time.UTC)
	clk := &fakeClock{now: now}

	v, err := tokenverify.NewTokenVerifier(tokenverify.VerifierConfig{
		JWKSURL:  jsrv.URL(),
		Issuer:   testIssuer,
		Audience: testAudience,
		Clock:    clk,
	})
	if err != nil {
		t.Fatalf("new verifier: %v", err)
	}
	defer v.Close()

	// Token expired 30s ago — within 60s skew tolerance, still valid.
	tok := signToken(t, key, jwt.SigningMethodRS256, jwt.MapClaims{
		"iss": testIssuer,
		"aud": testAudience,
		"sub": "user-42",
		"exp": now.Add(-30 * time.Second).Unix(),
	})
	if _, err := v.Verify(context.Background(), tok); err != nil {
		t.Errorf("token within skew tolerance should verify, got %v", err)
	}

	// Token expired 120s ago — outside skew tolerance.
	tok2 := signToken(t, key, jwt.SigningMethodRS256, jwt.MapClaims{
		"iss": testIssuer,
		"aud": testAudience,
		"sub": "user-42",
		"exp": now.Add(-120 * time.Second).Unix(),
	})
	if _, err := v.Verify(context.Background(), tok2); err == nil {
		t.Errorf("token outside skew tolerance should NOT verify")
	}
}

// --- JWKS rotation / forced refresh on unknown kid -------------------

func TestVerify_JWKSRotation(t *testing.T) {
	key1 := newTestKey(t, "kid-1")
	jsrv := newJWKSServer(t, key1)

	now := time.Date(2026, 4, 8, 12, 0, 0, 0, time.UTC)
	clk := &fakeClock{now: now}

	v, err := tokenverify.NewTokenVerifier(tokenverify.VerifierConfig{
		JWKSURL:  jsrv.URL(),
		Issuer:   testIssuer,
		Audience: testAudience,
		Clock:    clk,
		// Very long TTL so the background goroutine does NOT refresh
		// during the test — we want to verify the forced synchronous
		// refresh path triggered by an unknown kid.
		CacheTTL: 1 * time.Hour,
	})
	if err != nil {
		t.Fatalf("new verifier: %v", err)
	}
	defer v.Close()

	// Warm the cache with a valid kid-1 token.
	tok1 := signToken(t, key1, jwt.SigningMethodRS256, jwt.MapClaims{
		"iss": testIssuer, "aud": testAudience, "sub": "user-42",
		"exp": now.Add(5 * time.Minute).Unix(),
	})
	if _, err := v.Verify(context.Background(), tok1); err != nil {
		t.Fatalf("initial verify: %v", err)
	}

	hitsBefore := jsrv.Hits()

	// Rotate the JWKS on the server: key-1 is retired, key-2 is now
	// the only active key.
	key2 := newTestKey(t, "kid-2")
	jsrv.setKeys(key2)

	// Sign a token with key-2. The verifier's cache still only knows
	// kid-1 → it must trigger a forced refresh to pick up kid-2.
	tok2 := signToken(t, key2, jwt.SigningMethodRS256, jwt.MapClaims{
		"iss": testIssuer, "aud": testAudience, "sub": "user-42",
		"exp": now.Add(5 * time.Minute).Unix(),
	})
	if _, err := v.Verify(context.Background(), tok2); err != nil {
		t.Fatalf("verify after rotation: %v", err)
	}

	if jsrv.Hits() <= hitsBefore {
		t.Errorf("expected JWKS to be re-fetched on unknown kid; hits before=%d after=%d",
			hitsBefore, jsrv.Hits())
	}
}

// --- Concurrency -----------------------------------------------------

func TestVerify_Concurrent(t *testing.T) {
	key := newTestKey(t, "kid-1")
	jsrv := newJWKSServer(t, key)

	now := time.Date(2026, 4, 8, 12, 0, 0, 0, time.UTC)
	clk := &fakeClock{now: now}

	v, err := tokenverify.NewTokenVerifier(tokenverify.VerifierConfig{
		JWKSURL:  jsrv.URL(),
		Issuer:   testIssuer,
		Audience: testAudience,
		Clock:    clk,
	})
	if err != nil {
		t.Fatalf("new verifier: %v", err)
	}
	defer v.Close()

	tok := signToken(t, key, jwt.SigningMethodRS256, jwt.MapClaims{
		"iss": testIssuer, "aud": testAudience, "sub": "user-42",
		"exp": now.Add(5 * time.Minute).Unix(),
	})

	const workers = 32
	const perWorker = 50

	var wg sync.WaitGroup
	errs := make(chan error, workers*perWorker)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < perWorker; j++ {
				if _, err := v.Verify(context.Background(), tok); err != nil {
					errs <- err
					return
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		t.Errorf("concurrent verify: %v", e)
	}
}

// --- Config validation -----------------------------------------------

func TestNewTokenVerifier_ConfigValidation(t *testing.T) {
	cases := []struct {
		name string
		cfg  tokenverify.VerifierConfig
	}{
		{"missing jwks url", tokenverify.VerifierConfig{Issuer: "x", Audience: "y"}},
		{"missing issuer", tokenverify.VerifierConfig{JWKSURL: "http://x", Audience: "y"}},
		{"missing audience", tokenverify.VerifierConfig{JWKSURL: "http://x", Issuer: "y"}},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if _, err := tokenverify.NewTokenVerifier(tc.cfg); err == nil {
				t.Fatalf("expected error")
			}
		})
	}
}

func TestClose_Idempotent(t *testing.T) {
	key := newTestKey(t, "kid-1")
	jsrv := newJWKSServer(t, key)
	v, err := tokenverify.NewTokenVerifier(tokenverify.VerifierConfig{
		JWKSURL:  jsrv.URL(),
		Issuer:   testIssuer,
		Audience: testAudience,
	})
	if err != nil {
		t.Fatalf("new verifier: %v", err)
	}
	if err := v.Close(); err != nil {
		t.Fatalf("close 1: %v", err)
	}
	if err := v.Close(); err != nil {
		t.Fatalf("close 2: %v", err)
	}
}
