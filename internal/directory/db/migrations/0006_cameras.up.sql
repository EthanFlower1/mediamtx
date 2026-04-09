-- KAI-139: cameras — authoritative camera registry.
--
-- This is the source of truth for every camera in the Directory. Recorders
-- mirror a filtered subset into their local assigned_cameras cache (KAI-140)
-- via RecorderControl.StreamAssignments (KAI-142/143).
--
-- Columns:
--   id                         — stable camera identifier (UUID).
--   tenant_id                  — owning tenant.
--   recorder_id                — FK → recorders.id; which recorder owns the
--                                 RTSP pull. RESTRICT on delete so we never
--                                 orphan cameras if a recorder is removed
--                                 before cameras are reassigned.
--   name                       — human label.
--   manufacturer/model         — ONVIF-discovered metadata (KAI-296).
--   ip / port                  — device network address + RTSP port.
--   onvif_port                 — ONVIF service port (usually 80 or 8080).
--   rtsp_url                   — full RTSP URL (may embed placeholder creds).
--   rtsp_credentials_encrypted — AES-GCM ciphertext of username:password
--                                 (KAI-141 fills this column).
--   rtsp_credentials_nonce     — AES-GCM nonce (12 bytes) paired with the
--                                 ciphertext above.
--   profile_token              — ONVIF media profile token.
--   enabled                    — soft on/off switch; disabled cameras do not
--                                 stream or record.
--   created_at/updated_at      — audit timestamps.

CREATE TABLE IF NOT EXISTS cameras (
    id                          TEXT     NOT NULL PRIMARY KEY,
    tenant_id                   TEXT     NOT NULL,
    recorder_id                 TEXT     NOT NULL,
    name                        TEXT     NOT NULL DEFAULT '',
    manufacturer                TEXT     NOT NULL DEFAULT '',
    model                       TEXT     NOT NULL DEFAULT '',
    ip                          TEXT     NOT NULL DEFAULT '',
    port                        INTEGER  NOT NULL DEFAULT 554,
    onvif_port                  INTEGER  NOT NULL DEFAULT 80,
    rtsp_url                    TEXT     NOT NULL DEFAULT '',
    rtsp_credentials_encrypted  BLOB,
    rtsp_credentials_nonce      BLOB,
    profile_token               TEXT     NOT NULL DEFAULT '',
    enabled                     INTEGER  NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),
    created_at                  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at                  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (tenant_id, id),
    FOREIGN KEY (recorder_id) REFERENCES recorders (id) ON DELETE RESTRICT
);

CREATE INDEX IF NOT EXISTS idx_cameras_recorder
    ON cameras (recorder_id);

CREATE INDEX IF NOT EXISTS idx_cameras_tenant
    ON cameras (tenant_id);
