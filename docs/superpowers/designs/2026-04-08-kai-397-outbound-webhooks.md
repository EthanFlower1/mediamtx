# KAI-397 — Outbound Webhooks

**Status:** DRAFT (approved for inline drafting by CTO 2026-04-08; subject to lead-security follow-up review)
**Owner:** lead-devex
**Related:** KAI-398 (inbound webhooks), KAI-399 (Public API), KAI-400 (API key management)
**Seams touched:** #2 (proto-first contracts), #4 (multi-tenant isolation), #10 (tenant_id on every cloud table)

## 1. Goals

Provide a first-class outbound webhook system so tenants can subscribe to Kaivue events (camera online/offline, motion, face match, recording milestone, retention purge, access-control action, billing update, etc.) and have Kaivue POST JSON payloads to their HTTP(S) endpoints with:

- **At-least-once** delivery with idempotency support so consumers can dedupe safely.
- **Cryptographic authenticity** via per-subscription HMAC-SHA256 signatures.
- **Operational safety**: retry with exponential backoff, dead-letter queue on exhaustion, rate-limits per destination, full delivery log for audit + replay.
- **Zero-downtime secret rotation**.
- **Multi-tenant isolation** — a webhook belongs to exactly one tenant; delivery workers never cross the tenant boundary.

## 2. Non-goals

- Inbound webhooks (KAI-398 covers those).
- Streaming/long-lived webhook transports (websockets, SSE) — out of scope for v1.
- Complex filtering DSLs — v1 supports event-type subscription + simple attribute equality (e.g. `camera_id=X`). Richer filters deferred.
- Consumer-provided response-body handling — Kaivue ignores the response body other than status code.

## 3. Data model

