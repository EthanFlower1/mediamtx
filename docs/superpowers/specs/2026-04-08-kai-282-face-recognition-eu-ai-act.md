# KAI-282 — Face recognition with EU AI Act compliance (architecture memo)

**Status:** DRAFT memo. **Gate: no implementation code** until all three are signed off:
1. lead-security review of this memo + the KAI-294 compliance package
2. KAI-138 external SAML/auth review complete
3. KAI-251 cryptostore integration design approved

**Author:** lead-ai · **Date:** 2026-04-08 · **Ticket:** KAI-282 · **Deadline:** 2026-08-02 (EU AI Act Annex III §1(a) high-risk face recognition conformity)

---

## 1. Scope and intended purpose (Art. 13(3)(b)(iii))

Kaivue Recorder performs **post-hoc face recognition against an opt-in, per-camera, per-tenant enrolment vault** for investigation and access-event reconciliation. It is **not**:

- Real-time remote biometric identification in publicly accessible spaces (Art. 5(1)(h) — **prohibited**)
- Emotion inference (Art. 5(1)(f) — prohibited except safety/medical)
- Social scoring or predictive policing (Art. 5(1)(c), (d))
- Untargeted scraping of faces from the internet or CCTV for a reference database (Art. 5(1)(e))

The product **refuses to ship** these modes at the code path level, not just the UI level. See §6.

Intended purpose is restated verbatim in the instructions-for-use doc at `docs/compliance/eu-ai-act/transparency-and-information.md` (owned by KAI-294 package). This memo cites that package; it does not duplicate it.

## 2. Boundary conditions the architecture must enforce

These are non-negotiable code-level invariants, enforced at compile time or at the storage layer, not at the UI:

| # | Invariant | Enforcement point |
|---|-----------|-------------------|
| B1 | A face is only matched against a tenant's own enrolment vault, never cross-tenant | Per-tenant pgvector index (Seam #4 — see KAI-292); query builder rejects any request without a `tenant_id` scope |
| B2 | Face recognition is off by default per tenant AND per camera | Feature gate in tenant entitlements (`ai.face_recognition`) **AND** per-camera opt-in flag in `camera_ai_config.face_recognition_enabled` (default false); both must be true at runtime |
| B3 | No enrolment without explicit, logged subject consent OR a documented lawful basis (GDPR Art. 9(2)) | Enrolment API requires a `consent_record_id` that references an append-only row in the audit log (KAI-233). Enrolment without this id returns 400. |
| B4 | Minors (inferred age <18 or declared in enrolment metadata) are **blocked** from enrolment unless the tenant has explicitly passed a minors carve-out review | Pre-enrolment check; blocker is on by default. Art. 9(9) minors carve-out in KAI-294 risk-management-system.md R3 |
| B5 | Face embeddings are stored encrypted with a customer-managed key (CSE-CMK), key never leaves the tenant's KMS | KAI-251 cryptostore wraps every write; raw float32 vectors never touch disk unwrapped |
| B6 | Killswitch: tenant admin can disable face recognition immediately and schedule 30-day purge of the enrolment vault | Admin action writes a `face_vault_purge_scheduled_at` row; background worker deletes + tombstones. Killswitch is effective within 60s (not 30 days — the 30 days is the purge of historical vault data after the feature is already off) |
| B7 | Every match, enrolment, purge, and config change is audit-logged with actor, timestamp, and target subject id (pseudonymised) | Audit log via KAI-233, not an ad-hoc table |

If any of B1–B7 cannot be satisfied by the chosen architecture, **the architecture is wrong**, not the invariant.

## 3. Base model selection (DEFERRED — gates lead-security's 5 questions)

I am **not** picking a base model in this memo. Lead-security has asked for training data provenance, demographic composition, accuracy per demographic group, model architecture/version strategy, and drift monitoring pipeline. All five answers depend on which base model we pick. The decision matrix below is what will be presented to lead-security for joint sign-off:

| Option | Provenance story | Demographic balance available | License | Fairness audit available | Risk |
|--------|------------------|------------------------------|---------|--------------------------|------|
| **InsightFace `buffalo_l` (ArcFace R100, Glint360K-trained)** | Glint360K is scraped; provenance is **weak**. Unlikely to survive Art. 10(3) "relevant, sufficiently representative" scrutiny without supplementing. | Partial NIST FRVT results exist but not per-subgroup from vendor | Academic only for Glint360K; commercial use is murky | NIST FRVT 1:N | Provenance + licence |
| **FaceNet (Inception-ResNet-v1, VGGFace2)** | VGGFace2 is public but scraped; has a takedown history. | Limited | MIT code, dataset license restricted | Independent academic audits exist | Provenance + dataset retirement |
| **Train/fine-tune on a licensed dataset** (e.g., a commercial face dataset with subject releases) | **Strong** — subject consent documented | Can be measured at training time | Commercial, cost $$ | We run the audit ourselves (Art. 15(1)) | Cost, time, lead-ai bandwidth |
| **Buy a pre-audited model from a vendor with CE marking support** | Strong if vendor provides Annex IV docs | Vendor-declared | Commercial | Vendor-provided | Vendor lock-in, cost |

