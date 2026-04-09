# Post-Market Monitoring

**Article:** 72
**Status:** Draft — dashboards and pipelines land with KAI-282
**Owner:** lead-ai + lead-security

## Purpose

Art. 72 requires providers of high-risk AI systems to establish and document a post-market monitoring system proportionate to the nature of the system and its risks, collecting, documenting, and analysing relevant data on system performance throughout the lifetime.

This document defines Kaivue's post-market monitoring plan for face recognition.

## Objectives

1. **Continuous conformity.** Verify that the system in the field continues to meet the essential requirements of Chapter III Section 2.
2. **Drift detection.** Detect degradation in accuracy, robustness, or fairness before it causes customer harm.
3. **Misuse detection.** Surface patterns suggesting the system is being used outside its intended purpose.
4. **Feedback to the risk management loop.** Feed data into the Art. 9 loop (see `risk-management-system.md`).
5. **Incident pipeline feed.** Provide the signal that escalates into the Art. 73 serious-incident reporting path (see `serious-incident-reporting.md`).

## Data sources

### Runtime telemetry

- Per-inference latency, result, confidence score, feature used, hardware class, edge/cloud location (from the AI router — KAI-280).
- Per-tenant counts of matches, non-matches, low-confidence rejections.
- GPU saturation and fallback events from the router.
- Model version actually loaded at the time of the inference (from model registry — KAI-279).

Telemetry is aggregated via the Kaivue observability stack and surfaced in Grafana dashboards.

Telemetry DOES NOT include face images, embeddings, or subject identifiers. It is operational metrics only.

### Operator feedback

- Match-review decisions from operators (confirmed / dismissed / escalated).
- Operator-reported false positives and false negatives.
- Time-to-review distributions per operator and per tenant.

This channel is how the human-oversight layer feeds back into improvement. It is anonymised before ingestion into any retraining pipeline.

### Audit log (KAI-233)

- Vault mutations (add, remove, rename).
- Model-registry transitions (promote, deprecate, rollback).
- Administrative actions (feature enable/disable per camera, threshold changes).
- Access to compliance-relevant APIs.

Audit log is the canonical evidence source for post-market compliance investigations.

### Customer support and incident channels

- Customer-reported complaints.
- Field-observed incidents from customer admin staff.
- Integrations with customer ticketing systems where available.

### External signals

- Published CVEs affecting the inference stack, ONNX Runtime, Triton, CUDA, or model formats.
- Public fairness research that might change our thresholds.
- Regulatory guidance from the EU AI Office.

## Metrics and thresholds

| Metric | Source | Alert threshold | Action |
|---|---|---|---|
| Per-tenant false-positive rate | Operator feedback | > 2× baseline for 3 consecutive days | Open investigation ticket, page lead-ai |
| Per-cell equalised-odds TPR gap | Weekly fairness re-run | > 0.05 absolute | Re-run Art. 9 loop, consider rollback |
| Model-load sha256 mismatch | Runtime | Any occurrence | Page lead-security; halt new loads until resolved |
| Edge-fallback rate to cloud | Router telemetry | > 20% for any feature for 1 hour | Investigate probe accuracy, probable hardware degradation |
| Audit-log ingestion lag | Audit-log service | > 5 minutes | Page lead-security; monitoring blind spot |
| Operator confirm-rate (=100% in a session) | Operator feedback | Any occurrence with > 20 reviews | Account review, possible human-oversight failure |
| Unexpected 0–17 band activity spike | Operator feedback + demographic estimators | Any spike outside baseline | Investigate (possible minor-targeted use, Art. 9(9) concern) |

Thresholds are reviewed quarterly.

## Dashboards

Primary dashboards live in Grafana under the `/ai/face-recognition/post-market` folder. At v1 they include:

- **Accuracy and fairness.** Per-tenant and aggregated TPR/FPR, equalised-odds gap, weekly fairness re-run results.
- **Operational.** Inference latency, edge/cloud split, fallback rates, saturation events.
- **Human oversight.** Operator confirm-rate distributions, time-to-review, per-operator outlier detection.
- **Security.** Model-load verification events, audit-log ingestion health, vault access rates.
- **Data governance.** Erasure request queue depth, erasure SLA adherence.

The dashboard URLs are linked from the `transparency-and-information.md` deployer documentation so that customer compliance staff can see the relevant-to-them slices.

## Review cadence

- **Daily:** automated alerts and on-call triage.
- **Weekly:** lead-ai reviews the fairness re-run results and opens tickets for any drift.
- **Monthly:** joint lead-ai / lead-security post-market review meeting; minutes retained in the 10-year retention store.
- **Quarterly:** full risk management loop execution per `risk-management-system.md`, informed by the post-market signal.

## Escalation to serious-incident reporting

The following post-market signals escalate to an Art. 73 serious-incident investigation (see `serious-incident-reporting.md`):

- A confirmed wrong identification that caused or could have caused harm to health, safety, or fundamental rights.
- A bias metric breach that persists across multiple weekly re-runs and cannot be remediated in the ordinary loop.
- A security event affecting vault confidentiality.
- A model-integrity failure (sha256 mismatch, signature failure).

## Retention

All post-market monitoring data and the decisions made in response to it are retained per Art. 18: **10 years** from the last placing-on-market of the system they relate to. Storage mechanisms:

- Grafana metric data is backed by long-term storage in R2 cold archive (KAI-266).
- Decision records (meeting minutes, tickets, investigation reports) live in the repo under `docs/compliance/eu-ai-act/incidents/` and in Linear (referenced by stable ID).
- Audit log archives follow the same 10-year policy.

## Interactions with other documents

- `risk-management-system.md` — feeds Art. 9 loop step 3 (risks arising from post-market data).
- `serious-incident-reporting.md` — escalation target for high-severity signals.
- `fairness-testing-protocol.md` — weekly re-run is implemented by the fairness suite.
- `accuracy-robustness-cybersecurity.md` — defines the operational baselines that drift alerts reference.
