package reid_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	clouddb "github.com/bluenviron/mediamtx/internal/cloud/db"
	"github.com/bluenviron/mediamtx/internal/cloud/ml/reid"
)

func openTestDB(t *testing.T) *clouddb.DB {
	t.Helper()
	dir := t.TempDir()
	dsn := "sqlite://" + filepath.Join(dir, "reid_test.db")
	d, err := clouddb.Open(context.Background(), dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	return d
}

const (
	tenantAlpha = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	tenantBravo = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
)

func testEmbedding() []float32 {
	emb := make([]float32, 8)
	for i := range emb {
		emb[i] = float32(i+1) * 0.1
	}
	reid.NormalizeEmbedding(emb)
	return emb
}

// -----------------------------------------------------------------------
// Track CRUD
// -----------------------------------------------------------------------

func TestCreateGetTrackRoundTrip(t *testing.T) {
	d := openTestDB(t)
	store := reid.NewStore(d)
	ctx := context.Background()

	emb := testEmbedding()
	created, err := store.CreateTrack(ctx, reid.CreateTrackInput{
		TenantID:  tenantAlpha,
		Embedding: emb,
		CameraID:  "cam-lobby",
		SeenAt:    time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("CreateTrack: %v", err)
	}
	if created.ID == "" {
		t.Fatal("expected non-empty ID")
	}
	if created.FirstCamera != "cam-lobby" {
		t.Errorf("first_camera = %q, want cam-lobby", created.FirstCamera)
	}
	if created.LastCamera != "cam-lobby" {
		t.Errorf("last_camera = %q, want cam-lobby", created.LastCamera)
	}
	if created.MatchCount != 1 {
		t.Errorf("match_count = %d, want 1", created.MatchCount)
	}

	got, err := store.GetTrack(ctx, tenantAlpha, created.ID)
	if err != nil {
		t.Fatalf("GetTrack: %v", err)
	}
	if got.ID != created.ID {
		t.Errorf("id mismatch: got %q, want %q", got.ID, created.ID)
	}
	if len(got.Embedding) != len(emb) {
		t.Errorf("embedding dim: got %d, want %d", len(got.Embedding), len(emb))
	}
}

func TestTrackTenantIsolation(t *testing.T) {
	d := openTestDB(t)
	store := reid.NewStore(d)
	ctx := context.Background()

	emb := testEmbedding()
	created, err := store.CreateTrack(ctx, reid.CreateTrackInput{
		TenantID:  tenantAlpha,
		Embedding: emb,
		CameraID:  "cam-A",
		SeenAt:    time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("CreateTrack: %v", err)
	}

	// Bravo cannot see Alpha's track.
	_, err = store.GetTrack(ctx, tenantBravo, created.ID)
	if !errors.Is(err, reid.ErrNotFound) {
		t.Errorf("cross-tenant GetTrack: expected ErrNotFound, got %v", err)
	}
}

func TestUpdateTrackMatch(t *testing.T) {
	d := openTestDB(t)
	store := reid.NewStore(d)
	ctx := context.Background()

	emb := testEmbedding()
	created, err := store.CreateTrack(ctx, reid.CreateTrackInput{
		TenantID:  tenantAlpha,
		Embedding: emb,
		CameraID:  "cam-A",
		SeenAt:    time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("CreateTrack: %v", err)
	}

	newEmb := make([]float32, len(emb))
	copy(newEmb, emb)
	newEmb[0] += 0.1
	reid.NormalizeEmbedding(newEmb)

	newSeen := time.Now().UTC().Add(5 * time.Minute)
	err = store.UpdateTrackMatch(ctx, reid.UpdateTrackInput{
		TenantID:   tenantAlpha,
		TrackID:    created.ID,
		Embedding:  newEmb,
		LastCamera: "cam-B",
		LastSeen:   newSeen,
	})
	if err != nil {
		t.Fatalf("UpdateTrackMatch: %v", err)
	}

	got, err := store.GetTrack(ctx, tenantAlpha, created.ID)
	if err != nil {
		t.Fatalf("GetTrack after update: %v", err)
	}
	if got.LastCamera != "cam-B" {
		t.Errorf("last_camera = %q, want cam-B", got.LastCamera)
	}
	if got.MatchCount != 2 {
		t.Errorf("match_count = %d, want 2", got.MatchCount)
	}
}

func TestListActiveTracks(t *testing.T) {
	d := openTestDB(t)
	store := reid.NewStore(d)
	ctx := context.Background()

	emb := testEmbedding()
	now := time.Now().UTC()

	// Create two tracks: one recent, one old.
	_, err := store.CreateTrack(ctx, reid.CreateTrackInput{
		TenantID:  tenantAlpha,
		Embedding: emb,
		CameraID:  "cam-A",
		SeenAt:    now,
	})
	if err != nil {
		t.Fatalf("CreateTrack 1: %v", err)
	}

	old, err := store.CreateTrack(ctx, reid.CreateTrackInput{
		TenantID:  tenantAlpha,
		Embedding: emb,
		CameraID:  "cam-B",
		SeenAt:    now.Add(-2 * time.Hour),
	})
	if err != nil {
		t.Fatalf("CreateTrack 2: %v", err)
	}

	// Backdate the old track's last_seen.
	err = store.UpdateTrackMatch(ctx, reid.UpdateTrackInput{
		TenantID:   tenantAlpha,
		TrackID:    old.ID,
		Embedding:  emb,
		LastCamera: "cam-B",
		LastSeen:   now.Add(-2 * time.Hour),
	})
	if err != nil {
		t.Fatalf("UpdateTrackMatch (backdate): %v", err)
	}

	// List tracks from the last 30 minutes.
	active, err := store.ListActiveTracks(ctx, tenantAlpha, now.Add(-30*time.Minute), nil)
	if err != nil {
		t.Fatalf("ListActiveTracks: %v", err)
	}
	if len(active) != 1 {
		t.Errorf("active count = %d, want 1", len(active))
	}
}

func TestDeleteTrack(t *testing.T) {
	d := openTestDB(t)
	store := reid.NewStore(d)
	ctx := context.Background()

	emb := testEmbedding()
	created, err := store.CreateTrack(ctx, reid.CreateTrackInput{
		TenantID:  tenantAlpha,
		Embedding: emb,
		CameraID:  "cam-A",
		SeenAt:    time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("CreateTrack: %v", err)
	}

	// Add a sighting.
	_, err = store.CreateSighting(ctx, reid.CreateSightingInput{
		TenantID:   tenantAlpha,
		TrackID:    created.ID,
		CameraID:   "cam-A",
		Embedding:  emb,
		Confidence: 0.95,
		SeenAt:     time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("CreateSighting: %v", err)
	}

	// Delete.
	if err := store.DeleteTrack(ctx, tenantAlpha, created.ID); err != nil {
		t.Fatalf("DeleteTrack: %v", err)
	}

	// Should be gone.
	_, err = store.GetTrack(ctx, tenantAlpha, created.ID)
	if !errors.Is(err, reid.ErrNotFound) {
		t.Errorf("expected ErrNotFound after delete, got %v", err)
	}

	// Sightings should be gone too.
	sightings, err := store.ListSightings(ctx, tenantAlpha, created.ID, 10)
	if err != nil {
		t.Fatalf("ListSightings after delete: %v", err)
	}
	if len(sightings) != 0 {
		t.Errorf("sightings count = %d, want 0 after delete", len(sightings))
	}
}

// -----------------------------------------------------------------------
// Sighting CRUD
// -----------------------------------------------------------------------

func TestCreateListSightings(t *testing.T) {
	d := openTestDB(t)
	store := reid.NewStore(d)
	ctx := context.Background()

	emb := testEmbedding()
	track, err := store.CreateTrack(ctx, reid.CreateTrackInput{
		TenantID:  tenantAlpha,
		Embedding: emb,
		CameraID:  "cam-A",
		SeenAt:    time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("CreateTrack: %v", err)
	}

	// Create 3 sightings.
	for i := 0; i < 3; i++ {
		_, err := store.CreateSighting(ctx, reid.CreateSightingInput{
			TenantID:   tenantAlpha,
			TrackID:    track.ID,
			CameraID:   "cam-A",
			Embedding:  emb,
			Confidence: 0.9,
			BBoxX:      100.0,
			BBoxY:      200.0,
			BBoxW:      50.0,
			BBoxH:      120.0,
			SeenAt:     time.Now().UTC(),
		})
		if err != nil {
			t.Fatalf("CreateSighting %d: %v", i, err)
		}
	}

	sightings, err := store.ListSightings(ctx, tenantAlpha, track.ID, 10)
	if err != nil {
		t.Fatalf("ListSightings: %v", err)
	}
	if len(sightings) != 3 {
		t.Errorf("sightings count = %d, want 3", len(sightings))
	}

	// Check bbox fields persisted.
	s := sightings[0]
	if s.BBoxX != 100.0 {
		t.Errorf("bbox_x = %f, want 100.0", s.BBoxX)
	}
	if s.Confidence != 0.9 {
		t.Errorf("confidence = %f, want 0.9", s.Confidence)
	}
}

func TestDeleteTrackCrossTenant(t *testing.T) {
	d := openTestDB(t)
	store := reid.NewStore(d)
	ctx := context.Background()

	emb := testEmbedding()
	created, err := store.CreateTrack(ctx, reid.CreateTrackInput{
		TenantID:  tenantAlpha,
		Embedding: emb,
		CameraID:  "cam-A",
		SeenAt:    time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("CreateTrack: %v", err)
	}

	// Bravo tries to delete Alpha's track.
	err = store.DeleteTrack(ctx, tenantBravo, created.ID)
	if !errors.Is(err, reid.ErrNotFound) {
		t.Errorf("cross-tenant DeleteTrack: expected ErrNotFound, got %v", err)
	}

	// Track should still exist.
	_, err = store.GetTrack(ctx, tenantAlpha, created.ID)
	if err != nil {
		t.Errorf("track deleted by wrong tenant: %v", err)
	}
}
