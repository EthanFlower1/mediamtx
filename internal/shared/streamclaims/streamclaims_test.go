package streamclaims_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/bluenviron/mediamtx/internal/shared/auth"
	"github.com/bluenviron/mediamtx/internal/shared/streamclaims"
)

// ─── helpers ────────────────────────────────────────────────────────────────

const (
	testIssuer   = "https://us-east-2.api.kaivue.com"
	testAudience = "kaivue-recorder"
)

func mustIssuer(t *testing.T) *streamclaims.Issuer {
	t.Helper()
	key, err := streamclaims.GenerateTestKey()
	if err != nil {
		t.Fatalf("GenerateTestKey: %v", err)
	}
	iss, err := streamclaims.NewIssuer(key, testIssuer, testAudience)
	if err != nil {
		t.Fatalf("NewIssuer: %v", err)
	}
	return iss
}

func mustVerifier(t *testing.T, iss *streamclaims.Issuer) *streamclaims.Verifier {
	t.Helper()
	jwksJSON, err := iss.PublicKeySet()
	if err != nil {
		t.Fatalf("PublicKeySet: %v", err)
	}
	v, err := streamclaims.NewVerifier(jwksJSON, testIssuer, testAudience)
	if err != nil {
		t.Fatalf("NewVerifier: %v", err)
	}
	return v
}

func validClaims(t *testing.T) streamclaims.StreamClaims {
	t.Helper()
	nonce, err := streamclaims.GenerateNonce()
	if err != nil {
		t.Fatalf("GenerateNonce: %v", err)
	}
	return streamclaims.StreamClaims{
		UserID: "user-001",
		TenantRef: auth.TenantRef{
			Type: auth.TenantTypeCustomer,
			ID:   "tenant-abc",
		},
		CameraID:    "cam-001",
		RecorderID:  "rec-001",
		DirectoryID: "dir-001",
		Kind:        streamclaims.StreamKindLive,
		Protocol:    streamclaims.ProtocolWebRTC,
		ExpiresAt:   time.Now().Add(2 * time.Minute),
		Nonce:       nonce,
	}
}

// ─── round-trip ─────────────────────────────────────────────────────────────

func TestRoundTrip_Live(t *testing.T) {
	iss := mustIssuer(t)
	ver := mustVerifier(t, iss)

	c := validClaims(t)
	tok, err := iss.Sign(c)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}

	got, err := ver.Verify(tok)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}

	if got.UserID != c.UserID {
		t.Errorf("UserID: got %q want %q", got.UserID, c.UserID)
	}
	if !got.TenantRef.Equal(c.TenantRef) {
		t.Errorf("TenantRef: got %+v want %+v", got.TenantRef, c.TenantRef)
	}
	if got.CameraID != c.CameraID {
		t.Errorf("CameraID: got %q want %q", got.CameraID, c.CameraID)
	}
	if got.RecorderID != c.RecorderID {
		t.Errorf("RecorderID: got %q want %q", got.RecorderID, c.RecorderID)
	}
	if got.DirectoryID != c.DirectoryID {
		t.Errorf("DirectoryID: got %q want %q", got.DirectoryID, c.DirectoryID)
	}
	if got.Kind != c.Kind {
		t.Errorf("Kind: got %d want %d", got.Kind, c.Kind)
	}
	if got.Protocol != c.Protocol {
		t.Errorf("Protocol: got %q want %q", got.Protocol, c.Protocol)
	}
	if got.Nonce != c.Nonce {
		t.Errorf("Nonce: got %q want %q", got.Nonce, c.Nonce)
	}
}

func TestRoundTrip_Playback(t *testing.T) {
	iss := mustIssuer(t)
	ver := mustVerifier(t, iss)

	nonce, _ := streamclaims.GenerateNonce()
	now := time.Now().UTC()
	c := streamclaims.StreamClaims{
		UserID:      "user-002",
		TenantRef:   auth.TenantRef{Type: auth.TenantTypeIntegrator, ID: "integrator-xyz"},
		CameraID:    "cam-002",
		RecorderID:  "rec-002",
		DirectoryID: "dir-001",
		Kind:        streamclaims.StreamKindPlayback,
		Protocol:    streamclaims.ProtocolHLS,
		PlaybackRange: &streamclaims.TimeRange{
			Start: now.Add(-1 * time.Hour),
			End:   now.Add(-30 * time.Minute),
		},
		ExpiresAt: now.Add(3 * time.Minute),
		Nonce:     nonce,
	}

	tok, err := iss.Sign(c)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	got, err := ver.Verify(tok)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !got.Kind.Has(streamclaims.StreamKindPlayback) {
		t.Error("expected PLAYBACK kind bit set")
	}
	if got.PlaybackRange == nil {
		t.Fatal("expected PlaybackRange to be non-nil")
	}
	if !got.PlaybackRange.Start.Equal(c.PlaybackRange.Start) {
		t.Errorf("PlaybackRange.Start: got %v want %v", got.PlaybackRange.Start, c.PlaybackRange.Start)
	}
}

