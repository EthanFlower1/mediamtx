package thumbnail

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestThumbnailDir(t *testing.T) {
	got := ThumbnailDir("/recordings", "cam1")
	want := filepath.Join("/recordings", "cam1", "thumbnails")
	if got != want {
		t.Errorf("ThumbnailDir() = %q, want %q", got, want)
	}
}

func TestThumbnailFilename(t *testing.T) {
	ts := time.Date(2025, 6, 15, 14, 30, 45, 0, time.UTC)
	got := ThumbnailFilename("cam1", ts)
	want := "cam1_20250615T143045.jpg"
	if got != want {
		t.Errorf("ThumbnailFilename() = %q, want %q", got, want)
	}
}

func TestListThumbnails_EmptyDir(t *testing.T) {
	tmpDir := t.TempDir()
	start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2025, 12, 31, 0, 0, 0, 0, time.UTC)

	results, err := ListThumbnails(tmpDir, "cam1", start, end)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestListThumbnails_FiltersTimeRange(t *testing.T) {
	tmpDir := t.TempDir()
	cameraID := "cam1"
	thumbDir := ThumbnailDir(tmpDir, cameraID)
	if err := os.MkdirAll(thumbDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Create test thumbnail files.
	timestamps := []time.Time{
		time.Date(2025, 6, 1, 10, 0, 0, 0, time.UTC),
		time.Date(2025, 6, 1, 10, 0, 10, 0, time.UTC),
		time.Date(2025, 6, 1, 10, 0, 20, 0, time.UTC),
		time.Date(2025, 6, 1, 10, 0, 30, 0, time.UTC),
		time.Date(2025, 6, 1, 10, 0, 40, 0, time.UTC),
	}

	for _, ts := range timestamps {
		name := ThumbnailFilename(cameraID, ts)
		path := filepath.Join(thumbDir, name)
		if err := os.WriteFile(path, []byte("fake-jpg"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// Query a subset.
	start := time.Date(2025, 6, 1, 10, 0, 10, 0, time.UTC)
	end := time.Date(2025, 6, 1, 10, 0, 30, 0, time.UTC)

	results, err := ListThumbnails(tmpDir, cameraID, start, end)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should get timestamps at 10:00:10 and 10:00:20 (start inclusive, end exclusive).
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if !results[0].Timestamp.Equal(timestamps[1]) {
		t.Errorf("first result timestamp = %v, want %v", results[0].Timestamp, timestamps[1])
	}
	if !results[1].Timestamp.Equal(timestamps[2]) {
		t.Errorf("second result timestamp = %v, want %v", results[1].Timestamp, timestamps[2])
	}
}

func TestListThumbnails_SortedAscending(t *testing.T) {
	tmpDir := t.TempDir()
	cameraID := "cam1"
	thumbDir := ThumbnailDir(tmpDir, cameraID)
	if err := os.MkdirAll(thumbDir, 0o755); err != nil {
		t.Fatal(err)
	}

	ts1 := time.Date(2025, 6, 1, 10, 0, 20, 0, time.UTC)
	ts2 := time.Date(2025, 6, 1, 10, 0, 10, 0, time.UTC)

	// Write in reverse order.
	for _, ts := range []time.Time{ts1, ts2} {
		name := ThumbnailFilename(cameraID, ts)
		if err := os.WriteFile(filepath.Join(thumbDir, name), []byte("jpg"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	start := time.Date(2025, 6, 1, 10, 0, 0, 0, time.UTC)
	end := time.Date(2025, 6, 1, 10, 1, 0, 0, time.UTC)

	results, err := ListThumbnails(tmpDir, cameraID, start, end)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if !results[0].Timestamp.Before(results[1].Timestamp) {
		t.Error("results should be sorted ascending by timestamp")
	}
}

func TestCleanupThumbnails(t *testing.T) {
	tmpDir := t.TempDir()
	cameraID := "cam1"
	thumbDir := ThumbnailDir(tmpDir, cameraID)
	if err := os.MkdirAll(thumbDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Create old and new thumbnail files.
	oldTS := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	newTS := time.Date(2025, 6, 15, 0, 0, 0, 0, time.UTC)

	oldFile := filepath.Join(thumbDir, ThumbnailFilename(cameraID, oldTS))
	newFile := filepath.Join(thumbDir, ThumbnailFilename(cameraID, newTS))

	if err := os.WriteFile(oldFile, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(newFile, []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Cleanup thumbnails before March 1, 2025.
	cutoff := time.Date(2025, 3, 1, 0, 0, 0, 0, time.UTC)
	removed := CleanupThumbnails(tmpDir, cameraID, cutoff)

	if removed != 1 {
		t.Errorf("expected 1 removed, got %d", removed)
	}

	// Old file should be gone.
	if _, err := os.Stat(oldFile); !os.IsNotExist(err) {
		t.Error("old file should have been deleted")
	}

	// New file should remain.
	if _, err := os.Stat(newFile); err != nil {
		t.Error("new file should still exist")
	}
}

func TestCleanupThumbnails_NonexistentDir(t *testing.T) {
	removed := CleanupThumbnails("/nonexistent", "cam1", time.Now())
	if removed != 0 {
		t.Errorf("expected 0 removed for nonexistent dir, got %d", removed)
	}
}
