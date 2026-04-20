package recovery

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunEndToEnd(t *testing.T) {
	dir := t.TempDir()
	subdir := filepath.Join(dir, "nvr", "cam1", "main")
	require.NoError(t, os.MkdirAll(subdir, 0o755))

	// Create a complete file (should be skipped or marked OK).
	completeData := buildValidFMP4(3)
	completePath := filepath.Join(subdir, "complete.mp4")
	require.NoError(t, os.WriteFile(completePath, completeData, 0o644))

	// Create a truncated file (should be repaired).
	truncatedData := buildValidFMP4(2)
	// Chop off last 30 bytes to make the second mdat incomplete.
	truncatedData = truncatedData[:len(truncatedData)-30]
	truncatedPath := filepath.Join(subdir, "truncated.mp4")
	require.NoError(t, os.WriteFile(truncatedPath, truncatedData, 0o644))

	// Create an unrecoverable file (ftyp + moov, no fragments).
	var noFragData []byte
	noFragData = append(noFragData, makeFtyp()...)
	noFragData = append(noFragData, makeMoov()...)
	noFragPath := filepath.Join(subdir, "nofrag.mp4")
	require.NoError(t, os.WriteFile(noFragPath, noFragData, 0o644))

	// All files are orphans (not in DB).
	db := &mockDB{}
	rec := newMockReconciler()
	rec.cameraMap["cam1"] = [2]string{"camera-1", "main"}

	result, err := Run(RunConfig{
		RecordDirs: []string{dir},
		DB:         db,
		Reconciler: rec,
	})
	require.NoError(t, err)

	assert.Equal(t, 3, result.Scanned)
	assert.Equal(t, 1, result.Repaired)      // truncated.mp4
	assert.Equal(t, 1, result.AlreadyOK)     // complete.mp4
	assert.Equal(t, 1, result.Unrecoverable) // nofrag.mp4

	// The complete and truncated files should have been inserted as new recordings.
	assert.Equal(t, 2, result.Reconcile.Inserted)

	// Verify the truncated file was actually repaired on disk.
	info, err := os.Stat(truncatedPath)
	require.NoError(t, err)
	assert.Less(t, info.Size(), int64(len(truncatedData)))
}

func TestRunNoRecordDirs(t *testing.T) {
	db := &mockDB{}
	rec := newMockReconciler()

	result, err := Run(RunConfig{
		RecordDirs: []string{},
		DB:         db,
		Reconciler: rec,
	})
	require.NoError(t, err)
	assert.Equal(t, 0, result.Scanned)
}

func TestRunEmptyDirectory(t *testing.T) {
	dir := t.TempDir()
	db := &mockDB{}
	rec := newMockReconciler()

	result, err := Run(RunConfig{
		RecordDirs: []string{dir},
		DB:         db,
		Reconciler: rec,
	})
	require.NoError(t, err)
	assert.Equal(t, 0, result.Scanned)
}
