-- KAI-269: federation_tokens table for single-use Directory-to-Directory pairing.
--
-- status follows the same pattern as pairing_tokens:
--   'pending'  — issued, not yet redeemed, not yet expired
--   'redeemed' — successfully consumed by one peer Directory join
--   'expired'  — swept by the background expiry job
--
-- The founding Directory mints a FED-prefixed token and stores it here.
-- When a peer Directory presents the token via POST /api/v1/federation/join,
-- the handler atomically transitions status to 'redeemed' via
-- UPDATE ... WHERE status='pending' AND expires_at > now().

CREATE TABLE IF NOT EXISTS federation_tokens (
    token_id          TEXT        NOT NULL PRIMARY KEY,
    encoded_blob      TEXT        NOT NULL,
    status            TEXT        NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','redeemed','expired')),
    peer_site_id      TEXT        NOT NULL,
    issued_by         TEXT        NOT NULL,
    expires_at        DATETIME    NOT NULL,
    created_at        DATETIME    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    redeemed_at       DATETIME
);

CREATE INDEX IF NOT EXISTS idx_federation_tokens_status_expires
    ON federation_tokens (status, expires_at);

-- KAI-269: federation_members table records peer Directories that have joined.
--
-- After a successful handshake (token validation + JWKS exchange + optional
-- mTLS cert issuance), a row is written here. This is the authoritative
-- membership list for the federation cluster.

CREATE TABLE IF NOT EXISTS federation_members (
    site_id           TEXT        NOT NULL PRIMARY KEY,
    name              TEXT        NOT NULL DEFAULT '',
    endpoint          TEXT        NOT NULL,
    jwks_json         TEXT        NOT NULL,
    ca_fingerprint    TEXT        NOT NULL DEFAULT '',
    joined_at         DATETIME    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_seen_at      DATETIME,
    status            TEXT        NOT NULL DEFAULT 'active' CHECK (status IN ('active','suspended','removed'))
);

CREATE INDEX IF NOT EXISTS idx_federation_members_status
    ON federation_members (status);
