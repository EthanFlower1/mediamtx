package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCheckHealth_Healthy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ok := checkHealth(srv.URL, 2*time.Second)
	require.True(t, ok)
}

func TestCheckHealth_Unhealthy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	ok := checkHealth(srv.URL, 2*time.Second)
	require.False(t, ok)
}

func TestCheckHealth_Timeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(3 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ok := checkHealth(srv.URL, 500*time.Millisecond)
	require.False(t, ok)
}

func TestCheckHealth_ConnectionRefused(t *testing.T) {
	ok := checkHealth("http://127.0.0.1:1", 1*time.Second)
	require.False(t, ok)
}

func TestHistoryPersistence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "history.json")

	h := loadHistory(path)
	require.Empty(t, h.Restarts)

	h.Restarts = append(h.Restarts, restartRecord{
		Timestamp: time.Now().UTC(),
		Reason:    "test crash",
		ExitCode:  1,
		Attempt:   1,
	})
	saveHistory(path, h)

	// Reload and verify.
	h2 := loadHistory(path)
	require.Len(t, h2.Restarts, 1)
	require.Equal(t, "test crash", h2.Restarts[0].Reason)
	require.Equal(t, 1, h2.Restarts[0].ExitCode)

	// Verify JSON is well-formed.
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var raw map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &raw))
}

func TestExitCodeFrom(t *testing.T) {
	require.Equal(t, 0, exitCodeFrom(nil))
	require.Equal(t, -1, exitCodeFrom(os.ErrNotExist))
}
