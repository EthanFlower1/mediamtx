# `internal/` — Kaivue Recording Server package layout

This directory mixes two generations of code:

1. **Upstream MediaMTX packages** (`core`, `servers`, `stream`, `recorder`,
   `playback`, `protocols`, ...) inherited from the bluenviron/mediamtx
   project. These continue to provide the streaming/recording engine.
2. **NVR subsystem** (`nvr/`) — the first-generation Kaivue NVR layer that
   wraps MediaMTX with cameras, ONVIF, recordings, users, etc.
3. **Role-scoped Kaivue packages** introduced by **KAI-236**:

   ```
   internal/
     directory/   # cloud-aware Directory role (single-tenant on-prem control plane)
     recorder/    # Recorder role (camera capture + on-disk recording engine)
     shared/      # types, protos, primitives shared by both roles
   ```

   These start as **skeletons with `doc.go` only**. Code is migrated out of
   `internal/nvr/` (and, where appropriate, the upstream packages) into the
   role packages incrementally; nothing is moved as part of KAI-236 itself.

## Boundary rules (enforced by `depguard`)

The Kaivue Recording Server runs as a single Go binary in three modes —
`directory`, `recorder`, and `all-in-one`. To keep those modes truly
separable, the role packages have hard import boundaries:

| From                  | May import `internal/shared/...` | May import `internal/directory/...` | May import `internal/recorder/...` |
|-----------------------|:--------------------------------:|:------------------------------------:|:----------------------------------:|
| `internal/directory/` | yes                              | (self)                               | **NO**                             |
| `internal/recorder/`  | yes                              | **NO**                               | (self)                             |
| `internal/shared/`    | (self)                           | **NO**                               | **NO**                             |

`internal/shared/` is a **leaf** with respect to the role packages: it must
not depend on either of them. Cross-role communication MUST go through the
Connect-Go services defined in `internal/shared/proto/v1/` — never through a
direct Go import or a hand-rolled REST shape.

These rules are enforced by `depguard` rules in `.golangci.yml` (search for
`KAI-236`). A violation produces a lint error that blocks CI.

### Existing `internal/recorder` (upstream MediaMTX)

The `internal/recorder` package already exists upstream as the on-disk
segment recorder. It is logically part of the new Recorder role, so the
KAI-236 skeleton colocates the role doc with it (`internal/recorder/doc.go`)
rather than introducing a parallel directory. Existing files are not moved
or modified; they simply gain a new sibling that documents the role and is
covered by the new depguard rules.

### Why these rules exist

* **Independent runtime modes.** A pure-Recorder build must not transitively
  pull in Directory code (Zitadel client, policy engine, cloud sync), and
  vice versa. Import boundaries make that guarantee mechanical.
* **Stable cross-role contract.** Forcing all cross-role traffic through
  `shared/proto` means the wire shape is the contract. Refactors on either
  side can land without coordinating Go API changes.
* **Failure isolation.** The Recorder must keep recording when the
  Directory, the cloud, or the mesh is unreachable. A leaky Go-level
  dependency would tempt code to assume Directory is in-process.

## Running the linter

If you have `golangci-lint` v2 installed:

```sh
golangci-lint run ./internal/directory/... ./internal/recorder/... ./internal/shared/...
```

To lint the whole tree (slower; respects the existing `internal/nvr` and
`cmd/backfill` exclusions):

```sh
golangci-lint run ./...
```

If `golangci-lint` is not on your PATH, install it via:

```sh
go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest
```

To **manually verify** that the boundary rules trip, drop a test import in
`internal/directory/doc.go` such as:

```go
import _ "github.com/bluenviron/mediamtx/internal/recorder"
```

then re-run `golangci-lint run ./internal/directory/...`. You should see a
`depguard` error referencing `KAI-236`. Remove the test import before
committing.

## Migration guidance

When moving code out of `internal/nvr/` into a role package:

1. Decide which role owns the subsystem. If both need it, the answer is
   `internal/shared/`, possibly behind an interface defined there.
2. Move the package, update imports, and run
   `go build ./... && go test ./<moved>/...`.
3. Run `golangci-lint run ./...` and fix any boundary violations by either
   moving the dependency to `shared/` or replacing it with a Connect-Go
   call through `shared/proto`.
4. Update `docs/architecture/package-layout.md` with the new home.

See [`../docs/architecture/package-layout.md`](../docs/architecture/package-layout.md)
for the full design.
