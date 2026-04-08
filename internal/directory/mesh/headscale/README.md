# headscale — per-site tailnet coordinator (KAI-240)

This package runs inside the Directory binary as the single per-site
tailnet coordinator. Every other component on the customer's network
(Recorder, pair CLI, sidecar, etc.) registers with this coordinator
via `tsnet` (see `internal/shared/mesh/tsnet`, KAI-239).

## Public surface

```go
c, err := headscale.New(headscale.Config{
    StateDir:  "/var/lib/mediamtx-directory/mesh",
    Namespace: "kaivue-site",
    ServerURL: "https://directory.example.lan:8443",
    MasterKey: masterKeyFromNvrJWTSecret,
    Logf:      slogLogf,
})
_ = c.Start(ctx)
defer c.Shutdown(ctx)

key, _ := c.MintPreAuthKey(ctx, headscale.DefaultNamespace, time.Hour)
nodes, _ := c.ListNodes(ctx)
_ = c.RevokeNode(ctx, "n1")
_ = c.Addr()       // control URL for tsnet clients
_ = c.Healthy()    // readiness probe
```

## Design decisions

### Namespace-per-site, not per-tenant

Kaivue runs exactly one Headscale namespace per customer site. A
single customer may host multiple tenants, but they share the on-prem
mesh — tenancy separation happens above the mesh via mTLS identity
(KAI-241) and RBAC, not via separate tailnets. The default namespace
name is `kaivue-site`. The namespace is chosen on first boot and
persisted; changing it mid-life is intentionally not supported.

### Master key reuse (do NOT add new config fields)

Per `CLAUDE.md`, we do not modify `mediamtx.yml` runtime settings. In
particular, we do NOT add a new `headscale.masterKey` or similar
field. The coordinator reuses the existing `nvrJWTSecret` as its
master key and derives a purpose-specific subkey via
`cryptostore.NewFromMaster(master, nil, "directory-mesh-headscale")`.

### State encrypted at rest

All persistent state lives in a single file under `StateDir`:

    <StateDir>/coordinator.state.enc

It is AES-256-GCM encrypted via `internal/shared/cryptostore` using
the HKDF-derived subkey above. Writes are atomic (write-to-temp +
rename) and files are `0600`. A test (`TestStateDecryptFailsWith
WrongMasterKey`) verifies that a wrong master key fails to decrypt.

### Loopback-only bind

The coordinator's local listener always binds to a loopback address
(default `127.0.0.1:0`). Customer-facing traffic reaches it either
via the mesh (from a tsnet client) or via the Directory's
reverse-proxy ingress. `ServerURL` is the public URL the pairing
flow hands out; it is independent from the loopback bind.

### Test-mode escape hatch

`Config.TestMode = true` short-circuits every disk and network side
effect:

- `MasterKey` is not required.
- No state file is written or read.
- The coordinator still binds a loopback listener (so `Addr()`
  returns something dialable for tsnet test harnesses) but holds no
  persistent state.
- `MintPreAuthKey` / `ListNodes` / `RevokeNode` / `Healthy` all
  behave identically to real mode from the caller's perspective.

Unit tests across the codebase should use `TestMode: true` so they
do not require a real Headscale or SQLite database on the CI runner.
See `coordinator_test.go` for the baseline.

## TODO — real embedded Headscale

The long-term plan is to host an actual `github.com/juanfont/headscale`
inside the Directory process. This ships today as a stub/fake backend
for the reasons laid out in `coordinator.go`:

1. Headscale's control-plane machinery lives under `internal/hscontrol`,
   which Go's module system forbids importing from outside the
   Headscale repo.
2. The package paths and constructor signatures of the pieces that
   *are* public (e.g. `types`, `db`) move on every minor release. The
   roadmap spec explicitly budgets "~1 day per Headscale upgrade" for
   adapter fixes, and "~1 week" for a sidecar fallback if embedding
   becomes unsustainable.
3. Wave 2 (KAI-239 tsnet, KAI-241 step-ca, KAI-243 pairing, KAI-246
   sidecar supervisor) needs a stable Coordinator surface *now*, and
   cannot block on upstream API churn.

### Follow-up ticket scope

When the real embedded Headscale lands, the work is:

1. Vendor `github.com/juanfont/headscale` at a pinned tag (plus
   transitive deps — `zcache`, `gorm`, `derp`, etc.).
2. Add a `real_backend.go` that constructs `hscontrol.Headscale`
   directly from an in-memory config struct (mirroring what
   headscale's own `cmd/headscale/main.go` does) so no YAML config
   file or command-line flags are required.
3. Drive its SQLite state file through `internal/shared/cryptostore`
   (open-encrypt-write wrapper, or SQLCipher-via-cryptostore if we
   prefer page-level encryption).
4. Wire `Coordinator.MintPreAuthKey` / `ListNodes` / `RevokeNode`
   onto the embedded admin client (or, if the admin API is still
   gRPC-only, a short-circuit in-process dialer).
5. Flip the branch in `New()` from `newStubBackend` to
   `newRealBackend` when `!cfg.TestMode`. The stub backend stays in
   the package as the `TestMode: true` implementation.
6. Add an integration test behind a build tag (`//go:build
   integration`) that runs the real backend end-to-end against a
   tsnet client.
7. Write `docs/operations/headscale-upgrades.md` (called out in the
   Linear acceptance criteria) describing how to bump the Headscale
   version and run the adapter-fix budget.

Until then, the stub behaves enough like the real thing for KAI-239,
KAI-243, and KAI-246 to develop against: it mints realistic
`hskey-auth-…` pre-auth keys, tracks nodes, persists state encrypted
at rest, and exposes a loopback listener for readiness probes.

## Test status

```
$ go test ./internal/directory/mesh/headscale/... -race
ok  github.com/bluenviron/mediamtx/internal/directory/mesh/headscale
```

12 tests cover: master-key requirement, namespace validation, start/
shutdown lifecycle, pre-auth key minting (and empty-namespace
rejection), node list/revoke, double-start rejection, idempotent
shutdown, state persistence across restart (real-mode code path with
tempdir master key), and decrypt failure under a wrong master key.
