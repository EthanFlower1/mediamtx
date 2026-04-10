# `internal/cloud/permissions`

Casbin-backed authorization engine for the MediaMTX cloud control plane
(KAI-225). Every cloud API must route its permission checks through
`Enforcer.Enforce` — there is no other sanctioned path.

## Quick reference

```go
store := permissions.NewInMemoryStore() // KAI-216 replaces this with RDS
enf, err := permissions.NewEnforcer(store, permissions.DefaultAuditSink)

// Seed the default admin role for tenant A and bind a user to it.
adminRole, _ := permissions.SeedRole(enf,
    permissions.DefaultRoleTemplates[permissions.RoleAdmin], tenantA)
_ = permissions.BindSubjectToRole(enf,
    permissions.NewUserSubject("user-1", tenantA), adminRole)

// Enforce.
ok, err := enf.Enforce(ctx,
    permissions.NewUserSubject("user-1", tenantA),
    permissions.NewObject(tenantA, "cameras", "cam-1"),
    permissions.ActionViewLive,
)
```

## Model

See `model.conf`. RBAC with pattern matching:

- `r = sub, obj, act`
- `p = sub, obj, act, eft`
- `g = subject, role`
- matcher: `(r.sub == p.sub || g(r.sub, p.sub)) && keyMatch2(r.obj, p.obj) && keyMatch2(r.act, p.act)`
- effect: allow-and-deny-with-deny-override (deny wins)

The Go enforcer layer adds a **fail-closed default** on top: any missing
policy, empty action, malformed subject, or tenantless object returns deny.
There is no "allow by default" branch anywhere.

## Subject format

| Kind         | Wire format                                    | When to use                         |
| ------------ | ---------------------------------------------- | ----------------------------------- |
| `user`       | `user:<user_id>@<tenant_id>`                   | direct in-tenant identity           |
| `integrator` | `integrator:<user_id>@<customer_tenant_id>`    | reseller staff acting on a customer |
| `federation` | `federation:<peer_directory_id>`               | federated peer tenant               |

**Critical:** the `<tenant_id>` suffix on `integrator:` subjects is the
**customer** tenant, not the integrator's own tenant. This is the whole
reason the cross-tenant prefix exists — it binds the acting identity to the
customer it is currently servicing.

Build subjects via the constructors (`NewUserSubject`, `NewIntegratorSubject`,
`NewFederationSubject`, `SubjectFromClaims`) — never by string concatenation.

## Object format

Objects are always rendered as `<tenant_id>/<resource_type>/<resource_id>`.
The tenant prefix is **mandatory** — passing a tenantless ObjectRef to
`Enforce` is a hard error. This structurally prevents a policy like
`cameras/*` from accidentally matching another tenant's cameras.

Use `"*"` as the resource id (or `NewObjectAll`) for "any instance of this
type in this tenant".

## Canonical actions

Every authorization check must use one of the constants from `actions.go`.
Adding an action means adding a constant here and a corresponding role-grant
entry in `roles.go` if any default role should be able to use it.

Categories:

- **Viewing:** `view.thumbnails`, `view.live`, `view.playback`, `view.snapshot`
- **Control:** `ptz.control`, `audio.talkback`
- **Cameras:** `cameras.add`, `cameras.edit`, `cameras.delete`, `cameras.move`
- **Users:** `users.view`, `users.create`, `users.edit`, `users.delete`, `users.impersonate`
- **Permissions:** `permissions.grant`, `permissions.revoke`
- **Platform:** `integrations.configure`, `federation.configure`, `billing.view`, `billing.change`
- **Observability:** `audit.read`, `system.health`, `settings.edit`
- **Recorder:** `recorder.pair`, `recorder.unpair`
- **AI / FaceVault:** `ai.configure`, `ai.facevault.read`, `ai.facevault.write`, `ai.facevault.erase`, `ai.models.upload`

## Role templates

`DefaultRoleTemplates` provides the seed bundles every new tenant is
provisioned with: `admin`, `operator`, `viewer`, `integrator_admin`,
`integrator_support`. They use keyMatch2 wildcards (`view.*`, `*`) so adding
a new action inside an existing namespace does not require re-seeding.

Role ids are rendered as `role:<name>@<tenant_id>` — they are tenant-scoped
by construction, so a `role:admin@tenant-A` binding gives nothing in
`tenant-B`.

## Sub-reseller narrowing

`ResolveIntegratorScope(ctx, integratorUserID, customerTenant)` walks the
`parent_integrator` chain (stored alongside KAI-228's
`customer_integrator_relationships`) and intersects `scoped_permissions` at
every step. **The child never broadens the parent** — if the parent only
holds `view.live`, the child cannot grant itself `view.playback` no matter
what its own row says. The walk is bounded at 32 hops to prevent cycles.

## Policy storage seam

`PolicyStore` is an interface with `LoadAll`, `AddPolicy`, `RemovePolicy`,
`AddGrouping`, `RemoveGrouping`, `ListPolicies`, `ListGroupings`. KAI-225
ships only `InMemoryStore`; KAI-216 will add the RDS-backed adapter. The
interface is intentionally narrow so the swap is drop-in and does not touch
any call sites.

## Audit log integration

Every `Enforce` call emits one `AuditRecord` via `AuditSink.RecordEnforce`.
Until KAI-233 lands, `DefaultAuditSink` is a no-op. When KAI-233 merges,
replace `DefaultAuditSink` with `audit.Log.RecordEnforce` — see the
`TODO(KAI-233)` marker in `enforcer.go`.

## Integration with `internal/shared/auth`

`SubjectFromClaims(auth.Claims)` is the only sanctioned way to derive a
subject from an authenticated session. Handlers must:

1. Verify the token via the `IdentityProvider` (KAI-222).
2. Extract `Claims` from the resulting session.
3. Call `SubjectFromClaims` to build the `SubjectRef`.
4. Pass that ref to `Enforcer.Enforce`.

Never construct a `SubjectRef` from a request body. Trusting client-supplied
tenant ids is the canonical multi-tenant isolation bug.

## Every cloud API PR must add an isolation test

**Non-negotiable.** Before merging, any PR that adds a new cloud API
endpoint must add a test under `internal/cloud/permissions/` (or its own
handler package) that verifies:

- A user in tenant A cannot reach tenant B's instance of the new resource.
- An integrator without a relationship to a tenant cannot reach that
  tenant's instance either.
- A federation subject without an explicit grant is denied.
- Fail-closed: removing the grant immediately denies on the next call.

The Wave-1 isolation test suite in `enforcer_test.go` is the reference shape
for these tests.
