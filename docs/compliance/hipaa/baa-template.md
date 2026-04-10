# Business Associate Agreement — Template

> **TEMPLATE — NOT FOR EXECUTION AS-IS.**
> This template requires review by **Kaivue legal counsel** and **customer legal counsel** before execution. It is provided as a starting point derived from the HHS sample Business Associate Agreement and tailored to Kaivue Recording Server. Bracketed fields `[LIKE_THIS]` must be completed. Do not rely on this document as legal advice.

---

**BUSINESS ASSOCIATE AGREEMENT**

This Business Associate Agreement ("**Agreement**" or "**BAA**") is entered into as of **[EFFECTIVE_DATE]** (the "Effective Date") by and between:

**[COVERED_ENTITY_LEGAL_NAME]**, a **[COVERED_ENTITY_STATE_OF_INCORPORATION]** **[COVERED_ENTITY_ENTITY_TYPE]** with its principal place of business at **[COVERED_ENTITY_ADDRESS]** ("**Covered Entity**"); and

**Kaivue, Inc.**, a **[KAIVUE_STATE]** corporation with its principal place of business at **[KAIVUE_ADDRESS]** ("**Business Associate**").

Covered Entity and Business Associate are each a "**Party**" and collectively the "**Parties**."

## Recitals

A. Covered Entity and Business Associate have entered into, or intend to enter into, one or more agreements under which Business Associate provides the Kaivue Recording Server service and related offerings (the "**Underlying Services Agreement**").

B. In the course of providing the services, Business Associate may create, receive, maintain, or transmit Protected Health Information on behalf of Covered Entity.

C. The Parties intend to comply with the Health Insurance Portability and Accountability Act of 1996, as amended by the HITECH Act (collectively, "**HIPAA**"), and the regulations promulgated thereunder at 45 CFR Parts 160 and 164 (the "**HIPAA Rules**").

NOW, THEREFORE, in consideration of the mutual promises below and other good and valuable consideration, the Parties agree as follows.

---

## 1. Definitions

Capitalized terms used but not otherwise defined in this Agreement shall have the meanings ascribed to them in the HIPAA Rules. For convenience:

1.1 **"Breach"** has the meaning given at 45 CFR §164.402.

1.2 **"Business Associate"** has the meaning given at 45 CFR §160.103 and in this Agreement refers to Kaivue, Inc.

1.3 **"Covered Entity"** has the meaning given at 45 CFR §160.103 and in this Agreement refers to the entity named above.

1.4 **"Designated Record Set"** has the meaning given at 45 CFR §164.501.

1.5 **"Electronic Protected Health Information"** or **"ePHI"** has the meaning given at 45 CFR §160.103, limited to information that Business Associate creates, receives, maintains, or transmits on behalf of Covered Entity.

1.6 **"HIPAA Rules"** means the Privacy, Security, Breach Notification, and Enforcement Rules at 45 CFR Part 160 and Part 164.

1.7 **"Individual"** has the meaning given at 45 CFR §160.103 and includes a person who qualifies as a personal representative under 45 CFR §164.502(g).

1.8 **"Privacy Rule"** means the Standards for Privacy of Individually Identifiable Health Information at 45 CFR Part 160 and Part 164, Subparts A and E.

1.9 **"Protected Health Information"** or **"PHI"** has the meaning given at 45 CFR §160.103, limited to information that Business Associate creates, receives, maintains, or transmits on behalf of Covered Entity. For the avoidance of doubt, video and audio recordings of patient-facing areas within Covered Entity's facilities captured by the Kaivue Recording Server are PHI when they identify, or can reasonably be used to identify, an Individual.

1.10 **"Required By Law"** has the meaning given at 45 CFR §164.103.

1.11 **"Secretary"** means the Secretary of the U.S. Department of Health and Human Services or the Secretary's designee.

1.12 **"Security Incident"** has the meaning given at 45 CFR §164.304.

