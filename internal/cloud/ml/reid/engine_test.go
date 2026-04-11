package reid_test

import (
	"context"
	"testing"
	"time"

	"github.com/bluenviron/mediamtx/internal/cloud/ml/reid"
)

// TestEngineProcessNewTrack verifies that a detection with no prior tracks
// creates a new track.
func TestEngineProcessNewTrack(t *testing.T) {
	d := openTestDB(t)
	store := reid.NewStore(d)
	extractor := reid.NewFakeExtractor(8)
	graph := reid.NewAdjacencyGraph()
	cfg := reid.DefaultMatchConfig()
	engine := reid.NewEngine(extractor, store, graph, cfg)
	defer engine.Close()

	ctx := context.Background()
	result, err := engine.Process(ctx, tenantAlpha, reid.Detection{
		CameraID:  "cam-lobby",
		ImageData: []byte("person-image-1"),
		Width:     128,
		Height:    256,
		Timestamp: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if !result.IsNew {
		t.Error("expected IsNew=true for first detection")
	}
	if result.TrackID == "" {
		t.Error("expected non-empty TrackID")
	}
	if result.CameraID != "cam-lobby" {
		t.Errorf("camera_id = %q, want cam-lobby", result.CameraID)
	}
}

// TestEngineProcessMatchExisting verifies that a detection with the same
// image data matches the existing track.
func TestEngineProcessMatchExisting(t *testing.T) {
	d := openTestDB(t)
	store := reid.NewStore(d)
	extractor := reid.NewFakeExtractor(8)
	graph := reid.NewAdjacencyGraph()
	graph.AddEdge("cam-lobby", "cam-hallway", 30*time.Second)
	cfg := reid.DefaultMatchConfig()
	cfg.SimilarityThreshold = 0.5
	engine := reid.NewEngine(extractor, store, graph, cfg)
	defer engine.Close()

	ctx := context.Background()
	now := time.Now().UTC()

	// First detection creates a track.
	r1, err := engine.Process(ctx, tenantAlpha, reid.Detection{
		CameraID:  "cam-lobby",
		ImageData: []byte("person-A-image"),
		Width:     128,
		Height:    256,
		Timestamp: now,
	})
	if err != nil {
		t.Fatalf("Process 1: %v", err)
	}
	if !r1.IsNew {
		t.Fatal("first detection should create a new track")
	}

	// Same person seen at adjacent camera a minute later.
	r2, err := engine.Process(ctx, tenantAlpha, reid.Detection{
		CameraID:  "cam-hallway",
		ImageData: []byte("person-A-image"),
		Width:     128,
		Height:    256,
		Timestamp: now.Add(1 * time.Minute),
	})
	if err != nil {
		t.Fatalf("Process 2: %v", err)
	}
	if r2.IsNew {
		t.Error("second detection of same person should match existing track")
	}
	if r2.TrackID != r1.TrackID {
		t.Errorf("track_id mismatch: r1=%q, r2=%q", r1.TrackID, r2.TrackID)
	}
}

// TestEngineProcessDifferentPeople verifies that different image data
// creates separate tracks.
func TestEngineProcessDifferentPeople(t *testing.T) {
	d := openTestDB(t)
	store := reid.NewStore(d)
	extractor := reid.NewFakeExtractor(8)
	graph := reid.NewAdjacencyGraph()
	cfg := reid.DefaultMatchConfig()
	cfg.SimilarityThreshold = 0.9 // High threshold = strict matching
	engine := reid.NewEngine(extractor, store, graph, cfg)
	defer engine.Close()

	ctx := context.Background()
	now := time.Now().UTC()

	r1, err := engine.Process(ctx, tenantAlpha, reid.Detection{
		CameraID:  "cam-lobby",
		ImageData: []byte("person-A-totally-unique-image-data"),
		Width:     128,
		Height:    256,
		Timestamp: now,
	})
	if err != nil {
		t.Fatalf("Process person A: %v", err)
	}

	r2, err := engine.Process(ctx, tenantAlpha, reid.Detection{
		CameraID:  "cam-lobby",
		ImageData: []byte("person-B-completely-different-image"),
		Width:     128,
		Height:    256,
		Timestamp: now.Add(10 * time.Second),
	})
	if err != nil {
		t.Fatalf("Process person B: %v", err)
	}

	if r1.TrackID == r2.TrackID {
		t.Error("different people should get different track IDs")
	}
	if !r1.IsNew || !r2.IsNew {
		t.Error("both detections should be new tracks")
	}
}

// TestEngineProcessThreeCameras verifies track persistence across 3+ cameras,
// which is a key acceptance criterion.
func TestEngineProcessThreeCameras(t *testing.T) {
	d := openTestDB(t)
	store := reid.NewStore(d)
	extractor := reid.NewFakeExtractor(8)

	graph := reid.NewAdjacencyGraph()
	graph.AddEdge("cam-entrance", "cam-lobby", 20*time.Second)
	graph.AddEdge("cam-lobby", "cam-corridor", 30*time.Second)
	graph.AddEdge("cam-corridor", "cam-office", 45*time.Second)

	cfg := reid.DefaultMatchConfig()
	cfg.SimilarityThreshold = 0.5
	engine := reid.NewEngine(extractor, store, graph, cfg)
	defer engine.Close()

	ctx := context.Background()
	now := time.Now().UTC()
	imageData := []byte("same-person-consistent-image")

	// Camera 1: entrance.
	r1, err := engine.Process(ctx, tenantAlpha, reid.Detection{
		CameraID:  "cam-entrance",
		ImageData: imageData,
		Width:     128,
		Height:    256,
		Timestamp: now,
	})
	if err != nil {
		t.Fatalf("Process cam-entrance: %v", err)
	}
	if !r1.IsNew {
		t.Fatal("first sighting should be new")
	}
	trackID := r1.TrackID

	// Camera 2: lobby (30s later).
	r2, err := engine.Process(ctx, tenantAlpha, reid.Detection{
		CameraID:  "cam-lobby",
		ImageData: imageData,
		Width:     128,
		Height:    256,
		Timestamp: now.Add(30 * time.Second),
	})
	if err != nil {
		t.Fatalf("Process cam-lobby: %v", err)
	}
	if r2.IsNew {
		t.Error("same person at cam-lobby should match existing track")
	}
	if r2.TrackID != trackID {
		t.Errorf("cam-lobby track_id = %q, want %q", r2.TrackID, trackID)
	}

	// Camera 3: corridor (90s later).
	r3, err := engine.Process(ctx, tenantAlpha, reid.Detection{
		CameraID:  "cam-corridor",
		ImageData: imageData,
		Width:     128,
		Height:    256,
		Timestamp: now.Add(90 * time.Second),
	})
	if err != nil {
		t.Fatalf("Process cam-corridor: %v", err)
	}
	if r3.IsNew {
		t.Error("same person at cam-corridor should match existing track")
	}
	if r3.TrackID != trackID {
		t.Errorf("cam-corridor track_id = %q, want %q", r3.TrackID, trackID)
	}

	// Verify sightings trail.
	sightings, err := store.ListSightings(ctx, tenantAlpha, trackID, 10)
	if err != nil {
		t.Fatalf("ListSightings: %v", err)
	}
	if len(sightings) != 3 {
		t.Errorf("sightings count = %d, want 3 (one per camera)", len(sightings))
	}

	// Verify track was updated.
	track, err := store.GetTrack(ctx, tenantAlpha, trackID)
	if err != nil {
		t.Fatalf("GetTrack: %v", err)
	}
	if track.MatchCount < 3 {
		t.Errorf("match_count = %d, want >= 3", track.MatchCount)
	}
	if track.LastCamera != "cam-corridor" {
		t.Errorf("last_camera = %q, want cam-corridor", track.LastCamera)
	}
	if track.FirstCamera != "cam-entrance" {
		t.Errorf("first_camera = %q, want cam-entrance", track.FirstCamera)
	}
}

// TestEngineProcessTenantIsolation verifies that tracks from one tenant
// are invisible to another.
func TestEngineProcessTenantIsolation(t *testing.T) {
	d := openTestDB(t)
	store := reid.NewStore(d)
	extractor := reid.NewFakeExtractor(8)
	graph := reid.NewAdjacencyGraph()
	cfg := reid.DefaultMatchConfig()
	cfg.SimilarityThreshold = 0.5
	engine := reid.NewEngine(extractor, store, graph, cfg)
	defer engine.Close()

	ctx := context.Background()
	now := time.Now().UTC()
	imageData := []byte("shared-person-image")

	// Alpha's detection.
	r1, err := engine.Process(ctx, tenantAlpha, reid.Detection{
		CameraID:  "cam-A",
		ImageData: imageData,
		Width:     128,
		Height:    256,
		Timestamp: now,
	})
	if err != nil {
		t.Fatalf("Process Alpha: %v", err)
	}

	// Bravo's detection with the same image — should NOT match Alpha's track.
	r2, err := engine.Process(ctx, tenantBravo, reid.Detection{
		CameraID:  "cam-A",
		ImageData: imageData,
		Width:     128,
		Height:    256,
		Timestamp: now.Add(10 * time.Second),
	})
	if err != nil {
		t.Fatalf("Process Bravo: %v", err)
	}
	if !r2.IsNew {
		t.Error("Bravo should not match Alpha's track")
	}
	if r2.TrackID == r1.TrackID {
		t.Error("different tenants should have different track IDs")
	}
}

// TestEngineProcessBatch tests batch processing.
func TestEngineProcessBatch(t *testing.T) {
	d := openTestDB(t)
	store := reid.NewStore(d)
	extractor := reid.NewFakeExtractor(8)
	graph := reid.NewAdjacencyGraph()
	cfg := reid.DefaultMatchConfig()
	cfg.SimilarityThreshold = 0.9
	engine := reid.NewEngine(extractor, store, graph, cfg)
	defer engine.Close()

	ctx := context.Background()
	now := time.Now().UTC()

	detections := []reid.Detection{
		{CameraID: "cam-A", ImageData: []byte("alpha-person-unique-biometric-data-set-one-xyzzy"), Width: 128, Height: 256, Timestamp: now},
		{CameraID: "cam-B", ImageData: []byte("bravo-individual-completely-different-features-qwert"), Width: 128, Height: 256, Timestamp: now},
		{CameraID: "cam-C", ImageData: []byte("charlie-third-subject-distinct-appearance-zxcvb"), Width: 128, Height: 256, Timestamp: now},
	}

	results, err := engine.ProcessBatch(ctx, tenantAlpha, detections)
	if err != nil {
		t.Fatalf("ProcessBatch: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("results count = %d, want 3", len(results))
	}

	// All should be new tracks with unique IDs.
	ids := map[string]bool{}
	for i, r := range results {
		if !r.IsNew {
			t.Errorf("result[%d]: expected IsNew=true", i)
		}
		if ids[r.TrackID] {
			t.Errorf("result[%d]: duplicate track ID %q", i, r.TrackID)
		}
		ids[r.TrackID] = true
	}
}
