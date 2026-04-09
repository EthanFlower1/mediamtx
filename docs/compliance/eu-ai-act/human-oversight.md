# Human Oversight

**Article:** 14
**Status:** Draft — UI controls tracked under KAI-327
**Owner:** lead-ai + lead-frontend

## Purpose

Art. 14 requires that high-risk AI systems are designed and developed so they can be effectively overseen by natural persons during the period of use, such that risks to health, safety, or fundamental rights are minimised.

## Oversight objectives (Art. 14(4))

Oversight measures enable the natural persons to whom human oversight is assigned to:

1. Understand the capacities and limitations of the system and duly monitor its operation.
2. Remain aware of the tendency of automation bias.
3. Correctly interpret the system's output.
4. Decide, in any particular situation, not to use the system or otherwise disregard, override, or reverse the output.
5. Intervene or interrupt the system through a "stop" button or similar procedure.

## Operator roles

- **Customer admin:** configures which cameras run face recognition, manages the vault, sets thresholds, sees dashboards.
- **Customer reviewer:** receives match alerts, reviews them, decides to confirm or dismiss.
- **Customer auditor:** reads-only access to audit log and match history.

Roles are enforced by Casbin (KAI-225) with documented policies per tenant.

## Controls

### Pre-use understanding (Art. 14(4)(a))

- `transparency-and-information.md` ships with the product and is surfaced in-app.
- KAI-327 admin UI displays model version, accuracy metrics, fairness metrics, and known limitations for every active model.
- First-run activation flow for face recognition requires the customer admin to acknowledge the limitations and intended purpose.

### Automation-bias mitigation (Art. 14(4)(b))

- Match-review UI does NOT default to "confirm." Reviewers must actively choose.
- Confidence scores are always visible; low-confidence matches are flagged as such.
- A periodic "review quality" summary surfaces to customer admins showing the per-reviewer confirm-rate, time-to-review distribution, and agreement with peer reviewers.
- Training material (linked from the admin UI) explicitly addresses automation bias.

### Output interpretation (Art. 14(4)(c))

- Match events include: matched identity, confidence score, candidate frame, timestamp, camera, model version, alternate candidates (top-k).
- The reviewer sees the candidate frame alongside the enrolment image, not just a score.
- Demographic fairness metrics for the active model are one click away from every match review screen.

### Override and disregard (Art. 14(4)(d))

- Reviewers can dismiss, confirm, escalate, or annotate.
- Dismissals feed post-market monitoring as operator feedback (see `post-market-monitoring.md`).
- Customer admins can disable face recognition on any camera at any time, from the admin UI. This is a **single click** — no confirmation dialog, no tier escalation.
- Customer admins can disable face recognition across the entire tenant with a single toggle.

### Stop button (Art. 14(4)(e))

- The per-camera and per-tenant disable toggles above constitute the stop control.
- In addition, a provider-side kill switch allows Kaivue to disable face recognition globally for a tenant in response to a Kaivue-detected serious issue (see `serious-incident-reporting.md`). Use of the provider kill switch is logged and notified to the customer.

## Special measures for real-time live detection

If, after legal review, real-time live detection is enabled in the EU (see the Art. 5(1)(h) open question in `conformity-assessment.md`):

- Live detection is opt-in per camera, default off.
- Every live match requires reviewer confirmation before any automated action.
- Live detection dashboards display review latency so that customer admins can see if reviewers are falling behind.
- Exceeding a review-backlog threshold automatically pauses live detection on the affected cameras with a visible banner.

## Training and documentation

Customers (deployers) are obligated under Art. 26 to ensure their operators have the competence to perform human oversight. Kaivue supports this by:

- Providing operator-facing training material (video + text) covering automation bias, confidence interpretation, and review procedure.
- Providing a certification quiz that customer admins can assign to reviewers.
- Documenting in `transparency-and-information.md` the minimum competence expected of a reviewer.

## Evidence

Human-oversight effectiveness is measured in `post-market-monitoring.md`:

- Per-operator confirm-rate distribution.
- Time-to-review distribution.
- Agreement between reviewers on the same event.
- Override frequency (reviewers disregarding high-confidence matches).

## Interactions with other documents

- `risk-management-system.md` — R7 (failure of human oversight) mitigations are implemented here.
- `transparency-and-information.md` — customer-facing description of oversight controls.
- `post-market-monitoring.md` — measures oversight effectiveness in the field.