1.13 **"Security Rule"** means the Security Standards for the Protection of Electronic Protected Health Information at 45 CFR Part 160 and Part 164, Subparts A and C.

1.14 **"Subcontractor"** has the meaning given at 45 CFR §160.103.

1.15 **"Unsecured PHI"** has the meaning given at 45 CFR §164.402.

---

## 2. Obligations and Activities of Business Associate

2.1 **Limits on use and disclosure.** Business Associate shall not use or disclose PHI other than as permitted or required by this Agreement, the Underlying Services Agreement, or as Required By Law.

2.2 **Safeguards.** Business Associate shall use appropriate administrative, physical, and technical safeguards, and shall comply with the Security Rule with respect to ePHI, to prevent use or disclosure of PHI other than as provided for by this Agreement. Without limiting the foregoing, Business Associate's safeguards are described in its HIPAA architecture document (currently `docs/compliance/hipaa/architecture.md`), which is incorporated by reference. Business Associate shall not materially degrade those safeguards during the term of this Agreement.

2.3 **Mitigation.** Business Associate shall mitigate, to the extent practicable, any harmful effect that is known to Business Associate of a use or disclosure of PHI by Business Associate in violation of this Agreement.

2.4 **Reporting.**
   (a) Business Associate shall report to Covered Entity any use or disclosure of PHI not provided for by this Agreement of which it becomes aware, including Breaches of Unsecured PHI as required at 45 CFR §164.410, and any Security Incident of which it becomes aware.
   (b) **Timing.** Business Associate shall report a Breach without unreasonable delay and in no case later than **sixty (60) calendar days** after discovery, consistent with 45 CFR §164.410. Business Associate commits to a **fifteen (15) calendar day** internal target for initial notification to Covered Entity's designated contact, subject to the needs of law enforcement and forensic preservation.
   (c) **Content of notification.** Notification shall include, to the extent known: (i) identification of each Individual whose Unsecured PHI has been, or is reasonably believed to have been, accessed, acquired, used, or disclosed during the Breach; (ii) a description of what happened, including dates; (iii) the types of Unsecured PHI involved; (iv) steps taken to investigate, mitigate, and prevent recurrence; and (v) contact information for follow-up.
   (d) **Unsuccessful Security Incidents.** Covered Entity acknowledges that unsuccessful Security Incidents (pings, port scans, denied access attempts, and similar) occur continuously. Business Associate shall not be required to report such unsuccessful incidents individually; an annual summary satisfies the reporting obligation for these.
   (e) **Joint investigation.** The Parties shall cooperate in good faith to investigate Breaches, preserve evidence, and determine root cause.

2.5 **Subcontractors.** In accordance with 45 CFR §164.502(e)(1)(ii) and §164.308(b)(2), Business Associate shall ensure that any Subcontractor that creates, receives, maintains, or transmits PHI on behalf of Business Associate agrees in writing to restrictions and conditions at least as restrictive as those that apply to Business Associate under this Agreement. Business Associate shall maintain a current list of such Subcontractors and shall make it available to Covered Entity on reasonable request. The current list as of the Effective Date is attached as **Exhibit A**.

2.6 **Access to PHI.** To the extent Business Associate maintains PHI in a Designated Record Set, Business Associate shall make such PHI available to Covered Entity within **[15] days** of a written request, so that Covered Entity may meet its obligations under 45 CFR §164.524.

2.7 **Amendment of PHI.** To the extent Business Associate maintains PHI in a Designated Record Set, Business Associate shall make amendments to PHI as directed or agreed to by Covered Entity pursuant to 45 CFR §164.526 within **[30] days** of a written request. Covered Entity acknowledges that amendment of video recordings is technically infeasible without destroying the underlying evidentiary value; in such cases the Parties shall append an amendment notice rather than alter the recording.

