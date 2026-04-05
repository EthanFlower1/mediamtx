package nvr

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNVRInitialize(t *testing.T) {
	tmpDir := t.TempDir()

	// Write a minimal mediamtx.yml so the YAML writer has something to work with.
	confPath := filepath.Join(tmpDir, "mediamtx.yml")
	err := os.WriteFile(confPath, []byte("paths:\n"), 0o644)
	require.NoError(t, err)

	n := &NVR{
		DatabasePath: filepath.Join(tmpDir, "nvr.db"),
		JWTSecret:    "test-secret-key-for-testing-1234",
		ConfigPath:   confPath,
	}

	err = n.Initialize()
	require.NoError(t, err)
	defer n.Close()

	require.NotNil(t, n.DB())
	require.NotNil(t, n.PrivateKey())
	require.NotEmpty(t, n.JWKSJSON())
}

func TestNVRFirstRunSetup(t *testing.T) {
	tmpDir := t.TempDir()

	confPath := filepath.Join(tmpDir, "mediamtx.yml")
	err := os.WriteFile(confPath, []byte("paths:\n"), 0o644)
	require.NoError(t, err)

	n := &NVR{
		DatabasePath: filepath.Join(tmpDir, "nvr.db"),
		JWTSecret:    "test-secret-key-for-testing-1234",
		ConfigPath:   confPath,
	}

	err = n.Initialize()
	require.NoError(t, err)
	defer n.Close()

	require.True(t, n.IsSetupRequired(), "IsSetupRequired should return true on fresh DB")
}

func TestNVRFirstBootDetection(t *testing.T) {
	tmpDir := t.TempDir()

	confPath := filepath.Join(tmpDir, "mediamtx.yml")
	err := os.WriteFile(confPath, []byte("paths:\n"), 0o644)
	require.NoError(t, err)

	dbPath := filepath.Join(tmpDir, "data", "nvr.db")
	recPath := filepath.Join(tmpDir, "recordings")

	// First boot: DB does not exist yet.
	n := &NVR{
		DatabasePath:   dbPath,
		JWTSecret:      "test-secret-key-for-testing-1234",
		ConfigPath:     confPath,
		RecordingsPath: recPath,
	}

	err = n.Initialize()
	require.NoError(t, err)

	require.True(t, n.IsFirstBoot(), "IsFirstBoot should be true on first run")
	require.True(t, n.IsSetupRequired(), "IsSetupRequired should be true on first run")

	// Verify default directories were created.
	_, err = os.Stat(filepath.Join(tmpDir, "data", "backups"))
	require.NoError(t, err, "backups directory should be created")
	_, err = os.Stat(filepath.Join(tmpDir, "data", "tls"))
	require.NoError(t, err, "tls directory should be created")
	_, err = os.Stat(recPath)
	require.NoError(t, err, "recordings directory should be created")

	// Verify first_boot_at was recorded.
	val, err := n.DB().GetConfig("first_boot_at")
	require.NoError(t, err)
	require.NotEmpty(t, val)

	n.Close()

	// Second boot: DB already exists.
	n2 := &NVR{
		DatabasePath:   dbPath,
		JWTSecret:      "test-secret-key-for-testing-1234",
		ConfigPath:     confPath,
		RecordingsPath: recPath,
	}

	err = n2.Initialize()
	require.NoError(t, err)
	defer n2.Close()

	require.False(t, n2.IsFirstBoot(), "IsFirstBoot should be false on subsequent runs")
}
