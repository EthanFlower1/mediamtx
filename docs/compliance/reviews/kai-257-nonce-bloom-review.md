# KAI-257 Nonce Bloom Filter — Security Review

**PR:** #156 `feat/kai-257-nonce-bloom-filter`
**Reviewer:** lead-security
**Date:** 2026-04-08
**Scope:** `internal/shared/nonce/` (filter.go, doc.go, filter_test.go — 652 LOC)
**Verdict:** **APPROVE-WITH-FOLLOWUPS** (do NOT wire into production replay-protected paths until followups F1, F2, F6 land)

---

## 1. File Walkthrough

### `internal/shared/nonce/filter.go` (325 LOC)

| Lines | Element | Notes |
|---|---|---|
| 7 | `import "hash/maphash"` | **CRITICAL FINDING.** Hash is `hash/maphash`, NOT a keyed cryptographic hash. See §3.2. |
| 22-46 | `window` | Plain `[]uint64` bitset. `setBit`/`hasBit` use `idx % mBits`. No leaking metadata. Thread-unsafe — protected by Filter.mu. |
| 59-71 | `Filter` struct | Two-window sliding design (active + previous). Single `sync.Mutex` — coarse but correct. |
| 98-151 | `New(capacity, fpRate, ttl, opts...)` | Sizing math is standard and correct: `m = -n ln(p)/ln²(2)`, `k = (m/n) ln 2`. `k` capped at 32 (reasonable). Seeds generated via `maphash.MakeSeed()` at construction (per-process random). |
| 115 | `mBits` computation | Correct. Example: n=1M, p=0.001 → ~14.37 Mbits, k=10. |
| 128-131 | Seed init | `maphash.MakeSeed()` — process-local random, NOT persisted, NOT derived from a configured secret. See §3.2. |
| 145-149 | Background goroutine | Auto-rotates every `ttl/2`. `Close()` shuts it down deterministically. |
| 156-167 | `Close` | Idempotent, fail-closed after close. Good. |
| 169-185 | `rotateLoop` | Uses wall-clock `time.Ticker` (not the injected `Clock`). Real clock usage is fine in prod but is inconsistent with the Clock abstraction — test determinism relies on `WithoutBackgroundRotation`. |
| 187-198 | `Rotate()` | Two-generation rotation: `previous = active; active = fresh`. **Meets requirement 3** (2 active generations, boundary nonces remain visible at least `ttl/2`, at most `ttl`). |
| 200-211 | `hashIndices` | **k independent hashes via k independent seeds.** This is sound double-hashing avoidance — each seed gives an independent 64-bit hash. No Kirsch-Mitzenmacher g(i)=h1+i·h2 shortcut (which would be vulnerable). |
| 218-231 | `Check` | Non-mutating lookup. Allocates `idx` slice per call — minor GC pressure under load, not a bug. |
| 233-254 | `checkLocked` | Checks active THEN previous. Short-circuit on first missing bit — see §3.7 (timing side-channel). |
| 256-274 | `Add` | No check — pure insert. |
| 276-300 | `CheckAndAdd` | Atomic under mu. Correct. This is the intended production entry point. |
| 302-308 | `CheckAndAddString` | Allocates; comment acknowledges the copy. Fine. |
| 310-324 | `Stats` | Exposes sizing only. **Does NOT expose fill estimate / insertion count** — see §3.5 (capacity overflow). |

### `internal/shared/nonce/doc.go` (37 LOC)
Package doc is clear and correctly identifies this as a security primitive. States FPR and sizing math. **Missing: capacity ceiling guidance, restart behavior, multi-instance behavior.**

### `internal/shared/nonce/filter_test.go` (290 LOC)
Covers: basic insertion, distinct nonces, rotation expiry, FPR measurement (loose 1% bound), concurrent race, duplicate rejection under contention, nil/closed fail-closed, background rotation smoke, random nonces. See §4.

---

## 2. Checklist Verdict — 8 Requirements

