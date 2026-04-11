-- KAI-372: Escalation chains with per-alert state machine and PagerDuty fallback.
--
-- escalation_chains: customer-defined escalation rule sets.
-- escalation_steps: ordered tiers within a chain.
-- alert_escalations: per-alert state machine tracking escalation progress.

CREATE TABLE IF NOT EXISTS escalation_chains (
    chain_id    TEXT        NOT NULL,
    tenant_id   TEXT        NOT NULL,
    name        TEXT        NOT NULL,
    description TEXT        NOT NULL DEFAULT '',
    enabled     BOOLEAN     NOT NULL DEFAULT true,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (chain_id, tenant_id)
);

CREATE INDEX IF NOT EXISTS idx_escalation_chains_tenant
    ON escalation_chains (tenant_id, enabled);

CREATE TABLE IF NOT EXISTS escalation_steps (
    step_id         TEXT        NOT NULL,
    chain_id        TEXT        NOT NULL,
    tenant_id       TEXT        NOT NULL,
    step_order      INTEGER     NOT NULL,
    target_type     TEXT        NOT NULL CHECK (target_type IN ('user', 'group', 'pagerduty')),
    target_id       TEXT        NOT NULL DEFAULT '',
    channel_type    TEXT        NOT NULL CHECK (channel_type IN ('email', 'push', 'sms', 'webhook', 'pagerduty')),
    timeout_seconds INTEGER     NOT NULL DEFAULT 300,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (step_id, tenant_id)
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_escalation_steps_chain_order
    ON escalation_steps (chain_id, tenant_id, step_order);

CREATE INDEX IF NOT EXISTS idx_escalation_steps_chain
    ON escalation_steps (chain_id, tenant_id);

CREATE TABLE IF NOT EXISTS alert_escalations (
    alert_id        TEXT        NOT NULL,
    tenant_id       TEXT        NOT NULL,
    chain_id        TEXT        NOT NULL,
    current_step    INTEGER     NOT NULL DEFAULT 0,
    state           TEXT        NOT NULL DEFAULT 'pending' CHECK (state IN ('pending', 'notified', 'timeout', 'acknowledged', 'resolved', 'pagerduty_fallback', 'exhausted')),
    acked_by        TEXT,
    acked_at        TIMESTAMPTZ,
    next_escalation TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (alert_id, tenant_id)
);

CREATE INDEX IF NOT EXISTS idx_alert_escalations_pending
    ON alert_escalations (tenant_id, state, next_escalation);