2.8 **Accounting of disclosures.** Business Associate shall maintain, and make available to Covered Entity on request, information required to provide an accounting of disclosures of PHI in accordance with 45 CFR §164.528. Business Associate's audit log supports this obligation.

2.9 **Governmental access.** Business Associate shall make its internal practices, books, and records, including policies and procedures and PHI, relating to the use and disclosure of PHI received from, or created or received by Business Associate on behalf of, Covered Entity, available to the Secretary for purposes of determining Covered Entity's compliance with the HIPAA Rules.

2.10 **Minimum necessary.** Business Associate shall limit its requests for, use, and disclosure of PHI to the minimum necessary to accomplish the intended purpose of the use, disclosure, or request, in accordance with 45 CFR §164.502(b) and §164.514(d).

2.11 **Compliance with Covered Entity's obligations.** To the extent Business Associate is to carry out one or more of Covered Entity's obligations under Subpart E of 45 CFR Part 164, Business Associate shall comply with the requirements of Subpart E that apply to Covered Entity in the performance of such obligations.

---

## 3. Permitted Uses and Disclosures by Business Associate

3.1 **Service delivery.** Business Associate may use and disclose PHI as necessary to perform the services set forth in the Underlying Services Agreement.

3.2 **Management and administration.** Business Associate may use PHI for its proper management and administration, or to carry out Business Associate's legal responsibilities, provided that any disclosure for such purposes is either Required By Law or subject to reasonable assurances from the recipient that the PHI will be held confidentially, used or further disclosed only as Required By Law or for the purposes for which it was disclosed, and any Breach of confidentiality is reported to Business Associate.

3.3 **Data aggregation.** Business Associate may provide data aggregation services relating to the health care operations of Covered Entity as permitted by 45 CFR §164.504(e)(2)(i)(B).

3.4 **De-identification.** Business Associate may de-identify PHI in accordance with 45 CFR §164.514(a)-(c). De-identified data is not PHI and is not subject to this Agreement.

3.5 **Prohibited uses.** Business Associate shall not:
   (a) Use or disclose PHI in a manner that would violate Subpart E of 45 CFR Part 164 if done by Covered Entity, except as permitted in 3.2 and 3.3;
   (b) Sell PHI or use PHI for marketing in violation of 45 CFR §164.502(a)(5); or
   (c) Use PHI to train machine learning models for purposes unrelated to the services provided to Covered Entity, without Covered Entity's prior written consent.

---

## 4. Obligations of Covered Entity

4.1 **Notice of Privacy Practices.** Covered Entity shall notify Business Associate of any limitation in its Notice of Privacy Practices under 45 CFR §164.520 that may affect Business Associate's use or disclosure of PHI.

4.2 **Changes in permission.** Covered Entity shall notify Business Associate of any changes in, or revocation of, the permission by an Individual to use or disclose PHI, to the extent such changes may affect Business Associate's use or disclosure of PHI.

4.3 **Restrictions.** Covered Entity shall notify Business Associate of any restriction on the use or disclosure of PHI that Covered Entity has agreed to in accordance with 45 CFR §164.522, to the extent such restriction may affect Business Associate's use or disclosure of PHI.

4.4 **Permissible requests.** Covered Entity shall not request Business Associate to use or disclose PHI in any manner that would not be permissible under the HIPAA Rules if done by Covered Entity, except that Business Associate may use and disclose PHI for the purposes of management, administration, and data aggregation as set forth in §3.

4.5 **Configuration and use.** Covered Entity is responsible for configuring the Kaivue Recording Server appropriately for its environment, including but not limited to camera placement, retention policies, user role assignments, and physical security of on-premises Recorders.

---

## 5. Term and Termination

5.1 **Term.** This Agreement shall be effective as of the Effective Date and shall terminate on the earlier of (a) termination of the Underlying Services Agreement or (b) termination for cause as authorized in §5.2.

