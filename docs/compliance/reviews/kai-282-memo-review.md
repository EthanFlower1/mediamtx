# KAI-282 Face Recognition Memo — Lead-Security Review

**Reviewer:** lead-security
**Memo:** `docs/superpowers/specs/2026-04-08-kai-282-face-recognition-eu-ai-act.md`
**Branch / PR:** `docs/kai-282-face-recognition-memo` / draft PR #185
**Date:** 2026-04-08
**Deadline context:** CE marking hard gate 2026-08-02; KAI-390 pen test + KAI-385 SOC 2 Type I gate GA.

---

## 1. Summary verdict

**APPROVE-WITH-CHANGES.**

The memo is structurally sound and correctly defers the three decisions that are legitimately lead-security's call (base model, encrypted-search scheme, conformity path). The non-negotiables B1–B7 are present and mostly mapped to code-level enforcement. However, the memo has material gaps in biometric-specific threat modeling (template inversion, side channels, enrollment poisoning), does not yet cite the KAI-294 + KAI-293 merged compliance package by path, and leaves §5's encrypted-search design as an open gate with no candidate recommendation. None of these are blockers to memo signoff, but all must be tracked as MUST-CHANGE items before implementation begins.

**EU AI Act article coverage (memo-level, pre-implementation):**

| Article | Coverage | Notes |
|---|---|---|
| Art. 9 (risk mgmt) | Partial | Cites KAI-294 `risk-management-system.md` but does not enumerate top-5 residual risks here. SHOULD-CHANGE. |
| Art. 10 (data governance) | Partial | §3 correctly defers per-model provenance but the inherited-vs-measured fairness convention is strong. Training-set provenance for Path A/B is still open. |
| Art. 13 (transparency) | Adequate | §1 + §10 map cleanly to instructions-for-use doc. |
| Art. 14 (human oversight) | Adequate | §7 meets requirements; stop-condition at 100% confirm rate is a nice touch. |
| Art. 15 (accuracy/robustness/cybersecurity) | Partial | §8 covers the basics. Missing: constant-time nn-search, model-extraction rate-limiting, template-inversion resistance claim. MUST-CHANGE. |
| Art. 43 / Annex VI (conformity) | Deferred | §11 asserts internal control; legal counsel sign-off required before this becomes final (see Q5 below). |

---

## 2. Per-section findings

### §1 Scope and intended purpose
- **Strong.** The explicit "refuses to ship" framing for Art. 5 prohibited uses is the right posture; couples memo language to §6 code-path refusals.
- **Concern:** §1 describes the system as "post-hoc face recognition" but §4's diagram shows the embedder running in the real-time recorder pipeline. The distinction between *matching in real time against an enrolled vault* vs *real-time remote biometric identification in publicly accessible spaces* (Art. 5(1)(h)) is legally narrow. **MUST-CHANGE:** add one paragraph clarifying that match events from private/non-public-space cameras with opt-in-enrolled subjects do NOT constitute "remote biometric identification in publicly accessible spaces" per Recital 17 / Art. 3(42). Legal counsel should sign this paragraph.

### §2 Boundary conditions (B1–B7)
- See §3 below for per-invariant enforcement verification.
- **SHOULD-CHANGE:** add B8 — "No biometric category inference (race, religion, political opinion, etc.) from face embeddings or face crops, ever." Art. 5(1)(g) prohibits biometric categorization into Art. 9 GDPR special categories; we should refuse this at the code-path level the same way we refuse §6 items.

### §3 Base model selection
- The decision matrix is honest and well-constructed. Inherited-vs-measured fairness flag convention is excellent and should be hoisted into `data-governance.md`.
- **MUST-CHANGE:** matrix is missing a column for **training-set retraction / takedown risk**. VGGFace2, MS-Celeb-1M, and MS1MV2 have all been withdrawn by their original publishers; using derivative weights exposes us to a future data-source-unavailable event during an audit. This is a provenance concern distinct from licensing.
- **SHOULD-CHANGE:** lead-ai should produce a two-line diff between "Path A" and "Path B" on who owns the Annex IV evidence bundle — if a vendor fails to deliver their promised evidence, we are still the Art. 16 provider and on the hook.

