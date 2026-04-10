-- KAI-400: API key management — per-tenant keys, scoping, rotation, audit
--
-- api_keys stores the hashed key material and metadata. The plaintext key is
-- returned exactly once at creation and never stored.

CREATE TABLE IF NOT EXISTS api_keys (
    id              TEXT PRIMARY KEY,
    tenant_id       TEXT NOT NULL,
    name            TEXT NOT NULL,
    key_prefix      TEXT NOT NULL,          -- first 8 chars of the key for display ("kvue_a1b2…")
    key_hash        TEXT NOT NULL,          -- SHA-256 hex of the full key
    scopes          JSONB NOT NULL DEFAULT '[]'::jsonb,
    tier            TEXT NOT NULL DEFAULT 'free',
    created_by      TEXT NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at      TIMESTAMPTZ,            -- NULL = never expires
    revoked_at      TIMESTAMPTZ,            -- NULL = not revoked
    last_used_at    TIMESTAMPTZ,
    rotated_from_id TEXT,                   -- links to the previous key on rotation
    grace_expires_at TIMESTAMPTZ            -- when the old key stops working after rotation
);

-- postgres-only:begin
CREATE INDEX idx_api_keys_tenant ON api_keys (tenant_id);
CREATE INDEX idx_api_keys_hash ON api_keys (key_hash);
CREATE INDEX idx_api_keys_expires ON api_keys (expires_at) WHERE expires_at IS NOT NULL AND revoked_at IS NULL;
-- postgres-only:end

-- api_key_audit_log records every lifecycle event for a key.
CREATE TABLE IF NOT EXISTS api_key_audit_log (
    id          TEXT PRIMARY KEY,
    key_id      TEXT NOT NULL,
    tenant_id   TEXT NOT NULL,
    action      TEXT NOT NULL,             -- create, rotate, revoke, authenticate, auth_fail
    actor_id    TEXT NOT NULL,             -- user or "system"
    ip_address  TEXT,
    user_agent  TEXT,
    metadata    JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- postgres-only:begin
CREATE INDEX idx_api_key_audit_tenant ON api_key_audit_log (tenant_id, created_at DESC);
CREATE INDEX idx_api_key_audit_key ON api_key_audit_log (key_id, created_at DESC);
-- postgres-only:end
