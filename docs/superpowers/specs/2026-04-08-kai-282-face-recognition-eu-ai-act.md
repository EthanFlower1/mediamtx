# KAI-282 — Face recognition with EU AI Act compliance (architecture memo)

**Status:** DRAFT v1.1 (addresses lead-security APPROVE-WITH-CHANGES from 2026-04-08). **Gate: no implementation code** until all three are signed off:
1. lead-security review of this memo + the KAI-294 compliance package
2. KAI-138 external SAML/auth review complete
3. KAI-251 cryptostore integration design approved

**Changelog (v1.1):**
- MUST-CHANGE #1: Reclassified face embeddings as GDPR Art. 9 special-category biometric data throughout (template-inversion risk)
- MUST-CHANGE #2: consent_records table landed in KAI-292 migration 0015 (cross-ref added)
- MUST-CHANGE #3: KAI-251 cryptostore integration design doc delivered (see `2026-04-08-kai-251-face-vault-crypto-integration.md`)
- MUST-CHANGE #4: §8 expanded with PAD testing tiers (iBeta + NIST FRVT PAD + in-house adversarial)
- MUST-CHANGE #5: §11 rewritten as conditional pending legal counsel on harmonized-standards gap

**Author:** lead-ai · **Date:** 2026-04-08 · **Ticket:** KAI-282 · **Deadline:** 2026-08-02 (EU AI Act Annex III §1(a) high-risk face recognition conformity)

---

## 1. Scope and intended purpose (Art. 13(3)(b)(iii))

Kaivue Recorder performs **post-hoc face recognition against an opt-in, per-camera, per-tenant enrolment vault** for investigation and access-event reconciliation. It is **not**:

- Real-time remote biometric identification in publicly accessible spaces (Art. 5(1)(h) — **prohibited**)
- Emotion inference (Art. 5(1)(f) — prohibited except safety/medical)
- Social scoring or predictive policing (Art. 5(1)(c), (d))
- Untargeted scraping of faces from the internet or CCTV for a reference database (Art. 5(1)(e))

The product **refuses to ship** these modes at the code path level, not just the UI level. See §6.

Intended purpose is restated verbatim in the instructions-for-use doc at `docs/compliance/eu-ai-act/transparency-and-instructions-for-use.md` (owned by KAI-294 package). This memo cites that package; it does not duplicate it.

## 2. Boundary conditions the architecture must enforce

These are non-negotiable code-level invariants, enforced at compile time or at the storage layer, not at the UI:

| # | Invariant | Enforcement point |
|---|-----------|-------------------|
| B1 | A face is only matched against a tenant's own enrolment vault, never cross-tenant | Per-tenant pgvector index (Seam #4 — see KAI-292); query builder rejects any request without a `tenant_id` scope |
| B2 | Face recognition is off by default per tenant AND per camera | Feature gate in tenant entitlements (`ai.face_recognition`) **AND** per-camera opt-in flag in `camera_ai_config.face_recognition_enabled` (default false); both must be true at runtime |
| B3 | No enrolment without explicit, logged subject consent OR a documented lawful basis (GDPR Art. 9(2)) | Enrolment API requires a `consent_record_id` that references an append-only row in the audit log (KAI-233). Enrolment without this id returns 400. |
| B4 | Minors (inferred age <18 or declared in enrolment metadata) are **blocked** from enrolment unless the tenant has explicitly passed a minors carve-out review | Pre-enrolment check; blocker is on by default. Art. 9(9) minors carve-out in KAI-294 risk-management-system.md R3 |
| B5 | Face embeddings are **GDPR Art. 9 special-category biometric data** (not metadata — template-inversion attacks can reconstruct recognizable faces from embeddings; see Mai et al. 2019, Vendrow & Friedler 2021). Stored encrypted with a customer-managed key (CSE-CMK), key never leaves the tenant's KMS | KAI-251 cryptostore wraps every write; raw float32 vectors never touch disk unwrapped. In-memory plaintext vectors are mlock'd and zeroed on shutdown (see KAI-251 integration design §5). |
| B6 | Killswitch: tenant admin can disable face recognition immediately and schedule 30-day purge of the enrolment vault | Admin action writes a `face_vault_purge_scheduled_at` row; background worker deletes + tombstones. Killswitch is effective within 60s (not 30 days — the 30 days is the purge of historical vault data after the feature is already off) |
| B7 | Every match, enrolment, purge, and config change is audit-logged with actor, timestamp, and target subject id (pseudonymised) | Audit log via KAI-233, not an ad-hoc table |

If any of B1–B7 cannot be satisfied by the chosen architecture, **the architecture is wrong**, not the invariant.

