package reid

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Engine is the top-level orchestrator for cross-camera re-identification.
// It wires together the embedding extractor, matching engine, adjacency graph,
// and persistent store.
type Engine struct {
	extractor EmbeddingExtractor
	matcher   *Matcher
	store     Store
	cfg       MatchConfig

	mu     sync.RWMutex
	closed bool
}

// NewEngine creates a re-id Engine.
func NewEngine(
	extractor EmbeddingExtractor,
	store Store,
	graph *AdjacencyGraph,
	cfg MatchConfig,
) *Engine {
	return &Engine{
		extractor: extractor,
		matcher:   NewMatcher(cfg, graph),
		store:     store,
		cfg:       cfg,
	}
}

// Process takes a person detection, extracts an embedding, matches it against
// existing tracks, and either updates the matched track or creates a new one.
// Returns the match result with the assigned track ID.
func (e *Engine) Process(ctx context.Context, tenantID string, det Detection) (MatchResult, error) {
	e.mu.RLock()
	if e.closed {
		e.mu.RUnlock()
		return MatchResult{}, fmt.Errorf("reid: engine closed")
	}
	e.mu.RUnlock()

	if tenantID == "" {
		return MatchResult{}, ErrInvalidTenantID
	}

	// Step 1: Extract embedding from the person crop.
	embedding, err := e.extractor.Extract(ctx, det.ImageData, det.Width, det.Height)
	if err != nil {
		return MatchResult{}, fmt.Errorf("reid: extract embedding: %w", err)
	}

	timestamp := det.Timestamp
	if timestamp.IsZero() {
		timestamp = time.Now().UTC()
	}

	// Step 2: Load active tracks within the time window.
	since := timestamp.Add(-e.cfg.TimeWindow)
	activeTracks, err := e.store.ListActiveTracks(ctx, tenantID, since, nil)
	if err != nil {
		return MatchResult{}, fmt.Errorf("reid: list active tracks: %w", err)
	}

	// Step 3: Match against active tracks.
	match := e.matcher.Match(embedding, det.CameraID, timestamp, activeTracks)

	var result MatchResult
	if match != nil {
		// Update existing track.
		// Compute exponential moving average of embeddings for track stability.
		existingTrack, err := e.store.GetTrack(ctx, tenantID, match.TrackID)
		if err != nil {
			return MatchResult{}, fmt.Errorf("reid: get matched track: %w", err)
		}

		avgEmb := exponentialMovingAverage(existingTrack.Embedding, embedding, 0.3)

		if err := e.store.UpdateTrackMatch(ctx, UpdateTrackInput{
			TenantID:   tenantID,
			TrackID:    match.TrackID,
			Embedding:  avgEmb,
			LastCamera: det.CameraID,
			LastSeen:   timestamp,
		}); err != nil {
			return MatchResult{}, fmt.Errorf("reid: update track: %w", err)
		}

		result = MatchResult{
			TrackID:    match.TrackID,
			IsNew:      false,
			Confidence: match.Score,
			CameraID:   det.CameraID,
		}
	} else {
		// Create a new track.
		track, err := e.store.CreateTrack(ctx, CreateTrackInput{
			TenantID:  tenantID,
			Embedding: embedding,
			CameraID:  det.CameraID,
			SeenAt:    timestamp,
		})
		if err != nil {
			return MatchResult{}, fmt.Errorf("reid: create track: %w", err)
		}

		result = MatchResult{
			TrackID:    track.ID,
			IsNew:      true,
			Confidence: 1.0,
			CameraID:   det.CameraID,
		}
	}

	// Step 4: Record the sighting.
	confidence := result.Confidence
	if _, err := e.store.CreateSighting(ctx, CreateSightingInput{
		TenantID:   tenantID,
		TrackID:    result.TrackID,
		CameraID:   det.CameraID,
		Embedding:  embedding,
		Confidence: confidence,
		BBoxX:      det.BBoxX,
		BBoxY:      det.BBoxY,
		BBoxW:      det.BBoxW,
		BBoxH:      det.BBoxH,
		SeenAt:     timestamp,
	}); err != nil {
		return MatchResult{}, fmt.Errorf("reid: create sighting: %w", err)
	}

	return result, nil
}

