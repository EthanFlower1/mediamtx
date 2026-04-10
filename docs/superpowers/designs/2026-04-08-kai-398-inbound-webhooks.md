# KAI-398 — Inbound Webhooks (Action Triggers)

**Status:** DRAFT (inline parallel work, pending lead-security review per task #159)
**Owner:** lead-devex
**Related:** KAI-397 (outbound webhooks), KAI-399 (Public API), KAI-400 (API keys), KAI-145/146/148 (Casbin policy + API enforcement)
**Seams touched:** #2 (proto-first), #3 (IdentityProvider firewall), #4 (multi-tenant isolation), #10 (tenant_id on every table)

## 1. Goals

Let external systems POST signed events into Kaivue to trigger in-product actions: start a recording, add a bookmark, arm/disarm a zone, raise an event to an operator, fire a notification, write an audit log, run a named Kaivue "action" (scoped automation).

The endpoint must be:
- **Authenticated** by signature (HMAC-SHA256, per-tenant + per-subscription secret) — not by bearer API key. (Bearer-API-key inbound is KAI-399; this is the **signed-webhook** flavor designed for untrusted transports like public SaaS "outgoing webhook" configurators.)
- **Idempotent** (honor `Idempotency-Key`) so the caller's retries don't cause double-fire.
- **Replay-protected** with a tight timestamp window (60s default).
- **Tenant-isolated** — every inbound webhook belongs to exactly one tenant, and the handler runs in that tenant's context with normal Casbin enforcement.
- **Action-scoped** — a webhook can only fire the actions its subscription was granted. You can't create one webhook and use it to do everything.

## 2. Non-goals

- Free-form RPC over webhooks. If the caller wants the Public API, they use a bearer API key and the Connect-Go service (KAI-399). Inbound webhooks are strictly "fire a named action with a small payload."
- Long-running responses / streaming. Handler returns <= 30s.
- WebSub / PubSubHubbub / feed polling.

## 3. Endpoint

```
POST /v1/webhooks/inbound/{subscription_id}
Content-Type: application/json
Kaivue-Signature: t=<unix>,v1=<hex>,kid=<key_id>
Idempotency-Key: <caller-provided opaque string, 1..=128 chars>

{
  "action": "camera.bookmark.create",
  "params": { ... }
}
```

`{subscription_id}` is a short opaque id (`iwh_...`), generated at subscription creation. It is NOT secret by itself — it's just a routing handle. Authentication is the signature.

## 4. Data model

All tables carry `tenant_id`.

### 4.1 `inbound_webhook_subscriptions`

| column | type | notes |
|---|---|---|
| `id` | `text` | `iwh_...` short id |
| `tenant_id` | `uuid` | FK |
| `allowed_actions` | `text[]` | enumerated action names; empty = reject all |
| `allowed_source_ips` | `cidr[]` | optional allowlist; empty = any |
| `description` | `text` | |
| `status` | `enum('active','paused','disabled')` | |
| `created_at` / `updated_at` / `created_by` | audit | |
| `last_success_at` / `last_failure_at` | | |

### 4.2 `inbound_webhook_signing_secrets`

Same structure as outbound (KAI-397 §3.2) — `primary`/`secondary`/`retired` with rotation overlap. Reuse the `internal/shared/webhooks/signing/` package.

### 4.3 `inbound_webhook_deliveries` (for audit + idempotency)

| column | type | notes |
|---|---|---|
| `id` | `uuid` | pk |
| `tenant_id` | `uuid` | |
| `subscription_id` | `text` | FK |
| `idempotency_key` | `text` | |
| `received_at` | `timestamptz` | |
| `source_ip` | `inet` | |
| `signature_kid` | `text` | which key signed it |
| `signature_timestamp` | `timestamptz` | parsed from header |
| `payload_hash` | `bytea` | sha256(raw body) — for audit, not for signing |
| `action` | `text` | |
| `result` | `enum('accepted','rejected','duplicate','unauthorized','replay','rate_limited','action_failed')` | |
| `http_status_returned` | `int` | |
| `action_run_id` | `uuid` | nullable FK into an action_runs table |
| `error` | `text` | nullable |

UNIQUE `(subscription_id, idempotency_key)` where `result='accepted'` — enforces idempotency.

## 5. Signature verification (strict)

Rejection order (fail fast, return the SAME generic 401 for all auth-related rejections to avoid oracle leaks):

1. Parse `Kaivue-Signature` header. If absent or malformed → 401.
2. Extract `t` (unix seconds), `v1`, `kid`. Reject if any missing.
3. Check timestamp window: `|now - t| <= 60s`. Otherwise → 401 (`result=replay`).
4. Lookup subscription by `{subscription_id}`. If missing or `status != active` → 401.
5. Lookup signing secret by `(subscription_id, kid)`. Accept if secret is `primary` or `secondary` (during rotation overlap). Retired → 401.
6. Read raw request body (NO re-parse at this point). Recompute `HMAC-SHA256(t + "." + raw_body, secret)` in hex lowercase. Constant-time compare to `v1`. Mismatch → 401.
7. Check source-IP allowlist if configured. No match → 401.

Return 401 for all of the above. Log the true `result` internally for audit. External caller only sees the 401 + a generic `{"error":"unauthorized"}`.

**Why strict:** inbound webhook endpoints are public internet. Any leak about why auth failed is a signal to an attacker. Distinct error codes for unauth vs. replay vs. missing sub are all convergent anti-patterns.

## 6. Idempotency

After successful signature validation, before dispatching the action:

```sql
INSERT INTO inbound_webhook_deliveries
   (id, tenant_id, subscription_id, idempotency_key, ...)
VALUES (...)
ON CONFLICT (subscription_id, idempotency_key) WHERE result='accepted'
  DO NOTHING
RETURNING id;
```

If the INSERT returned no row → it's a duplicate. Look up the existing `accepted` row, return `200 OK` with the original `action_run_id` + cached body. Caller sees exactly the same response as the first time.

If the INSERT returned a row → this is the first time, dispatch the action and fill `result` + `action_run_id` when done.

`Idempotency-Key` is REQUIRED. Missing header → 400. Recommended format: caller's own event UUID.

## 7. Action dispatch

Inbound webhooks never execute code directly — they enqueue an `action_run` on the tenant's action queue. This gives us:

- Sync-only semantics for the webhook response (we return once the action is **queued**, not once it completes, with one exception noted below).
- Fan-out + audit via the same mechanism as UI-initiated actions.
- Clean Casbin enforcement: the action run inherits the subscription's allowed actions and runs under a synthetic "webhook principal" identity.

Synthetic principal: `webhook:<subscription_id>`. Casbin grants are computed at subscription-create time from `allowed_actions`. The webhook principal is a first-class subject in the policy model.

**Exception — fast-path actions:** a small allowlist of always-cheap actions (`camera.bookmark.create`, `event.annotate`, `notification.send_custom`) MAY be executed inline if completion is <200ms, and the response body includes the action's result. These are configured in code, not per-tenant.

For everything else, the response is:

```json
HTTP/1.1 202 Accepted
{
  "action_run_id": "arn_...",
  "status": "queued",
  "poll_url": "/v1/actions/runs/arn_..."
}
```

## 8. Rate limits

Per-subscription sliding-window: default **60 requests/minute** + burst 20. Per-tenant aggregate: default **600 requests/minute**. Both configurable by tenant plan.

Over limit → 429 + `Retry-After`. Logged as `result=rate_limited`.

## 9. Multi-tenant isolation

- Subscription → tenant mapping resolved before any handler runs.
- The dispatched action inherits the tenant context. No code path reads another tenant's state.
- Rate limiter is partitioned by tenant.
- Audit log rows go into the per-tenant audit stream.

## 10. Action catalog (v1)

Enumerated action names understood by the inbound webhook dispatcher:

| action | description | sync fast-path? |
|---|---|---|
| `camera.bookmark.create` | Bookmark a moment on a camera | yes |
| `camera.recording.start` | Start on-demand recording | no |
| `camera.recording.stop` | Stop on-demand recording | no |
| `camera.ptz.goto_preset` | Move PTZ to named preset | no |
| `event.annotate` | Add note/tag to an existing event | yes |
| `notification.send_custom` | Send a tenant-scoped notification (email/push/in-app) | yes |
| `action.run` | Run a named Kaivue Action (user-defined automation) | no |
| `alarm.raise` | Raise an alarm to operators | no |
| `alarm.clear` | Clear an alarm | no |

New actions added via proto changes to the action service (separate ticket).

## 11. Public API surface (for webhook management)

```
service InboundWebhookService {
  rpc CreateSubscription(CreateSubscriptionRequest) returns (Subscription);
  rpc GetSubscription(GetSubscriptionRequest) returns (Subscription);
  rpc ListSubscriptions(ListSubscriptionsRequest) returns (ListSubscriptionsResponse);
  rpc UpdateSubscription(UpdateSubscriptionRequest) returns (Subscription);
  rpc DeleteSubscription(DeleteSubscriptionRequest) returns (google.protobuf.Empty);

  rpc RotateSigningSecret(RotateSigningSecretRequest) returns (RotateSigningSecretResponse);
  rpc RetireSigningSecret(RetireSigningSecretRequest) returns (google.protobuf.Empty);

  rpc ListDeliveries(ListDeliveriesRequest) returns (ListDeliveriesResponse);
  rpc GetDelivery(GetDeliveryRequest) returns (Delivery);
}
```

Note: the **inbound POST endpoint itself** is NOT a Connect-Go service — it's a plain HTTP handler because callers are third-party systems that only speak HTTP+JSON webhooks. Management of webhooks IS Connect-Go.

## 12. Audit + compliance

- Every delivery (accepted OR rejected) writes a row to `inbound_webhook_deliveries` + an audit-log entry (KAI-233).
- Rejected rows are kept 30 days for abuse investigation, then purged.
- Source IP + User-Agent recorded for every request.
- Replay attempts surface to the tenant admin UI (KAI-324 events page) with a "suspicious" flag.
- Auto-pause subscription after N replay/unauthorized attempts in M minutes (default: 20 in 5 min).

## 13. Packaging

- `internal/cloud/webhooks/inbound/` — HTTP handler, signature verifier, rate limiter, dispatcher.
- `internal/shared/webhooks/signing/` — shared with outbound (KAI-397 §15).
- `proto/kaivue/webhooks/inbound/v1/` — management API proto.
- `internal/cloud/actions/` — action dispatch queue (likely already owned by lead-cloud; confirm before implementation).

## 14. Open questions

**To lead-security:**
1. 60s window ok for all tenants or should regulated tenants be tighter (30s)?
2. Should we require TLS client cert on top of signature for actions with real-world consequences (alarm, recording start)?
3. Do we need a per-action "high-risk" flag that forces a second factor (e.g. a recent Public API key challenge) before dispatch?
4. Source-IP allowlist — store as `cidr[]` or push operators to IPv6 prefix lists?

**To lead-cloud:**
1. Confirm `internal/cloud/actions/` action-run queue is owned by cloud team, and the synthetic webhook principal integration point is acceptable.
2. Default rate limits: 60/min/sub, 600/min/tenant — acceptable as starting defaults?

**To lead-web:**
1. Inbound webhook management UI: new sub-section under Customer Admin → Integrations. Target KAI-402 developer docs + screenshots of this section.

## 15. Rollout

Same phased approach as KAI-397. Proto → DB migrations → handler + signer (reuse) → rate limiter → action dispatch → management API → UI → dogfood → early access → GA.

Blocks on:
- KAI-397 shared signing package.
- `internal/cloud/actions/` existing (lead-cloud — confirm).
- KAI-399 Connect-Go service mount for the management API.
