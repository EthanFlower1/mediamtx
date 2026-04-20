package legacydb

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func createTestUser(t *testing.T, d *DB) *User {
	t.Helper()
	u := &User{Username: "tokenuser", PasswordHash: "hash"}
	require.NoError(t, d.CreateUser(u))
	return u
}

func TestCreateRefreshToken(t *testing.T) {
	d := newTestDB(t)
	u := createTestUser(t, d)

	tok := &RefreshToken{
		UserID:    u.ID,
		TokenHash: "abc123hash",
		ExpiresAt: "2026-01-01T00:00:00.000Z",
	}
	err := d.CreateRefreshToken(tok)
	require.NoError(t, err)
	require.NotEmpty(t, tok.ID)
}

func TestGetRefreshToken(t *testing.T) {
	d := newTestDB(t)
	u := createTestUser(t, d)

	tok := &RefreshToken{
		UserID:    u.ID,
		TokenHash: "lookup-hash",
		ExpiresAt: "2026-06-01T00:00:00.000Z",
	}
	require.NoError(t, d.CreateRefreshToken(tok))

	got, err := d.GetRefreshToken("lookup-hash")
	require.NoError(t, err)
	require.Equal(t, tok.ID, got.ID)
	require.Equal(t, u.ID, got.UserID)
	require.Nil(t, got.RevokedAt)

	_, err = d.GetRefreshToken("nonexistent-hash")
	require.ErrorIs(t, err, ErrNotFound)
}

func TestRevokeRefreshToken(t *testing.T) {
	d := newTestDB(t)
	u := createTestUser(t, d)

	tok := &RefreshToken{
		UserID:    u.ID,
		TokenHash: "revoke-me",
		ExpiresAt: "2026-06-01T00:00:00.000Z",
	}
	require.NoError(t, d.CreateRefreshToken(tok))

	require.NoError(t, d.RevokeRefreshToken(tok.ID))

	got, err := d.GetRefreshToken("revoke-me")
	require.NoError(t, err)
	require.NotNil(t, got.RevokedAt)

	// Revoking a nonexistent token should return ErrNotFound.
	require.ErrorIs(t, d.RevokeRefreshToken("nonexistent"), ErrNotFound)
}

func TestRevokeAllUserTokens(t *testing.T) {
	d := newTestDB(t)
	u := createTestUser(t, d)

	for i, h := range []string{"hash-a", "hash-b", "hash-c"} {
		_ = i
		require.NoError(t, d.CreateRefreshToken(&RefreshToken{
			UserID:    u.ID,
			TokenHash: h,
			ExpiresAt: "2026-06-01T00:00:00.000Z",
		}))
	}

	require.NoError(t, d.RevokeAllUserTokens(u.ID))

	for _, h := range []string{"hash-a", "hash-b", "hash-c"} {
		got, err := d.GetRefreshToken(h)
		require.NoError(t, err)
		require.NotNil(t, got.RevokedAt)
	}
}

func TestCleanExpiredTokens(t *testing.T) {
	d := newTestDB(t)
	u := createTestUser(t, d)

	// One expired, one not.
	require.NoError(t, d.CreateRefreshToken(&RefreshToken{
		UserID:    u.ID,
		TokenHash: "expired",
		ExpiresAt: "2020-01-01T00:00:00.000Z",
	}))
	require.NoError(t, d.CreateRefreshToken(&RefreshToken{
		UserID:    u.ID,
		TokenHash: "valid",
		ExpiresAt: "2030-01-01T00:00:00.000Z",
	}))

	require.NoError(t, d.CleanExpiredTokens(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)))

	_, err := d.GetRefreshToken("expired")
	require.ErrorIs(t, err, ErrNotFound)

	got, err := d.GetRefreshToken("valid")
	require.NoError(t, err)
	require.Equal(t, "valid", got.TokenHash)
}

func TestConfigGetSetDelete(t *testing.T) {
	d := newTestDB(t)

	// Get nonexistent key.
	_, err := d.GetConfig("theme")
	require.ErrorIs(t, err, ErrNotFound)

	// Set and get.
	require.NoError(t, d.SetConfig("theme", "dark"))
	val, err := d.GetConfig("theme")
	require.NoError(t, err)
	require.Equal(t, "dark", val)

	// Upsert.
	require.NoError(t, d.SetConfig("theme", "light"))
	val, err = d.GetConfig("theme")
	require.NoError(t, err)
	require.Equal(t, "light", val)

	// Delete.
	require.NoError(t, d.DeleteConfig("theme"))
	_, err = d.GetConfig("theme")
	require.ErrorIs(t, err, ErrNotFound)

	// Delete nonexistent.
	require.ErrorIs(t, d.DeleteConfig("theme"), ErrNotFound)
}
