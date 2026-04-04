package db

import (
	"bytes"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func openAuditTestDB(t *testing.T) *DB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	d, err := Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { d.Close() })
	return d
}

func insertAuditAt(t *testing.T, d *DB, action string, createdAt time.Time) {
	t.Helper()
	entry := &AuditEntry{
		UserID:       "u1",
		Username:     "admin",
		Action:       action,
		ResourceType: "system",
		ResourceID:   "",
		Details:      "test",
		IPAddress:    "127.0.0.1",
		CreatedAt:    createdAt.UTC().Format(timeFormat),
	}
	require.NoError(t, d.InsertAuditEntry(entry))
}

func TestDeleteGeneralAuditEntriesBefore(t *testing.T) {
	d := openAuditTestDB(t)
	now := time.Now().UTC()

	// Insert general entries (old and recent).
	insertAuditAt(t, d, "create", now.AddDate(0, 0, -100))
	insertAuditAt(t, d, "update", now.AddDate(0, 0, -95))
	insertAuditAt(t, d, "delete", now.AddDate(0, 0, -10))

	// Insert security entries (old).
	insertAuditAt(t, d, "login_failed", now.AddDate(0, 0, -100))
	insertAuditAt(t, d, "login", now.AddDate(0, 0, -95))

	// Delete general entries older than 90 days.
	cutoff := now.AddDate(0, 0, -90)
	n, err := d.DeleteGeneralAuditEntriesBefore(cutoff)
	require.NoError(t, err)
	require.Equal(t, int64(2), n) // create + update

	// Security entries should still exist.
	entries, total, err := d.QueryAuditLog(100, 0, "", "")
	require.NoError(t, err)
	require.Equal(t, 3, total) // delete (recent) + login_failed + login

	// Verify only security old + recent general remain.
	actions := map[string]bool{}
	for _, e := range entries {
		actions[e.Action] = true
	}
	require.True(t, actions["login_failed"])
	require.True(t, actions["login"])
	require.True(t, actions["delete"])
	require.False(t, actions["create"])
	require.False(t, actions["update"])
}

func TestDeleteSecurityAuditEntriesBefore(t *testing.T) {
	d := openAuditTestDB(t)
	now := time.Now().UTC()

	// Insert security entries (old and recent).
	insertAuditAt(t, d, "login_failed", now.AddDate(-2, 0, 0))
	insertAuditAt(t, d, "login", now.AddDate(0, 0, -10))

	// Insert a general entry (old, should not be touched).
	insertAuditAt(t, d, "create", now.AddDate(-2, 0, 0))

	// Delete security entries older than 365 days.
	cutoff := now.AddDate(0, 0, -365)
	n, err := d.DeleteSecurityAuditEntriesBefore(cutoff)
	require.NoError(t, err)
	require.Equal(t, int64(1), n) // old login_failed

	// The old general entry and recent login should remain.
	entries, total, err := d.QueryAuditLog(100, 0, "", "")
	require.NoError(t, err)
	require.Equal(t, 2, total)

	actions := map[string]bool{}
	for _, e := range entries {
		actions[e.Action] = true
	}
	require.True(t, actions["create"])
	require.True(t, actions["login"])
}

func TestQueryAuditLogByDateRange(t *testing.T) {
	d := openAuditTestDB(t)
	now := time.Now().UTC()

	insertAuditAt(t, d, "create", now.AddDate(0, 0, -5))
	insertAuditAt(t, d, "update", now.AddDate(0, 0, -3))
	insertAuditAt(t, d, "delete", now.AddDate(0, 0, -1))

	// Query last 4 days.
	from := now.AddDate(0, 0, -4)
	to := now
	entries, err := d.QueryAuditLogByDateRange(from, to, "", "")
	require.NoError(t, err)
	require.Len(t, entries, 2) // update + delete

	// With action filter.
	entries, err = d.QueryAuditLogByDateRange(from, to, "", "delete")
	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.Equal(t, "delete", entries[0].Action)
}

func TestWriteAuditCSV(t *testing.T) {
	entries := []*AuditEntry{
		{
			ID:           1,
			UserID:       "u1",
			Username:     "admin",
			Action:       "create",
			ResourceType: "camera",
			ResourceID:   "cam1",
			Details:      "Created camera",
			IPAddress:    "10.0.0.1",
			CreatedAt:    "2026-01-15T10:00:00.000Z",
		},
	}

	var buf bytes.Buffer
	err := WriteAuditCSV(&buf, entries)
	require.NoError(t, err)

	csv := buf.String()
	require.Contains(t, csv, "id,user_id,username,action,resource_type")
	require.Contains(t, csv, "1,u1,admin,create,camera,cam1,Created camera,10.0.0.1,2026-01-15T10:00:00.000Z")
}

func TestIsSecurityAction(t *testing.T) {
	require.True(t, isSecurityAction("login"))
	require.True(t, isSecurityAction("login_failed"))
	require.True(t, isSecurityAction("logout"))
	require.True(t, isSecurityAction("password_change"))
	require.False(t, isSecurityAction("create"))
	require.False(t, isSecurityAction("update"))
	require.False(t, isSecurityAction("delete"))
}
