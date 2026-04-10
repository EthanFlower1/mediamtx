-- KAI-142: assigned_cameras table for camera-to-recorder assignments.
--
-- The Directory pushes camera assignments to Recorders via the
-- RecorderControl.StreamAssignments RPC. This table is the authoritative
-- source of which cameras are assigned to which recorder on-prem.
--
-- credential_ref is an opaque handle into the cryptostore (KAI-251).
-- config_json stores the serialized CameraConfig proto as JSON.
-- config_version is a monotonically increasing counter bumped on every
-- config change; the Recorder uses it to detect out-of-order events.

CREATE TABLE IF NOT EXISTS assigned_cameras (
    camera_id       TEXT    NOT NULL PRIMARY KEY,
    recorder_id     TEXT    NOT NULL,
    name            TEXT    NOT NULL DEFAULT '',
    credential_ref  TEXT    NOT NULL DEFAULT '',
    config_json     TEXT    NOT NULL DEFAULT '{}',
    config_version  INTEGER NOT NULL DEFAULT 1,
    created_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_assigned_cameras_recorder
    ON assigned_cameras (recorder_id);
