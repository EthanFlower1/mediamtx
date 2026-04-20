package recovery

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockReconciler struct {
	inserted  []string           // file paths inserted
	updated   map[int64]int64    // recording ID -> new file size
	corrupted map[int64]string   // recording ID -> detail
	cameraMap map[string][2]string // path substring -> [cameraID, streamID]
}

func newMockReconciler() *mockReconciler {
	return &mockReconciler{
		updated:   make(map[int64]int64),
		corrupted: make(map[int64]string),
		cameraMap: make(map[string][2]string),
	}
}

func (m *mockReconciler) InsertRecording(cameraID, streamID string, startTime, endTime time.Time, durationMs, fileSize int64, filePath, format string) (int64, error) {
	m.inserted = append(m.inserted, filePath)
	return int64(len(m.inserted)), nil
}

func (m *mockReconciler) UpdateRecordingFileSize(id int64, fileSize int64) error {
	m.updated[id] = fileSize
	return nil
}

func (m *mockReconciler) UpdateRecordingStatus(id int64, status string, detail *string, verifiedAt string) error {
	d := ""
	if detail != nil {
		d = *detail
	}
	m.corrupted[id] = d
	return nil
}

func (m *mockReconciler) MatchCameraFromPath(filePath string) (string, string, bool) {
	for substr, ids := range m.cameraMap {
		if strings.Contains(filePath, substr) {
			return ids[0], ids[1], true
		}
	}
	return "", "", false
}

func TestReconcileOrphanedFile(t *testing.T) {
	rec := newMockReconciler()
	rec.cameraMap["cam1"] = [2]string{"camera-1", "main"}

	outcomes := []RepairOutcome{{
		Candidate: Candidate{FilePath: "/recordings/nvr/cam1/seg.mp4", HasDBEntry: false},
		Result:    RepairResult{Repaired: true, NewSize: 1024, FragmentsRecovered: 3},
	}}

	result, err := Reconcile(outcomes, rec)
	require.NoError(t, err)
	assert.Equal(t, 1, result.Inserted)
	assert.Len(t, rec.inserted, 1)
}

func TestReconcileUnindexedEntry(t *testing.T) {
	rec := newMockReconciler()

	outcomes := []RepairOutcome{{
		Candidate: Candidate{FilePath: "/recordings/seg.mp4", HasDBEntry: true, RecordingID: 42},
		Result:    RepairResult{Repaired: true, NewSize: 2048, OriginalSize: 3000, FragmentsRecovered: 5},
	}}

	result, err := Reconcile(outcomes, rec)
	require.NoError(t, err)
	assert.Equal(t, 1, result.Updated)
	assert.Equal(t, int64(2048), rec.updated[42])
}

func TestReconcileUnrecoverableWithDB(t *testing.T) {
	rec := newMockReconciler()

	outcomes := []RepairOutcome{{
		Candidate: Candidate{FilePath: "/recordings/seg.mp4", HasDBEntry: true, RecordingID: 99},
		Result:    RepairResult{Unrecoverable: true, Detail: "no complete moof+mdat pairs"},
	}}

	result, err := Reconcile(outcomes, rec)
	require.NoError(t, err)
	assert.Equal(t, 1, result.MarkedCorrupt)
	assert.Contains(t, rec.corrupted[99], "no complete moof+mdat pairs")
}

func TestMoveToRecoveryFailed(t *testing.T) {
	dir := t.TempDir()
	nvrDir := filepath.Join(dir, "nvr", "cam1", "main")
	require.NoError(t, os.MkdirAll(nvrDir, 0o755))

	filePath := filepath.Join(nvrDir, "segment.mp4")
	require.NoError(t, os.WriteFile(filePath, []byte("test"), 0o644))

	err := moveToRecoveryFailed(filePath)
	require.NoError(t, err)

	// Original file should be gone.
	_, err = os.Stat(filePath)
	assert.True(t, os.IsNotExist(err))

	// File should be in .recovery_failed directory.
	expectedDir := filepath.Join(dir, "nvr", ".recovery_failed", "cam1", "main")
	expectedPath := filepath.Join(expectedDir, "segment.mp4")
	_, err = os.Stat(expectedPath)
	assert.NoError(t, err)
}

func TestReconcileUnrecoverableOrphan(t *testing.T) {
	dir := t.TempDir()
	nvrDir := filepath.Join(dir, "nvr", "cam1")
	require.NoError(t, os.MkdirAll(nvrDir, 0o755))
	filePath := filepath.Join(nvrDir, "orphan.mp4")
	require.NoError(t, os.WriteFile(filePath, []byte("bad data"), 0o644))

	rec := newMockReconciler()
	outcomes := []RepairOutcome{{
		Candidate: Candidate{FilePath: filePath, HasDBEntry: false},
		Result:    RepairResult{Unrecoverable: true, Detail: "no fragments"},
	}}

	result, err := Reconcile(outcomes, rec)
	require.NoError(t, err)
	assert.Equal(t, 1, result.MarkedCorrupt)

	// File should have been moved to .recovery_failed.
	_, err = os.Stat(filePath)
	assert.True(t, os.IsNotExist(err))
}

func TestReconcileOrphanNoCameraMatch(t *testing.T) {
	rec := newMockReconciler()
	// No camera mappings — MatchCameraFromPath will return false.

	outcomes := []RepairOutcome{{
		Candidate: Candidate{FilePath: "/recordings/unknown/seg.mp4", HasDBEntry: false},
		Result:    RepairResult{Repaired: true, NewSize: 1024, FragmentsRecovered: 2},
	}}

	result, err := Reconcile(outcomes, rec)
	require.NoError(t, err)
	assert.Equal(t, 1, result.Skipped)
	assert.Len(t, rec.inserted, 0)
}

func TestReconcileAlreadyComplete(t *testing.T) {
	rec := newMockReconciler()

	outcomes := []RepairOutcome{{
		Candidate: Candidate{FilePath: "/recordings/seg.mp4", HasDBEntry: true, RecordingID: 10},
		Result:    RepairResult{AlreadyComplete: true, FragmentsRecovered: 5},
	}}

	result, err := Reconcile(outcomes, rec)
	require.NoError(t, err)
	assert.Equal(t, 1, result.Skipped)
}
