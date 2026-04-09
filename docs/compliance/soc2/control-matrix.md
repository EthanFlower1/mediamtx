# SOC 2 Type I Control Matrix and Gap Analysis (KAI-385)

**Status:** WORKING DRAFT — not an audit deliverable. Maintained by lead-security.
**Phase:** 1 of 5 — Scoping + Trust Services Criteria selection (sub-ticket **KAI-438**).
**Last updated:** 2026-04-08
**Target observation date (Type I):** TBD — see §6.
**Pen test gate (KAI-390):** must complete before audit fieldwork.

This is a living document. Every uncertain claim is marked `TODO` with the
specific question that needs answering. Do not promote out of draft until
the §"Open questions for lead-security" block is empty.

---

## 1. Scope statement

### 1.1 In-scope systems

The "system" for SOC 2 purposes is the **Kaivue recording, storage,
playback, and administration platform**. Specifically:

- **Cloud control plane** — all Go services under `internal/cloud/` including
  `apiserver/`, `archive/`, `audit/`, `cameras/`, `db/`, `directoryingest/`,
  `identity/`, `jobs/`, `permissions/`, `recordercontrol/`, `relationships/`,
  `streams/`, `tenants/`. Deployed on AWS EKS (KAI-215, pending), backed by
  RDS Postgres + pgvector (KAI-216) and ElastiCache Redis (KAI-217).
- **On-prem Recorder / Directory / Gateway** — Go binaries built from this
  repo (`internal/directory/`, `internal/recorder/`, `internal/recordstore/`,
  `internal/playback/`, `internal/nvr/`). SQLite via `modernc.org/sqlite`,
  no CGO. Runs on customer premises but managed from the cloud control plane.
- **Customer admin web console** — `ui-v2/` React app (KAI-307 scaffold landed).
- **Integrator portal** — same React bundle, integrator-scoped routes
  (KAI-310 / KAI-311 / KAI-313 / KAI-315, pending).
- **Flutter end-user clients** — `clients/flutter/` for iOS/Android/desktop/web.
  Token storage hardened per KAI-298. **Note:** the client-side is in scope
  for data-flow review only; SOC 2 service-org controls do not apply to
  customer-installed apps. Authentication path through the cloud _is_ in scope.
- **Public REST + Connect-Go API** (KAI-399, in progress) and webhook delivery
  (KAI-397 outbound, KAI-398 inbound). Will be in scope once `pkg/api/` lands.
- **Identity firewall** — `internal/shared/auth/` (`provider.go`, `types.go`,
  `IdentityProvider` interface). Production binding is Zitadel
  (KAI-220 deploy, KAI-221 bootstrap — both pending).
- **Supporting shared packages** — `internal/shared/cryptostore/` (KAI-251,
  AES-256-GCM + HKDF, FIPS-track), `internal/shared/mesh/`, `internal/shared/proto/`.

### 1.2 Out of scope

- Marketing website (`marketing/`) and public trust/status page content.
- Internal developer tooling: `scripts/`, `.claude/`, `.claude-flow/`, build helpers.
- Documentation portal (`docs-portal/` Mintlify site).
- Qt 6 video wall client (`clients/video-wall/`) — operator UI on customer
  premises; not a service-organization service.
- Third-party SaaS products we _consume_ — their controls are inherited
  via sub-service carve-out (see §1.3).

### 1.3 Sub-service organizations (carve-out)

We plan a **carve-out** report — these providers' controls are excluded
from our opinion, and we document the complementary user-entity controls
(CUECs) we rely on them for. Auditor will want the most recent SOC 2 report
from each on file.

| Sub-service                            | Purpose                                                      | SOC 2 report on file?              |
| -------------------------------------- | ------------------------------------------------------------ | ---------------------------------- |
| AWS                                    | EKS (KAI-215), RDS (KAI-216), ElastiCache (KAI-217), S3, KMS | Yes (public via Artifact)          |
| Cloudflare (R2)                        | Cloud archive (KAI-265, KAI-266)                             | TODO — pull current report         |
| Zitadel Cloud _(or self-hosted — TBD)_ | OIDC IdP (KAI-220 / KAI-221)                                 | TODO — depends on hosting decision |
| Stripe (+ Stripe Connect)              | Billing, marketplace (KAI-361)                               | Yes (public)                       |
| Avalara / Anrok                        | Tax compliance (KAI-365)                                     | TODO                               |
| Twilio / SendGrid                      | Transactional email + SMS (if adopted)                       | TODO — confirm adoption            |
| Sentry                                 | Error telemetry (if adopted)                                 | TODO — confirm adoption            |
| PagerDuty / Opsgenie                   | Paging / IR (KAI-408)                                        | TODO — pending selection           |

