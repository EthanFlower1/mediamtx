-- KAI-249: camera_segment_index — per-camera video segment catalogue.
-- Partitioned monthly by start_ts via pg_partman so old partitions can be
-- detached / archived without locking the live table.
--
-- SQLite tests skip the partitioning block; CRUD tests still exercise the
-- column layout through the non-partitioned table created below.
--
-- storage_tier mirrors the retention policy tiers: hot (on-recorder SSD),
-- warm (on-recorder HDD / S3 Intelligent-Tiering), cold (S3 Standard-IA),
-- archive (S3 Glacier / R2 Archive).

-- postgres-only:begin
CREATE TABLE IF NOT EXISTS camera_segment_index (
    camera_id           TEXT NOT NULL REFERENCES cameras(id) ON DELETE CASCADE,
    recorder_id         TEXT NOT NULL,
    tenant_id           TEXT NOT NULL,
    start_ts            TIMESTAMPTZ NOT NULL,
    end_ts              TIMESTAMPTZ NOT NULL,
    file_path           TEXT NOT NULL,
    file_size_bytes     BIGINT NOT NULL DEFAULT 0,
    storage_tier        TEXT NOT NULL DEFAULT 'hot'
        CHECK (storage_tier IN ('hot', 'warm', 'cold', 'archive')),
    checksum_sha256     TEXT,
    region              TEXT NOT NULL DEFAULT 'us-east-2',
    PRIMARY KEY (camera_id, start_ts)
) PARTITION BY RANGE (start_ts);

-- pg_partman: create monthly partitions automatically, pre-creating 3 months
-- ahead. The background job (KAI-234/River) runs run_maintenance_proc() weekly.
SELECT partman.create_parent(
    p_parent_table := 'public.camera_segment_index',
    p_control      := 'start_ts',
    p_type         := 'native',
    p_interval     := 'monthly',
    p_premake      := 3
);

-- Indexes on the parent table propagate to child partitions automatically.
CREATE INDEX IF NOT EXISTS idx_seg_camera_time
    ON camera_segment_index(camera_id, start_ts DESC, end_ts DESC);

CREATE INDEX IF NOT EXISTS idx_seg_tenant_time
    ON camera_segment_index(tenant_id, start_ts DESC);

CREATE INDEX IF NOT EXISTS idx_seg_recorder_time
    ON camera_segment_index(recorder_id, start_ts DESC);

CREATE INDEX IF NOT EXISTS idx_seg_tier
    ON camera_segment_index(storage_tier, start_ts DESC);
-- postgres-only:end

-- SQLite-compatible fallback (no partitioning, same column layout).
-- The postgres-only block is stripped by translateToSQLite; this CREATE TABLE
-- is always present and lets unit tests exercise inserts / range queries.
CREATE TABLE IF NOT EXISTS camera_segment_index_sqlite (
    camera_id           TEXT NOT NULL,
    recorder_id         TEXT NOT NULL,
    tenant_id           TEXT NOT NULL,
    start_ts            DATETIME NOT NULL,
    end_ts              DATETIME NOT NULL,
    file_path           TEXT NOT NULL,
    file_size_bytes     INTEGER NOT NULL DEFAULT 0,
    storage_tier        TEXT NOT NULL DEFAULT 'hot',
    checksum_sha256     TEXT,
    region              TEXT NOT NULL DEFAULT 'us-east-2',
    PRIMARY KEY (camera_id, start_ts)
);
