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
