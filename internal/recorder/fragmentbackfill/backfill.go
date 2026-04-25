package fragmentbackfill

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/bluenviron/mediamtx/internal/recorder/db"
)

// ScanResult holds the output of scanning a single fMP4 file.
type ScanResult struct {
	InitSize  int64
	Fragments []FragmentInfo
}

// Scanner is a function that parses an fMP4 file and returns its fragment layout.
// The default implementation is scanFile; tests inject a fake.
type Scanner func(filePath string) (ScanResult, error)

// Database abstracts the three DB methods used by the backfill goroutine.
// Production passes a *db.DB; tests inject a fake implementation.
type Database interface {
	GetUnindexedRecordings() ([]*db.Recording, error)
	UpdateRecordingInitSize(recordingID int64, initSize int64) error
	InsertFragments(recordingID int64, fragments []db.RecordingFragment) error
}

// Config carries all dependencies for the backfill goroutine.
type Config struct {
	DB      Database
	Logger  *slog.Logger
	Scanner Scanner // optional; defaults to scanFile
}

// Run starts a background goroutine that indexes any recordings that lack
// fragment metadata. It waits 5 seconds before starting so the server can
// finish booting, then processes newest-first (the legacy default).
//
// The goroutine exits cleanly when ctx is cancelled.
func Run(ctx context.Context, cfg Config) {
	if cfg.Scanner == nil {
		cfg.Scanner = scanFile
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}

	go func() {
		select {
		case <-ctx.Done():
			return
		case <-time.After(5 * time.Second):
		}
		runOnce(ctx, cfg)
	}()
}

// runOnce performs a single backfill pass (exported for testing via package-internal access).
func runOnce(ctx context.Context, cfg Config) {
	recs, err := cfg.DB.GetUnindexedRecordings()
	if err != nil {
		cfg.Logger.Error("fragment backfill: query failed", "err", err)
		return
	}

	if len(recs) == 0 {
		return
	}

	cfg.Logger.Info("fragment backfill: starting", "count", len(recs))

	indexed := 0
	for _, rec := range recs {
		// Respect context cancellation mid-batch.
		select {
		case <-ctx.Done():
			cfg.Logger.Info("fragment backfill: cancelled mid-batch",
				"indexed", indexed, "total", len(recs))
			return
		default:
		}

		if rec.Format != "fmp4" {
			continue
		}

		// TODO: move the quarantine filter into the GetUnindexedRecordings SQL query
		// for symmetry with GetUnindexedRecordingPaths (which already does so).
		if rec.Status == "quarantined" {
			cfg.Logger.Debug("fragment backfill: skipping quarantined",
				slog.Int64("recording_id", rec.ID))
			continue
		}

		if _, statErr := os.Stat(rec.FilePath); os.IsNotExist(statErr) {
			cfg.Logger.Warn("fragment backfill: file missing, skipping",
				"recording_id", rec.ID, "path", rec.FilePath)
			continue
		}

		result, scanErr := cfg.Scanner(rec.FilePath)
		if scanErr != nil {
			cfg.Logger.Warn("fragment backfill: scan failed, skipping",
				"recording_id", rec.ID, "path", rec.FilePath, "err", scanErr)
			continue
		}

		if err := cfg.DB.UpdateRecordingInitSize(rec.ID, result.InitSize); err != nil {
			// Do NOT fall through to InsertFragments: leaving init_size=0 while
			// inserting fragments would mark the recording as indexed but break
			// byte-range playback permanently. Skip so the next boot retries.
			cfg.Logger.Warn("fragment backfill: init_size update failed, skipping",
				"recording_id", rec.ID, "err", err)
			continue
		}

		dbFrags := buildDBFragments(rec.ID, result.Fragments)
		if err := cfg.DB.InsertFragments(rec.ID, dbFrags); err != nil {
			cfg.Logger.Warn("fragment backfill: insert fragments failed",
				"recording_id", rec.ID, "err", err)
			continue
		}

		indexed++
		if indexed%100 == 0 {
			cfg.Logger.Info("fragment backfill: progress",
				"indexed", indexed, "total", len(recs))
		}
	}

	cfg.Logger.Info("fragment backfill: complete", "indexed", indexed, "total", len(recs))
}

// buildDBFragments converts FragmentInfo slice into db.RecordingFragment rows.
// TimestampMs and IsKeyframe are not available from the raw box parse, so they
// are left at zero/false (same as legacy behaviour).
func buildDBFragments(recordingID int64, frags []FragmentInfo) []db.RecordingFragment {
	out := make([]db.RecordingFragment, len(frags))
	for i, f := range frags {
		out[i] = db.RecordingFragment{
			RecordingID:   recordingID,
			FragmentIndex: i,
			ByteOffset:    f.Offset,
			Size:          f.Size,
			DurationMs:    f.DurationMs,
			IsKeyframe:    false,
			TimestampMs:   0,
		}
	}
	return out
}
