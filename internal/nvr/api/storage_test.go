package api

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/nvr/db"
	"github.com/bluenviron/mediamtx/internal/nvr/storage"
)

func newTestDBForStorage(t *testing.T) *db.DB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	d, err := db.Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { d.Close() })
	return d
}

func TestStorageStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)
	d := newTestDBForStorage(t)
	mgr := storage.New(d, nil, "./recordings", ":9997")

	h := &StorageHandler{DB: d, Manager: mgr}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/api/storage/status", nil)
	h.Status(c)

	require.Equal(t, http.StatusOK, w.Code)
}

func TestStoragePending(t *testing.T) {
	gin.SetMode(gin.TestMode)
	d := newTestDBForStorage(t)
	mgr := storage.New(d, nil, "./recordings", ":9997")

	h := &StorageHandler{DB: d, Manager: mgr}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/api/storage/pending", nil)
	h.Pending(c)

	require.Equal(t, http.StatusOK, w.Code)
}
