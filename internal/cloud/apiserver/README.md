# `internal/cloud/apiserver` — Cloud Control-Plane API Server

The cloud-platform team's HTTP entry point. One process per region hosts every
Connect-Go service defined in `internal/shared/proto/v1` behind a single mux
and a single shared middleware stack.

> **Status:** scaffolding only. Service handlers are deliberate stubs that
> return `connect.NewError(connect.CodeUnimplemented, …)`. The wire paths,
> middleware order, region routing, health, metrics, audit, permissions and
> rate limiting are all fully wired so that handler authors can focus on
> business logic.
>
> All `TODO(KAI-310)` markers in this package are removed when the proto
> generation pipeline lands and we replace the local `connect_stub.go` shim
> with real `connectrpc.com/connect` types — see "KAI-310 handoff" below.

---

## Quick start

```go
srv, err := apiserver.New(apiserver.Config{
    ListenAddr:    ":8443",
    Region:        "us-east-2",
    DB:            cloudDB,
    Identity:      idp,             // internal/shared/auth.IdentityProvider
    Enforcer:      casbinEnforcer,  // internal/cloud/permissions.Enforcer
    AuditRecorder: auditRecorder,   // internal/cloud/audit.Recorder

    // optional
    CORSAllowedOrigins: []string{"https://app.kaivue.io"},
    RateLimit:          apiserver.RateLimitConfig{RequestsPerSecond: 50, Burst: 100},
    RegionRoutes: []apiserver.RegionRoute{
        {Region: "us-west-2", BaseURL: "https://api-us-west-2.kaivue.io"},
        {Region: "eu-west-1", BaseURL: "https://api-eu-west-1.kaivue.io"},
    },
    Tracer: otelHook,
})
if err != nil { ... }

go srv.Start(ctx)
// ...
srv.Shutdown(shutdownCtx) // honours both ctx deadline and cfg.ShutdownTimeout
```

---

## Middleware stack

The chain is built once in `Server.buildConnectChain` and wraps every Connect
method handler. Order is **outside-to-inside**: the topmost middleware sees the
request first and the response last.

```
┌──────────────────────────────────────────────────────────────────┐
│  HTTP request                                                    │
└──────────────────────────────────────────────────────────────────┘
                              │
                              ▼
   ┌───────────────────────────────────────────┐
   │ 1.  recovery          (panic → 500)       │ ◀── outermost
   ├───────────────────────────────────────────┤
   │ 2.  metrics           (count terminal     │
   │                        status classes)    │
   ├───────────────────────────────────────────┤
   │ 3.  request ID        (X-Request-Id →     │
   │                        ctx via            │
   │                        internal/shared/   │
   │                        logging)           │
   ├───────────────────────────────────────────┤
   │ 4.  tracing           (optional OTel hook)│
   ├───────────────────────────────────────────┤
   │ 5.  region routing    (307 → peer region  │
   │                        if X-Kaivue-Region │
   │                        mismatches; 400 if │
   │                        unknown)  [seam #9]│
   ├───────────────────────────────────────────┤
   │ 6.  CORS              (preflight + allow- │
   │                        list)              │
   ├───────────────────────────────────────────┤
   │ 7.  tenant resolution (header / subdomain │
   │                        hint, OVERRIDDEN   │
   │                        by verified claims │
   │                        in step 9)         │
   ├───────────────────────────────────────────┤
   │ 8.  rate limiting     (per-tenant token   │
   │                        bucket; runs       │
   │                        BEFORE auth so DoS │
   │                        traffic is cheap)  │
   ├───────────────────────────────────────────┤
   │ 9.  authentication    (Bearer → IdP       │
   │                        VerifyToken; sets  │
   │                        verified claims +  │
   │                        authoritative      │
   │                        TenantRef)         │
   ├───────────────────────────────────────────┤
   │ 10. audit (KAI-233)   (wraps permission   │
   │                        so a 403 still     │
   │                        surfaces as a deny │
   │                        entry on the way   │
   │                        back up)           │
   ├───────────────────────────────────────────┤
   │ 11. permission        (Casbin enforce on  │
   │                        the route's        │
   │                        (resource, action) │
   │                        pair; default-deny │
   │                        for unknown route) │
   ├───────────────────────────────────────────┤
   │     handler           (Connect stub /     │
   │                        real implementation│
   │                        in KAI-310+)       │
   └───────────────────────────────────────────┘
```

