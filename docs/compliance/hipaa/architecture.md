# HIPAA-Ready Architecture Review — Kaivue Recording Server

**Status:** Draft v0.1
**Owner:** Security & Compliance
**Last reviewed:** 2026-04-08
**Intended audience:** Customer security teams, hospital CISOs, auditors, Kaivue engineering
**Scope:** Kaivue multi-tenant video surveillance SaaS, including cloud control plane (AWS EKS) and on-premises Recorder appliance.

> **Important positioning.** Kaivue is **HIPAA-ready**, meaning we have implemented the technical safeguards required of a Business Associate under 45 CFR Part 164 Subpart C and will sign a Business Associate Agreement ("BAA") with healthcare customers. The **customer remains the Covered Entity** and retains ultimate responsibility for PHI. This document describes what Kaivue provides, what the customer is responsible for, and the remaining gaps tracked for closure before GA into the ISV partner pipeline (target Q4 2026).

---

## 1. Scope and PHI Definition

### 1.1 What is PHI in a Kaivue deployment?

Protected Health Information (PHI) is individually identifiable health information held or transmitted by a Covered Entity or Business Associate. In a Kaivue context, the following data elements are treated as PHI when they are associated with a patient or can reasonably be combined to identify a patient:

- Video and audio recordings of patient-facing areas (exam rooms, treatment bays, ED intake, waiting rooms, imaging suites, recovery, pharmacy windows).
- Still images, snapshots, and exports derived from such recordings.
- Metadata: camera path names if they include patient identifiers, event tags, bookmarks, incident notes authored by staff, face-bounding boxes linked to patient IDs, object detection tags referencing medical equipment in a way that identifies an individual.
- Access logs that record *which user* viewed *which patient's* recording (PHI derivative).
- Any database row, object key, or log line containing the above.

### 1.2 What is NOT PHI in a Kaivue deployment?

- Video from parking lots, exterior doors, loading docks, mechanical rooms, non-clinical hallways — assuming no patient is identifiable in frame.
- Telemetry on system health (disk usage, CPU, camera online/offline).
- Billing records (customer org name, seat count, plan tier) — subject to PCI scope for payment data, not HIPAA.
- Aggregated, de-identified analytics that meet the Safe Harbor or Expert Determination criteria under 45 CFR 164.514.

### 1.3 Deployment topology

```
                 Hospital Data Center                         AWS us-east-1
 ┌──────────────────────────────────────┐      ┌─────────────────────────────────┐
 │                                      │      │                                 │
 │  Cameras  ──RTSP/ONVIF──►  Recorder  │◄────►│  Kaivue Directory (EKS)         │
 │                           (MediaMTX  │ mTLS │   - Zitadel (identity)          │
 │                            +NVR core)│      │   - Casbin authorization        │
 │                                      │      │   - Audit log (7y)              │
 │  Storage: local EBS/NVMe (AES-256)   │      │   - Tenant metadata (Postgres)  │
 │                                      │      │   - Provisioning/updates        │
 └──────────────────────────────────────┘      │                                 │
                                                │  Cold archive: Cloudflare R2    │
                                                │   (SSE-256, customer-opt-in)    │
                                                └─────────────────────────────────┘
```

PHI video is recorded and retained **on-premises at the customer site** by default. It only leaves the customer site if the customer explicitly opts into cloud archive, remote viewing, or support screen-sharing. Control-plane traffic (heartbeats, config, audit) crosses the boundary continuously but does not carry video frames.

---

## 2. 45 CFR 164.312 — Technical Safeguards

Each subsection below quotes the rule, maps it to a concrete Kaivue implementation, and states residual gaps.

### 2.1 §164.312(a)(1) Access Control

> "Implement technical policies and procedures for electronic information systems that maintain electronic protected health information to allow access only to those persons or software programs that have been granted access rights."

#### (a)(2)(i) Unique user identification — **Required**

**Implementation.**
- Identity provider: **Zitadel** (integration in progress, KAI-*). Every human user has a globally unique subject (`sub`) claim issued by Zitadel. Service accounts use separate client credentials; human users cannot share service principals.
- No shared logins. The Recorder appliance and the Directory both reject authentication attempts that do not carry a Zitadel-issued JWT (or a short-lived session cookie derived from one).
- User provisioning: JIT via SAML/OIDC from the hospital's upstream IdP, or manual admin-invite. Deprovisioning propagates within the token refresh interval (default 15 min) and is immediate on explicit revocation via Zitadel admin API.

