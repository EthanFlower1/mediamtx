---
title: Data Governance — Face Recognition
owner: lead-ai (dataset facts) | lead-security (process)
status: draft
last-updated: 2026-04-08
eu-ai-act-article: 10
---

# Data Governance — Face Recognition

Article 10 requires training, validation, and test datasets for high-risk AI
systems to be **relevant, sufficiently representative, free of errors to the
best extent possible, and complete in view of the intended purpose**. This
document describes how Kaivue meets those requirements for face recognition.

## Intended purpose recap

1:N identification of **enrolled** persons (employees, visitors, authorized
watchlist entries) in **controlled environments** (offices, industrial sites,
schools operating under parental consent, retail loss-prevention with posted
notices). Not intended for mass surveillance or law-enforcement watchlist
matching against the general public.

## Training / validation / test datasets

**TODO (lead-ai):** fill in concrete values for each sub-field below. None of
this can be left blank in the Annex IV technical file.

### Provenance

- [ ] Dataset 1 name, source URL / vendor, license, commercial use allowed?
- [ ] Dataset 2 ...
- [ ] Dataset 3 ...
- [ ] Any scraped data? If yes, what was the legal basis? (Likely non-compliant
      for EU — must flag with legal.)
- [ ] Synthetic augmentation used? Tool, seed, volume, validation.

### Consent basis for each dataset

- [ ] Explicit consent signed per subject — describe mechanism and retention
      of consent records.
- [ ] Public-interest / research exemption claimed — cite legal basis.
- [ ] Licensed from a vendor that warrants consent — store vendor warranty
      text and contract reference.

### Size and composition

- [ ] Total identities, total images
- [ ] Images per identity distribution (mean, median, p5, p95)
- [ ] Collection date range (relevant for drift baseline)
- [ ] Geographic diversity — continents / countries represented
- [ ] Pose range (frontal, profile, angle buckets)
- [ ] Lighting conditions (indoor, outdoor, low-light, backlit)
- [ ] Capture devices (consumer, surveillance, mixed)

### Demographic composition

Fitzpatrick skin tone buckets (1–6), age buckets, gender categories, plus an
"unknown / unlabeled" bucket. A data governance failure if any single cell
is below the minimum power-analysis threshold for bias testing.

- [ ] TODO(lead-ai): fill the 6 x 5 x N contingency table with counts.
- [ ] TODO(lead-ai): document the labeling protocol — who labeled, training
      given to labelers, inter-annotator agreement.

### Error correction

- **Mislabel discovery:** operator-reported mismatch or internal QA.
- **Removal process:** identity row marked tombstoned; embeddings purged;
  training run manifest updated; hash pinned.
- **Retraining trigger:** accumulated label corrections > 0.5% of the training
  set, OR a class-conditional error rate anomaly, OR quarterly schedule.
- **Documentation:** every correction batch gets a changelog entry in the
  model registry (KAI-279).

## Bias testing protocol

Detailed methodology lives in `fairness-testing-protocol.md`. Data governance
ensures the **test set itself** is:

- stratified by Fitzpatrick skin tone (6 buckets)
- stratified by age bucket (<18 excluded unless parental consent, 18–25,
  26–40, 41–60, 61+)
- stratified by gender (as labeled, with "unknown" bucket)
- stratified by glasses / no glasses
- stratified by mask / no mask
- minimum N per cell documented and justified by power analysis

**TODO (lead-ai):** confirm the test set is **disjoint** from training and
validation at the **identity** level, not just the image level.

## Data quality criteria (capture-time)

Applied both at enrollment and at inference:

- Minimum face pixel width: 80 px (configurable per deployment)
- Minimum brightness / maximum blur: measured via Laplacian variance
- Maximum pose deviation from frontal: documented per model
- Quality score threshold: images below threshold rejected with a user-visible
  reason; no silent fallback to a lower-quality path
- Eye open, mouth closed: optional, configurable
- No multiple faces within the enrollment crop

## GDPR lawful basis

Face data is **special category personal data** under GDPR Article 9.
Processing is only lawful under one of the Article 9(2) exceptions.

- **Enrollees (employees, known visitors):** Article 9(2)(a) **explicit
  consent**, captured per `opt-in-consent-flow.md`. Consent must be freely
  given, specific, informed, unambiguous, and withdrawable.
- **Public-area detection (non-enrolled persons who happen to walk past):**
  the legitimate-interest argument is **weak** and likely **not compliant**
  without: (a) explicit posted signage, (b) a real opt-out mechanism, and
  (c) strict purpose limitation. DPA guidance (EDPB Guidelines 3/2019 and
  successor guidance) is hostile to untargeted biometric processing in public
  spaces. **Default product behavior:** non-enrolled faces must not be stored,
  not even as a template, beyond the inference-time evaluation window.
- **TODO (legal):** confirm position with DPAs in DE, FR, NL, IT, ES.

## Retention

- **Training data:** retained only as long as necessary to train and to
  reproduce a shipped model, plus the regulatory record-retention period for
  Annex IV technical documentation (10 years per Article 18). Data held beyond
  retraining is held in cold, encrypted storage with access logging.
- **Deployed face vault (customer tenant):** retention per customer policy,
  with a **hard cap** configurable by tenant admin, default 365 days from
  last enrollment update. Tombstoned entries purged after 30 days.
- **Audit logs referencing face inference:** 7 years (see `audit-log-requirements.md`).
- **Withdrawn consent:** enrolment record and embeddings purged within 30 days
  of withdrawal; audit log entry retained (contains no biometric data).

## Cross-border transfer

- **TODO (lead-security):** document Standard Contractual Clauses, Transfer
  Impact Assessment, and data residency options for EU customers.
- Default: EU customer data stays in EU regions. Training data movement
  documented separately.

## Documentation artifacts produced

- Dataset data sheet (Gebru et al. "Datasheets for Datasets")
- Model card (Mitchell et al.)
- Training run record (hyperparameters, seed, code commit, dataset hash)
- Bias test report (see `fairness-testing-protocol.md`)
- Labeling protocol document

## TODOs

- [ ] lead-ai — fill in every `TODO(lead-ai)` above
- [ ] DPO — sign off on lawful basis section
- [ ] legal — Member-State-specific DPA guidance review
- [ ] lead-security — integrate retention schedule with KAI-233 audit log
      retention policy
