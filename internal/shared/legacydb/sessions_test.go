package legacydb

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCreateRefreshTokenWithDeviceInfo(t *testing.T) {
	d := newTestDB(t)
	u := createTestUser(t, d)

	tok := &RefreshToken{
		UserID:    u.ID,
		TokenHash: "device-hash",
		ExpiresAt: "2027-01-01T00:00:00.000Z",
		IPAddress: "192.168.1.100",
		UserAgent: "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)",
		DeviceName: "Mac",
	}
	err := d.CreateRefreshToken(tok)
	require.NoError(t, err)
	require.NotEmpty(t, tok.ID)
	require.NotEmpty(t, tok.CreatedAt)
	require.NotEmpty(t, tok.LastActivity)

	got, err := d.GetRefreshToken("device-hash")
	require.NoError(t, err)
	require.Equal(t, "192.168.1.100", got.IPAddress)
	require.Equal(t, "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)", got.UserAgent)
	require.Equal(t, "Mac", got.DeviceName)
}

func TestUpdateSessionActivity(t *testing.T) {
	d := newTestDB(t)
	u := createTestUser(t, d)

	tok := &RefreshToken{
		UserID:    u.ID,
		TokenHash: "activity-hash",
		ExpiresAt: "2027-01-01T00:00:00.000Z",
		IPAddress: "10.0.0.1",
	}
	require.NoError(t, d.CreateRefreshToken(tok))

	// Update activity.
	err := d.UpdateSessionActivity(tok.ID, "10.0.0.2")
	require.NoError(t, err)

	got, err := d.GetRefreshToken("activity-hash")
	require.NoError(t, err)
	require.Equal(t, "10.0.0.2", got.IPAddress)
	require.NotEmpty(t, got.LastActivity)
}

func TestListActiveSessions(t *testing.T) {
	d := newTestDB(t)
	u := createTestUser(t, d)

	// Create two active and one revoked session.
	for _, h := range []string{"session-a", "session-b"} {
		require.NoError(t, d.CreateRefreshToken(&RefreshToken{
			UserID:     u.ID,
			TokenHash:  h,
			ExpiresAt:  "2027-06-01T00:00:00.000Z",
			IPAddress:  "10.0.0.1",
			DeviceName: "Test",
		}))
	}
	revoked := &RefreshToken{
		UserID:    u.ID,
		TokenHash: "session-revoked",
		ExpiresAt: "2027-06-01T00:00:00.000Z",
	}
	require.NoError(t, d.CreateRefreshToken(revoked))
	require.NoError(t, d.RevokeRefreshToken(revoked.ID))

	// List all active sessions.
	sessions, err := d.ListActiveSessions("")
	require.NoError(t, err)
	require.Len(t, sessions, 2)

	// List for specific user.
	sessions, err = d.ListActiveSessions(u.ID)
	require.NoError(t, err)
	require.Len(t, sessions, 2)

	// Each session should include username.
	for _, s := range sessions {
		require.Equal(t, "tokenuser", s.Username)
		require.Equal(t, u.ID, s.UserID)
	}
}

func TestGetRefreshTokenByID(t *testing.T) {
	d := newTestDB(t)
	u := createTestUser(t, d)

	tok := &RefreshToken{
		UserID:    u.ID,
		TokenHash: "byid-hash",
		ExpiresAt: "2027-01-01T00:00:00.000Z",
	}
	require.NoError(t, d.CreateRefreshToken(tok))

	got, err := d.GetRefreshTokenByID(tok.ID)
	require.NoError(t, err)
	require.Equal(t, tok.ID, got.ID)
	require.Equal(t, u.ID, got.UserID)

	_, err = d.GetRefreshTokenByID("nonexistent")
	require.ErrorIs(t, err, ErrNotFound)
}

func TestRevokeIdleSessions(t *testing.T) {
	d := newTestDB(t)
	u := createTestUser(t, d)

	// Create a session with old activity.
	old := &RefreshToken{
		UserID:       u.ID,
		TokenHash:    "idle-old",
		ExpiresAt:    "2027-06-01T00:00:00.000Z",
		LastActivity: time.Now().UTC().Add(-2 * time.Hour).Format(timeFormat),
	}
	require.NoError(t, d.CreateRefreshToken(old))

	// Create a session with recent activity.
	recent := &RefreshToken{
		UserID:       u.ID,
		TokenHash:    "idle-recent",
		ExpiresAt:    "2027-06-01T00:00:00.000Z",
		LastActivity: time.Now().UTC().Format(timeFormat),
	}
	require.NoError(t, d.CreateRefreshToken(recent))

	// Revoke sessions idle for more than 1 hour.
	n, err := d.RevokeIdleSessions(1 * time.Hour)
	require.NoError(t, err)
	require.Equal(t, int64(1), n)

	// Old session should be revoked.
	got, err := d.GetRefreshToken("idle-old")
	require.NoError(t, err)
	require.NotNil(t, got.RevokedAt)

	// Recent session should still be active.
	got, err = d.GetRefreshToken("idle-recent")
	require.NoError(t, err)
	require.Nil(t, got.RevokedAt)
}