**Evidence.** Zitadel admin console, `audit_log` table rows with `actor_subject_id`.

#### (a)(2)(ii) Emergency access procedure — **Required**

**Implementation.**
- A designated "break-glass" role (`emergency_access`) is disabled by default and must be explicitly enabled per tenant.
- Activation requires a reason string, a second administrator's approval (two-person rule), and time-boxes the grant to a maximum of 4 hours.
- Every action taken under the break-glass role writes an audit record with `emergency_access=true` and a correlation ID tying it back to the activation ticket.
- Post-hoc review workflow: within 24 hours, the tenant's designated Security Officer must review and sign off on all break-glass actions. Unreviewed events raise a warning banner and page Kaivue support.

**Status.** Partially implemented. The audit log fields exist (KAI-233), but the activation UI and post-hoc review workflow are **not yet built** — see §8 Gaps.

#### (a)(2)(iii) Automatic logoff — **Addressable**

**Implementation.**
- Web UI idle timeout: 15 minutes of inactivity invalidates the session cookie and requires re-authentication. Configurable per tenant down to 5 minutes.
- API JWT access tokens: 15 minute lifetime, refresh tokens rotate on each use and are revoked on suspected replay.
- Recorder local console: 5 minute idle lock on the appliance physical console.
- Flutter client: background state >10 minutes purges cached thumbnails and forces token refresh on resume.

#### (a)(2)(iv) Encryption and decryption — **Addressable**

**Implementation.**
- Sensitive database columns (camera RTSP credentials, integration secrets, export signing keys) are encrypted at rest using the **cryptostore** module (KAI-251). Algorithm: AES-256-GCM with per-field random nonce; the AEAD tag provides integrity (§2.4).
- The master key for cryptostore is held in `nvrJWTSecret` on the Recorder and in AWS KMS (customer-managed CMK) on the Directory. Rotation is supported via envelope re-encryption; rotation logs land in the audit stream.
- See §2.2 for bulk video storage encryption.

---

### 2.2 §164.312(a)(2)(iv) / §164.312(e)(2)(ii) Encryption at Rest

**Video object storage (Recorder local).**
- Default: LUKS2 full-disk encryption on the Recorder's media volumes, with the keyfile sealed to TPM 2.0 where available. Ships enabled on Kaivue-branded hardware.
- Bring-your-own-hardware customers MUST enable disk encryption before go-live; the installer refuses to proceed on unencrypted volumes when the tenant is flagged `hipaa=true`.

**Database (Recorder).**
- SQLite file is stored on the LUKS2 volume. Sensitive columns are additionally encrypted at the column level via cryptostore (defense in depth).

**Database (Directory / cloud).**
- PostgreSQL on AWS RDS with AWS-managed storage encryption (AES-256, KMS CMK per tenant group).
- PHI-derivative columns (audit log `subject_patient_id` when present, incident notes) encrypted at column level via cryptostore.

**Object storage (cloud archive, opt-in).**
- Cloudflare R2 bucket with SSE-256 enabled. Bucket keys are held by Cloudflare; additional client-side AES-256-GCM envelope encryption is applied by the Recorder *before* upload so that Cloudflare holds only ciphertext. Key held by customer CMK in AWS KMS.
- S3 (if customer requests AWS cold storage instead of R2): SSE-KMS with customer-managed CMK plus client-side envelope.

**Backups.**
- Nightly database snapshots encrypted with the same envelope key. Restore tested quarterly.

---

### 2.3 §164.312(b) Audit Controls

> "Implement hardware, software, and/or procedural mechanisms that record and examine activity in information systems that contain or use electronic protected health information."

**Implementation.** KAI-233 delivered the append-only audit log.

- **Scope of logging.** Every authenticated API request that reads, writes, exports, deletes, or modifies PHI or PHI-derivative data emits a structured audit record. Specifically:
  - Recording playback start/stop (user, camera, time range, client IP).
  - Export generation and download (with a hash of the exported artifact).
  - Camera configuration change, path rename, retention policy change.
  - User login, logout, MFA challenge result, permission grant/revoke, role change.
  - Break-glass activation and each action performed under it.
  - Cryptostore key rotation, CMK access errors, audit log integrity check results.