| # | Requirement | Status | Notes |
|---|---|---|---|
| 1 | FPR documented & tunable | **PASS** | `New(capacity, fpRate, ttl)` parameterizes both. Doc.go documents math. Default 0.001 meets auth target (≤10⁻³); high-value (10⁻⁹) is achievable but caller must size explicitly. Recommend doc update with a table. |
| 2 | Append-only, time-windowed (no deletion) | **PASS** | Filter exposes no Remove/Delete. Aging is by generation rotation only. |
| 3 | ≥2 active generations, boundary nonces still seen | **PASS** | Two-window design (active + previous) queried via OR. Nonces visible ttl/2–ttl. Correct. |
| 4 | Cryptographically keyed hash (BLAKE2/SipHash w/ secret) | **FAIL (F1)** | Uses `hash/maphash`. maphash is *seeded per process with crypto/rand* internally, which does defeat **offline** precomputation, BUT: (a) maphash is explicitly documented as NOT cryptographic and its security properties are not guaranteed across Go versions; (b) seeds are process-local so distinct replicas have distinct seeds (actually helps) but cannot be rotated, persisted, or shared; (c) an attacker with any seed-leak oracle (e.g., a FPR-rate statistical oracle over many queries) can still grind. **Must replace with SipHash-2-4 keyed with a runtime-injected secret** (e.g., derived from `nvrJWTSecret` via HKDF) or `blake2b.New(k, key)`. |
| 5 | Capacity ceiling + alarm | **FAIL (F2)** | No insertion counter. No fill-ratio estimate. No metric exposed. Operators cannot detect when the filter is oversubscribed and FPR has inflated from 10⁻³ toward 10⁻¹. **Add a `Load()` method** (approximate fill ratio via popcount sampling or running insert counter) **and Prometheus gauges** for inserts-per-window, estimated fill, and observed rotation interval. |
| 6 | Persistence / restart behavior | **FAIL (F3)** | In-memory only. On process restart, state is lost and the replay window reopens immediately. For a 5-minute TTL this means a 5-minute replay window on every Directory restart. **Mitigation options:** (a) startup cooldown equal to nonce validity window (reject all nonces for `ttl` after start), (b) snapshot bitset to disk every ttl/4 and reload on start, (c) require callers to also enforce timestamp-based validity shorter than server restart MTBF. Option (a) is the simplest and should be the default. |
| 7 | Multi-instance behavior | **FAIL (F4)** | Filter is per-process. Nothing in this PR addresses multi-Directory-replica behavior. With per-instance independent filters and a load balancer, an attacker can replay the same nonce once per replica (N-replica amplification of the replay window). **Decision needed from lead-devex + lead-platform:** consistent-hash nonces by tenant to pin a tenant to one replica, OR move shared state to ElastiCache Redis (KAI-217), OR enforce replay-window shorter than LB session stickiness. Out of scope of this primitive but **must be documented as a caller contract** before KAI-260/KAI-261 wire it up. |
| 8 | Clock skew bound | **OUT-OF-SCOPE-BUT-UNADDRESSED (F5)** | Filter itself does not process nonce timestamps — it only tracks seen/unseen. Caller must bound accepted timestamp drift. Doc.go does not say this. **Add caller-contract note:** "callers MUST reject nonces with timestamps outside `±ttl/2` of server time before calling CheckAndAdd." |

**Score: 3 PASS, 4 FAIL, 1 OUT-OF-SCOPE.**

---

## 3. Attack Scenarios

### 3.1 Adversarial false-positive flood (DoS legitimate users)
**Scenario:** Attacker crafts nonces that collide in the bloom bitset, filling many bits so that later *legitimate* nonces hit pre-set bits and are rejected as replays.
**Current code:** Without a keyed hash (see 3.2) an attacker who learns the hash function can precompute nonces that set maximally diverse bits. With `maphash`, the per-process seed makes naive offline grinding hard, but online grinding via statistical oracles is feasible if the attacker has any replay-outcome feedback.
**Status:** Partially mitigated by maphash seeding, not mitigated structurally. **Fix F1** (keyed hash) closes this.
**Test:** NOT PRESENT. No test attempts to overfill the filter with targeted nonces and measure legitimate-user FPR degradation.

### 3.2 Hash without secret key → precomputed collision
**Scenario:** Offline attacker computes nonce set that produces N collisions in the filter, then submits them to forcibly shift FPR or pre-poison specific bit patterns.
**Current code:** `hash/maphash` uses a per-process random seed set at `MakeSeed()` time — this prevents pure offline attack but is NOT designed as a cryptographic MAC. Go's maphash docs explicitly state "Hash is not a cryptographic hash function." If Go changes maphash implementation (e.g., for performance), security properties could degrade silently.
**Status:** **FAIL.** Must migrate to SipHash-2-4 (`github.com/dchest/siphash` or `aead.dev/siphash`) or keyed BLAKE2b with a key derived from `nvrJWTSecret`. Key must be constant for the filter's lifetime (rotating hash keys invalidates the bitset).
**Test:** NOT PRESENT.

### 3.3 Restart replay window
**Scenario:** Attacker waits for or induces Directory restart, then replays a captured nonce immediately.
**Current code:** Filter starts empty. Accepts previously seen nonces.
**Status:** **FAIL (F3).**
**Test:** NOT PRESENT. Should add: construct filter, Add(nonce), close, construct new filter with same seed-derivation — first test demonstrates the vulnerability; then startup-cooldown implementation must close it.

