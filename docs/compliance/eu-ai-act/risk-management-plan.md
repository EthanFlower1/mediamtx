---
title: Risk Management Plan — Face Recognition
owner: lead-security (process) | lead-ai (model facts)
status: draft
last-updated: 2026-04-08
eu-ai-act-article: 9
---

# Risk Management Plan — Face Recognition

## Purpose

Article 9 of the EU AI Act requires a **continuous, iterative risk management
system** that runs across the **entire lifecycle** of a high-risk AI system:
development, deployment, and post-market. This document is the top-level plan.
Operational risks detected post-deployment feed back into the development
backlog through the post-market monitoring plan.

## Scope

Kaivue's face recognition feature: enrollment of known persons (employees,
visitors, watchlist) and real-time 1:N identification against the enrolled set
using live camera streams processed by the Recorder and/or a central inference
service. **No code exists yet** — this plan constrains future implementation.

## Lifecycle phases covered

1. **Design & training** — model selection, dataset curation, bias testing
2. **Pre-release** — red-teaming, fairness thresholds, pen testing
3. **Deployment** — per-customer opt-in, configuration hardening
4. **Post-market** — telemetry, drift detection, incident response
5. **Retirement** — model sunset, embedding deletion, customer notification

## Risk inventory

Each risk below includes: identification method, mitigation, residual risk,
acceptance criteria, and review cadence.

### R1. False positive → wrongful exclusion or arrest

- **Identification:** fairness test set evaluation, red team adversarial images,
  customer-reported incidents, operator override rate telemetry.
- **Mitigation:** confidence threshold tuned for low FPR; mandatory operator
  confirmation before any automated action; four-eyes on watchlist additions;
  no direct integration with access control or law enforcement APIs without
  explicit customer configuration.
- **Residual risk:** non-zero. Any biometric system has residual FPR.
- **Acceptance criteria:** aggregate FPR < 0.1% at the chosen operating
  threshold; no demographic group FPR > 1.5x the group with the lowest FPR.
- **Review cadence:** every model update + monthly telemetry review.

### R2. False negative → missed threat

- **Identification:** fairness test set; customer-reported missed identifications.
- **Mitigation:** threshold calibration documented; customers informed in
  instructions-for-use that face recognition is **not a sole safety mechanism**;
  human monitoring remains primary.
- **Residual risk:** non-zero, especially under adverse capture conditions
  (low light, occlusion, extreme pose).
- **Acceptance criteria:** aggregate FNR < 2% at the operating threshold;
  no demographic group FNR > 1.5x the best group.
- **Review cadence:** every model update + monthly.

### R3. Demographic bias (FPR/FNR disparity by skin tone, age, gender)

- **Identification:** stratified test set per `fairness-testing-protocol.md`;
  NIST FRVT published results where available; internal regression tests.
- **Mitigation:** dataset balancing; per-group thresholding considered and
  documented; ship/no-ship gate on disparity ratios; publication of aggregate
  metrics in the Trust Center (KAI-394).
- **Residual risk:** measurable disparity expected. Goal: bounded and disclosed.
- **Acceptance criteria:** max(FPR)/min(FPR) < 1.5 and max(FNR)/min(FNR) < 1.5
  across Fitzpatrick skin tone buckets, age buckets, and gender.
- **Review cadence:** every model update + quarterly regression test.

### R4. Dataset poisoning during training

- **Identification:** dataset provenance audit; hash-verified training sets;
  anomaly detection on per-class loss during training.
- **Mitigation:** no training on customer data by default (explicit opt-in only);
  training data sources restricted to vetted, licensed corpora; SHA-256 manifest
  of every training sample pinned to the training run; reproducible builds.
- **Residual risk:** low if sources are vetted. Higher if customer-data opt-in
  is used at scale.
- **Acceptance criteria:** every shipped model has a signed training manifest;
  zero unverified sources; reproducibility within tolerance documented by lead-ai.
- **Review cadence:** every training run.

### R5. Model theft via inference API

- **Identification:** rate limiting telemetry; anomaly detection on query patterns
  (model extraction attacks often show systematic probing).
- **Mitigation:** per-tenant API rate limits; no raw embedding exposure in API
  responses; only match decisions + confidence; authenticated endpoints only;
  no public-unauthenticated inference path.
- **Residual risk:** low.
- **Acceptance criteria:** rate limits enforced; penetration test includes a
  model-extraction scenario; no unauthenticated inference endpoint exists.
- **Review cadence:** quarterly.

### R6. Enrollment photo exfiltration

- **Identification:** data access audit logs; DLP; customer-reported incidents.
- **Mitigation:** enrollment photos and embeddings encrypted at rest with the
  per-tenant key (references KAI-141); strict tenant isolation (references
  KAI-235); no cross-tenant indexing; no embedding export endpoint.
- **Residual risk:** standard data breach residual.
- **Acceptance criteria:** per-tenant encryption verified; tenant isolation
  integration tests green; pen test includes cross-tenant exfil scenarios.
- **Review cadence:** every release + quarterly.

### R7. Watchlist abuse by operators

- **Identification:** audit log review; anomaly detection on watchlist edits
  per operator; customer complaint channel.
- **Mitigation:** four-eyes approval for watchlist additions (admin + second
  approver, see `human-oversight.md`); every watchlist change is logged with
  reason text; tenant admins can review a per-operator change log.
- **Residual risk:** medium — the most likely misuse vector.
- **Acceptance criteria:** four-eyes enforced in UI and API; no path to add a
  watchlist entry with a single actor; quarterly audit sample by lead-security.
- **Review cadence:** monthly telemetry + quarterly audit.

### R8. Drift over time

- **Identification:** weekly KL divergence on confidence distribution vs.
  baseline; monthly fairness smoke test on held-out set; customer feedback.
- **Mitigation:** model version pinning; canary rollout for updates; rollback
  path; re-enrollment prompts when a person's last enrollment photo is > 5
  years old; automatic alerts when drift metric crosses threshold.
- **Residual risk:** inherent to deployed ML.
- **Acceptance criteria:** drift dashboard in place; alert threshold documented;
  runbook owned by lead-ai.
- **Review cadence:** continuous telemetry + weekly review.

## Governance

- **Risk owner:** lead-security (process), lead-ai (model-level risks)
- **Escalation path:** lead-security → CISO → Board for any R3 breach, any
  R1/R2 incident with reported harm, or any R7 abuse incident.
- **Change control:** any change to acceptance criteria requires written
  approval from both lead-security and lead-ai.

## TODOs

- [ ] lead-ai — confirm numeric thresholds are achievable with the chosen
      model architecture; propose alternates if not.
- [ ] lead-security — draft the per-risk runbook for R1, R5, R6, R7.
- [ ] lead-security — integrate this plan with the SOC 2 risk register
      (KAI-385) so duplicates are eliminated.
- [ ] legal — review whether R1 mitigation language adequately addresses
      Member-State-specific worker protection law.