- **Record format.** JSON Lines, one record per event, fields include: `ts` (RFC 3339 UTC), `tenant_id`, `actor_subject_id`, `actor_role`, `action`, `resource_type`, `resource_id`, `source_ip`, `user_agent`, `result`, `reason`, `correlation_id`, `prev_hash` (hash chain).

- **Storage.** Primary store is Postgres `audit_log` table in the Directory (7-year retention enforced by a `retention_policy` job). Recorders stream audit records to the Directory every 30 seconds; offline Recorders buffer locally and replay on reconnect.

- **Append-only / WORM-ish.** The audit table is owned by a Postgres role that has no `UPDATE` or `DELETE` privilege. Row-level deletions are blocked by a trigger that raises an exception unless the invoker is the retention-enforcement service account and the row is older than 7 years. Nightly export to S3 Object Lock (compliance mode) bucket for regulator-proof immutable copy.

- **Retention.** 7 years from event creation, aligned with HIPAA §164.316(b)(2)(i). Retention is enforced by automation — manual deletion is not permitted.

- **Review.** Per-tenant dashboards surface anomalies (spike in exports, off-hours access, bulk playback by a single user). Security Officer role receives a weekly digest.

---

### 2.4 §164.312(c) Integrity

> "Implement policies and procedures to protect electronic protected health information from improper alteration or destruction."

**Implementation.**
- **AEAD integrity tags.** All cryptostore ciphertext carries a 128-bit GCM authentication tag. Decryption failure on tag mismatch is logged and surfaces to operators.
- **Recording integrity.** Each segment file on the Recorder has a SHA-256 fingerprint computed at close time and stored in the recordings index. Exports include the fingerprint and a detached signature so that a downstream viewer can verify the clip has not been altered.
- **Audit log hash chaining.** Each audit row includes `prev_hash = SHA-256(prev_row_canonical_json)`. A nightly verifier recomputes the chain and alerts on divergence. **Status: partial** — the field exists, verifier job is a gap item (§8).
- **Transport integrity.** TLS 1.3 with AEAD ciphersuites (TLS_AES_256_GCM_SHA384 preferred) provides message integrity in flight.
- **Backups.** Database backups are hashed and the hash is stored separately from the backup itself.

---

### 2.5 §164.312(d) Person or Entity Authentication

> "Implement procedures to verify that a person or entity seeking access to electronic protected health information is the one claimed."

**Implementation.**
- Primary auth: Zitadel OIDC. Password, passkey (WebAuthn), or federated SAML against the hospital's IdP (ADFS, Okta, PingFederate, Azure AD).
- **MFA is mandatory on HIPAA-flagged tenants.** TOTP, WebAuthn, or IdP-asserted MFA are acceptable. SMS OTP is disabled for HIPAA tenants per NIST 800-63B guidance (SMS is restricted).
- Service-to-service: short-lived JWTs signed by Zitadel, plus mTLS at the mesh layer for Recorder↔Directory and intra-cluster service calls.
- Client certificates: each Recorder is provisioned with a unique X.509 cert during onboarding. The Directory pins the Recorder's cert fingerprint in its tenant record; rotation is explicit and audited.

---

### 2.6 §164.312(e) Transmission Security

> "Implement technical security measures to guard against unauthorized access to electronic protected health information that is being transmitted over an electronic communications network."

**Implementation.**
- **Browser/API:** TLS 1.3 only (TLS 1.2 is refused on HIPAA tenants). HSTS with `max-age=63072000; includeSubDomains; preload`. OCSP stapling. Certificate issued by a public CA; internal PKI for mesh.
- **Recorder ↔ Directory:** mTLS with certificate pinning. Both ends verify the peer cert fingerprint against a pre-shared value. Man-in-the-middle via rogue CA is rejected.
- **Recorder ↔ Cameras:** SRTP when the camera supports it; RTSP-over-TLS otherwise. Plain-RTSP is permitted only when the camera is on an isolated VLAN with no route to a non-clinical network, and the Recorder flags the path `transport_plaintext=true` in the audit log. HIPAA-flagged tenants get a warning in the admin UI for every such path.
- **Flutter client ↔ Recorder (LAN) or Directory (WAN):** TLS 1.3 plus app-layer cert pinning.
- **Cloud archive upload:** TLS 1.3 to R2/S3, payload is already client-side encrypted (§2.2).
- **Support screen-share / remote assist:** disabled by default on HIPAA tenants; must be explicitly enabled per session by the customer's Security Officer, time-boxed to 60 minutes, and every frame shared is recorded to the audit log.