---

## 2. Trust Services Criteria selection

For **Type I** we target **Security (Common Criteria CC1–CC9) only**. Every
other TSC is explicitly deferred with written rationale:

| TSC                      | Type I decision                                      | Rationale                                                                                                                                                                                                                                                                                                                                                                            |
| ------------------------ | ---------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| **Security** (CC1–CC9)   | **Included**                                         | Non-negotiable baseline. Every SOC 2 report includes it.                                                                                                                                                                                                                                                                                                                             |
| **Availability**         | **Deferred to Type II**                              | Availability requires SLOs, monitoring, and incident history. Type II's observation window is the natural place to demonstrate uptime evidence (KAI-422 Prometheus/Grafana, KAI-341 auto-recovery, KAI-408 paging). Committing to it for a point-in-time Type I adds cost without demonstrating real operational evidence.                                                           |
| **Confidentiality**      | **Deferred to Type II**                              | Overlaps ~80% with Security; the incremental controls (data classification policy, customer data handling matrix) are on the gap list (§3) but not yet written. Add in Type II when they exist and have operated long enough to be sampled.                                                                                                                                          |
| **Processing Integrity** | **Deferred permanently (re-evaluate post-GA)**       | Designed for financial / transactional systems where the output must equal the input per a defined calculation. Not a natural fit for a recording server — "processing integrity" of a video stream is not what auditors mean by this TSC. Our media integrity concerns (timestamp correctness, no silent frame drops) are addressed by engineering test coverage, not TSC evidence. |
| **Privacy**              | **Deferred — addressed via GDPR program separately** | Privacy TSC maps to AICPA's Generally Accepted Privacy Principles. We are addressing privacy via the GDPR program (DSAR tooling spec — task #50, DPO workflows) and EU AI Act (KAI-282 / KAI-294). A future SOC 2 + Privacy report is possible once the GDPR program is operational.                                                                                                 |

Recording this decision explicitly is important because auditors sometimes
push teams to include more TSCs than their control set can support. **Do not
agree to add Confidentiality or Availability during auditor selection** — the
answer is "planned for Type II, out of scope for Type I, see the written TSC
selection in the control matrix."

---

## 3. Common Criteria coverage table

One row per applicable Common Criterion. Status legend: **I** = implemented,
**P** = partial, **G** = gap. "TODO" in Evidence means "evidence source must
be created" — either by writing a policy, generating a log, or pointing at
an artifact that already exists but is not yet cataloged here.

### CC1 — Control Environment

| ID    | Title                           | Control description                                                                                    | Evidence                                                                                     | Status | Owner                       |
| ----- | ------------------------------- | ------------------------------------------------------------------------------------------------------ | -------------------------------------------------------------------------------------------- | ------ | --------------------------- |
| CC1.1 | Integrity & ethical values      | Code of conduct, employee handbook, security AUP signed at hire.                                       | TODO — lead-people: where does the handbook live? Does it exist?                             | G      | lead-people / lead-security |
| CC1.2 | Board / governance oversight    | Security review cadence; lead-security reports to founder; quarterly security review meeting minutes.  | TODO — meeting cadence not yet established. Decision for founder.                            | G      | lead-security / founder     |
| CC1.3 | Org structure & reporting lines | `.claude/agents/` tech-lead routing, 10 lead roles documented in `.worktrees/engineering-briefing.md`. | `.worktrees/engineering-briefing.md` (internal); needs an externalized org chart for auditor | P      | founder                     |
| CC1.4 | Hiring & competence             | Background checks, role-based onboarding, security responsibilities in JDs.                            | TODO — no documented policy                                                                  | G      | lead-people                 |
| CC1.5 | Accountability                  | Performance reviews referencing security responsibilities; discipline for policy violations.           | TODO                                                                                         | G      | lead-people                 |

### CC2 — Communication and Information

