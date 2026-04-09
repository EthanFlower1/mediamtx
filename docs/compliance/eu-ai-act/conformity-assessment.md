# Conformity Assessment

**Article:** 43 + Annex VI
**Status:** Draft — legal sign-off required before 2026-06-01
**Owner:** lead-security + external compliance counsel

## Chosen path

Kaivue intends to use **Annex VI conformity assessment based on internal control** (self-assessment) for face recognition at v1. This path is available for Annex III high-risk systems provided the provider:

1. Applies a harmonised standard or common specification covering all relevant essential requirements, OR provides alternative equivalent technical documentation; AND
2. Has a quality management system in place (Article 17).

**Contingency:** if counsel determines we must instead go through a **notified body** (Annex VII), we engage one by 2026-06-01. Notified-body lead time is 4–8 weeks, so decision MUST be made no later than 2026-05-15.

## Applicable harmonised standards

As of 2026-04-08 the following are drafted or in-force under the AI Act's harmonised standards mandate:

- **EN ISO/IEC 23894** — Information technology — Artificial intelligence — Guidance on risk management. (Covers Art. 9.)
- **EN ISO/IEC 42001** — AI management systems. (Covers Art. 17.)
- **EN ISO/IEC 25059** — Quality model for AI systems. (Covers Art. 15 accuracy/robustness.)
- **EN ISO/IEC 24027** — Bias in AI systems and AI aided decision making. (Covers Art. 10(5) + Art. 15 fairness.)
- **EN ISO/IEC 5338** — AI system life cycle processes.
- **EN ISO/IEC 5469** — Functional safety and AI systems.

Kaivue applies all six above. Gaps between what the standards cover and what the AI Act requires are filled by in-package technical documentation (this repo).

## Essential requirements mapping

Annex VI requires that internal control verify conformity with the Chapter III Section 2 essential requirements. The mapping below is the audit trail.

| Article | Essential requirement | Evidence document |
|---|---|---|
| 9 | Risk management system | [risk-management-system.md](risk-management-system.md) |
| 10 | Data and data governance | [data-governance.md](data-governance.md) |
| 11 + Annex IV | Technical documentation | [annex-iv-technical-documentation.md](annex-iv-technical-documentation.md) |
| 12 | Automatic recording of logs | audit-log facility (KAI-233) + [post-market-monitoring.md](post-market-monitoring.md) |
| 13 | Transparency and provision of information to deployers | [transparency-and-information.md](transparency-and-information.md) |
| 14 | Human oversight | [human-oversight.md](human-oversight.md) |
| 15 | Accuracy, robustness, and cybersecurity | [accuracy-robustness-cybersecurity.md](accuracy-robustness-cybersecurity.md) + pen-test report (KAI-390) |
| 16 | Provider obligations | [provider-obligations.md](provider-obligations.md) |
| 17 | Quality management system | SOC 2 Type I control library (KAI-385) + ISO/IEC 42001 mapping |
| 27 | Fundamental rights impact assessment (deployer obligation) | [fundamental-rights-impact-assessment.md](fundamental-rights-impact-assessment.md) — template for customers |

## Internal control procedure

Per Annex VI §4:

1. **Provider declares conformity** in a written EU declaration of conformity (template in `ce-marking.md`).
2. **Provider draws up technical documentation** per Article 11 + Annex IV (template + per-model body in `annex-iv-technical-documentation.md`).
3. **Provider verifies the design and development process** including data governance, risk management, and testing, against this package and the harmonised standards above.
4. **Provider keeps technical documentation at the disposal of national competent authorities for 10 years** after the system is placed on the market (Art. 18).
5. **Provider affixes the CE marking** to the product and to the EU declaration of conformity.
6. **Provider registers the system** in the EU database under Art. 71 before placing it on the market.

## Evidence retention

All evidence referenced in this package MUST be retained for **10 years** after the last instance of the system is placed on the market (Art. 18). Kaivue retention is enforced by:

- Git history: this repo is the source-of-truth for documentation. Retain the repo (GitHub + off-site mirror) for 10 years past end-of-sale.
- Audit log: customer-side audit log (KAI-233) retains every face-recognition match, every vault operation, every model promotion, for 10 years. Archive to R2 cold storage on rotation (KAI-266).
- Model artifacts: model registry (KAI-279) retains every deployed model binary, version, metrics, and approval state. Content-addressed storage in R2.
- Training data provenance: per-model data governance records in `data-governance.md` link to the dataset licence, source, and access date. Datasets themselves are retained under their own licence terms; where licence prohibits long retention we retain the *provenance record* only (allowed under Art. 10(5) legitimate interest).

## Open legal questions (for counsel)

1. **Are our customers "deployers" or "users" under Art. 3?** — Our customers operate the system in their own premises, so they are deployers. They acquire Art. 26 deployer obligations (monitoring, logging, human oversight, FRIA in Art. 27 contexts). This must be reflected in the standard customer agreement.
2. **Is Kaivue a "provider" even when a customer uploads their own model via KAI-291?** — If a customer trains their own face model and uploads it to run on our platform, who bears the provider obligations? Our current product position: customer becomes the provider of their custom model (Art. 25 "Modifier"), Kaivue remains the provider of the platform. Custom model upload acceptance terms must reflect this. Counsel to confirm.
3. **Notified body or self-assessment?** — As documented above, we intend Annex VI self-assessment. Counsel must confirm based on the exact feature scope at ship time.
4. **Does real-time live face detection (KAI-282 live-view feature) cross into Art. 5(1)(h) prohibited territory?** — Our current design: real-time live detection is available ONLY against a customer-managed vault on customer-owned cameras on customer-owned premises. This is NOT "real-time remote biometric identification in publicly accessible spaces for law enforcement" which is what Art. 5(1)(h) prohibits. But counsel must confirm the "publicly accessible spaces" definition does not catch customer retail floors, schools, or workplaces — because some of those are exactly our customer profile. If any of those ARE captured, we ship with live detection disabled by default in the EU and enable only post-hoc search.
