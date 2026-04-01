package ai

import (
	"sort"
	"strings"
	"time"

	"github.com/bluenviron/mediamtx/internal/nvr/db"
)

// SearchResult represents a single result from a semantic search query.
type SearchResult struct {
	DetectionID   int64   `json:"detection_id"`
	EventID       int64   `json:"event_id"`
	CameraID      string  `json:"camera_id"`
	CameraName    string  `json:"camera_name"`
	Class         string  `json:"class"`
	Confidence    float64 `json:"confidence"`
	Similarity    float64 `json:"similarity"`
	FrameTime     string  `json:"frame_time"`
	ThumbnailPath string  `json:"thumbnail_path,omitempty"`
}

// Search performs a semantic search over stored detections using CLIP text
// embeddings or class-name matching.
//
// When a CLIP embedder is available and detections have stored visual embeddings,
// the search computes cosine similarity between the text query embedding and
// each detection's visual embedding. Since the CLIP visual encoder (768-dim) and
// text encoder (512-dim) produce different-sized vectors in these models, the
// search uses a hybrid approach: class-name matching is combined with embedding
// similarity when dimensions match, and class-name matching alone is used
// otherwise.
//
// Results are sorted by similarity score descending and limited to the top N.
func Search(embedder *Embedder, database *db.DB, query string, cameraID string, start, end time.Time, limit int) ([]SearchResult, error) {
	if limit <= 0 {
		limit = 20
	}

	queryLower := strings.ToLower(strings.TrimSpace(query))
	queryWords := strings.Fields(queryLower)

	var textEmb []float32
	if embedder != nil {
		var err error
		textEmb, err = embedder.EncodeText(query)
		if err != nil {
			textEmb = nil
		}
	}

	type scored struct {
		result SearchResult
		score  float64
	}
	var results []scored

	// Source 1: Live detections (not yet consolidated).
	dets, err := database.ListDetectionsWithEvents(cameraID, start, end)
	if err != nil {
		return nil, err
	}
	for _, det := range dets {
		score := classMatchScore(det.Class, queryWords)
		if textEmb != nil && len(det.Embedding) > 0 {
			visualEmb := bytesToFloat32Slice(det.Embedding)
			if visualEmb != nil {
				compareEmb := visualEmb
				if len(compareEmb) != len(textEmb) {
					compareEmb = embedder.ProjectVisual(visualEmb)
				}
				if compareEmb != nil && len(compareEmb) == len(textEmb) {
					sim := CosineSimilarity(textEmb, compareEmb)
					score = 0.3*score + 0.7*sim
				}
			}
		}
		if score > 0 {
			results = append(results, scored{
				result: SearchResult{
					DetectionID:   det.ID,
					EventID:       det.MotionEventID,
					CameraID:      det.CameraID,
					CameraName:    det.CameraName,
					Class:         det.Class,
					Confidence:    det.Confidence,
					Similarity:    score,
					FrameTime:     det.FrameTime,
					ThumbnailPath: det.ThumbnailPath,
				},
				score: score,
			})
		}
	}

	// Source 2: Consolidated events (already compacted).
	events, err := database.ListSearchableEvents(cameraID, start, end)
	if err != nil {
		return nil, err
	}
	for _, ev := range events {
		score := classMatchScore(ev.Class, queryWords)
		if textEmb != nil && len(ev.Embedding) > 0 {
			visualEmb := bytesToFloat32Slice(ev.Embedding)
			if visualEmb != nil {
				compareEmb := visualEmb
				if len(compareEmb) != len(textEmb) {
					compareEmb = embedder.ProjectVisual(visualEmb)
				}
				if compareEmb != nil && len(compareEmb) == len(textEmb) {
					sim := CosineSimilarity(textEmb, compareEmb)
					score = 0.3*score + 0.7*sim
				}
			}
		}
		if score > 0 {
			results = append(results, scored{
				result: SearchResult{
					EventID:       ev.EventID,
					CameraID:      ev.CameraID,
					CameraName:    ev.CameraName,
					Class:         ev.Class,
					Confidence:    ev.Confidence,
					Similarity:    score,
					FrameTime:     ev.StartedAt,
					ThumbnailPath: ev.ThumbnailPath,
				},
				score: score,
			})
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].score > results[j].score
	})

	if len(results) > limit {
		results = results[:limit]
	}

	out := make([]SearchResult, len(results))
	for i, r := range results {
		out[i] = r.result
	}
	return out, nil
}

// classMatchScore computes a simple relevance score by matching query words
// against the detection class name. Returns a value between 0 and 1.
func classMatchScore(className string, queryWords []string) float64 {
	if len(queryWords) == 0 {
		return 0
	}

	classLower := strings.ToLower(className)
	classWords := strings.Fields(classLower)

	matched := 0
	for _, qw := range queryWords {
		for _, cw := range classWords {
			if cw == qw || strings.Contains(cw, qw) || strings.Contains(qw, cw) {
				matched++
				break
			}
		}
	}

	if matched == 0 {
		return 0
	}

	// Score based on fraction of query words matched, boosted by confidence.
	return float64(matched) / float64(len(queryWords))
}
