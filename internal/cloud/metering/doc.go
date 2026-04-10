// Package metering records per-tenant usage events and rolls them up into
// aggregates that billing (KAI-361 Stripe Connect) and the customer billing
// portal (KAI-367) consume.
//
// # Two-table design
//
// The metering package owns two tables (migration 0016):
//
//   - usage_events      Append-only, one row per Record call. In production
//                       (Postgres) partitioned monthly by pg_partman; in
//                       tests (SQLite) a flat table with the same columns.
//                       This is the source of truth.
//   - usage_aggregates  Rollup table written by Aggregator.Run. Stores
//                       (tenant_id, period_start, period_end, metric) with
//                       sum/max/snapshot_count. Customer billing exports
//                       and Stripe usage reporters read from this table.
//
// # Tenant scoping (Seam #4)
//
// Every read and every write is keyed on tenant_id. There is no "all tenants"
// query surface in this package. Record requires a non-empty TenantID. Store
// queries always bind tenant_id as the first WHERE predicate. Aggregator.Run
// iterates tenants one at a time and the rollup INSERTs always carry
// tenant_id.
//
// # Metric vocabulary
//
// The v1 metric set is intentionally small. Billing rules in KAI-361 use
// these three metric names:
//
//   - MetricCameraHours       Sum of camera-online-hours in the period.
//   - MetricRecordingBytes    Sum of bytes written to the archive.
//   - MetricAIInferenceCount  Count of AI inference calls billed per-unit.
//
// New metrics must be added to the AllMetrics list and the drift-guard test
// so billing and the reporter stay in sync.
//
// # Usage in the hot path
//
// Record is designed to be called from the Recorder → Directory ingest
// pipeline (KAI-254). It does a single parameterised INSERT. It does not
// buffer, does not retry, and does not fail open: the caller is responsible
// for deciding whether a metering failure should propagate or be logged.
//
// # Aggregator
//
// Aggregator.Run is idempotent for a given (tenant, period_start, metric)
// triple thanks to an ON CONFLICT DO UPDATE clause. KAI-232 (CI/CD) will
// schedule this as a nightly Kubernetes CronJob; for now it is a callable
// function exercised by tests.
//
// # UsageReporter
//
// The UsageReporter interface is the seam between this package and the
// KAI-361 Stripe Connect integration. metering knows nothing about Stripe
// (no imports). The Stripe adapter lives in internal/cloud/billing/stripe/
// and imports this package, not the other way around.
package metering