| ID    | Title                                | Control description                                                                                                                                                                    | Evidence                                                                | Status | Owner         |
| ----- | ------------------------------------ | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------- | ------ | ------------- |
| CC2.1 | Internal info quality                | Engineering uses Linear (KAI-\*) as system of record; PRs link tickets; git log references KAI numbers.                                                                                | Linear workspace; recent `git log` (e.g. commit `5792529c` ci(kai-428)) | I      | tech-lead     |
| CC2.2 | Internal communication of objectives | Roadmap at `docs/superpowers/specs/2026-04-07-v1-roadmap.md`; engineering briefing at `.worktrees/engineering-briefing.md`.                                                            | Those files                                                             | P      | tech-lead     |
| CC2.3 | External communication               | Trust center (KAI-394, pending), status page (TODO), `security@kaivue.*` inbox (TODO), responsible disclosure policy (TODO — current `SECURITY.md` only points upstream mediamtx.org). | `SECURITY.md` (upstream-only — gap)                                     | G      | lead-security |

### CC3 — Risk Assessment

| ID    | Title                                  | Control description                                                                                  | Evidence                                                   | Status | Owner               |
| ----- | -------------------------------------- | ---------------------------------------------------------------------------------------------------- | ---------------------------------------------------------- | ------ | ------------------- |
| CC3.1 | Objectives specified to enable risk ID | Roadmap + GA criteria documented; hard deadlines (Aug 2 2026 EU AI Act, GA-gating tickets).          | `docs/superpowers/specs/2026-04-07-v1-roadmap.md`          | P      | tech-lead           |
| CC3.2 | Risk identification                    | Subsystem threat models — cryptostore has a written model with in/out-of-scope sections.             | `internal/shared/cryptostore/README.md` §"Threat model"    | P      | lead-security       |
| CC3.3 | Fraud risk                             | Billing / Stripe Connect fraud surface (KAI-361), face-recognition misuse (KAI-282).                 | TODO — no documented fraud-risk review yet                 | G      | lead-security       |
| CC3.4 | Significant-change risk                | PR review + CI gates (KAI-432 typecheck, KAI-431 proto, KAI-428 reproducible build + SBOM + cosign). | `.github/workflows/`, commit `5792529c`, commit `f19425bd` | P      | lead-eng / lead-sre |

### CC4 — Monitoring Activities

