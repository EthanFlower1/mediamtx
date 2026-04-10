-- KAI-357: per-integrator sender domains + DKIM keypair storage.
--
-- integrator_email_domains holds each tenant's custom sender domain
-- (e.g. "alerts.acme-security.com") together with the current
-- verification state of its SPF / DKIM / DMARC DNS records and a
-- pointer to the SendGrid subuser that was provisioned for it.
--
-- dkim_keys stores the DKIM keypairs themselves. Private keys live in
-- the cryptostore (KAI-251) and this table references them by
-- cryptostore key id; we never persist raw private key material in
-- the cloud DB. Two selectors (s1/s2) are tracked per domain so we
-- can rotate with a 48h grace period (industry-standard practice for
-- DKIM rotation).
--
-- Everything is tenant_id-scoped (Seam #4) and every read/write in
-- the Go package is required to pass tenant_id as the first WHERE
-- predicate.

CREATE TABLE IF NOT EXISTS integrator_email_domains (
    id                  TEXT        PRIMARY KEY,
    tenant_id           TEXT        NOT NULL,
    domain              TEXT        NOT NULL,
    sendgrid_subuser    TEXT        NOT NULL,
    active_selector     TEXT        NOT NULL DEFAULT 's1',
    verification_state  TEXT        NOT NULL DEFAULT 'pending',
    spf_verified_at     TIMESTAMPTZ,
    dkim_verified_at    TIMESTAMPTZ,
    dmarc_verified_at   TIMESTAMPTZ,
    last_checked_at     TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL,
    updated_at          TIMESTAMPTZ NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_integrator_email_domains_tenant_domain
    ON integrator_email_domains (tenant_id, domain);

CREATE INDEX IF NOT EXISTS idx_integrator_email_domains_state
    ON integrator_email_domains (verification_state, last_checked_at);

-- dkim_keys: one row per (domain, selector). Private key bytes live in
-- the KAI-251 cryptostore and are referenced by cryptostore_key_id.
CREATE TABLE IF NOT EXISTS dkim_keys (
    id                  TEXT        PRIMARY KEY,
    tenant_id           TEXT        NOT NULL,
    domain_id           TEXT        NOT NULL,
    selector            TEXT        NOT NULL,
    public_key_pem      TEXT        NOT NULL,
    cryptostore_key_id  TEXT        NOT NULL,
    key_size_bits       INTEGER     NOT NULL,
    status              TEXT        NOT NULL DEFAULT 'active',
    rotated_at          TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_dkim_keys_domain_selector
    ON dkim_keys (domain_id, selector);

CREATE INDEX IF NOT EXISTS idx_dkim_keys_tenant
    ON dkim_keys (tenant_id);
