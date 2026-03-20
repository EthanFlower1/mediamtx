package yamlwriter

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const testConfig = `# MediaMTX configuration
# General settings
logLevel: info
api: yes

# Path configuration
paths:
  cam1:
    source: rtsp://example.com/cam1
  nvr/lobby:
    source: rtsp://example.com/lobby
`

func writeTestConfig(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "mediamtx.yml")
	require.NoError(t, os.WriteFile(path, []byte(testConfig), 0o644))
	return path
}

func TestAddPath(t *testing.T) {
	path := writeTestConfig(t)
	w := New(path)

	err := w.AddPath("nvr/parking", map[string]interface{}{
		"source": "rtsp://example.com/parking",
	})
	require.NoError(t, err)

	data, err := os.ReadFile(path)
	require.NoError(t, err)

	content := string(data)
	// Verify the new path was added.
	require.Contains(t, content, "nvr/parking")
	require.Contains(t, content, "rtsp://example.com/parking")
	// Verify comments are preserved.
	require.Contains(t, content, "# MediaMTX configuration")
	require.Contains(t, content, "# Path configuration")
}

func TestRemovePath(t *testing.T) {
	path := writeTestConfig(t)
	w := New(path)

	err := w.RemovePath("nvr/lobby")
	require.NoError(t, err)

	data, err := os.ReadFile(path)
	require.NoError(t, err)

	content := string(data)
	// Verify the path was removed.
	require.NotContains(t, content, "nvr/lobby")
	// Verify other paths are kept.
	require.Contains(t, content, "cam1")
}

func TestGetNVRPaths(t *testing.T) {
	path := writeTestConfig(t)
	w := New(path)

	paths, err := w.GetNVRPaths()
	require.NoError(t, err)

	// Only nvr/-prefixed paths should be returned.
	require.Equal(t, []string{"nvr/lobby"}, paths)
}

func TestAtomicWrite(t *testing.T) {
	path := writeTestConfig(t)
	dir := filepath.Dir(path)
	w := New(path)

	err := w.AddPath("nvr/test", map[string]interface{}{
		"source": "rtsp://example.com/test",
	})
	require.NoError(t, err)

	// Verify no temp files are left behind.
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)

	for _, entry := range entries {
		require.False(t, strings.HasPrefix(entry.Name(), ".mediamtx-"),
			"temp file left behind: %s", entry.Name())
	}

	// Verify the final file is valid.
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Contains(t, string(data), "nvr/test")
}
