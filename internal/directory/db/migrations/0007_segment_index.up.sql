-- KAI-139: segment_index — authoritative index of recorded segments.
--
-- Each row describes a single finalized recording segment stored either
-- locally on a Recorder or in cloud archive tier (KAI-265). Ingested from
-- Recorders via DirectoryIngest.PublishSegmentIndex (KAI-144) and queried
-- by the multi-recorder timeline assembly API (KAI-262).
--
-- Columns:
--   id           — surrogate auto-increment PK.
--   camera_id    — FK → cameras.id; CASCADE so deleting a camera removes
--                   its index rows (the underlying files are purged by a
--                   separate retention job).
--   recorder_id  — FK → recorders.id; CASCADE for the same reason.
--   start_ts     — inclusive start timestamp (unix nanoseconds).
--   end_ts       — exclusive end timestamp (unix nanoseconds).
--   storage_uri  — file:///… for local segments, s3://… or r2://… for
--                   archived ones.
--   size_bytes   — segment file size.
--   checksum     — BLAKE3 or SHA-256 content hash for integrity checks.
--   created_at   — when this index row was ingested.

CREATE TABLE IF NOT EXISTS segment_index (
    id           INTEGER NOT NULL PRIMARY KEY AUTOINCREMENT,
    camera_id    TEXT    NOT NULL,
    recorder_id  TEXT    NOT NULL,
    start_ts     INTEGER NOT NULL,
    end_ts       INTEGER NOT NULL,
    storage_uri  TEXT    NOT NULL,
    size_bytes   INTEGER NOT NULL DEFAULT 0,
    checksum     BLOB,
    created_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (camera_id)   REFERENCES cameras   (id) ON DELETE CASCADE,
    FOREIGN KEY (recorder_id) REFERENCES recorders (id) ON DELETE CASCADE
);

-- Primary timeline query: "give me segments for camera X between t0 and t1".
CREATE INDEX IF NOT EXISTS idx_segment_index_camera_time
    ON segment_index (camera_id, start_ts, end_ts);

-- Used by retention / recorder-scoped scans.
CREATE INDEX IF NOT EXISTS idx_segment_index_recorder
    ON segment_index (recorder_id);