## 3. Base model selection (DEFERRED — point of decision for lead-security)

I am **not** picking a base model in this memo. Lead-security framed this correctly: an Art. 10(2)(f) data-governance doc is **requirements first, instance data second**. Specific base-model provenance, demographic composition, accuracy per group, architecture/version, and drift numbers become a *versioned artifact* keyed to each deployed model — they get filled in after model selection, not before.

The matrix below is the decision surface lead-security needs to weigh in on. License audit and inherited fairness claims are explicit columns because (per lead-security): research-only weights are a **legal** blocker on top of the compliance question, and citing inherited fairness numbers from upstream model cards is infinitely better for the auditor than promising to measure later.

| Candidate | Architecture | Backbone training set | Provenance | Licence (weights) | Inherited fairness claims | Verdict |
|-----------|-------------|----------------------|------------|-------------------|--------------------------|---------|
| **InsightFace `buffalo_l` (ArcFace R100)** | ArcFace, ResNet-100 | Glint360K (scraped) | Weak | Glint360K is research-only; commercial weight licence is **murky → likely blocker** | NIST FRVT 1:N partial; per-subgroup TPR not vendor-published | **Likely fail Art. 10(3) AND legal** |
| **FaceNet (Inception-ResNet-v1)** | Triplet loss / FaceNet | VGGFace2 (scraped, takedown history) | Weak | MIT code, dataset licence restricted; weights are derivative of restricted dataset | Independent academic audits exist (e.g. RFW) but not in model card | **Likely fail Art. 10(3) + dataset retirement risk** |
| **AdaFace (R100)** | Adaptive margin loss, ResNet-100 | MS1MV2 / WebFace4M variants | Weak (MS1MV2 has known retraction issues; WebFace4M scraped) | Apache-2.0 code; weight licence depends on training set used (some redistributions are research-only) | Model card reports IJB-B/IJB-C TAR@FAR; **per-subgroup numbers exist on RFW benchmark** | **Possible if WebFace4M variant is licence-clean — legal audit needed** |
| **MagFace (R100)** | Magnitude-aware margin, ResNet-100 | MS1MV2 | Weak (same MS1MV2 problem) | Apache-2.0 code; same weight-licence ambiguity | Reports IJB-B/IJB-C; **per-quality-bucket TAR/FAR published** but not per-demographic | **Possible only on a re-trained backbone with clean dataset** |
| **TransFace (ViT-S/B/L)** | Vision Transformer face recognition | Glint360K / WebFace42M | Weak (Glint360K) or unclear (WebFace42M) | Apache-2.0 code; weight licence inherits dataset issues | Reports IJB-B/IJB-C TAR@FAR; minimal per-demographic disclosure | **Same provenance problem as InsightFace** |
| **Path A: train / fine-tune on a licensed dataset** (chosen architecture: ArcFace, AdaFace, MagFace, or vendor recommendation) | TBD | Licensed commercial set with documented subject releases | **Strong** | Commercial, contract terms control distribution | We measure ourselves at training time (Art. 15(1)) | **Defensible, expensive, slow** |
| **Path B: buy a pre-audited model from a vendor with CE-marking support** | Vendor-declared | Vendor-declared (contract requires Annex IV evidence delivery) | **Strong if vendor delivers Annex IV** | Commercial, vendor lock-in | Vendor-provided fairness audit | **Fastest defensible, vendor lock-in, $$** |

**Lead-security verdict (2026-04-08):** Options 1-5 (off-the-shelf academic weights) are **dead** — Glint360K and VGGFace2 provenance issues are legal blockers, not just compliance risk. MS1MV2 retraction history kills MagFace. WebFace4M/42M provenance is unclear.

**Decision: Path B (pre-audited vendor) is primary.** Shortlist: Paravision, Idemia, Innovatrics, Corsight. 2-week vendor RFP sprint targeting 2026-05-15 decision. Vendor must deliver: Annex IV evidence package, per-demographic fairness audit, PAD test results, and contractual Art. 10 data-governance compliance.

**Fallback: Path A (licensed dataset fine-tune)** if no vendor meets requirements or pricing is prohibitive. Architecture (ArcFace/AdaFace/MagFace backbone) TBD at that point.

**AdaFace licence-clean variant** remains a theoretical third path but is deprioritized — lead-security assessment is that the legal audit cost and timeline risk exceed the vendor lock-in cost of Path B.

