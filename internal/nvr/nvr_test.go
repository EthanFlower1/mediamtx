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
