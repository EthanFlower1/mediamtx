package api

import (
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/bluenviron/mediamtx/internal/nvr/db"
)

// setupTrackTestDB creates an in-memory SQLite DB with the required schema.
func setupTrackTestDB(t *testing.T) *db.DB {
	t.Helper()
	d, err := db.Open(":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

func TestListTracksEmpty(t *testing.T) {
	d := setupTrackTestDB(t)
	handler := &TrackHandler{DB: d}

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/tracks", handler.ListTracks)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/tracks", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if resp["beta"] != true {
		t.Error("expected beta=true in response")
	}
	if count, ok := resp["count"].(float64); !ok || count != 0 {
		t.Errorf("expected count=0, got %v", resp["count"])
	}
}

func TestGetTrackNotFound(t *testing.T) {
	d := setupTrackTestDB(t)
	handler := &TrackHandler{DB: d}

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/tracks/:id", handler.GetTrack)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/tracks/9999", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCreateAndGetTrack(t *testing.T) {
	d := setupTrackTestDB(t)

	// Insert a camera so FK works.
	_, err := d.Exec(`INSERT INTO cameras (id, name) VALUES ('cam1', 'Front Door')`)
	if err != nil {
		t.Fatalf("insert camera: %v", err)
	}

	// Create a track directly via DB.
	track := &db.Track{
		Label:       "Test person",
		DetectionID: 1,
	}
	if err := d.InsertTrack(track); err != nil {
		t.Fatalf("insert track: %v", err)
	}

	// Add sightings.
	s1 := &db.Sighting{
		TrackID:    track.ID,
		CameraID:   "cam1",
		Timestamp:  "2026-04-10T10:00:00.000Z",
		Confidence: 0.92,
	}
	if err := d.InsertSighting(s1); err != nil {
		t.Fatalf("insert sighting: %v", err)
	}

	handler := &TrackHandler{DB: d}

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/tracks/:id", handler.GetTrack)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/tracks/1", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if resp["beta"] != true {
		t.Error("expected beta=true")
	}

	trackData, ok := resp["track"].(map[string]interface{})
	if !ok {
		t.Fatal("expected track object in response")
	}

	sightings, ok := trackData["sightings"].([]interface{})
	if !ok || len(sightings) != 1 {
		t.Errorf("expected 1 sighting, got %v", trackData["sightings"])
	}

	if int(trackData["camera_count"].(float64)) != 1 {
		t.Errorf("expected camera_count=1, got %v", trackData["camera_count"])
	}
}

func TestStartTrackingDetectionNotFound(t *testing.T) {
	d := setupTrackTestDB(t)
	handler := &TrackHandler{DB: d}

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/detections/:id/track", handler.StartTracking)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/detections/9999/track", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCosineSimilarity(t *testing.T) {
	a := []float32{1, 0, 0}
	b := []float32{1, 0, 0}
	sim := cosineSimilarity(a, b)
	if math.Abs(sim-1.0) > 0.001 {
		t.Errorf("identical vectors: expected 1.0, got %f", sim)
	}

	c := []float32{0, 1, 0}
	sim = cosineSimilarity(a, c)
	if math.Abs(sim) > 0.001 {
		t.Errorf("orthogonal vectors: expected 0.0, got %f", sim)
	}

	sim = cosineSimilarity(nil, nil)
	if sim != 0 {
		t.Errorf("nil vectors: expected 0.0, got %f", sim)
	}
}

func TestBytesToFloat32(t *testing.T) {
	// 1.0 in IEEE 754 little-endian: 0x3F800000
	b := []byte{0x00, 0x00, 0x80, 0x3F}
	result := bytesToFloat32(b)
	if len(result) != 1 {
		t.Fatalf("expected 1 float, got %d", len(result))
	}
	if math.Abs(float64(result[0])-1.0) > 0.001 {
		t.Errorf("expected 1.0, got %f", result[0])
	}

	empty := bytesToFloat32(nil)
	if empty != nil {
		t.Error("expected nil for empty input")
	}
}

func TestMultipleSightingsAcrossCameras(t *testing.T) {
	d := setupTrackTestDB(t)

	// Insert cameras.
	for _, cam := range []struct{ id, name string }{
		{"cam1", "Front Door"},
		{"cam2", "Parking Lot"},
		{"cam3", "Lobby"},
	} {
		_, err := d.Exec(`INSERT INTO cameras (id, name) VALUES (?, ?)`, cam.id, cam.name)
		if err != nil {
			t.Fatalf("insert camera %s: %v", cam.id, err)
		}
	}

	track := &db.Track{Label: "Multi-cam test", DetectionID: 1}
	if err := d.InsertTrack(track); err != nil {
		t.Fatalf("insert track: %v", err)
	}

	// Add sightings across 3 cameras.
	sightings := []db.Sighting{
		{TrackID: track.ID, CameraID: "cam1", Timestamp: "2026-04-10T10:00:00.000Z", Confidence: 0.95},
		{TrackID: track.ID, CameraID: "cam2", Timestamp: "2026-04-10T10:01:30.000Z", Confidence: 0.88},
		{TrackID: track.ID, CameraID: "cam3", Timestamp: "2026-04-10T10:03:00.000Z", Confidence: 0.82},
	}
	for i := range sightings {
		if err := d.InsertSighting(&sightings[i]); err != nil {
			t.Fatalf("insert sighting %d: %v", i, err)
		}
	}

	result, err := d.GetTrackWithSightings(track.ID)
	if err != nil {
		t.Fatalf("get track: %v", err)
	}

	if result.CameraCount != 3 {
		t.Errorf("expected 3 cameras, got %d", result.CameraCount)
	}
	if len(result.Sightings) != 3 {
		t.Errorf("expected 3 sightings, got %d", len(result.Sightings))
	}

	// Verify sightings are ordered by timestamp.
	for i := 1; i < len(result.Sightings); i++ {
		if result.Sightings[i].Timestamp < result.Sightings[i-1].Timestamp {
			t.Error("sightings not in chronological order")
		}
	}

	// Verify camera names resolved.
	expectedNames := map[string]string{
		"cam1": "Front Door",
		"cam2": "Parking Lot",
		"cam3": "Lobby",
	}
	for _, s := range result.Sightings {
		if s.CameraName != expectedNames[s.CameraID] {
			t.Errorf("camera %s: expected name %q, got %q", s.CameraID, expectedNames[s.CameraID], s.CameraName)
		}
	}
}
