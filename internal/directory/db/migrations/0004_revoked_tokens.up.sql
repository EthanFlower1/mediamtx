-- KAI-158: revoked_tokens — local JWT blocklist for force-revocation.
--
-- When an admin force-revokes all tokens for a recorder (or globally), each
-- affected token's JTI is inserted here. The Directory middleware checks
-- this table on every authenticated request; a match results in 401.
--
-- Rows are GC'd after the token's original expiry (expires_at) since
-- expired tokens are rejected by the verifier anyway.
--
-- Supported revocation modes:
--   'recorder'  — all tokens for a specific recorder (by subject prefix)
--   'global'    — all tokens system-wide (e.g. compromise response)
--
-- The recorder_id column is informational / query-helper; the JTI is
-- the actual enforcement key.

CREATE TABLE IF NOT EXISTS revoked_tokens (
    jti           TEXT     NOT NULL PRIMARY KEY,
    recorder_id   TEXT     NOT NULL DEFAULT '',
    tenant_id     TEXT     NOT NULL DEFAULT '',
    revoked_by    TEXT     NOT NULL,               -- admin user ID
    reason        TEXT     NOT NULL DEFAULT '',
    revoked_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at    DATETIME NOT NULL                -- original token expiry; row is GC-eligible after this
);

CREATE INDEX IF NOT EXISTS idx_revoked_tokens_recorder
    ON revoked_tokens (recorder_id);

CREATE INDEX IF NOT EXISTS idx_revoked_tokens_expires
    ON revoked_tokens (expires_at);
