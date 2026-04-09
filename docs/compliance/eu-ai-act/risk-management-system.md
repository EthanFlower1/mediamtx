# Risk Management System

**Article:** 9
**Status:** Draft — awaiting compliance counsel review
**Owner:** lead-ai + lead-security (joint)

## Purpose

Article 9 requires a continuous, iterative risk management process running across the entire lifecycle of a high-risk AI system. This document defines Kaivue's process for face recognition. It is not a one-off risk assessment — it is a loop that runs forever.

## Scope

Applies to every face-recognition model placed on the market by Kaivue under any of:

- Embedded Recorder appliance (on-prem).
- Kaivue SaaS cloud control plane.
- Customer-uploaded custom models via KAI-291 (where Kaivue retains platform-provider obligations).

## The loop

Per Art. 9(2), the process runs as a documented iterative loop:

1. **Identification and analysis** of known and reasonably foreseeable risks to health, safety, and fundamental rights that the system poses when used as intended or under reasonably foreseeable misuse.
2. **Estimation and evaluation** of risks that may emerge when the system is used in accordance with its intended purpose and under conditions of reasonably foreseeable misuse.
3. **Evaluation of other risks possibly arising** based on post-market monitoring data (see `post-market-monitoring.md`).
4. **Adoption of appropriate and targeted risk-management measures** designed to address the risks identified.

The loop is triggered by any of:
- A new model version reaching `candidate` state in the model registry (KAI-279).
- A serious incident being opened under `serious-incident-reporting.md`.
- A post-market monitoring alert (drift, fairness-metric drop, error-rate spike).
- A material product scope change (new deployment context, new customer vertical).
- Quarterly, unconditionally, on the first business day of the quarter.

Each loop execution produces a dated entry in the per-model risk register (per-model body; template below).

## Identified risk categories

The following categories are always evaluated. Per-model instances document the concrete findings and residual risk after mitigation.

### R1 — False match (false positive)

- **Harm:** A person is wrongly flagged as matching a watchlist entry. Consequences range from embarrassment to wrongful detention.
- **Likelihood driver:** Model accuracy, vault quality, lighting, camera resolution, demographic coverage.
- **Mitigation layers:**
  1. Human review gate — every match requires human confirmation before any customer-configured action (Art. 14, see `human-oversight.md`).
  2. Configurable match threshold per vault, defaulting to a conservative FPR.
  3. Audit log (KAI-233) records every match with the confidence score and reviewer decision.
  4. Operator feedback loop: confirmed-false-positives feed back into `post-market-monitoring.md` and trigger retraining when thresholds are crossed.
- **Residual risk owner:** customer deployer under Art. 26, with Kaivue providing the tools to make human review effective.

### R2 — Missed match (false negative)

- **Harm:** A known threat is not identified; the customer's security objective is undermined.
- **Likelihood driver:** Same as R1, plus occlusion and pose variation.
- **Mitigation layers:**
  1. Per-camera enrolment quality feedback in KAI-327 admin UI (reject low-quality enrolments).
  2. Operator-tagged false negatives feed `post-market-monitoring.md`.
  3. Documented accuracy envelope in `accuracy-robustness-cybersecurity.md` so deployers understand where the system will not help them.

### R3 — Demographic bias

- **Harm:** Different error rates across demographic groups, creating discriminatory impact in violation of Art. 9(9) and Art. 10(5) and EU fundamental-rights law.
- **Likelihood driver:** Training data composition, benchmark skew, deployment population drift.
- **Mitigation layers:**
  1. Fairness testing gate at promotion: `fairness-testing-protocol.md` MUST pass before any model reaches `approved` in KAI-279.
  2. Ongoing fairness monitoring in post-market phase with an alerting threshold; breach triggers a loop re-run and may trigger field correction per `serious-incident-reporting.md`.
  3. Documented demographic coverage in `data-governance.md` per-model body.
  4. Hard-no list (see `README.md`) forbids any use as a law-enforcement biometric identification system in publicly accessible spaces.

### R4 — Unauthorised vault access or exfiltration

- **Harm:** Biometric data leak. Biometric data is special-category personal data under GDPR Art. 9.
- **Likelihood driver:** Credential compromise, misconfigured tenant isolation, software vulnerabilities.
- **Mitigation layers:**
  1. Customer-side encrypted vault (CSE-CMK) — Kaivue holds only ciphertext (see `data-governance.md`).
  2. Multi-tenant isolation enforced at every layer: database (row-level + `tenant_id` on every row), API (Casbin policies — KAI-225), storage (per-tenant S3/R2 prefixes), inference (per-tenant Triton model repo paths).
  3. mTLS on all inference and control-plane traffic.
  4. Annual third-party pen test (KAI-390).
  5. Audit log covering every vault read, write, and mutation.

