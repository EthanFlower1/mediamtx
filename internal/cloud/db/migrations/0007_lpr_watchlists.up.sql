-- KAI-283: License plate recognition — watchlist schema.
--
-- Multi-tenant isolation: every table is scoped by tenant_id.
-- Retention: retention_days on lpr_watchlists controls how long lpr_reads
-- rows for a given watchlist are kept (background sweeper, not in this
-- migration). Default 90 days per the EU AI Act data-minimisation guideline.

CREATE TABLE IF NOT EXISTS lpr_watchlists (
    id              TEXT PRIMARY KEY,
    tenant_id       TEXT NOT NULL,
    name            TEXT NOT NULL,
    type            TEXT NOT NULL CHECK (type IN ('allow', 'deny', 'alert')),
    retention_days  INTEGER NOT NULL DEFAULT 90,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_lpr_watchlists_tenant
    ON lpr_watchlists (tenant_id);

CREATE TABLE IF NOT EXISTS lpr_watchlist_entries (
    id              TEXT PRIMARY KEY,
    watchlist_id    TEXT NOT NULL REFERENCES lpr_watchlists (id) ON DELETE CASCADE,
    plate_text      TEXT NOT NULL,   -- normalised: upper-case, no separators
    label           TEXT,
    expires_at      TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_lpr_entries_watchlist
    ON lpr_watchlist_entries (watchlist_id);

CREATE INDEX IF NOT EXISTS idx_lpr_entries_plate
    ON lpr_watchlist_entries (plate_text);

-- lpr_reads is the forensic read log. All reads are stored regardless of
-- watchlist membership.
--
-- Assumption: pg_partman is configured in the RDS instance (KAI-216).
-- The parent table is created here; pg_partman manages monthly child
-- partitions via the partition_table() call below.

-- postgres-only:begin
CREATE TABLE IF NOT EXISTS lpr_reads (
    id                  BIGSERIAL,
    tenant_id           TEXT NOT NULL,
    camera_id           TEXT NOT NULL,
    read_at             TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    plate_text          TEXT NOT NULL,
    confidence          NUMERIC(5,2) NOT NULL,
    region              TEXT NOT NULL DEFAULT '',
    bounding_box        JSONB NOT NULL DEFAULT '{}'::jsonb,
    cropped_image_ref   TEXT,
    watchlist_match_id  TEXT REFERENCES lpr_watchlist_entries (id) ON DELETE SET NULL,
    PRIMARY KEY (id, read_at)
) PARTITION BY RANGE (read_at);

CREATE INDEX IF NOT EXISTS idx_lpr_reads_tenant_time
    ON lpr_reads (tenant_id, read_at DESC);

CREATE INDEX IF NOT EXISTS idx_lpr_reads_plate
    ON lpr_reads (tenant_id, plate_text, read_at DESC);

-- Hand the partition management to pg_partman (configured in KAI-216's
-- RDS setup). This creates the first monthly partition immediately.
SELECT partman.create_parent(
    p_parent_table  => 'public.lpr_reads',
    p_control       => 'read_at',
    p_type          => 'range',
    p_interval      => 'monthly',
    p_premake       => 3
);
-- postgres-only:end
