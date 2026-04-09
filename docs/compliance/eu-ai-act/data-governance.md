# Data Governance

**Article:** 10
**Status:** Draft — per-model bodies filled as KAI-282 lands
**Owner:** lead-ai + lead-security

## Purpose

Art. 10 requires that training, validation, and testing datasets meet quality criteria relevant to the intended purpose. Biometric data is Art. 9 GDPR special-category data, so processing it is additionally governed by Art. 10(5) of the AI Act, which permits processing of special categories *to the strict extent necessary for bias detection and correction* under enumerated safeguards.

This document defines the governance regime for every dataset touched by a Kaivue face-recognition model.

## Categories of data

| Category | Examples | Controller | Governance layer |
|---|---|---|---|
| Training data | Pre-training datasets, fine-tuning datasets | Kaivue (provider) | This document + per-model body |
| Validation data | Held-out benchmarks for accuracy / fairness | Kaivue | This document + `fairness-testing-protocol.md` |
| Customer enrolment data | Face-vault identities provided by the deployer | Customer (deployer) | Customer under GDPR; Kaivue processor |
| Customer inference data | Live video frames processed at inference time | Customer (deployer) | Customer under GDPR; Kaivue processor |
| Operator feedback data | False-positive / false-negative tags | Joint | Anonymised before ingestion into retraining |

Kaivue is the **provider** for categories 1–2 and a **processor** for categories 3–5.

## Training and validation data (provider-controlled)

### Sourcing

Per-model bodies MUST record for every dataset:

- Source (URL, vendor, DOI where applicable).
- Licence (SPDX identifier or full text).
- Access date.
- Whether subjects gave consent (and under what regime).
- Whether the dataset is redistributable.
- Whether the dataset contains known demographic skew (document the skew, do not hide it).

Datasets whose provenance cannot be established to this standard are NOT eligible for Kaivue training or validation. No exceptions.

### Preparation

Per-model bodies document:

- Cleaning procedure (deduplication, quality thresholds, face-detection filtering).
- Augmentation (horizontal flip, lighting, pose jitter — pinned in the preprocessing script).
- Labelling procedure (manual, semi-automatic, crowd-sourced — if crowdsourced, label-noise estimate).
- Train/validation/test split, split seed, and the script that produced the split.

### Examination for bias (Art. 10(2)(f))

Every training and validation dataset is examined for possible biases before use:

1. Demographic coverage analysis (age bands, apparent gender, Fitzpatrick skin type, or equivalent — the specific taxonomy is documented per-model).
2. Geographic coverage.
3. Capture conditions (indoor/outdoor, lighting, camera quality).
4. Known collection biases (e.g. web-scraped data skews towards celebrities and people in developed economies).

Findings and mitigations feed `fairness-testing-protocol.md`.

### Art. 10(5) processing of special categories

Kaivue processes biometric data (facial images) *for bias detection and correction*. Art. 10(5) permits this only where:

1. Bias detection and correction cannot effectively be fulfilled by processing other data (including synthetic or anonymised data). Kaivue's position: synthetic faces today do not capture the demographic distribution needed for real-world bias measurement, so real biometric data is strictly necessary for validation/fairness testing. This position is reviewed annually.
2. Sensitive categories are subject to technical limitations on reuse and state-of-the-art security measures.
3. Data is not transmitted or accessed by other parties.
4. The processing is logged and recorded.
5. Personal data is deleted once the bias has been corrected or the retention period ends.

Kaivue implements these as:

1. **Necessity justification:** documented annually by lead-ai; current position above.
2. **Technical limitations:** biometric data used for bias testing lives in a hardened VPC-private bucket with per-use access grants via short-lived tokens. Only the model-evaluation pipeline can read it. No human access without a documented exception request signed by lead-security.
3. **Security measures:** AES-256 at rest, TLS 1.3 in transit, KMS-managed keys with annual rotation.
4. **No third-party access:** sensitive datasets are never moved outside the Kaivue AWS organisation. Contractor access requires a DPA and is logged.
5. **Logging:** every read of a bias-testing dataset is logged to the Kaivue audit log with user, time, purpose.
6. **Deletion:** dataset retention is tied to the specific model generation it was used for. When that model generation is retired from the registry, the dataset is deleted, and the deletion is evidenced by an entry in the audit log and the per-model body.

