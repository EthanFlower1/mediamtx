-- KAI-XXX: Extend the cameras table with NVR columns so that
-- directory/db can own the Camera CRUD that was previously in legacydb.
--
-- The original 0006 schema has tenant_id/recorder_id as NOT NULL with no
-- defaults, which makes it incompatible with the legacydb-style Camera CRUD
-- that does not supply those fields. We work around SQLite's inability to
-- ALTER COLUMN by recreating the table with relaxed constraints and all NVR
-- columns, then migrating existing rows.
--
-- All FK child tables (segment_index, recording_schedules, retention_policies,
-- recording_rule_actions, assigned_cameras) reference cameras(id) — because
-- SQLite enforces FK integrity at DML time (not schema time) and we preserve
-- all id values, the FKs remain valid after the swap.

PRAGMA foreign_keys = OFF;

-- Step 1: rename old table.
ALTER TABLE cameras RENAME TO cameras_v1;

-- Step 2: create new table with all columns and relaxed NOT NULL constraints.
CREATE TABLE cameras (
    id                          TEXT     NOT NULL PRIMARY KEY,
    -- Legacy Directory columns (now nullable for NVR-created cameras).
    tenant_id                   TEXT     NOT NULL DEFAULT '',
    recorder_id                 TEXT     NOT NULL DEFAULT '',
    manufacturer                TEXT     NOT NULL DEFAULT '',
    model                       TEXT     NOT NULL DEFAULT '',
    ip                          TEXT     NOT NULL DEFAULT '',
    port                        INTEGER  NOT NULL DEFAULT 554,
    onvif_port                  INTEGER  NOT NULL DEFAULT 80,
    rtsp_credentials_encrypted  BLOB,
    rtsp_credentials_nonce      BLOB,
    profile_token               TEXT     NOT NULL DEFAULT '',
    enabled                     INTEGER  NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),
    -- NVR / legacydb columns.
    name                        TEXT     NOT NULL DEFAULT '',
    onvif_endpoint              TEXT     NOT NULL DEFAULT '',
    onvif_username              TEXT     NOT NULL DEFAULT '',
    onvif_password              TEXT     NOT NULL DEFAULT '',
    onvif_profile_token         TEXT     NOT NULL DEFAULT '',
    rtsp_url                    TEXT     NOT NULL DEFAULT '',
    ptz_capable                 INTEGER  NOT NULL DEFAULT 0,
    mediamtx_path               TEXT     NOT NULL DEFAULT '',
    status                      TEXT     NOT NULL DEFAULT 'disconnected',
    tags                        TEXT     NOT NULL DEFAULT '',
    retention_days              INTEGER  NOT NULL DEFAULT 0,
    event_retention_days        INTEGER  NOT NULL DEFAULT 0,
    detection_retention_days    INTEGER  NOT NULL DEFAULT 0,
    supports_ptz                INTEGER  NOT NULL DEFAULT 0,
    supports_imaging            INTEGER  NOT NULL DEFAULT 0,
    supports_events             INTEGER  NOT NULL DEFAULT 0,
    supports_relay              INTEGER  NOT NULL DEFAULT 0,
    supports_audio_backchannel  INTEGER  NOT NULL DEFAULT 0,
    snapshot_uri                TEXT     NOT NULL DEFAULT '',
    supports_media2             INTEGER  NOT NULL DEFAULT 0,
    supports_analytics          INTEGER  NOT NULL DEFAULT 0,
    supports_edge_recording     INTEGER  NOT NULL DEFAULT 0,
    service_capabilities        TEXT     NOT NULL DEFAULT '',
    motion_timeout_seconds      INTEGER  NOT NULL DEFAULT 0,
    sub_stream_url              TEXT     NOT NULL DEFAULT '',
    ai_enabled                  INTEGER  NOT NULL DEFAULT 0,
    ai_stream_id                TEXT     NOT NULL DEFAULT '',
    ai_track_timeout            INTEGER  NOT NULL DEFAULT 5,
    ai_confidence               REAL     NOT NULL DEFAULT 0.5,
    audio_transcode             INTEGER  NOT NULL DEFAULT 0,
    recording_stream_id         TEXT     NOT NULL DEFAULT '',
    storage_path                TEXT     NOT NULL DEFAULT '',
    quota_bytes                 INTEGER  NOT NULL DEFAULT 0,
    quota_warning_percent       INTEGER  NOT NULL DEFAULT 80,
    quota_critical_percent      INTEGER  NOT NULL DEFAULT 90,
    supported_event_topics      TEXT     NOT NULL DEFAULT '',
    device_id                   TEXT,
    channel_index               INTEGER,
    multicast_enabled           INTEGER  NOT NULL DEFAULT 0,
    multicast_address           TEXT     NOT NULL DEFAULT '',
    multicast_port              INTEGER  NOT NULL DEFAULT 0,
    multicast_ttl               INTEGER  NOT NULL DEFAULT 0,
    confidence_thresholds       TEXT     NOT NULL DEFAULT '',
    device_info                 TEXT     NOT NULL DEFAULT '',
    created_at                  TEXT     NOT NULL DEFAULT '',
    updated_at                  TEXT     NOT NULL DEFAULT ''
);

-- Step 3: copy existing rows from old table (new NVR columns get defaults).
INSERT INTO cameras (
    id, tenant_id, recorder_id, manufacturer, model, ip, port, onvif_port,
    rtsp_credentials_encrypted, rtsp_credentials_nonce, profile_token, enabled,
    name, rtsp_url,
    created_at, updated_at
)
SELECT
    id, tenant_id, recorder_id, manufacturer, model, ip, port, onvif_port,
    rtsp_credentials_encrypted, rtsp_credentials_nonce, profile_token, enabled,
    name, rtsp_url,
    CAST(created_at AS TEXT), CAST(updated_at AS TEXT)
FROM cameras_v1;

-- Step 4: drop old table.
DROP TABLE cameras_v1;

-- Step 5: recreate indexes.
CREATE INDEX IF NOT EXISTS idx_cameras_recorder ON cameras (recorder_id);
CREATE INDEX IF NOT EXISTS idx_cameras_tenant   ON cameras (tenant_id);

PRAGMA foreign_keys = ON;
