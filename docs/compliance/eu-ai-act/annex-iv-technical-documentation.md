# Annex IV Technical Documentation — Template

**Article:** 11 + Annex IV
**Status:** Template complete. Per-model body filled in as KAI-282 face recognition lands.
**Owner:** lead-ai

## Purpose

Annex IV of the EU AI Act lists the minimum technical documentation a provider must produce for a high-risk AI system. This file is the **template**. For each deployed face-recognition model (at v1: one production model; at v1.x: customer-uploaded models via KAI-291) a per-model instance of this document is produced and committed under `docs/compliance/eu-ai-act/models/<model-id>.md`.

## 1. General description of the system

### 1.1 Intended purpose
Kaivue face recognition identifies known faces from a customer-managed face vault and alerts on watchlist matches, within the customer's own on-prem or cloud-recorded video footage.

### 1.2 Person(s) who developed the system
- Provider: Kaivue (TBD legal entity)
- Technical lead: lead-ai
- Compliance lead: lead-security

### 1.3 Date and version
Per-model. Populated from the model registry (KAI-279) record.

### 1.4 Hardware and software dependencies
- Edge: Linux x86_64 or arm64 NVR appliance, optional NVIDIA GPU. ONNX Runtime 1.17+.
- Cloud: NVIDIA Triton Inference Server on EKS, g5.2xlarge+ GPU nodes. mTLS per-tenant request routing.
- Integration interface: `internal/ai/router` (KAI-280) is the *only* entry point for inference dispatch.

### 1.5 Forms in which the system is placed on the market
- Embedded in the Kaivue Recorder binary (on-prem appliance).
- As a SaaS feature in the Kaivue cloud control plane.
- As an optional feature, opt-in per camera, disabled by default.

### 1.6 Instructions for use
Documented in `transparency-and-information.md` and in the customer admin console (KAI-327 AI Settings page).

## 2. Detailed description of the elements of the system and its development

### 2.1 Methods and steps for development
- Model selection: [per-model: candidate comparison matrix, acceptance criteria, selection justification]
- Training or fine-tuning: [per-model: whether we fine-tuned, on what data, with what optimisation objective]
- Validation: [per-model: validation procedure, held-out dataset, pass/fail criteria]
- Fairness evaluation: see `fairness-testing-protocol.md`
- Third-party pre-trained components: [per-model: list + licence + version pin]

### 2.2 Design specifications
- Architecture: [per-model: family, depth, parameters, embedding dim]
- Key design choices and assumptions
- Trade-offs (accuracy vs. latency vs. demographic parity — documented with rationale)
- Main classification / detection categories: face present/absent, match/no-match against vault

### 2.3 System architecture
- Inference router decides edge vs. cloud per `internal/ai/router` (KAI-280).
- Face vault is encrypted with customer-managed keys (CSE-CMK). Keys live in the customer's KMS; Kaivue holds only ciphertext. See `data-governance.md`.
- Face recognition output events flow through `DirectoryIngest.PublishAIEvents` with audit tagging.

### 2.4 Data requirements
See `data-governance.md` for the full Art. 10 mapping. Per-model body records:
- Data provenance (source, licence, acquisition date)
- Data preparation (cleaning, augmentation, labelling)
- Known data characteristics (demographic coverage, known gaps)
- Data availability considerations (whether subjects consented, whether the data is public, whether it is redistributable)

### 2.5 Human oversight measures
- Every face-recognition alert requires human review before any automated action.
- The Customer Admin UI (KAI-327) provides a one-click "disable face recognition on this camera" control at all times.
- The Customer Admin UI displays model version, accuracy metrics, and fairness metrics per active model.
- Operators may mark a match as "false positive" or "false negative"; this feedback flows into post-market monitoring.

### 2.6 Predetermined changes
- Customer-uploaded models (KAI-291) are treated as a distinct AI system — customer becomes the provider under Art. 25 Modifier provisions and the customer agreement reflects this.
- Model updates trigger a re-validation + re-fairness-testing pipeline BEFORE promotion via model registry (KAI-279).

### 2.7 Validation and testing procedures
- Unit + integration tests for the feature pipeline.
- Fairness test suite (`fairness-testing-protocol.md`) runs on every model promotion.
- Synthetic adversarial test set (see `accuracy-robustness-cybersecurity.md`).
- Cybersecurity: pen test (KAI-390) covers the face-recognition API surface.

### 2.8 Cybersecurity measures
- Face vault is encrypted at rest (CSE-CMK) and in transit (mTLS).
- Model artifacts are content-addressed (sha256) in R2 and signed.
- Model loading validates sha256 before execution.
- Audit log (KAI-233) records every vault mutation and every match.
- Access control via Casbin (KAI-225) enforces per-tenant isolation.

## 3. Monitoring, functioning, and control

### 3.1 Capabilities and limitations in performance
- Expected accuracy: per-model target TPR ≥ 0.95 at FPR ≤ 1e-4 on the validation set.
- Known limitations: reduced accuracy on low-resolution feeds (< 200 px face), heavy occlusion, extreme lighting.
- Age-group caveat: per-model body MUST document accuracy delta on subjects under 18, per Art. 9(9).
- Demographic parity: per-model body MUST document demographic parity metrics per `fairness-testing-protocol.md`.

### 3.2 Foreseeable unintended outcomes
- False match: mitigated by human review gate + audit log.
- Missed match: mitigated by operator feedback loop.
- Bias against underrepresented demographics: mitigated by fairness testing + per-model retraining when metrics drop below threshold.

### 3.3 Risks to health, safety, fundamental rights
Documented in `risk-management-system.md`.

### 3.4 Specifications of input data
- Frame resolution ≥ 480p recommended.
- Colour or grayscale.
- Face size ≥ 64×64 pixels recommended.
- Timestamp + camera_id + tenant_id REQUIRED (for audit trail).

### 3.5 Pre-determined changes
See 2.6 above.

## 4. Detailed description of the risk management system

See `risk-management-system.md`.

## 5. Description of changes through the system life cycle

[Per-model body: changelog, version history, reason for each version bump]

## 6. List of harmonised standards applied

See `conformity-assessment.md` — same six standards apply to every model.

## 7. EU declaration of conformity

See `ce-marking.md` for the template EU declaration. One instance per placed-on-market release.

## 8. Description of the system used to evaluate post-market performance

See `post-market-monitoring.md`.

---

## Template instantiation checklist (per-model)

Before a face-recognition model is promoted to `approved` in the model registry (KAI-279), a per-model document instantiating this template MUST exist at `docs/compliance/eu-ai-act/models/<model-id>.md` AND be approved by both lead-ai and lead-security. The `approved` state transition in KAI-279 SHOULD programmatically verify this file exists — implementation note for when we wire the gate.

- [ ] All `[per-model: ...]` placeholders filled.
- [ ] Training data provenance attached or referenced by URL + licence + access date.
- [ ] Fairness test results attached (per `fairness-testing-protocol.md`).
- [ ] Cybersecurity review signed by lead-security.
- [ ] Human-reviewed summary < 500 words suitable for the customer-facing transparency page (`transparency-and-information.md`).
