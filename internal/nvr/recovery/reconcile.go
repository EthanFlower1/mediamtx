package recovery

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Reconciler abstracts the database operations needed for reconciliation.
type Reconciler interface {
	InsertRecording(cameraID, streamID string, startTime, endTime time.Time, durationMs, fileSize int64, filePath, format string) (int64, error)
	UpdateRecordingFileSize(id int64, fileSize int64) error
	UpdateRecordingStatus(id int64, status string, detail *string, verifiedAt string) error
	MatchCameraFromPath(filePath string) (cameraID string, streamID string, ok bool)
}

// ReconcileResult summarizes the reconciliation outcome.
type ReconcileResult struct {
	Inserted      int
	Updated       int
	MarkedCorrupt int
	Skipped       int
}

// RepairOutcome pairs a candidate with its repair result.
type RepairOutcome struct {
	Candidate Candidate
	Result    RepairResult
}

// Reconcile updates the database for each repaired segment.
// - Orphaned files (no DB entry): insert a new recording.
// - Existing entries with stale data: update file_size.
// - Unrecoverable files: mark as corrupted.
func Reconcile(outcomes []RepairOutcome, rec Reconciler) (ReconcileResult, error) {
	var summary ReconcileResult

	for _, o := range outcomes {
		if o.Result.Unrecoverable {
			if o.Candidate.HasDBEntry {
				detail := o.Result.Detail
				now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
				if err := rec.UpdateRecordingStatus(o.Candidate.RecordingID, "corrupted", &detail, now); err != nil {
					return summary, fmt.Errorf("mark corrupted recording %d: %w", o.Candidate.RecordingID, err)
				}
				summary.MarkedCorrupt++
			} else {
				// No DB entry and unrecoverable — move to .recovery_failed/.
				if err := moveToRecoveryFailed(o.Candidate.FilePath); err != nil {
					fmt.Fprintf(os.Stderr, "recovery: failed to move unrecoverable file %s: %v\n", o.Candidate.FilePath, err)
				}
				summary.MarkedCorrupt++
			}
			continue
		}

		if o.Candidate.HasDBEntry {
			// Existing DB entry — update file_size if repaired.
			if o.Result.Repaired {
				if err := rec.UpdateRecordingFileSize(o.Candidate.RecordingID, o.Result.NewSize); err != nil {
					return summary, fmt.Errorf("update file size for recording %d: %w", o.Candidate.RecordingID, err)
				}
				summary.Updated++
			} else {
				summary.Skipped++
			}
		} else {
			// Orphaned file — insert new recording.
			cameraID, streamID, ok := rec.MatchCameraFromPath(o.Candidate.FilePath)
			if !ok {
				summary.Skipped++
				continue
			}

			// Estimate duration from fragments. Use 1s per fragment as rough estimate
			// since actual duration requires parsing trun boxes (done later by backfill).
			estimatedDuration := time.Duration(o.Result.FragmentsRecovered) * time.Second
			now := time.Now().UTC()
			start := now.Add(-estimatedDuration)

			size := o.Result.NewSize
			if size == 0 {
				size = o.Result.OriginalSize
			}

			_, err := rec.InsertRecording(cameraID, streamID, start, now,
				estimatedDuration.Milliseconds(), size, o.Candidate.FilePath, "fmp4")
			if err != nil {
				return summary, fmt.Errorf("insert recovered recording %s: %w", o.Candidate.FilePath, err)
			}
			summary.Inserted++
		}
	}

	return summary, nil
}

// moveToRecoveryFailed moves an unrecoverable file to a .recovery_failed directory
// adjacent to its parent recording directory.
func moveToRecoveryFailed(filePath string) error {
	dir := filepath.Dir(filePath)
	base := filepath.Base(filePath)

	// Walk up to find the recording root (directory containing "nvr/").
	recoveryDir := dir
	if idx := strings.Index(dir, "nvr/"); idx >= 0 {
		root := dir[:idx+4] // includes "nvr/"
		rel, err := filepath.Rel(root, dir)
		if err != nil {
			rel = filepath.Base(dir)
		}
		recoveryDir = filepath.Join(root, ".recovery_failed", rel)
	} else {
		recoveryDir = filepath.Join(dir, ".recovery_failed")
	}

	if err := os.MkdirAll(recoveryDir, 0o755); err != nil {
		return err
	}
	return os.Rename(filePath, filepath.Join(recoveryDir, base))
}