5.2 **Termination for cause.** Upon Covered Entity's knowledge of a material breach of this Agreement by Business Associate, Covered Entity shall provide written notice of the breach and an opportunity for Business Associate to cure within **thirty (30) days**. If Business Associate does not cure the breach within the cure period, Covered Entity may terminate this Agreement and the Underlying Services Agreement. If cure is not feasible, Covered Entity may terminate immediately upon written notice.

5.3 **Return or destruction of PHI.**
   (a) Upon termination of this Agreement, Business Associate shall, if feasible, return or destroy all PHI received from Covered Entity, or created, maintained, or received by Business Associate on behalf of Covered Entity, that Business Associate still maintains in any form. Business Associate shall retain no copies of the PHI.
   (b) **Infeasibility.** In the event that Business Associate determines that returning or destroying the PHI is infeasible (for example, because it is commingled with operational backups or subject to a litigation hold), Business Associate shall provide to Covered Entity notification of the conditions that make return or destruction infeasible. Business Associate shall extend the protections of this Agreement to such PHI and limit further uses and disclosures of such PHI to those purposes that make the return or destruction infeasible, for so long as Business Associate maintains such PHI.
   (c) **Timing.** Feasible return or destruction shall be completed within **[60] days** of termination. Business Associate shall provide a written certification of destruction on request.

5.4 **Survival.** The obligations of Business Associate under §5.3 shall survive the termination of this Agreement.

---

## 6. Indemnification and Liability

> The following terms are proposed as commercially reasonable. Both Parties should confirm with counsel and align with the Underlying Services Agreement.