| ID    | Title                                      | Control description                                                                                                                                                                                                         | Evidence                                                                                                                                                       | Status | Owner         |
| ----- | ------------------------------------------ | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------ | ------------- |
| CC4.1 | Ongoing monitoring                         | **Audit log (KAI-233).** Every authenticated cloud API handler emits exactly one entry: 2xx → allow, 403 → deny, error → explicit. Tenant-scoped; cross-tenant chaos-tested. 7-year default retention, per-tenant override. | `internal/cloud/audit/README.md`, `entry.go`, `sql.go`, `middleware/`, `TestSQLRecorder_ChaosCrossTenant`. **Not yet on main — lives on `wave1-integration`.** | P      | lead-security |
| CC4.2 | Evaluation & communication of deficiencies | Pen test (KAI-390) report intake; remediation tracked in Linear; quarterly control self-assessment.                                                                                                                         | Pen test not yet scoped (task #44)                                                                                                                             | G      | lead-security |

### CC5 — Control Activities

| ID    | Title                                  | Control description                                                                                                                         | Evidence                               | Status | Owner         |
| ----- | -------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------- | ------ | ------------- |
| CC5.1 | Control activities for risk mitigation | Cryptostore (KAI-251), Casbin RBAC (KAI-225), tenant isolation (KAI-235), audit log (KAI-233), identity firewall (`internal/shared/auth/`). | See CC6 rows below                     | P      | lead-security |
| CC5.2 | Technology general controls            | CI build, reproducible builds, SBOM (CycloneDX), cosign container signing (KAI-428, commit `5792529c`).                                     | `.github/workflows/` from KAI-428      | P      | lead-sre      |
| CC5.3 | Policies & procedures                  | Written security policy set: AUP, access control, change management, IR, vendor management, data classification, encryption, BCP/DR.        | **None yet** — covered by gap #2 below | G      | lead-security |

### CC6 — Logical and Physical Access Controls

_The bulk of our implemented controls live here._

| ID    | Title                                      | Control description                                                                                                                                                                                                                                                                                                                     | Evidence                                                                                                                   | Status                          | Owner         |
| ----- | ------------------------------------------ | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------- | ------------------------------- | ------------- |
| CC6.1 | Logical access — identity & authentication | `IdentityProvider` interface (`internal/shared/auth/provider.go`) is the architectural seam — the IdP firewall. Production binding: Zitadel. Tokens verified via `VerifyToken`; interface contract forbids information leak. MFA delegated to IdP.                                                                                      | `internal/shared/auth/provider.go`, `types.go`, `certmgr/`, `fake/`                                                        | P (Zitadel KAI-220/221 pending) | lead-security |
| CC6.2 | User provisioning / deprovisioning         | Tenant-scoped CRUD via `IdentityProvider` (`ListUsers`, `GetUser`, group sync KAI-147). No application-local password store.                                                                                                                                                                                                            | `internal/shared/auth/provider.go`                                                                                         | P                               | lead-security |
| CC6.3 | Authorization (RBAC)                       | **Casbin enforcer (KAI-225)** at `internal/cloud/permissions/` — `enforcer.go`, `model.conf`, `roles.go`, `actions.go`. Fail-closed default. Every decision emits an audit record via `AuditSink.RecordEnforce`. Subjects bound to tenant by construction (`user:<id>@<tenant>`). Integrator sub-reseller narrowing bounded at 32 hops. | `internal/cloud/permissions/README.md`, `enforcer_test.go`. **Not yet on main** — needs lead-security sign-off (task #31). | P                               | lead-security |
| CC6.4 | Physical access                            | AWS data centers (carve-out). Customer-premises Recorders are customer-controlled hardware (CUEC).                                                                                                                                                                                                                                      | Customer-responsibility section needed in CUEC matrix                                                                      | P                               | lead-security |
| CC6.5 | Asset disposal                             | Cloud: AWS media destruction (inherited). On-prem: factory reset wipes encrypted SQLite; master key lives in `mediamtx.yml` (never in `nvr.db`), so losing the config renders the DB useless.                                                                                                                                           | `internal/shared/cryptostore/README.md` §"Threat model"; TODO — write decommissioning runbook                              | G                               | lead-onprem   |
| CC6.6 | External access boundary                   | TLS everywhere; mTLS on mesh (`internal/shared/mesh/`); `internal/restrictnetwork/` hardening; HMAC-SHA256 signed outbound webhooks (KAI-397).                                                                                                                                                                                          | `internal/shared/mesh/`, `internal/restrictnetwork/`, KAI-397 WIP                                                          | P                               | lead-onprem   |
| CC6.7 | Data in transit                            | TLS 1.2+ enforced end-to-end. Certificate management via `internal/shared/auth/certmgr/`. Column encryption at rest via cryptostore (KAI-251).                                                                                                                                                                                          | `internal/certloader/`, `internal/shared/auth/certmgr/`, `internal/shared/cryptostore/README.md`                           | P                               | lead-onprem   |
| CC6.8 | Malware / unauthorized software            | EV-signed Windows installer + .deb/.rpm (KAI-340, in progress); cosign-signed container images (KAI-428); reproducible builds; SBOM for vulnerability matching.                                                                                                                                                                         | KAI-428 commit `5792529c`, KAI-340 WIP                                                                                     | P                               | lead-sre      |

### CC7 — System Operations

| ID    | Title                          | Control description                                                                                                                             | Evidence                                                                                  | Status | Owner                    |
| ----- | ------------------------------ | ----------------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------- | ------ | ------------------------ |
| CC7.1 | Detection of vulnerabilities   | `govulncheck` in CI, Trivy/Grype container scanning on image build, Dependabot/Renovate on dependencies, monthly triage cadence.                | TODO — KAI-428 SBOM is the upstream prerequisite and has landed; scanner wiring is gap #4 | G      | lead-sre                 |
| CC7.2 | Anomaly detection & monitoring | Audit log (KAI-233) is the substrate; Prometheus + Grafana (KAI-422) for infra metrics; alerting rules TODO.                                    | KAI-233 (wave1), KAI-422 (worktree)                                                       | P      | lead-sre / lead-security |
| CC7.3 | Incident response              | Written IR plan: severity ladder, paging path (KAI-408 PagerDuty/Opsgenie), customer notification SLAs, post-mortem template, on-call rotation. | **None written** — gap #3                                                                 | G      | lead-security            |
| CC7.4 | Incident recovery              | Auto-recovery + crash recovery on Recorder (KAI-341, in progress). Cloud DB automated backups via RDS (default); restore never exercised.       | KAI-341, RDS console                                                                      | P      | lead-onprem / lead-sre   |
| CC7.5 | Recovery from disruption       | Business continuity / disaster recovery plan + annual tabletop. Multi-region readiness called out as goal in KAI-215 but not delivered.         | **None written** — gap #8                                                                 | G      | lead-sre                 |

### CC8 — Change Management

| ID    | Title                       | Control description                                                                                                                                                                                      | Evidence                                                          | Status | Owner    |
| ----- | --------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------- | ------ | -------- |
| CC8.1 | Authorized & tested changes | All changes via GitHub PR + required review; CI gates (KAI-432 typecheck, KAI-431 proto, KAI-428 build). Branch protection on `main` (TODO confirm settings). Release tagging + cosign artifact signing. | `.github/workflows/`, recent `git log`, KAI-428 commit `5792529c` | P      | lead-sre |

### CC9 — Risk Mitigation

| ID    | Title                               | Control description                                                                                                                                               | Evidence                               | Status | Owner                   |
| ----- | ----------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------- | ------ | ----------------------- |
| CC9.1 | Business-disruption risk mitigation | Cyber-liability insurance (TODO). DR plan (TODO — gap #8). BCP tabletop (TODO).                                                                                   | None                                   | G      | founder / lead-security |
| CC9.2 | Vendor & business-partner risk      | Sub-service carve-out matrix (§1.3). Vendor security review process: collect SOC 2 / ISO 27001 reports, BAAs where PHI is present, DPAs where EU data is present. | TODO — no review process yet — gap #10 | G      | lead-security           |

---

## 4. Gap list — ranked by GA-blocking severity

GA-blocking means: shipping without this materially increases breach risk
**or** makes the SOC 2 audit unachievable on the planned timeline. Effort:
S ≤ 1 wk, M = 1–4 wk, L > 4 wk.

| #   | Gap                                                                                                                                                                                                                            | GA-blocking?                           | Owner                       | Effort | Notes                                                                                                                               |
| --- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | -------------------------------------- | --------------------------- | ------ | ----------------------------------------------------------------------------------------------------------------------------------- |
| 1   | **Land KAI-225 (Casbin) + KAI-233 (audit log) to `main`.** Currently both live on `wave1-integration`. Without them, CC4.1 / CC6.3 / CC7.2 regress from P to G.                                                                | **YES**                                | lead-security               | M      | Highest leverage single item — lands three CC rows simultaneously. Tasks #25, #31.                                                  |
| 2   | **Written security policy set (CC5.3).** AUP, access control, change management, incident response, vendor management, data classification, encryption, BCP/DR, secrets management. Auditor asks on day one.                   | **YES**                                | lead-security               | M      | Phase 4 of this program (sub-ticket KAI-441). Use Vanta/Drata/Strike Graph template library as starting point. Target: 30 policies. |
| 3   | **Incident response runbook + on-call rotation (CC7.3).** Severity ladder, paging path via KAI-408, customer-notification SLAs, post-mortem template, tabletop.                                                                | **YES**                                | lead-security + lead-sre    | M      | Blocks _irrespective_ of SOC 2.                                                                                                     |
| 4   | **Vulnerability management program (CC7.1).** `govulncheck` in CI, Trivy/Grype on images, Dependabot on deps, monthly triage cadence, SLA for patching (critical 7d / high 30d).                                               | **YES**                                | lead-sre                    | S      | Upstream KAI-428 SBOM landed; scanner wiring is cheap.                                                                              |
| 5   | **HR controls (CC1.1 / CC1.4 / CC1.5).** Background-check policy, signed AUP at hire, annual security-awareness training, security responsibilities in job descriptions.                                                       | **YES**                                | lead-people / founder       | M      | Cheap to fix, embarrassing at audit.                                                                                                |
| 6   | **Pen test (KAI-390) scoping and scheduling.** Not strictly required by SOC 2 but auditors expect it and we promised it GA-gating.                                                                                             | **YES**                                | lead-security               | L      | 4–8 wk lead time + 2 wk fieldwork + remediation. Task #44.                                                                          |
| 7   | **Zitadel deployment (KAI-220 / KAI-221).** Until production IdP exists, CC6.1 / CC6.2 evidence is "interface + fake" only — an auditor cannot sample real logins.                                                             | **YES**                                | lead-cloud                  | M      | Currently only `provider.go` interface + `fake/` implementation.                                                                    |
| 8   | **BCP / DR plan + annual tabletop (CC7.5 / CC9.1).** Documented RPO/RTO targets for cloud control plane and on-prem Recorder; restore exercised at least once from cold RDS backup.                                            | **YES**                                | lead-sre                    | M      | Open question #3: what RPO/RTO do we commit to?                                                                                     |
| 9   | **Formal risk register (CC3.1 / CC3.2 / CC3.3).** Single spreadsheet or GRC entry per identified risk with likelihood/impact/owner/mitigation/status. Quarterly review.                                                        | **YES**                                | lead-security               | S      | Phase 2 deliverable (sub-ticket KAI-439).                                                                                           |
| 10  | **Vendor management program (CC9.2).** SOC 2 / ISO 27001 report intake for every sub-service in §1.3; BAAs where PHI (HIPAA customers); DPAs where EU data.                                                                    | **YES**                                | lead-security               | M      | Overlaps with HIPAA-ready work (task #47).                                                                                          |
| 11  | **Formal access-review cadence (CC6.2 / CC6.3).** Quarterly review of all cloud IAM, Zitadel admin roles, Casbin role bindings, GitHub org access, AWS IAM. Documented approval.                                               | **YES**                                | lead-security               | S      | Must begin on day 1 of Type II observation window.                                                                                  |
| 12  | **Logical-access provisioning / deprovisioning procedure.** Documented flow for joiner/mover/leaver across Zitadel, AWS, GitHub, Linear, Slack, 1Password. Same-day deprovisioning SLA.                                        | **YES**                                | lead-people / lead-security | S      | Auditors sample hires and terminations.                                                                                             |
| 13  | **Asset inventory (CC6.5).** Employee laptops, cloud infra (partial — Terraform), SaaS apps, data stores. Single source of truth.                                                                                              | YES                                    | lead-sre                    | M      | GRC platform (open question #4) typically handles this.                                                                             |
| 14  | **MDM + endpoint protection for employee laptops.** Full-disk encryption enforced, screen lock policy, OS patch compliance, malware protection (e.g. Kandji/Jamf + CrowdStrike/SentinelOne).                                   | YES                                    | lead-sre / founder          | M      | Auditors will test a sample of employee laptops.                                                                                    |
| 15  | **Secrets management policy + implementation.** Production secrets in AWS Secrets Manager / SSM / Vault — not committed, not in `.env`, not pasted to Slack. Rotation policy. `nvrJWTSecret` handling specifically called out. | YES                                    | lead-sre / lead-security    | M      | Overlaps with cryptostore master-key handling (KAI-252 pending).                                                                    |
| 16  | **Data classification policy.** Defines Public / Internal / Confidential / Restricted tiers, examples per tier, handling rules (encryption, access, retention, disposal).                                                      | YES                                    | lead-security               | S      | Prereq for Confidentiality TSC in Type II.                                                                                          |
| 17  | **Change advisory / prod-change approval trail (CC8.1).** Beyond PR review: prod deploys logged with approver, rollback plan, post-deploy verification. Branch protection on `main` confirmed.                                 | YES                                    | lead-sre                    | S      | GitHub deployment environments + required reviewers covers most of this.                                                            |
| 18  | **Acceptable use policy + signed acknowledgment at hire.**                                                                                                                                                                     | NO (post-GA acceptable if others slip) | lead-people                 | S      | Ships inside the policy pack (gap #2).                                                                                              |
| 19  | **Trust center (KAI-394) + `security@` inbox + responsible disclosure (CC2.3).** Current `SECURITY.md` only points at upstream mediamtx.org.                                                                                   | NO                                     | lead-security               | S      | Can land GA + 30 if schedule tightens.                                                                                              |
| 20  | **Asset-disposal procedure for on-prem Recorders (CC6.5).** Factory-reset flow that cryptographically guarantees `nvr.db` + cryptostore master are unrecoverable.                                                              | NO                                     | lead-onprem                 | S      | Cryptostore threat model already addresses the crypto side; need the runbook.                                                       |

---

## 5. Auditor selection criteria

Do **not** pick a firm during Phase 1. Evaluate against these criteria when
Phase 5 (KAI-442) begins:

### 5.1 Criteria

1. **AICPA member firm in good standing.** Verify at
   `https://us.aicpa.org/forthepublic/findacpa.html`. Only licensed CPA
   firms can issue SOC reports — reject anyone who is not.
2. **Multi-tenant SaaS experience.** At least 5 Type II reports signed for
   multi-tenant SaaS in the last 24 months. Ask for redacted client names
   and report sizes.
3. **Prior SOC 2 + HIPAA dual reports.** We will need HIPAA-readiness
   (task #47) for medical-vertical customers. One firm doing both is
   materially cheaper and avoids contradictory advice.
4. **Go / cloud-native stack familiarity.** They should not blink at "EKS",
   "Casbin", "ABAC", "mTLS", "cosign", "SBOM". Bonus: prior video /
   surveillance / VMS clients.
5. **Cost band.**
   - **Type I:** $25k–$60k typical for our company size. Target $25k–$40k.
   - **Type II:** $60k–$120k typical. Target $60k–$90k for the first report.
   - Anything materially above the top of the band indicates over-scoping or
     boutique premium pricing we are not ready to pay.
6. **Timeline fit.** Can they start fieldwork within 6 weeks of contract
   and deliver the Type I report within 4 weeks of fieldwork close? GA
   date depends on this.
7. **Tooling neutrality.** Will they accept evidence from whichever GRC
   platform we choose (Vanta / Drata / Secureframe / Thoropass) without
   forcing a specific integration?
8. **Re-test policy on remediation.** Cost of re-evidencing a control that
   fails on first pass should be published, not negotiated after the fact.
9. **References.** Three references from Series-A, <50-employee SaaS clients
   who completed both Type I and Type II with the same firm.

### 5.2 Candidate firms for lead-security to evaluate

In no particular order — lead-security to RFP 4 of these and shortlist 2:

- **Prescient Assurance** — Affordable, strong Type I turnaround for startups; broad SaaS book.
- **Johanson Group** — Startup-friendly pricing; fast fieldwork; common choice for pre-Series-B.
- **Insight Assurance** — Multi-framework (SOC 2 + HIPAA + ISO 27001); growing SaaS practice.
- **A-LIGN** — Large firm, premium pricing, strong brand recognition with enterprise buyers. Consider if enterprise sales cycle demands a recognizable auditor name.
- **Schellman** — Largest pure-play cybersecurity assessor; top-tier brand; expensive; best for post-Series-B scale.
- **KirkpatrickPrice** — Full-service; known for video-content / training library; dual SOC 2 + HIPAA common.

Do not engage more than 4 at a time — RFP fatigue wastes lead-security
cycles.

---

## 6. Type I → Type II roadmap

### 6.1 What Type I actually is

Type I is a **point-in-time** opinion: _"as of date X, the described controls
were suitably designed and implemented to meet the applicable trust services
criteria."_ It does not test operating effectiveness over time. It is faster
and cheaper but less valuable to enterprise buyers — most enterprise
procurement teams accept Type I only as a 6-month bridge to Type II.

### 6.2 What Type II requires

Type II is an **observation-window** opinion: _"over the period [start, end],
the controls operated effectively."_ Minimum window is **3 months**,
enterprise-standard is **6 months**, gold-standard is **12 months**.

The auditor samples evidence from across the window — access reviews from
each quarter, incident tickets from each month, change-management evidence
from each release, vulnerability-scan results from each cycle. A control
that works on day 1 but stops generating logs on day 30 is a finding. This
is why gap #1 (landing KAI-225 + KAI-233) is the highest-leverage item in
the entire list: the audit log underwrites the evidence trail for almost
every other control.

### 6.3 Timing arithmetic

- **Type I at GA:** feasible if fieldwork closes ~1 month before GA. Means
  gap remediation + readiness assessment must finish ~3 months before GA.
- **Type II at GA:** **not feasible** unless we started observation ~6 months
  ago, which we did not. Any claim otherwise is wishful thinking.
- **Type II at GA + 6 months:** feasible if Type II observation window
  starts on the same day Type I fieldwork closes, which is _earlier_ than
  GA day.

### 6.4 Recommended strategy

1. **Target Type I for GA.** Report date ~1 month before GA.
2. **Start Type II observation window on Type I fieldwork close** — not at
   GA. This buys us a month of observation "for free."
3. **Target Type II report at GA + 6 months.** That is the earliest date
   an enterprise prospect requiring Type II can sign a multi-year
   contract — build sales-ops expectations around that number.
4. **Do not promise Type II before GA + 6 months to anyone.** Over-promising
   here is the single most common mistake startups make with SOC 2.

### 6.5 EU AI Act interaction (Aug 2 2026)

SOC 2 is a US framework and is not legally required for EU AI Act
compliance (KAI-282 / KAI-294). But the evidence base is ~70% shared —
audit log, access control, change management, risk assessment, vendor
management. Treat SOC 2 and the EU AI Act conformity assessment as **one
evidence pipeline, two reports**. Do not duplicate control work.

---

## 7. Phase plan reference (KAI-438..442)

This document is the deliverable for **Phase 1 of 5** of the SOC 2 Type I
program (KAI-385, task #34).

| Phase | Sub-ticket  | Scope                         | Primary deliverable                                                                                                                                                             | Section(s) of this doc                    |
| ----- | ----------- | ----------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------- |
| **1** | **KAI-438** | Scoping + TSC selection       | This control matrix + gap analysis (draft)                                                                                                                                      | §1, §2, §5, §6, §7                        |
| 2     | KAI-439     | Control design                | Per-criterion control narratives; formal risk register; updated matrix status column                                                                                            | §3 (depth), gap #9                        |
| 3     | KAI-440     | Control implementation        | Land KAI-225 + KAI-233 to `main`; wire `govulncheck` + Trivy; ship IR runbook; enable branch protection; MDM rollout                                                            | Gaps #1, #3, #4, #11, #12, #14, #17       |
| 4     | KAI-441     | Policy documents (30)         | Full policy set: AUP, access control, change management, IR, vendor mgmt, data classification, encryption, BCP/DR, secrets mgmt, background check, onboarding/offboarding, etc. | Gap #2 (and dependents #5, #15, #16, #18) |
| 5     | KAI-442     | Auditor engagement + snapshot | Signed engagement letter; readiness assessment; Type I fieldwork; report delivery                                                                                               | §5 (execute), §6 (execute)                |

Phase 1 exit criteria: this document reviewed and signed off by
lead-security and founder; §"Open questions" reduced to zero; Linear
sub-tickets KAI-438..442 created and linked to KAI-385.

---

## Open questions for lead-security (must be resolved before Phase 2)

These items could not be determined from the codebase. Each needs a human
decision before the matrix can be promoted out of draft.

1. **Is the on-prem Recorder in-scope as a service-org service, or is it a
   customer-deployed product?** Biggest single scope question. If in scope,
   every customer site is inside the SOC 2 boundary — adds physical-access
   controls and per-site inventory. If it is a customer product, only the
   cloud control plane is in scope and the Recorder becomes a CUEC. §1.1
   currently lists it in scope; confirm before any auditor conversation.
2. **Zitadel hosting — self-hosted on our EKS, or Zitadel Cloud?** Changes
   the sub-service carve-out in §1.3 and the CC6.1 evidence story
   materially. Decision owner: lead-cloud + lead-security.
3. **Documented RPO and RTO for cloud control plane and on-prem Recorder.**
   Without targets, CC7.4 / CC7.5 / CC9.1 cannot be evidenced and gap #8
   cannot be closed. Proposed default: RPO 15 min (cloud) / 24 h
   (on-prem), RTO 4 h (cloud) / 24 h (on-prem). Confirm or override.
4. **GRC platform choice.** Vanta vs. Drata vs. Secureframe vs. Thoropass
   vs. none (roll own on Google Drive). Affects evidence-collection cost,
   auditor-selection criterion #7, and whether gap #13 (asset inventory)
   is free or manual. Decision owner: lead-security + founder.
5. **Trust services categories beyond Security.** §2 currently defers
   Availability and Confidentiality to Type II. If a critical early
   enterprise prospect demands Confidentiality in Type I, the decision
   reverses but only if gap #16 (data classification policy) can be
   closed in time. Who has authority to change this decision?

---

_End of Phase 1 working draft — KAI-438. Do not cite in auditor-facing
materials until §"Open questions" is empty and lead-security signs off._
