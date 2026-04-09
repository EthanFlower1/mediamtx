# EU AI Act Compliance Package

**Status:** Draft — awaiting compliance counsel review
**Owner:** lead-ai, lead-security (joint)
**Hard deadline:** 2026-08-02 (EU AI Act effective for high-risk AI systems)
**Linear:** KAI-294, KAI-282
**Last updated:** 2026-04-08

## Why this exists

The EU AI Act (Regulation (EU) 2024/1689) classifies Kaivue's face recognition feature as a **high-risk AI system** under Annex III, §1(a) — "biometric identification and categorisation of natural persons." High-risk systems placed on the EU market after 2 August 2026 must:

1. Complete a conformity assessment under Article 43.
2. Bear a CE marking under Article 48.
3. Maintain technical documentation under Article 11 + Annex IV.
4. Implement risk management under Article 9.
5. Implement data governance under Article 10.
6. Maintain post-market monitoring under Article 72.
7. Report serious incidents under Article 73 within strict timelines.
8. Register the system in the EU AI database under Article 71.

This package is the canonical Kaivue response to each of those requirements. It lives in-repo so it ships with the code it describes and cannot drift.

## Contents

| Document | Article | Status |
|---|---|---|
| [conformity-assessment.md](conformity-assessment.md) | Art. 43 + Annex VI | Draft |
| [ce-marking.md](ce-marking.md) | Art. 48 | Draft |
| [annex-iv-technical-documentation.md](annex-iv-technical-documentation.md) | Art. 11 + Annex IV | Draft — body per-model, see KAI-282 |
| [risk-management-system.md](risk-management-system.md) | Art. 9 | Draft |
| [data-governance.md](data-governance.md) | Art. 10 | Draft |
| [post-market-monitoring.md](post-market-monitoring.md) | Art. 72 | Draft |
| [serious-incident-reporting.md](serious-incident-reporting.md) | Art. 73 | Draft |
| [eu-database-registration.md](eu-database-registration.md) | Art. 71 | Draft |
| [transparency-and-information.md](transparency-and-information.md) | Art. 13 | Draft |
| [human-oversight.md](human-oversight.md) | Art. 14 | Draft |
| [accuracy-robustness-cybersecurity.md](accuracy-robustness-cybersecurity.md) | Art. 15 | Draft |
| [fundamental-rights-impact-assessment.md](fundamental-rights-impact-assessment.md) | Art. 27 | Draft (deployer, but we provide the template) |
| [fairness-testing-protocol.md](fairness-testing-protocol.md) | Art. 10(5) + Art. 15 | Draft |
| [provider-obligations.md](provider-obligations.md) | Art. 16 | Draft |

## What remains blocked on other work

| Blocker | Owner | Why it matters |
|---|---|---|
| KAI-282 face recognition implementation lands | lead-ai | Annex IV technical documentation is per-model; we cannot finish it until we pick the model, record the training dataset, and run the fairness tests. Template is ready. |
| Encrypted vault (CSE-CMK) lands | lead-ai + lead-security | `data-governance.md` references vault key rotation + right-to-erasure. Design memo landing with KAI-282. |
| Audit log facility lands | lead-security (KAI-233) | `post-market-monitoring.md` depends on audit log as the evidence source. |
| SOC 2 control library lands | lead-security (KAI-385) | `provider-obligations.md` cross-references quality management system; significant overlap with SOC 2 controls. |
| Legal review by external counsel | Business | Conformity assessment procedure (self-assessment vs. notified body) is a legal call. Kaivue expects to use self-assessment under Annex VI since face recognition is NOT Annex III §1(a) biometric categorisation of sensitive attributes — but this MUST be confirmed. |

## Review checklist before Aug 2

- [ ] External compliance counsel signs off on conformity assessment path (Annex VI vs. notified body).
- [ ] Notified body engaged if required (4-8 week lead time — must decide by 2026-06-01).
- [ ] Annex IV technical documentation complete per-model, signed by lead-ai.
- [ ] Fairness testing complete with documented demographic parity across tested groups (see `fairness-testing-protocol.md`).
- [ ] CE marking generated + affixed to product distribution (per `ce-marking.md`).
- [ ] EU AI database registration submitted (per `eu-database-registration.md`).
- [ ] Serious-incident reporting pipeline tested end-to-end (per `serious-incident-reporting.md`).
- [ ] Post-market monitoring dashboards live in Grafana (per `post-market-monitoring.md`).
- [ ] Deployer transparency materials published (per `transparency-and-information.md`).
- [ ] Human oversight controls shipped in Customer Admin UI (per `human-oversight.md`).
- [ ] Insurance coverage confirmed — provider liability under Art. 99 is uncapped.

## Hard-no list (will NOT do at v1)

The EU AI Act prohibits certain practices under Article 5. Kaivue WILL NOT implement any of the following, and any customer request to do so is rejected at the product level:

- **Social scoring** (Art. 5(1)(c)).
- **Real-time remote biometric identification in publicly accessible spaces for law enforcement** (Art. 5(1)(h)) — our face recognition is post-hoc search against a customer-managed vault, not a live law-enforcement watchlist.
- **Biometric categorisation inferring race, political opinions, trade union membership, religious or philosophical beliefs, sex life or sexual orientation** (Art. 5(1)(g)).
- **Emotion recognition in workplaces and education** (Art. 5(1)(f)).
- **Predictive policing** (Art. 5(1)(d)).

These are enforced via product policy and documented refusal in `provider-obligations.md`.

## Contact

- Technical owner: lead-ai (this repo)
- Compliance owner: lead-security
- Business / legal owner: (TBD — to be assigned by CTO)
- External compliance counsel: (TBD — to be contracted by 2026-05-01)