func TestRoundTrip_MultiKind(t *testing.T) {
	iss := mustIssuer(t)
	ver := mustVerifier(t, iss)

	nonce, _ := streamclaims.GenerateNonce()
	c := streamclaims.StreamClaims{
		UserID:      "user-003",
		TenantRef:   auth.TenantRef{Type: auth.TenantTypeCustomer, ID: "tenant-multi"},
		CameraID:    "cam-003",
		RecorderID:  "rec-003",
		DirectoryID: "dir-001",
		Kind:        streamclaims.StreamKindLive | streamclaims.StreamKindAudioTalkback,
		Protocol:    streamclaims.ProtocolWebRTC,
		ExpiresAt:   time.Now().Add(2 * time.Minute),
		Nonce:       nonce,
	}

	tok, err := iss.Sign(c)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	got, err := ver.Verify(tok)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !got.Kind.Has(streamclaims.StreamKindLive) {
		t.Error("expected LIVE bit set")
	}
	if !got.Kind.Has(streamclaims.StreamKindAudioTalkback) {
		t.Error("expected AUDIO_TALKBACK bit set")
	}
	if got.Kind.Has(streamclaims.StreamKindPlayback) {
		t.Error("unexpected PLAYBACK bit set")
	}
}

// ─── expiry ─────────────────────────────────────────────────────────────────

func TestVerify_ExpiredToken(t *testing.T) {
	iss := mustIssuer(t)
	ver := mustVerifier(t, iss)

	// We need to sign a token that is already expired. Since Sign enforces
	// ExpiresAt > now, we sign with the shortest allowed TTL, sleep past it,
	// and verify it fails. Use a tiny fixed duration to keep the test fast.
	// NOTE: this requires the token to have been signed before expiry, so we
	// use a separate issuer with no time manipulation — instead we sign a
	// near-future expiry and then call verify after the deadline using a
	// pre-expired token built by abusing the sign path.
	//
	// Approach: sign with 1-second TTL and immediately time.Sleep(1100ms).
	nonce, _ := streamclaims.GenerateNonce()
	c := validClaims(t)
	c.ExpiresAt = time.Now().Add(1 * time.Second)
	c.Nonce = nonce

	tok, err := iss.Sign(c)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}

	time.Sleep(1100 * time.Millisecond)

	_, err = ver.Verify(tok)
	if err == nil {
		t.Fatal("expected error for expired token, got nil")
	}
}

// ─── TTL cap ────────────────────────────────────────────────────────────────

func TestSign_TTLExceedsMaxTTL(t *testing.T) {
	iss := mustIssuer(t)

	c := validClaims(t)
	c.ExpiresAt = time.Now().Add(10 * time.Minute) // > MaxTTL (5 min)

	_, err := iss.Sign(c)
	if err == nil {
		t.Fatal("expected error when ExpiresAt exceeds MaxTTL")
	}
}

func TestSign_ExpiresInPast(t *testing.T) {
	iss := mustIssuer(t)

	c := validClaims(t)
	c.ExpiresAt = time.Now().Add(-1 * time.Minute)

	_, err := iss.Sign(c)
	if err == nil {
		t.Fatal("expected error when ExpiresAt is in the past")
	}
}

// ─── invalid signature ──────────────────────────────────────────────────────

func TestVerify_WrongSigningKey(t *testing.T) {
	iss1 := mustIssuer(t)
	iss2 := mustIssuer(t) // different key

	// Build a verifier that trusts iss2's key.
	jwksJSON2, _ := iss2.PublicKeySet()
	ver, err := streamclaims.NewVerifier(jwksJSON2, testIssuer, testAudience)
	if err != nil {
		t.Fatalf("NewVerifier: %v", err)
	}

	tok, err := iss1.Sign(validClaims(t))
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}

	_, err = ver.Verify(tok)
	if err == nil {
		t.Fatal("expected signature verification failure")
	}
}

// ─── scope mismatch ─────────────────────────────────────────────────────────

