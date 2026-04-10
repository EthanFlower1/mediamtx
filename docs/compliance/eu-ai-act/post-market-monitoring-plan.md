---
title: Post-Market Monitoring Plan — Face Recognition
owner: lead-security (process) | lead-ai (telemetry)
status: draft
last-updated: 2026-04-08
eu-ai-act-article: 72, 73
---

# Post-Market Monitoring Plan

Article 72 requires providers of high-risk AI systems to establish and
document a post-market monitoring system proportionate to the nature of the
system and its risks. Article 73 requires serious-incident reporting to
market surveillance authorities. This document defines both for Kaivue face
recognition.

## Objectives

1. Continuously verify that the shipped model performs as claimed.
2. Detect drift, degradation, and previously unknown failure modes.
3. Detect operator misuse.
4. Meet the Art. 73 serious-incident reporting deadlines.
5. Feed findings back into the Article 9 risk management plan.

## Telemetry schema

Collected per customer tenant and aggregated. Raw biometric data is NEVER
telemetered. Face images stay in the tenant. Only derived metrics leave.

**Per-inference metrics (aggregated, never raw):**

- Tenant id (hashed for aggregate views)
- Site id (hashed)
- Model version hash
- Timestamp bucket (minute or hour)
- Match decision counts (match / no-match / inconclusive)
- Confidence score histogram (10 buckets, documented)
- Operator decision counts (confirm / reject / escalate / pending)
- Time-to-decision distribution
- Failure counts by class (camera offline, quality-reject, timeout)

**Per-customer aggregates published to the monitoring dashboard:**

- Match rate
- Operator override rate (rejects / total reviews)
- Confidence distribution compared to a reference baseline
- False-positive reports via in-app channel
- Mean time-to-decision
- Feature uptime / error rate

## Drift detection

Weekly job computes KL divergence between the current week's confidence
histogram and the reference baseline captured at model release.

- **Green:** KL < T1 (tight bound). Document T1 in the model card.
- **Yellow:** T1 <= KL < T2. Alert lead-ai. Investigate but do not block.
- **Red:** KL >= T2. Alert lead-ai and lead-security. Freeze model rollout
  until triaged. Consider rollback.

A separate monthly job re-runs the fairness test set against the current
production model and checks disparity ratios are still within bounds defined
in `fairness-testing-protocol.md`.

**TODO (lead-ai):** pick numeric T1, T2 after a month of real telemetry.
**TODO (lead-ai):** specify the data pipeline: telemetry store, batch job
owner, dashboard location, alert destination.

## Customer feedback channel

- In-app **Report a false positive** and **Report a false negative** button
  on every match review screen.
- Feedback report includes: timestamp, camera, confidence score, operator
  note, a de-identified reference id. No face image is exfiltrated to
  Kaivue support; the customer retains the images.
- Triage SLA: lead-ai reviews within 3 business days.
- Aggregated feedback flows into the next training-data refresh.

Customers also have a support email for direct contact and a formal
complaint channel via the Trust Center.

## Operator-behavior monitoring

Rubber-stamping and disengagement detection are described in
`human-oversight.md`. Aggregate rubber-stamping incidents per month are a
KPI reported to lead-security.

## Serious incident reporting (Article 73)

A **serious incident** under the AI Act includes, among others:

- any incident or malfunctioning that directly or indirectly leads to death
  or serious harm to a person's health,
- serious and irreversible disruption of critical infrastructure,
- infringement of obligations under Union law intended to protect fundamental
  rights,
- serious harm to property or the environment.

For Kaivue face recognition the most plausible serious incident is a
**misidentification leading to harm** — e.g., a wrongly matched person is
detained, denied entry to a needed location, publicly accused, or otherwise
suffers material harm as a result of the system's output.

### Reporting procedure

1. Any report reaching Kaivue (via support, customer escalation, press, DPA
   inquiry) that alleges harm from a face recognition output is **immediately
   classified as a potential serious incident** and routed to lead-security.
2. Lead-security opens an incident record with a fixed timeline:
   - **Hour 0:** initial triage, notification to CISO, preservation of
     relevant audit logs (legal hold).
   - **Day 1–3:** facts-gathering with the customer, confirmation of whether
     the system's output materially contributed to the harm.
   - **Day 5:** draft incident report.
   - **Day 10:** internal review and sign-off.
   - **Day 15 at the latest:** report submitted to the market surveillance
     authority of the Member State where the incident occurred. Article 73
     sets a 15-day maximum; for incidents causing death or widespread harm
     the deadline is 2 days.
3. Customer is notified of Kaivue's submission and provided a copy.
4. Lessons-learned review within 30 days, feeding back to the Art. 9 risk
   register.

**TODO (lead-security):** maintain a current list of competent market
surveillance authority contact details for each EU Member State.
**TODO (legal):** confirm Kaivue's legal entity and EU representative for
filing purposes.

## Reporting cadence

- **Daily:** automated anomaly alerts to on-call
- **Weekly:** lead-ai drift review
- **Monthly:** lead-security + lead-ai joint review, KPIs reported to CISO
- **Quarterly:** DPO and legal review; customer-visible summary published
  in the Trust Center (KAI-394)
- **Annually:** external audit sample for SOC 2 / ISO 42001 alignment

## Records retention

Post-market monitoring records retained for 10 years per Article 18
(documentation retention obligation), including:

- Telemetry aggregates
- Drift alerts and resolution notes
- Customer feedback reports and dispositions
- Serious incident files
- Corrective action logs

## Integration with other processes

- SOC 2 Type I (KAI-385): incident process reuses the SOC 2 IR runbook.
- Audit log service (KAI-233): source of truth for operator behavior metrics.
- Model registry (KAI-279): source of truth for model versions in production.
- Trust Center (KAI-394): publication surface.

## TODOs

- [ ] lead-ai — build the drift dashboard + thresholds
- [ ] lead-security — Member State authority contacts
- [ ] lead-security — incident runbook with on-call roster
- [ ] lead-security — annual dry-run of the serious-incident reporting
      procedure with a tabletop exercise