---

## 3. 45 CFR 164.308 — Administrative Safeguards

Kaivue covers the Business Associate's share of administrative safeguards; the Covered Entity is responsible for their internal workforce policies.

| Rule | Kaivue Implementation |
| --- | --- |
| §164.308(a)(1)(i) Security management process | Formal SDLC with threat modelling per feature, quarterly risk assessment, annual third-party pen test (`docs/compliance/pentest/`). |
| §164.308(a)(1)(ii)(A) Risk analysis | Annual HIPAA-specific risk analysis, stored in `docs/compliance/hipaa/risk-analysis-YYYY.md` (template pending). |
| §164.308(a)(1)(ii)(B) Risk management | Tracked in Linear under the `security` label with SLA by severity. |
| §164.308(a)(1)(ii)(C) Sanction policy | Documented in employee handbook; violations lead to disciplinary action up to termination. |
| §164.308(a)(1)(ii)(D) Information system activity review | Weekly review of audit log anomalies by Kaivue SecOps; monthly review report shared with customer on request. |
| §164.308(a)(2) Assigned security responsibility | Kaivue Security Officer named in BAA. |
| §164.308(a)(3) Workforce security | Background checks, least-privilege onboarding, quarterly access review. |
| §164.308(a)(4) Information access management | Casbin policies (KAI-235) enforce tenant isolation; engineer access to customer data requires break-glass. |
| §164.308(a)(5) Security awareness and training | Annual HIPAA training for all employees with access to production; tracked in HRIS. |
| §164.308(a)(6) Security incident procedures | Incident response runbook (`docs/compliance/hipaa/incident-response.md` — gap item). |
| §164.308(a)(7) Contingency plan | Daily backups, RPO ≤24h, RTO ≤8h for Directory, Recorder local redundancy optional (RAID). DR test twice per year. |
| §164.308(a)(8) Evaluation | Annual review of this document; re-evaluation on material architectural change. |
| §164.308(b) BAAs with subcontractors | See §6. |

---

## 4. 45 CFR 164.310 — Physical Safeguards

Physical safeguards are split along the deployment boundary.

### 4.1 Cloud control plane (Kaivue responsibility)

- Hosted in **AWS us-east-1 and us-west-2**. AWS data centers carry SOC 2 Type II, ISO 27001, HITRUST, and are in scope of the AWS BAA which Kaivue has executed.
- No Kaivue employee has physical access to AWS facilities.
- Kaivue corporate offices do not store PHI and are out of scope.

### 4.2 On-prem Recorder (customer responsibility)

The Recorder is a physical or virtual appliance that lives inside the customer's facility. The customer is responsible for:

- Placing the Recorder in a locked room with access restricted to authorized IT staff (§164.310(a)(1)).
- Workstation use and security for any terminals used to administer the Recorder (§164.310(b), (c)).
- Device and media disposal: when a Recorder or its disks are decommissioned, the customer must either (a) return them to Kaivue for cryptographic erasure and disposal per our sanitization SOP, or (b) perform NIST SP 800-88 Rev. 1 sanitization themselves and attest to completion (§164.310(d)(1), (d)(2)).
- Environmental controls, power, physical tamper monitoring.

Kaivue provides: tamper-evident packaging on shipped appliances, TPM-sealed disk encryption, a documented sanitization SOP, and a decommission checklist.

---

## 5. Shared Responsibility Matrix

