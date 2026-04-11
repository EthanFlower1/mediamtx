// Package clipsearch implements the cloud CLIP text-to-image search service
// for the Kaivue multi-tenant NVR platform (KAI-480).
//
// Architecture:
//
//   - Text encoding: natural-language queries are encoded into 768-dim CLIP
//     embeddings via a Triton Inference Server sidecar. The TextEncoder
//     interface abstracts the RPC so tests can use a deterministic fake.
//
//   - Vector similarity: queries run against pgvector HNSW indexes on the
//     clip_embeddings table, which is LIST-partitioned by tenant_id (KAI-292).
//     Every query includes tenant_id in the WHERE clause, hitting only that
//     tenant's partition and HNSW index. Cross-tenant vector leakage is
//     structurally impossible (Seam #4).
//
//   - Result ranking: raw cosine-similarity scores from pgvector are combined
//     with temporal-proximity deduplication. When multiple frames from the
//     same camera fall within a configurable time window, only the
//     highest-scoring frame is kept. This reduces redundant results from
//     continuous recording.
//
//   - Query API: a Gin handler serves GET /api/cloud/search consumed by
//     both the React admin console and the Flutter primary client.
//
// Migration path (v1.x): the current pgvector backend is designed to be
// replaceable. The VectorStore interface can be re-implemented against
// Qdrant or Weaviate when the embedding corpus outgrows a single Postgres
// instance. The HNSW parameters (m=32, ef_construction=64) are tuned for
// corpora up to ~10M vectors per tenant.
package clipsearch
