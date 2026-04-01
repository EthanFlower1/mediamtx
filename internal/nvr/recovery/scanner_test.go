package recovery

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockDB implements DBQuerier for testing.
type mockDB struct {
	allPaths          map[string]int64
	unindexedPaths    map[string]int64
	allPathsErr       error
	unindexedPathsErr error
}

func (m *mockDB) GetAllRecordingPaths() (map[string]int64, error) {
	if m.allPathsErr != nil {
		return nil, m.allPathsErr
	}
	if m.allPaths == nil {
		return map[string]int64{}, nil
	}
	return m.allPaths, nil
}

func (m *mockDB) GetUnindexedRecordingPaths() (map[string]int64, error) {
	if m.unindexedPathsErr != nil {
		return nil, m.unindexedPathsErr
	}
	if m.unindexedPaths == nil {
		return map[string]int64{}, nil
	}
	return m.unindexedPaths, nil
}

func TestScanFindsOrphans(t *testing.T) {
	dir := t.TempDir()
	// Create two .mp4 files — neither in DB.
	f1 := filepath.Join(dir, "cam1", "2026", "04", "01", "12-00-00.mp4")
	f2 := filepath.Join(dir, "cam1", "2026", "04", "01", "12-01-00.mp4")
	require.NoError(t, os.MkdirAll(filepath.Dir(f1), 0o755))
	require.NoError(t, os.WriteFile(f1, buildValidFMP4(1), 0o644))
	require.NoError(t, os.WriteFile(f2, buildValidFMP4(1), 0o644))

	db := &mockDB{}
	candidates, err := ScanForCandidates([]string{dir}, db)
	require.NoError(t, err)
	assert.Len(t, candidates, 2)
	for _, c := range candidates {
		assert.False(t, c.HasDBEntry)
	}
}

func TestScanSkipsIndexed(t *testing.T) {
	dir := t.TempDir()
	fpath := filepath.Join(dir, "cam1", "segment.mp4")
	require.NoError(t, os.MkdirAll(filepath.Dir(fpath), 0o755))
	require.NoError(t, os.WriteFile(fpath, buildValidFMP4(1), 0o644))

	db := &mockDB{
		allPaths: map[string]int64{fpath: 1},
	}
	candidates, err := ScanForCandidates([]string{dir}, db)
	require.NoError(t, err)
	assert.Len(t, candidates, 0)
}

func TestScanFindsUnindexed(t *testing.T) {
	dir := t.TempDir()
	fpath := filepath.Join(dir, "cam1", "segment.mp4")
	require.NoError(t, os.MkdirAll(filepath.Dir(fpath), 0o755))
	require.NoError(t, os.WriteFile(fpath, buildValidFMP4(1), 0o644))

	db := &mockDB{
		allPaths:       map[string]int64{fpath: 42},
		unindexedPaths: map[string]int64{fpath: 42},
	}
	candidates, err := ScanForCandidates([]string{dir}, db)
	require.NoError(t, err)
	assert.Len(t, candidates, 1)
	assert.True(t, candidates[0].HasDBEntry)
	assert.Equal(t, int64(42), candidates[0].RecordingID)
}

func TestScanSkipsQuarantined(t *testing.T) {
	dir := t.TempDir()
	qdir := filepath.Join(dir, ".quarantine", "cam1")
	require.NoError(t, os.MkdirAll(qdir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(qdir, "seg.mp4"), buildValidFMP4(1), 0o644))

	db := &mockDB{}
	candidates, err := ScanForCandidates([]string{dir}, db)
	require.NoError(t, err)
	assert.Len(t, candidates, 0)
}

func TestScanSkipsSmallFiles(t *testing.T) {
	dir := t.TempDir()
	fpath := filepath.Join(dir, "tiny.mp4")
	require.NoError(t, os.WriteFile(fpath, []byte("tiny"), 0o644))

	db := &mockDB{}
	candidates, err := ScanForCandidates([]string{dir}, db)
	require.NoError(t, err)
	assert.Len(t, candidates, 0)
}

func TestScanPropagatesDBError(t *testing.T) {
	db := &mockDB{
		allPathsErr: fmt.Errorf("connection refused"),
	}
	_, err := ScanForCandidates([]string{t.TempDir()}, db)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "connection refused")
}