### 3.4 Generation-boundary bypass
**Scenario:** Attacker inserts nonce N at time T, waits until T + ttl - ε (just before second rotation) and replays.
**Current code:** At T, N is in active. At T+ttl/2, rotation → N is in previous. At T+ttl, rotation → previous dropped. Between T+ttl/2 and T+ttl, N is still in `previous` and Check reports seen.
**Status:** **PASS.** `TestRotation_OldNoncesExpire` (lines 80-114) exercises exactly this boundary.

### 3.5 Multi-instance desync
**Scenario:** Two Directory replicas behind a load balancer without sticky sessions. Attacker replays nonce; LB routes first request to replica A (accepted, added to A's filter), replay routed to replica B (accepted, B doesn't know).
**Current code:** No cross-instance sharing. Vulnerable.
**Status:** **FAIL (F4).**
**Test:** NOT PRESENT (nor is it easily writable at this layer).

### 3.6 Capacity overflow silent failure
**Scenario:** Traffic spike pushes insertions to 10x configured capacity. FPR inflates from 10⁻³ to ≫10⁻¹. Legitimate users begin to be rejected as replays. No alarm fires.
**Current code:** No fill tracking. `Stats()` exposes only static sizing.
**Status:** **FAIL (F2).**
**Test:** NOT PRESENT. Should have a test that inserts 10×capacity distinct nonces and asserts FPR goes up — the point being that operators must be warned, not that the filter should magically keep working.

### 3.7 Timing side-channel on `Test` revealing set membership pattern
**Scenario:** `checkLocked` (lines 233-254) short-circuits on first missing bit in active window, then on first missing bit in previous window. Time-to-reject for a truly-absent nonce scales with "how many bits happen to already be set," leaking rough filter load and potentially per-nonce membership hints.
**Current code:** Short-circuit is present. The time variation is a function of the filter's aggregate fill, not of the *specific* nonce's hash pattern against the attacker's target, so this is a weak side-channel at best. Still, for a security primitive this should be constant-time per lookup: iterate all k indices regardless.
**Status:** **MINOR (F6).** Low-exploitability but should be fixed for defense-in-depth. Remove short-circuits; OR all k lookups into a single boolean accumulator.
**Test:** NOT PRESENT.

---

## 4. Test Coverage Assessment

| Scenario | Test Name | Present? | Adequacy |
|---|---|---|---|
| Basic insert/check | `TestCheckAndAdd_BasicInsertionLookup` | YES | Good |
| Distinct nonces | `TestDistinctNonces_AllUnique` | YES | Good (10K) |
| Rotation / expiry | `TestRotation_OldNoncesExpire` | YES | **Excellent** — covers generation boundary (§3.4) |
| FPR measurement | `TestFalsePositiveRate_Loose` | YES | Loose 1% bound; comment acknowledges this is loose. Missing a second test at full capacity asserting design FPR (~0.1%). |
| Concurrency race | `TestConcurrent_CheckAndAdd_RaceSafe` | YES | Good, paired with `-race` assumption |
| Concurrent duplicate | `TestConcurrent_DuplicateRejection` | YES | Good — 4-way race on same nonce, exactly-one-winner |
| Fail-closed nil | `TestNilFilter_FailClosed` | YES | Good |
| Fail-closed closed | `TestClosed_FailClosed` | YES | Good |
| Background rotation | `TestBackgroundRotation_Runs` | YES | Smoke only |
| Random nonces | `TestRandomNonces_NoDuplicatesReported` | YES | Good |
| **Adversarial FP flood (§3.1)** | — | **NO** | **Required** |
| **Hash key / grinding (§3.2)** | — | **NO** | **Required** |
| **Restart replay (§3.3)** | — | **NO** | **Required** |
| **Multi-instance desync (§3.5)** | — | N/A | Caller contract test instead |
| **Capacity overflow alarm (§3.6)** | — | **NO** | **Required** |
| **Timing side-channel (§3.7)** | — | **NO** | Nice-to-have |

**Verdict: test coverage is strong for functional correctness, weak for adversarial scenarios.** Adding the four "Required" tests above is a precondition for wiring this into KAI-260 / KAI-261.

---

## 5. Reusability for KAI-397 Webhook HMAC Replay Protection

**Question from lead-devex:** Can this primitive be reused for outbound webhook HMAC replay protection (KAI-397) and inbound webhooks (KAI-398)?

**Short answer: YES with changes — this is the right shape, but the same F1–F4 followups apply and the reuse actually makes F4 more urgent.**

