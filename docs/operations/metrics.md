# Prometheus Metrics — Scrape Setup

## Overview

Every Kaivue component exposes Prometheus metrics on a **dedicated admin listener** that is separate from the main API port. The admin listener is never placed behind TLS or authentication — it is designed to be reachable only from within the trusted LAN or internal network.

| Component | Default admin address | Path |
|---|---|---|
| Cloud API server | `:9090` | `/metrics` |
| Directory (on-prem) | `:9091` (planned v1.x) | `/metrics` |
| Recorder (on-prem) | `:9092` (planned v1.x) | `/metrics` |

The port is configurable via `MetricsListenAddr` in the component's `Config` struct.

## On-prem customer scrape

Customers running their own Prometheus instance on the same LAN as the Kaivue Directory can scrape directly:

```yaml
# prometheus.yml

scrape_configs:
  - job_name: kaivue-directory
    static_configs:
      - targets: ["<directory-host>:9091"]
    scrape_interval: 30s
    scrape_timeout: 10s

  - job_name: kaivue-recorder
    static_configs:
      - targets: ["<recorder-host>:9092"]
    scrape_interval: 30s
    scrape_timeout: 10s
```

Because the `/metrics` endpoint is on the internal admin listener (not the public API port), no `Authorization` header is required. Do not expose the admin listener on a public interface.

## Cloud scrape (Kaivue SRE)

The cloud API server admin listener is accessible within the VPC only (security group restricts inbound on `:9090` to the internal Prometheus scraper). No customer action is required.

## Standard metric names

All metrics use the `kaivue_` prefix. Stable names (do not rename after v1.0):

| Metric | Type | Labels |
|---|---|---|
| `kaivue_requests_total` | counter | `component`, `method`, `route`, `code` |
| `kaivue_request_duration_seconds` | histogram | `component`, `route` |
| `kaivue_errors_total` | counter | `component`, `code` |
| `kaivue_build_info` | gauge (=1) | `version`, `commit`, `goversion` |
| `kaivue_goroutines` | gauge | — |
| `kaivue_memory_bytes` | gauge | `type` |
| `kaivue_recordercontrol_connected` | gauge | `recorder_id` |
| `kaivue_recordercontrol_events_applied_total` | counter | `event_type` |
| `kaivue_recordercontrol_reconnects_total` | counter | `reason` |
| `kaivue_directoryingest_messages_total` | counter | `stream`, `result` |
| `kaivue_directoryingest_backpressure_drops_total` | counter | — |
| `kaivue_streams_minted_total` | counter | `kind`, `protocol`, `result` |
| `kaivue_streams_ttl_seconds` | histogram | — |
| `kaivue_certmgr_cert_expires_at` | gauge | — |
| `kaivue_certmgr_renewals_total` | counter | `result` |
| `kaivue_certmgr_reenrollments_total` | counter | `result` |
| `kaivue_relationships_granted_total` | counter | — |
| `kaivue_relationships_revoked_total` | counter | — |

## Multi-tenant label discipline

Any metric that counts activity scoped to a single tenant **must** carry a `tenant_id` label. Reviewers will reject PRs that touch tenant data without this label. Use `sum by (tenant_id)` in Prometheus queries to isolate per-tenant values — cross-tenant leakage is prevented by the label dimension, not by separate registries.

## Fail-open policy

Metric writes that encounter an error (label cardinality violation, registry conflict) are silently recovered. A metric write failure must never block or fail a request. This is enforced in `internal/shared/metrics/middleware.go` via panic recovery in `safeInc` and `safeObserve`.

## Grafana dashboards

Pre-built dashboards are in `dashboards/`. See `dashboards/README.md` for import instructions.
