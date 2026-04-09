---
title: Human Oversight — Face Recognition
owner: lead-security (process) | lead-ai (product surfaces)
status: draft
last-updated: 2026-04-08
eu-ai-act-article: 14
---

# Human Oversight

Article 14 requires high-risk AI systems to be designed so they "can be
effectively overseen by natural persons" during use. Oversight must be
**meaningful**, not a rubber stamp. This document specifies the UI, the
process, and the monitoring needed to satisfy that requirement for Kaivue's
face recognition feature.

As of 2026-04-08 no UI exists. The surfaces described here are binding
requirements for the implementation tickets (KAI-327 Customer Admin AI
Settings + face vault mgmt, KAI-337 operator workflow).

## Design principles

1. **No autonomous action.** The system surfaces a candidate match. A human
   decides what happens next.
2. **Confidence is always visible.** No match is shown without its score.
3. **Reversibility.** Every operator action can be reviewed, undone, or
   escalated.
4. **Kill switch.** An admin can turn the feature off instantly.
5. **Accountability.** Every human decision is logged with operator identity.

## Operator review queue

Every candidate match produces a queue entry with:

- Timestamp, camera, site
- Live-frame thumbnail
- Enrolment thumbnail of each top-k candidate (k configurable, default 3)
- Confidence score for each candidate, with a visual indicator (e.g. color
  band keyed to the tenant's tuned threshold)
- Context: camera name, location, recent activity
- Action buttons: **Confirm match**, **Reject match**, **Escalate**, **Report
  as false positive**

**Required behaviors:**

- The Confirm and Reject buttons are **equally prominent**. Confirm must not
  be highlighted or default-focused in a way that biases toward acceptance.
- The operator cannot confirm a match without clicking the confirm button. No
  keyboard shortcut that collapses review and confirmation into a single key.
- A minimum interaction time (e.g. 800 ms) before Confirm becomes clickable,
  to prevent reflexive acceptance.
- If the operator clicks Reject, a one-tap reason chip is captured
  (not-the-same-person / too-blurry / other).
- Every action writes an audit log entry per `audit-log-requirements.md`.

## Watchlist management — four-eyes principle

Adding a person to a watchlist is a high-impact action. Required:

1. Operator A (admin role) initiates the addition, providing: identity, source
   photos (min N), reason text, retention override if any.
2. Operator B (a second approver with admin or higher role, distinct from A)
   reviews the request in a pending queue and either approves or rejects.
3. Only after approval does the entry become active in the matcher.
4. Both identities are logged. Both receive email notifications of the
   activation.
5. **Emergency override** is permitted only with a written justification
   captured in the audit log, and must be followed within 24 hours by the
   second-approver review. Emergency-override usage is visible in the tenant
   admin dashboard and counted as a KPI reviewed monthly.

The same four-eyes rule applies to: adding a high-retention override,
changing a tenant-wide confidence threshold below a floor value, and
re-enabling the feature after a kill-switch shutdown.

## Enrollee removal / unenroll

- Any admin operator can unenroll any person in one click.
- Unenroll purges embeddings within 30 seconds from the matcher and schedules
  the long-term purge of backups per retention policy.
- Audit log entry is immutable.
- Persons who have withdrawn consent are added to a soft deny list so
  accidental re-enrollment by a different operator is blocked.

## Kill switch

- A tenant admin can disable face recognition tenant-wide with a single
  confirmation dialog. Disabling takes effect within 10 seconds across all
  Recorders.
- Disabling is audit-logged and emailed to all admins.
- Re-enabling requires four-eyes.
- A per-site or per-camera pause is also available.

## Operator training

See `transparency-and-instructions-for-use.md` Section 8. Summary:

- Minimum 4 hours initial training
- Written test, 80% threshold
- Annual refresher
- Sign-off stored in Admin Console
- No sign-off → no queue access

## Monitoring operators (meta-oversight)

Article 14 requires deployers to be able to monitor operator behavior to
detect automation bias. Kaivue provides:

- **Operator confirmation rate** per operator, with baseline and alerts when
  the rate is > 2 standard deviations above cohort mean (potential
  rubber-stamping).
- **Operator rejection rate** — alert when an operator has never rejected a
  match in N consecutive shifts (may indicate disengagement or bias).
- **Time-to-decision** distribution — alert on suspiciously low medians
  (< minimum interaction floor).
- **Override rate on high-confidence rejections** — alert if an operator
  frequently rejects high-confidence matches, which may indicate either
  miscalibration or discrimination.
- All dashboards visible to the tenant admin. Individual operator data is
  only visible to authorized managers in that tenant.

## Accessibility

The review queue UI must meet WCAG 2.1 AA (see KAI-389). In particular:

- Color is not the sole carrier of confidence information.
- Screen-reader labels on thumbnails, scores, and action buttons.
- Keyboard navigation reaches every control.

## Escalation

- Operator suspects a wrongful match with real-world consequence: **Escalate**
  button routes to the site manager and creates a high-priority incident
  record.
- Site manager can trigger a Kaivue support ticket for model-level review.
- Incidents that caused harm follow the serious-incident reporting process
  in `post-market-monitoring-plan.md`.

## Article 14(4) specific capabilities

Article 14(4) enumerates capabilities oversight persons must have. Mapping:

| 14(4) requirement                     | Kaivue implementation                                                        |
| ------------------------------------- | ---------------------------------------------------------------------------- |
| Understand capacities and limitations | Ships per `transparency-and-instructions-for-use.md`, reinforced in training |
| Remain aware of automation bias       | Meta-oversight telemetry + training module                                   |
| Interpret output correctly            | Confidence score, top-k candidates, side-by-side thumbnails                  |
| Decide not to use output              | Reject button always present and equal-weighted                              |
| Intervene or interrupt                | Kill switch tenant-wide and per-site                                         |

## TODOs

- [ ] design — mockups for review queue and four-eyes watchlist flow
- [ ] lead-ai — pick the minimum interaction floor and justify
- [ ] lead-security — integrate meta-oversight alerts with KAI-233 audit log
- [ ] lead-security — define the cohort-baseline algorithm for confirmation
      rate alerts (avoid punishing low-traffic tenants with noisy stats)