### Why this order?

- **Recovery is outermost** so a panic anywhere — including in the audit
  middleware itself — still terminates with a JSON 500 envelope and a logged
  stack trace.
- **Metrics is just below recovery** so the recovered 500 is counted in
  `kaivue_apiserver_requests_5xx_total`.
- **Request ID is above tracing** so trace spans carry the inbound ID without
  re-deriving it.
- **Region routing is above auth** because a 307 should be cheap — there is no
  point validating a token destined for a redirect.
- **Rate limiting is above auth** for the same reason: token-validation work
  is the most expensive thing the chain does, so we shed load before we
  spend that CPU. The downside is that we key by an unverified tenant hint,
  so an attacker can poison their own bucket. That's acceptable; we can move
  rate limiting below auth in the future if it ever matters.
- **Audit wraps permission**, not the other way around. The permission
  middleware writes the 403 response itself — if audit were inside, the
  audit middleware would never observe the deny. Audit OUTSIDE means the
  status recorder still sees `403` on the way back up.

### Health & metrics endpoints bypass the chain

`/healthz`, `/readyz`, and `/metrics` are mounted with **only** the region
middleware wrapped around them. They must respond during outages that have
already taken down the DB or the IdP, so they cannot share a chain that talks
to either. The `TestMiddlewareChainOrder` test asserts this by checking that
the tracing hook does **not** fire on `/healthz`.

---

## Region-scoped routing (seam #9)

A single apiserver process serves exactly one region. Wrong-region traffic is
handled by `regionMiddleware`:

| `X-Kaivue-Region` header  | Behaviour                                            |
| ------------------------- | ---------------------------------------------------- |
| absent                    | proceed (treat as local)                             |
| matches `cfg.Region`      | proceed                                              |
| matches a `RegionRoute`   | `307 Temporary Redirect` to `RegionRoute.BaseURL`    |
| no match                  | `400 Bad Request` JSON envelope                      |

This is architectural seam #9 from the v1 roadmap: every request is pinned to
its data-residency region by the time it reaches a handler, and the redirect
target is computed from the configured route table — no DNS magic, no service
mesh dependency.

---

## Health, readiness, metrics

| Endpoint   | Status when healthy | Status when unhealthy | Purpose                                          |
| ---------- | ------------------- | --------------------- | ------------------------------------------------ |
| `/healthz` | `200`               | n/a                   | liveness — process is up                         |
| `/readyz`  | `200`               | `503`                 | readiness — DB ping, IdP ping, Casbin store ok   |
| `/metrics` | `200`               | n/a                   | Prometheus text exposition format                |

Readiness probes are pluggable via `Server.SetReadinessProbes` so tests (and
the eventual chaos-test suite) can simulate a DB outage without touching the
real handle.

The metrics endpoint emits the following counters today:

- `kaivue_apiserver_requests_total`
- `kaivue_apiserver_requests_allowed_total`  (2xx)
- `kaivue_apiserver_requests_denied_total`   (403)
- `kaivue_apiserver_requests_5xx_total`
- `kaivue_apiserver_rate_limit_hits_total`   (429)
- `kaivue_apiserver_panics_recovered_total`

When KAI-421 lands its real Prometheus client wiring, this hand-rolled
exposition format goes away in favour of registered Prometheus metrics.

---

## Adding a Connect-Go service

There are two phases. Until KAI-310 (`buf generate`) lands we are in **Phase
1**; the `services.go` table is the source of truth.

### Phase 1 — adding a stub today

1. Open `services.go` and add an entry to `connectServices`:

   ```go
   {"BillingService", []string{
       "GetInvoice",
       "ListInvoices",
   }},
   ```

   Keep the slice in alphabetical order by service name.

2. Add a `RouteAuthorization` row to `defaultRouteAuthorizations` for every
   method that requires Casbin enforcement. Anything you omit is treated as
   "authenticated but not permission-checked":

   ```go
   ServicePath("BillingService", "GetInvoice"):   {ResourceType: "billing", Action: "read"},
   ServicePath("BillingService", "ListInvoices"): {ResourceType: "billing", Action: "read"},
   ```

3. If a method must be reachable without a bearer token (e.g. a public
   webhook), add its path to the `publicPrefixes` list in `middleware.go`.

4. Add a focused test in `server_test.go` that POSTs to the new path and
   asserts the unimplemented (501), 401, 403, or 200 envelope you expect.

