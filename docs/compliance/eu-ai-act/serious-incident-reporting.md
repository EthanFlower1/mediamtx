# Serious Incident Reporting

**Article:** 73
**Status:** Draft — pipeline implementation tracked under KAI-233 + KAI-294
**Owner:** lead-security + lead-ai

## Purpose

Art. 73 requires providers of high-risk AI systems to report serious incidents to the market surveillance authorities of the Member States where the incident occurred, within strict timelines. This document defines Kaivue's pipeline.

## Definitions

A **serious incident** under Art. 3(49) of the AI Act means an incident or malfunction leading, directly or indirectly, to any of:

- Death of a person, or serious harm to a person's health.
- A serious and irreversible disruption of the management and operation of critical infrastructure.
- Infringement of obligations under Union law intended to protect fundamental rights.
- Serious harm to property or the environment.

For a face-recognition system the most plausible pathways are:

1. A wrongful identification triggering a physical intervention that caused serious harm.
2. A bias-driven pattern of errors infringing fundamental-rights protections at scale.
3. A security breach of biometric data (vault exfiltration) that crosses into fundamental-rights infringement.

## Reporting timelines (Art. 73(2))

| Incident type | Deadline |
|---|---|
| Any serious incident | Immediately after the provider establishes a causal link (or reasonable likelihood of one), and not later than **15 days** after awareness. |
| Widespread infringement or infringement affecting fundamental rights | Not later than **2 days** after awareness. |
| Death of a person | Not later than **10 days** after awareness. |

"Awareness" means the moment Kaivue has enough information to reasonably conclude the event meets the Art. 3(49) threshold.

## Pipeline

### Intake

Signals that can open an incident:

1. Post-market monitoring alert crossing a serious threshold (see `post-market-monitoring.md`).
2. Customer-reported incident via support channel.
3. Regulator inquiry referencing a specific event.
4. Internal discovery (engineer noticing an anomaly).
5. Third-party disclosure (security researcher, press report).

All of these funnel into a single Linear project `KAI-INCIDENTS` (new, to be created by lead-security) with a standard template.

### Triage

Within **24 hours** of any intake, lead-security and lead-ai jointly triage:

- Does the event meet the Art. 3(49) threshold?
- Which of the 2-day / 10-day / 15-day clocks is running?
- Which Member States are in scope?
- What are the contained-by-default mitigations (disable feature per tenant, rollback model, revoke vault access)?
- Who is the external disclosure lead?

Triage minutes are retained.

### Containment

If the event is ongoing, immediate containment steps are taken in parallel with the investigation:

- Disable the implicated feature for the affected tenant(s) via the customer admin switch (KAI-327) or via a provider-initiated kill-switch for widespread events.
- Rollback the implicated model to the previous approved version via the model registry (KAI-279).
- Revoke compromised vault access via KMS key rotation for affected tenant(s) (this invalidates all cached vault keys).
- Isolate affected infrastructure.

### Investigation

Investigation produces a dated report with:

- Factual narrative.
- Technical root cause.
- Scope of affected users / tenants / Member States.
- Causal link analysis to the Art. 3(49) threshold.
- Corrective actions taken and planned.
- Lessons learned feeding back into `risk-management-system.md`.

The investigation report is retained for the 10-year period.

### External disclosure

Disclosures to competent authorities are prepared by the external disclosure lead (lead-security by default, with business / legal sign-off) and submitted through the channels designated by each affected Member State's market surveillance authority.

The AI Act anticipates that a single Union-level reporting portal will exist — once the portal is operational, Kaivue routes all reports through it. Until then, per-Member-State submissions are tracked in the incident Linear record.

Simultaneously:
- Customers in the affected Member States are notified per the customer agreement's incident-notification clause.
- Data-subject notifications are made where GDPR Art. 34 applies.
- Public disclosure is coordinated with the business team.

### Post-incident loop

Every serious incident triggers:

- A re-execution of `risk-management-system.md` (the loop explicitly lists "serious incident" as a trigger).
- An entry in the incident log retained alongside the compliance package.
- A review of whether the existing mitigation layers were sufficient, and whether new ones are needed.
- An update to `post-market-monitoring.md` thresholds if the incident exposed a monitoring gap.

## Customer obligations

Customers (deployers under Art. 26) have their own reporting obligations to market surveillance authorities. The customer agreement obligates customers to:

- Notify Kaivue without undue delay of any incident they believe may be a serious incident.
- Cooperate in the investigation.
- Preserve evidence (audit logs, incident tickets, model version at time of incident).

Kaivue in turn commits to:

- Provide technical support for customer-led investigations.
- Provide the data needed for the customer's own Art. 73 reporting where the customer is obligated.
- Coordinate timelines so that provider and deployer reports are consistent.

## Retention

- Incident reports: 10 years per Art. 18.
- Evidence (audit log excerpts, model artifacts, telemetry): 10 years.
- Disclosure correspondence: 10 years.

## Open operational items

- [ ] Create `KAI-INCIDENTS` Linear project (lead-security).
- [ ] Wire the post-market alerting system to auto-file a KAI-INCIDENTS ticket on any serious-threshold breach.
- [ ] Identify the specific market surveillance authority contact for each Member State we ship into.
- [ ] Legal review of the disclosure template.
- [ ] Tabletop exercise before 2026-08-02.

## Interactions with other documents

- `risk-management-system.md` — receives feedback after every incident.
- `post-market-monitoring.md` — feeds the intake channel.
- `provider-obligations.md` — this document IS the implementation of part of Kaivue's provider obligations.
