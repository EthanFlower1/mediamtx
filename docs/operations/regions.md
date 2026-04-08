# Region-scoped URL routing — operations guide

**KAI-230** | Last updated: 2026-04-08 | Owner: cloud-platform

---

## Overview

Every Kaivue cloud API endpoint is region-scoped under `https://<region>.api.yourbrand.com/v1/...`. In v1 only `us-east-2` exists. The routing infrastructure is live from day one so adding region 2 in v1.x is an uncommenting operation, not a schema change.

Architectural seam #9: build for multi-region, ship single-region.

---

## URL scheme

| Purpose | Pattern | v1 example |
|---|---|---|
| Public API | `https://<region>.api.yourbrand.com/v1/...` | `https://us-east-2.api.yourbrand.com/v1/cameras` |
| JWKS (public key set) | `https://<region>.api.yourbrand.com/.well-known/jwks.json` | — |
| Health probe | `https://<region>.api.yourbrand.com/healthz` | — |
| Health probe (region validate) | `https://<region>.api.yourbrand.com/healthz?region=<r>` | — |
| Internal VPC only | `https://<region>.api-int.yourbrand.com/v1/...` | — |

---

## DNS layout

DNS entries live in the Terraform module at `infrastructure/modules/region/<region>/dns.tf` (KAI-231). v1 deploys:

```
us-east-2.api.yourbrand.com  →  ALB in us-east-2 (CNAME / alias)
```

When eu-west-1 goes live, add:

```
eu-west-1.api.yourbrand.com  →  ALB in eu-west-1
```

No wildcard is used. Each region must be explicitly provisioned.

---

## Routing middleware behaviour

The region routing middleware (`internal/cloud/regionrouter/middleware`) runs at position #5 in the apiserver chain — before CORS, before auth. This means cross-region detection is free: no token validation work is done for misrouted requests.

| Request host | X-Kaivue-Region header | Outcome |
|---|---|---|
| `us-east-2.api.yourbrand.com` | any / absent | Pass-through (region injected into ctx) |
| `eu-west-1.api.yourbrand.com` | — | 307 → `https://eu-west-1.api.yourbrand.com/...` |
| `ap-southeast-1.api.yourbrand.com` | — | 421 Misdirected Request |
| `localhost` / IP | absent | Pass-through, local region injected |
| `localhost` / IP | `us-west-2` | 307 → `https://us-west-2.api.yourbrand.com/...` |
| `localhost` / IP | `unknown-region` | 421 Misdirected Request |

---

## Health probe region validation

`GET /healthz?region=<r>` validates the region claim without touching the DB.

| ?region | Response |
|---|---|
| omitted or matches local region | 200 `{"status":"ok","region":"us-east-2"}` |
| known peer region | 302 → `https://<peer>/healthz?region=<peer>` |
| unknown region | 400 `{"error":"unknown region"}` |

Kubernetes liveness probes should NOT pass a `?region=` param; that param is for monitoring automation that auto-discovers endpoints.

---

## Tenant home-region lookup

The `TenantRegionResolver` (`internal/cloud/regionrouter`) answers "what region is this tenant's home region?" It is consumed by the cross-region redirect handler. Cache TTL defaults to 5 minutes; on a cache miss it queries the DB tenant table (KAI-218 `region` column).

When Redis (KAI-217) is wired in, swap the in-memory `Cache` for a Redis-backed implementation. The `TenantLookup` interface is the only seam that changes.

---

## Adding a new region (v1.x checklist)

1. Add the region to `regionrouter.KnownRegions` and `regionrouter.BaseURLForRegion` in `internal/cloud/regionrouter/regionrouter.go`.
2. Add a `RegionRoute` entry to the `Config.RegionRoutes` field in the region's apiserver deployment config.
3. Apply the DB migration that seeds the new region value in `region` columns (KAI-218 schema already has the column).
4. Add the DNS CNAME/alias in `infrastructure/modules/region/<new-region>/dns.tf` (KAI-231).
5. Deploy the new region's EKS cluster + RDS replica (KAI-215/216 per-region Terraform module).
6. Smoke-test: `curl https://<new-region>.api.yourbrand.com/healthz?region=<new-region>` → 200.

---

## Invariants (enforced by tests)

- Tenant A's cache entry cannot satisfy a lookup for tenant B (`TestCache_MultiTenantIsolation`).
- A request to region X for a tenant homed in region Y receives a 302 to Y (`TestMiddleware_MismatchTenantHomeRegion_Redirects`).
- An unknown region in the host always returns 421, never a 4xx that leaks allowlist information beyond the error message.