## Customer enrolment data (face vault)

### Architecture

The face vault is the customer-managed store of identities and their reference embeddings that the face-recognition feature matches against. It is designed so that Kaivue as a processor holds only ciphertext.

- **Customer-side encryption with customer-managed keys (CSE-CMK).** The vault payload (reference embeddings, labels, metadata) is encrypted in the customer's browser or on-prem agent using a data encryption key that is itself wrapped by a key held in the customer's KMS (AWS KMS, GCP KMS, Azure Key Vault, or on-prem HSM). Kaivue never sees the wrapping key.
- **Inference-time decryption.** At inference time, the matching runtime (edge or cloud Triton) requests a short-lived data-key unwrap from the customer KMS via the customer-configured key policy. The unwrapped key lives in memory only, for the duration of the matching session.
- **Key rotation.** Customers may rotate the KMS key at any time; Kaivue provides a rewrap tool that re-encrypts the vault without the plaintext ever leaving customer control.
- **Tenant isolation.** Vault objects are keyed on `(tenant_id, vault_id)` at every layer — database row, S3/R2 prefix, Triton model-repo path, log field.

### Right to erasure

Face recognition's worst-case failure mode for GDPR compliance is the inability to remove a subject from the vault. Kaivue implements erasure as a first-class operation:

1. Customer admin or subject-access tool issues an erasure request.
2. The vault service removes the encrypted record and emits an invalidation event.
3. The invalidation event propagates to all edge Recorders and to Triton cloud nodes that have cached the vault, forcing them to drop the corresponding embeddings from memory.
4. The audit log records the erasure with timestamp, actor, subject ID, and a confirmation from every runtime that processed the invalidation.
5. Backups are designed with erasure propagation in mind: per-subject erasure tombstones flow into the nightly backup process, so restores from backup do not resurrect erased subjects beyond a bounded tolerance window.

Erasure SLA: **7 days** from request to full propagation including backups. This is stricter than GDPR's "without undue delay" requirement and matches the industry norm that customers are willing to defend in their own compliance reviews.

### Retention

Customer vault data is retained only as long as the customer chooses. Kaivue defaults are:

- Enrolled identities: retained until customer deletion.
- Enrolment source images: deleted after embedding generation unless the customer opts into retention for re-enrolment on model upgrade.
- Match events: retained per the customer's configured audit-log retention policy (bounded by KAI-266).

## Customer inference data (live frames)

- Live frames are processed in-memory at the inference site and are not persisted by the face-recognition pipeline.
- Any frame retention is a function of the recorder's general recording policy, which the customer configures independently.
- No frame ever leaves the customer's deployment boundary except for cloud inference, in which case:
  - The frame travels over mTLS,
  - Is processed by Triton and immediately discarded,
  - Triton nodes are operated in a mode that prohibits request-body logging.

## Operator feedback data

Operator feedback (confirmed false positives / false negatives) is used to improve models. This data is:

- Anonymised at collection — the feedback record contains an event ID, the operator-assigned label, and the model's prediction, but not the face image or embedding.
- Joined back to training data only with customer consent and through a documented pipeline.
- Documented per-model as "data used for post-market improvement."

## Interactions with other documents

- `risk-management-system.md` — data risks feed R3, R4, R8.
- `fairness-testing-protocol.md` — operationalises Art. 10(2)(f) bias examination and Art. 10(5) special-category processing.
- `post-market-monitoring.md` — defines how operator feedback closes the loop.
- `serious-incident-reporting.md` — the path when a data-governance failure becomes a reportable incident.
