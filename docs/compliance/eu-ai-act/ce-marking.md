# CE Marking + EU Declaration of Conformity

**Article:** 48 + 47
**Status:** Draft — awaiting legal-entity selection and counsel sign-off
**Owner:** lead-security + business

## Purpose

Art. 48 requires providers to affix a CE marking to high-risk AI systems to indicate conformity with the AI Act. Art. 47 requires a written EU declaration of conformity. This document holds the CE marking procedure and the declaration template.

## CE marking procedure

### Eligibility

A Kaivue release may bear the CE marking only when ALL of the following are true:

1. Conformity assessment per `conformity-assessment.md` is complete (Annex VI self-assessment, unless counsel directs the notified-body route).
2. Technical documentation per `annex-iv-technical-documentation.md` is complete, including the per-model body for the deployed face-recognition model.
3. The EU declaration of conformity (template below) has been signed by a person authorised to bind the provider.
4. EU database registration per `eu-database-registration.md` is complete.
5. The release has passed the accuracy, robustness, and cybersecurity gates (`accuracy-robustness-cybersecurity.md`).
6. The release has passed the fairness gates (`fairness-testing-protocol.md`).
7. The post-market monitoring plan is active for the release (`post-market-monitoring.md`).
8. All six harmonised standards referenced in `conformity-assessment.md` are applied or a documented alternative is in place.
9. The risk management loop has been run for the release (`risk-management-system.md`).

### Affixing

The CE marking takes the graphical form specified in Regulation (EC) No 765/2008 Annex II. It is affixed:

- **Digitally** in the "About" section of the customer admin console (KAI-327), displayed with the version, the name of the provider, and a link to the declaration of conformity and the instructions for use.
- **In documentation** on the product packaging (for on-prem Recorder appliance releases), in the release notes, and on the trust portal.
- **In the EU database** alongside the registration record.

Where the marking is affixed, a unique identifier resolvable to the exact version of the system is included so that the declaration of conformity and the technical documentation can be retrieved without ambiguity.

### Release gate

A release is ineligible for CE marking until the pre-release checklist in this document is complete. The release pipeline (KAI-428) enforces the gate: a release cannot be tagged with the `ce` marker unless a release-engineer signs off after verifying each item in the checklist.

## EU declaration of conformity template

### Template

The declaration MUST include all items in Annex V of the AI Act. Kaivue's template:

---

**EU Declaration of Conformity**

**Issued under sole responsibility of:**

Kaivue [legal entity TBD]
[Registered address]
[Contact email]

**Authorised representative (where applicable):**

[Name and address of authorised representative in a Member State, per Art. 22, if Kaivue is not established in the Union]

**This declaration applies to:**

Kaivue Face Recognition — model `<model-id>`, deployed within Kaivue Recorder release `<release-version>`.

**Unique identifier of the AI system:** `<release-version + model-id + sha256 of technical documentation package>`.

**Object of the declaration:**

Kaivue Face Recognition is a high-risk AI system under Annex III §1(a) of Regulation (EU) 2024/1689 (AI Act), performing biometric identification of natural persons against a customer-managed face vault in post-hoc or opt-in-per-camera configurations.

**The object of the declaration described above is in conformity with:**

- Regulation (EU) 2024/1689 (AI Act)
- Regulation (EU) 2016/679 (GDPR) to the extent that processing of personal data is governed thereby
- The following harmonised standards:
  - EN ISO/IEC 23894
  - EN ISO/IEC 42001
  - EN ISO/IEC 25059
  - EN ISO/IEC 24027
  - EN ISO/IEC 5338
  - EN ISO/IEC 5469

**Conformity assessment procedure applied:** Annex VI (internal control), unless stated otherwise in this specific release.

**Name and identification number of the notified body (where applicable):** Not applicable / `<notified-body-name-and-number>` (as determined by the conformity assessment path taken for this release).

**Reference to the technical documentation:**

The technical documentation for this declaration is located in `docs/compliance/eu-ai-act/` of the source repository for the release, and is retained per Art. 18 for 10 years from the last placing-on-market of this system. A content-addressed copy is archived at `<archive-URI>`.

**Signature:**

Signed for and on behalf of Kaivue:

- Place of issue: `<city>`
- Date of issue: `<YYYY-MM-DD>`
- Name: `<authorised signatory>`
- Function: `<role>`
- Signature: `<signature>`

---

### Storage

Signed declarations are retained in `docs/compliance/eu-ai-act/declarations/<release-version>.pdf` for the full 10-year period and uploaded to the EU database per `eu-database-registration.md`.

### Translation

Per Art. 47(2), the declaration must be translated into the language(s) required by the Member State(s) where the system is placed on the market. The template is maintained in English; translated versions are produced per release in consultation with the authorised representative.

## Interactions with other documents

- `conformity-assessment.md` — the upstream artefact that determines whether a release is eligible for CE marking.
- `annex-iv-technical-documentation.md` — referenced by the declaration.
- `eu-database-registration.md` — registration is a precondition for placing on the market.
- `provider-obligations.md` — CE marking is a specific Art. 16 obligation.
