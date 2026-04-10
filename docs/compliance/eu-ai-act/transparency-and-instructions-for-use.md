---
title: Transparency and Instructions for Use — Face Recognition
owner: lead-security (process) | lead-ai (model facts)
status: draft
last-updated: 2026-04-08
eu-ai-act-article: 13
---

# Transparency and Instructions for Use

Article 13 requires that high-risk AI systems be "designed and developed in
such a way to ensure that their operation is sufficiently transparent to
enable deployers to interpret the system's output and use it appropriately."
Instructions for use must accompany the system. This document is the source
of truth for what ships to customers.

## Audience

Two distinct audiences:

1. **Deployers (customers)** — tenant admins, security managers, DPOs
2. **End operators** — guards, receptionists, SOC analysts using the live
   dashboard

The customer-facing PDF and the in-product help are both generated from this
document plus the model facts in `data-governance.md`.

## 1. System identity

- **Provider:** Kaivue / Ruflo Labs (legal entity TBD — TODO lead-security)
- **Product:** Kaivue Recording Server — Face Recognition Module
- **Version:** TODO(lead-ai) semver + model version hash
- **Intended purpose:** 1:N identification of enrolled persons in controlled
  environments. Outputs a candidate match with a confidence score; an
  operator confirms or rejects the match.
- **CE marked under:** EU AI Act 2024/1689, conformity procedure per
  Article 43 (route TBD — see README)

## 2. What the system does

Plain language: the camera sees a face, the system compares it to a list of
enrolled faces the customer has loaded, and if one matches closely enough
the operator sees a pop-up with the candidate's name, a thumbnail of the
live frame, a thumbnail of the enrolment photo, and a confidence score. The
operator decides what to do.

The system does **not** decide autonomously. No doors open without operator
confirmation unless the customer has explicitly enabled an access-control
automation with a documented risk acceptance.

## 3. What the system does NOT do

- It does **not** perform emotion recognition, aggression detection, or any
  inference about character or intent. (Such uses are prohibited under AI Act
  Art. 5 in workplaces and educational settings.)
- It does **not** categorize persons by race, political view, sexual
  orientation, religion, or trade union membership. (Prohibited under Art. 5.)
- It does **not** perform real-time remote biometric identification in publicly
  accessible spaces for law enforcement, which is separately restricted under
  Art. 5(1)(h).
- It does **not** build behavioral profiles.

## 4. Prohibited uses

Customers MUST NOT use the face recognition module for:

- Law enforcement watchlist matching without a specific legal basis and a
  court order where required.
- Public mass surveillance of non-enrolled persons.
- Workplace monitoring without: (a) a lawful basis per the Member State's
  worker-protection law; (b) works-council consultation where applicable; and
  (c) a clearly articulated, proportionate purpose.
- School monitoring without parental consent and a DPA consultation.
- Automated decisions with legal or similarly significant effect on a data
  subject without human review, per GDPR Art. 22.
- Scraping face images from the internet to enroll unknown persons.

Violation may result in tenant suspension per Kaivue's Acceptable Use Policy.

## 5. Known limitations

The system performance degrades, sometimes sharply, under the following
conditions:

- Low light below the configured minimum illumination
- Extreme head pose (> 30 degrees from frontal)
- Occlusion (masks, scarves, large sunglasses, helmets)
- Age gap > 5 years between the enrolment photo and the current appearance
- Heavy motion blur
- Faces at large distance — minimum face pixel width per Section 7
- Underrepresented phenotypes in the training data — see the fairness report

Customers must ensure camera placement meets the minimum conditions.

## 6. Accuracy metrics

**TODO (lead-ai):** complete the following table with the evaluation from the
release-candidate model. The numbers published here must match the Trust
Center (KAI-394) publication.

| Metric              | Aggregate | Fitz 1-2 | Fitz 3-4 | Fitz 5-6 | 18-25 | 26-40 | 41-60 | 61+ |
| ------------------- | --------- | -------- | -------- | -------- | ----- | ----- | ----- | --- |
| FPR @ op threshold  |           |          |          |          |       |       |       |     |
| FNR @ op threshold  |           |          |          |          |       |       |       |     |
| TAR @ FPR=1e-4      |           |          |          |          |       |       |       |     |
| Max disparity ratio |           |          |          |          |       |       |       |     |

Source dataset, size, collection date, evaluation code commit hash — all
TODO(lead-ai).

## 7. Operating envelope

- Minimum face pixel width: 80 px
- Minimum illumination: TODO(lead-ai) lux or equivalent
- Maximum pose deviation: TODO(lead-ai)
- Supported camera resolutions: TODO(lead-ai)
- Concurrent stream limit per Recorder: TODO(lead-ai)

## 8. Operator training requirements

Customers must ensure each operator completes before first use and annually
thereafter:

- Minimum 4-hour training covering: what the system can and cannot do;
  demographic bias limitations; legal context in their jurisdiction; consent
  and posted-notice obligations; how to log a disputed match; how to correct
  a mislabeled enrollment.
- A written test covering bias awareness, limitations, and legal context.
  Passing threshold: 80%.
- Annual refresher, same content.
- Sign-off recorded in the Admin Console. Operators without a current sign-off
  are locked out of the face recognition review queue.

Training materials provided by Kaivue as PDF + short video.

## 9. Maintenance obligations (customer)

- Keep model updates current; critical fairness fixes ship as mandatory
  updates the customer cannot defer more than 30 days.
- Review audit log samples quarterly.
- Re-enroll persons whose enrolment photo is > 5 years old.
- Update enrolment data promptly when persons leave the organization.
- Configure retention and honor withdrawal requests within 30 days.

## 10. Customer rights and channels

- In-app "Report a false positive / false negative" button on every match
  review screen.
- Support email for compliance questions.
- Trust Center for published fairness metrics and model changelogs.

## 11. Changelog

Every shipped model version will be listed here with: version hash, release
date, training data snapshot, fairness report link, changes since previous.

- TODO(lead-ai): populate on first release.

## TODOs

- [ ] lead-ai — fill in accuracy table and operating envelope
- [ ] legal — sign off on prohibited-uses section
- [ ] lead-security — finalize Kaivue legal entity + EU representative
- [ ] design — produce the PDF layout and in-product help surfaces
