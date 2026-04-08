-- KAI-243: pairing_tokens table for single-use Recorder enrollment credentials.
--
-- status is an enum-like column:
--   'pending'  — issued, not yet redeemed, not yet expired
--   'redeemed' — successfully consumed by one Recorder check-in
--   'expired'  — swept by the background expiry job or explicitly cancelled
--
-- The single-use constraint is enforced at the DB level via a partial unique
-- index that prevents two rows with status='redeemed' for the same token_id,
-- and via the atomic UPDATE ... WHERE status='pending' pattern in Redeem().
--
-- signed_by is the UserID of the admin that generated the token (for audit).

CREATE TABLE IF NOT EXISTS pairing_tokens (
    token_id          TEXT        NOT NULL PRIMARY KEY,
    encoded_blob      TEXT        NOT NULL,
    status            TEXT        NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','redeemed','expired')),
    suggested_roles   TEXT        NOT NULL DEFAULT '[]',
    signed_by         TEXT        NOT NULL,
    cloud_tenant      TEXT        NOT NULL DEFAULT '',
    expires_at        DATETIME    NOT NULL,
    created_at        DATETIME    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    redeemed_at       DATETIME
);

CREATE INDEX IF NOT EXISTS idx_pairing_tokens_status_expires
    ON pairing_tokens (status, expires_at);
