-- Camera state: latest health snapshot per camera per recorder.
-- Upserted on each StreamCameraState batch.
CREATE TABLE IF NOT EXISTS camera_states (
    camera_id    TEXT    NOT NULL,
    recorder_id  TEXT    NOT NULL,
    state        TEXT    NOT NULL CHECK(state IN ('online','degraded','offline','error')),
    error_message TEXT   NOT NULL DEFAULT '',
    current_bitrate_kbps INTEGER NOT NULL DEFAULT 0,
    current_framerate    INTEGER NOT NULL DEFAULT 0,
    last_frame_at        DATETIME,
    config_version       INTEGER NOT NULL DEFAULT 0,
    observed_at          DATETIME NOT NULL,
    updated_at           DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (camera_id, recorder_id)
);

CREATE INDEX IF NOT EXISTS idx_camera_states_recorder ON camera_states(recorder_id);
CREATE INDEX IF NOT EXISTS idx_camera_states_state    ON camera_states(state);

-- Segment index: authoritative recording timeline.
-- Inserted on each PublishSegmentIndex batch. segment_id is globally unique.
CREATE TABLE IF NOT EXISTS segment_index (
    segment_id   TEXT    PRIMARY KEY,
    camera_id    TEXT    NOT NULL,
    recorder_id  TEXT    NOT NULL,
    start_time   DATETIME NOT NULL,
    end_time     DATETIME NOT NULL,
    bytes        INTEGER NOT NULL DEFAULT 0,
    codec        TEXT    NOT NULL DEFAULT '',
    has_audio    BOOLEAN NOT NULL DEFAULT 0,
    is_event_clip BOOLEAN NOT NULL DEFAULT 0,
    storage_tier TEXT    NOT NULL DEFAULT 'local',
    sequence     INTEGER NOT NULL DEFAULT 0,
    created_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_segment_index_camera_time
    ON segment_index(camera_id, start_time, end_time);
CREATE INDEX IF NOT EXISTS idx_segment_index_recorder
    ON segment_index(recorder_id);

-- AI events: detection events from feature pipelines.
-- Inserted on each PublishAIEvents batch. event_id is globally unique.
CREATE TABLE IF NOT EXISTS ai_events (
    event_id     TEXT    PRIMARY KEY,
    camera_id    TEXT    NOT NULL,
    recorder_id  TEXT    NOT NULL,
    kind         TEXT    NOT NULL,
    kind_label   TEXT    NOT NULL DEFAULT '',
    observed_at  DATETIME NOT NULL,
    confidence   REAL    NOT NULL DEFAULT 0,
    bbox_x       REAL    NOT NULL DEFAULT 0,
    bbox_y       REAL    NOT NULL DEFAULT 0,
    bbox_width   REAL    NOT NULL DEFAULT 0,
    bbox_height  REAL    NOT NULL DEFAULT 0,
    track_id     TEXT    NOT NULL DEFAULT '',
    segment_id   TEXT    NOT NULL DEFAULT '',
    thumbnail_ref TEXT   NOT NULL DEFAULT '',
    attributes   TEXT    NOT NULL DEFAULT '{}',
    created_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_ai_events_camera_time
    ON ai_events(camera_id, observed_at);
CREATE INDEX IF NOT EXISTS idx_ai_events_kind
    ON ai_events(kind, observed_at);
CREATE INDEX IF NOT EXISTS idx_ai_events_recorder
    ON ai_events(recorder_id);
