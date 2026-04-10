---
title: Opt-In and Consent Flow — Face Recognition
owner: lead-security (process) | lead-ai (product UX)
status: draft
last-updated: 2026-04-08
eu-ai-act-article: 13, 26
---

# Opt-In and Consent Flow

Face recognition requires layered consent: (a) the **customer** (tenant)
opts the feature in; (b) each **enrollee** individually consents; (c)
**bystanders** are given notice via signage. This document is the binding
UX spec for the opt-in flow that must be implemented in Kaivue's Customer
Admin Console (KAI-327) and Recorder UI.

No implementation exists as of 2026-04-08. This spec precedes implementation.

## Layer 1 — Tenant admin opt-in

### Default state

- Face recognition is **OFF by default** for every new tenant.
- The feature is not advertised in the main navigation until opted in,
  except via an entry under Settings > AI Features labeled **Face Recognition
  (disabled)**.

### Opt-in sequence

1. Tenant admin navigates to **Settings > AI Features > Face Recognition**.
2. The page displays:
   - Plain-language description of what the feature does
   - Link to the published model fairness report
   - Link to the Transparency / Instructions for Use document
   - Link to the customer DPIA template
   - A required checklist:
     - [ ] I have read the Instructions for Use
     - [ ] I have completed a DPIA
     - [ ] I have a lawful basis for processing biometric data
     - [ ] I will post the required signage
     - [ ] My operators have completed training
3. A free-text **purpose** field (minimum 100 characters) — what the tenant
   is using it for. This is stored and reviewable on audit.
4. A **four-eyes** confirmation: a second admin must approve the opt-in.
5. On activation, every existing tenant admin is emailed a confirmation.
6. The activation event is written to the audit log.

### Deactivation

- One click from the same page. Effective within 10 seconds.
- Does not automatically purge the face vault; a separate **Purge face vault**
  action is required and is four-eyes.

### Changes after activation

- Four-eyes required to: raise retention, lower confidence threshold below
  a floor, add new sites, enable an access-control integration.

## Layer 2 — Per-enrollee consent

### Who can be enrolled

- Employees with a currently valid employment relationship, operating under
  a lawful basis (consent or employment-law basis depending on Member State)
- Contractors with a signed data-processing acknowledgement
- Visitors who present at reception and complete the visitor consent flow
- Watchlist entries under a specific legal basis (restraining order,
  trespass ban) — each documented
- **Never** enrolled without one of the above

### Consent capture UX

Two paths:

**A. Self-service capture (e.g. HR onboarding)**

1. Person is directed to a tablet or web form in a private setting.
2. They read a plain-language consent text (multi-language required):
   - What is collected (face image + derived template)
   - Purpose (explicit: e.g. building access, visitor check-in)
   - Who will see it (their employer's security staff; Kaivue as processor)
   - Retention period (explicit)
   - Withdrawal: one-click, no questions asked, how to do it
   - Contact for the tenant's DPO
   - Link to the full policy
3. They check a consent box, enter their name, and sign (drawn signature
   or typed name with IP + timestamp capture).
4. A photo is captured on the device or uploaded.
5. A receipt is issued (printable / emailed) including a reference id the
   enrollee can use to withdraw later.
6. The consent artifact is stored alongside the enrollment record and
   referenced by `consent_record_ref` in every enrollment event.

**B. Operator-assisted capture (visitors, supervised)**

- Same content, operator walks the visitor through it, signs attestation.
- Still captures a signed record.

### Minors

- No enrollment of under-18s without parental/guardian consent AND a
  separate lawful basis.
- The product must hard-block enrollment of a face estimated to be under 18
  (by a conservative age estimator) unless a minor-specific consent
  workflow has been completed with attached parental consent artifact.
- Under-13 enrollment is blocked without Member-State-specific parental
  consent + DPA review. **TODO (legal):** confirm per Member State.

### Withdrawal

- Enrollees can withdraw via: tenant admin portal, email to tenant DPO, or
  QR code printed on the consent receipt that leads to a self-service page.
- Withdrawal is one click. No justification required.
- Processing SLA: within 30 seconds from the matcher, within 30 days from
  backups.
- Tenant admin receives a notification to update HR systems.
- Withdrawal is audit-logged; the audit entry contains no biometric data.

## Layer 3 — Public / bystander signage

### Signage requirement

Customers MUST post conspicuous signage at every entrance to a space
monitored by face recognition and at any room containing a camera feed used
for face recognition.

Kaivue provides a downloadable template in the supported Member-State
languages. Minimum content:

> **Video surveillance with automated facial recognition in operation.**
>
> Purpose: [customer-entered]
> Controller: [customer legal name]
> Retention: [customer-entered]
> Contact for questions or to exercise your rights: [DPO email + phone]
> Legal basis: [customer-entered]
> More information: [URL]

Template is downloaded from the Admin Console after opt-in. The Admin
Console records which templates were downloaded and on what date
(evidence that signage was produced).

### Opt-out for bystanders

Kaivue's default behavior is **not** to store face data for non-enrolled
persons, so a bystander's face is processed transiently for matching and
discarded. Bystanders retain the right to request confirmation of
processing and to object under GDPR.

## Integration with existing Kaivue surfaces

- Customer Admin AI Settings + face vault management: KAI-327
- Audit log service: KAI-233
- Trust Center (public fairness reports, templates): KAI-394
- Tenant master key for encrypted enrollment data: KAI-141

## Test plan (to be referenced by implementation tickets)

- Default-off test — new tenant cannot invoke inference APIs
- Four-eyes tests — single operator cannot activate, cannot add watchlist
- Consent receipt present for every active enrollment (invariant check)
- Withdrawal SLA test — measure end-to-end time
- Minor-block test — under-18 estimator triggers hard stop
- Signage template download is logged

## TODOs

- [ ] design — produce mocks for admin opt-in, enrollee capture, withdrawal
- [ ] legal — localized consent and signage text for DE, FR, IT, ES, NL
- [ ] lead-security — integrate consent artifact storage with per-tenant
      encryption (KAI-141)
- [ ] lead-ai — age-estimator choice + false-positive minimum for the
      under-18 hard block
