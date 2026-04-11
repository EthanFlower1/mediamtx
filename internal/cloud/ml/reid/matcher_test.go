package reid

import (
	"testing"
	"time"
)

func makeTrack(id, camera string, embedding []float32, lastSeen time.Time) Track {
	return Track{
		ID:           id,
		TenantID:     "tenant-a",
		Embedding:    embedding,
		EmbeddingDim: len(embedding),
		FirstCamera:  camera,
		LastCamera:   camera,
		FirstSeen:    lastSeen,
		LastSeen:     lastSeen,
		MatchCount:   1,
	}
}

func TestMatcherExactMatch(t *testing.T) {
	cfg := DefaultMatchConfig()
	cfg.SimilarityThreshold = 0.5
	graph := NewAdjacencyGraph()
	m := NewMatcher(cfg, graph)

	emb := []float32{1.0, 0.0, 0.0, 0.0}
	NormalizeEmbedding(emb)

	now := time.Now()
	tracks := []Track{
		makeTrack("track-1", "cam-A", emb, now.Add(-1*time.Minute)),
	}

	result := m.Match(emb, "cam-A", now, tracks)
	if result == nil {
		t.Fatal("expected a match for identical embeddings")
	}
	if result.TrackID != "track-1" {
		t.Errorf("track_id = %q, want track-1", result.TrackID)
	}
	if result.RawSim < 0.99 {
		t.Errorf("raw_sim = %f, want ~1.0", result.RawSim)
	}
}

func TestMatcherNoMatch(t *testing.T) {
	cfg := DefaultMatchConfig()
	cfg.SimilarityThreshold = 0.9
	graph := NewAdjacencyGraph()
	m := NewMatcher(cfg, graph)

	emb := []float32{1.0, 0.0, 0.0, 0.0}
	NormalizeEmbedding(emb)

	// Orthogonal embedding.
	other := []float32{0.0, 1.0, 0.0, 0.0}
	NormalizeEmbedding(other)

	now := time.Now()
	tracks := []Track{
		makeTrack("track-1", "cam-A", other, now.Add(-1*time.Minute)),
	}

	result := m.Match(emb, "cam-A", now, tracks)
	if result != nil {
		t.Errorf("expected no match for orthogonal embeddings, got %+v", result)
	}
}

func TestMatcherAdjacencyBoost(t *testing.T) {
	cfg := DefaultMatchConfig()
	cfg.SimilarityThreshold = 0.6
	cfg.AdjacencyBoost = 0.15

	graph := NewAdjacencyGraph()
	graph.AddEdge("cam-A", "cam-B", 30*time.Second)

	m := NewMatcher(cfg, graph)

	// Two tracks with similar embeddings.
	emb1 := []float32{0.9, 0.1, 0.0, 0.0}
	NormalizeEmbedding(emb1)

	emb2 := []float32{0.85, 0.15, 0.05, 0.0}
	NormalizeEmbedding(emb2)

	query := []float32{0.88, 0.12, 0.02, 0.0}
	NormalizeEmbedding(query)

	now := time.Now()
	tracks := []Track{
		makeTrack("track-far", "cam-C", emb1, now.Add(-2*time.Minute)),
		makeTrack("track-near", "cam-A", emb2, now.Add(-1*time.Minute)),
	}

	// Query from cam-B (adjacent to cam-A where track-near was).
	result := m.Match(query, "cam-B", now, tracks)
	if result == nil {
		t.Fatal("expected a match")
	}
	// track-near should get an adjacency boost.
	if result.TrackID != "track-near" {
		t.Errorf("expected track-near (adjacent boost), got %q", result.TrackID)
	}
	if result.AdjBoost <= 0 {
		t.Errorf("expected adjacency boost > 0, got %f", result.AdjBoost)
	}
}

func TestMatcherBestOfMultiple(t *testing.T) {
	cfg := DefaultMatchConfig()
	cfg.SimilarityThreshold = 0.5
	m := NewMatcher(cfg, nil)

	query := []float32{1.0, 0.0, 0.0, 0.0}
	NormalizeEmbedding(query)

	good := []float32{0.95, 0.05, 0.0, 0.0}
	NormalizeEmbedding(good)

	mediocre := []float32{0.7, 0.3, 0.0, 0.0}
	NormalizeEmbedding(mediocre)

	now := time.Now()
	tracks := []Track{
		makeTrack("mediocre", "cam-A", mediocre, now.Add(-1*time.Minute)),
		makeTrack("good", "cam-A", good, now.Add(-1*time.Minute)),
	}

	result := m.Match(query, "cam-A", now, tracks)
	if result == nil {
		t.Fatal("expected a match")
	}
	if result.TrackID != "good" {
		t.Errorf("expected best match 'good', got %q", result.TrackID)
	}
}

func TestMatcherEmptyCandidates(t *testing.T) {
	cfg := DefaultMatchConfig()
	m := NewMatcher(cfg, nil)

	emb := []float32{1.0, 0.0}
	result := m.Match(emb, "cam-A", time.Now(), nil)
	if result != nil {
		t.Errorf("expected nil for empty candidates, got %+v", result)
	}
}

func TestMatcherTimePenalty(t *testing.T) {
	cfg := DefaultMatchConfig()
	cfg.SimilarityThreshold = 0.5
	cfg.TimeWindow = 10 * time.Minute
	m := NewMatcher(cfg, nil)

	emb := []float32{1.0, 0.0, 0.0, 0.0}
	NormalizeEmbedding(emb)

	now := time.Now()

	// Two identical tracks: one recent, one old.
	tracks := []Track{
		makeTrack("recent", "cam-A", emb, now.Add(-30*time.Second)),
		makeTrack("old", "cam-A", emb, now.Add(-9*time.Minute)),
	}

	result := m.Match(emb, "cam-A", now, tracks)
	if result == nil {
		t.Fatal("expected a match")
	}
	// Recent track should score higher due to lower time penalty.
	if result.TrackID != "recent" {
		t.Errorf("expected 'recent' (lower time penalty), got %q", result.TrackID)
	}
}
