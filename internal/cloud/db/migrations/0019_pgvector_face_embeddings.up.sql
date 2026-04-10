-- KAI-292: pgvector setup + per-tenant face and CLIP embedding tables.
--
-- Design notes:
--   - vector extension must be pre-loaded via shared_preload_libraries in the
--     DB parameter group (KAI-216 PR #187 handles this). This migration creates
--     the extension and the parent tables.
--   - Per-tenant physical isolation: both face_embeddings and clip_embeddings
--     are LIST-partitioned by tenant_id. Each tenant gets its own partition
--     (created at tenant-provisioning time, not in this migration) with its own
--     HNSW index. This enforces Seam #4 at the storage/index level — tenant A's
--     index physically cannot contain tenant B's vectors, even under a buggy
--     WHERE clause.
--   - consent_records table (per lead-security MUST-CHANGE #2 on KAI-282 memo):
--     stable FK target for face enrollment. audit_event_id is a back-pointer
--     to KAI-233 audit log, NOT a foreign key (audit partitions rotate).
--   - model_versions table: tracks which embedding model version produced each
--     vector. Required for §5.1 model version transitions (Annex IV).
--   - Embedding dimensions: 512 for face (ArcFace family), 768 for CLIP visual.
--   - HNSW indexes are created per-partition, not on the parent table. The
--     parent table carries no index; Postgres routes queries to partition indexes
--     automatically when the partition key is in the WHERE clause.
--
-- SQLite tests: the entire pgvector section is postgres-only. SQLite tests
-- exercise CRUD via a simplified stub (see 0015 down migration comments).

-- postgres-only:begin
CREATE EXTENSION IF NOT EXISTS vector;

-- consent_records: stable FK target for face enrollment (Seam #4: partitioned
-- by tenant_id for physical isolation of biometric consent data).
CREATE TABLE IF NOT EXISTS consent_records (
    consent_id          TEXT        NOT NULL,
    tenant_id           TEXT        NOT NULL,
    subject_identifier  TEXT        NOT NULL,
    consent_given_at    TIMESTAMPTZ NOT NULL,
    audit_event_id      TEXT        NOT NULL DEFAULT '',
    revoked_at          TIMESTAMPTZ,
    revocation_reason   TEXT        NOT NULL DEFAULT '',
    policy_version      TEXT        NOT NULL DEFAULT '',
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (consent_id, tenant_id)
) PARTITION BY LIST (tenant_id);

CREATE INDEX IF NOT EXISTS idx_consent_records_subject
    ON consent_records (tenant_id, subject_identifier);

-- model_versions: tracks embedding model versions for §5.1 transitions.
-- Not partitioned (small table, cross-tenant reads by platform admin).
CREATE TABLE IF NOT EXISTS model_versions (
    model_version_id    TEXT        NOT NULL PRIMARY KEY,
    model_family        TEXT        NOT NULL DEFAULT '',
    version_label       TEXT        NOT NULL DEFAULT '',
    embedding_dim       INTEGER     NOT NULL,
    status              TEXT        NOT NULL DEFAULT 'active',
    registered_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deprecated_at       TIMESTAMPTZ,
    registry_ref        TEXT        NOT NULL DEFAULT ''
);

-- face_embeddings: per-tenant face vault. LIST-partitioned by tenant_id.
-- Each partition gets its own HNSW index (created at tenant-provisioning time).
-- Embedding is vector(512) for ArcFace family; dimension is fixed per model
-- family and validated at insert time by the application layer.
CREATE TABLE IF NOT EXISTS face_embeddings (
    embedding_id        TEXT        NOT NULL,
    tenant_id           TEXT        NOT NULL,
    consent_id          TEXT        NOT NULL,
    model_version_id    TEXT        NOT NULL,
    subject_label       TEXT        NOT NULL DEFAULT '',
    embedding           vector(512) NOT NULL,
    source_crop_ref     TEXT        NOT NULL DEFAULT '',
    enrolled_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    enrolled_by         TEXT        NOT NULL DEFAULT '',
    is_active           INTEGER     NOT NULL DEFAULT 1,
    deactivated_at      TIMESTAMPTZ,
    PRIMARY KEY (embedding_id, tenant_id),
    FOREIGN KEY (consent_id, tenant_id) REFERENCES consent_records (consent_id, tenant_id),
    FOREIGN KEY (model_version_id) REFERENCES model_versions (model_version_id)
) PARTITION BY LIST (tenant_id);

CREATE INDEX IF NOT EXISTS idx_face_embeddings_subject
    ON face_embeddings (tenant_id, subject_label);

CREATE INDEX IF NOT EXISTS idx_face_embeddings_model
    ON face_embeddings (tenant_id, model_version_id);

CREATE INDEX IF NOT EXISTS idx_face_embeddings_consent
    ON face_embeddings (tenant_id, consent_id);

-- clip_embeddings: per-tenant CLIP visual embeddings for semantic search.
-- LIST-partitioned by tenant_id. 768-dim for CLIP ViT-L/14.
-- No consent_id required — CLIP embeddings are scene-level, not biometric.
CREATE TABLE IF NOT EXISTS clip_embeddings (
    embedding_id        TEXT        NOT NULL,
    tenant_id           TEXT        NOT NULL,
    camera_id           TEXT        NOT NULL DEFAULT '',
    segment_id          TEXT        NOT NULL DEFAULT '',
    model_version_id    TEXT        NOT NULL,
    embedding           vector(768) NOT NULL,
    captured_at         TIMESTAMPTZ NOT NULL,
    indexed_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (embedding_id, tenant_id),
    FOREIGN KEY (model_version_id) REFERENCES model_versions (model_version_id)
) PARTITION BY LIST (tenant_id);

CREATE INDEX IF NOT EXISTS idx_clip_embeddings_camera_time
    ON clip_embeddings (tenant_id, camera_id, captured_at DESC);

CREATE INDEX IF NOT EXISTS idx_clip_embeddings_model
    ON clip_embeddings (tenant_id, model_version_id);
-- postgres-only:end

-- SQLite-compatible stubs for unit tests. These provide the same column layout
-- but without vector types, partitioning, or HNSW indexes. Application-level
-- CRUD tests run against these; vector-similarity tests require a real Postgres.

-- consent_records stub (SQLite)
CREATE TABLE IF NOT EXISTS consent_records (
    consent_id          TEXT        NOT NULL,
    tenant_id           TEXT        NOT NULL,
    subject_identifier  TEXT        NOT NULL,
    consent_given_at    DATETIME    NOT NULL,
    audit_event_id      TEXT        NOT NULL DEFAULT '',
    revoked_at          DATETIME,
    revocation_reason   TEXT        NOT NULL DEFAULT '',
    policy_version      TEXT        NOT NULL DEFAULT '',
    created_at          DATETIME    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (consent_id, tenant_id)
);

-- model_versions stub (SQLite)
CREATE TABLE IF NOT EXISTS model_versions (
    model_version_id    TEXT        NOT NULL PRIMARY KEY,
    model_family        TEXT        NOT NULL DEFAULT '',
    version_label       TEXT        NOT NULL DEFAULT '',
    embedding_dim       INTEGER     NOT NULL,
    status              TEXT        NOT NULL DEFAULT 'active',
    registered_at       DATETIME    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deprecated_at       DATETIME,
    registry_ref        TEXT        NOT NULL DEFAULT ''
);

-- face_embeddings stub (SQLite — embedding stored as TEXT blob, no vector ops)
CREATE TABLE IF NOT EXISTS face_embeddings (
    embedding_id        TEXT        NOT NULL,
    tenant_id           TEXT        NOT NULL,
    consent_id          TEXT        NOT NULL,
    model_version_id    TEXT        NOT NULL,
    subject_label       TEXT        NOT NULL DEFAULT '',
    embedding           TEXT        NOT NULL DEFAULT '',
    source_crop_ref     TEXT        NOT NULL DEFAULT '',
    enrolled_at         DATETIME    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    enrolled_by         TEXT        NOT NULL DEFAULT '',
    is_active           INTEGER     NOT NULL DEFAULT 1,
    deactivated_at      DATETIME,
    PRIMARY KEY (embedding_id, tenant_id)
);

-- clip_embeddings stub (SQLite — embedding stored as TEXT blob, no vector ops)
CREATE TABLE IF NOT EXISTS clip_embeddings (
    embedding_id        TEXT        NOT NULL,
    tenant_id           TEXT        NOT NULL,
    camera_id           TEXT        NOT NULL DEFAULT '',
    segment_id          TEXT        NOT NULL DEFAULT '',
    model_version_id    TEXT        NOT NULL,
    embedding           TEXT        NOT NULL DEFAULT '',
    captured_at         DATETIME    NOT NULL,
    indexed_at          DATETIME    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (embedding_id, tenant_id)
);
