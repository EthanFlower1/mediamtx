# Transparency and Provision of Information to Deployers

**Article:** 13
**Status:** Draft — customer-facing text pending marketing review
**Owner:** lead-ai + lead-product

## Purpose

Art. 13 requires that high-risk AI systems are designed to ensure their operation is sufficiently transparent to enable deployers to interpret the system's output and use it appropriately. It also requires specific items in the instructions for use.

## Delivery channels

Transparency information is delivered to deployers through three coordinated channels:

1. **The customer admin console (KAI-327 AI Settings).** In-product, always current with the deployed system.
2. **The instructions for use document** bundled with every release and linked from the admin console.
3. **The published Kaivue trust portal** (external URL — TBD by marketing), which carries the public-facing version.

Information in all three channels is generated from the same source (this document plus the per-model bodies of `annex-iv-technical-documentation.md`) so it cannot drift.

## Required content (Art. 13(3))

### (a) Identity and contact details of the provider

- Provider: Kaivue (TBD legal entity) — to be inserted after legal-entity selection.
- Address, email, and a designated compliance contact are published on the trust portal.
- For each Member State where Kaivue places the system on the market, an authorised representative is identified per Art. 22.

### (b) Characteristics, capabilities, and limitations of performance

Delivered per-model. Every per-model body must include a deployer-facing summary under 500 words covering:

- Intended purpose (face identification against a customer-managed vault).
- Declared accuracy at the operating threshold, with demographic breakdowns.
- Known limitations (low-resolution, occlusion, extreme lighting, age-band caveats).
- Demographic parity metrics.
- Cases where the system is known to fail and should not be relied on.

### (c) Changes to the system and its performance

- Model version changes are surfaced in the admin console with a changelog entry.
- Material changes that affect how the system should be used are surfaced with an acknowledgement requirement — customer admins must see and click through before the new version becomes active.

### (d) Human oversight measures

- See `human-oversight.md` — summarised for customers in the admin UI.
- Training material for reviewers is linked.
- The stop controls (per-camera and per-tenant disable) are described alongside the "how to use" section.

### (e) Expected lifetime and maintenance measures

- The deployed model has an expected service life which is declared per-model.
- Security patching, model updates, and end-of-life timelines are published on the trust portal.

### (f) Description of mechanisms included in the system for collecting, storing, and interpreting logs

- Audit log (KAI-233) is described with retention policy (10 years, see `conformity-assessment.md`).
- Customer access to audit log is via a documented API and the admin console.
- Log fields are enumerated so that customers can build their own compliance analyses.

## Intended purpose boundary

The intended purpose is **post-hoc and opt-in-per-camera face identification against a customer-managed face vault on customer-owned cameras on customer-controlled premises**.

The following are **out of scope** and customers are informed they must not use the system for them:

- Real-time remote biometric identification in publicly accessible spaces for law enforcement (Art. 5(1)(h)).
- Biometric categorisation inferring sensitive attributes (Art. 5(1)(g)).
- Emotion recognition in workplaces or education (Art. 5(1)(f)).
- Social scoring (Art. 5(1)(c)).
- Predictive policing (Art. 5(1)(d)).

The customer agreement prohibits these uses contractually. The product refuses to configure them (enforced in product code under `provider-obligations.md`).

## Deployer obligations summarised for customers

Art. 26 places obligations on deployers. Kaivue's customer-facing transparency materials include a plain-language summary of deployer obligations so customers understand what they are committing to:

- Use the system in accordance with the instructions for use.
- Assign human oversight to competent natural persons.
- Monitor the system and report serious incidents to Kaivue.
- Retain automatically-generated logs for the period required under Art. 19.
- Where applicable, conduct a fundamental-rights impact assessment (`fundamental-rights-impact-assessment.md`).
- Inform affected natural persons in accordance with Art. 26(11).

## Changes and versioning

This document is versioned with the release. Every deployed release carries a pointer to the exact version of the transparency information in force at the time. Customers can always retrieve historical versions from the trust portal and from the release archive.

## Interactions with other documents

- `annex-iv-technical-documentation.md` — per-model body is the source of the per-model transparency content.
- `human-oversight.md` — described to deployers here.
- `provider-obligations.md` — product-level refusals enforcing the intended-purpose boundary.
- `fundamental-rights-impact-assessment.md` — template provided to deployers.
