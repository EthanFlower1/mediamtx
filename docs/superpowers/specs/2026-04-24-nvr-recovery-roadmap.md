# NVR Recovery Roadmap (post-decomposition)

**Date:** 2026-04-24
**Branch:** `feat/nvr-decomposition`
**Status:** planning

## Context

The decomposition from monolithic `internal/nvr/` (366 .go files) into Directory + Recorder + Cloud Broker is incomplete. Two earlier scans inventoried the damage:

- **Recorder doesn't record.** `noopCaptureManager{}` at `internal/recorder/boot.go:425` accepts assignments but does nothing. `getCert` returns an empty `tls.Certificate{}` (mTLS bypass).
- **Recorder API isn't reachable.** Routes mount at root (`/recordings`) instead of `/api/nvr/recordings`; no JWT middleware. Flutter calls 404.
- **Directory legacynvrapi has 7 sub-routers terminating in 501** (`notImplemented`): recordings sub-paths, timeline intensity, screenshot capture, system/*, auth/*, detections/*, ONVIF sub-paths.
- **Auth bypass.** `X-Recorder-ID` and `X-User-ID` headers accepted with no validation. `/api/v1/cameras` Gin router has only `gin.Recovery()`.
- **Cloud relay path mismatched.** Resolve emits `/session/{id}`; relay handler serves `/relay/{id}/{side}`. `SessionManager.Create()` is never called.
- **Orphaned packages built but never wired:** Directory (`federation/`, `webhook/`, `groupsync/`, `revocation/`, `authz/`, `zitadel/`) and Recorder (`talkback/`, `backchannel/`, `thumbnail/`, `scheduler/`, `storage/`, `recovery/`, `integrity/`, `alerts/`, `connmgr/`, `archive/`, `managed/`).
- **Flutter masks errors as "no data"** with empty catches throughout, plus stubbed seams (`HttpStreamsApi`, `HttpPlaybackClient`, `HttpPtzApi`, `StreamUrlMinter`, push channels).

**Good news from archaeology:** The 9 orphaned recorder packages are byte-for-byte copies of the legacy implementations (only import-path rewrites). The "broken" handlers in `legacynvrapi` mostly exist verbatim in git history at `git show 86569ce37^:internal/nvr/api/<file>`. This is a port-and-rewire job, not a redesign.

## Goal

Restore the functionality the Flutter client expects, in the new split architecture, without rewriting it from scratch. Use the legacy implementations as reference.

## Existing related plans (already authored, status varies)

These overlap the recovery effort. We should review them before adding new plans:

- `docs/superpowers/plans/2026-04-20-legacynvrapi-completion.md` — explicitly about closing the 501 gaps
- `docs/superpowers/plans/2026-04-20-directory-recorder-consolidation.md` — split work
- `docs/superpowers/plans/2026-04-20-legacydb-elimination.md` — DB layer cleanup
- `docs/superpowers/plans/2026-04-22-cloud-connector.md` — cloud connect/proxy
- `docs/superpowers/plans/2026-04-24-frp-tunnel.md` — frp embedding (recently merged)
- `docs/superpowers/specs/2026-04-20-directory-recorder-consolidation-design.md` — design baseline

## Phases

Each phase produces working, testable software on its own. Phases are ordered by dependency: nothing in a later phase works correctly without the earlier phases.

### Phase 1 — Recorder capture loop (FOUNDATION)
**Plan:** `docs/superpowers/plans/2026-04-24-recorder-capture-loop.md`
**Why first:** Nothing else matters until cameras actually record. The orphaned scheduler/storage/recovery/integrity/thumbnail/connmgr/alerts packages are drop-in ready; a thin `CaptureManager` adapter translates the new imperative interface to the legacy declarative yaml-write model.
**Outcome:** Real cameras record fMP4 segments to disk, recovery+integrity scanners run, thumbnail generator runs, scheduler enforces recording rules.

### Phase 2 — Recorder API surface (`/api/nvr/*` handlers + JWT)
**Plan:** TBD
**Depends on:** Phase 1
**Scope:** Mount recorder Gin router under `/api/nvr/*` with JWT middleware. Restore `HLSHandler` (port from legacy), `ThumbnailHandler`, `ScreenshotHandler.Capture` (real impl), VoD segment download, export endpoints, snapshot. Wire `talkback/` and `backchannel/` HTTP handlers.
**Outcome:** Flutter live snapshot, playback, thumbnails, exports work against the recorder.

### Phase 3 — Directory `legacynvrapi` completion (replace 501s)
**Plan:** Continuation of `2026-04-20-legacynvrapi-completion.md`; new plan if that's stale.
**Depends on:** Phase 2 (some handlers proxy to recorder)
**Scope:** Replace `notImplemented` in recordings/auth/system/detections/camera-ONVIF sub-routers with real handlers ported from `git show 86569ce37^:internal/nvr/api/<file>`. Fix unhashed-password user creation, raw-bearer-token user-ID partitioning, dropped export format field.
**Outcome:** No 501s for any documented Flutter API call.

### Phase 4 — Auth & mTLS hardening
**Plan:** TBD
**Depends on:** independent of Phases 1-3 but should land before any external exposure
**Scope:** Implement real `getCert` (KAI-242) using `internal/recorder/certmgr` or stepca. Replace `X-Recorder-ID` / `X-User-ID` header bypass with bearer-token validation. Wire `internal/directory/authz/` Casbin middleware to admin routes. Audit-sink for pairing (KAI-233).
**Outcome:** No header-based auth bypass; mTLS ingest streams authenticate.

### Phase 5 — Cloud connect + relay path
**Plan:** Continuation of `2026-04-22-cloud-connector.md`
**Depends on:** Phase 1, Phase 2
**Scope:** Fix relay URL mismatch (`/session/{id}` vs `/relay/{id}/{side}`). Add session creation endpoint that mints `session_id` for both sides. Implement real `CameraRegistry` (KAI-249). Fix proxy handler (forward request body, multi-port, configurable timeout). Make frp `ServerAddr`/ports configurable. Fix `tunnel_test.go` compile error.
**Outcome:** Cloud-tunneled access for Flutter works; relay session lifecycle exists.

### Phase 6 — Flutter transport + error surfacing
**Plan:** TBD
**Depends on:** Phases 2, 5
**Scope:** Fix WHEP URL resolution (don't assume same port), pipe STUN/TURN ICE config from server. Fix WebSocket URL/auth (don't assume `apiPort+1`, send Bearer). Replace silent `catch (_)` with real error reporting in cameras/onvif/playback/websocket/auth providers. Wire or remove the new-architecture stubs (`HttpStreamsApi`, `HttpPlaybackClient`, `HttpPtzApi`, `StreamUrlMinter`).
**Outcome:** Flutter live view + playback work end-to-end; failures surface as real errors, not blank screens.

### Phase 7 — DEFERRED — Orphaned admin features (federation, webhooks, notification prefs, zitadel, group sync, revocation, push notifications)
**Status:** Out of scope for this recovery effort. The user explicitly de-prioritized these to focus on foundational record/playback/auth/transport. Revisit only after Phases 1–6 are solid and shipping.

## Recommended execution order

Phase 1 → Phase 2 → (Phase 3 ∥ Phase 4) → Phase 5 → Phase 6

Phases 3 and 4 can run in parallel.

## Working agreement

- One worktree per phase: `.worktrees/recovery-phase-N` on branch `feat/recovery-phase-N-<short-name>`.
- Each phase's plan is TDD (red-green-commit per task) per the project's `superpowers:writing-plans` skill.
- Pull legacy implementations via `git show 86569ce37^:<path>` and adapt rather than rewrite.
- After each phase, smoke test against the Flutter client.