### §4 System architecture
- Component reuse discipline is correct. No red flags.
- **Concern:** the diagram does not show where the **killswitch (B6)** intercepts the inference path. If the killswitch is only a config flag read at pipeline initialization, it will not take effect in ≤60s for workers that are already running. **MUST-CHANGE:** add a killswitch interception point at the *match engine* (post-embedding) with an in-memory TTL ≤10s on the tenant enablement cache, so that mid-inference requests fail closed within the B6 budget.

### §5 Data model
- Schemas look correct. `tenant_id` denormalization on `face_embeddings` is the right call for pgvector index scoping (Seam #4).
- **MUST-CHANGE — B5 crypto flow:** the memo says "a separate column containing the tenant-local projection" but leaves the scheme open. This is the single biggest unresolved security design question in the memo. See §5 below for my recommendation. Until a scheme is picked, B5 ("raw vectors never touch disk unwrapped") is a **documented requirement, not an enforced invariant**.
- **MUST-CHANGE — B3 FK pattern:** `consent_record_id` is described as "FK to audit_log (KAI-233)" but KAI-233's audit rows are append-only and are NOT currently a referenceable foreign-key target (they are event rows keyed by `(tenant_id, event_id, ts)`). Either (a) introduce a `consent_records` table with an `audit_event_id` back-pointer and enforce the FK there, or (b) store the audit_event_id as an opaque string with a check constraint verifying the event exists at enrollment time. Option (a) is cleaner and lets us enforce consent withdrawal via a `revoked_at` column.
- **SHOULD-CHANGE:** `face_match_events.confirmed_by` nullable + `confirmed_at` nullable is correct for Art. 14 human-in-the-loop, but there is no state for "explicitly rejected by reviewer." Add `rejection_reason` enum or a sibling `face_match_rejections` row — rejected matches are evidence for Art. 14 effectiveness monitoring.

### §5.1 Model version transitions
- **Strong.** The eager/opportunistic re-enrollment protocol with per-embedding `model_version` and mixed-result-never-aggregated rule is the right design and aligns with Annex IV §2(a).
- **Concern:** 90-day defer window + source-crop retention opt-in creates a situation where a tenant who opts *out* of source-crop retention AND defers 90 days is forced into manual re-capture. This is correct data-minimization but the memo should note this explicitly as a customer-facing trade-off.
- **SHOULD-CHANGE:** 10-year model retention per Art. 18 is cited for model artifacts, but the memo does not say what happens to the fairness-test-set evidence for the retired model. That evidence must also be retained for 10 years.

### §6 Code-path refusals
- **Strong.** No emotion endpoint defined, cross-tenant query builder has no escape hatch, bulk enrollment requires consent manifest. This is the right enforcement pattern.
- **SHOULD-CHANGE:** add a refusal for "biometric-categorization into Art. 9 GDPR categories" (see B8 suggestion above).

### §7 Human oversight
- Adequate. Stop-condition at 100% confirm rate is a good automation-bias circuit breaker.
- **Concern:** the "equal-weight not-a-match button" is a UX assertion the review will need to actually see in the KAI-327 design before GA. Not a memo issue, but flag for downstream review.

### §8 Accuracy, robustness, cybersecurity
- Frozen weights is correct. Signature check on load is correct.
- **MUST-CHANGE:** add the following Art. 15 cybersecurity controls:
  - **Constant-time similarity scoring** on the nn-search path (see §4.1 below).
  - **Model-extraction rate limiting** on the match endpoint per tenant + per API key (see §4.3 below).
  - **Template-inversion resistance** claim (see §4.4 — this is a GDPR-class question).
  - **Cross-tenant embedding collision** test case (see §4.5).
- **SHOULD-CHANGE:** FPR ≤1e-4 default threshold floor is good; add an explicit *per-tenant* audit log entry whenever a tenant raises their threshold above 1e-3 (a coarse threshold is a Art. 14 effectiveness concern).

### §9 Fairness
- 18-bucket protocol is reasonable. Equalised-odds gap ≤0.05 as release blocker is the right strictness.
- **SHOULD-CHANGE:** monthly drift re-evaluation cadence is too slow for a high-risk system on first 90 days post-GA. Recommend weekly during the first 90 days, monthly thereafter, per KAI-294 `post-market-monitoring.md`.

### §10 Provider/deployer split
- Correct reading of Art. 16/26. No changes.

### §11 Conformity assessment
- **DEFERRED — see Q5 below.** The memo's assertion of internal-control path is defensible but legally fragile given the harmonized-standards-not-fully-published state of face recognition. **MUST-CHANGE:** add an explicit note that §11's conclusion is contingent on legal counsel review and that a fallback to notified-body is scoped in the schedule.

### §12 What this memo does NOT commit to
- Good. No changes.

### §13 / §14 Open questions
- Answered in §5 below.

---

## 3. Non-negotiables B1–B7 code-level coverage

| # | Documented? | Enforced at code? | Verdict |
|---|---|---|---|
| B1 | Yes | **Partial.** Seam #4 per-tenant pgvector index + query-builder scope is real (KAI-292). Missing: a test case that attempts a cross-tenant query via a service-account token and verifies the query builder refuses. MUST-CHANGE: add this test as a gating CI check before KAI-282 implementation lands. |
| B2 | Yes | **Yes.** Two-flag AND gate is straightforward. |
| B3 | Yes | **No — FK pattern is not real as described.** KAI-233 is not FK-targetable. MUST-CHANGE per §5 above. |
| B4 | Yes | **Partial.** Memo leaves age-inference ownership open (Q3). Until Q3 is resolved, B4 is aspirational. |
| B5 | Yes | **No — crypto flow is not yet designed.** §5 encrypted-search scheme is open (Q2). Until Q2 is resolved, B5 is aspirational. |
| B6 | Yes | **Partial.** See §4 finding — killswitch needs a match-engine-level interception, not just a config flag. |
| B7 | Yes | **Yes.** KAI-233 covers this. |

**Summary:** B2 and B7 are real. B1 needs a CI test. B3, B5 need design resolution. B4, B6 need concrete mechanisms.

---

## 4. Security-specific concerns (gaps in the memo)

### 4.1 Vector similarity side channels
Nearest-neighbor search over a pgvector HNSW index is **not** constant-time by default. An attacker who can submit query embeddings and observe response latency can infer vault contents via timing analysis. For biometric data this is a GDPR-severity leak.

- **MUST-CHANGE:** match engine must either (a) batch all tenant queries to a fixed-size top-k and return in fixed wall-clock time, or (b) add a per-request random delay sized to the 99p latency. Option (a) is preferred.
- Also: exposing `similarity_score` in the API response for non-matches is a side channel. Only return `similarity_score` for confirmed matches; for non-matches return a boolean.

### 4.2 Adversarial robustness
Memo §8 mentions "presentation-attack test set (print, replay, mask)" but omits:

- **Adversarial patches** (printable patterns that cause misclassification) — must be in the pre-release test set.
- **Morphing attacks** (a face image blended from two identities that matches both) — a documented enrollment attack against access-control systems.
- **Digital adversarial perturbations** in the cloud-routed inference path.

**MUST-CHANGE:** expand §8 presentation-attack list to include adversarial patches + morphing. Recommend iBeta Level 1 as baseline (see Q4).

### 4.3 Model extraction attacks
With a query-based extraction attack, an adversary with unlimited match API access can clone the embedding function in ~10⁴–10⁶ queries depending on architecture. This is a theft-of-model-IP concern AND a defeats-the-killswitch concern (extracted model can be run offline).

- **MUST-CHANGE:** match API must have per-tenant + per-API-key rate limits sized to legitimate operational need (not developer convenience). Recommend starting at 100 match queries / minute / API key and tuning up with evidence.
- **SHOULD-CHANGE:** log queries with embeddings-unique-per-minute counts to detect extraction attempts.

### 4.4 Template-inversion attacks (GDPR-critical)
**This is the most important security finding in this review.** Recent research (e.g., NBNet, 2021+; diffusion-based inversion 2023+) has demonstrated that face embeddings from ArcFace-class models can be **inverted back to recognizable face images** with sufficient fidelity to fool both human recognition and a second face recognition system. If our stored embeddings are invertible, they are **biometric personal data in the strong sense under GDPR Art. 9**, not "anonymous features."

**Implication:** the memo's framing of embeddings-are-not-face-images is legally unsafe. The CSE-CMK story (B5) is *load-bearing* — if an attacker can exfiltrate plaintext embeddings, that is a biometric data breach, not a metadata breach.

- **MUST-CHANGE:** memo must explicitly state that stored embeddings are treated as Art. 9 GDPR special-category data AND that B5's encryption is the control making exfiltration of the at-rest vault survivable.
- **MUST-CHANGE:** DPIA update (lead-ai + lead-security joint sign) must explicitly list template inversion in the threat model.

### 4.5 Cross-tenant embedding collisions
Two tenants independently enrolling the same person will produce *similar* embeddings (not identical — detection/crop differences — but similar). If our indexing scheme is *globally* shared across tenants (which Seam #4 forbids but we should test), one tenant could infer whether a subject is enrolled in another tenant's vault by observing whether their enrollment was rejected as a duplicate, or whether a query latency changed.

- **MUST-CHANGE:** explicit test case in B1 coverage — enroll subject S in tenant T1, then attempt enroll of S in T2 and verify there is no observable signal (error, latency, or duplicate-hash) linking the two.

### 4.6 Enrollment poisoning
An attacker with enroll permission can submit a crafted embedding (or a crafted face image that produces a crafted embedding) designed to be near many identities in the embedding space — effectively a "skeleton key" that matches everyone above threshold.

- **MUST-CHANGE:** enrollment must validate that the submitted face crop produces an embedding within the expected distribution (norm, distance from centroid of known-good set). Reject outliers.
- **SHOULD-CHANGE:** after enrollment, sanity-check by running the new embedding against the existing vault and flagging any cross-match above 0.9 similarity — a legitimate new enrollment should not already be in the vault.

### 4.7 Audit log re-identification risk
Memo §2 B7 says "target subject id (pseudonymised)." But an audit log row that records `"camera-X at time-T matched enrolled identity #47 with score 0.92, confirmed by reviewer Y"` is effectively PII for identity #47 in the context of that tenant — the pseudonym is stable and linkable to other events.

- **MUST-CHANGE:** audit log retention policy for `face_match_events`-derived rows must be consistent with GDPR retention for biometric data, not with the default audit log retention. Recommend 2 years max for confirmed-match events unless tenant extends via explicit data-retention policy; rejected-match events should roll off in 90 days.

### 4.8 Killswitch race conditions (B6)
See §4 architecture finding. The memo asserts ≤60s but does not describe the propagation mechanism. Worker caches, in-flight inference requests, batched DB writes, and the transition between "killswitch flipped" and "purge scheduled" are all race windows.

- **MUST-CHANGE:** killswitch must fail closed — when a worker reads `enabled=false`, it drops all in-flight face recognition requests for that tenant WITHOUT writing a match event. B6's 60s budget is measured from admin action to "no new match events are being written."
- **SHOULD-CHANGE:** emit a tenant-scoped killswitch event on a pub/sub channel so workers don't rely on polling the config table.

### 4.9 Key rotation for tenant vault keys
KAI-251 supports key rotation but the memo does not describe what happens to stored embeddings when a tenant's CMK is rotated.

- **MUST-CHANGE:** add a §5.2 subsection describing the rotation flow: re-wrap all embeddings under the new KEK (HKDF-derived subkey per row), drop the old KEK reference, audit-log the rotation. This must be an online operation with bounded wall-clock (sub-minute for <10k enrollments, background for larger).
- **SHOULD-CHANGE:** force rotation on customer offboarding, so stored ciphertexts become cryptographically inaccessible even if backups linger.

### 4.10 Backup/restore and audit-trail continuity
Art. 12 + Art. 19 require continuous audit logs for high-risk AI systems. A DR restore from a backup taken *before* a killswitch action could re-enable a feature and lose audit log continuity.

- **MUST-CHANGE:** DR restore procedure must include a "replay killswitch state" step from the audit log forward, AND the audit log itself must be backed up on a different cadence from the vault data (audit log = continuous WAL ship; vault = periodic snapshot).
- **SHOULD-CHANGE:** post-restore, automatically write an audit event describing the restore point and delta, signed by the operator who authorized it.

---

## 5. Answers to the 5 gating questions (security perspective)

### Q1 — Base model choice: Path A (licensed training), Path B (pre-audited vendor), or licence-clean AdaFace?

**Recommendation: Path B (pre-audited vendor with CE support) as primary; Path A as fallback.**

From an Art. 10 data-governance and provenance standpoint:

- **Path B** hands us a complete Annex IV evidence bundle (training-data provenance, fairness results, robustness results, model card) contractually. This is the **lowest-risk path to the 2026-08-02 deadline**. Vendor-lock-in is a commercial concern, not a compliance concern, and commercial concerns lose to a hard regulatory deadline.
- **Path A** is defensible but we own the Annex IV evidence burden end-to-end. Cost and schedule risk are high; if our fairness testing reveals bucket gaps we cannot close, we have no fallback.
- **Licence-clean AdaFace** is *possibly* acceptable but only after a legal audit of the specific dataset variant used for the public weights AND a full re-measurement of fairness on our test set. The upstream fairness claims are **not sufficient** for Art. 10(3) because they are not our intended-use population. This path does not beat Path B on schedule or on risk.

Options 1 and 2 (InsightFace buffalo_l, FaceNet) are rejected per lead-ai's read.

**Lead-security verdict:** pursue Path B vendor RFP starting this week. Path A as contingency if no vendor meets the Annex IV delivery bar within 30 days.

### Q2 — Encrypted search scheme: CGKN/SSE vs tenant-key-derived permutation?

**Recommendation: tenant-scoped deterministic permutation + per-tenant HKDF-derived keys via KAI-251.**

CGKN / SSE schemes are academically interesting but production-hostile:
- Performance is 10–100× worse than plaintext pgvector
- Operational debugging is hell
- Implementation bugs are silent biometric data leaks

**The simpler production design:**

1. Use KAI-251 HKDF to derive a per-tenant subkey `K_tenant` from the tenant's CMK.
2. At enrollment, wrap the raw float32 embedding under `K_tenant` using AES-256-GCM → ciphertext goes to `face_embeddings.embedding_ciphertext` (stored, never used for search).
3. For the *search index column*, compute `Permute(embedding, K_tenant)` where `Permute` is a key-dependent deterministic function that preserves local cosine distance within a tenant's vault but is uncorrelated across tenants. A simple construction: an HKDF-derived random orthogonal rotation matrix per tenant applied to the unit-normalized embedding, optionally followed by sign flipping on a key-derived subset of dimensions.
4. At query time, apply the same `Permute` to the probe embedding using the same `K_tenant`, then run standard pgvector HNSW cosine nn-search.

**Security properties:**
- Raw plaintext vectors never touch disk. B5 holds.
- The permuted column is deterministic per tenant, so cross-tenant correlation is infeasible without the tenant key.
- Nearest-neighbor topology is preserved within tenant, so search quality is unchanged.
- An attacker who steals the permuted index without the CMK sees a random-looking high-dimensional matrix.

**Caveats:**
- This is NOT encryption in the formal-security sense (IND-CPA). It is obfuscation keyed to the tenant. For IND-CPA resistance against an attacker with chosen-plaintext access, CGKN/SSE is stronger — but we do not have that threat model (the attacker at rest is a disk-image grabber, not an oracle).
- This must be documented as an **engineered obfuscation, not a cryptographic encryption** in the Annex IV technical doc. Honesty with the auditor matters.
- The CMK-wrapped ciphertext column is the compliance-grade control. The permuted column is a performance accommodation.

**I will produce a separate KAI-251 design doc covering this scheme before implementation starts.**

### Q3 — Age inference ownership for the minors block (B4)

**Recommendation: shared ownership.**
- **lead-ai** implements the age-inference model (or selects a vendor component).
- **lead-security** reviews the model card, bias tests, and false-negative cost model, and signs the DPIA update covering the minors pathway.
- **Both** sign the carve-out review template in `risk-management-system.md` R3.
- **Fallback policy:** "if in doubt, block enrollment" is correct. Add: tenant attestation for override must be retained for 10 years per Art. 18.
- **Operational guardrail:** false-negative cost (enrolling a minor as adult) is categorically higher than false-positive cost (blocking an adult). Bias the age threshold accordingly — reject enrollment for any estimated age <21 unless tenant attestation is provided.

### Q4 — Presentation-attack test set: build or buy?

**Recommendation: buy baseline + supplement in-house.**

- **Buy:** iBeta Level 1 PAD certification (~$15k one-time, ~$5k annual maintenance). Gives us an auditor-friendly certificate that maps cleanly to ISO/IEC 30107-3 and is recognized by notified bodies.
- **Baseline public set:** NIST FRVT PAD test set as a continuous integration gate (free, reproducible).
- **Supplement in-house:** adversarial patches, morphing attacks, and Kaivue-specific camera-quality degradation scenarios (low-light, wide-angle distortion, motion blur). These are not covered by iBeta or NIST and are operationally relevant for our deployment profile.
- **Budget:** ~$25k year one, ~$10k annually.

### Q5 — Internal control (Annex VI) vs notified body?

**Recommendation: plan for internal control AS PRIMARY, but scope notified-body engagement as contingency. Legal counsel MUST review before final declaration.**

**Analysis:**

Art. 43(1) says that for Annex III high-risk systems, the provider may follow the internal-control procedure in Annex VI **unless** the provider has not applied harmonized standards (or common specifications) referred to in Art. 40/41, in which case the provider MUST follow the conformity assessment based on assessment of the quality management system and the technical documentation involving a notified body (Annex VII).

**The problem:** harmonized standards for face recognition are **not yet fully published**. The relevant candidates are:
- EN ISO/IEC 23894 (AI risk management) — published
- ISO/IEC 42001 (AI management system) — published
- ISO/IEC 25059 (software quality model for AI) — published
- ISO/IEC DIS 24027 (bias in AI systems) — still draft as of 2026-04
- ISO/IEC JTC 1/SC 37 face recognition specific parts — partially published, some parts still draft

Per Art. 43(1), if a harmonized standard covering the high-risk requirements is NOT applied in full, the provider falls into the notified-body path. The memo's §11 asserts harmonized standards coverage but this is **aspirational** given the current state of ISO drafts.

**Kaivue-specific analysis:**
- We are a **provider placing on market** (not a deployer using internally), so Art. 43(1) applies directly.
- We have strong coverage for AI management, risk management, and quality — those are published standards we can credibly claim full application of.
- We have **weak or no coverage** for face-recognition-specific requirements because those standards are not yet published in harmonized form. This is the risk.

**Concrete verdict:**
1. Plan the engineering + documentation work as if internal control will apply. The work is required either way.
2. Retain legal counsel with EU AI Act practice experience before 2026-05-15 to get a written opinion on Annex VI applicability given the state of harmonized standards at signing.
3. Scope a notified-body engagement as a contingency. If we need notified-body involvement, the ~4-month lead time means we cannot decide this after 2026-05-01.
4. **Flag for CTO:** if legal says "notified body required," the 2026-08-02 deadline is at serious risk. Have a conversation with the CTO about what the deadline represents (see §14 Q3 — shippable product vs conformity declaration) BEFORE we commit to a conformity path.

**Lead-security is NOT signing off on §11 as "internal control is correct" without legal counsel review. §11 must be annotated as conditional in the memo.**

---

## 6. Overlap with KAI-294 compliance package

The memo cites KAI-294 multiple times (§3, §7, §8, §9, §11) by document name but **does not cite file paths**. It also does not acknowledge the KAI-293 security compliance package at all.

**MUST-CHANGE:**
- Add a "Related compliance documents" subsection in §1 or at end of memo, listing the 18 compliance files by full path:
  - **KAI-294 (lead-ai's 8 unique):** Annex IV technical docs, conformity-assessment.md, ce-marking.md, provider-obligations.md, serious-incident-reporting.md, accuracy-robustness-cybersecurity.md, eu-database-registration.md, fundamental-rights-impact-assessment.md
  - **KAI-293 (lead-security's 3 unique):** audit-log-requirements.md, dpia-template.md, opt-in-consent-flow.md
  - **Shared (7 overlaps):** risk-management-system.md, data-governance.md, fairness-testing-protocol.md, human-oversight.md, transparency-and-information.md, post-market-monitoring.md, [seventh TBD post-merge]
- Target merge path: `docs/compliance/eu-ai-act/*.md`
- Add a merge-ownership note: who resolves conflicts when KAI-294 + KAI-293 both modify the 7 overlapping files.

---

## 7. Concrete changes needed to sign off

### MUST-CHANGE (blockers for memo signoff)

1. **§1** — Add the Recital 17 / Art. 3(42) "remote biometric identification in publicly accessible spaces" carve-out paragraph, with legal counsel sign-off.
2. **§2** — Add B8 refusing biometric categorization into Art. 9 GDPR special categories.
3. **§3** — Add training-set retraction/takedown risk column to base-model matrix.
4. **§4** — Document killswitch interception point at match engine with TTL ≤10s and fail-closed semantics.
5. **§5 / B3** — Resolve `consent_record_id` FK pattern; KAI-233 is not FK-targetable as described. Introduce `consent_records` table.
6. **§5 / B5** — Adopt the tenant-scoped deterministic permutation + HKDF scheme (or an alternative lead-security approves). Document it honestly as obfuscation, not encryption, in Annex IV.
7. **§5** — Document key rotation flow for tenant CMKs (new §5.2).
8. **§6** — Add refusal for biometric categorization (ties to B8).
9. **§8** — Add: constant-time nn-search, model-extraction rate limiting, template-inversion resistance claim, morphing attack test, adversarial patch test.
10. **§8** — Do not return `similarity_score` on non-match responses.
11. **New section** — Explicit statement that stored embeddings are treated as GDPR Art. 9 special-category data; B5 is the control making vault exfiltration survivable.
12. **§11** — Annotate as "conditional on legal counsel review"; scope notified-body contingency.
13. **B1 coverage** — Add CI test for cross-tenant query rejection.
14. **Enrollment path** — Add out-of-distribution check to prevent skeleton-key enrollment poisoning.
15. **Cross-tenant collision** — Add test case for enroll-same-subject-in-two-tenants.
16. **Audit log retention** — Document biometric-specific retention policy for `face_match_events`-derived rows.
17. **DR / backup** — Document audit-trail continuity across restore and WAL-ship cadence for audit log.
18. **Compliance file paths** — Cite KAI-294 and KAI-293 files by path; add Related compliance documents section.
19. **DPIA** — Joint lead-ai + lead-security DPIA update covering template inversion, age inference, minors pathway.

### SHOULD-CHANGE (strong recommendations, not blockers)

1. **§2** — Track B8 biometric-categorization refusal as the new invariant.
2. **§3** — Two-line diff on Annex IV ownership for Path A vs Path B.
3. **§5** — Add `rejection_reason` or sibling rejection table for reviewer-rejected matches.
4. **§5.1** — Note customer-facing trade-off for no-crop-retention tenants; retain fairness-test-set evidence for retired models for 10 years.
5. **§7** — UX spec sign-off tracked in KAI-327 review.
6. **§8** — Audit log entry when tenant raises threshold above FPR 1e-3.
7. **§9** — Weekly drift re-evaluation for first 90 days post-GA.
8. **§4.3** — Query entropy logging for extraction attempt detection.
9. **§4.6** — Post-enrollment cross-match sanity check.
10. **§4.9** — Force CMK rotation on customer offboarding.
11. **§4.10** — Post-restore auto-audit-event.

---

## Sign-off

- **Verdict:** APPROVE-WITH-CHANGES. The memo is a solid architectural foundation. Resolve the 19 MUST-CHANGE items (especially the template-inversion framing, the encrypted-search scheme, and the §11 legal-counsel conditionality) and this memo becomes the basis for KAI-282 implementation.
- **Gating condition:** no implementation code until (a) MUST-CHANGE items resolved, (b) KAI-251 encrypted-search design doc produced by lead-security, (c) legal counsel opinion on Annex VI applicability received.
- **Next action (lead-ai):** iterate on memo addressing MUST-CHANGE items 1–12 and 18–19. Items 13–17 can be tracked as implementation-time requirements rather than memo changes.
- **Next action (lead-security):** produce the KAI-251 encrypted-search design doc and kick off legal counsel retention for Art. 43 opinion.

— lead-security, 2026-04-08
