package revocation

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	_ "modernc.org/sqlite"
)

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	// Apply migration 0003 schema directly.
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS revoked_tokens (
			jti           TEXT     NOT NULL PRIMARY KEY,
			recorder_id   TEXT     NOT NULL DEFAULT '',
			tenant_id     TEXT     NOT NULL DEFAULT '',
			revoked_by    TEXT     NOT NULL,
			reason        TEXT     NOT NULL DEFAULT '',
			revoked_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			expires_at    DATETIME NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_revoked_tokens_recorder
			ON revoked_tokens (recorder_id);
		CREATE INDEX IF NOT EXISTS idx_revoked_tokens_expires
			ON revoked_tokens (expires_at);
	`)
	require.NoError(t, err)
	return db
}

func TestStoreRevokeAndCheck(t *testing.T) {
	db := openTestDB(t)
	s := NewStore(db)
	ctx := context.Background()

	// Not revoked initially.
	revoked, err := s.IsRevoked(ctx, "jti-1")
	require.NoError(t, err)
	require.False(t, revoked)

	// Revoke it.
	err = s.Revoke(ctx, RevokedToken{
		JTI:        "jti-1",
		RecorderID: "rec-A",
		TenantID:   "tenant-1",
		RevokedBy:  "admin-1",
		Reason:     "test",
		RevokedAt:  time.Now(),
		ExpiresAt:  time.Now().Add(time.Hour),
	})
	require.NoError(t, err)

	// Now revoked.
	revoked, err = s.IsRevoked(ctx, "jti-1")
	require.NoError(t, err)
	require.True(t, revoked)

	// Other JTIs still not revoked.
	revoked, err = s.IsRevoked(ctx, "jti-2")
	require.NoError(t, err)
	require.False(t, revoked)
}

func TestStoreRevokeIdempotent(t *testing.T) {
	db := openTestDB(t)
	s := NewStore(db)
	ctx := context.Background()

	tok := RevokedToken{
		JTI:        "jti-dup",
		RecorderID: "rec-A",
		RevokedBy:  "admin-1",
		RevokedAt:  time.Now(),
		ExpiresAt:  time.Now().Add(time.Hour),
	}

	require.NoError(t, s.Revoke(ctx, tok))
	require.NoError(t, s.Revoke(ctx, tok)) // should not error

	n, err := s.Count(ctx)
	require.NoError(t, err)
	require.Equal(t, int64(1), n)
}

func TestStoreRevokeBatch(t *testing.T) {
	db := openTestDB(t)
	s := NewStore(db)
	ctx := context.Background()
	now := time.Now()

	tokens := []RevokedToken{
		{JTI: "b1", RecorderID: "rec-A", RevokedBy: "admin", RevokedAt: now, ExpiresAt: now.Add(time.Hour)},
		{JTI: "b2", RecorderID: "rec-A", RevokedBy: "admin", RevokedAt: now, ExpiresAt: now.Add(time.Hour)},
		{JTI: "b3", RecorderID: "rec-B", RevokedBy: "admin", RevokedAt: now, ExpiresAt: now.Add(time.Hour)},
	}

	n, err := s.RevokeBatch(ctx, tokens)
	require.NoError(t, err)
	require.Equal(t, int64(3), n)

	for _, jti := range []string{"b1", "b2", "b3"} {
		ok, err := s.IsRevoked(ctx, jti)
		require.NoError(t, err)
		require.True(t, ok, "expected %s to be revoked", jti)
	}
}

func TestStoreListByRecorder(t *testing.T) {
	db := openTestDB(t)
	s := NewStore(db)
	ctx := context.Background()
	now := time.Now()

	// Insert tokens for two recorders.
	require.NoError(t, s.Revoke(ctx, RevokedToken{
		JTI: "r1", RecorderID: "rec-A", RevokedBy: "admin", RevokedAt: now, ExpiresAt: now.Add(time.Hour),
	}))
	require.NoError(t, s.Revoke(ctx, RevokedToken{
		JTI: "r2", RecorderID: "rec-A", RevokedBy: "admin", RevokedAt: now.Add(time.Second), ExpiresAt: now.Add(time.Hour),
	}))
	require.NoError(t, s.Revoke(ctx, RevokedToken{
		JTI: "r3", RecorderID: "rec-B", RevokedBy: "admin", RevokedAt: now, ExpiresAt: now.Add(time.Hour),
	}))

	list, err := s.ListByRecorder(ctx, "rec-A")
	require.NoError(t, err)
	require.Len(t, list, 2)
	// Ordered by revoked_at DESC.
	require.Equal(t, "r2", list[0].JTI)
	require.Equal(t, "r1", list[1].JTI)

	listB, err := s.ListByRecorder(ctx, "rec-B")
	require.NoError(t, err)
	require.Len(t, listB, 1)
}

func TestStorePurgeExpired(t *testing.T) {
	db := openTestDB(t)
	s := NewStore(db)
	ctx := context.Background()
	now := time.Now()

	// One expired, one still valid.
	require.NoError(t, s.Revoke(ctx, RevokedToken{
		JTI: "old", RecorderID: "rec", RevokedBy: "admin",
		RevokedAt: now.Add(-2 * time.Hour), ExpiresAt: now.Add(-time.Hour),
	}))
	require.NoError(t, s.Revoke(ctx, RevokedToken{
		JTI: "new", RecorderID: "rec", RevokedBy: "admin",
		RevokedAt: now, ExpiresAt: now.Add(time.Hour),
	}))

	n, err := s.Count(ctx)
	require.NoError(t, err)
	require.Equal(t, int64(2), n)

	purged, err := s.PurgeExpired(ctx, now)
	require.NoError(t, err)
	require.Equal(t, int64(1), purged)

	n, err = s.Count(ctx)
	require.NoError(t, err)
	require.Equal(t, int64(1), n)

	// The "new" one should still be revoked.
	ok, err := s.IsRevoked(ctx, "new")
	require.NoError(t, err)
	require.True(t, ok)

	// The "old" one should be gone.
	ok, err = s.IsRevoked(ctx, "old")
	require.NoError(t, err)
	require.False(t, ok)
}

func TestStoreIsRecorderRevoked(t *testing.T) {
	db := openTestDB(t)
	s := NewStore(db)
	ctx := context.Background()
	now := time.Now()

	// Not revoked initially.
	revoked, err := s.IsRecorderRevoked(ctx, "rec-A")
	require.NoError(t, err)
	require.False(t, revoked)

	// Insert a sentinel.
	require.NoError(t, s.Revoke(ctx, RevokedToken{
		JTI:        "revoke-all:rec-A:" + now.Format(time.RFC3339Nano),
		RecorderID: "rec-A",
		RevokedBy:  "admin",
		RevokedAt:  now,
		ExpiresAt:  now.Add(30 * 24 * time.Hour),
	}))

	revoked, err = s.IsRecorderRevoked(ctx, "rec-A")
	require.NoError(t, err)
	require.True(t, revoked)

	// Different recorder not affected.
	revoked, err = s.IsRecorderRevoked(ctx, "rec-B")
	require.NoError(t, err)
	require.False(t, revoked)
}
