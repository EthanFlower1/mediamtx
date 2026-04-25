-- Reverse of 0030: restore the original lean cameras schema.
-- WARNING: all NVR-only column data will be lost.

PRAGMA foreign_keys = OFF;

ALTER TABLE cameras RENAME TO cameras_v30;

CREATE TABLE cameras (
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

INSERT INTO cameras (id, tenant_id, recorder_id, name, manufacturer, model, ip, port, onvif_port,
    rtsp_url, rtsp_credentials_encrypted, rtsp_credentials_nonce, profile_token, enabled,
    created_at, updated_at)
SELECT id, tenant_id, recorder_id, name, manufacturer, model, ip, port, onvif_port,
    rtsp_url, rtsp_credentials_encrypted, rtsp_credentials_nonce, profile_token, enabled,
    created_at, updated_at
FROM cameras_v30;

DROP TABLE cameras_v30;

CREATE INDEX IF NOT EXISTS idx_cameras_recorder ON cameras (recorder_id);
CREATE INDEX IF NOT EXISTS idx_cameras_tenant   ON cameras (tenant_id);

PRAGMA foreign_keys = ON;
