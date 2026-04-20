package db

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestOpenRunsMigrations verifies that Open on a fresh :memory: database
// applies all known migrations and leaves the schema at the expected version.
func TestOpenRunsMigrations(t *testing.T) {
	ctx := context.Background()
	d, err := Open(ctx, ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = d.Close() })

	versions, err := d.AppliedVersions(ctx)
	require.NoError(t, err)
	// Update this assertion when new migrations are added.
	require.Equal(t, []int{1, 2}, versions, "expected migrations 0001 and 0002 applied")
}

// TestMigrateIdempotent verifies that calling Migrate twice does not error.
func TestMigrateIdempotent(t *testing.T) {
	ctx := context.Background()
	d, err := Open(ctx, ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = d.Close() })

	require.NoError(t, d.Migrate(ctx))
	require.NoError(t, d.Migrate(ctx))
}

// TestAssignedCamerasSchema exercises the assigned_cameras table: insert a
// representative row, read it back, and verify CHECK constraints reject bad
// values. Hot-path readers downstream (KAI-152/259) depend on the exact
// column set, so this test doubles as a schema contract.
func TestAssignedCamerasSchema(t *testing.T) {
	ctx := context.Background()
	d, err := Open(ctx, ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = d.Close() })

	_, err = d.ExecContext(ctx, `
		INSERT INTO assigned_cameras (
			id, tenant_id, name, manufacturer, model,
			ip, port, onvif_port, rtsp_url,
			rtsp_credentials_encrypted, rtsp_credentials_nonce,
			profile_token, enabled, recording_mode, schedule_cron,
			pre_roll_seconds, post_roll_seconds,
			hot_days, warm_days, cold_days, delete_after_days,
			archive_tier, last_applied_revision
		) VALUES (
			'cam-1', 'tenant-a', 'Front Door', 'Axis', 'P3245',
			'10.0.0.5', 554, 80, 'rtsp://10.0.0.5:554/axis-media/media.amp',
			X'DEADBEEF', X'CAFE',
			'Profile_1', 1, 'continuous', NULL,
			5, 10,
			7, 14, 30, 90,
			'sse-kms', 42
		)
	`)
	require.NoError(t, err)

	var (
		id, tenant, recordingMode, archiveTier string
		enabled                                int
		rev                                    int64
	)
	err = d.QueryRowContext(ctx, `
		SELECT id, tenant_id, enabled, recording_mode, archive_tier, last_applied_revision
		FROM assigned_cameras WHERE id = ?`, "cam-1",
	).Scan(&id, &tenant, &enabled, &recordingMode, &archiveTier, &rev)
	require.NoError(t, err)
	require.Equal(t, "cam-1", id)
	require.Equal(t, "tenant-a", tenant)
	require.Equal(t, 1, enabled)
	require.Equal(t, "continuous", recordingMode)
	require.Equal(t, "sse-kms", archiveTier)
	require.Equal(t, int64(42), rev)

	// CHECK: enabled must be 0 or 1.
	_, err = d.ExecContext(ctx,
		`INSERT INTO assigned_cameras (id, tenant_id, enabled) VALUES ('bad', 't', 7)`)
	require.Error(t, err, "enabled CHECK should reject non-boolean")

	// CHECK: recording_mode must be one of the four enum values.
	_, err = d.ExecContext(ctx,
		`INSERT INTO assigned_cameras (id, tenant_id, recording_mode) VALUES ('bad2', 't', 'sometimes')`)
	require.Error(t, err, "recording_mode CHECK should reject unknown value")

	// CHECK: archive_tier must be one of standard|sse-kms|cse-cmk.
	_, err = d.ExecContext(ctx,
		`INSERT INTO assigned_cameras (id, tenant_id, archive_tier) VALUES ('bad3', 't', 'glacier')`)
	require.Error(t, err, "archive_tier CHECK should reject unknown value")
}

// TestAssignedCamerasEnabledIndex confirms the hot-path index for enabled
// cameras exists. Raikada path-config generator (KAI-152) issues this query
// in its tight loop.
func TestAssignedCamerasEnabledIndex(t *testing.T) {
	ctx := context.Background()
	d, err := Open(ctx, ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = d.Close() })

	var name string
	err = d.QueryRowContext(ctx,
		`SELECT name FROM sqlite_master WHERE type='index' AND name='idx_assigned_cameras_enabled'`,
	).Scan(&name)
	require.NoError(t, err)
	require.Equal(t, "idx_assigned_cameras_enabled", name)
}

// TestLocalStateSchema exercises the KV table: upsert, read, update, and
// confirm PK uniqueness.
func TestLocalStateSchema(t *testing.T) {
	ctx := context.Background()
	d, err := Open(ctx, ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = d.Close() })

	_, err = d.ExecContext(ctx,
		`INSERT INTO local_state (key, value) VALUES (?, ?)`,
		"recorder_id", "rec-abc-123")
	require.NoError(t, err)

	var v string
	err = d.QueryRowContext(ctx,
		`SELECT value FROM local_state WHERE key = ?`, "recorder_id").Scan(&v)
	require.NoError(t, err)
	require.Equal(t, "rec-abc-123", v)

	// PK violation on duplicate key.
	_, err = d.ExecContext(ctx,
		`INSERT INTO local_state (key, value) VALUES (?, ?)`,
		"recorder_id", "different")
	require.Error(t, err, "duplicate key should violate PK")

	// Upsert pattern used by callers.
	_, err = d.ExecContext(ctx, `
		INSERT INTO local_state (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = CURRENT_TIMESTAMP`,
		"recorder_id", "rec-xyz-999")
	require.NoError(t, err)

	err = d.QueryRowContext(ctx,
		`SELECT value FROM local_state WHERE key = ?`, "recorder_id").Scan(&v)
	require.NoError(t, err)
	require.Equal(t, "rec-xyz-999", v)
}

// TestMigrationsRoundTrip verifies every migration has a working down.sql by
// applying all migrations, rolling them back one by one, and confirming the
// schema is empty at the end (no lingering tables beyond schema_migrations).
func TestMigrationsRoundTrip(t *testing.T) {
	ctx := context.Background()
	d, err := Open(ctx, ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = d.Close() })

	versions, err := d.AppliedVersions(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, versions)

	// Roll back each migration in reverse order.
	for i := 0; i < len(versions); i++ {
		require.NoError(t, d.Rollback(ctx))
	}

	post, err := d.AppliedVersions(ctx)
	require.NoError(t, err)
	require.Empty(t, post)

	// Verify assigned_cameras and local_state are actually gone.
	for _, tbl := range []string{"assigned_cameras", "local_state"} {
		var name string
		err := d.QueryRowContext(ctx,
			`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, tbl).Scan(&name)
		require.Error(t, err, "table %q should be dropped after rollback", tbl)
	}

	// And Migrate should bring them back cleanly.
	require.NoError(t, d.Migrate(ctx))
	versions2, err := d.AppliedVersions(ctx)
	require.NoError(t, err)
	require.Equal(t, versions, versions2)
}
