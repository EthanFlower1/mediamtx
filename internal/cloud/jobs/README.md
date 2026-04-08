# cloud/jobs — KAI-234

Background job system for the Kaivue / MediaMTX cloud control plane.

`JobEnqueuer` + `JobRunner` interfaces with two backends:

- **MemoryEnqueuer** — in-process, used by unit tests, local dev, and
  any path that needs deterministic, synchronous execution of jobs
  (fake clock, no Postgres).
- **RiverEnqueuer** — wraps `github.com/riverqueue/river`, gated
  behind the `river` build tag. In the default build it is a stub
  that errors on every call so the package compiles without the real
  dependency. Flip `-tags river` and vendor river + a pgx pool once
  Postgres is available to the cloud control plane.

The package is the canonical place for every async operation in the
cloud half of the system. If you're on the verge of spawning a
goroutine from an HTTP handler, stop and enqueue a job instead.

## The non-negotiable contract

> **Every worker MUST verify the tenant id on its payload before
> touching any state.**

This is Seam #4 (multi-tenant isolation). The runner does the first
line of defence by calling `TenantVerifier.KnownTenant` — a job whose
payload carries an unknown tenant id is shunted directly to the DLQ
without ever reaching the worker. `TestCrossTenantIsolation` is the
living proof.

Workers themselves must still scope every DB query / API call by
`payload.TenantID()`. Never pull the tenant id from context, config,
or request headers inside a worker — read it from the payload, the
same field the runner validated.

## Job kinds

| Kind                                 | Payload                            | Real impl ticket | Retry override                 |
|--------------------------------------|------------------------------------|------------------|--------------------------------|
| `tenant.welcome_email`               | `TenantWelcomeEmailPayload`        | KAI-371          | 5 attempts, 2s → 1m            |
| `tenant.bootstrap_stripe`            | `TenantBootstrapStripePayload`     | KAI-361          | 6 attempts, 5s → 10m           |
| `tenant.bootstrap_zitadel`           | `TenantBootstrapZitadelPayload`    | KAI-223          | 6 attempts, 5s → 10m           |
| `bulk.push_config`                   | `BulkPushConfigPayload`            | KAI-343          | default (5 attempts, 1s → 5m)  |
| `cloud_archive.upload_trigger`       | `CloudArchiveUploadTriggerPayload` | KAI-258          | default                        |
| `billing.monthly_rollup`             | `BillingMonthlyRollupPayload`      | KAI-363          | 3 attempts, 10s → 5m           |
| `audit.partition_create_next_month`  | `AuditPartitionCreateNextPayload`  | KAI-218          | 3 attempts, 10s → 5m           |
| `audit.drop_expired_partitions`      | `AuditDropExpiredPayload`          | KAI-218          | 3 attempts, 10s → 5m           |

All payloads implement `jobs.TenantScoped` (one method, `TenantID()
string`). The two `audit.*` jobs operate on shared infrastructure, so
they carry the pseudo-tenant `jobs.SystemTenant = "__system__"` — your
tenant verifier must register that id for the runner to dispatch them.

## Retry policy

Exponential backoff with a per-kind cap.

```go
type RetryPolicy struct {
    MaxAttempts int
    BaseDelay   time.Duration
    MaxDelay    time.Duration
    Multiplier  float64
}
```

- `DefaultRetry` is 5 attempts, `1s` base, `5m` cap, `2x` growth.
- Per-kind overrides live in `kindPolicies` (see `jobs.go`).
- `EnqueueOptions.MaxAttempts` on a single enqueue wins over the
  table.
- After `MaxAttempts`, the job moves to `StateDLQ` and the DLQ
  counter increments. It is **not** retried again — a human (or a
  follow-up ticket) has to decide.

## Idempotency

Every enqueue call accepts `EnqueueOptions.IdempotencyKey`. If the key
is non-empty and a previous enqueue already used it, the existing
`*Job` is returned and the `jobs_dropped` counter increments. This is
the webhook-replay story: Stripe retrying the same event id must not
trigger N welcome emails.

For this to work the enqueuer has to see both attempts (same process
for `MemoryEnqueuer`, same Postgres table for `RiverEnqueuer`). If
you're behind a load balancer with real River this is fine — River's
unique-job machinery backs the same key.

## Metrics

`MemoryEnqueuer.StatsPtr()` returns a `*Stats` with atomic counters:

```
jobs_enqueued
jobs_succeeded
jobs_failed   # individual attempt failures
jobs_retried  # attempts that were scheduled for another try
jobs_dlq      # jobs parked in DLQ (permanent failures)
jobs_dropped  # duplicate idempotency hits
```

`Stats.Snapshot()` returns a `map[string]int64` suitable for logging
via `internal/shared/logging` fields. When `internal/shared/metrics`
(Prometheus) lands, wire these into counters with a `kind` label.

## Graceful shutdown

```go
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()
if err := runner.Shutdown(ctx); err != nil { ... }
```

`Shutdown` marks the runner as closed (rejecting new enqueues with
`ErrShuttingDown`) and blocks until every in-flight worker returns or
the context deadline fires. Pending-but-not-yet-started jobs remain in
the queue for the next process to pick up.

## Wiring example

```go
log := logging.New(logging.Options{Component: "jobs"})

verifier := jobs.NewMemoryTenantSet("tenant-a", "tenant-b", jobs.SystemTenant)

m := jobs.NewMemoryEnqueuer(jobs.MemoryConfig{
    TenantVerifier: verifier,
})
if err := m.RegisterAll(jobs.DefaultWorkers(log)...); err != nil {
    return err
}

var enq jobs.JobEnqueuer = m // hand to KAI-227 (tenant provisioning)
```

## Swapping to River

When KAI-216 RDS + the real river dependency are ready:

1. `go get github.com/riverqueue/river github.com/riverqueue/river/riverdriver/riverpgxv5`
2. Fill in `river_real.go` — build with `-tags river`.
3. Change the construction site from `NewMemoryEnqueuer` to
   `NewRiverEnqueuer(...)` — the consuming code does not change
   because everything talks to the `JobEnqueuer` / `JobRunner`
   interfaces.
4. Keep `MemoryEnqueuer` as the default in tests. The chaos test at
   `TestCrossTenantIsolation` runs against whichever backend you
   configure; both MUST pass.

## Tests

```
$ go test ./internal/cloud/jobs/...
```

Covers: sync run, per-kind routing for all 8 seeded kinds, exponential
backoff with a fake clock, DLQ after max attempts, idempotency,
cross-tenant chaos, graceful shutdown draining in-flight work,
unknown-kind typed errors, `DefaultWorkers` registration, backoff
curve unit, and the River stub rejection contract.
