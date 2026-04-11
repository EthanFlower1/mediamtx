-- KAI-291: custom model upload registry
CREATE TABLE IF NOT EXISTS models (
    id            TEXT NOT NULL PRIMARY KEY,
    tenant_id     TEXT NOT NULL,
    name          TEXT NOT NULL,
    version       TEXT NOT NULL,
    framework     TEXT NOT NULL,
    file_ref      TEXT NOT NULL,
    file_sha256   TEXT NOT NULL DEFAULT '',
    size_bytes    INTEGER NOT NULL DEFAULT 0,
    metrics       TEXT NOT NULL DEFAULT '{}',
    approval_state TEXT NOT NULL DEFAULT 'draft',
    approved_by   TEXT,
    approved_at   DATETIME,
    owner_user_id TEXT NOT NULL DEFAULT '',
    created_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(tenant_id, name, version)
);
