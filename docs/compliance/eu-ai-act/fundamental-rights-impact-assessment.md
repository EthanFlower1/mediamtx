# Fundamental Rights Impact Assessment (Template for Deployers)

**Article:** 27
**Status:** Template — Kaivue provides, customer completes
**Owner:** lead-ai (template) / customer (completion)

## Purpose

Art. 27 requires deployers that are bodies governed by public law, or private entities providing public services, and deployers operating high-risk AI systems listed in Annex III §5(b) and §5(c), to carry out a fundamental rights impact assessment (FRIA) prior to deploying the system.

Kaivue (as provider) is not itself subject to Art. 27, but many of our customers are. Kaivue provides this template so that customers can complete their FRIA efficiently using information that only Kaivue can supply (model characteristics, fairness metrics, known limitations).

Customers MUST complete their own FRIA. Kaivue does not and cannot complete it for the customer — the deployment context is customer-specific and the assessment requires the customer's judgment about their own use.

## Template

### 1. Description of the deployment

- **Deployer identity.** Name, address, contact.
- **Purposes for which the system will be used.** In plain language.
- **Categories of natural persons affected.** Who will be subject to face recognition? Staff? Visitors? Customers? General public?
- **Specific locations.** Where are the cameras placed? Are any locations publicly accessible within the meaning of the AI Act?
- **Duration of use.** Continuous, scheduled, event-driven?

### 2. Description of the system from the provider

Kaivue supplies for each active model:

- Intended purpose statement (see `transparency-and-information.md`).
- Declared accuracy at the operating threshold.
- Demographic fairness metrics (from `fairness-testing-protocol.md`).
- Known limitations and failure modes.
- Human oversight controls available in the product.

Customers copy or reference this information in their FRIA.

### 3. Fundamental rights at risk

For each category of natural person affected, the deployer assesses the risk to the following fundamental rights as protected by the EU Charter:

- **Human dignity** (Art. 1).
- **Right to integrity of the person** (Art. 3).
- **Respect for private and family life** (Art. 7).
- **Protection of personal data** (Art. 8).
- **Freedom of thought, conscience, and religion** (Art. 10).
- **Freedom of expression and information** (Art. 11).
- **Freedom of assembly and association** (Art. 12).
- **Right to non-discrimination** (Art. 21).
- **Rights of the child** (Art. 24).
- **Right to good administration** (Art. 41).
- **Right to an effective remedy and a fair trial** (Art. 47).
- **Presumption of innocence** (Art. 48).

### 4. Specific categories of persons likely to be affected

Particular attention is given to:

- Children (Art. 24 Charter and Art. 9(9) AI Act).
- Vulnerable groups, including those likely to be underrepresented in training data.
- Persons with disabilities.
- Persons in protected-characteristic groups likely to experience disparate error rates.

### 5. Risks of harm

For each identified risk, the deployer documents:

- Nature of the potential harm (e.g. wrongful identification, chilling effect on assembly, disparate error rate).
- Likelihood.
- Severity.
- Reversibility.
- Cumulative impact (e.g. repeated identification events for the same individual).

### 6. Governance and human oversight measures at the deployer

The deployer documents how the human oversight controls Kaivue provides will be operationalised in the specific deployment:

- Who are the assigned reviewers?
- What is their competence and training?
- What is the review workload?
- What is the escalation procedure?
- What are the stop-the-system triggers?

### 7. Measures to address identified risks

The deployer documents the mitigations they will put in place beyond Kaivue's product-level controls — e.g. signage, privacy notices, limited retention of match events, restricted access to the review interface, community engagement.

### 8. Complaint mechanism

The deployer documents how affected persons can:

- Learn that face recognition is in use (Art. 26(11) information obligation).
- Request access to data about themselves.
- Request erasure.
- File a complaint.
- Seek redress.

### 9. Notification to the national authority

Art. 27(3) requires the deployer to notify the market surveillance authority of the results of the FRIA in the designated form. Kaivue does not file this notification — the customer does.

### 10. Review and update

The deployer commits to reviewing the FRIA:

- Before any material change in deployment context.
- After any serious incident.
- On a periodic cadence (at minimum annually).

The deployer retains the FRIA for at least the period required under Art. 27 and makes it available to the market surveillance authority on request.

## How Kaivue supports the deployer

- `transparency-and-information.md` provides the provider-side information the deployer needs for sections 2 and 5.
- The admin console (KAI-327) surfaces model version, accuracy, fairness metrics in a form the deployer can screenshot or export into their FRIA.
- The Kaivue trust portal publishes the aggregate package for customers unable to access the admin console.
- The customer agreement commits Kaivue to respond to reasoned information requests related to a customer's FRIA within a stated SLA.

## Interactions with other documents

- `transparency-and-information.md` — source of provider-side information for the deployer FRIA.
- `risk-management-system.md` — Kaivue's own risk analysis is distinct from the deployer FRIA but can be referenced.
- `fairness-testing-protocol.md` — source of fairness metrics for section 2.
