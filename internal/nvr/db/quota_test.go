package db

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpsertAndGetStorageQuota(t *testing.T) {
	d := openTestDB(t)

	q := &StorageQuota{
		ID:              "global",
		Name:            "Global Quota",
		QuotaBytes:      1099511627776, // 1 TB
		WarningPercent:  80,
		CriticalPercent: 90,
		Enabled:         true,
	}
	require.NoError(t, d.UpsertStorageQuota(q))

	got, err := d.GetStorageQuota("global")
	require.NoError(t, err)
	assert.Equal(t, "global", got.ID)
	assert.Equal(t, int64(1099511627776), got.QuotaBytes)
	assert.Equal(t, 80, got.WarningPercent)
	assert.Equal(t, 90, got.CriticalPercent)
	assert.True(t, got.Enabled)

	// Update existing.
	q.QuotaBytes = 2199023255552 // 2 TB
	require.NoError(t, d.UpsertStorageQuota(q))

	got, err = d.GetStorageQuota("global")
	require.NoError(t, err)
	assert.Equal(t, int64(2199023255552), got.QuotaBytes)
}

func TestListStorageQuotas(t *testing.T) {
	d := openTestDB(t)

	q1 := &StorageQuota{ID: "global", Name: "Global", QuotaBytes: 1000, Enabled: true}
	q2 := &StorageQuota{ID: "path-1", Name: "NAS Path", QuotaBytes: 500, Enabled: true}
	require.NoError(t, d.UpsertStorageQuota(q1))
	require.NoError(t, d.UpsertStorageQuota(q2))

	quotas, err := d.ListStorageQuotas()
	require.NoError(t, err)
	assert.Len(t, quotas, 2)
}

func TestDeleteStorageQuota(t *testing.T) {
	d := openTestDB(t)

	q := &StorageQuota{ID: "test", Name: "Test", QuotaBytes: 100, Enabled: true}
	require.NoError(t, d.UpsertStorageQuota(q))

	require.NoError(t, d.DeleteStorageQuota("test"))
	_, err := d.GetStorageQuota("test")
	assert.ErrorIs(t, err, ErrNotFound)

	assert.ErrorIs(t, d.DeleteStorageQuota("nonexistent"), ErrNotFound)
}

func TestCameraQuotaFields(t *testing.T) {
	d := openTestDB(t)

	cam := &Camera{
		Name:                 "quota-cam",
		QuotaBytes:           107374182400, // 100 GB
		QuotaWarningPercent:  75,
		QuotaCriticalPercent: 85,
	}
	require.NoError(t, d.CreateCamera(cam))

	got, err := d.GetCamera(cam.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(107374182400), got.QuotaBytes)
	assert.Equal(t, 75, got.QuotaWarningPercent)
	assert.Equal(t, 85, got.QuotaCriticalPercent)
}

func TestUpdateCameraQuota(t *testing.T) {
	d := openTestDB(t)

	cam := &Camera{Name: "test-cam"}
	require.NoError(t, d.CreateCamera(cam))

	require.NoError(t, d.UpdateCameraQuota(cam.ID, 53687091200, 70, 85))

	got, err := d.GetCamera(cam.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(53687091200), got.QuotaBytes)
	assert.Equal(t, 70, got.QuotaWarningPercent)
	assert.Equal(t, 85, got.QuotaCriticalPercent)

	assert.ErrorIs(t, d.UpdateCameraQuota("nonexistent", 0, 80, 90), ErrNotFound)
}

func TestGetCameraStorageUsage(t *testing.T) {
	d := openTestDB(t)

	cam := &Camera{Name: "usage-cam"}
	require.NoError(t, d.CreateCamera(cam))

	now := time.Now().UTC()
	for i := 0; i < 3; i++ {
		rec := &Recording{
			CameraID:  cam.ID,
			StartTime: now.Add(time.Duration(-3+i) * time.Hour).Format(timeFormat),
			EndTime:   now.Add(time.Duration(-2+i) * time.Hour).Format(timeFormat),
			FilePath:  fmt.Sprintf("/tmp/rec-%d.mp4", i),
			FileSize:  1000,
		}
		require.NoError(t, d.InsertRecording(rec))
	}

	usage, err := d.GetCameraStorageUsage(cam.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(3000), usage)
}