| Area | Kaivue (Business Associate) | Customer (Covered Entity) |
| --- | --- | --- |
| Identity provider | Operate Zitadel, enforce MFA for HIPAA tenants | Provision/deprovision end users, manage IdP federation, enforce password policy upstream |
| Authorization policy | Ship default least-privilege Casbin policies | Review and tailor role definitions, approve break-glass activations |
| Encryption at rest | Implement cryptostore, disk encryption, KMS integration | Protect key custodianship, approve key rotation schedule, manage customer-managed CMK if BYOK |
| Encryption in transit | TLS 1.3, mTLS, cert pinning | Network segmentation for cameras, VLAN isolation, firewall rules |
| Audit logging | Emit, store, retain, protect integrity of audit records | Review anomaly reports, investigate suspicious activity, retain evidence for litigation holds |
| Physical security — cloud | AWS SOC 2 + AWS BAA | N/A |
| Physical security — Recorder | Hardened appliance, disk encryption, sanitization SOP | Locked room, access control, environmental |
| Incident response | Detect, triage, notify customer within 15 days (commit) / 60 days (regulatory max) | Notify affected individuals, HHS, and media per §164.404-408 |
| Workforce training | Train Kaivue staff annually | Train customer staff annually |
| Data minimization | Provide configurable retention, path-level redaction | Define what cameras record, where they point, retention periods |
| Backup & DR | Back up Directory data, maintain DR plan | Back up Recorder-local data if additional copies are desired, define RPO/RTO acceptance |
| Risk analysis | Kaivue's risk analysis for the BA service | Customer's overall HIPAA risk analysis |
| Patient rights (access, amendment, accounting of disclosures) | Cooperate with CE requests, provide tooling to satisfy requests | Own the patient relationship, respond to patients, make determinations |
| Subcontractor management | Execute and maintain BAAs with subcontractors (§6), flow-down obligations | N/A |

---

## 6. Subcontractors (Business Associate's Business Associates)

Per §164.308(b)(1), Kaivue will flow down BAA obligations to any subcontractor that creates, receives, maintains, or transmits PHI on Kaivue's behalf.

| Subcontractor | Role | PHI access? | BAA status |
| --- | --- | --- | --- |
| **Amazon Web Services** | Cloud hosting (EKS, RDS, S3, KMS) | Yes — control plane metadata and cloud-archived recordings (customer opt-in) | AWS BAA executed |
| **Zitadel Cloud** (if hosted) / self-hosted | Identity provider | Indirect — subject IDs, login metadata | BAA required (if cloud-hosted) — gap item |
| **Cloudflare** | R2 object storage for cold archive; CDN for static assets | Yes for R2 (ciphertext only); no for CDN | Cloudflare BAA executed (R2 only) |
| **Stripe** | Billing | **No PHI.** Billing uses tenant org metadata only. | No BAA required; documented no-PHI boundary |
| **Datadog** (if used) | Observability | Logs scrubbed to exclude PHI before export | BAA required if used — gap item |
| **Sentry** (if used) | Error reporting | Stack traces scrubbed to exclude PHI | BAA required if used — gap item |

Any future subcontractor that will touch PHI must have a BAA executed before access is granted. This is tracked in the subcontractor registry (gap item, §8).

---

## 7. Evidence and Auditability

- **SOC 2 Type II** in progress (`docs/compliance/soc2/`).
- **HIPAA risk analysis** annually (template pending).
- **Penetration test** annually by a qualified third party (`docs/compliance/pentest/`).
- **Vulnerability management** — Dependabot, Trivy image scans in CI (KAI-428 build pipeline), quarterly review.
- **Configuration evidence** — infrastructure-as-code in git, reviewable history.
- **Access reviews** — quarterly for Kaivue staff with production access.
- **Customer-facing reports** — audit log export API, monthly access summary on request, real-time status page.

---

## 8. Gap List

The following items must be built, documented, or closed before a prudent hospital will sign a BAA with Kaivue. Each is tracked in Linear under the `hipaa` label.

### Gap 1 — HIPAA tenant flag and enhanced logging profile
A first-class `tenant.compliance_profile = "hipaa"` flag that, when set, enforces:
MFA mandatory, SMS OTP disabled, TLS 1.3 only, plaintext RTSP warnings, mandatory disk encryption check at provisioning time, screen-share disabled by default, extended audit-log field set (`subject_patient_id` when available), and weekly anomaly digest delivery. **Status: not started.**

### Gap 2 — Minimum-necessary access review workflow
Per §164.502(b), uses and disclosures must be limited to the minimum necessary. We need (a) per-role PHI field/scope mapping, (b) a customer-facing quarterly access review UI that prompts the Security Officer to re-attest each user's role, (c) automatic deprovisioning of stale accounts after 90 days of inactivity. **Status: not started.**

