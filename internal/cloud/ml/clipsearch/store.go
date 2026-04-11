package clipsearch

import (
	"context"
	"fmt"
	"strings"
	"time"

	clouddb "github.com/bluenviron/mediamtx/internal/cloud/db"
)

// PgVectorStore implements VectorStore using pgvector cosine-distance
// queries against the clip_embeddings table. All queries include tenant_id
// in the WHERE clause, which causes Postgres to prune to the tenant's
// LIST partition and use its dedicated HNSW index (Seam #4).
//
// For SQLite test mode the store falls back to a brute-force scan (no
// vector ops); this is acceptable because test corpora are tiny.
type PgVectorStore struct {
	db *clouddb.DB
}

// NewPgVectorStore constructs a VectorStore backed by the cloud DB.
func NewPgVectorStore(db *clouddb.DB) *PgVectorStore {
	return &PgVectorStore{db: db}
}

// SimilaritySearch finds the top-K clip_embeddings closest to the query
// vector, scoped to a single tenant with optional camera and time filters.
//
// Postgres path: uses pgvector's `<=>` cosine-distance operator, which
// benefits from the per-partition HNSW index created by
// db.ProvisionVectorPartitions. The operator returns distance (0 = identical),
// so we convert to similarity as (1 - distance).
//
// SQLite path: returns an empty result set. Vector similarity requires a
// real Postgres with the vector extension. Application-level CRUD tests
// exercise the schema stubs; vector-similarity tests must run against
// Postgres in CI.
func (s *PgVectorStore) SimilaritySearch(ctx context.Context, params SimilarityParams) ([]VectorMatch, error) {
	if params.TenantID == "" {
		return nil, ErrMissingTenantID
	}

	if s.db.Dialect() != clouddb.DialectPostgres {
		// SQLite stub: no vector operations available.
		return []VectorMatch{}, nil
	}

	// Build the pgvector query with tenant scoping.
	vecLiteral := float32SliceToVectorLiteral(params.QueryEmbedding)

	var conditions []string
	var args []any
	argIdx := 1

	// tenant_id is always required (drives partition pruning).
	conditions = append(conditions, fmt.Sprintf("tenant_id = $%d", argIdx))
	args = append(args, params.TenantID)
	argIdx++

	// Optional camera filter.
	if len(params.CameraIDs) > 0 {
		placeholders := make([]string, len(params.CameraIDs))
		for i, cid := range params.CameraIDs {
			placeholders[i] = fmt.Sprintf("$%d", argIdx)
			args = append(args, cid)
			argIdx++
		}
		conditions = append(conditions, fmt.Sprintf("camera_id IN (%s)", strings.Join(placeholders, ",")))
	}

	// Optional time range.
	if !params.Start.IsZero() {
		conditions = append(conditions, fmt.Sprintf("captured_at >= $%d", argIdx))
		args = append(args, params.Start)
		argIdx++
	}
	if !params.End.IsZero() {
		conditions = append(conditions, fmt.Sprintf("captured_at <= $%d", argIdx))
		args = append(args, params.End)
		argIdx++
	}

	where := strings.Join(conditions, " AND ")

	limit := params.Limit
	if limit <= 0 {
		limit = DefaultLimit
	}
	// Fetch more than needed so the ranker has room after dedup.
	fetchLimit := limit * 3
	if fetchLimit > 500 {
		fetchLimit = 500
	}

	// The <=> operator is pgvector cosine distance. We order by ascending
	// distance (closest first) and convert to similarity in the SELECT.
	query := fmt.Sprintf(
		`SELECT embedding_id, tenant_id, camera_id, segment_id, captured_at,
		        1 - (embedding <=> '%s'::vector) AS similarity
		 FROM clip_embeddings
		 WHERE %s
		 ORDER BY embedding <=> '%s'::vector ASC
		 LIMIT %d`,
		vecLiteral, where, vecLiteral, fetchLimit,
	)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrStoreFailed, err)
	}
	defer rows.Close()

	var results []VectorMatch
	for rows.Next() {
		var m VectorMatch
		if err := rows.Scan(&m.EmbeddingID, &m.TenantID, &m.CameraID, &m.SegmentID, &m.CapturedAt, &m.Similarity); err != nil {
			return nil, fmt.Errorf("clipsearch: scan result: %w", err)
		}
		// Apply minimum similarity filter.
		if params.MinSimilarity > 0 && m.Similarity < params.MinSimilarity {
			continue
		}
		results = append(results, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("clipsearch: iterate results: %w", err)
	}

	return results, nil
}

// InsertEmbedding stores a CLIP embedding for a single frame. This is
// called by the indexing pipeline (not the search path). Tenant scoping
// is enforced by the PRIMARY KEY (embedding_id, tenant_id).
func (s *PgVectorStore) InsertEmbedding(ctx context.Context, tenantID, embeddingID, cameraID, segmentID, modelVersionID string, embedding []float32, capturedAt time.Time) error {
	if tenantID == "" {
		return ErrMissingTenantID
	}

	vecLiteral := float32SliceToVectorLiteral(embedding)

	if s.db.Dialect() == clouddb.DialectPostgres {
		_, err := s.db.ExecContext(ctx,
			`INSERT INTO clip_embeddings
			    (embedding_id, tenant_id, camera_id, segment_id, model_version_id, embedding, captured_at)
			 VALUES ($1, $2, $3, $4, $5, $6::vector, $7)
			 ON CONFLICT (embedding_id, tenant_id) DO NOTHING`,
			embeddingID, tenantID, cameraID, segmentID, modelVersionID, vecLiteral, capturedAt,
		)
		return err
	}

	// SQLite stub: store the vector literal as TEXT.
	_, err := s.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO clip_embeddings
		    (embedding_id, tenant_id, camera_id, segment_id, model_version_id, embedding, captured_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		embeddingID, tenantID, cameraID, segmentID, modelVersionID, vecLiteral, capturedAt,
	)
	return err
}

// float32SliceToVectorLiteral converts a float32 slice to the pgvector
// text literal format: "[0.1,0.2,0.3,...]".
func float32SliceToVectorLiteral(v []float32) string {
	if len(v) == 0 {
		return "[]"
	}
	var b strings.Builder
	b.WriteByte('[')
	for i, f := range v {
		if i > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(&b, "%g", f)
	}
	b.WriteByte(']')
	return b.String()
}