**Inherited fairness claims convention** (applies regardless of which model is chosen): wherever this package cites a fairness number, it MUST be flagged either `[inherited from upstream model card vX.Y, dataset Z]` or `[measured by Kaivue on test set vN at date D]`. Mixing inherited and measured numbers without that flag is an audit finding waiting to happen. This convention also lives in `data-governance.md` (lead-security's file, post-merge).

## 4. System architecture (component map)

```
                    ┌──────────────────────────────────────┐
                    │        Recorder (on-prem)            │
                    │                                      │
                    │   pipeline.go (YOLO person detect)   │
                    │             │                        │
                    │             ▼                        │
                    │   face_detector (MTCNN/RetinaFace)   │
                    │             │                        │
                    │             ▼                        │
                    │   face_embedder (base model §3)      │
                    │             │                        │
                    │             ▼  encrypted via KAI-251 │
                    │   tenant_face_vault (pgvector)       │
                    │             │                        │
                    │             ▼                        │
                    │   match engine (cosine, threshold)   │
                    │             │                        │
                    │             ▼                        │
                    │   event + audit (KAI-233)            │
                    └──────────────┬───────────────────────┘
                                   │ edge vs cloud routing
                                   │ (KAI-280 already landed)
                                   ▼
                    ┌──────────────────────────────────────┐
                    │     Directory / Cloud (optional)     │
                    │                                      │
                    │   Per-tenant vault (pgvector, seam#4)│
                    │   Model registry (KAI-279)           │
                    │   Casbin policies (KAI-225)          │
                    │   Audit log aggregation (KAI-233)    │
                    └──────────────────────────────────────┘
```

**Component reuse (consume, don't rebuild):**
- **Detection path:** existing `internal/nvr/ai/pipeline.go` already runs YOLOv8n. Face detection is a second stage that crops person bboxes and runs MTCNN/RetinaFace. **No new pipeline layer** — it plugs into the existing pipeline as a post-detection stage.
- **Embeddings storage:** **pgvector with per-tenant index (Seam #4)**. Not a separate Milvus/Qdrant deployment. KAI-292 is landing this surface.
- **Edge vs cloud routing:** **KAI-280 consumed as-is**, not rebuilt. Routing decision is per-request and driven by existing policy.
- **Crypto:** **KAI-251 cryptostore** wraps every embedding write and unwraps on match query. No new crypto code in this package.
- **Audit log:** **KAI-233**. No ad-hoc `face_recognition_audit` table.
- **Authorisation:** **Casbin (KAI-225)**. Three roles: `face_admin` (enrol, configure, purge), `face_reviewer` (query, view matches, confirm/deny), `face_auditor` (read-only, read the audit log). Defined as default roles in KAI-146.
- **Model distribution:** **KAI-279 model registry**. Face model version is pinned per tenant; upgrades are explicit.

## 5. Data model (schemas only, no DDL in this memo)

All tables carry `tenant_id NOT NULL` (Seam #10). All tables live in per-tenant-indexed storage (Seam #4).

**GDPR Art. 9 classification:** `face_embeddings.embedding_ciphertext` and the in-memory plaintext vectors derived from it are **special-category biometric data** under GDPR Art. 9(1). Template-inversion attacks (Mai et al. 2019 "Reconstruction of Face Images from Deep Templates"; Vendrow & Friedler 2021 "The Case for Banning Face Recognition") demonstrate that face embeddings can be inverted to produce recognizable face images. This means embeddings carry the same legal classification as source face crops, not the weaker "metadata" classification. All retention, encryption, consent, and purge obligations that apply to source biometric data apply equally to embeddings. The `consent_records` table (KAI-292 migration 0015, LIST-partitioned by tenant_id) stores the Art. 9(2) lawful basis for each embedding.

```
face_enrolments
  id                   uuid pk
  tenant_id            uuid not null
  subject_pseudonym    text not null       -- never a real name
  external_subject_id  text null           -- opaque, client-supplied
  consent_record_id    uuid not null       -- FK to audit_log (KAI-233)
  enrolled_at          timestamptz not null
  enrolled_by          uuid not null       -- actor
  model_version        text not null       -- FK to model_registry (KAI-279)
  is_minor             boolean not null default false
  purged_at            timestamptz null
  INDEX (tenant_id, subject_pseudonym)

face_embeddings
  enrolment_id         uuid fk → face_enrolments
  tenant_id            uuid not null       -- denormalised for index scoping
  embedding_ciphertext bytea not null      -- KAI-251 wrapped
  embedding_dim        int not null
  embedding_hash       bytea not null      -- for dedup, not for match
  model_version        text not null
  INDEX (tenant_id)   -- pgvector HNSW/IVF index is PER tenant_id, not global

face_match_events
  id                   uuid pk
  tenant_id            uuid not null
  camera_id            uuid not null
  track_id             uuid not null       -- links to existing person track
  matched_enrolment_id uuid null           -- null = no match above threshold
  similarity_score     real not null
  confirmed_by         uuid null           -- human confirm (Art. 14)
  confirmed_at         timestamptz null
  model_version        text not null
  created_at           timestamptz not null

face_vault_config
  tenant_id            uuid pk
  enabled              boolean not null default false
  match_threshold      real not null
  minor_enrolment_allowed boolean not null default false
  purge_scheduled_at   timestamptz null    -- killswitch (B6)
  last_updated_by      uuid not null
  last_updated_at      timestamptz not null
```

**Storage note:** `face_embeddings.embedding_ciphertext` is written by KAI-251's cryptostore wrapper. The full encryption lifecycle — per-tenant CMK derivation, HKDF info strings, CMK rotation re-wrap flow, in-memory index rebuild, mlock/zeroization, and fail-closed fault tolerance — is specified in the KAI-251 integration design doc (`2026-04-08-kai-251-face-vault-crypto-integration.md`), currently under lead-security 24h review.

**Encrypted search (lead-security Q2 answer):** tenant-key-derived permutation is acceptable for v1. The pgvector HNSW index operates on **plaintext vectors held in mlock'd memory** (decrypted at process start from the encrypted DB column). All similarity searches run against the in-memory plaintext index; the database column stores only ciphertext. This avoids the complexity of CGKN/SSE while maintaining the invariant that raw float32 vectors never touch disk unwrapped. See KAI-251 integration design §4 for the full index-rebuild flow.

### 5.1 Model version transitions (Annex IV requirement)

Annex IV §2(a) requires us to document how we handle "the methods and steps performed for the development of the AI system" *across versions*. Face embeddings are **not portable across model versions** — a vector produced by AdaFace-R100 v1.0 has no meaningful cosine similarity to a vector produced by AdaFace-R100 v1.1 even with the same input image. So a model upgrade is also a re-enrolment event.

**Transition protocol:**

1. **New model version registration.** Lead-ai promotes a new model version through the KAI-279 model registry. The registry assigns a stable `model_version` string (e.g. `kaivue-face-v2.0.0-adaface-r100-licensedset-2026-09-15`). Promotion requires: passing the fairness gate (KAI-294 `fairness-testing-protocol.md`), passing the presentation-attack gate (§8), and a signed model-card commit.
2. **Per-tenant opt-in to upgrade.** Customer admin sees a version-transition banner in the face vault UI (KAI-327): `A new face recognition model is available. Re-enrolment will run in the background. [Schedule] [Defer] [More info]`. Tenants can defer up to 90 days before old model versions are end-of-lifed for that tenant.
3. **Coexistence window.** During the transition window (default 30 days, configurable per tenant up to 90), both `model_version` values are active in the tenant's vault. Each `face_embeddings` row is keyed by `(enrolment_id, model_version)`, allowing both versions to coexist. Match queries run against the **new** model version's embeddings only; old-version embeddings serve as a fall-back **only** for enrolments that have not yet been re-encoded.
4. **Re-enrolment strategies (tenant-selectable):**
   - **Eager (default for ≤1000 enrolments):** background worker re-encodes every enrolment from the stored source crop within 24h of upgrade acceptance.
   - **Opportunistic (default for >1000 enrolments):** re-encode each enrolment the next time that subject is seen by a camera, OR within the 30-day window, whichever comes first. Source crops not seen by 30-day deadline get the eager treatment as fallback.
5. **Match labelling during coexistence.** Every match event in `face_match_events` is labelled with `model_version` so the auditor and the operator can both tell which model produced the match. Mixed-model match results are NEVER aggregated into a single confidence score.
6. **Final purge.** At end of transition window, all `face_embeddings` rows with the old `model_version` are deleted, an audit log entry is written with old/new version + count + tenant_id, and the old model artifact is retained in the model registry (KAI-279) for the 10-year evidence-retention obligation (Art. 18) but is no longer loadable for inference on this tenant.
7. **Source-crop retention (lead-security ACK: keep opt-in).** Re-enrolment requires the original face crop to still exist. Per Art. 5(1)(c) GDPR data minimisation, source crops are retained ONLY when the tenant has explicitly opted into "preserve enrolment images for re-encoding on model upgrade". Tenants who opt out lose access to opportunistic re-enrolment and must re-capture subjects manually for every model upgrade. Lead-security confirmed this opt-in model is correct (2026-04-08); source-crop retention must never be the default.

**Why this matters for Annex IV:** the technical documentation must explain how the system maintains accuracy across model upgrades AND how subject data is treated during transition. Without an explicit version-transition protocol, "we just deploy a new model" is an audit finding because (a) embeddings are silently invalidated, (b) match precision can degrade without operator awareness, and (c) source-crop retention may violate data minimisation if not explicitly opted into.

## 6. Code-path refusals (Art. 5)

These must be enforced at the handler layer and return HTTP 403 with a machine-readable reason, not a feature flag that can be flipped:

| Refusal | Surface |
|---------|---------|
| Real-time streaming match against a live watchlist from a public-space camera | Configuration validator rejects `face.real_time_public = true`; no code path exists to enable it |
| Emotion classification endpoint | **Endpoint not defined.** There is no `POST /face/emotion`. If one appears in a PR, review blocks. |
| Enrolment via a dataset upload without per-subject consent records | Bulk enrolment endpoint requires a `consent_manifest` with one row per face and a signed tenant attestation |
| Cross-tenant match | Query builder has no `tenant_id` escape hatch; service-account admin endpoints also scope by tenant |
| Match against an untargeted internet-scraped database | No such database exists in the product; there is no ingestion path for one |

## 7. Human oversight (Art. 14)

Mapped to existing Kaivue surfaces per KAI-294 `human-oversight.md`:

- **Oversight responsibility:** `face_reviewer` role must confirm every match before the match event is published downstream (webhooks, search results, access-control integrations). Unconfirmed matches are visible only to reviewers, not operators.
- **Automation-bias mitigation:** Match UI shows similarity score, threshold, model version, and a **visible** "not a match" action placed equal-weight with "confirm match". No default selection.
- **Kill switch:** Per-camera and per-tenant, with effect ≤60s (B6).
- **Stop condition:** If confirmed-match rate in a 1-hour window goes to 100% (anomalous — normal is ≪100%), system auto-disables match publication for that tenant and pages on-call. Listed in KAI-294 `post-market-monitoring.md`.

### 7.1 Four-eyes watchlist UI stub (web + Flutter)

This section is a **stub spec** for lead-web and lead-flutter to implement. It does not mandate pixels; it mandates invariants that the code-review process (and the Art. 14 audit trail) will check against. Shipped in KAI-327 (admin) and the Flutter client.

**Who can do what:**

- `face_reviewer` role (defined in KAI-225 Casbin policy) is the only role authorised to enrol or flag a watchlist subject.
- **Enrolment is a two-person operation.** Reviewer A proposes a watchlist entry (identity label, consent_record_id, source crop, optional justification note). The entry enters state `proposed`. It is invisible to matching until Reviewer B — a *different* `face_reviewer` principal — independently confirms it, at which point state becomes `active`. If Reviewer A and Reviewer B resolve to the same principal, the UI refuses to submit and the backend (KAI-225) rejects.
- Same-principal confirmation attempts are logged to KAI-233 audit log as a distinct event type (`watchlist.four_eyes_violation_attempt`) and count toward the tenant's post-market monitoring anomaly counters in KAI-294 `post-market-monitoring.md`.
- Deactivation follows the same two-person rule. A watchlist entry cannot be deleted by a single reviewer.

**What Reviewer B sees (the confirmation screen):**

- The source crop Reviewer A uploaded, at the **original resolution** (no automatic upscaling, no enhancement filters — see B6 and Art. 14 automation-bias mitigation).
- The identity label and the justification note.
- The `consent_record_id` with a **required** click-through to the consent record detail view. Reviewer B cannot confirm without opening that view at least once (UI enforces, backend double-checks via a `consent_viewed_at` timestamp on the confirmation API payload).
- The model version the entry will be embedded under (per §5.1 — watchlist entries bind to a model version).
- Two equal-weight buttons: **"Confirm watchlist entry"** and **"Reject — not a valid watchlist subject"**. No default focus. No pre-selection. Identical size, colour, and position per Art. 14 automation-bias rules already stated in §7.
- A visible **"I am not the reviewer who proposed this entry"** affirmation checkbox. Unchecked by default. Submit button disabled until checked. This is belt-and-braces in front of the backend same-principal check.

**What the match review screen (the normal day-to-day screen) looks like:**

This is the existing §7 match-review surface, not new UI, but the following watchlist-specific invariants apply:

- When a match fires against a watchlist entry, the match card must show the **state** of that watchlist entry (`active`, `suspended`, `pending_reconfirmation`). A match against a non-`active` entry must NOT be published downstream even if the reviewer confirms the match — the reviewer can only mark it for follow-up.
- Watchlist subject metadata shown to the reviewer is limited to the identity label and the consent record link. Notes from the proposing reviewer are **hidden** during match review (they may bias the match decision). Notes are visible only on the watchlist management screen.
- The similarity score, threshold, and model version remain visible (from §7). If the match is against a watchlist entry enrolled under an older model version than the currently-running model, the UI displays a banner: "Entry enrolled under model vX; current model is vY. Re-enrol before the Z deadline." (Ties to §5.1 transition protocol.)

**Anomaly hooks (feeds KAI-294 `post-market-monitoring.md`):**

- Per-reviewer confirm rate: if a single `face_reviewer` confirms >95% of their presented watchlist proposals or match reviews over any rolling 100-event window, the system raises a `reviewer.rubber_stamping_suspected` monitoring event and pages on-call. This is the anti-automation-bias analogue of the 100%-per-tenant stop condition in §7.
- Zero-rejection reviewer: if a reviewer has never rejected a match or a proposal in their lifetime on the system and has >50 events, same event type, lower severity.
- Time-to-confirm: confirmations that happen in <2 seconds are flagged (insufficient time to view the consent record and the source crop). This is a *signal*, not a block — reviewer may legitimately confirm fast on a very obvious case — but it feeds the rubber-stamping detector.

**Audit trail (KAI-233):**

Every one of the following is an audit-log event, with tenant_id, reviewer principal, watchlist entry id, model version, similarity score where applicable, and reason code:

- `watchlist.proposed`, `watchlist.confirmed`, `watchlist.rejected`, `watchlist.suspended`, `watchlist.reactivated`, `watchlist.purged`
- `watchlist.four_eyes_violation_attempt` (same principal tried to confirm their own proposal)
- `watchlist.consent_record_viewed` (required before confirmation)
- `match.reviewed_confirm`, `match.reviewed_reject`, `match.marked_followup`

These feed the Art. 12 automatic logging obligation and satisfy the Art. 14(4)(e) "intervene or interrupt" audit requirement.

**Ownership:**

- lead-web implements the admin web surface (KAI-327 scope).
- lead-flutter implements the mobile/tablet surface (needed because security managers confirm on phones in the field — known customer pattern).
- lead-security reviews both implementations against this stub spec before merge.
- lead-ai (me) owns the backend API + state machine + KAI-233 event emission.

## 8. Accuracy, robustness, cybersecurity (Art. 15)

Cites KAI-294 `accuracy-robustness-cybersecurity.md` in full. Face-recognition-specific addenda:

- **No online learning.** Model weights are frozen per version; only the enrolment vault grows. Eliminates feedback-loop risk.
- **Model integrity:** sha256 + signature check on every load (existing pattern from KAI-279 model registry). Signature verification failure = hard fail, not a warning.
- **Adversarial robustness — presentation attack detection (PAD):** three-tier testing requirement before any model version is allowed in production:

  **Tier 1 — Third-party lab certification:** iBeta PAD Level 1 (ISO 30107-3). This is a hard gate for GA launch. Budget estimate: ~$15-20k per model version. Lead-security owns the vendor relationship.

  **Tier 2 — NIST FRVT PAD benchmark:** submit the model to NIST FRVT Presentation Attack Detection evaluation. Results are public and provide independent third-party validation. Timeline: 4-8 weeks from submission to publication. Not a launch gate (NIST timeline is outside our control), but submission must happen before GA and results must be incorporated into the Art. 15 technical documentation when available.

  **Tier 3 — In-house adversarial test set:** maintained by lead-ai, reviewed by lead-security. Must include:
  - **Print attacks:** high-resolution photo printouts (A4, A3) at multiple distances
  - **Replay attacks:** tablet/phone screen replay of subject photos and videos
  - **3D mask attacks:** silicone and resin masks (at least 3 mask types from different vendors)
  - **Morphing attacks:** GAN-generated morph between two subjects (tests whether morphed face matches both)
  - **Adversarial patch attacks:** physically-realizable adversarial patches (eyeglass frames, hat patterns) using state-of-the-art attack methods (e.g., Sharif et al. 2016 adversarial glasses, Komkov & Petiushko 2021 adversarial hat)

  Failure on any tier = model version blocked from production. Tier 3 is run on every model version promotion through the KAI-279 registry; Tiers 1-2 are run on major version bumps (not patch releases).

  **Budget note for CTO:** Tier 1 + Tier 3 infrastructure estimated at $30-40k initial + $10-15k per major model version. Tier 2 is free (NIST service) but requires staff time for submission preparation.

- **Threshold selection:** per-tenant threshold default is set so that FPR ≤1e-4 on the fairness test set (KAI-294 `fairness-testing-protocol.md`). Tenants can raise the threshold but **not** lower it below the default.

## 9. Fairness (Art. 10(5), Art. 15)

Cites KAI-294 `fairness-testing-protocol.md`. Face-recognition-specific:

- Fairness test set is Fitzpatrick I-II / III-IV / V-VI × age 18-30 / 31-50 / 51+ × gender M / F / other (18 buckets, minors excluded).
- Equalised-odds gap ≤0.05 absolute across buckets is a **release blocker**, not a warning.
- Per-bucket TPR at FPR=1e-4 is reported to the tenant in the admin console and the trust portal.
- Drift: monthly re-evaluation on a held-out set; regression >0.02 pages on-call and triggers a Post-Market Monitoring report (Art. 72).

## 10. Provider vs deployer split (Art. 16, Art. 26)

Kaivue is the **provider** under Art. 16: we ship the face recognition function, attach CE marking, and write the Annex IV technical documentation.

Tenants are **deployers** under Art. 26: they decide *whether* to turn it on, pick cameras, handle subject consent, and run the DPIA (template in KAI-294 `fundamental-rights-impact-assessment.md` — we provide the template, they fill it in).

**Contract:** tenants agree to use the feature only within the intended purpose (§1). Violating the Art. 5 boundary is a contract breach AND, if caught, a provider-level refusal at the code path. Kaivue is not liable for deployer misuse beyond the code-path refusals in §6 and the instructions-for-use doc in KAI-294 `transparency-and-instructions-for-use.md`.

## 11. Conformity assessment (Art. 43, Annex VI) — CONDITIONAL

**This section is conditional pending legal counsel confirmation by 2026-05-01.** The path below is lead-ai's recommendation; lead-security flagged a material ambiguity that must be resolved before this section is final.

**Primary path (Annex VI internal control):** Kaivue performs the internal control conformity assessment per Annex VI. The standards we claim coverage against: EN ISO/IEC 23894 (AI risk management), ISO/IEC 42001 (AI management system), ISO/IEC 25059 (quality for AI). Full procedure lives in KAI-294 `conformity-assessment.md`.

**The harmonized-standards gap (lead-security MUST-CHANGE #5):** As of 2026-04-08, **no harmonized standards specific to face recognition have been published** under the EU AI Act. CEN-CENELEC JTC 21 WG 4 is drafting them, but publication is not expected before late 2026 or early 2027. This creates an ambiguity in Art. 43(1):

- **Strict reading:** Art. 43(1) says providers "shall follow" the conformity assessment procedure "referred to in Annex VI" *only when* the AI system has been "designed and developed in accordance with the harmonised standards referred to in Article 40." If no harmonised standards exist for our specific high-risk category (Annex III §1(a) — biometric identification), the internal control path may not be available, and a **notified body assessment (Annex VII)** may be required.
- **Practical reading:** the general AI management and risk standards (23894, 42001) provide sufficient coverage for the internal control path, and the face-recognition-specific standards will be adopted when published. Most legal commentary supports this reading, but it is not settled.

**Contingency path (Annex VII notified body):** if legal counsel determines that the strict reading applies, Kaivue must engage a notified body for third-party conformity assessment. This adds 3-6 months and significant cost. Lead-security to identify candidate notified bodies as a contingency.

**Action items:**
1. **Legal counsel must answer by 2026-05-01:** Does the absence of face-recognition-specific harmonised standards require a notified body assessment under Art. 43(1), or do the general AI management standards (23894, 42001, 25059) satisfy the "harmonised standards" condition?
2. If notified body required: lead-security begins notified body selection by 2026-05-15.
3. If internal control sufficient: proceed with Annex VI as primary path, with commitment to adopt face-recognition-specific harmonised standards when published (transition plan in KAI-294 `conformity-assessment.md`).
4. **CTO escalation:** 5 specific questions for legal counsel have been sent (see §14). Engagement THIS WEEK is required to meet the 2026-05-01 deadline.

**B6 killswitch interaction:** regardless of conformity path, the B6 killswitch (tenant admin can disable face recognition immediately + 30-day vault purge) must be fully operational before CE marking. If the conformity assessment reveals that the killswitch latency (currently ≤60s for feature disable, 30 days for data purge) is insufficient, the purge timeline will be shortened. Lead-security has final say on acceptable purge latency.

## 12. What this memo does NOT commit to

- Specific base model (see §3, gated on lead-security)
- Exact pgvector schema / index type (gated on KAI-292 landing, but must honour Seam #4)
- Exact encrypted-search scheme (gated on lead-security + KAI-251 design)
- Implementation timeline (gated on lead-security signoff)
- Training-data-sourcing decision (cost/schedule implication; CTO decision)

## 13. Open questions for lead-security (status as of v1.1)

1. ~~**§3 model decision:**~~ **RESOLVED.** Path B (pre-audited vendor) is primary. 2-week RFP sprint targeting 2026-05-15. Shortlist: Paravision, Idemia, Innovatrics, Corsight.
2. ~~**§5 encrypted-search scheme:**~~ **RESOLVED.** Tenant-key-derived permutation acceptable for v1. In-memory plaintext HNSW index with mlock'd memory. Full design in KAI-251 integration doc.
3. **§5 minor detection:** who owns the age-inference step, and what is the false-negative cost model? Lead-security proposed split ownership: lead-ai owns the inference model, lead-security owns the policy (block threshold, carve-out review process). **Awaiting formal confirmation.**
4. ~~**§8 presentation-attack test set:**~~ **RESOLVED.** Three-tier approach: iBeta PAD Level 1 (third-party lab) + NIST FRVT PAD (benchmark submission) + in-house adversarial (morphing + adversarial patches required). Budget: $30-40k initial. Lead-security owns iBeta vendor relationship.
5. ~~**§11 conformity-assessment path:**~~ **ESCALATED.** Harmonized standards for face recognition don't exist yet. Legal counsel must confirm by 2026-05-01 whether internal control (Annex VI) is available or notified body (Annex VII) required. See §11 for full analysis.

## 14. Open questions for CTO (updated v1.1)

1. **Vendor RFP budget authority** (§3 Path B). 2-week RFP sprint targeting 2026-05-15 decision. Shortlist: Paravision, Idemia, Innovatrics, Corsight. Need budget approval for: vendor evaluation licenses, integration PoC, and annual model license (ballpark five- to six-figure).
2. **PAD testing budget** (~$30-40k initial + $10-15k per major model version). Covers iBeta PAD Level 1 certification, in-house adversarial test set infrastructure (3D masks, adversarial patches). See §8 Tier breakdown.
3. **Legal counsel engagement THIS WEEK.** 5 specific questions requiring answer by 2026-05-01:
   - Does absence of face-recognition-specific harmonised standards require Annex VII (notified body)?
   - Does internal control (Annex VI) with general AI standards (23894, 42001, 25059) satisfy Art. 43(1)?
   - Art. 25 Modifier liability for customer-uploaded models (KAI-291) — does the CMPA click-through shift sufficient liability?
   - GDPR Art. 9(2)(a) explicit consent vs Art. 9(2)(g) substantial public interest — which lawful basis do we advise deployers to use?
   - Cross-border data transfer implications for face embeddings (classified as Art. 9 special-category biometric data) stored in US-region RDS?
4. **Auditor selection:** who do we retain for the Annex VI internal-control quality system audit? Same firm as SOC 2 (KAI-385)?
5. **Launch sequencing vs 2026-08-02 deadline:** KAI-282 implementation cannot start until all three signoffs (memo top). Is the deadline a hard deadline on *shippable product* or on *conformity declaration*? If the latter, we have more runway than the former.

---

**Next steps (updated v1.1):**
1. ~~Resolve §3 model decision~~ **DONE.** Path B (pre-audited vendor). RFP sprint scoped; pending CTO budget approval.
2. ~~Pick encrypted-search scheme~~ **DONE.** Tenant-key-derived permutation for v1. KAI-251 integration design under 24h review.
3. **Awaiting:** Legal counsel engagement (§11, §14) — CTO action, deadline 2026-05-01
4. **Awaiting:** KAI-251 integration design signoff from lead-security (24h turnaround from 2026-04-08)
5. **Awaiting:** KAI-138 external SAML/auth review completion (gate #2)
6. Write implementation plan (separate doc) targeting KAI-282 sub-tickets — after gates 3-5 clear
7. Begin implementation in priority order: face_vault_config → enrolments CRUD → detector/embedder → match engine → review UI

**Related tickets:**
- KAI-294 (EU AI Act compliance package — cited throughout)
- KAI-251 (cryptostore — §5)
- KAI-225 (Casbin — §4)
- KAI-233 (audit log — §4, §5)
- KAI-279 (model registry — §4)
- KAI-292 (pgvector + per-tenant index — §5, Seam #4)
- KAI-280 (edge vs cloud routing — §4)
- KAI-146 (default Casbin roles — §4)
- KAI-138 (external auth review — gate)
- KAI-327 (customer admin face vault management UI — downstream)
