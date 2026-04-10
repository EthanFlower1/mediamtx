-- KAI-139: retention_policies — tiered retention + archive settings.
--
-- Like recording_schedules, a row with camera_id = NULL is the tenant default
-- and rows with a concrete camera_id override it.
--
-- Tiers (days):
--   hot_days          — local recorder storage (fast random access).
--   warm_days         — local but colder (e.g. secondary disk).
--   cold_days         — cloud archive (KAI-265, R2).
--   delete_after_days — hard delete horizon (must be >= sum of tiers).
--
-- archive_tier describes server-side encryption for cold storage:
--   'standard' — plain S3/R2 SSE.
--   'sse-kms'  — KMS-managed keys.
--   'cse-cmk'  — client-side encryption with a customer-managed key.

CREATE TABLE IF NOT EXISTS retention_policies (
    id                 TEXT    NOT NULL PRIMARY KEY,
    tenant_id          TEXT    NOT NULL,
    camera_id          TEXT,
    hot_days           INTEGER NOT NULL DEFAULT 7,
    warm_days          INTEGER NOT NULL DEFAULT 0,
    cold_days          INTEGER NOT NULL DEFAULT 0,
    delete_after_days  INTEGER NOT NULL DEFAULT 30,
    archive_tier       TEXT    NOT NULL DEFAULT 'standard'
                               CHECK (archive_tier IN ('standard', 'sse-kms', 'cse-cmk')),
    created_at         DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at         DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (camera_id) REFERENCES cameras (id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_retention_policies_tenant
    ON retention_policies (tenant_id);

CREATE INDEX IF NOT EXISTS idx_retention_policies_camera
    ON retention_policies (camera_id);
