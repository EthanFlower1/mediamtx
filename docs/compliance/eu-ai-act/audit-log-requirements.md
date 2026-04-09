---
title: Audit Log Requirements — Face Recognition
owner: lead-security
status: draft
last-updated: 2026-04-08
eu-ai-act-article: 12
---

# Audit Log Requirements — Face Recognition

Article 12 requires high-risk AI systems to automatically record events
("logs") throughout their lifecycle sufficient to enable traceability of the
system's functioning appropriate to its intended purpose. For a biometric
identification system this means every inference and every human decision.

This document specifies **what** is logged, **where** it is stored, **who**
can see it, and **for how long**. It references the KAI-233 audit log
service, which is the canonical audit log infrastructure for all Kaivue
subsystems.

## Logged events

### 1. Inference event

Emitted for every face recognition inference, match or no-match.

| Field                   | Notes                                                                                           |
| ----------------------- | ----------------------------------------------------------------------------------------------- |
| `event_id`              | UUID, unique per event                                                                          |
| `tenant_id`             | Tenant (customer) identifier                                                                    |
| `site_id`               | Site within tenant                                                                              |
| `camera_id`             | Source camera                                                                                   |
| `timestamp_utc`         | Nanosecond-precision timestamp                                                                  |
| `model_version_hash`    | SHA-256 of the deployed model weights + config                                                  |
| `feature_version`       | Kaivue build version                                                                            |
| `input_image_hash`      | SHA-256 of the cropped face frame. **The image itself is NEVER stored in the audit log.**       |
| `input_quality_score`   | From the quality gate                                                                           |
| `candidates`            | Array of top-k (id, confidence) tuples                                                          |
| `decision`              | `match`, `no-match`, `quality-reject`, `error`                                                  |
| `operator_id`           | If the pipeline produced an operator-facing alert, populated when operator acts; otherwise null |
| `operator_action`       | `confirm`, `reject`, `escalate`, `timeout`, `pending`, null                                     |
| `operator_reason`       | Optional reason chip/text                                                                       |
| `downstream_action`     | `none`, `door-unlock`, `alarm-raise`, `notification`, etc.                                      |
| `enrollment_photo_hash` | SHA-256 of the enrollment photo for the matched candidate                                       |

### 2. Enrollment event

Emitted for enrollment add, update, remove, consent withdrawal.

| Field                                    | Notes                                                                  |
| ---------------------------------------- | ---------------------------------------------------------------------- |
| `event_id`, `tenant_id`, `timestamp_utc` | as above                                                               |
| `actor_operator_id`                      | Who performed the action                                               |
| `second_approver_id`                     | For four-eyes actions                                                  |
| `target_enrollee_id`                     | Subject of the action (pseudo-id, not name)                            |
| `action`                                 | `enroll`, `update`, `remove`, `withdraw-consent`, `emergency-override` |
| `reason_text`                            | Free text, required for emergency override                             |
| `enrollment_photo_hash`                  | SHA-256 of the photo used (if any)                                     |
| `consent_record_ref`                     | Reference to the signed consent artifact in the tenant store           |

### 3. Configuration event

Emitted on any change to face recognition configuration.

- Kill-switch on/off
- Confidence threshold changes
- Retention policy changes
- Integration enable/disable (access control, alarm)
- Cosigned by four-eyes where required

### 4. Administrative event

- Operator training sign-off
- Operator lockout / reactivation
- Access to the audit log itself (read-log-of-the-log)

## What is explicitly NOT logged

- Raw face images (only SHA-256 hashes)
- Raw embedding vectors (stored in the tenant face vault, not the audit log)
- Personally identifying name strings where a pseudo-id suffices
- Data from cameras belonging to a different tenant (hard isolation)

## Storage

- **Backend:** KAI-233 audit log service, cloud-hosted, per-tenant partition.
- **Encryption at rest:** per-tenant key derived from the tenant master key.
- **Encryption in transit:** TLS 1.3 minimum.
- **Immutability:** append-only, tamper-evident (hash-chained or equivalent).
  Modifications require a new entry referencing the original; the original
  is never mutated.
- **Backup:** per the KAI-233 backup policy, encrypted, geo-resilient, with
  documented restore tests.

## Retention

**7 years** minimum. This exceeds the AI Act Article 19 minimum of 6 months
(or longer where required by Union or national law) to align with SOC 2
evidence retention and common HIPAA audit log retention expectations. Tenants
may extend retention further by configuration. Retention cannot be shortened
below 7 years.

## Access control

- **Tenant admin:** may read their own tenant's audit logs in full. No
  mutation. No export of raw hashes beyond the Admin Console without a
  logged export action.
- **Tenant operator:** may read their own actions only.
- **Kaivue lead-security:** may read for incident response, with the read
  itself being logged. Break-glass procedure documented separately.
- **Kaivue engineers (non-security):** no access to production audit logs.
- **External auditors:** time-boxed, scoped, read-only access under NDA.
- **Never leaves the tenant boundary:** audit logs for tenant A are never
  visible to tenant B. Cross-tenant queries require explicit customer
  consent and are logged.

## Integrity and verification

- Each log entry includes a hash chain pointer (prev entry hash), so any
  missing or altered entry is detectable.
- A daily cron runs a self-consistency check per tenant and files a
  discrepancy alert if the chain breaks.
- Annual external audit tests integrity on a sample.

## Latency and volume budget

**TODO (lead-ai + lead-security):** estimate per-tenant event rate at
production scale and confirm KAI-233 ingestion capacity. Face recognition
is expected to be the highest-volume producer, potentially 10x LPR volume.

## Access of the logs by the AI Act authorities

Article 21 requires providers to make logs available to competent authorities
upon a reasoned request. Kaivue's obligation:

- Provide a documented export format.
- Provide a search tool scoped to the relevant time window, camera, or
  enrolled id.
- Retain the request and the disclosure in its own audit trail.
- **TODO (lead-security):** publish the authority-cooperation runbook.

## Integration with KAI-233

- Schema: face recognition events register a dedicated namespace
  `ai.face_recognition.*` with the KAI-233 event registry.
- Ingest path: Recorder emits over the authenticated audit-log gRPC, falls
  back to local spool on outage, ships on reconnect.
- Tests: integration tests cover spool-and-flush and per-tenant isolation.

## TODOs

- [ ] lead-security — finalize the event schema with the KAI-233 team
- [ ] lead-security — authority-cooperation runbook
- [ ] lead-ai — event-rate estimate and pipeline capacity confirmation
- [ ] lead-security — tamper-evidence mechanism choice (hash chain vs Merkle)
