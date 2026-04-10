-- KAI-429: behavioral_config — per-camera, per-detector configuration for
-- the six behavioral analytics detectors (KAI-284).
--
-- Design notes:
--   - PRIMARY KEY (tenant_id, camera_id, detector_type): one row per detector
--     per camera. UPSERT semantics on write.
--   - params is JSONB for flexible, schema-less detector params. Validated
--     by the Go validator before write; never interpreted by SQL.
--   - enabled defaults to false — detectors must be explicitly enabled.
--   - Cross-tenant isolation: every query MUST include tenant_id in its
--     predicate. The store enforces this by construction.
--   - SQLite tests: JSONB → TEXT, TIMESTAMPTZ → DATETIME, BOOLEAN → INTEGER
--     (translateToSQLite in migrations.go handles the rewrite).

CREATE TABLE IF NOT EXISTS behavioral_config (
    tenant_id       TEXT        NOT NULL REFERENCES customer_tenants(id) ON DELETE CASCADE,
    camera_id       TEXT        NOT NULL REFERENCES cameras(id) ON DELETE CASCADE,
    detector_type   TEXT        NOT NULL CHECK (detector_type IN (
                                    'loitering',
                                    'line_crossing',
                                    'roi',
                                    'crowd_density',
                                    'tailgating',
                                    'fall_detection'
                                )),
    params          JSONB       NOT NULL DEFAULT '{}'::jsonb,
    enabled         BOOLEAN     NOT NULL DEFAULT false,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (tenant_id, camera_id, detector_type)
);

CREATE INDEX IF NOT EXISTS idx_behavioral_config_enabled
    ON behavioral_config (tenant_id, camera_id, enabled);