All tables carry `tenant_id` and are enforced by per-query tenant filters (seam #4, seam #10).

### 3.1 `webhook_subscriptions`

| column | type | notes |
|---|---|---|
| `id` | `uuid` | primary key |
| `tenant_id` | `uuid` | FK tenants.id, indexed |
| `url` | `text` | validated: https only in prod, http allowed in dev |
| `event_types` | `text[]` | e.g. `{camera.online, motion.detected}` — empty array = all |
| `filter` | `jsonb` | optional simple equality filter, e.g. `{"camera_id": "..."}` |
| `status` | `enum('active','paused','disabled')` | auto-paused by circuit breaker |
| `description` | `text` | tenant-facing label |
| `created_at` / `updated_at` / `created_by` | | audit cols |
| `last_success_at` / `last_failure_at` | `timestamptz` | |
| `consecutive_failures` | `int` | drives circuit breaker |

Rate-limit / backoff tuning is per-subscription and inherits tenant defaults unless overridden.

### 3.2 `webhook_signing_secrets`

Supports zero-downtime rotation via overlap window.

| column | type | notes |
|---|---|---|
| `id` | `uuid` | primary key |
| `subscription_id` | `uuid` | FK |
| `tenant_id` | `uuid` | denorm for tenant filters |
| `secret_ciphertext` | `bytea` | encrypted at rest via cryptostore (KAI-251) |
| `key_id` | `text` | short public identifier used in signature header |
| `status` | `enum('primary','secondary','retired')` | |
| `created_at` / `activated_at` / `retired_at` | | |

- Exactly one `primary` at a time.
- During rotation, operator creates a new secret as `secondary`. Signatures are computed with BOTH the `primary` and `secondary` and delivered together on every event. Consumer validates against either. After the overlap window (default 24h; configurable), operator promotes `secondary`→`primary` and retires the old `primary`.
- Retired secrets kept 30 days for audit/debug, then purged.

### 3.3 `webhook_events`

Immutable record of every event that was emitted into the webhook fan-out system. This is the canonical "what we tried to deliver" log.

| column | type | notes |
|---|---|---|
| `id` | `uuid` | primary key — **becomes the `idempotency-key`** for consumers |
| `tenant_id` | `uuid` | |
| `event_type` | `text` | |
| `occurred_at` | `timestamptz` | when the underlying real-world event happened |
| `enqueued_at` | `timestamptz` | when it entered the webhook fan-out |
| `payload` | `jsonb` | the canonical raw event body (proto-serialized to JSON) |
| `source` | `text` | e.g. `recorder/abc123`, `directory`, `billing` |
| `correlation_id` | `text` | propagated request ID when available |

### 3.4 `webhook_deliveries`

One row per subscription per event. Drives retries.

| column | type | notes |
|---|---|---|
| `id` | `uuid` | primary key |
| `tenant_id` | `uuid` | |
| `event_id` | `uuid` | FK webhook_events |
| `subscription_id` | `uuid` | FK webhook_subscriptions |
| `attempt` | `int` | 1-based |
| `max_attempts` | `int` | default 14 (see §5) |
| `next_attempt_at` | `timestamptz` | scheduler polls `WHERE status='pending' AND next_attempt_at<=now()` |
| `status` | `enum('pending','in_flight','succeeded','failed','dlq')` | |
| `response_status` | `int` | HTTP status of last attempt |
| `response_headers` | `jsonb` | truncated |
| `response_body_snippet` | `text` | first 2 KiB |
| `error` | `text` | DNS/TLS/connect/timeout errors |
| `duration_ms` | `int` | of last attempt |
| `delivered_at` | `timestamptz` | nullable |
| `dlq_reason` | `text` | nullable |

Retention: succeeded deliveries kept 30 days, failed/dlq kept 90 days, then archived to R2 per retention policy.

## 4. Delivery semantics — at-least-once + idempotency

**Delivery guarantee:** at-least-once. Transient failures retry; successful 2xx responses mark the delivery `succeeded`.

**Idempotency key:** every delivery includes HTTP header

```
Idempotency-Key: whk_evt_<webhook_events.id>
```

Consumers MUST dedupe on this key. Kaivue will also **echo the idempotency key back to the caller** on any consumer-initiated replay request (see §9) so operators can reason about retries end-to-end.

**Ordering guarantee:** NONE by default. Events may arrive out of order. Subscriptions may opt in to per-key ordered delivery in v2 (deferred).

**Success condition:** HTTP 2xx response within the timeout (default 10s, configurable 1–30s). Anything else → retry.

**Permanent-fail short-circuit:** HTTP 410 Gone OR HTTP 404 on an endpoint that previously returned 2xx for >7 days → subscription auto-paused and an alert posted to the tenant admin.

## 5. Retry schedule

Exponential backoff with jitter, starting at 1s, doubling, capped at 4h between attempts, with a 24h **total elapsed** budget from `enqueued_at`.

Default schedule (14 attempts total):

| attempt | delay-before | cumulative |
|---|---|---|
| 1 | 0s | 0s |
| 2 | 1s | 1s |
| 3 | 2s | 3s |
| 4 | 4s | 7s |
| 5 | 8s | 15s |
| 6 | 16s | 31s |
| 7 | 32s | 1m03s |
| 8 | 64s | 2m07s |
| 9 | 128s | 4m15s |
| 10 | 256s | 8m31s |
| 11 | 512s | 17m03s |
| 12 | 1024s | 34m07s |
| 13 | 2048s | 1h08m15s |
| 14 | 4096s (capped 4h) | ~2h08m |

Scheduler continues retrying at the 4h cap until `now() - enqueued_at > 24h`, at which point the delivery is moved to `dlq`. Random jitter ±20% applied to every delay to avoid thundering-herd on shared endpoints.

**DLQ:** `status='dlq'` deliveries are retained, visible in the Customer Admin UI (KAI-323/326 etc.), and manually replayable via API for 90 days. After 90 days they are archived + metadata kept for audit; payloads dropped.

## 6. Signature format

Header (per active signing secret):

```
Kaivue-Signature: t=<unix-seconds>,v1=<hex-hmac-sha256>,kid=<key_id>
```

Multiple `Kaivue-Signature` headers MAY appear (one per active secret during rotation overlap). Consumers MUST accept any that validates.

**Signed payload (canonical string):**

```
<t>.<raw_body_bytes>
```

where `<t>` is the same unix-seconds as the header and `<raw_body_bytes>` is the EXACT bytes of the HTTP request body (no re-serialization). HMAC-SHA256 is computed over this string using the signing secret, hex-encoded lowercase.

**Consumer verification steps (documented for developers):**

1. Read the `Kaivue-Signature` header(s).
2. Extract `t` and `v1` values.
3. Reject if `|now - t| > 300s` (outbound tolerance; see §7).
4. Recompute HMAC-SHA256(`<t>.<raw_body>`, signing_secret).
5. Compare with `v1` in constant time.
6. If any header validates, request is authentic.

Replay protection at the consumer side is the 300s window + idempotency-key dedupe.

## 7. Timestamp tolerance

- **Outbound (what we stamp, what consumers tolerate):** 300s. Our signature is valid if the consumer's clock is within ±300s of ours. Conservative to survive NTP skew on edge consumers.
- **Inbound (KAI-398, separate doc):** 60s. Strict rejection of old replays.

These are **defaults**. Tenants on regulated plans can opt into tighter windows.

## 8. Circuit breaker

Per subscription, sliding window:

- >= 20 consecutive failures OR >= 80% failure rate over last 100 attempts → subscription auto-`paused`.
- Paused subscriptions do not receive new deliveries; new events are still recorded in `webhook_events` but no `webhook_deliveries` rows are created for the paused sub.
- Tenant admin gets a notification (KAI notifications stack).
- Manual resume via API or UI.

## 9. Operator operations

### 9.1 Replay

```
POST /v1/webhooks/deliveries/{delivery_id}/replay
```

Creates a new `webhook_deliveries` row pointing at the same `event_id` with `attempt=1`. Original row is preserved. Replayed delivery includes the same `Idempotency-Key`, so well-behaved consumers will dedupe.

### 9.2 Event replay (bulk)

```
POST /v1/webhooks/events/replay
{
  "filter": { "event_type": "camera.online", "from": "...", "to": "..." },
  "subscription_ids": ["..."]
}
```

Creates fresh `webhook_deliveries` rows; useful after fixing a broken consumer.

### 9.3 Preview / test-fire

```
POST /v1/webhooks/subscriptions/{id}/test
```

Fires a synthetic `webhook.test` event; goes through the full delivery pipeline so the tenant can verify signature + endpoint without waiting for a real event.

## 10. Multi-tenant isolation

- Every `webhook_*` table has `tenant_id`; every query is filtered by tenant (Casbin policy + row-level enforcement via shared tenant-filter wrapper, seam #4).
- Delivery workers pull from a per-tenant queue shard (see §11) so a noisy tenant cannot monopolize shared workers.
- The HTTP client used to call out is isolated per tenant with a per-tenant outbound egress budget (RPS + concurrency). Default: 100 RPS, 64 concurrent.

## 11. Queue + scheduler implementation

v1 implementation: **Postgres-backed work queue**, not an external broker. Reasons:

1. Simpler ops story for self-hosted / on-prem Directory deployments that don't want to run SQS/Redis.
2. Transactional semantics: event insert + delivery insert in one tx guarantees we don't lose events on crash.
3. Tenant isolation is easy with WHERE tenant_id=X + LIMIT.

**Scheduler loop:**

```sql
WITH claimed AS (
  SELECT id FROM webhook_deliveries
  WHERE status = 'pending' AND next_attempt_at <= now()
  ORDER BY next_attempt_at
  LIMIT $batch
  FOR UPDATE SKIP LOCKED
)
UPDATE webhook_deliveries d
   SET status='in_flight', attempt = attempt + 1
 FROM claimed c
 WHERE d.id = c.id
 RETURNING d.*;
```

`FOR UPDATE SKIP LOCKED` → multiple workers can run in parallel safely.

**v2 upgrade path:** if Postgres queue becomes a hotspot, migrate cloud deployments to SQS/Redis-Streams behind the same interface. On-prem stays on Postgres.

## 12. Metrics

Prometheus labels: `tenant_id` (bucketed/cardinality-capped), `event_type`, `status`.

- `kaivue_webhook_deliveries_total{status}` — counter
- `kaivue_webhook_delivery_duration_seconds` — histogram
- `kaivue_webhook_queue_depth{status}` — gauge
- `kaivue_webhook_dlq_total` — counter
- `kaivue_webhook_subscriptions_paused_total` — counter
- `kaivue_webhook_signature_rotations_total` — counter

Alerts:
- DLQ rate > 1% over 15m.
- Queue depth > 10k sustained 5m.
- Scheduler lag (now − min(pending.next_attempt_at)) > 60s sustained 2m.

## 13. Audit + compliance

- Every create/update/rotate/delete on `webhook_subscriptions` + `webhook_signing_secrets` writes an audit-log entry (KAI-233).
- Delivery payloads may contain PII — retention rules follow the tenant's data retention policy (HIPAA/GDPR, seam routed via lead-security).
- `webhook_signing_secrets.secret_ciphertext` encrypted at rest via the cryptostore (KAI-251).
- Signature verification + HMAC rotation design routed to lead-security for review before merge.

## 14. Public API surface (proto-first)

Proto lives in `proto/kaivue/webhooks/v1/webhooks.proto`. Generated via buf → Connect-Go + REST gateway.

```
service WebhookService {
  rpc CreateSubscription(CreateSubscriptionRequest) returns (Subscription);
  rpc GetSubscription(GetSubscriptionRequest) returns (Subscription);
  rpc ListSubscriptions(ListSubscriptionsRequest) returns (ListSubscriptionsResponse);
  rpc UpdateSubscription(UpdateSubscriptionRequest) returns (Subscription);
  rpc DeleteSubscription(DeleteSubscriptionRequest) returns (google.protobuf.Empty);

  rpc RotateSigningSecret(RotateSigningSecretRequest) returns (RotateSigningSecretResponse);
  rpc RetireSigningSecret(RetireSigningSecretRequest) returns (google.protobuf.Empty);

  rpc TestFire(TestFireRequest) returns (TestFireResponse);
  rpc ReplayDelivery(ReplayDeliveryRequest) returns (Delivery);
  rpc ReplayEvents(ReplayEventsRequest) returns (ReplayEventsResponse);

  rpc ListDeliveries(ListDeliveriesRequest) returns (ListDeliveriesResponse);
  rpc GetDelivery(GetDeliveryRequest) returns (Delivery);
}
```

OpenAPI is generated from proto via the same buf pipeline that powers KAI-399 — no hand-maintained API spec.

## 15. Packaging / ownership

- `internal/cloud/webhooks/` — cloud package (delivery workers, scheduler, DB, HTTP client, metrics, signing).
- `internal/shared/webhooks/signing/` — HMAC signing primitives; consumed by both outbound (KAI-397) and inbound verification of OUR own signatures if ever loopback tested.
- `proto/kaivue/webhooks/v1/` — proto contracts.
- `pkg/api/webhooks/` — public client wrappers used by SDKs (KAI-401).

No dependency from on-prem Directory into cloud webhook delivery workers; the on-prem flavor emits events into a local Postgres webhook_events table and its own scheduler runs the delivery loop, but the SAME package compiles both.

## 16. Open questions (to lead-security)

1. Do we need to ship a `v2=<ed25519-sig>` alongside `v1=<hmac>` for FedRAMP / FIPS tenants in v1, or is hmac-sha256 sufficient for GA?
2. Required minimum overlap window during rotation for regulated tenants? (Default 24h proposed.)
3. Is 300s outbound tolerance acceptable, or must regulated tenants be forced to ≤60s?
4. HIPAA posture: do we need to strip/hash identifiable fields from DLQ-archived payloads after 30 days, or is encrypted-at-rest + audit log sufficient?

## 17. Open questions (to lead-cloud)

1. Per-tenant egress budget: 100 RPS / 64 concurrent — reasonable default given expected v1 customer sizes?
2. Postgres-backed queue vs SQS for cloud path — confirm OK for v1 GA.

## 18. Rollout plan

1. Proto + DB migrations (behind feature flag `webhooks.outbound.enabled`).
2. Signer + HMAC primitives + unit tests.
3. Scheduler + worker loop + Postgres-backed queue + integration tests.
4. Public API surface wired into KAI-399 (blocks on KAI-399 Connect-Go service mount).
5. Customer Admin UI (delegated to lead-web).
6. Dogfood on staging for 7 days with synthetic events.
7. Enable for early-access tenants.
8. GA.
