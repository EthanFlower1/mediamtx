-- KAI-139: recorders — cluster members of the Directory.
--
-- A Recorder is a tier-2 node that runs MediaMTX, owns local storage, and
-- records assigned cameras. Recorders pair with the Directory via KAI-245
-- (mDNS + approval) and check in via KAI-430, but the authoritative registry
-- of recorders and their metadata lives here.
--
-- Columns:
--   id              — stable recorder identifier (UUID or recorder hostname)
--   tenant_id       — tenant this recorder belongs to
--   name            — human label shown in admin UI
--   hostname        — DNS/hostname on the LAN
--   mesh_address    — WireGuard / mesh VPN address (nullable until joined)
--   lan_cidrs       — JSON array of CIDRs this recorder fronts on the LAN;
--                     used by KAI-258 LAN-vs-gateway routing and KAI-149
--                     stream URL minting.
--   tier2_enabled   — true if this recorder participates in tier-2 (local
--                     recording) — nearly always true.
--   tier3_enabled   — true if this recorder also acts as a gateway to
--                     cloud tier-3 (rare; dedicated gateway nodes).
--   gateway_url     — optional external URL when tier3_enabled.
--   cloud_relay_url — URL the recorder uses to reach the cloud relay.
--   device_pubkey   — Ed25519 public key for mTLS / signature verification
--                     (raw 32 bytes).
--   last_checkin_at — last successful POST /api/v1/recorders/checkin.
--   created_at/updated_at — audit timestamps.
--
-- FK behavior: cameras.recorder_id → recorders.id uses ON DELETE RESTRICT
-- (defined in cameras migration) because deleting a recorder while cameras
-- still reference it would orphan recordings.

CREATE TABLE IF NOT EXISTS recorders (
    id               TEXT        NOT NULL PRIMARY KEY,
    tenant_id        TEXT        NOT NULL,
    name             TEXT        NOT NULL DEFAULT '',
    hostname         TEXT        NOT NULL DEFAULT '',
    mesh_address     TEXT        NOT NULL DEFAULT '',
    lan_cidrs        TEXT        NOT NULL DEFAULT '[]',   -- JSON array
    tier2_enabled    INTEGER     NOT NULL DEFAULT 1 CHECK (tier2_enabled IN (0, 1)),
    tier3_enabled    INTEGER     NOT NULL DEFAULT 0 CHECK (tier3_enabled IN (0, 1)),
    gateway_url      TEXT        NOT NULL DEFAULT '',
    cloud_relay_url  TEXT        NOT NULL DEFAULT '',
    device_pubkey    BLOB,
    last_checkin_at  DATETIME,
    created_at       DATETIME    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at       DATETIME    NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_recorders_tenant
    ON recorders (tenant_id);

CREATE INDEX IF NOT EXISTS idx_recorders_last_checkin
    ON recorders (last_checkin_at);
