-- KAI-140: assigned_cameras — Recorder-local cache of the camera subset
-- assigned to this node by the Directory (via KAI-143 StreamAssignments push).
--
-- This is a denormalized mirror of internal/directory/db `cameras` +
-- `recording_schedules` + `retention_policies`, containing only the columns
-- the Recorder needs to drive MediaMTX path generation (KAI-152/259) and
-- the capture/archive hot path (KAI-265). It is intentionally duplicated;
-- the seam is "Directory pushes, Recorder applies".
--
-- Fail-open recording: the capture loop reads this table, NOT the Directory,
-- so recording continues if the Directory link is down.
--
-- Credentials are stored as (encrypted_blob, nonce) and decrypted at point of
-- use via the Recorder keyring (KAI-141). They must never be logged.
--
-- last_applied_revision is a monotonic counter from Directory's StreamAssignments
-- push; the reconciler (KAI-143) uses it to drive idempotent apply.

CREATE TABLE IF NOT EXISTS assigned_cameras (
    id                           TEXT     NOT NULL PRIMARY KEY,
    tenant_id                    TEXT     NOT NULL,
    name                         TEXT     NOT NULL DEFAULT '',
    manufacturer                 TEXT     NOT NULL DEFAULT '',
    model                        TEXT     NOT NULL DEFAULT '',
    ip                           TEXT     NOT NULL DEFAULT '',
    port                         INTEGER  NOT NULL DEFAULT 0,
    onvif_port                   INTEGER  NOT NULL DEFAULT 0,
    rtsp_url                     TEXT     NOT NULL DEFAULT '',
    rtsp_credentials_encrypted   BLOB,
    rtsp_credentials_nonce       BLOB,
    profile_token                TEXT     NOT NULL DEFAULT '',
    enabled                      INTEGER  NOT NULL DEFAULT 0 CHECK (enabled IN (0, 1)),
    recording_mode               TEXT     NOT NULL DEFAULT 'off'
                                          CHECK (recording_mode IN ('continuous', 'motion', 'schedule', 'off')),
    schedule_cron                TEXT,
    pre_roll_seconds             INTEGER  NOT NULL DEFAULT 0,
    post_roll_seconds            INTEGER  NOT NULL DEFAULT 0,
    hot_days                     INTEGER  NOT NULL DEFAULT 0,
    warm_days                    INTEGER  NOT NULL DEFAULT 0,
    cold_days                    INTEGER  NOT NULL DEFAULT 0,
    delete_after_days            INTEGER  NOT NULL DEFAULT 0,
    archive_tier                 TEXT     NOT NULL DEFAULT 'standard'
                                          CHECK (archive_tier IN ('standard', 'sse-kms', 'cse-cmk')),
    last_applied_revision        INTEGER  NOT NULL DEFAULT 0,
    cached_at                    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Hot filter for MediaMTX path generator: "give me every enabled camera".
CREATE INDEX IF NOT EXISTS idx_assigned_cameras_enabled
    ON assigned_cameras (enabled);
