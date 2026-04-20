package recovery

import (
	"strings"
	"time"

	"github.com/bluenviron/mediamtx/internal/nvr/db"
)

// Compile-time interface checks.
var (
	_ DBQuerier  = (*recoveryDBAdapter)(nil)
	_ Reconciler = (*recoveryReconcileAdapter)(nil)
)

// NewDBAdapter returns a DBQuerier backed by the real database.
func NewDBAdapter(database *db.DB) DBQuerier {
	return &recoveryDBAdapter{db: database}
}

// NewReconcileAdapter returns a Reconciler backed by the real database.
func NewReconcileAdapter(database *db.DB) Reconciler {
	return &recoveryReconcileAdapter{db: database}
}

// recoveryDBAdapter implements DBQuerier using the real DB.
type recoveryDBAdapter struct {
	db *db.DB
}

func (a *recoveryDBAdapter) GetAllRecordingPaths() (map[string]int64, error) {
	return a.db.GetAllRecordingPaths()
}

func (a *recoveryDBAdapter) GetUnindexedRecordingPaths() (map[string]int64, error) {
	return a.db.GetUnindexedRecordingPaths()
}

// recoveryReconcileAdapter implements Reconciler using the real DB.
type recoveryReconcileAdapter struct {
	db *db.DB
}

func (a *recoveryReconcileAdapter) InsertRecording(cameraID, streamID string, startTime, endTime time.Time, durationMs, fileSize int64, filePath, format string) (int64, error) {
	rec := &db.Recording{
		CameraID:   cameraID,
		StreamID:   streamID,
		StartTime:  startTime.UTC().Format("2006-01-02T15:04:05.000Z"),
		EndTime:    endTime.UTC().Format("2006-01-02T15:04:05.000Z"),
		DurationMs: durationMs,
		FilePath:   filePath,
		FileSize:   fileSize,
		Format:     format,
	}
	if err := a.db.InsertRecording(rec); err != nil {
		return 0, err
	}
	return rec.ID, nil
}

func (a *recoveryReconcileAdapter) UpdateRecordingFileSize(id int64, fileSize int64) error {
	return a.db.UpdateRecordingFileSize(id, fileSize)
}

func (a *recoveryReconcileAdapter) UpdateRecordingStatus(id int64, status string, detail *string, verifiedAt string) error {
	return a.db.UpdateRecordingStatus(id, status, detail, verifiedAt)
}

func (a *recoveryReconcileAdapter) MatchCameraFromPath(filePath string) (cameraID string, streamID string, ok bool) {
	// Extract camera ID from path convention: .../nvr/<camera-id>/...
	if idx := strings.Index(filePath, "nvr/"); idx >= 0 {
		rest := filePath[idx+4:]
		parts := strings.SplitN(rest, "/", 2)
		if len(parts) >= 1 {
			candidate := parts[0]
			stream := "main"
			if tildeIdx := strings.Index(candidate, "~"); tildeIdx > 0 {
				stream = candidate[tildeIdx+1:]
				candidate = candidate[:tildeIdx]
			}
			if c, err := a.db.GetCamera(candidate); err == nil && c != nil {
				return c.ID, stream, true
			}
		}
	}

	// Fallback: match against known camera paths.
	cameras, err := a.db.ListCameras()
	if err != nil {
		return "", "", false
	}
	for _, c := range cameras {
		if c.MediaMTXPath != "" && strings.Contains(filePath, c.MediaMTXPath) {
			return c.ID, "main", true
		}
	}
	return "", "", false
}
