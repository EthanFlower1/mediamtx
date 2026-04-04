package db

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func openExportTestDB(t *testing.T) *DB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	d, err := Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { d.Close() })
	return d
}

func TestCreateAndGetBulkExportJob(t *testing.T) {
	d := openExportTestDB(t)

	items := []*BulkExportItem{
		{CameraID: "cam-1", CameraName: "Front Door", StartTime: "2026-01-01T00:00:00Z", EndTime: "2026-01-01T01:00:00Z"},
		{CameraID: "cam-2", CameraName: "Back Yard", StartTime: "2026-01-01T00:00:00Z", EndTime: "2026-01-01T02:00:00Z"},
	}

	job := &BulkExportJob{}
	err := d.CreateBulkExportJob(job, items)
	require.NoError(t, err)
	require.NotEmpty(t, job.ID)
	require.Equal(t, "pending", job.Status)
	require.Equal(t, 2, job.TotalItems)

	// Items should have IDs assigned.
	for _, item := range items {
		require.NotEmpty(t, item.ID)
		require.Equal(t, job.ID, item.JobID)
	}

	// Retrieve job.
	got, err := d.GetBulkExportJob(job.ID)
	require.NoError(t, err)
	require.Equal(t, job.ID, got.ID)
	require.Equal(t, 2, got.TotalItems)
	require.Equal(t, 0, got.CompletedItems)

	// Retrieve items.
	gotItems, err := d.GetBulkExportItems(job.ID)
	require.NoError(t, err)
	require.Len(t, gotItems, 2)
}

func TestUpdateBulkExportItemStatus(t *testing.T) {
	d := openExportTestDB(t)

	items := []*BulkExportItem{
		{CameraID: "cam-1", CameraName: "Cam 1", StartTime: "2026-01-01T00:00:00Z", EndTime: "2026-01-01T01:00:00Z"},
	}
	job := &BulkExportJob{}
	require.NoError(t, d.CreateBulkExportJob(job, items))

	// Mark item as completed.
	err := d.UpdateBulkExportItemStatus(items[0].ID, "completed", 3, 1024, nil)
	require.NoError(t, err)

	// Job counters should update.
	got, err := d.GetBulkExportJob(job.ID)
	require.NoError(t, err)
	require.Equal(t, 1, got.CompletedItems)
	require.Equal(t, int64(1024), got.TotalBytes)
}

func TestUpdateBulkExportItemStatusFailed(t *testing.T) {
	d := openExportTestDB(t)

	items := []*BulkExportItem{
		{CameraID: "cam-1", CameraName: "Cam 1", StartTime: "2026-01-01T00:00:00Z", EndTime: "2026-01-01T01:00:00Z"},
	}
	job := &BulkExportJob{}
	require.NoError(t, d.CreateBulkExportJob(job, items))

	errMsg := "disk full"
	err := d.UpdateBulkExportItemStatus(items[0].ID, "failed", 0, 0, &errMsg)
	require.NoError(t, err)

	got, err := d.GetBulkExportJob(job.ID)
	require.NoError(t, err)
	require.Equal(t, 1, got.FailedItems)
}

func TestCompleteBulkExportJob(t *testing.T) {
	d := openExportTestDB(t)

	job := &BulkExportJob{}
	require.NoError(t, d.CreateBulkExportJob(job, []*BulkExportItem{
		{CameraID: "cam-1", CameraName: "Cam 1", StartTime: "2026-01-01T00:00:00Z", EndTime: "2026-01-01T01:00:00Z"},
	}))

	zipPath := "/tmp/export.zip"
	err := d.CompleteBulkExportJob(job.ID, "completed", &zipPath, nil)
	require.NoError(t, err)

	got, err := d.GetBulkExportJob(job.ID)
	require.NoError(t, err)
	require.Equal(t, "completed", got.Status)
	require.NotNil(t, got.ZipPath)
	require.Equal(t, zipPath, *got.ZipPath)
	require.NotNil(t, got.CompletedAt)
}

func TestListBulkExportJobs(t *testing.T) {
	d := openExportTestDB(t)

	// Create two jobs.
	for i := 0; i < 2; i++ {
		job := &BulkExportJob{}
		require.NoError(t, d.CreateBulkExportJob(job, []*BulkExportItem{
			{CameraID: "cam-1", CameraName: "Cam 1", StartTime: "2026-01-01T00:00:00Z", EndTime: "2026-01-01T01:00:00Z"},
		}))
	}

	jobs, err := d.ListBulkExportJobs(10)
	require.NoError(t, err)
	require.Len(t, jobs, 2)
}

func TestDeleteBulkExportJob(t *testing.T) {
	d := openExportTestDB(t)

	job := &BulkExportJob{}
	require.NoError(t, d.CreateBulkExportJob(job, []*BulkExportItem{
		{CameraID: "cam-1", CameraName: "Cam 1", StartTime: "2026-01-01T00:00:00Z", EndTime: "2026-01-01T01:00:00Z"},
	}))

	err := d.DeleteBulkExportJob(job.ID)
	require.NoError(t, err)

	_, err = d.GetBulkExportJob(job.ID)
	require.ErrorIs(t, err, ErrNotFound)

	// Items should also be deleted (cascade).
	items, err := d.GetBulkExportItems(job.ID)
	require.NoError(t, err)
	require.Empty(t, items)
}

func TestDeleteBulkExportJobNotFound(t *testing.T) {
	d := openExportTestDB(t)
	err := d.DeleteBulkExportJob("nonexistent")
	require.ErrorIs(t, err, ErrNotFound)
}
