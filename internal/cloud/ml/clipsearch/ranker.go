package clipsearch

import (
	"sort"
	"time"
)

// RankAndDedup takes raw vector matches, removes temporal duplicates from
// the same camera, and returns the top-N results sorted by similarity
// descending.
//
// Temporal deduplication: when multiple frames from the same camera fall
// within dedupWindow of each other, only the highest-scoring frame in that
// cluster is kept. This prevents the result set from being dominated by
// a single static scene.
//
// The algorithm is a greedy sweep: matches are sorted by (camera_id,
// captured_at). For each camera, adjacent frames within the dedup window
// are grouped; the group representative is the frame with the highest
// similarity score.
func RankAndDedup(matches []VectorMatch, dedupWindow time.Duration, limit int) []SearchResult {
	if len(matches) == 0 {
		return nil
	}

	// Phase 1: deduplicate per-camera temporal clusters.
	deduped := temporalDedup(matches, dedupWindow)

	// Phase 2: sort by similarity descending.
	sort.Slice(deduped, func(i, j int) bool {
		return deduped[i].Similarity > deduped[j].Similarity
	})

	// Phase 3: truncate to limit.
	if limit > 0 && len(deduped) > limit {
		deduped = deduped[:limit]
	}

	// Phase 4: convert to SearchResult.
	results := make([]SearchResult, len(deduped))
	for i, m := range deduped {
		results[i] = SearchResult{
			EmbeddingID: m.EmbeddingID,
			CameraID:    m.CameraID,
			SegmentID:   m.SegmentID,
			CapturedAt:  m.CapturedAt,
			Similarity:  m.Similarity,
		}
	}
	return results
}

// temporalDedup groups matches by camera and collapses temporally adjacent
// frames into the single best match per cluster.
func temporalDedup(matches []VectorMatch, window time.Duration) []VectorMatch {
	if window <= 0 {
		// No dedup: return all.
		return matches
	}

	// Group by camera_id.
	byCamera := map[string][]VectorMatch{}
	for _, m := range matches {
		byCamera[m.CameraID] = append(byCamera[m.CameraID], m)
	}

	var result []VectorMatch
	for _, cameraMatches := range byCamera {
		// Sort by captured_at ascending within each camera.
		sort.Slice(cameraMatches, func(i, j int) bool {
			return cameraMatches[i].CapturedAt.Before(cameraMatches[j].CapturedAt)
		})

		// Sweep through and pick the best match in each window.
		best := cameraMatches[0]
		clusterStart := cameraMatches[0].CapturedAt

		for i := 1; i < len(cameraMatches); i++ {
			m := cameraMatches[i]
			if m.CapturedAt.Sub(clusterStart) <= window {
				// Same cluster: keep the higher-scoring match.
				if m.Similarity > best.Similarity {
					best = m
				}
			} else {
				// New cluster: emit the previous best and start fresh.
				result = append(result, best)
				best = m
				clusterStart = m.CapturedAt
			}
		}
		// Emit the last cluster's best.
		result = append(result, best)
	}

	return result
}
