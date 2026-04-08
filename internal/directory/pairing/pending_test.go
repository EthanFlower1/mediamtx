package pairing_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	directorydb "github.com/bluenviron/mediamtx/internal/directory/db"
	"github.com/bluenviron/mediamtx/internal/directory/pairing"
)

// openTestDB opens an in-memory directory DB with all migrations applied.
func openTestDB(t *testing.T) *directorydb.DB {
	t.Helper()
	db, err := directorydb.Open(context.Background(), ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestPendingStore_CreateAndGet(t *testing.T) {
	db := openTestDB(t)
	store := pairing.NewPendingStore(db)

	req, err := store.Create(context.Background(), "recorder-a.local", "192.168.1.10", "camera room B", nil)
	require.NoError(t, err)
	require.NotEmpty(t, req.ID)
	require.Equal(t, "recorder-a.local", req.RecorderHostname)
	require.Equal(t, []string{"recorder"}, req.RequestedRoles) // default
	require.Equal(t, pairing.PendingStatusPending, req.Status)
	require.True(t, req.ExpiresAt.After(time.Now()))

	got, err := store.Get(context.Background(), req.ID)
	require.NoError(t, err)
	require.Equal(t, req.ID, got.ID)
	require.Equal(t, pairing.PendingStatusPending, got.Status)
}

func TestPendingStore_ListPending(t *testing.T) {
	db := openTestDB(t)
	store := pairing.NewPendingStore(db)

	_, _ = store.Create(context.Background(), "rec-1.local", "10.0.0.1", "", nil)
	_, _ = store.Create(context.Background(), "rec-2.local", "10.0.0.2", "", nil)

	list, err := store.ListPending(context.Background())
	require.NoError(t, err)
	require.Len(t, list, 2)
}

func TestPendingStore_Deny(t *testing.T) {
	db := openTestDB(t)
	store := pairing.NewPendingStore(db)

	req, err := store.Create(context.Background(), "rec-deny.local", "10.0.0.3", "", nil)
	require.NoError(t, err)

	err = store.Deny(context.Background(), req.ID, pairing.UserID("admin-1"))
	require.NoError(t, err)

	got, err := store.Get(context.Background(), req.ID)
	require.NoError(t, err)
	require.Equal(t, pairing.PendingStatusDenied, got.Status)
	require.Equal(t, pairing.UserID("admin-1"), got.DecidedBy)
	require.NotNil(t, got.DecidedAt)

	// Deny again → already decided.
	err = store.Deny(context.Background(), req.ID, pairing.UserID("admin-1"))
	require.ErrorIs(t, err, pairing.ErrPendingAlreadyDecided)
}

func TestPendingStore_Approve(t *testing.T) {
	db := openTestDB(t)
	store := pairing.NewPendingStore(db)

	req, err := store.Create(context.Background(), "rec-approve.local", "10.0.0.4", "", nil)
	require.NoError(t, err)

	err = store.Approve(context.Background(), req.ID, "fake-token-uuid", pairing.UserID("admin-2"))
	require.NoError(t, err)

	got, err := store.Get(context.Background(), req.ID)
	require.NoError(t, err)
	require.Equal(t, pairing.PendingStatusApproved, got.Status)
	require.Equal(t, "fake-token-uuid", got.TokenID)
}

func TestPendingStore_MarkExpired(t *testing.T) {
	db := openTestDB(t)
	store := pairing.NewPendingStore(db)

	req, err := store.Create(context.Background(), "rec-exp.local", "10.0.0.5", "", nil)
	require.NoError(t, err)

	// Manually push expires_at into the past.
	_, err = db.ExecContext(context.Background(),
		"UPDATE pending_pairing_requests SET expires_at = ? WHERE id = ?",
		time.Now().UTC().Add(-1*time.Second).Format(time.RFC3339),
		req.ID,
	)
	require.NoError(t, err)

	n, err := store.MarkExpired(context.Background())
	require.NoError(t, err)
	require.Equal(t, int64(1), n)

	got, err := store.Get(context.Background(), req.ID)
	require.NoError(t, err)
	require.Equal(t, pairing.PendingStatusExpired, got.Status)
}

func TestPendingStore_ErrNotFound(t *testing.T) {
	db := openTestDB(t)
	store := pairing.NewPendingStore(db)

	_, err := store.Get(context.Background(), "does-not-exist")
	require.ErrorIs(t, err, pairing.ErrPendingNotFound)
}

func TestPendingStore_CreateRequiresHostname(t *testing.T) {
	db := openTestDB(t)
	store := pairing.NewPendingStore(db)

	_, err := store.Create(context.Background(), "", "10.0.0.6", "", nil)
	require.Error(t, err)
}