**Recommendation pending:** option 3 or 4 is the only path that survives Art. 10 without heroics. Option 1 and 2 are faster but the notified body will reject them and we burn the schedule restarting.

Lead-security — this table is the point of decision. Please weigh in before I spend any time on a specific model.

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

**Storage note:** `face_embeddings.embedding_ciphertext` is written by KAI-251's cryptostore wrapper. The pgvector index is built on a **separate column** containing the *tenant-local* projection (not plaintext, not raw ciphertext — a form that allows approximate cosine search without decrypting at query time). The exact scheme is a cryptostore design question for lead-security; options include encrypted search schemes (CGKN, SSE) or a tenant-scoped key-derived permutation. **Do not build until lead-security has picked a scheme.**

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

## 8. Accuracy, robustness, cybersecurity (Art. 15)

Cites KAI-294 `accuracy-robustness-cybersecurity.md` in full. Face-recognition-specific addenda:

- **No online learning.** Model weights are frozen per version; only the enrolment vault grows. Eliminates feedback-loop risk.
- **Model integrity:** sha256 + signature check on every load (existing pattern from KAI-279 model registry). Signature verification failure = hard fail, not a warning.
- **Adversarial robustness:** promotion pipeline includes a presentation-attack test set (print, replay, mask) before any model version is allowed in production. Failure to meet the threshold = version blocked.
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

**Contract:** tenants agree to use the feature only within the intended purpose (§1). Violating the Art. 5 boundary is a contract breach AND, if caught, a provider-level refusal at the code path. Kaivue is not liable for deployer misuse beyond the code-path refusals in §6 and the instructions-for-use doc in KAI-294 `transparency-and-information.md`.

## 11. Conformity assessment (Art. 43, Annex VI)

Annex III high-risk path. Kaivue performs the **internal control** conformity assessment (Annex VI) because we have harmonised standards coverage (EN ISO/IEC 23894 risk management, 42001 AI management system, 25059 quality for AI) and we do not fall into the notified-body-required exceptions. Full procedure lives in KAI-294 `conformity-assessment.md` — this memo does not duplicate it.

## 12. What this memo does NOT commit to

- Specific base model (see §3, gated on lead-security)
- Exact pgvector schema / index type (gated on KAI-292 landing, but must honour Seam #4)
- Exact encrypted-search scheme (gated on lead-security + KAI-251 design)
- Implementation timeline (gated on lead-security signoff)
- Training-data-sourcing decision (cost/schedule implication; CTO decision)

## 13. Open questions for lead-security

1. **§3 model decision:** option 1/2 (fast but risky provenance), option 3 (train/fine-tune on licensed), or option 4 (buy pre-audited vendor model)?
2. **§5 encrypted-search scheme:** do we invest in CGKN/SSE, or is a tenant-key-derived permutation acceptable for v1?
3. **§5 minor detection:** who owns the age-inference step, and what is the false-negative cost model? Fallback is: if in doubt, block enrolment and require tenant attestation.
4. **§8 presentation-attack test set:** do we build this in-house or buy? Lead-security has the SOC 2 vendor relationships.
5. **§11 conformity-assessment path:** confirm internal control is correct vs. notified-body — I am >90% confident but you should have the final call.

## 14. Open questions for CTO

1. **Budget for a licensed training set or pre-audited vendor model** (§3 option 3/4). Ballpark: five- to six-figure one-time + annual re-licence.
2. **Auditor selection:** who do we retain for the Annex VI internal-control quality system audit? Same firm as SOC 2 (KAI-385)?
3. **Launch sequencing vs 2026-08-02 deadline:** KAI-282 implementation cannot start until all three signoffs (memo top). Is the deadline a hard deadline on *shippable product* or on *conformity declaration*? If the latter, we have more runway than the former.

---

**Next steps after lead-security signoff:**
1. Resolve §3 model decision → answer lead-security's 5 questions with real data
2. Pick encrypted-search scheme with lead-security
3. Write implementation plan (separate doc) targeting KAI-282 sub-tickets
4. Begin implementation in priority order: face_vault_config → enrolments CRUD → detector/embedder → match engine → review UI

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
