---
title: EU AI Act Compliance Package — Face Recognition (Kaivue Recording Server)
owner: lead-security (process) | lead-ai (model facts)
status: draft
last-updated: 2026-04-08
eu-ai-act-article: 6, 43, 49
---

# EU AI Act Compliance Package — Face Recognition

This directory holds the compliance artifacts required to place Kaivue's face
recognition feature on the EU market as a **high-risk AI system** under the
EU AI Act (Regulation (EU) 2024/1689).

**Linear tickets:** KAI-282 (compliance package), KAI-294 (CE marking), task #41.
**Co-owners:** lead-security (process, governance, legal interface) and lead-ai
(model facts, metrics, dataset documentation).

## Hard deadline

**2026-08-02** — CE marking must be in place before the feature is offered
to any customer with an EU establishment or EU end users. Miss this date and
we cannot sell face recognition in the EU.

## Classification

Face recognition is **Annex III, point 1(a)** — "biometric identification and
categorisation of natural persons" — which makes it a **high-risk** AI system.
This triggers the obligations in Title III, Chapter 2 (Articles 8–15), the
quality management system obligations in Article 17, the conformity assessment
obligation in Article 43, the registration obligation in Article 49, and the
post-market monitoring obligations in Articles 72–73.

## Current implementation status

As of 2026-04-08, **no face recognition code exists in this repository**. Recon
performed during package creation:

- `internal/recorder/features/` contains: `lpr/`, `objectdetection/` (no `face/`)
- `internal/ai/behavioral/` contains: behavioral analytics (fall, loitering,
  line crossing, crowd density, tailgating, ROI) — no face recognition
- `ui-v2/src/` contains no face-related screens
- No model weights, no face-embedding store, no enrollment flow

This package is therefore **requirements-ahead-of-implementation**. Every
document here should be treated as a binding constraint for implementation
work tracked in future tickets. Implementation PRs MUST reference the relevant
document in this package.

## Conformity assessment route (Article 43)

Article 43 gives providers of Annex III point 1 systems a choice:

1. **Internal control (Annex VI)** — allowed only if the provider applies
   harmonized standards (or common specifications) that cover all essential
   requirements in Chapter 2. Cheaper, faster, no notified body.
2. **Notified body (Annex VII)** — required if no harmonized standard applies
   or the provider chooses not to follow one in full. External third party.

For biometric identification systems under Annex III point 1(a), internal
control is **permitted** if a harmonized standard is followed in full.

**Current recommendation:** pursue the internal-control route, contingent on
the following harmonized / international standards being in force and applied:

- **ISO/IEC 42001:2023** — AI management system
- **ISO/IEC 22989:2022** — AI concepts and terminology
- **ISO/IEC TR 24027:2021** — Bias in AI systems and AI-aided decision making
- **ISO/IEC TR 24028:2020** — Trustworthiness in AI
- **ISO/IEC 23894:2023** — Risk management

**TODO (lead-security, legal):** confirm with outside EU counsel which of the
above are formally harmonized under the AI Act (listed in the Official Journal
of the EU) as of the decision date. If any Chapter-2 essential requirement is
not covered by a harmonized standard at decision time, we MUST fall back to
the notified body route, which needs 6–9 months lead time. Decision deadline:
**2026-05-15**.

**TODO (lead-security):** if internal-control route is chosen, draft the EU
Declaration of Conformity (Annex V) and identify the signatory (must be an
authorized representative established in the EU under Article 22 if Kaivue
itself is not EU-established).

## Package contents and order of operations

The 10 files in this package, in the order they should be authored and
approved:

1. `README.md` (this file) — index, timeline, route decision
2. `risk-management-plan.md` (Art. 9) — drives everything else
3. `data-governance.md` (Art. 10) — dataset facts, bias testing inputs
4. `fairness-testing-protocol.md` — measurement methodology
5. `human-oversight.md` (Art. 14) — UI and operator constraints
6. `transparency-and-instructions-for-use.md` (Art. 13) — customer-facing
7. `audit-log-requirements.md` — evidence trail (feeds post-market monitoring)
8. `post-market-monitoring-plan.md` (Art. 72, 73) — after shipping
9. `opt-in-consent-flow.md` — GDPR + AI Act intersection
10. `dpia-template.md` — customer-facing artifact (ships with the product)

Annex IV technical documentation is assembled from (2)–(8) plus the model
card and training run records maintained by lead-ai.

## Ownership split

| Area                               | Owner            | Notes                             |
| ---------------------------------- | ---------------- | --------------------------------- |
| Process, policy, legal interface   | lead-security    |                                   |
| Model facts, metrics, datasets     | lead-ai          | Provides inputs for (3), (4), (6) |
| Product UI for oversight & consent | lead-ai + design | Specs in (5), (9)                 |
| Audit log plumbing                 | lead-security    | References KAI-233                |
| Trust Center publication           | lead-security    | References KAI-394                |

## Article-to-file crosswalk

| Article | Topic                        | File(s)                                               |
| ------- | ---------------------------- | ----------------------------------------------------- |
| Art. 9  | Risk management              | risk-management-plan.md                               |
| Art. 10 | Data governance              | data-governance.md, fairness-testing-protocol.md      |
| Art. 12 | Record keeping               | audit-log-requirements.md                             |
| Art. 13 | Transparency                 | transparency-and-instructions-for-use.md              |
| Art. 14 | Human oversight              | human-oversight.md                                    |
| Art. 15 | Accuracy/robustness/cybersec | fairness-testing-protocol.md, risk-management-plan.md |
| Art. 43 | Conformity assessment        | README.md                                             |
| Art. 49 | Registration + CE marking    | README.md                                             |
| Art. 72 | Post-market monitoring       | post-market-monitoring-plan.md                        |
| Art. 73 | Serious incident reporting   | post-market-monitoring-plan.md                        |

## Escalation and review cadence

- Weekly stand-up between lead-security and lead-ai until 2026-06-01
- Monthly review with legal and the DPO through CE marking
- Post-CE: quarterly review of every document in this package

## TODO hotlist

- [ ] lead-security — confirm harmonized standards (Art. 43 route) by 2026-05-15
- [ ] lead-ai — fill in all `TODO(lead-ai)` markers across this package
- [ ] legal — sign off on `transparency-and-instructions-for-use.md`
- [ ] DPO — sign off on `data-governance.md` and `dpia-template.md`
- [ ] lead-security — identify EU authorized representative under Article 22
