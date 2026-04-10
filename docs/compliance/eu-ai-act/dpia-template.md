---
title: DPIA Template — Customer Pre-Enablement (Face Recognition)
owner: lead-security (template) | customer DPO (completion)
status: draft
last-updated: 2026-04-08
eu-ai-act-article: 26, 27
---

# Data Protection Impact Assessment — Face Recognition

**This template MUST be completed by the customer before face recognition is
enabled on any Kaivue tenant.** Face recognition involves systematic
processing of biometric data and requires a DPIA under GDPR Article 35(3)(b)
and an AI-Act-Article-27 fundamental rights impact assessment for deployers
of high-risk AI systems.

Completed DPIA is retained by the customer **and** attached to the tenant's
compliance record in the Kaivue Admin Console. Kaivue does not review the
content; Kaivue records that it was completed and signed.

---

## 1. Controller details

- Organization name:
- Legal entity type:
- Primary establishment (country):
- Data Protection Officer (name, email, phone):
- Date of this assessment:
- Assessment version:

## 2. Description of the processing

### 2.1 Purpose

Describe, in plain language, **why** face recognition is being enabled.
Examples: access control for restricted areas; safety — identification of
persons on a pre-approved restraining order list who have attempted to enter
the premises; loss prevention in retail under posted notice; automated visitor
check-in.

_Your answer:_

### 2.2 Nature of the processing

- [ ] Enrollment of known persons (employees / known visitors)
- [ ] Matching against an internal watchlist
- [ ] Real-time alerts on match
- [ ] Logging-only mode (no operator alert, retrospective investigation only)
- [ ] Integration with access control hardware (specify which system)
- [ ] Integration with alarm panel / SOC escalation

### 2.3 Scope

- Camera locations (list or map reference):
- Hours of operation:
- Estimated daily unique faces encountered:
- Estimated enrolled population:

### 2.4 Context

- Physical environment (office / retail / industrial / school / public):
- Are minors present? If yes, what safeguards apply?
- Are vulnerable persons present?
- Are workers monitored? If yes, confirm Member-State worker-protection law
  compliance and works-council consultation status.

## 3. Necessity and proportionality

Answer each question and justify.

- Why can the purpose not be achieved by a **less intrusive** means (e.g.
  RFID badge, PIN, manual check-in)?
- Is the system limited to the minimum cameras, minimum enrolled population,
  minimum retention necessary?
- Are non-enrolled persons excluded from storage?
- Is there a less-biometric fallback for persons who object?

## 4. Data subjects and data categories

- Categories of data subjects (employees, visitors, customers, general public):
- Biometric data types (face template / embedding; image thumbnails; quality
  metadata):
- Linked identifiers (name, employee ID, role, permissions):

## 5. Lawful basis

Select and justify:

- [ ] GDPR Art. 9(2)(a) — Explicit consent (preferred for enrolled persons)
- [ ] GDPR Art. 9(2)(b) — Employment / social security law (cite statute)
- [ ] GDPR Art. 9(2)(g) — Substantial public interest (cite Member State law)
- [ ] Other (explain)

Attach: consent form template, signage used, works-council agreement if
applicable.

## 6. Retention schedule

- Enrollment record retention:
- Face template retention after employment termination:
- Audit log retention (system default 7 years, can be longer):
- Non-enrolled incidental face capture retention: 0 (Kaivue default)
- Withdrawal-of-consent processing time:

## 7. Security measures

Kaivue provides:

- Per-tenant encryption at rest (references KAI-141)
- Tenant isolation (references KAI-235)
- TLS in transit
- Role-based access via Casbin (references KAI-225)
- Audit logging (references KAI-233)
- Four-eyes watchlist edits
- Operator confirmation on every match

Customer must add:

- Staff training (document completion)
- Access restriction to the Face Vault Management page
- Physical security of viewing stations
- Incident response plan linking to Kaivue support

## 8. Risk assessment to rights and freedoms

For each of the following, assess likelihood and severity (Low/Med/High),
mitigation, and residual risk:

| Risk                                | L   | S   | Mitigation | Residual |
| ----------------------------------- | --- | --- | ---------- | -------- |
| Wrongful identification             |     |     |            |          |
| Demographic bias harm               |     |     |            |          |
| Unlawful surveillance creep         |     |     |            |          |
| Data breach of face vault           |     |     |            |          |
| Function creep (purpose drift)      |     |     |            |          |
| Chilling effect on workers/visitors |     |     |            |          |

## 9. Consultation

- [ ] DPO consulted (date):
- [ ] Works council / employee representatives consulted (date):
- [ ] Data subjects or their representatives consulted (date / method):
- [ ] DPA prior consultation under GDPR Art. 36 required? (High residual risk
      requires this.) If yes, attach correspondence.

## 10. Sign-off

- Controller authorized signatory:
- DPO:
- Date:
- Review date (max 12 months from signing):

---

**TODO (lead-security):** translate this template into DE, FR, ES, IT, NL.
**TODO (lead-security):** publish a redacted example DPIA in the Trust Center
(KAI-394) as customer guidance.
**TODO (legal):** Member-State annexes for DE worker codetermination, FR CNIL
public-space rules, IT Garante guidance.
