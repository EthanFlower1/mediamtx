# Package layout — Kaivue Recording Server

Status: **active** — introduced by KAI-236 as part of _MS: On-Prem Foundation
& Hardware Compatibility_. Reference: design doc §4.2.

## Goals

The Kaivue Recording Server ships as a single Go binary that runs in three
modes on customer hardware:

- `directory` — the cloud-aware control plane for a single tenant.
- `recorder` — camera capture and on-disk recording for one or more cameras.
- `all-in-one` — both roles in one process, for small sites.

To make those modes truly independent at build time and at runtime, the Go
package tree under `internal/` is split into three role-scoped trees with
hard import boundaries enforced by a linter.

## Layout

```
internal/
  directory/   # Directory role
  recorder/    # Recorder role (colocated with the upstream segment recorder)
  shared/      # types, protos, primitives shared by both roles
    proto/v1/  # Connect-Go contracts for every inter-role service
    certmgr/   # mTLS cert manager backed by per-site step-ca
    sidecar/   # supervisor for Zitadel / MediaMTX sidecars
    tsnetnode/ # embedded tsnet/Headscale mesh helpers
    errors/    # stable error codes (KAI-424)
  nvr/         # legacy NVR subsystem; gradually emptied during migration
  core, servers, stream, playback, protocols, ...   # upstream MediaMTX
```

KAI-236 creates the three role trees as **skeletons** (`doc.go` only) — no
existing code is moved. Subsequent tickets in the
_On-Prem Foundation & Hardware Compatibility_ milestone (KAI-237 onward)
migrate code out of `internal/nvr/` into the appropriate role.

## Boundary rules

| From                  | `internal/shared/...` | `internal/directory/...` | `internal/recorder/...` |
| --------------------- | :-------------------: | :----------------------: | :---------------------: |
| `internal/directory/` |        allowed        |          (self)          |      **forbidden**      |
| `internal/recorder/`  |        allowed        |      **forbidden**       |         (self)          |
| `internal/shared/`    |        (self)         |      **forbidden**       |      **forbidden**      |

- The two role packages **never** import each other directly. All
  cross-role communication rides Connect-Go services defined in
  `internal/shared/proto/v1/`.
- `internal/shared/` is a **leaf** with respect to the role packages: it
  exposes types and primitives, never depends on a role.
- Editing anything under `internal/shared/proto/` requires the proto-lock
  (see `docs/proto-lock.md`).

## Enforcement

The boundary is enforced by a `depguard` linter configured in
`.golangci.yml`. The relevant rules are tagged `KAI-236` in comments and
include three rule sets:

- `directory-no-recorder` — denies any import of
  `github.com/bluenviron/mediamtx/internal/recorder` from files under
  `internal/directory/`.
- `recorder-no-directory` — the symmetric rule for the Recorder role.
- `shared-is-leaf` — denies any import of either role package from files
  under `internal/shared/`.

CI runs `golangci-lint run ./...` and blocks PRs that violate the rules.
Locally:

```sh
golangci-lint run ./internal/directory/... ./internal/recorder/... ./internal/shared/...
```

To verify the rules trip, temporarily add a forbidden import in a doc.go and
re-run the command — you should see a `depguard` error referencing KAI-236.

## Why this matters

- **Independent builds.** A pure-Recorder image must not transitively pull
  in the Directory's Zitadel client, Casbin policy, or cloud sync. Import
  boundaries make that guarantee mechanical, not aspirational.
- **Stable cross-role contract.** With the wire shape (`shared/proto`) as
  the contract, either side can refactor freely without coordinating Go API
  changes.
- **Failure isolation.** The Recorder must keep recording when the
  Directory, the cloud, or the mesh is unreachable. A leaky Go-level
  dependency would tempt code to assume Directory is in-process.

## Migration plan

KAI-236 lands the skeleton and the linter only. Subsequent tickets move
code one subsystem at a time:

1. Identify the subsystem in `internal/nvr/` (e.g. `nvr/onvif`,
   `nvr/recordings`, `nvr/users`).
2. Pick a destination: `directory/`, `recorder/`, or `shared/`. If both
   roles need it, the answer is `shared/`, possibly behind an interface.
3. Move the package, update imports, run
   `go build ./... && go test ./<moved>/...`.
4. Run `golangci-lint run ./...` and resolve any new boundary violations by
   pushing the dependency into `shared/` or replacing it with a Connect-Go
   call through `shared/proto`.
5. Bump `internal/nvr/db` migration version if a schema changed.

The `internal/nvr/` tree stays in place until the migration is complete.
Both old and new layouts can coexist; the linter only governs the new role
packages.
