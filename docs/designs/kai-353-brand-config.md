# KAI-353 — Per-integrator brand config schema + asset storage

- Linear: https://linear.app/kaivue/issue/KAI-353/white-label-per-integrator-brand-asset-storage-brand-config-schema
- Project: MS: White-Label & Mobile Build Pipeline
- Status: draft (initial schema + in-memory impl)
- Package: `internal/cloud/whitelabel`
- Dependants: KAI-354 (mobile build pipeline), KAI-310 (integrator portal brand UI), KAI-355 (credential vault / asset encryption), KAI-356 (custom domain), KAI-357 (email infra)

## Goal

Define the single source of truth for every per-integrator white-label surface (mobile app artifacts, marketing emails, portal chrome). The brand config is versioned so integrators can audit changes and roll back, and assets are stored with content validation so downstream build jobs can trust what they fetch.

## Schema (`BrandConfig`)

| Field | Type | Notes |
| --- | --- | --- |
| `tenantId` | UUID | Foreign key to `cloud.tenants` (integrator tenants only). |
| `version` | int (>=1) | Monotonic; bumped on every PUT, never re-used after delete. |
| `appName` | string (<=64) | Appears on launch screen, push notif sender label. |
| `colors.{primary,secondary,accent}` | hex color | Required. `background`, `foreground` optional. |
| `typography.{headingFamily,bodyFamily,monoFamily?}` | string (<=64) | Family names; actual binaries live in `assets.fonts`. |
| `bundleIds.{ios,android}` | reverse-DNS | Stamped into iOS/Android builds by KAI-354. |
| `senderDomain` | FQDN | Used by KAI-357 for SPF/DKIM/DMARC. |
| `tosUrl`, `privacyUrl` | absolute https URL | Rendered in mobile settings and account creation. |
| `assets.logo` | `AssetRef` | Image. |
| `assets.splash` | `AssetRef` | Image. |
| `assets.icon` | `AssetRef` | Square 1024x1024 PNG. |
| `assets.fonts` | `[]AssetRef` | Multiple allowed (weights/styles). |
| `updatedAt`, `updatedBy` | audit metadata | Stamped by the store. |

Validation runs via `BrandConfig.Validate()` and enforces the acceptance criterion "assets validated (dimensions, format, safe content)" at the schema layer. Asset content validation lives in `validateAsset` (see below).

## Asset storage

`BrandAssetStore` interface:

```go
Put(ctx, tenantID, kind, filename, content) (AssetRef, error)
Get(ctx, tenantID, kind, version) (AssetRef, io.ReadCloser, error)
List(ctx, tenantID) ([]AssetRef, error)
Delete(ctx, tenantID, kind, version) error
```

Implementations:

- `MemoryAssetStore` — in-process, used for tests and the API round-trip. Monotonic per-(tenant, kind) versioning.
- `S3AssetStore` — stub with `TODO(lead-cloud)`; real implementation deferred to KAI-355 which owns KMS envelope encryption and signed URL issuance.

### Asset validation

Per-kind constraints in `asset_store.go`:

| Kind | Max size | MIME (sniffed) | Dimensions |
| --- | --- | --- | --- |
| `logo` | 2 MiB | png/jpeg/gif | 64x64 .. 4096x4096 |
| `splash` | 5 MiB | png/jpeg | 512x512 .. 4096x4096 |
| `icon` | 1 MiB | png | exactly 1024x1024, square |
| `font` | 4 MiB | octet-stream / font/* | extension in ttf/otf/woff/woff2 |

`http.DetectContentType` does the sniffing so untrusted uploads cannot lie via `Content-Type`; images additionally round-trip `image.DecodeConfig` so we know the bytes actually decode.

### Storage layout

```
s3://kaivue-whitelabel/{tenant_id}/brand/v{version}/{asset_kind}
```

Example: `s3://kaivue-whitelabel/11111111-.../brand/v7/logo`. The storage key is opaque to callers — they only ever reference assets via `AssetRef`. The memory store mirrors this path shape so test expectations carry over.

## Version semantics

- Each `PUT /brand` creates a new `BrandConfig` version for the tenant and is stamped with `updatedAt`.
- The version number is a dense monotonic integer per tenant; gaps are not allowed.
- Past versions remain readable via `GET /brand/versions/{v}` for audit and rollback.
- Asset versions are independent of config versions. A config references assets by their own version; this lets integrators swap a logo without rewriting the whole config, and lets KAI-354 build jobs pin "brand config v7" deterministically.
- Deletes never renumber. If an integrator deletes logo v3, config versions that referenced it will 404 when fetching that asset — the build job is expected to treat that as a hard failure.

## API surface

Emitted via `API.Routes()`; not wired into the cloud router here. KAI-354 owns the wiring.

```
GET    /api/v1/integrators/{id}/brand                  -> current BrandConfig
PUT    /api/v1/integrators/{id}/brand                  -> create new version
POST   /api/v1/integrators/{id}/brand/assets/{kind}    -> upload asset version
GET    /api/v1/integrators/{id}/brand/versions         -> list all versions
GET    /api/v1/integrators/{id}/brand/versions/{v}     -> fetch specific version
```

Defensive: PUT rejects mismatched `tenantId` between path and body to blunt confused-deputy attacks from a compromised portal session.

## Out of scope (handled elsewhere)

- Integrator portal UI — **KAI-310 (lead-web)**.
- Mobile build pipeline that consumes the brand config — **KAI-354 (lead-mobile)**.
- S3/R2 backend + KMS envelope encryption — **KAI-355 (lead-cloud)**.
- Custom domain CNAME validation — **KAI-356**.
- Email infra (SPF/DKIM/DMARC) — **KAI-357**.
- Postgres table DDL for `brand_configs` — folded into KAI-355 with the S3 work.

## Open questions

1. **Asset soft-delete vs hard-delete.** Current design hard-deletes. Integrators may need a "trash" window so a fat-fingered delete during a portal edit doesn't break their production mobile builds. Proposal: treat delete as soft-delete with a 30-day retention and resurrect via admin API.
2. **Config schema evolution.** We'll want an explicit `schemaVersion` field once KAI-354 ships so older build runners can refuse a config they don't understand. Deferring until we have the first runner.
3. **Preview / staging.** Should a tenant have a "draft" config separate from the currently-active version, or does every PUT go live? Current design: every PUT goes live. Portal UX can simulate drafts by holding state client-side until the user presses save.
4. **Font subsetting.** Should we accept raw TTF/OTF and subset in the build pipeline, or require integrators to upload already-subsetted WOFF2? Currently permissive. Pipeline perf will dictate.