### R5 — Model artifact tampering

- **Harm:** A malicious actor substitutes a corrupted or backdoored model, causing targeted misclassification.
- **Likelihood driver:** Supply-chain compromise, insider threat.
- **Mitigation layers:**
  1. Model artifacts are content-addressed (sha256) in R2 and cryptographically signed.
  2. Recorder + Triton loaders verify sha256 and signature before loading.
  3. Model registry (KAI-279) is the single source of truth for which model is approved for which tenant.
  4. Reproducible build pipeline (KAI-428) so artifact provenance is verifiable.

### R6 — Misuse against Kaivue intended purpose

- **Harm:** Customer deploys face recognition in a scenario outside the documented intended purpose (e.g. real-time law-enforcement watchlist in a publicly accessible space), crossing into Art. 5 prohibited territory.
- **Likelihood driver:** Customer ignorance, wilful misuse, deployment context drift.
- **Mitigation layers:**
  1. `transparency-and-information.md` sets the intended-purpose boundary in customer-facing language.
  2. Customer agreement (TBD legal) contractually prohibits use cases in the hard-no list.
  3. EU-region deployments ship with real-time live detection DISABLED by default pending counsel resolution of Art. 5(1)(h) scope (see open legal questions in `conformity-assessment.md`).
  4. `fundamental-rights-impact-assessment.md` template forces customers to document their deployment context.

### R7 — Failure of human oversight

- **Harm:** Operators rubber-stamp matches without review, collapsing the human-gate mitigation for R1.
- **Likelihood driver:** Alert volume, operator fatigue, UX that defaults-to-accept.
- **Mitigation layers:**
  1. Human-oversight controls specified in `human-oversight.md` include non-default confirmation steps, visible confidence scores, and operator workload metrics.
  2. Post-market monitoring tracks per-operator confirm-rates; anomalous confirm-rates (e.g. 100% confirmations with zero operator-elapsed-time) trigger an account review.
  3. Customer Admin UI (KAI-327) exposes per-camera and per-operator audit trails.

### R8 — Data-subject rights failure

- **Harm:** Data subject cannot exercise GDPR rights (erasure, access, objection) because vault design or operations do not support it.
- **Likelihood driver:** Design that couples identity to immutable vectors; operational gap between customer control plane and inference stack.
- **Mitigation layers:**
  1. Vault design supports per-subject erasure in a documented SLA (see `data-governance.md`).
  2. Erasure propagates to model caches and Triton in-memory state via a documented invalidation path.
  3. Audit log records erasure operations for the 10-year retention period.

## Risk register (per-model)

Every per-model instance of `annex-iv-technical-documentation.md` carries a risk register table of this shape:

| Risk ID | Description | Inherent likelihood | Inherent impact | Mitigation(s) applied | Residual likelihood | Residual impact | Accepted by |
|---|---|---|---|---|---|---|---|
| R1 | False match | ... | ... | ... | ... | ... | lead-ai + lead-security |
| R2 | Missed match | ... | ... | ... | ... | ... | lead-ai |
| ... | ... | ... | ... | ... | ... | ... | ... |

Acceptance criteria for promoting a model to `approved`:

- Every row MUST have residual likelihood × residual impact ≤ the tolerance defined in `fairness-testing-protocol.md` for bias-related risks, and ≤ the tolerance set by lead-security for security risks.
- Acceptance signature by both lead-ai and lead-security is MANDATORY for any row whose residual impact is "severe" or worse.

## Change log

The risk register is maintained under version control. Every loop execution produces a new dated entry in the per-model risk register; prior entries are retained (never overwritten) so the iterative nature required by Art. 9(2) is audit-traceable.

## Interactions with other documents

- `data-governance.md` — data risks feed R3, R4, R8.
- `accuracy-robustness-cybersecurity.md` — technical controls feed R1, R2, R4, R5.
- `human-oversight.md` — operational controls feed R1, R7.
- `post-market-monitoring.md` — continuous signal feeding loop step 3.
- `serious-incident-reporting.md` — escalation path when a risk materialises in production.
- `fairness-testing-protocol.md` — operationalises R3 mitigation.
