// Package reid implements cross-camera person re-identification (KAI-481).
//
// The pipeline: person detections arrive with cropped image data, an embedding
// is extracted via Triton (OSNet or similar), then the matching engine finds
// the best existing track (or creates a new one) using cosine similarity
// combined with spatial/temporal heuristics from a camera adjacency graph.
//
// Package boundary: imports internal/cloud/db only. Never imports apiserver
// or other cloud packages that would create cycles.
//
// Multi-tenant invariant: every exported method requires a tenant_id parameter.
// Cross-tenant track access is impossible by construction.
package reid

import (
	"encoding/binary"
	"errors"
	"math"
	"time"
)

// EmbeddingDim is the default dimensionality for OSNet re-id embeddings.
const EmbeddingDim = 512

// Track represents a globally-identified person tracked across cameras.
type Track struct {
	ID           string    `json:"id"`
	TenantID     string    `json:"tenant_id"`
	Embedding    []float32 `json:"embedding"`
	EmbeddingDim int       `json:"embedding_dim"`
	FirstCamera  string    `json:"first_camera"`
	LastCamera   string    `json:"last_camera"`
	FirstSeen    time.Time `json:"first_seen"`
	LastSeen     time.Time `json:"last_seen"`
	MatchCount   int       `json:"match_count"`
	Metadata     string    `json:"metadata"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// Sighting records a single detection matched to a track.
type Sighting struct {
	ID         string    `json:"id"`
	TenantID   string    `json:"tenant_id"`
	TrackID    string    `json:"track_id"`
	CameraID   string    `json:"camera_id"`
	Embedding  []float32 `json:"embedding"`
	Confidence float64   `json:"confidence"`
	BBoxX      float64   `json:"bbox_x"`
	BBoxY      float64   `json:"bbox_y"`
	BBoxW      float64   `json:"bbox_w"`
	BBoxH      float64   `json:"bbox_h"`
	SeenAt     time.Time `json:"seen_at"`
	CreatedAt  time.Time `json:"created_at"`
}

// Detection is the input to the re-id engine: a cropped person image from a
// specific camera at a specific time.
type Detection struct {
	CameraID  string    // source camera identifier
	ImageData []byte    // cropped person image (RGB, HWC)
	Width     int       // image width in pixels
	Height    int       // image height in pixels
	BBoxX     float64   // bounding box in original frame
	BBoxY     float64
	BBoxW     float64
	BBoxH     float64
	Timestamp time.Time // when the detection occurred
}

// MatchResult is returned by the matching engine when a detection is
// successfully matched (or a new track is created).
type MatchResult struct {
	TrackID    string  `json:"track_id"`
	IsNew      bool    `json:"is_new"`
	Confidence float64 `json:"confidence"`
	CameraID   string  `json:"camera_id"`
}

// MatchConfig holds tunable parameters for the matching engine.
type MatchConfig struct {
	// SimilarityThreshold is the minimum cosine similarity to consider a match.
	// Typical values: 0.6-0.8. Lower = more permissive matching.
	SimilarityThreshold float64

	// TimeWindow is the maximum duration between sightings for a match to be
	// considered valid. Beyond this window the person is treated as new.
	TimeWindow time.Duration

	// AdjacencyBoost is the bonus applied to similarity when the candidate
	// track's last camera is adjacent to the detection's camera.
	AdjacencyBoost float64

	// MaxCandidates limits the number of active tracks considered per match.
	MaxCandidates int
}

// DefaultMatchConfig returns production-ready defaults.
func DefaultMatchConfig() MatchConfig {
	return MatchConfig{
		SimilarityThreshold: 0.65,
		TimeWindow:          30 * time.Minute,
		AdjacencyBoost:      0.10,
		MaxCandidates:       200,
	}
}

// Sentinel errors.
var (
	ErrNotFound        = errors.New("reid: not found")
	ErrInvalidTenantID = errors.New("reid: tenant_id is required")
	ErrInvalidID       = errors.New("reid: id is required")
	ErrNoEmbedding     = errors.New("reid: embedding extraction failed")
	ErrTritonUnavail   = errors.New("reid: triton server unavailable")
)

// -----------------------------------------------------------------------
// Embedding helpers
// -----------------------------------------------------------------------

// EmbeddingToBytes converts a float32 embedding slice to little-endian bytes.
func EmbeddingToBytes(emb []float32) []byte {
	buf := make([]byte, len(emb)*4)
	for i, v := range emb {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(v))
	}
	return buf
}

// BytesToEmbedding converts little-endian bytes back to a float32 slice.
func BytesToEmbedding(data []byte) []float32 {
	n := len(data) / 4
	emb := make([]float32, n)
	for i := range emb {
		emb[i] = math.Float32frombits(binary.LittleEndian.Uint32(data[i*4:]))
	}
	return emb
}

// CosineSimilarity computes the cosine similarity between two vectors.
// Returns 0 if either vector is zero-length or all zeros.
func CosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		fa, fb := float64(a[i]), float64(b[i])
		dot += fa * fb
		normA += fa * fa
		normB += fb * fb
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}

// NormalizeEmbedding L2-normalizes an embedding vector in-place.
func NormalizeEmbedding(emb []float32) {
	var norm float64
	for _, v := range emb {
		norm += float64(v) * float64(v)
	}
	if norm == 0 {
		return
	}
	norm = math.Sqrt(norm)
	for i := range emb {
		emb[i] = float32(float64(emb[i]) / norm)
	}
}
