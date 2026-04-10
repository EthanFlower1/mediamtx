-- 0011_federated_catalog: Per-peer catalog cache for federation sync (KAI-271).
--
-- Each table caches the last-known state of a resource from a federated peer.
-- Rows are replaced in bulk on each successful sync cycle. The peer_id column
-- is the local identifier for the remote Directory instance.

CREATE TABLE IF NOT EXISTS federation_peers (
    peer_id       TEXT PRIMARY KEY,
    name          TEXT NOT NULL DEFAULT '',
    endpoint      TEXT NOT NULL DEFAULT '',
    tenant_id     TEXT NOT NULL DEFAULT '',
    tenant_type   INTEGER NOT NULL DEFAULT 0,
    last_sync_at  DATETIME,
    sync_status   TEXT NOT NULL DEFAULT 'pending',  -- pending | synced | error | stale
    sync_error    TEXT NOT NULL DEFAULT '',
    created_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS federated_cameras (
    id            TEXT NOT NULL,
    peer_id       TEXT NOT NULL REFERENCES federation_peers(peer_id) ON DELETE CASCADE,
    name          TEXT NOT NULL DEFAULT '',
    recorder_id   TEXT NOT NULL DEFAULT '',
    manufacturer  TEXT NOT NULL DEFAULT '',
    model         TEXT NOT NULL DEFAULT '',
    ip_address    TEXT NOT NULL DEFAULT '',
    state         INTEGER NOT NULL DEFAULT 0,
    labels        TEXT NOT NULL DEFAULT '[]',        -- JSON array
    synced_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (peer_id, id)
);

CREATE TABLE IF NOT EXISTS federated_users (
    id            TEXT NOT NULL,
    peer_id       TEXT NOT NULL REFERENCES federation_peers(peer_id) ON DELETE CASCADE,
    username      TEXT NOT NULL DEFAULT '',
    email         TEXT NOT NULL DEFAULT '',
    display_name  TEXT NOT NULL DEFAULT '',
    groups        TEXT NOT NULL DEFAULT '[]',        -- JSON array
    disabled      INTEGER NOT NULL DEFAULT 0,
    synced_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (peer_id, id)
);

CREATE TABLE IF NOT EXISTS federated_groups (
    id            TEXT NOT NULL,
    peer_id       TEXT NOT NULL REFERENCES federation_peers(peer_id) ON DELETE CASCADE,
    name          TEXT NOT NULL DEFAULT '',
    description   TEXT NOT NULL DEFAULT '',
    synced_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (peer_id, id)
);

CREATE INDEX IF NOT EXISTS idx_federated_cameras_peer ON federated_cameras(peer_id);
CREATE INDEX IF NOT EXISTS idx_federated_users_peer   ON federated_users(peer_id);
CREATE INDEX IF NOT EXISTS idx_federated_groups_peer  ON federated_groups(peer_id);
