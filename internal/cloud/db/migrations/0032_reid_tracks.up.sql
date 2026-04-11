-- KAI-481: Cross-camera re-identification tracks
--
-- reid_tracks stores the global track identity that persists across cameras.
-- Each track belongs to a single tenant and has a current embedding vector
-- plus metadata about when/where the person was last seen.

CREATE TABLE IF NOT EXISTS reid_tracks (
    id            TEXT        NOT NULL,
    tenant_id     TEXT        NOT NULL,
    embedding     BYTEA       NOT NULL,   -- float32 little-endian, 512-dim = 2048 bytes
    embedding_dim INTEGER     NOT NULL DEFAULT 512,
    first_camera  TEXT        NOT NULL,
    last_camera   TEXT        NOT NULL,
    first_seen    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    match_count   INTEGER     NOT NULL DEFAULT 1,
    metadata      JSONB       NOT NULL DEFAULT '{}'::jsonb,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (id, tenant_id)
);

CREATE INDEX IF NOT EXISTS idx_reid_tracks_tenant
    ON reid_tracks (tenant_id);

CREATE INDEX IF NOT EXISTS idx_reid_tracks_last_seen
    ON reid_tracks (tenant_id, last_seen DESC);

CREATE INDEX IF NOT EXISTS idx_reid_tracks_last_camera
    ON reid_tracks (tenant_id, last_camera);

-- reid_sightings records each individual detection that was matched to a track.
-- This provides the audit trail for "person X was seen at camera Y at time Z".

CREATE TABLE IF NOT EXISTS reid_sightings (
    id          TEXT        NOT NULL,
    tenant_id   TEXT        NOT NULL,
    track_id    TEXT        NOT NULL,
    camera_id   TEXT        NOT NULL,
    embedding   BYTEA       NOT NULL,
    confidence  REAL        NOT NULL DEFAULT 0.0,
    bbox_x      REAL        NOT NULL DEFAULT 0.0,
    bbox_y      REAL        NOT NULL DEFAULT 0.0,
    bbox_w      REAL        NOT NULL DEFAULT 0.0,
    bbox_h      REAL        NOT NULL DEFAULT 0.0,
    seen_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (id, tenant_id)
);

CREATE INDEX IF NOT EXISTS idx_reid_sightings_track
    ON reid_sightings (tenant_id, track_id, seen_at DESC);

CREATE INDEX IF NOT EXISTS idx_reid_sightings_camera
    ON reid_sightings (tenant_id, camera_id, seen_at DESC);