6.1 **Indemnification by Business Associate.** Business Associate shall indemnify and hold harmless Covered Entity from and against any third-party claims, losses, and expenses (including reasonable attorneys' fees) to the extent arising from Business Associate's material breach of this Agreement or its negligent or willful misconduct in handling PHI.

6.2 **Indemnification by Covered Entity.** Covered Entity shall indemnify and hold harmless Business Associate from and against any third-party claims, losses, and expenses (including reasonable attorneys' fees) to the extent arising from Covered Entity's material breach of this Agreement or its negligent or willful misconduct.

6.3 **Liability cap.** Notwithstanding anything to the contrary in the Underlying Services Agreement, each Party's aggregate liability under this Agreement and the Underlying Services Agreement combined shall not exceed **the greater of (a) twelve (12) months of fees paid or payable by Covered Entity to Business Associate under the Underlying Services Agreement preceding the event giving rise to liability, or (b) [USD_CAP_FLOOR]**. This cap is mutual.

6.4 **Excluded damages.** Neither Party shall be liable for indirect, incidental, consequential, special, or punitive damages, except that this exclusion shall not apply to (i) a Party's indemnification obligations, (ii) breach of confidentiality obligations, or (iii) liability that cannot be limited by law.

6.5 **Regulatory fines.** Regulatory fines and penalties imposed directly on a Party shall be borne by that Party, except where the fine arises from the other Party's material breach of this Agreement, in which case the breaching Party shall reimburse up to the liability cap in §6.3.

---

## 7. Miscellaneous

7.1 **Regulatory references.** A reference in this Agreement to a section in the HIPAA Rules means the section as in effect or as amended.

7.2 **Amendment.** The Parties agree to take such action as is necessary to amend this Agreement from time to time as is necessary for Business Associate or Covered Entity to comply with the requirements of the HIPAA Rules and any other applicable law.

7.3 **Survival.** The respective rights and obligations of Business Associate under §5.3 (Return or Destruction of PHI) of this Agreement shall survive the termination of this Agreement.

7.4 **Interpretation.** Any ambiguity in this Agreement shall be resolved in favor of a meaning that permits the Parties to comply with the HIPAA Rules.

7.5 **No third-party beneficiaries.** Nothing in this Agreement shall confer upon any person other than the Parties and their respective successors or assigns, any rights, remedies, obligations, or liabilities whatsoever.

7.6 **Order of precedence.** In the event of a conflict between this Agreement and the Underlying Services Agreement with respect to the handling of PHI, this Agreement controls.

7.7 **Governing law.** This Agreement shall be governed by the laws of **[GOVERNING_LAW_STATE]**, without regard to conflict-of-laws principles, except to the extent preempted by federal law.

7.8 **Notices.** Notices under this Agreement shall be in writing and delivered to:

   **Covered Entity:** [COVERED_ENTITY_NOTICE_CONTACT] — [COVERED_ENTITY_NOTICE_EMAIL] — [COVERED_ENTITY_NOTICE_ADDRESS]

   **Business Associate:** Kaivue Privacy Officer — privacy@kaivue.com — [KAIVUE_NOTICE_ADDRESS]

7.9 **Counterparts; electronic signature.** This Agreement may be executed in counterparts and by electronic signature, each of which shall be deemed an original and all of which together shall constitute one agreement.

7.10 **Entire agreement.** This Agreement, together with the Underlying Services Agreement, constitutes the entire agreement between the Parties with respect to the subject matter hereof.

---

## Signatures

**COVERED ENTITY**
[COVERED_ENTITY_LEGAL_NAME]

By: _______________________________
Name: [SIGNER_NAME]
Title: [SIGNER_TITLE]
Date: _______________________________

**BUSINESS ASSOCIATE**
Kaivue, Inc.

By: _______________________________
Name: [KAIVUE_SIGNER_NAME]
Title: [KAIVUE_SIGNER_TITLE]
Date: _______________________________

---

## Exhibit A — Current Subcontractors with PHI Access

As of the Effective Date, Business Associate engages the following Subcontractors that may create, receive, maintain, or transmit PHI on behalf of Business Associate. Each has a written agreement flowing down the obligations of this BAA.

| Subcontractor | Service Provided | Nature of PHI Access | BAA Status |
| --- | --- | --- | --- |
| Amazon Web Services, Inc. | Cloud infrastructure (EKS, RDS, S3, KMS) for the Kaivue Directory control plane, and optional cloud archive of recordings | Control plane metadata; ciphertext of opt-in cloud-archived recordings | AWS BAA executed |
| Cloudflare, Inc. | R2 object storage for opt-in cold archive of recordings | Ciphertext only (client-side encrypted before upload) | Cloudflare BAA executed (R2 scope) |
| Zitadel (hosting entity **[ZITADEL_ENTITY]**) | Identity provider | Subject identifiers, authentication metadata; no clinical content | BAA [STATUS] |

**Not a Subcontractor under this Agreement (no PHI access):**

| Vendor | Service | Rationale |
| --- | --- | --- |
| Stripe, Inc. | Billing and payment processing | Receives only Covered Entity's organizational billing details (company name, seat count, payment method). Does not receive PHI. |
| **[OBSERVABILITY_VENDOR]** | Application observability | Logs are scrubbed to exclude PHI before export; vendor is not in the PHI data path. BAA will be executed before the vendor is moved into any PHI-bearing workflow. |

Business Associate shall update this Exhibit as its Subcontractor roster changes and shall provide Covered Entity with the updated Exhibit on reasonable request. Material changes affecting Subcontractors that handle PHI shall be communicated to Covered Entity **at least [30] days** in advance where practicable.

---

## Exhibit B — Designated Contacts

**Covered Entity Privacy / Security Officer:** [CE_PRIVACY_OFFICER_NAME], [CE_PRIVACY_OFFICER_EMAIL], [CE_PRIVACY_OFFICER_PHONE]

**Covered Entity Breach Notification Contact (24x7):** [CE_BREACH_CONTACT]

**Kaivue Privacy Officer:** [KAIVUE_PRIVACY_OFFICER_NAME], privacy@kaivue.com

**Kaivue Security Incident Hotline (24x7):** [KAIVUE_SECURITY_HOTLINE]

---

*End of Template. Bracketed fields must be completed and both Parties' counsel must review before execution.*
