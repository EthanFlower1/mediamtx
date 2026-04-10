# Proto-Lock Protocol

**Purpose:** Serialize `.proto` schema changes across autonomous agents so they don't silently collide on field numbers, message shapes, or semantics.

**Scope:** Any file under `internal/shared/proto/v1/**`. The lock is held at the repo level — there is **one** lock, not one per file.

**Why this exists:** Seam #2 in the `tech-lead` agent says Connect-Go `.proto` is the only inter-role contract. Multiple agents working in parallel cannot negotiate schema changes with each other, so without a serialization point they will race on field numbers and ship silently-incompatible wire formats. The proto-lock is that serialization point.

See also: `.claude/agents/tech-lead.md`, `docs/superpowers/specs/2026-04-07-v1-roadmap.md`.

---

## Components

| File                               | Role                                                       |
| ---------------------------------- | ---------------------------------------------------------- |
| `.proto-lock.json`                 | The mutex itself. Source of truth. Never edit by hand.     |
| `scripts/proto-lock.sh`            | Helper that agents call to acquire/release/check the lock. |
| `.github/workflows/proto-lock.yml` | Server-side enforcement on every proto PR.                 |
| `docs/proto-lock.md`               | This file.                                                 |

## Lock file schema

```json
{
  "$schema_version": 1,
  "holder": "KAI-238",
  "kai_id": "KAI-238",
  "agent": "onprem-platform",
  "reason": "scaffold initial proto schemas",
  "acquired_at": "2026-04-07T22:00:00Z",
  "expires_at": "2026-04-08T00:00:00Z",
  "protos_touched": ["recorder_control.proto", "directory_ingest.proto"],
  "notes": "..."
}
```

When free, `holder` is `null` and the timestamp fields are also `null`.

Default TTL is **2 hours** (configurable via `PROTO_LOCK_TTL_HOURS`). After expiry, cleanup releases the lock automatically.

## Protocol (normal path)

```
Agent                         Lock repo (main branch)
-----                         -----------------------
1. acquire KAI-X              → opens proto-lock/acquire-kai-x PR
                                (only modifies .proto-lock.json)
                              ← CI runs proto-lock.yml:
                                - confirms base lock is free
                                - confirms only .proto-lock.json changed
                              ← PR auto-merges

2. (agent pulls main)         — now the agent holds the lock

3. edit protos in a worktree  — normal feature work
   commit with trailer:
     Proto-Lock-Holder: KAI-X

4. add release commit         — set holder back to null in the
   in the same branch,          same feature branch, includes
   bundled into the real PR     "Proto-Lock-Release: KAI-X" trailer

5. open feature PR            → CI runs proto-lock.yml:
                                - verifies Proto-Lock-Holder trailer
                                - verifies holder matches main HEAD
                              ← PR reviewed + merged
                              — lock is now free again
```

**Why two merges:** The acquire PR is intentionally separate so the lock file change is the single atomic point of serialization. Merge queue should serialize lock PRs against each other; the real proto PR doesn't need to be queued as long as it carries the trailer.

## Protocol (stale-lock cleanup)

A scheduled agent runs `scripts/proto-lock.sh cleanup` every 30 minutes (once `/schedule` is wired). If it finds an expired holder, it opens a `proto-lock/cleanup-<timestamp>` PR that releases the lock with a `Proto-Lock-Cleanup:` trailer.

If the original holder is still working, they must re-acquire the lock, rebase onto the cleaned-up main, and continue. This is the correct outcome: agents that took >2h on a proto change were probably stuck and need to re-plan.

## Script reference