That is the entire ceremony — no code generation, no proto file, no buf step.
The route is real, the middleware chain runs end-to-end, and the handler
returns the same wire-format error your generated handler will return.

### Phase 2 — once KAI-310 has run `buf generate`

1. `connect_stub.go` is deleted; switch every `apiserver.Code…` to
   `connect.Code…` and every `*ConnectError` to `*connect.Error`.
2. The `connectServices` table is replaced by reflection over each generated
   `XServiceHandler` interface — `apiserver.New` walks the registered
   handlers and mounts every method automatically.
3. The `defaultRouteAuthorizations` map is built from a `// connect-route:`
   custom option on each rpc, also via reflection. Until then, the hand-coded
   map is the canonical ACL.
4. `unimplementedHandler` goes away; each service is wired to a real handler
   passed in via the `Config` (one field per service, owned by the relevant
   ticket: KAI-224 cross-tenant, KAI-227 tenant provisioning, KAI-321
   recorder control, etc.).

A grep for `TODO(KAI-310)` finds every line that needs touching.

---

## Sibling integration points

| Sibling ticket | Surface in this package                                                |
| -------------- | ---------------------------------------------------------------------- |
| KAI-218        | `Config.DB`, `db.PingContext` in readiness probe                       |
| KAI-222 / 223  | `Config.Identity` (interface), used by auth + readiness                |
| KAI-225        | `Config.Enforcer`, `permissionMiddleware`                              |
| KAI-227        | `TenantProvisioningService` paths reserved in `connectServices`        |
| KAI-233        | `Config.AuditRecorder`, audit middleware wired into chain              |
| KAI-234        | `Config.River` + `Server.River()` accessor (typed interface stub)      |
| KAI-310        | replaces `connect_stub.go` and the `connectServices` table             |
| KAI-321        | recorder control + directory ingest service handlers                   |
| KAI-421        | Prometheus client wiring; replaces `metrics.go`                        |

Nothing in this package writes to the database. Handlers added by sibling
tickets pull `cfg.DB` off the server when they need to.

---

## Tests

```
go test ./internal/cloud/apiserver/...
```

Coverage today:

- `TestServerStartAndShutdown` — boot + graceful shutdown
- `TestMiddlewareChainOrder` — tracing shim observes the final status; request
  ID is set before tracing fires; `/healthz` bypasses the connect chain
- `TestRegionMismatchRedirects` — wrong-region request returns 307 with the
  canonical URL
- `TestRegionUnknownRejected` — unknown region returns 400
- `TestUnauthenticatedReturns401` — missing bearer token → 401 envelope
- `TestAuthenticatedForbiddenReturns403AndAuditDeny` — Casbin default-deny
  produces both a 403 response and an audit `ResultDeny` entry
- `TestAuthenticatedAllowedReturns2xxAndAuditAllow` — explicit allow policy
  produces an audit `ResultAllow` entry on a 200 path
- `TestHealthEndpointsWhenHealthy` — `/healthz` and `/readyz` both 200
- `TestReadinessFailsWhenDBDown` — `/readyz` returns 503 when DB probe fails
- `TestMetricsEndpointExposesCounters` — `/metrics` exposes the counter set
- `TestRateLimitKicksInAtThreshold` — 3rd request exceeds burst → 429

---

## KAI-310 handoff

Every file in this package starts with a `TODO(KAI-310): replace with
generated connectrpc code once buf is wired.` marker. When KAI-310 runs
`buf generate`:

1. Delete `connect_stub.go`. Replace `apiserver.Code` and `apiserver.NewError`
   with the real `connectrpc.com/connect` types.
2. Replace `connectServices` (in `services.go`) with reflection-based mounting
   driven by the generated `XServiceHandler` interfaces.
3. Replace `defaultRouteAuthorizations` with a builder that walks the
   generated `FileDescriptor` set and reads each method's
   `(kaivue.v1.connect_route)` custom option.
4. Wire each generated handler interface as a `Config` field; remove
   `unimplementedHandler` once every method has a real implementation
   (handlers landing across KAI-227, KAI-321, KAI-281, KAI-297, …).
5. Delete every `TODO(KAI-310)` marker.

The middleware stack, region routing, health endpoints, metrics, audit, and
rate limiter do **not** change in KAI-310; they are designed against the
canonical `/<proto-package>.<service>/<method>` path shape that connect-go
already uses.
