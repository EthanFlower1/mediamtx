-- Cloud connector settings — persisted so they survive restarts.
CREATE TABLE IF NOT EXISTS cloud_settings (
    key        TEXT PRIMARY KEY,
    value      TEXT NOT NULL,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
