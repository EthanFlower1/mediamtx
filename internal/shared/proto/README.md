# Kaivue shared proto schemas (`internal/shared/proto/v1/`)

This directory holds the **only** inter-role contract surface in Kaivue: the
`.proto` files that define how Directory, Recorder, Federation peers, and
clients talk to each other over Connect-Go (gRPC-compatible HTTP).

This is **Seam #2** in the architecture: no other wire format crosses a role
boundary. Anything not in these protos is an internal implementation detail of
a single role and MUST NOT be consumed by another role.

See also:
- `.claude/agents/tech-lead.md` — seam catalog
- `docs/proto-lock.md` — proto mutation protocol (required reading)
- `docs/superpowers/specs/2026-04-07-v1-roadmap.md` — v1 roadmap

## Services defined here

| File | Service | Direction | Purpose |
|---|---|---|---|
| `recorder_control.proto` | `RecorderControl` | Directory → Recorder | Server-streaming push of camera assignments |
| `directory_ingest.proto` | `DirectoryIngest` | Recorder → Directory | Client-streaming state, segment index, AI events |
| `federation_peer.proto` | `FederationPeer` | Directory ↔ Directory | Cross-tenant federation (ping, JWKS, directory reads, stream mint) |
| `streams.proto` | — | shared | `StreamClaims` + stream mint request/response types |
| `auth.proto` | — | shared | Login, refresh, SSO, `TenantRef`, `Session`, `TokenClaims`, provider config |
| `cameras.proto` | — | shared | Camera CRUD types (no credentials — those stay server-side) |
| `pairing.proto` | — | shared | Recorder pairing token + handshake envelope |

## The proto-lock is load-bearing

**Before touching anything in this directory, acquire the proto-lock.** Full
protocol lives in `docs/proto-lock.md`. The short version:

```bash
# 1. Acquire
scripts/proto-lock.sh acquire KAI-XYZ <agent-name> \
  "reason for this schema change" \
  file1.proto file2.proto

# 2. Make your edits. Every commit that touches internal/shared/proto/**
#    MUST carry a trailer:
#       Proto-Lock-Holder: KAI-XYZ

# 3. Release in the same feature branch
scripts/proto-lock.sh release KAI-XYZ
```

The lock exists because multiple autonomous agents work in parallel and
cannot negotiate field numbers with each other. Without serialization they
will silently collide and ship incompatible wire formats.

## Invariants — do NOT break these

1. **Field numbers are permanent.** Once a field number is assigned and
   shipped, it is never reused for a different field and never changed. To
   remove a field, reserve its number and name:
   ```proto
   message Foo {
     reserved 5, 7 to 9;
     reserved "old_field_name";
   }
   ```
2. **Never change a field's type.** Add a new field with a new number instead.
3. **Never rename a proto package.** `kaivue.v1` is permanent for v1. If the
   wire format needs to change incompatibly, create `kaivue.v2` alongside.
4. **Never delete a service RPC.** Deprecate it (`deprecated = true`) and add
   a new one.
5. **No credentials in proto.** Camera credentials, JWT signing keys, and
   step-ca enrolment material live server-side. Protos may reference them by
   opaque ID only.
6. **All timestamps use `google.protobuf.Timestamp`.** All durations use
   `google.protobuf.Duration`. No custom epoch encodings.
7. **All monetary or byte-count values use explicit units in the field name**
   (e.g. `retention_days`, `bitrate_kbps`, `disk_bytes`).

## Regenerating Go + Dart stubs

Generation is wired through `buf` but **not yet executed in CI**. Once `buf`
and the Connect-Go + Dart plugins are installed, run from this directory:

```bash
cd internal/shared/proto/v1
buf lint          # style + naming
buf breaking --against '.git#branch=main,subdir=internal/shared/proto/v1'
buf generate      # writes to internal/shared/proto/gen/go and clients/flutter/lib/src/gen/proto
```

Generated code lives outside this directory (see `buf.gen.yaml`) and is
committed to the repo so downstream builds don't need `buf` on the
developer machine. Generated files are NOT covered by the proto-lock — only
the hand-authored `.proto` sources are.

## Adding a new service

1. Acquire the proto-lock.
2. Create `<name>.proto` in this directory with `package kaivue.v1;`.
3. Give every message a clear doc comment. Field numbers start at 1.
4. Run `buf lint` (once installed).
5. Commit with `Proto-Lock-Holder: <KAI-id>` trailer.
6. Release the lock.

## Authoring style

- Package: `kaivue.v1`
- Go package prefix: `github.com/bluenviron/mediamtx/internal/shared/proto/gen`
- File naming: `snake_case.proto`
- Message naming: `PascalCase`
- Field naming: `snake_case`
- RPC naming: `PascalCase`
- Enum values: `SCREAMING_SNAKE_CASE` with a `_UNSPECIFIED = 0` zero value
- Every service has doc comments explaining direction (who calls whom) and
  whether the RPC is unary, server-streaming, client-streaming, or bidi.
