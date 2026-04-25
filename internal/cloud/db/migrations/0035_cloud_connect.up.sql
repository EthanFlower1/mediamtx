-- Sites registered via cloud connector (on-prem Directories).
CREATE TABLE IF NOT EXISTS connected_sites (
    site_id        TEXT PRIMARY KEY,
    tenant_id      TEXT NOT NULL REFERENCES customer_tenants(id),
    site_alias     TEXT NOT NULL,
    display_name   TEXT NOT NULL DEFAULT '',
    version        TEXT NOT NULL DEFAULT '',
    public_ip      TEXT NOT NULL DEFAULT '',
    lan_cidrs      TEXT NOT NULL DEFAULT '[]',
    capabilities   TEXT NOT NULL DEFAULT '{}',
    status         TEXT NOT NULL DEFAULT 'offline' CHECK (status IN ('online','offline')),
    relay_url      TEXT NOT NULL DEFAULT '',
    last_seen_at   DATETIME,
    camera_count   INTEGER NOT NULL DEFAULT 0,
    recorder_count INTEGER NOT NULL DEFAULT 0,
    disk_used_pct  REAL NOT NULL DEFAULT 0,
    created_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_connected_sites_alias
    ON connected_sites(tenant_id, site_alias);

-- Relay sessions — tracks active relay tunnels through the cloud.
CREATE TABLE IF NOT EXISTS relay_sessions (
    session_id    TEXT PRIMARY KEY,
    site_id       TEXT NOT NULL REFERENCES connected_sites(site_id),
    client_id     TEXT NOT NULL,
    stream_id     TEXT NOT NULL DEFAULT '',
    started_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_active   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    bytes_relayed INTEGER NOT NULL DEFAULT 0,
    status        TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active','closed'))
);

CREATE INDEX IF NOT EXISTS idx_relay_sessions_site
    ON relay_sessions(site_id, status);