```bash
# Check current state
scripts/proto-lock.sh status

# Acquire (opens a PR, waits for merge)
scripts/proto-lock.sh acquire KAI-238 onprem-platform \
  "scaffold initial proto schemas" \
  recorder_control.proto directory_ingest.proto

# Acquire without opening a PR (CI or local testing)
PROTO_LOCK_NO_PR=1 scripts/proto-lock.sh acquire KAI-238 onprem-platform "..." ...

# Dry-run — shows what would happen without mutating anything
PROTO_LOCK_DRY_RUN=1 scripts/proto-lock.sh acquire KAI-238 onprem-platform "..." ...

# Verify current holder (CI use)
scripts/proto-lock.sh check KAI-238

# Release (after your feature PR ships)
scripts/proto-lock.sh release KAI-238

# Expire stale locks
scripts/proto-lock.sh cleanup
```

Environment variables:

| Variable               | Effect                                               |
| ---------------------- | ---------------------------------------------------- |
| `PROTO_LOCK_TTL_HOURS` | How long the lock is held before expiry (default: 2) |
| `PROTO_LOCK_NO_PR`     | Update the lock file in place without opening a PR   |
| `PROTO_LOCK_DRY_RUN`   | Print what would happen without touching anything    |

## Commit message trailers

Real proto PRs **must** carry one of these trailers on at least one commit:

- `Proto-Lock-Holder: <KAI-id>` — required on any commit that modifies `internal/shared/proto/**`. Declares the lock holder.
- `Proto-Lock-Release: <KAI-id>` — optional convention for the release commit.
- `Proto-Lock-Acquire: <KAI-id>` — added automatically by the acquire script.
- `Proto-Lock-Cleanup: <KAI-id>` — added automatically by cleanup.
- `Proto-Lock-Agent: <agent-name>` — informational; which team agent held the lock.

The CI workflow reads the `Proto-Lock-Holder:` trailer via `git log -1 --format=%B`. Use the standard trailer format (one per line, no leading space).

## Required GitHub repo settings

For the lock to work bulletproof, configure:

1. **Merge queue** on `main`, scoped to the `proto-lock` label — serializes lock-acquire PRs against each other so CAS is atomic.
2. **Branch protection** on `main` requiring the `proto-lock / enforce` check on any PR that touches `internal/shared/proto/**` or `.proto-lock.json`.
3. **Bot account** (optional but recommended) — scheduled cleanup and autonomous agents should push under a dedicated service account so human commits remain identifiable.

Without merge queue, the lock still works but there's a small window where two agents can both see a free base and both open acquire PRs. The CI check will catch the second one (its base will show the lock held), but only after a merge round-trip.

## Failure modes and recovery

| Symptom                                                       | Cause                                                           | Fix                                                      |
| ------------------------------------------------------------- | --------------------------------------------------------------- | -------------------------------------------------------- |
| `proto-lock: lock is HELD on main — refusing to acquire`      | Another agent holds the lock                                    | Wait, or run `cleanup` if expired                        |
| `proto PR is missing a 'Proto-Lock-Holder:' trailer`          | Forgot to acquire, or trailer on wrong commit                   | Acquire lock, add trailer to your commits, force-push    |
| `proto PR trailer does not match current lock holder on main` | Lock was reclaimed (probably by cleanup) while you were working | Re-acquire, rebase                                       |
| Acquire PR fails CI with "base commit shows lock held"        | Another agent got there first                                   | Wait for their PR to merge, then retry                   |
| Lock expired mid-work                                         | Took too long                                                   | Re-acquire and rebase; investigate why the work ran long |

## Phase 2 enhancements (not implemented in MVP)

These would further reduce the risk of semantic collisions but aren't required for basic serialization:

1. **Field-number reservation** — the acquire command takes `--reserve msg:7,8` args and the CI check verifies no proto PR uses field numbers outside the reservation.
2. **`buf breaking` check** — wire-format compatibility against the previous main.
3. **Proto-lock coordinator agent** — a single agent that is the exclusive writer, with other agents filing "proto change requests" that it processes serially.
4. **Linear-backed lock status** — a dedicated Linear issue (`KAI-PROTO-LOCK`) mirrors the lock state so integrator portal dashboards can show it.
5. **Audit log** — every acquire/release/cleanup emits a structured log line into the project's observability pipeline.

MVP gives you correctness. Phase 2 gives you polish.
