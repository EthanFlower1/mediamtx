# `internal/cloud/db` — cloud control-plane schema (KAI-218 + KAI-219)

This package owns the PostgreSQL schema for the multi-tenant cloud control
plane of the Kaivue Recording Server. It ships the migration files, a thin
`database/sql` wrapper with tenant-scoped helpers, deterministic test fixtures,
and unit tests that execute the full migration set against an in-process
SQLite database.

No RDS or Postgres server is provisioned by this ticket. Real infrastructure
lands with **KAI-215** (EKS) and **KAI-216** (RDS + pgvector); a human pairs
with us on provisioning at the end of the v1 roadmap.

## What lives here

```
internal/cloud/db/
  README.md                 — this file
  db.go                     — Open / Migrate / AppliedVersions
  migrations.go             — embed.FS loader + SQLite translator
  tenant.go                 — tenant-scoped CRUD helpers (seam #4)
  db_test.go                — SQLite-backed unit tests
  migrations/
    0001_integrators.(up|down).sql
    0002_customer_tenants.(up|down).sql
    0003_customer_integrator_relationships.(up|down).sql
    0004_users.(up|down).sql
    0005_on_prem_directories.(up|down).sql
    0006_audit_log_partitioned.(up|down).sql
  fixtures/
    seed.go                 — deterministic test fixtures used by KAI-225, KAI-233
```

## Schema overview

| Table | Rows | Purpose |
|---|---|---|
| `integrators` | N | Security installers / MSPs; self-referencing for sub-reseller hierarchy (KAI-229) |
| `customer_tenants` | N | End-customer organizations; billed `direct` or `via_integrator` |
| `customer_integrator_relationships` | M | Many-to-many grants with `scoped_permissions`, markup, status |
| `users` | N | Authenticated identities; polymorphic `tenant_ref_{type,id}` pointer |
| `on_prem_directories` | N | Cloud's registry of paired on-prem Directory instances |
| `audit_log_partitioned` | N | Parent table only — KAI-233 builds the service + pg_partman monthly partitions |

Every tenant-scoped table carries `region TEXT NOT NULL DEFAULT 'us-east-2'`
and has a composite index on `(region, ...)` — this is **seam #9** of the v1
architecture: build for multi-region, ship single-region.

### Foreign-key semantics

All customer→integrator and directory→customer references use `ON DELETE
RESTRICT`. Tenants and relationships are archived, not hard-deleted, so the
audit log (KAI-233) can always resolve the `tenant_ref_id` for an historical
action. Hard deletion is gated behind the data-residency workflow
(KAI-387 GDPR/CCPA).

### Polymorphic user ownership

Users point at their owning tenant through `(tenant_ref_type, tenant_ref_id)`
where `tenant_ref_type ∈ {integrator, customer_tenant}`. This matches the
product invariant that *a user belongs to exactly one side of the channel*
(see `docs/superpowers/specs/2026-04-07-multi-recording-server-design.md`
§5.1). Casbin policy (KAI-225) keys off the same two columns.

## Migration file format

Files are named `NNNN_<short_name>.(up|down).sql`. The format is **compatible
with `golang-migrate/migrate`** — a future switch to that library is a
drop-in. We do not currently vendor `golang-migrate` itself; the in-package
runner in `migrations.go` loads files from `embed.FS`, applies them in
numerical order inside per-migration transactions, and records applied
versions in `schema_migrations`.

### Postgres-only blocks

Migration 0006 declares `audit_log_partitioned` using `PARTITION BY RANGE` —
a Postgres-only feature. The block is fenced with
`-- postgres-only:begin` … `-- postgres-only:end`. The SQLite test runner
strips these blocks entirely so the migration still advances the version
counter but creates no table. Any future Postgres-exclusive DDL (JSONB
operators, functional indexes, `pg_partman` config, `CREATE EXTENSION`) must
be fenced the same way.

## Test strategy

| Layer | Runs where | What it validates |
|---|---|---|
| **Unit tests** (this package) | SQLite via `modernc.org/sqlite`, in-process | Migration syntax parses, CRUD round-trips, tenant-scoped helpers reject missing `TenantRef`, region-scoped reads isolate tenants |
| **Postgres integration** (ships with KAI-216) | Local Postgres container in CI | JSONB columns, partitioned tables, `pg_partman`, Casbin policy tables, chaos isolation test (KAI-235) |
| **Chaos isolation test** (KAI-235) | EKS dev cluster | Full end-to-end tenant-crossing-through-API verification |

The SQLite runner rewrites a small subset of syntax:

| Postgres | SQLite |
|---|---|
| `TIMESTAMPTZ` | `DATETIME` |
| `JSONB`       | `TEXT`     |
| `NUMERIC(5,2)`| `REAL`     |
| `BIGSERIAL`   | `INTEGER`  |
| `NOW()`       | `CURRENT_TIMESTAMP` |
| `::jsonb`     | *(stripped)* |

**Postgres-specific features — `pg_partman`, partitioned tables, JSONB
operators, functional indexes — are validated manually against a local
Postgres container in the integration tests that land with KAI-216.** Do not
treat green SQLite tests as proof that a Postgres feature works.

## Using the wrapper

```go
import (
    "context"

    clouddb "github.com/bluenviron/mediamtx/internal/cloud/db"
)

d, err := clouddb.Open(ctx, "postgres://cloud_api:xxx@rds.../kaivue_cloud?sslmode=require")
if err != nil { ... }
defer d.Close()

// Seam #4: every tenant-scoped helper REQUIRES a TenantRef. Never query a
// tenant-scoped table without naming the tenant.
ref := clouddb.TenantRef{
    Type:   clouddb.TenantCustomerTenant,
    ID:     customerID,
    Region: clouddb.DefaultRegion,
}
users, err := d.ListUsersForTenant(ctx, ref)
```

Cross-tenant reads (e.g., integrator staff browsing a managed customer) are
**not** supported through these helpers. They go through the scoped-token
flow implemented in KAI-224 (cross-tenant access) using Casbin subjects with
`integrator:` / `federation:` prefixes. The helpers here are the inner core
that always runs after authorization.

## Coordination with sibling tickets

- **KAI-225** (Casbin policy store) — references `integrators`,
  `customer_tenants`, `users` by the column names declared in migrations
  0001/0002/0004. If you rename a column, update KAI-225's policy builder in
  the same PR.
- **KAI-233** (audit log service) — owns the writer, partition management
  (`pg_partman`), and retention policy for `audit_log_partitioned`. This
  package only declares the parent table shape in migration 0006 so sibling
  work can compile and reference the column names.
- **KAI-216** (RDS provisioning) — brings up the real Postgres, at which
  point the chaos isolation test (KAI-235) starts exercising the full schema
  with JSONB + partitioning in CI.

## Seam checklist

- [x] Seam #4 (multi-tenant isolation): every tenant-scoped helper takes a
      `TenantRef` with `Type`, `ID`, and `Region`. Validation runs before any
      SQL is issued.
- [x] Seam #9 (multi-region ready): `region` column on every tenant-scoped
      table; composite indexes on `(region, ...)`; `DefaultRegion =
      us-east-2`; wrong-region reads return no rows.
- [x] No `mediamtx.yml` runtime settings touched.
- [x] No RDS, EKS, or other AWS resources provisioned.
