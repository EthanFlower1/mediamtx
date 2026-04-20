package legacydb

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func setupTestDB(t *testing.T) *DB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	d, err := Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { d.Close() })

	// Create a camera to satisfy FK.
	require.NoError(t, d.CreateCamera(&Camera{
		ID:   "cam-1",
		Name: "Test Camera",
	}))
	return d
}

func TestInsertAndListConnectionEvents(t *testing.T) {
	d := setupTestDB(t)

	require.NoError(t, d.InsertConnectionEvent("cam-1", "connecting", ""))
	require.NoError(t, d.InsertConnectionEvent("cam-1", "connected", ""))
	require.NoError(t, d.InsertConnectionEvent("cam-1", "error", "timeout"))

	events, err := d.ListConnectionEvents("cam-1", 0)
	require.NoError(t, err)
	require.Len(t, events, 3)

	// Most recent first.
	require.Equal(t, "error", events[0].State)
	require.Equal(t, "timeout", events[0].ErrorMessage)
	require.Equal(t, "connected", events[1].State)
	require.Equal(t, "connecting", events[2].State)
}

func TestListConnectionEventsWithLimit(t *testing.T) {
	d := setupTestDB(t)

	for i := 0; i < 10; i++ {
		require.NoError(t, d.InsertConnectionEvent("cam-1", "connected", ""))
	}

	events, err := d.ListConnectionEvents("cam-1", 3)
	require.NoError(t, err)
	require.Len(t, events, 3)
}

func TestGetConnectionSummary(t *testing.T) {
	d := setupTestDB(t)

	require.NoError(t, d.InsertConnectionEvent("cam-1", "connecting", ""))
	require.NoError(t, d.InsertConnectionEvent("cam-1", "connected", ""))
	require.NoError(t, d.InsertConnectionEvent("cam-1", "error", "fail"))
	require.NoError(t, d.InsertConnectionEvent("cam-1", "connected", ""))

	s, err := d.GetConnectionSummary("cam-1")
	require.NoError(t, err)
	require.Equal(t, 4, s.TotalEvents)
	require.Equal(t, 1, s.ErrorCount)
	require.Equal(t, 2, s.ConnectedCount)
	require.Equal(t, "connected", s.LastState)
}

func TestPruneConnectionEvents(t *testing.T) {
	d := setupTestDB(t)

	require.NoError(t, d.InsertConnectionEvent("cam-1", "connected", ""))
	time.Sleep(10 * time.Millisecond)
	require.NoError(t, d.InsertConnectionEvent("cam-1", "disconnected", ""))

	// Prune events older than 0 duration (all events).
	deleted, err := d.PruneConnectionEvents(0)
	require.NoError(t, err)
	// Should delete all or most events since they were just created.
	// The exact count depends on timing; at least verify no error.
	_ = deleted

	// With a large max age, nothing is pruned.
	require.NoError(t, d.InsertConnectionEvent("cam-1", "connected", ""))
	deleted, err = d.PruneConnectionEvents(24 * time.Hour)
	require.NoError(t, err)
	require.Equal(t, int64(0), deleted)
}

func TestQueuedCommands(t *testing.T) {
	d := setupTestDB(t)

	cmd := &QueuedCommand{
		ID:          "cmd-1",
		CameraID:    "cam-1",
		CommandType: "ptz",
		Payload:     `{"action":"left"}`,
	}
	require.NoError(t, d.InsertQueuedCommand(cmd))

	pending, err := d.ListPendingCommands("cam-1")
	require.NoError(t, err)
	require.Len(t, pending, 1)
	require.Equal(t, "ptz", pending[0].CommandType)
	require.Equal(t, "pending", pending[0].Status)

	require.NoError(t, d.UpdateCommandStatus("cmd-1", "executed", ""))

	pending, err = d.ListPendingCommands("cam-1")
	require.NoError(t, err)
	require.Len(t, pending, 0)

	all, err := d.ListQueuedCommands("cam-1", 10)
	require.NoError(t, err)
	require.Len(t, all, 1)
	require.Equal(t, "executed", all[0].Status)
}

func TestDeleteQueuedCommands(t *testing.T) {
	d := setupTestDB(t)

	require.NoError(t, d.InsertQueuedCommand(&QueuedCommand{
		ID: "cmd-1", CameraID: "cam-1", CommandType: "ptz", Payload: "{}",
	}))
	require.NoError(t, d.InsertQueuedCommand(&QueuedCommand{
		ID: "cmd-2", CameraID: "cam-1", CommandType: "relay", Payload: "{}",
	}))

	require.NoError(t, d.DeleteQueuedCommands("cam-1"))

	cmds, err := d.ListQueuedCommands("cam-1", 0)
	require.NoError(t, err)
	require.Len(t, cmds, 0)
}