func TestHasKind_ScopeMismatch(t *testing.T) {
	iss := mustIssuer(t)
	ver := mustVerifier(t, iss)

	// Token has LIVE only.
	tok, err := iss.Sign(validClaims(t))
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	got, err := ver.Verify(tok)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}

	// Check that PLAYBACK is not present.
	if streamclaims.HasKind(got, streamclaims.StreamKindPlayback) {
		t.Error("PLAYBACK scope should not be present on a LIVE-only token")
	}
	if !streamclaims.HasKind(got, streamclaims.StreamKindLive) {
		t.Error("LIVE scope should be present")
	}
}

// ─── nonce ──────────────────────────────────────────────────────────────────

func TestNonce_Uniqueness(t *testing.T) {
	const count = 1000
	seen := make(map[string]struct{}, count)
	for i := range count {
		n, err := streamclaims.GenerateNonce()
		if err != nil {
			t.Fatalf("GenerateNonce[%d]: %v", i, err)
		}
		if _, dup := seen[n]; dup {
			t.Fatalf("nonce collision at iteration %d: %q", i, n)
		}
		seen[n] = struct{}{}
	}
}

func TestNonce_NonEmpty(t *testing.T) {
	n, err := streamclaims.GenerateNonce()
	if err != nil {
		t.Fatalf("GenerateNonce: %v", err)
	}
	if n == "" {
		t.Fatal("nonce must not be empty")
	}
}

func TestNonce_URLSafe(t *testing.T) {
	for range 100 {
		n, _ := streamclaims.GenerateNonce()
		for _, b := range []byte(n) {
			isUpper := b >= 'A' && b <= 'Z'
			isLower := b >= 'a' && b <= 'z'
			isDigit := b >= '0' && b <= '9'
			isDash := b == '-' || b == '_'
			if !isUpper && !isLower && !isDigit && !isDash {
				t.Fatalf("nonce %q contains non-URL-safe character %q", n, b)
			}
		}
	}
}

// ─── StreamKind bitfield ────────────────────────────────────────────────────

func TestStreamKind_Bitfield(t *testing.T) {
	cases := []struct {
		kind    streamclaims.StreamKind
		checks  []streamclaims.StreamKind
		present []bool
	}{
		{
			kind:    streamclaims.StreamKindLive,
			checks:  []streamclaims.StreamKind{streamclaims.StreamKindLive, streamclaims.StreamKindPlayback},
			present: []bool{true, false},
		},
		{
			kind: streamclaims.StreamKindLive | streamclaims.StreamKindAudioTalkback,
			checks: []streamclaims.StreamKind{
				streamclaims.StreamKindLive,
				streamclaims.StreamKindAudioTalkback,
				streamclaims.StreamKindSnapshot,
			},
			present: []bool{true, true, false},
		},
		{
			kind: streamclaims.StreamKindLive | streamclaims.StreamKindPlayback |
				streamclaims.StreamKindSnapshot | streamclaims.StreamKindAudioTalkback,
			checks: []streamclaims.StreamKind{
				streamclaims.StreamKindLive,
				streamclaims.StreamKindPlayback,
				streamclaims.StreamKindSnapshot,
				streamclaims.StreamKindAudioTalkback,
			},
			present: []bool{true, true, true, true},
		},
	}

	for _, tc := range cases {
		for j, check := range tc.checks {
			got := tc.kind.Has(check)
			if got != tc.present[j] {
				t.Errorf("kind=%d Has(%d): got %v want %v", tc.kind, check, got, tc.present[j])
			}
		}
	}
}

// ─── validation failures ────────────────────────────────────────────────────

func TestSign_MissingNonce(t *testing.T) {
	iss := mustIssuer(t)
	c := validClaims(t)
	c.Nonce = ""
	_, err := iss.Sign(c)
	if err == nil {
		t.Fatal("expected error for empty Nonce")
	}
}

func TestSign_MissingKind(t *testing.T) {
	iss := mustIssuer(t)
	c := validClaims(t)
	c.Kind = 0
	_, err := iss.Sign(c)
	if err == nil {
		t.Fatal("expected error for zero Kind")
	}
}

func TestSign_PlaybackWithoutRange(t *testing.T) {
	iss := mustIssuer(t)
	c := validClaims(t)
	c.Kind = streamclaims.StreamKindPlayback
	c.PlaybackRange = nil
	_, err := iss.Sign(c)
	if err == nil {
		t.Fatal("expected error for PLAYBACK without PlaybackRange")
	}
}

