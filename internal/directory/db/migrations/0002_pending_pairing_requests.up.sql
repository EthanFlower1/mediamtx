-- KAI-245: pending_pairing_requests table for mDNS auto-discovery approval flow.
--
-- When a new Recorder discovers the Directory via mDNS and calls
-- POST /api/v1/pairing/request, a row is inserted here with status='pending'.
-- An admin then approves or denies via the API. On approval the Directory
-- mints a PairingToken (KAI-243) and returns it to the waiting Recorder.
--
-- status values:
--   'pending'  — awaiting admin decision
--   'approved' — admin approved; token has been minted and delivered
--   'denied'   — admin explicitly denied
--   'expired'  — 5-minute TTL elapsed without a decision
--
-- The token_id column is populated on approval (FK → pairing_tokens.token_id).

CREATE TABLE IF NOT EXISTS pending_pairing_requests (
    id              TEXT        NOT NULL PRIMARY KEY,   -- UUID
    recorder_hostname TEXT      NOT NULL,
    recorder_ip     TEXT        NOT NULL DEFAULT '',
    requested_roles TEXT        NOT NULL DEFAULT '["recorder"]',
    status          TEXT        NOT NULL DEFAULT 'pending'
                                CHECK (status IN ('pending','approved','denied','expired')),
    token_id        TEXT,                               -- populated on approval
    request_note    TEXT        NOT NULL DEFAULT '',    -- optional human hint from Recorder
    expires_at      DATETIME    NOT NULL,               -- 5 min from created_at
    created_at      DATETIME    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    decided_at      DATETIME,
    decided_by      TEXT        NOT NULL DEFAULT ''     -- UserID of admin who acted
);

CREATE INDEX IF NOT EXISTS idx_pending_pairing_status_expires
    ON pending_pairing_requests (status, expires_at);
