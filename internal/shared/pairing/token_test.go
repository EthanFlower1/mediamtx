package pairing_test

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/shared/pairing"
)

func newSharedRootKey(t *testing.T) ed25519.PrivateKey {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	return priv
}

func newSharedSigningKey(t *testing.T, rootKey ed25519.PrivateKey) ed25519.PrivateKey {
	t.Helper()
	sk, err := pairing.NewSigningKey(rootKey)
	require.NoError(t, err)
	return sk
}

func newSharedToken(now time.Time) *pairing.PairingToken {
	return &pairing.PairingToken{
		TokenID:              "test-token-id",
		DirectoryEndpoint:    "https://dir.local:8443",
		HeadscalePreAuthKey:  "hskey-auth-deadbeef",
		StepCAFingerprint:    "aabbcc",
		StepCAEnrollToken:    "enroll-tok",
		DirectoryFingerprint: "ddeeff",
		SuggestedRoles:       []string{"recorder"},
		ExpiresAt:            now.Add(15 * time.Minute),
		SignedBy:             "admin-user-1",
		CloudTenantBinding:   "",
	}
}

func TestSharedEncodeDecodeRoundTrip(t *testing.T) {
	t.Parallel()
	rootKey := newSharedRootKey(t)
	sk := newSharedSigningKey(t, rootKey)

	tok := newSharedToken(time.Now())
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

func TestSharedDecodeRejectsExpiredToken(t *testing.T) {
	t.Parallel()
	rootKey := newSharedRootKey(t)
	sk := newSharedSigningKey(t, rootKey)

	tok := newSharedToken(time.Now())
	tok.ExpiresAt = time.Now().UTC().Add(-1 * time.Minute)
	encoded, err := tok.Encode(sk)
	require.NoError(t, err)

	pub := pairing.VerifyPublicKey(sk)
	_, err = pairing.Decode(encoded, pub)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expired")
}

func TestSharedDecodeRejectsTamperedToken(t *testing.T) {
	t.Parallel()
	rootKey := newSharedRootKey(t)
	sk := newSharedSigningKey(t, rootKey)

	tok := newSharedToken(time.Now())
	encoded, err := tok.Encode(sk)
	require.NoError(t, err)

	tampered := []byte(encoded)
	if len(tampered) > 10 {
		tampered[5] ^= 0xFF
	}

	pub := pairing.VerifyPublicKey(sk)
	_, err = pairing.Decode(string(tampered), pub)
	require.Error(t, err)
}

func TestSharedDecodeRejectsCrossDirectoryToken(t *testing.T) {
	t.Parallel()
	rootKeyA := newSharedRootKey(t)
	skA := newSharedSigningKey(t, rootKeyA)

	tok := newSharedToken(time.Now())
	encoded, err := tok.Encode(skA)
	require.NoError(t, err)

	rootKeyB := newSharedRootKey(t)
	skB := newSharedSigningKey(t, rootKeyB)
	pubB := pairing.VerifyPublicKey(skB)

	_, err = pairing.Decode(encoded, pubB)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "signature verification failed")
}

func TestSharedDecodeTokenUnsafe(t *testing.T) {
	t.Parallel()
	rootKey := newSharedRootKey(t)
	sk := newSharedSigningKey(t, rootKey)

	tok := newSharedToken(time.Now())
	encoded, err := tok.Encode(sk)
	require.NoError(t, err)

	decoded, pub, err := pairing.DecodeTokenUnsafe(encoded)
	require.NoError(t, err)
	assert.Equal(t, tok.TokenID, decoded.TokenID)
	assert.Nil(t, pub, "DecodeTokenUnsafe returns nil public key")
}