// ProcessBatch processes multiple detections in a single call. Embeddings
// are extracted in batch for throughput, then each is matched individually.
func (e *Engine) ProcessBatch(ctx context.Context, tenantID string, detections []Detection) ([]MatchResult, error) {
	e.mu.RLock()
	if e.closed {
		e.mu.RUnlock()
		return nil, fmt.Errorf("reid: engine closed")
	}
	e.mu.RUnlock()

	if tenantID == "" {
		return nil, ErrInvalidTenantID
	}
	if len(detections) == 0 {
		return nil, nil
	}

	// Batch extract embeddings.
	images := make([]ImageInput, len(detections))
	for i, det := range detections {
		images[i] = ImageInput{
			Data:   det.ImageData,
			Width:  det.Width,
			Height: det.Height,
		}
	}

	embeddings, err := e.extractor.BatchExtract(ctx, images)
	if err != nil {
		return nil, fmt.Errorf("reid: batch extract: %w", err)
	}

	results := make([]MatchResult, len(detections))
	for i, det := range detections {
		timestamp := det.Timestamp
		if timestamp.IsZero() {
			timestamp = time.Now().UTC()
		}

		since := timestamp.Add(-e.cfg.TimeWindow)
		activeTracks, err := e.store.ListActiveTracks(ctx, tenantID, since, nil)
		if err != nil {
			return nil, fmt.Errorf("reid: list active tracks for batch item %d: %w", i, err)
		}

		match := e.matcher.Match(embeddings[i], det.CameraID, timestamp, activeTracks)

		if match != nil {
			existingTrack, err := e.store.GetTrack(ctx, tenantID, match.TrackID)
			if err != nil {
				return nil, fmt.Errorf("reid: get matched track for batch item %d: %w", i, err)
			}

			avgEmb := exponentialMovingAverage(existingTrack.Embedding, embeddings[i], 0.3)

			if err := e.store.UpdateTrackMatch(ctx, UpdateTrackInput{
				TenantID:   tenantID,
				TrackID:    match.TrackID,
				Embedding:  avgEmb,
				LastCamera: det.CameraID,
				LastSeen:   timestamp,
			}); err != nil {
				return nil, fmt.Errorf("reid: update track for batch item %d: %w", i, err)
			}

			results[i] = MatchResult{
				TrackID:    match.TrackID,
				IsNew:      false,
				Confidence: match.Score,
				CameraID:   det.CameraID,
			}
		} else {
			track, err := e.store.CreateTrack(ctx, CreateTrackInput{
				TenantID:  tenantID,
				Embedding: embeddings[i],
				CameraID:  det.CameraID,
				SeenAt:    timestamp,
			})
			if err != nil {
				return nil, fmt.Errorf("reid: create track for batch item %d: %w", i, err)
			}

			results[i] = MatchResult{
				TrackID:    track.ID,
				IsNew:      true,
				Confidence: 1.0,
				CameraID:   det.CameraID,
			}
		}

		// Record sighting.
		if _, err := e.store.CreateSighting(ctx, CreateSightingInput{
			TenantID:   tenantID,
			TrackID:    results[i].TrackID,
			CameraID:   det.CameraID,
			Embedding:  embeddings[i],
			Confidence: results[i].Confidence,
			BBoxX:      det.BBoxX,
			BBoxY:      det.BBoxY,
			BBoxW:      det.BBoxW,
			BBoxH:      det.BBoxH,
			SeenAt:     timestamp,
		}); err != nil {
			return nil, fmt.Errorf("reid: create sighting for batch item %d: %w", i, err)
		}
	}

	return results, nil
}

// Close shuts down the engine and releases resources.
func (e *Engine) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return nil
	}
	e.closed = true
	return e.extractor.Close()
}

// exponentialMovingAverage blends the old and new embeddings:
//
//	result[i] = (1-alpha)*old[i] + alpha*new[i]
//
// The result is L2-normalized.
func exponentialMovingAverage(old, new []float32, alpha float64) []float32 {
	if len(old) != len(new) {
		// Dimension mismatch — use the new embedding directly.
		result := make([]float32, len(new))
		copy(result, new)
		NormalizeEmbedding(result)
		return result
	}

	result := make([]float32, len(old))
	for i := range old {
		result[i] = float32((1-alpha)*float64(old[i]) + alpha*float64(new[i]))
	}
	NormalizeEmbedding(result)
	return result
}
