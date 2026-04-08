package pairing_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	directorydb "github.com/bluenviron/mediamtx/internal/directory/db"
	"github.com/bluenviron/mediamtx/internal/directory/pairing"
)

// ----------------------------------------------------------------------------
// Helpers
// ----------------------------------------------------------------------------

func newTestRootKey(t *testing.T) ed25519.PrivateKey {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	return priv
}

func newTestSigningKey(t *testing.T, rootKey ed25519.PrivateKey) ed25519.PrivateKey {
	t.Helper()
	sk, err := pairing.NewSigningKey(rootKey)
	require.NoError(t, err)
	return sk
}

func newTestDB(t *testing.T) *directorydb.DB {
	t.Helper()
	db, err := directorydb.Open(context.Background(), ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func newToken(now time.Time, rootKey ed25519.PrivateKey) *pairing.PairingToken {
	return &pairing.PairingToken{
		TokenID:             "test-token-id",
		DirectoryEndpoint:   "https://dir.local:8443",
		HeadscalePreAuthKey: "hskey-auth-deadbeef",
		StepCAFingerprint:   "aabbcc",
		StepCAEnrollToken:   "enroll-tok",
		DirectoryFingerprint: "ddeeff",
		SuggestedRoles:      []string{"recorder"},
		ExpiresAt:           now.Add(15 * time.Minute),
		SignedBy:            "admin-user-1",
		CloudTenantBinding:  "",
	}
}

// ----------------------------------------------------------------------------
// Encode / Decode round-trip
// ----------------------------------------------------------------------------

func TestEncodeDecodeRoundTrip(t *testing.T) {
	t.Parallel()
	rootKey := newTestRootKey(t)
	sk := newTestSigningKey(t, rootKey)

	tok := newToken(time.Now(), rootKey)
	encoded, err := tok.Encode(sk)
	require.NoError(t, err)
	require.NotEmpty(t, encoded)

	pub := pairing.VerifyPublicKey(sk)
	decoded, err := pairing.Decode(encoded, pub)
	require.NoError(t, err)

	assert.Equal(t, tok.TokenID, decoded.TokenID)
	assert.Equal(t, tok.DirectoryEndpoint, decoded.DirectoryEndpoint)
	assert.Equal(t, tok.HeadscalePreAuthKey, decoded.HeadscalePreAuthKey)
	assert.Equal(t, tok.StepCAFingerprint, decoded.StepCAFingerprint)
	assert.Equal(t, tok.SuggestedRoles, decoded.SuggestedRoles)
	assert.Equal(t, tok.SignedBy, decoded.SignedBy)
	assert.WithinDuration(t, tok.ExpiresAt, decoded.ExpiresAt, time.Second)
}

// ----------------------------------------------------------------------------
// Expiry rejection
// ----------------------------------------------------------------------------

func TestDecodeRejectsExpiredToken(t *testing.T) {
	t.Parallel()
	rootKey := newTestRootKey(t)
	sk := newTestSigningKey(t, rootKey)

	tok := newToken(time.Now(), rootKey)
	tok.ExpiresAt = time.Now().UTC().Add(-1 * time.Minute) // already expired
	encoded, err := tok.Encode(sk)
	require.NoError(t, err)

	pub := pairing.VerifyPublicKey(sk)
	_, err = pairing.Decode(encoded, pub)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expired")
}

// ----------------------------------------------------------------------------
// Tampered-token signature rejection
// ----------------------------------------------------------------------------

func TestDecodeRejectsTamperedToken(t *testing.T) {
	t.Parallel()
	rootKey := newTestRootKey(t)
	sk := newTestSigningKey(t, rootKey)

	tok := newToken(time.Now(), rootKey)
	encoded, err := tok.Encode(sk)
	require.NoError(t, err)

	// Flip a bit in the payload segment.
	tampered := []byte(encoded)
	if len(tampered) > 10 {
		tampered[5] ^= 0xFF
	}

	pub := pairing.VerifyPublicKey(sk)
	_, err = pairing.Decode(string(tampered), pub)
	require.Error(t, err)
}

// ----------------------------------------------------------------------------
// Cross-directory rejection
// ----------------------------------------------------------------------------

func TestDecodeRejectsCrossDirectoryToken(t *testing.T) {
	t.Parallel()
	// Directory A signs a token.
	rootKeyA := newTestRootKey(t)
	skA := newTestSigningKey(t, rootKeyA)

	tok := newToken(time.Now(), rootKeyA)
	encoded, err := tok.Encode(skA)
	require.NoError(t, err)

	// Directory B's public key is different.
	rootKeyB := newTestRootKey(t)
	skB := newTestSigningKey(t, rootKeyB)
	pubB := pairing.VerifyPublicKey(skB)

	_, err = pairing.Decode(encoded, pubB)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "signature verification failed")
}

// ----------------------------------------------------------------------------
// Double-redeem race test
// ----------------------------------------------------------------------------

func TestDoubleRedeemRace(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := newTestDB(t)
	store := pairing.NewStore(db)

	// Insert a pending token directly into the store.
	rootKey := newTestRootKey(t)
	sk := newTestSigningKey(t, rootKey)
	tok := newToken(time.Now(), rootKey)
	encoded, err := tok.Encode(sk)
	require.NoError(t, err)
	require.NoError(t, store.Insert(ctx, tok, encoded))

	const goroutines = 20
	results := make([]error, goroutines)
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := range goroutines {
		go func(idx int) {
			defer wg.Done()
			results[idx] = store.Redeem(ctx, tok.TokenID)
		}(i)
	}
	wg.Wait()

	successCount := 0
	alreadyRedeemedCount := 0
	for _, err := range results {
		if err == nil {
			successCount++
		} else if err == pairing.ErrAlreadyRedeemed {
			alreadyRedeemedCount++
		} else {
			t.Errorf("unexpected error: %v", err)
		}
	}

	assert.Equal(t, 1, successCount, "exactly one goroutine should have redeemed the token")
	assert.Equal(t, goroutines-1, alreadyRedeemedCount, "all other goroutines should get ErrAlreadyRedeemed")
}

// ----------------------------------------------------------------------------
// Store: ErrNotFound
// ----------------------------------------------------------------------------

func TestRedeemNonExistentToken(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := newTestDB(t)
	store := pairing.NewStore(db)

	err := store.Redeem(ctx, "does-not-exist")
	require.Error(t, err)
	assert.Equal(t, pairing.ErrNotFound, err)
}

// ----------------------------------------------------------------------------
// Store: ErrTokenExpired via Redeem
// ----------------------------------------------------------------------------

func TestRedeemExpiredTokenViaStore(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := newTestDB(t)
	store := pairing.NewStore(db)

	rootKey := newTestRootKey(t)
	sk := newTestSigningKey(t, rootKey)
	tok := newToken(time.Now(), rootKey)
	tok.ExpiresAt = time.Now().UTC().Add(-1 * time.Minute) // already expired
	encoded, err := tok.Encode(sk)
	require.NoError(t, err)
	require.NoError(t, store.Insert(ctx, tok, encoded))

	err = store.Redeem(ctx, tok.TokenID)
	require.Error(t, err)
	assert.Equal(t, pairing.ErrTokenExpired, err)
}

// ----------------------------------------------------------------------------
// Service.Generate — integration with stub Headscale + CA
// ----------------------------------------------------------------------------

type stubHeadscale struct {
	key string
	err error
}

func (s *stubHeadscale) MintPreAuthKey(_ context.Context, _ string, _ time.Duration) (string, error) {
	return s.key, s.err
}

type stubCA struct {
	fp   string
	cert *tls.Certificate
}

func (s *stubCA) Fingerprint() string { return s.fp }
func (s *stubCA) IssueDirectoryServingCert(_ context.Context) (*tls.Certificate, error) {
	if s.cert == nil {
		// Minimal self-signed tls.Certificate with one DER byte so certFingerprint works.
		s.cert = &tls.Certificate{Certificate: [][]byte{{0xCA, 0xFE}}}
	}
	return s.cert, nil
}

func TestServiceGenerate(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := newTestDB(t)
	rootKey := newTestRootKey(t)

	svc, err := pairing.NewService(pairing.Config{
		DB:                db,
		Headscale:         &stubHeadscale{key: "hskey-auth-test"},
		ClusterCA:         &stubCA{fp: "aabbcc"},
		RootSigningKey:    rootKey,
		DirectoryEndpoint: "https://dir.test:8443",
	})
	require.NoError(t, err)

	result, err := svc.Generate(ctx, "admin-1", []string{"recorder"}, "")
	require.NoError(t, err)
	assert.NotEmpty(t, result.Encoded)
	assert.NotEmpty(t, result.TokenID)

	// Decoded token must verify correctly.
	pub := svc.VerifyPublicKeyForDecode()
	decoded, err := pairing.Decode(result.Encoded, pub)
	require.NoError(t, err)
	assert.Equal(t, result.TokenID, decoded.TokenID)
	assert.Equal(t, "https://dir.test:8443", decoded.DirectoryEndpoint)
	assert.Equal(t, "hskey-auth-test", decoded.HeadscalePreAuthKey)
}

// ----------------------------------------------------------------------------
// Sweeper
// ----------------------------------------------------------------------------

func TestSweeperExpiresTokens(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	db := newTestDB(t)
	store := pairing.NewStore(db)
	metrics := pairing.NewMetrics()

	rootKey := newTestRootKey(t)
	sk := newTestSigningKey(t, rootKey)

	// Insert a token that is already past its TTL.
	tok := newToken(time.Now(), rootKey)
	tok.ExpiresAt = time.Now().UTC().Add(-1 * time.Minute)
	encoded, err := tok.Encode(sk)
	require.NoError(t, err)
	require.NoError(t, store.Insert(ctx, tok, encoded))

	// Run the sweeper once inline (tiny interval so it fires immediately in test).
	sweeperCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		pairing.RunSweeper(sweeperCtx, pairing.SweeperConfig{
			Store:    store,
			Interval: 10 * time.Millisecond,
			Metrics:  metrics,
		})
	}()
	// Wait long enough for at least one tick.
	time.Sleep(100 * time.Millisecond)
	cancel()
	<-done

	stored, err := store.Get(ctx, tok.TokenID)
	require.NoError(t, err)
	assert.Equal(t, pairing.StatusExpired, stored.Status)
	assert.GreaterOrEqual(t, metrics.Expired.Load(), uint64(1))
}
