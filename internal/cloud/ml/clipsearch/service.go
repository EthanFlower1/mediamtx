package clipsearch

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

// Service orchestrates the search pipeline: text encoding, vector
// similarity lookup, and result ranking with deduplication.
type Service struct {
	encoder TextEncoder
	store   VectorStore
	logger  *slog.Logger
}

// ServiceOption configures the Service at construction time.
type ServiceOption func(*Service)

// WithLogger sets the structured logger for the service.
func WithLogger(l *slog.Logger) ServiceOption {
	return func(s *Service) {
		s.logger = l
	}
}

// NewService constructs a search service with the given encoder and store.
func NewService(encoder TextEncoder, store VectorStore, opts ...ServiceOption) *Service {
	s := &Service{
		encoder: encoder,
		store:   store,
		logger:  slog.Default(),
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Search executes the full search pipeline:
//  1. Validate and normalize the request.
//  2. Encode the query text into a CLIP embedding via Triton.
//  3. Run a vector similarity search against pgvector.
//  4. Rank and deduplicate results.
//  5. Return the response.
func (s *Service) Search(ctx context.Context, req SearchRequest) (*SearchResponse, error) {
	start := time.Now()

	// Step 1: validate.
	if err := req.Validate(); err != nil {
		return nil, err
	}

	s.logger.DebugContext(ctx, "clip search starting",
		slog.String("tenant_id", req.TenantID),
		slog.String("query", req.Query),
		slog.Int("limit", req.Limit),
	)

	// Step 2: encode query text.
	embedding, err := s.encoder.Encode(ctx, req.Query)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrEncoderFailed, err)
	}

	encodeLatency := time.Since(start)
	s.logger.DebugContext(ctx, "text encoding complete",
		slog.Duration("latency", encodeLatency),
		slog.Int("embed_dim", len(embedding)),
	)

	// Step 3: vector similarity search.
	params := SimilarityParams{
		TenantID:       req.TenantID,
		QueryEmbedding: embedding,
		CameraIDs:      req.CameraIDs,
		Start:          req.Start,
		End:            req.End,
		Limit:          req.Limit,
		MinSimilarity:  req.MinSimilarity,
	}

	matches, err := s.store.SimilaritySearch(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrStoreFailed, err)
	}

	searchLatency := time.Since(start)
	s.logger.DebugContext(ctx, "vector search complete",
		slog.Duration("latency", searchLatency),
		slog.Int("raw_matches", len(matches)),
	)

	// Step 4: rank and deduplicate.
	dedupWindow := time.Duration(req.DedupWindowSec) * time.Second
	results := RankAndDedup(matches, dedupWindow, req.Limit)

	totalLatency := time.Since(start)
	s.logger.DebugContext(ctx, "search complete",
		slog.Duration("total_latency", totalLatency),
		slog.Int("result_count", len(results)),
	)

	// Step 5: assemble response.
	return &SearchResponse{
		Query:        req.Query,
		TenantID:     req.TenantID,
		Count:        len(results),
		Results:      results,
		LatencyMs:    totalLatency.Milliseconds(),
		ModelVersion: s.encoder.ModelVersion(),
	}, nil
}