func TestGetTotalStorageUsage(t *testing.T) {
	d := openTestDB(t)

	cam1 := &Camera{Name: "cam1", MediaMTXPath: "nvr/cam1"}
	cam2 := &Camera{Name: "cam2", MediaMTXPath: "nvr/cam2"}
	require.NoError(t, d.CreateCamera(cam1))
	require.NoError(t, d.CreateCamera(cam2))

	now := time.Now().UTC()
	for _, camID := range []string{cam1.ID, cam2.ID} {
		rec := &Recording{
			CameraID:  camID,
			StartTime: now.Add(-1 * time.Hour).Format(timeFormat),
			EndTime:   now.Format(timeFormat),
			FilePath:  fmt.Sprintf("/tmp/rec-%s.mp4", camID),
			FileSize:  5000,
		}
		require.NoError(t, d.InsertRecording(rec))
	}

	total, err := d.GetTotalStorageUsage()
	require.NoError(t, err)
	assert.Equal(t, int64(10000), total)
}

func TestDeleteOldestRecordingsWithoutEvents(t *testing.T) {
	d := openTestDB(t)

	cam := &Camera{Name: "oldest-cam"}
	require.NoError(t, d.CreateCamera(cam))

	now := time.Now().UTC()

	// Insert 5 recordings, 1000 bytes each, oldest first.
	for i := 0; i < 5; i++ {
		rec := &Recording{
			CameraID:  cam.ID,
			StartTime: now.Add(time.Duration(-10+i*2) * time.Hour).Format(timeFormat),
			EndTime:   now.Add(time.Duration(-9+i*2) * time.Hour).Format(timeFormat),
			FilePath:  fmt.Sprintf("/tmp/rec-%d.mp4", i),
			FileSize:  1000,
		}
		require.NoError(t, d.InsertRecording(rec))
	}

	// Delete 2500 bytes worth (should delete 3 oldest: 1000+1000+1000 >= 2500).
	paths, freed, err := d.DeleteOldestRecordingsWithoutEvents(cam.ID, 2500)
	require.NoError(t, err)
	assert.Equal(t, 3, len(paths))
	assert.Equal(t, int64(3000), freed)

	// Should have 2 remaining.
	usage, err := d.GetCameraStorageUsage(cam.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(2000), usage)
}

func TestDeleteOldestRecordingsWithEvents(t *testing.T) {
	d := openTestDB(t)

	cam := &Camera{Name: "event-cam"}
	require.NoError(t, d.CreateCamera(cam))

	now := time.Now().UTC()

	// Insert a recording.
	rec := &Recording{
		CameraID:  cam.ID,
		StartTime: now.Add(-5 * time.Hour).Format(timeFormat),
		EndTime:   now.Add(-4 * time.Hour).Format(timeFormat),
		FilePath:  "/tmp/event-rec.mp4",
		FileSize:  2000,
	}
	require.NoError(t, d.InsertRecording(rec))

	// Insert an overlapping motion event.
	event := &MotionEvent{
		CameraID:  cam.ID,
		StartedAt: now.Add(-5 * time.Hour).Format(timeFormat),
	}
	require.NoError(t, d.InsertMotionEvent(event))
	require.NoError(t, d.EndMotionEvent(cam.ID, now.Add(-4*time.Hour).Format(timeFormat)))

	// Non-event deletion should not touch this recording.
	paths, freed, err := d.DeleteOldestRecordingsWithoutEvents(cam.ID, 5000)
	require.NoError(t, err)
	assert.Empty(t, paths)
	assert.Equal(t, int64(0), freed)

	// Event deletion should delete it.
	paths, freed, err = d.DeleteOldestRecordingsWithEvents(cam.ID, 5000)
	require.NoError(t, err)
	assert.Len(t, paths, 1)
	assert.Equal(t, int64(2000), freed)
}

func TestCameraQuotaDefaultThresholds(t *testing.T) {
	d := openTestDB(t)

	// Camera with zero thresholds should get defaults on create.
	cam := &Camera{Name: "default-cam", QuotaBytes: 1000}
	require.NoError(t, d.CreateCamera(cam))

	got, err := d.GetCamera(cam.ID)
	require.NoError(t, err)
	assert.Equal(t, 80, got.QuotaWarningPercent)
	assert.Equal(t, 90, got.QuotaCriticalPercent)
}
