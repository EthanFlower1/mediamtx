package timeline

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandler_ValidRequest(t *testing.T) {
	store := &fakeStore{segments: []Segment{
		{CameraID: "cam-1", RecorderID: "rec-A", Start: t1, End: t3},
	}}
	h := Handler(NewAssembler(store))

	url := fmt.Sprintf("/api/v1/timeline?cameras=cam-1&start=%s&end=%s",
		t0.Format(time.RFC3339), t6.Format(time.RFC3339))
	req := httptest.NewRequest(http.MethodGet, url, nil)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))

	var resp TimelineResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Len(t, resp.Segments, 1)
	assert.Len(t, resp.Gaps, 2)
}

func TestHandler_MissingCameras(t *testing.T) {
	h := Handler(NewAssembler(&fakeStore{}))

	url := fmt.Sprintf("/api/v1/timeline?start=%s&end=%s",
		t0.Format(time.RFC3339), t6.Format(time.RFC3339))
	req := httptest.NewRequest(http.MethodGet, url, nil)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	var body map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "BAD_REQUEST", body["code"])
	assert.Contains(t, body["message"], "cameras")
}

func TestHandler_MissingStart(t *testing.T) {
	h := Handler(NewAssembler(&fakeStore{}))

	url := fmt.Sprintf("/api/v1/timeline?cameras=cam-1&end=%s", t6.Format(time.RFC3339))
	req := httptest.NewRequest(http.MethodGet, url, nil)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandler_MissingEnd(t *testing.T) {
	h := Handler(NewAssembler(&fakeStore{}))

	url := fmt.Sprintf("/api/v1/timeline?cameras=cam-1&start=%s", t0.Format(time.RFC3339))
	req := httptest.NewRequest(http.MethodGet, url, nil)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandler_InvalidTimeFormat(t *testing.T) {
	h := Handler(NewAssembler(&fakeStore{}))

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/timeline?cameras=cam-1&start=not-a-date&end=2026-01-02T00:00:00Z", nil)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	var body map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Contains(t, body["message"], "invalid start time")
}

func TestHandler_EndBeforeStart(t *testing.T) {
	h := Handler(NewAssembler(&fakeStore{}))

	url := fmt.Sprintf("/api/v1/timeline?cameras=cam-1&start=%s&end=%s",
		t6.Format(time.RFC3339), t0.Format(time.RFC3339))
	req := httptest.NewRequest(http.MethodGet, url, nil)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	var body map[string]string
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Contains(t, body["message"], "end must be after start")
}

func TestHandler_WrongMethod(t *testing.T) {
	h := Handler(NewAssembler(&fakeStore{}))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/timeline", nil)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
}

func TestHandler_MultipleCameras(t *testing.T) {
	store := &fakeStore{segments: []Segment{
		{CameraID: "cam-1", RecorderID: "rec-A", Start: t0, End: t2},
		{CameraID: "cam-2", RecorderID: "rec-B", Start: t1, End: t3},
	}}
	h := Handler(NewAssembler(store))

	url := fmt.Sprintf("/api/v1/timeline?cameras=cam-1,cam-2&start=%s&end=%s",
		t0.Format(time.RFC3339), t4.Format(time.RFC3339))
	req := httptest.NewRequest(http.MethodGet, url, nil)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	var resp TimelineResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Len(t, resp.Segments, 2)
}

func TestHandler_StoreError(t *testing.T) {
	store := &fakeStore{err: fmt.Errorf("db gone")}
	h := Handler(NewAssembler(store))

	url := fmt.Sprintf("/api/v1/timeline?cameras=cam-1&start=%s&end=%s",
		t0.Format(time.RFC3339), t6.Format(time.RFC3339))
	req := httptest.NewRequest(http.MethodGet, url, nil)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}
