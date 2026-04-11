package reid

import (
	"math"
	"sort"
	"time"
)

// Matcher scores candidate tracks against a new detection embedding using
// cosine similarity, spatial adjacency bonuses, and temporal plausibility.
type Matcher struct {
	cfg   MatchConfig
	graph *AdjacencyGraph
}

// NewMatcher creates a Matcher with the given config and adjacency graph.
// If graph is nil, an empty graph is used (no adjacency bonuses).
func NewMatcher(cfg MatchConfig, graph *AdjacencyGraph) *Matcher {
	if graph == nil {
		graph = NewAdjacencyGraph()
	}
	return &Matcher{cfg: cfg, graph: graph}
}

// candidate is an internal scored track for ranking.
type candidate struct {
	track      Track
	rawSim     float64 // raw cosine similarity
	adjBoost   float64 // adjacency bonus
	timePenalty float64 // time decay penalty
	finalScore float64 // combined score
}

// Match finds the best matching track for the given embedding, or returns nil
// if no track exceeds the similarity threshold. The candidates slice is the
// set of active tracks to consider (already filtered by tenant and time window
// by the caller).
func (m *Matcher) Match(
	embedding []float32,
	cameraID string,
	timestamp time.Time,
	candidates []Track,
) *MatchCandidate {
	if len(candidates) == 0 {
		return nil
	}

	// Limit candidates to avoid excessive computation.
	limit := m.cfg.MaxCandidates
	if limit <= 0 {
		limit = 200
	}
	if len(candidates) > limit {
		// Sort by last_seen descending to prefer recent tracks.
		sort.Slice(candidates, func(i, j int) bool {
			return candidates[i].LastSeen.After(candidates[j].LastSeen)
		})
		candidates = candidates[:limit]
	}

	var scored []candidate
	for _, track := range candidates {
		if len(track.Embedding) == 0 {
			continue
		}

		rawSim := CosineSimilarity(embedding, track.Embedding)
		if rawSim < m.cfg.SimilarityThreshold*0.8 {
			// Early reject: even with max boost this cannot pass.
			continue
		}

		// Adjacency bonus.
		adjBoost := 0.0
		if m.graph.Adjacent(track.LastCamera, cameraID) && track.LastCamera != cameraID {
			// Only boost if adjacency is plausible given the time elapsed.
			elapsed := timestamp.Sub(track.LastSeen)
			if m.graph.TransitPlausible(track.LastCamera, cameraID, elapsed) {
				adjBoost = m.cfg.AdjacencyBoost
			}
		}

		// Time decay: reduce score for older tracks. Half-life = TimeWindow/2.
		elapsed := timestamp.Sub(track.LastSeen)
		halfLife := float64(m.cfg.TimeWindow) / 2.0
		timePenalty := 0.0
		if elapsed > 0 && halfLife > 0 {
			timePenalty = 0.05 * (1.0 - math.Exp(-float64(elapsed)/halfLife))
		}

		finalScore := rawSim + adjBoost - timePenalty

		scored = append(scored, candidate{
			track:       track,
			rawSim:      rawSim,
			adjBoost:    adjBoost,
			timePenalty: timePenalty,
			finalScore:  finalScore,
		})
	}

	if len(scored) == 0 {
		return nil
	}

	// Sort by final score descending.
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].finalScore > scored[j].finalScore
	})

	best := scored[0]
	if best.finalScore < m.cfg.SimilarityThreshold {
		return nil
	}

	return &MatchCandidate{
		TrackID:    best.track.ID,
		Score:      best.finalScore,
		RawSim:     best.rawSim,
		AdjBoost:   best.adjBoost,
		TimePenalty: best.timePenalty,
	}
}

// MatchCandidate is the result of a successful match.
type MatchCandidate struct {
	TrackID     string
	Score       float64
	RawSim      float64
	AdjBoost    float64
	TimePenalty float64
}
