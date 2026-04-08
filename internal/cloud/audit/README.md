# cloud/audit — KAI-233

Multi-tenant audit log service for the MediaMTX / Kaivue cloud control plane.

## The contract (non-negotiable)

> **Every authenticated API handler in the cloud MUST emit exactly one audit
> entry before returning.**

- 2xx responses → `ResultAllow`
- 403 responses → `ResultDeny`
- Errors (4xx/5xx other than 403) → handler calls `Recorder.Record` explicitly with `ResultError` + a specific `ErrorCode`

The HTTP middleware at `./middleware` automates the first two cases. The
third case cannot be automated because only the handler knows the correct
`error_code` to attribute.

## Why

- **SOC 2** CC4.1 / CC7.2 — continuous monitoring + incident detection
- **HIPAA** §164.312(b) — audit controls over ePHI access
- **Seam #4** (multi-tenant isolation) — the cross-tenant chaos test
  (`TestMemoryRecorder_ChaosCrossTenant`, `TestSQLRecorder_ChaosCrossTenant`)
  is the living proof that tenant A can never see tenant B's log

## Schema

Entries are stored in a partitioned `audit_log` parent table (declared by
the KAI-218 migration set, with a fallback stub in
`SQLRecorder.ApplyStubSchema` for tests and local dev until KAI-218
merges). Monthly range partitions on `timestamp`, one child per month,
named `audit_log_YYYY_MM`.

Columns:

| column                    | type         | notes                                           |
|---------------------------|--------------|-------------------------------------------------|
| id                        | TEXT PK      | 24-char hex, generated if empty                 |
| tenant_id                 | TEXT NOT NULL | tenant whose data was touched                   |
| actor_user_id             | TEXT NOT NULL | authenticated principal                        |
| actor_agent               | TEXT         | cloud / on_prem / integrator / federation      |
| impersonating_user_id     | TEXT NULL    | set on integrator/federation cross-tenant hops |
| impersonated_tenant_id    | TEXT NULL    | set together with impersonating_user_id         |
| action                    | TEXT         | canonical action string (see KAI-225)          |
| resource_type             | TEXT         | e.g. "camera", "user"                          |
| resource_id               | TEXT         | empty for collection-level actions             |
| result                    | TEXT         | allow / deny / error                           |
| error_code                | TEXT NULL    | required when result = error                   |
| ip_address                | TEXT         |                                                 |
| user_agent                | TEXT         |                                                 |
| request_id                | TEXT         |                                                 |
| timestamp                 | TIMESTAMPTZ  | insert time, UTC                                |

Indexes:

- `(tenant_id, timestamp DESC)` — primary query path
- `(impersonated_tenant_id, timestamp DESC)` — integrator-home audit
- `(actor_user_id, timestamp DESC)` — "what did this user do" queries

## Retention

- **Default: 7 years** (`audit.DefaultRetention`). Covers SOC 2 evidence
  retention, HIPAA §164.316(b)(2), and PCI DSS 10.7.
- **Per-tenant override** via `tenant_audit_retention` — a regulated
  tenant (finance, medical) can contractually require 10+ years.
- `PartitionManager.DropExpiredPartitions(now, retention)` drops any child
  partition whose upper bound is older than `now - retention`. Safe to run
  as a daily River job.
- `PartitionManager.CreateNextMonthPartition(now)` runs pg_partman-style
  to pre-create the partition one month ahead of the insert edge.

## Dependencies on sibling tickets

### KAI-218 — `internal/cloud/db/migrations/`

At the time of writing, KAI-218's migration set contains
`0001_integrators` → `0003_customer_integrator_relationships` and does
**not** yet include:

- the partitioned `audit_log` parent table
- the `tenant_audit_retention` override table

**TODO for KAI-218:** add `0006_audit_log_partitioned.up.sql` with:

```sql
CREATE TABLE audit_log (
    id                     TEXT NOT NULL,
    tenant_id              TEXT NOT NULL,
    actor_user_id          TEXT NOT NULL,
    actor_agent            TEXT NOT NULL,
    impersonating_user_id  TEXT,
    impersonated_tenant_id TEXT,
    action                 TEXT NOT NULL,
    resource_type          TEXT NOT NULL,
    resource_id            TEXT NOT NULL DEFAULT '',
    result                 TEXT NOT NULL,
    error_code             TEXT,
    ip_address             TEXT NOT NULL DEFAULT '',
    user_agent             TEXT NOT NULL DEFAULT '',
    request_id             TEXT NOT NULL DEFAULT '',
    timestamp              TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (timestamp, id)
) PARTITION BY RANGE (timestamp);

CREATE INDEX idx_audit_log_tenant_ts
    ON audit_log (tenant_id, timestamp DESC);
CREATE INDEX idx_audit_log_impersonated
    ON audit_log (impersonated_tenant_id, timestamp DESC);
CREATE INDEX idx_audit_log_actor
    ON audit_log (actor_user_id, timestamp DESC);

CREATE TABLE tenant_audit_retention (
    tenant_id         TEXT PRIMARY KEY
                     REFERENCES customer_tenants(id) ON DELETE CASCADE,
    retention_seconds BIGINT NOT NULL
);
```

Plus an initial partition created at deploy time so the first insert
doesn't fail:

```sql
CREATE TABLE audit_log_2026_04
  PARTITION OF audit_log
  FOR VALUES FROM ('2026-04-01') TO ('2026-05-01');
```

Until KAI-218 lands this, tests call
`SQLRecorder.ApplyStubSchema` which creates an equivalent (unpartitioned)
table so the code path is exercised end-to-end.

### KAI-225 — `internal/cloud/permissions/`

The Casbin enforcer calls `audit.Recorder.Record` on every authorization
decision. The interface shape in `entry.go` is stable — KAI-225 imports
`audit.Entry`, `audit.ResultAllow`, and `audit.ResultDeny` directly.

## Seam #4 guarantees

1. `QueryFilter.TenantID` is required — empty is a hard error.
2. Both in-memory and SQL backends filter by tenant **before** any other
   predicate in the WHERE clause.
3. The SQL backend double-checks the returned row's `tenant_id` after
   scanning, returning `ErrTenantMismatch` if a future refactor ever
   silently drops the WHERE clause.
4. The chaos tests populate entries across 5–10 tenants and assert that
   every per-tenant query returns ONLY that tenant's rows.

## Files

- `entry.go` — `Entry`, `QueryFilter`, `Recorder` interface, constants
- `memory.go` — in-memory Recorder for tests and scaffolding
- `sql.go` — SQL Recorder (Postgres production, SQLite for tests)
- `export.go` — shared CSV + JSON streaming exporter
- `partitions.go` — monthly partition management helpers
- `retention.go` — per-tenant retention override store
- `middleware/middleware.go` — HTTP middleware: 2xx → allow, 403 → deny
