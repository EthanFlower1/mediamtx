package clipsearch

import (
	"context"
	"errors"
	"time"
)

// EmbeddingDim is the CLIP ViT-L/14 embedding dimension used in the
// clip_embeddings pgvector column (KAI-292 migration 0019).
const EmbeddingDim = 768

// DefaultLimit is the maximum number of results returned when no limit is
// specified in the query.
const DefaultLimit = 20

// MaxLimit caps the result set to prevent expensive unbounded scans.
const MaxLimit = 100

// DefaultDedupWindowSec is the default temporal-proximity deduplication
// window in seconds. Frames from the same camera within this window are
// collapsed to the single highest-scoring frame.
const DefaultDedupWindowSec = 30

// SearchRequest is the caller-facing query specification.
type SearchRequest struct {
	// TenantID is required. Every query is scoped to exactly one tenant.
	TenantID string

	// Query is the natural-language search text (e.g. "red car near gate").
	Query string

	// CameraIDs optionally restricts the search to specific cameras.
	// An empty slice searches all cameras within the tenant.
	CameraIDs []string

	// Start and End bound the time range. Zero values mean "unbounded".
	Start time.Time
	End   time.Time

	// Limit caps the result count. Clamped to [1, MaxLimit].
	Limit int

	// DedupWindowSec overrides the temporal deduplication window.
	// Zero means use DefaultDedupWindowSec.
	DedupWindowSec int

	// MinSimilarity is the minimum cosine similarity threshold.
	// Results below this score are discarded. Zero means no threshold.
	MinSimilarity float64
}

// Validate checks that required fields are present and clamps limits.
func (r *SearchRequest) Validate() error {
	if r.TenantID == "" {
		return ErrMissingTenantID
	}
	if r.Query == "" {
		return ErrEmptyQuery
	}
	if r.Limit <= 0 {
		r.Limit = DefaultLimit
	}
	if r.Limit > MaxLimit {
		r.Limit = MaxLimit
	}
	if r.DedupWindowSec <= 0 {
		r.DedupWindowSec = DefaultDedupWindowSec
	}
	return nil
}

// SearchResult is a single ranked result returned to the caller.
type SearchResult struct {
	EmbeddingID string    `json:"embedding_id"`
	CameraID    string    `json:"camera_id"`
	SegmentID   string    `json:"segment_id"`
	CapturedAt  time.Time `json:"captured_at"`
	Similarity  float64   `json:"similarity"`
}

// SearchResponse wraps the result set with query metadata.
type SearchResponse struct {
	Query        string         `json:"query"`
	TenantID     string         `json:"tenant_id"`
	Count        int            `json:"count"`
	Results      []SearchResult `json:"results"`
	LatencyMs    int64          `json:"latency_ms"`
	ModelVersion string         `json:"model_version,omitempty"`
}

// TextEncoder converts a natural-language query into a CLIP embedding vector.
// The production implementation calls Triton Inference Server; tests use a
// deterministic fake.
type TextEncoder interface {
	// Encode returns a 768-dim L2-normalized embedding for the given text.
	Encode(ctx context.Context, text string) ([]float32, error)

	// ModelVersion returns the identifier of the loaded CLIP text model.
	ModelVersion() string
}

// VectorMatch is a raw similarity result from the vector store before
// ranking and deduplication.
type VectorMatch struct {
	EmbeddingID string
	TenantID    string
	CameraID    string
	SegmentID   string
	CapturedAt  time.Time
	Similarity  float64
}

// VectorStore abstracts the pgvector similarity query layer. The interface
// is the migration seam for a future Qdrant/Weaviate backend.
type VectorStore interface {
	// SimilaritySearch finds the top-K vectors closest to the query embedding,
	// scoped to a single tenant and optional camera/time filters.
	SimilaritySearch(ctx context.Context, params SimilarityParams) ([]VectorMatch, error)
}

// SimilarityParams bundles the inputs for a vector similarity query.
type SimilarityParams struct {
	TenantID      string
	QueryEmbedding []float32
	CameraIDs     []string
	Start         time.Time
	End           time.Time
	Limit         int
	MinSimilarity float64
}

// Sentinel errors.
var (
	ErrMissingTenantID = errors.New("clipsearch: tenant_id is required")
	ErrEmptyQuery      = errors.New("clipsearch: query text is required")
	ErrEncoderFailed   = errors.New("clipsearch: text encoder failed")
	ErrStoreFailed     = errors.New("clipsearch: vector store query failed")
)
