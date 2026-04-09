# EU Database Registration

**Article:** 71 + 49
**Status:** Draft — actual registration blocked on EU database becoming operational
**Owner:** lead-security + business

## Purpose

Art. 49(1) requires providers to register a high-risk AI system in the EU database referred to in Art. 71 before placing it on the market or putting it into service. This document defines Kaivue's registration procedure.

## Registration obligations

Before placing Kaivue Face Recognition on the market in the Union, the provider (Kaivue) must register the system in the Art. 71 EU database. Registration requires:

- Provider name, address, and contact details.
- Authorised representative details (where Kaivue is not established in the Union).
- Trade name of the system and any additional unambiguous identifiers.
- Intended purpose description.
- Status of the conformity assessment (e.g. Annex VI internal control completed).
- Electronic declaration of conformity (or link to where it is stored).
- Short summary of the instructions for use.
- URL of additional information where available.
- Member State(s) in which the system is placed on the market.

## Procedure

1. **Prerequisite checks.** All items in the `ce-marking.md` release gate are complete.
2. **Drafting.** The registration payload is drafted from the same sources that produce the declaration of conformity. The `annex-iv-technical-documentation.md` per-model body provides the intended-purpose description; `transparency-and-information.md` provides the short summary of the instructions for use.
3. **Review.** lead-security and business review the registration payload. Legal counsel review is mandatory for the first registration.
4. **Submission.** The payload is submitted to the EU database at the URL published by the Commission. Submission is made under the Kaivue provider account.
5. **Record.** The database registration ID is recorded alongside the declaration of conformity in `docs/compliance/eu-ai-act/declarations/<release-version>.pdf`.
6. **Publication.** The CE-marked release is placed on the market only after the registration ID is issued.

## Updates

Per Art. 49(3), registrations are kept up to date:

- Material changes to the system (as defined in `risk-management-system.md`) trigger a registration update.
- Provider contact changes trigger an update.
- Corrections to the intended purpose or instructions for use trigger an update.
- Annual review of the registration for accuracy.

Historical registration versions are retained with the 10-year compliance package.

## Non-public information

Some information in the registration may be marked non-public per Art. 71(3). Kaivue's default stance is that all required fields are public except where disclosure would (a) compromise cybersecurity (model internals, security architecture beyond what is disclosed in `accuracy-robustness-cybersecurity.md`) or (b) include commercially sensitive information (pricing, customer lists). Any non-public marking is documented with a reason.

## EU database operational status

As of 2026-04-08 the Art. 71 database is in the process of being brought into operation by the Commission. Kaivue's pre-launch readiness:

- [ ] Monitor Commission communications for the database URL and onboarding process.
- [ ] Identify the authorised representative and provision their access.
- [ ] Prepare the registration payload in draft form so it can be submitted on day one.
- [ ] Confirm that the EU database registration is a blocking precondition for the first EU-region release in the release pipeline (KAI-428).

## Interactions with other documents

- `ce-marking.md` — registration is a precondition for placing on the market and is a release-gate item.
- `conformity-assessment.md` — the source of the conformity assessment status field.
- `annex-iv-technical-documentation.md` — the source of the intended-purpose description.
- `transparency-and-information.md` — the source of the instructions-for-use summary.
