-- KAI-254: ai_events — cloud-side store for AI detection events pushed from
-- Recorders via DirectoryIngest.PublishAIEvents.
--
-- Design notes:
--   - tenant_id is always present; every query must include it (seam #4).
--   - recorder_id + segment_id allow jump-to-playback once KAI-249 lands.
--   - attributes is JSONB for structured metadata (LP text, face id, etc.).
--   - thumbnail_ref is an opaque recorder-local id; fetched on demand.
--   - Partition by observed_at monthly (postgres-only) for forensic retention.
--
-- SQLite tests: JSONB → TEXT, TIMESTAMPTZ → DATETIME (translateToSQLite).

-- postgres-only:begin
CREATE TABLE IF NOT EXISTS ai_events (
    id              TEXT        NOT NULL,
    tenant_id       TEXT        NOT NULL,
    recorder_id     TEXT        NOT NULL,
    camera_id       TEXT        NOT NULL,
    kind            TEXT        NOT NULL,
    kind_label      TEXT        NOT NULL DEFAULT '',
    observed_at     TIMESTAMPTZ NOT NULL,
    confidence      NUMERIC(5,2) NOT NULL DEFAULT 0,
    bbox_x          NUMERIC(5,4) NOT NULL DEFAULT 0,
    bbox_y          NUMERIC(5,4) NOT NULL DEFAULT 0,
    bbox_width      NUMERIC(5,4) NOT NULL DEFAULT 0,
    bbox_height     NUMERIC(5,4) NOT NULL DEFAULT 0,
    track_id        TEXT        NOT NULL DEFAULT '',
    segment_id      TEXT        NOT NULL DEFAULT '',
    thumbnail_ref   TEXT        NOT NULL DEFAULT '',
    attributes      JSONB       NOT NULL DEFAULT '{}'::jsonb,
    received_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (id, observed_at)
) PARTITION BY RANGE (observed_at);

CREATE INDEX IF NOT EXISTS idx_ai_events_tenant_time
    ON ai_events (tenant_id, observed_at DESC);

CREATE INDEX IF NOT EXISTS idx_ai_events_camera_time
    ON ai_events (tenant_id, camera_id, observed_at DESC);

CREATE INDEX IF NOT EXISTS idx_ai_events_kind
    ON ai_events (tenant_id, kind, observed_at DESC);

SELECT partman.create_parent(
    p_parent_table  => 'public.ai_events',
    p_control       => 'observed_at',
    p_type          => 'range',
    p_interval      => 'monthly',
    p_premake       => 3
);
-- postgres-only:end

-- camera_state — latest health snapshot per camera as reported by the Recorder.
-- Updated (upserted) on each StreamCameraState batch; not time-series.
CREATE TABLE IF NOT EXISTS camera_state (
    camera_id           TEXT        NOT NULL,
    tenant_id           TEXT        NOT NULL,
    recorder_id         TEXT        NOT NULL,
    state               TEXT        NOT NULL DEFAULT 'unknown',
    error_message       TEXT        NOT NULL DEFAULT '',
    current_bitrate_kbps INTEGER    NOT NULL DEFAULT 0,
    current_framerate   INTEGER     NOT NULL DEFAULT 0,
    last_frame_at       TIMESTAMPTZ,
    config_version      BIGINT      NOT NULL DEFAULT 0,
    observed_at         TIMESTAMPTZ NOT NULL,
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (camera_id)
);

CREATE INDEX IF NOT EXISTS idx_camera_state_tenant
    ON camera_state (tenant_id);

CREATE INDEX IF NOT EXISTS idx_camera_state_recorder
    ON camera_state (tenant_id, recorder_id);

-- segment_index_stub — receives SegmentIndexEntry batches from Recorders.
-- Full schema lands with KAI-249; this is the stub that lets
-- DirectoryIngest.PublishSegmentIndex write without blocking on KAI-249.
-- TODO(KAI-249): replace with the real camera_segment_index schema once
-- KAI-249 lands and migrate existing rows.
CREATE TABLE IF NOT EXISTS segment_index_stub (
    segment_id      TEXT        NOT NULL,
    recorder_id     TEXT        NOT NULL,
    camera_id       TEXT        NOT NULL,
    tenant_id       TEXT        NOT NULL,
    start_time      TIMESTAMPTZ NOT NULL,
    end_time        TIMESTAMPTZ NOT NULL,
    bytes           BIGINT      NOT NULL DEFAULT 0,
    codec           TEXT        NOT NULL DEFAULT '',
    has_audio       BOOLEAN     NOT NULL DEFAULT FALSE,
    is_event_clip   BOOLEAN     NOT NULL DEFAULT FALSE,
    storage_tier    TEXT        NOT NULL DEFAULT '',
    sequence        BIGINT      NOT NULL DEFAULT 0,
    received_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (segment_id, recorder_id)
);

CREATE INDEX IF NOT EXISTS idx_segment_stub_camera_time
    ON segment_index_stub (tenant_id, camera_id, start_time DESC);
