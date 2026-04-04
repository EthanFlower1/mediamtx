package db

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExportJobCreate(t *testing.T) {
	d := newTestDB(t)

	// Create a camera first (foreign key).
	cam := &Camera{Name: "Test Cam", RTSPURL: "rtsp://localhost/stream"}
	require.NoError(t, d.CreateCamera(cam))

	job := &ExportJob{
		CameraID:  cam.ID,
		StartTime: "2025-01-01T00:00:00Z",
		EndTime:   "2025-01-01T01:00:00Z",
	}
	err := d.CreateExportJob(job)
	require.NoError(t, err)
	require.NotEmpty(t, job.ID)
	assert.Equal(t, "pending", job.Status)
	assert.NotEmpty(t, job.CreatedAt)
}

func TestExportJobGet(t *testing.T) {
	d := newTestDB(t)

	cam := &Camera{Name: "Test Cam", RTSPURL: "rtsp://localhost/stream"}
	require.NoError(t, d.CreateCamera(cam))

	job := &ExportJob{
		CameraID:  cam.ID,
		StartTime: "2025-01-01T00:00:00Z",
		EndTime:   "2025-01-01T01:00:00Z",
	}
	require.NoError(t, d.CreateExportJob(job))

	got, err := d.GetExportJob(job.ID)
	require.NoError(t, err)
	assert.Equal(t, job.ID, got.ID)
	assert.Equal(t, cam.ID, got.CameraID)
	assert.Equal(t, "pending", got.Status)
}

func TestExportJobGetNotFound(t *testing.T) {
	d := newTestDB(t)

	_, err := d.GetExportJob("nonexistent-id")
	require.ErrorIs(t, err, ErrNotFound)
}

func TestExportJobList(t *testing.T) {
	d := newTestDB(t)

	cam := &Camera{Name: "Test Cam", RTSPURL: "rtsp://localhost/stream"}
	require.NoError(t, d.CreateCamera(cam))

	job1 := &ExportJob{CameraID: cam.ID, StartTime: "2025-01-01T00:00:00Z", EndTime: "2025-01-01T01:00:00Z"}
	job2 := &ExportJob{CameraID: cam.ID, StartTime: "2025-01-02T00:00:00Z", EndTime: "2025-01-02T01:00:00Z"}
	require.NoError(t, d.CreateExportJob(job1))
	require.NoError(t, d.CreateExportJob(job2))

	// List all.
	jobs, err := d.ListExportJobs("", "")
	require.NoError(t, err)
	assert.Len(t, jobs, 2)

	// List by camera.
	jobs, err = d.ListExportJobs(cam.ID, "")
	require.NoError(t, err)
	assert.Len(t, jobs, 2)

	// List by status.
	jobs, err = d.ListExportJobs("", "pending")
	require.NoError(t, err)
	assert.Len(t, jobs, 2)

	// List by non-matching status.
	jobs, err = d.ListExportJobs("", "completed")
	require.NoError(t, err)
	assert.Len(t, jobs, 0)
}

func TestExportJobUpdateStatus(t *testing.T) {
	d := newTestDB(t)

	cam := &Camera{Name: "Test Cam", RTSPURL: "rtsp://localhost/stream"}
	require.NoError(t, d.CreateCamera(cam))

	job := &ExportJob{CameraID: cam.ID, StartTime: "2025-01-01T00:00:00Z", EndTime: "2025-01-01T01:00:00Z"}
	require.NoError(t, d.CreateExportJob(job))

	// Update to processing.
	err := d.UpdateExportJobStatus(job.ID, "processing", 50, "")
	require.NoError(t, err)

	got, err := d.GetExportJob(job.ID)
	require.NoError(t, err)
	assert.Equal(t, "processing", got.Status)
	assert.InDelta(t, 50.0, got.Progress, 0.01)

	// Update to completed.
	err = d.UpdateExportJobStatus(job.ID, "completed", 100, "")
	require.NoError(t, err)

	got, err = d.GetExportJob(job.ID)
	require.NoError(t, err)
	assert.Equal(t, "completed", got.Status)
	assert.NotEmpty(t, got.CompletedAt)
}

func TestExportJobUpdateStatusNotFound(t *testing.T) {
	d := newTestDB(t)

	err := d.UpdateExportJobStatus("nonexistent-id", "processing", 0, "")
	require.ErrorIs(t, err, ErrNotFound)
}

func TestExportJobDelete(t *testing.T) {
	d := newTestDB(t)

	cam := &Camera{Name: "Test Cam", RTSPURL: "rtsp://localhost/stream"}
	require.NoError(t, d.CreateCamera(cam))

	job := &ExportJob{CameraID: cam.ID, StartTime: "2025-01-01T00:00:00Z", EndTime: "2025-01-01T01:00:00Z"}
	require.NoError(t, d.CreateExportJob(job))

	err := d.DeleteExportJob(job.ID)
	require.NoError(t, err)

	_, err = d.GetExportJob(job.ID)
	require.ErrorIs(t, err, ErrNotFound)
}

func TestExportJobDeleteNotFound(t *testing.T) {
	d := newTestDB(t)

	err := d.DeleteExportJob("nonexistent-id")
	require.ErrorIs(t, err, ErrNotFound)
}

func TestExportJobGetPending(t *testing.T) {
	d := newTestDB(t)

	cam := &Camera{Name: "Test Cam", RTSPURL: "rtsp://localhost/stream"}
	require.NoError(t, d.CreateCamera(cam))

	job1 := &ExportJob{CameraID: cam.ID, StartTime: "2025-01-01T00:00:00Z", EndTime: "2025-01-01T01:00:00Z"}
	job2 := &ExportJob{CameraID: cam.ID, StartTime: "2025-01-02T00:00:00Z", EndTime: "2025-01-02T01:00:00Z"}
	require.NoError(t, d.CreateExportJob(job1))
	require.NoError(t, d.CreateExportJob(job2))

	// Mark job1 as processing.
	require.NoError(t, d.UpdateExportJobStatus(job1.ID, "processing", 0, ""))

	pending, err := d.GetPendingExportJobs()
	require.NoError(t, err)
	assert.Len(t, pending, 1)
	assert.Equal(t, job2.ID, pending[0].ID)
}

func TestExportJobUpdateOutput(t *testing.T) {
	d := newTestDB(t)

	cam := &Camera{Name: "Test Cam", RTSPURL: "rtsp://localhost/stream"}
	require.NoError(t, d.CreateCamera(cam))

	job := &ExportJob{CameraID: cam.ID, StartTime: "2025-01-01T00:00:00Z", EndTime: "2025-01-01T01:00:00Z"}
	require.NoError(t, d.CreateExportJob(job))

	err := d.UpdateExportJobOutput(job.ID, "/exports/test.mp4")
	require.NoError(t, err)

	got, err := d.GetExportJob(job.ID)
	require.NoError(t, err)
	assert.Equal(t, "/exports/test.mp4", got.OutputPath)
}