func TestSign_NonPlaybackWithRange(t *testing.T) {
	iss := mustIssuer(t)
	now := time.Now()
	c := validClaims(t)
	c.Kind = streamclaims.StreamKindLive
	c.PlaybackRange = &streamclaims.TimeRange{Start: now.Add(-1 * time.Hour), End: now}
	_, err := iss.Sign(c)
	if err == nil {
		t.Fatal("expected error for non-PLAYBACK token with PlaybackRange set")
	}
}

func TestSign_MissingTenantRef(t *testing.T) {
	iss := mustIssuer(t)
	c := validClaims(t)
	c.TenantRef = auth.TenantRef{}
	_, err := iss.Sign(c)
	if err == nil {
		t.Fatal("expected error for zero TenantRef")
	}
}

// ─── JWKS publication ───────────────────────────────────────────────────────

func TestPublicKeySet_ValidJSON(t *testing.T) {
	iss := mustIssuer(t)
	raw, err := iss.PublicKeySet()
	if err != nil {
		t.Fatalf("PublicKeySet: %v", err)
	}
	if !json.Valid(raw) {
		t.Fatalf("PublicKeySet returned invalid JSON: %s", raw)
	}

	// Confirm it has a "keys" array.
	var jwks struct {
		Keys []json.RawMessage `json:"keys"`
	}
	if unmarshalErr := json.Unmarshal(raw, &jwks); unmarshalErr != nil {
		t.Fatalf("unmarshal JWKS: %v", unmarshalErr)
	}
	if len(jwks.Keys) == 0 {
		t.Fatal("JWKS keys array is empty")
	}
}

// ─── audience / issuer mismatch ─────────────────────────────────────────────

func TestVerify_WrongAudience(t *testing.T) {
	iss := mustIssuer(t)
	jwksJSON, _ := iss.PublicKeySet()

	ver, err := streamclaims.NewVerifier(jwksJSON, testIssuer, "wrong-audience")
	if err != nil {
		t.Fatalf("NewVerifier: %v", err)
	}
	tok, _ := iss.Sign(validClaims(t))
	_, err = ver.Verify(tok)
	if err == nil {
		t.Fatal("expected error for audience mismatch")
	}
}

func TestVerify_WrongIssuer(t *testing.T) {
	iss := mustIssuer(t)
	jwksJSON, _ := iss.PublicKeySet()

	ver, err := streamclaims.NewVerifier(jwksJSON, "https://wrong.example.com", testAudience)
	if err != nil {
		t.Fatalf("NewVerifier: %v", err)
	}
	tok, _ := iss.Sign(validClaims(t))
	_, err = ver.Verify(tok)
	if err == nil {
		t.Fatal("expected error for issuer mismatch")
	}
}

func TestVerify_EmptyToken(t *testing.T) {
	iss := mustIssuer(t)
	ver := mustVerifier(t, iss)
	_, err := ver.Verify("")
	if err == nil {
		t.Fatal("expected error for empty token string")
	}
}

// ─── NewVerifier construction errors ────────────────────────────────────────

func TestNewVerifier_EmptyJWKS(t *testing.T) {
	_, err := streamclaims.NewVerifier(nil, testIssuer, testAudience)
	if err == nil {
		t.Fatal("expected error for nil jwksJSON")
	}
}

func TestNewVerifier_EmptyIssuer(t *testing.T) {
	iss := mustIssuer(t)
	jwksJSON, _ := iss.PublicKeySet()
	_, err := streamclaims.NewVerifier(jwksJSON, "", testAudience)
	if err == nil {
		t.Fatal("expected error for empty issuerID")
	}
}

func TestNewVerifier_EmptyAudience(t *testing.T) {
	iss := mustIssuer(t)
	jwksJSON, _ := iss.PublicKeySet()
	_, err := streamclaims.NewVerifier(jwksJSON, testIssuer, "")
	if err == nil {
		t.Fatal("expected error for empty audience")
	}
}

// ─── RemainingTTL ─────────────────────────────────────────────────────────

func TestRemainingTTL_FutureToken(t *testing.T) {
	iss := mustIssuer(t)
	ver := mustVerifier(t, iss)
	c := validClaims(t)
	c.ExpiresAt = time.Now().Add(2 * time.Minute)

	tok, _ := iss.Sign(c)
	got, _ := ver.Verify(tok)

	ttl := streamclaims.RemainingTTL(got)
	if ttl <= 0 {
		t.Errorf("expected positive RemainingTTL, got %s", ttl)
	}
	if ttl > 2*time.Minute+time.Second {
		t.Errorf("RemainingTTL %s suspiciously large", ttl)
	}
}
