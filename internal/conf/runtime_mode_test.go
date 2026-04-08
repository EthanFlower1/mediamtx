package conf

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/shared/runtime"
)

// TestRuntimeModeFromFile parses each of the three legal mode values
// (plus the empty/legacy default) and confirms Load() returns the
// expected RuntimeMode. Invalid values must be rejected with a clear
// error.
func TestRuntimeModeFromFile(t *testing.T) {
	t.Run("default is legacy", func(t *testing.T) {
		tmpf, err := createTempFile([]byte("paths:\n  cam1:\n"))
		require.NoError(t, err)
		defer os.Remove(tmpf)

		conf, _, err := Load(tmpf, nil, nil)
		require.NoError(t, err)
		require.Equal(t, RuntimeModeLegacy, conf.Mode)
		require.Equal(t, runtime.ModeLegacy, conf.Mode.Runtime())
	})

	t.Run("directory", func(t *testing.T) {
		tmpf, err := createTempFile([]byte("mode: directory\npaths:\n  cam1:\n"))
		require.NoError(t, err)
		defer os.Remove(tmpf)

		conf, _, err := Load(tmpf, nil, nil)
		require.NoError(t, err)
		require.Equal(t, RuntimeModeDirectory, conf.Mode)
		require.Equal(t, runtime.ModeDirectory, conf.Mode.Runtime())
	})

	t.Run("recorder", func(t *testing.T) {
		tmpf, err := createTempFile([]byte("mode: recorder\npaths:\n  cam1:\n"))
		require.NoError(t, err)
		defer os.Remove(tmpf)

		conf, _, err := Load(tmpf, nil, nil)
		require.NoError(t, err)
		require.Equal(t, RuntimeModeRecorder, conf.Mode)
		require.Equal(t, runtime.ModeRecorder, conf.Mode.Runtime())
	})

	t.Run("all-in-one", func(t *testing.T) {
		tmpf, err := createTempFile([]byte("mode: all-in-one\npaths:\n  cam1:\n"))
		require.NoError(t, err)
		defer os.Remove(tmpf)

		conf, _, err := Load(tmpf, nil, nil)
		require.NoError(t, err)
		require.Equal(t, RuntimeModeAllInOne, conf.Mode)
		require.Equal(t, runtime.ModeAllInOne, conf.Mode.Runtime())
	})

	t.Run("invalid value is rejected", func(t *testing.T) {
		tmpf, err := createTempFile([]byte("mode: cluster\npaths:\n  cam1:\n"))
		require.NoError(t, err)
		defer os.Remove(tmpf)

		_, _, err = Load(tmpf, nil, nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "invalid")
		require.Contains(t, err.Error(), "cluster")
	})

	t.Run("case-sensitive: uppercase is rejected", func(t *testing.T) {
		tmpf, err := createTempFile([]byte("mode: RECORDER\npaths:\n  cam1:\n"))
		require.NoError(t, err)
		defer os.Remove(tmpf)

		_, _, err = Load(tmpf, nil, nil)
		require.Error(t, err)
	})
}

// TestRuntimeModeFromEnv confirms MTX_MODE works via the env loader.
func TestRuntimeModeFromEnv(t *testing.T) {
	t.Setenv("MTX_MODE", "recorder")

	conf, _, err := Load("", nil, nil)
	require.NoError(t, err)
	require.Equal(t, RuntimeModeRecorder, conf.Mode)
}

func TestRuntimeModeFromEnvInvalid(t *testing.T) {
	t.Setenv("MTX_MODE", "bogus")

	_, _, err := Load("", nil, nil)
	require.Error(t, err)
}