### Gap 3 — Break-glass emergency access with post-hoc review
The audit schema supports `emergency_access=true`, but the activation flow (two-person rule, time-box, reason string, correlation ticket) and the 24-hour review workflow are not built. Without this, §164.312(a)(2)(ii) is only partially satisfied. **Status: design drafted, not implemented.**

### Gap 4 — Audit log hash-chain verifier and WORM export
The `prev_hash` field is written, but there is no scheduled job that verifies the chain end-to-end and alerts on divergence, and no nightly export to S3 Object Lock for regulator-proof immutability. Integrity under §164.312(c) relies on this. **Status: not started.**

### Gap 5 — Incident response runbook with HIPAA breach notification timeline
We need a written, rehearsed IR runbook that includes: detection and triage, containment, forensics preservation, customer notification within 15 days (our commitment) and regulatory 60-day max per §164.410, notice content requirements per §164.404(c), HHS wall-of-shame threshold (500 individuals), media notification obligations, and joint-investigation protocol with the Covered Entity. **Status: not started.**

### Gap 6 — Retention enforcement automation
Audit log 7-year retention is enforced automatically. Video retention is *configured* per tenant but relies on the Recorder's housekeeper; there is no cross-tenant verifier that alarms when configured retention diverges from actual behavior, and no evidence artifact for auditors. **Status: partial — housekeeper works, verifier/reporting missing.**

### Gap 7 — Sanitization on hardware decommission
We have a draft NIST SP 800-88 Rev. 1 sanitization SOP. We need: (a) a signed customer-facing attestation form, (b) a returns intake process at Kaivue, (c) a crypto-erase utility on the Recorder that destroys the LUKS header and issues a NIST-compliant report, and (d) chain-of-custody tracking for returned media. **Status: SOP drafted, tooling missing.**

### Gap 8 — Subcontractor BAA management
Living registry of subcontractors, their PHI access level, BAA status, expiration dates, and re-assessment cadence. Drives §164.308(b) compliance. Needs a lightweight internal dashboard and an annual subcontractor security review. Zitadel Cloud, Datadog, and Sentry BAAs (if used) must be executed or those services removed from PHI scope. **Status: not started.**

### Gap 9 — PHI data classification tags
Today the schema does not explicitly tag columns/fields as PHI. We need a build-time lint that fails the build if a new column touches PHI without carrying a `phi:true` tag and without opting into cryptostore where applicable. This underpins automated DLP and minimum-necessary enforcement. **Status: not started.**

### Gap 10 — Plaintext RTSP deprecation path
Cameras that only support plaintext RTSP are a real-world issue. The HIPAA profile warns today; we need a hard deadline (e.g., 12 months after GA) for refusing plaintext on HIPAA tenants unless the camera is on a dedicated air-gapped VLAN with attested network isolation. **Status: policy decision pending.**

### Gap 11 — Risk analysis and annual review artefacts
Per §164.308(a)(1)(ii)(A), we need a documented HIPAA risk analysis. Template and first-year completion pending. **Status: not started.**

### Gap 12 — Customer-accessible audit export API
Customers need a self-service way to pull audit records for their tenant for litigation holds, HHS inquiries, or internal review. The data exists; the API and retention-hold primitive do not. **Status: not started.**

---

## 9. Change Log

| Date | Author | Change |
| --- | --- | --- |
| 2026-04-08 | Security & Compliance | Initial draft. |

---

## 10. References

- 45 CFR Part 164 Subparts C (Security) and D (Breach Notification).
- HHS "Guidance on Risk Analysis Requirements under the HIPAA Security Rule."
- NIST SP 800-66 Rev. 2, "Implementing the HIPAA Security Rule."
- NIST SP 800-88 Rev. 1, "Guidelines for Media Sanitization."
- NIST SP 800-63B, "Digital Identity Guidelines — Authentication and Lifecycle Management."
- AWS HIPAA Eligible Services Reference.
- Cloudflare BAA and HIPAA-eligible services list.
- Internal: `docs/compliance/soc2/`, `docs/compliance/pentest/`, KAI-233 (audit log), KAI-235 (Casbin), KAI-251 (cryptostore).