**Fit analysis:**
- KAI-397 outbound webhooks sign with HMAC-SHA256 over `(timestamp, nonce, body)`. The *receiver* needs replay protection — not the sender. So KAI-397 the sender side needs a nonce *generator* (crypto/rand), not this filter. The filter is applicable to **KAI-398 inbound webhooks** (Kaivue receives webhooks, must reject replays) and to the *receiver-side reference implementation we ship to customers* for KAI-397.
- The data shape is identical: short-lived unique identifier per request, window of 5–15 minutes, FPR ≤10⁻⁶ (one webhook misrejected per million is acceptable, though a followup could tighten).
- The API (`CheckAndAdd(nonce []byte)`) is directly usable.

**Changes required before KAI-397/398 can depend on this:**
1. **F1 (keyed hash)** — **mandatory.** Webhook replay tolerance is lower than auth because webhook secrets may leak more readily (customer misconfigurations). Non-keyed hash is not acceptable.
2. **F3 (restart cooldown)** — **mandatory.** Webhook endpoints restart more often than auth services and attackers have more time to queue replays during rolling deploys.
3. **F4 (multi-instance)** — **mandatory for webhook ingest.** KAI-398 inbound webhook handler will run across many replicas; without shared state or consistent hashing the replay window is N-amplified. Recommend per-tenant consistent hashing to pin to one replica, OR (preferred for future) Redis-backed shared bloom.
4. **Higher default FPR target for webhooks.** Add `NewForWebhooks(...)` helper constructing with p=10⁻⁶ and appropriate capacity sizing.
5. **Per-tenant filter isolation.** Caller should instantiate one Filter per tenant to avoid cross-tenant nonce collision (a collision between tenants is not a security issue — nonces are scoped — but separation simplifies capacity math).

**Recommendation to lead-devex:** Reuse is approved conditional on F1/F3/F4 landing. Do NOT block KAI-397/398 design work on F1–F4 now, but gate the merge of either ticket on them. Flag this primitive as "shared between KAI-257 and KAI-397/398" in the dependency graph.

---

## 6. Verdict

### **APPROVE-WITH-FOLLOWUPS**

**Rationale:** The design is correct at the structural level — two-generation sliding bloom, fail-closed semantics, concurrency-safe, well-tested for functional cases, and the sizing math is right. The package is a sound starting point. However, **it is NOT yet safe to wire into a replay-protected production path.** Four of eight security requirements are unmet.

### Required followups (tracked as KAI-257 blockers for KAI-260, KAI-261, KAI-397, KAI-398):

| ID | Blocker? | Description |
|---|---|---|
| **F1** | **YES** | Replace `hash/maphash` with a keyed cryptographic hash (SipHash-2-4 keyed from `nvrJWTSecret` via HKDF, or `blake2b.New(k, key)`). Document that hash key is constant for filter lifetime and is rotated only via filter reconstruction (fresh windows). |
| **F2** | **YES** | Add fill tracking (insert counter + popcount sampling) and expose Prometheus metrics: `nonce_filter_inserts_total`, `nonce_filter_estimated_load`, `nonce_filter_rotations_total`. Emit `WARN` log + alert when load > 1.5× design capacity. |
| **F3** | **YES** | Implement startup cooldown: reject all nonces for the full TTL after filter construction (configurable, default on). Alternative: bitset snapshot to disk. |
| **F4** | **CONTRACT** | Document caller contract: "Filter is per-process. Deployments with multiple replicas MUST pin nonce validation to a single replica per tenant (consistent hashing), use shared state (Redis), or accept the N-replica replay window amplification." Open decision ticket with lead-devex + lead-platform. |
| **F5** | **DOC** | Document clock-skew caller contract in doc.go: nonces carry timestamps and callers MUST bound drift to ±ttl/2. |
| **F6** | NO | Make `checkLocked` constant-time (no short-circuit) for side-channel defense in depth. |
| **F7** | NO | Add adversarial unit tests: FP flood (§3.1), simulated restart replay (§3.3), capacity overflow alarm-fires (§3.6). |

### Merge gates:
- **Merge of KAI-257 PR #156 to main:** permitted after F1, F2, F3, F5, F7 land. F4 doc and F6 may follow within 1 sprint.
- **Wire into KAI-260/261 auth webhook:** blocked until F1, F2, F3, F4, F5, F7 all closed.
- **Wire into KAI-397/398 webhook replay:** blocked until F1, F2, F3, F4 all closed; F4 must be Redis-backed OR consistent-hash-pinned (not "document and ignore").

### Do not merge while lead-security freeze is active (reviewer note):
Per active-freeze directive, this review does not itself merge the PR. This document is the merge prerequisite. Handoff back to the PR author to implement F1–F3, F5, F7.

---

**Signed:** lead-security
**Review version:** 1.0
**Re-review required:** Yes, after F1 (hash change) — hash change is structurally significant.
