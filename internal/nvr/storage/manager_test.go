package storage

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckPathHealth_Healthy(t *testing.T) {
	dir := t.TempDir()
	healthy, err := checkPathHealth(dir)
	require.NoError(t, err)
	assert.True(t, healthy)
}

func TestCheckPathHealth_Unreachable(t *testing.T) {
	healthy, err := checkPathHealth("/nonexistent/path/that/does/not/exist")
	require.NoError(t, err)
	assert.False(t, healthy)
}

func TestCheckPathHealth_NotWritable(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("requires non-root user")
	}
	dir := t.TempDir()
	os.Chmod(dir, 0o444)
	t.Cleanup(func() { os.Chmod(dir, 0o755) })
	healthy, err := checkPathHealth(dir)
	require.NoError(t, err)
	assert.False(t, healthy)
}

func TestManager_EvaluateHealth(t *testing.T) {
	healthyDir := t.TempDir()
	badDir := "/nonexistent/storage/path"

	m := &Manager{
		health:    make(map[string]bool),
		ioMonitor: NewIOMonitor(50, 200),
	}

	m.evaluateHealth(map[string][]string{
		healthyDir: {"cam1"},
		badDir:     {"cam2"},
	})

	assert.True(t, m.GetHealth(healthyDir))
	assert.False(t, m.GetHealth(badDir))
}
