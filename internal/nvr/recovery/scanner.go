package recovery

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Candidate represents a segment file that may need repair.
type Candidate struct {
	FilePath    string // Absolute path to the file on disk
	HasDBEntry  bool   // Whether a recording row exists in the DB
	RecordingID int64  // DB recording ID (0 if no DB entry)
}

// DBQuerier abstracts the database queries needed by the scanner.
type DBQuerier interface {
	// GetAllRecordingPaths returns all file_path values from the recordings table.
	GetAllRecordingPaths() (map[string]int64, error)
	// GetUnindexedRecordingPaths returns recording IDs and paths that have no fragments
	// and are not quarantined.
	GetUnindexedRecordingPaths() (map[string]int64, error)
}

// ScanForCandidates finds fMP4 files that may need recovery. It combines:
// 1. Orphaned files: .mp4 files on disk with no DB entry
// 2. Unindexed DB entries: recordings with no fragment rows
func ScanForCandidates(recordDirs []string, db DBQuerier) ([]Candidate, error) {
	// Get all known recording paths from DB.
	knownPaths, err := db.GetAllRecordingPaths()
	if err != nil {
		return nil, fmt.Errorf("query recording paths: %w", err)
	}

	// Get unindexed recordings (DB entry exists but no fragments).
	unindexedPaths, err := db.GetUnindexedRecordingPaths()
	if err != nil {
		return nil, fmt.Errorf("query unindexed recordings: %w", err)
	}

	var candidates []Candidate

	// Walk recording directories to find orphaned files.
	for _, dir := range recordDirs {
		err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil // skip inaccessible paths
			}
			if d.IsDir() {
				// Skip quarantine and recovery_failed directories.
				base := filepath.Base(path)
				if base == ".quarantine" || base == ".recovery_failed" {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(path, ".mp4") {
				return nil
			}
			info, err := d.Info()
			if err != nil {
				return nil // skip if we can't stat
			}
			if info.Size() < 8 {
				return nil // too small to be valid fMP4
			}

			// Normalize to absolute path for DB comparison.
			absPath, _ := filepath.Abs(path)

			// Check if this file is known to the DB.
			if _, known := knownPaths[absPath]; !known {
				candidates = append(candidates, Candidate{
					FilePath:   absPath,
					HasDBEntry: false,
				})
			}
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("walk %s: %w", dir, err)
		}
	}

	// Add unindexed DB entries as candidates.
	for path, recID := range unindexedPaths {
		// Only include if the file still exists on disk.
		if _, err := os.Stat(path); err == nil {
			candidates = append(candidates, Candidate{
				FilePath:    path,
				HasDBEntry:  true,
				RecordingID: recID,
			})
		}
	}

	return candidates, nil
}
