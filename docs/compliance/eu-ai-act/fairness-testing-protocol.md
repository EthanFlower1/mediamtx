---
title: Fairness Testing Protocol — Face Recognition
owner: lead-ai (methodology) | lead-security (sign-off + publication)
status: draft
last-updated: 2026-04-08
eu-ai-act-article: 10, 15
---

# Fairness Testing Protocol

Article 10 requires data governance practices to mitigate bias. Article 15
requires appropriate levels of accuracy, robustness, and cybersecurity
throughout the lifecycle, and that these be declared in the instructions for
use. This protocol specifies **how** Kaivue measures fairness for its face
recognition feature and the **ship / no-ship** criteria derived from it.

## Scope

Applies to:

- Every release-candidate face recognition model
- Every production model, re-evaluated quarterly and on any material
  infrastructure change
- Any on-prem custom-model path (KAI-291) — the customer is responsible
  for running this protocol on their custom model and Kaivue will provide
  the test harness but not a warranty

## Test set construction

### Composition requirements

Stratification dimensions:

- **Fitzpatrick skin tone**: 6 buckets (I, II, III, IV, V, VI).
- **Age**: 5 buckets (18–25, 26–40, 41–60, 61–75, 76+). Under-18 only with
  parental consent datasets, used separately.
- **Gender**: labeled categories as recorded by the source, plus "unknown".
- **Glasses**: yes / no.
- **Mask / face covering**: yes / no.
- **Lighting**: daylight / low-light / mixed.

Minimum cell counts determined by a power analysis targeting detection of
a 50% relative difference in FNR between groups at alpha=0.05 and
power=0.8. **TODO (lead-ai):** compute and document the resulting N per cell.

### Identity disjointness

- Every identity in the test set must be **absent** from training and
  validation sets. Identity-level disjointness, not image-level.
- Identities that overlap between training and test are removed from the
  test set, not from training.

### Provenance

- Sources documented per `data-governance.md`.
- Consent basis documented per identity or per bulk license.
- License permits evaluation use and publication of aggregate metrics.
- Dataset hash pinned in the model registry with the evaluation run.

## Metrics

### Per-group metrics

For each stratum cell (e.g. Fitzpatrick IV + 41–60 + female + no-glasses +
no-mask + daylight):

- **FPR** (false positive rate) at the operating threshold
- **FNR** (false negative rate) at the operating threshold
- **Precision**
- **Recall**
- **TAR @ FPR=1e-4** (true accept rate at a fixed low false accept rate)
- **ROC curve**

### Disparity metrics

Cross-group ratios calculated across marginal slices (e.g. Fitzpatrick only,
age only):

- `max(FPR_group) / min(FPR_group)` across groups with cell counts above the
  minimum. **Ship threshold: < 1.5.**
- `max(FNR_group) / min(FNR_group)`. **Ship threshold: < 1.5.**
- `max(TAR@1e-4) - min(TAR@1e-4)`. **Ship threshold: < 5 percentage points.**

Disparity is also reported per pair-wise group combination for the
top-impact cross-stratifications (e.g. Fitzpatrick VI + 61+ vs Fitzpatrick I

- 26–40).

## Ship / no-ship decision

A model release candidate MAY ship only if **all** of the following hold:

1. Aggregate FPR < 0.1% at the chosen operating threshold.
2. Aggregate FNR < 2% at the chosen operating threshold.
3. All disparity ratios above are within the stated thresholds.
4. No cell with N above the minimum has FNR > 10% (hard floor — no group
   left behind).
5. The test was run on a pinned, hashed dataset distinct from training.
6. The run is signed off by lead-ai and reviewed by lead-security.

If any criterion fails:

- The candidate is **blocked from release**.
- A mitigation plan is filed as an Art. 9 risk review entry.
- Options: rebalance training data, threshold retuning per group (with
  legal review), architecture change, or ship-with-restriction (i.e., do
  not ship for deployment profiles where the weakness applies and disclose).
- A ship-with-restriction requires CISO + legal sign-off and is published
  in the Trust Center alongside the published metrics.

## Regression cadence

- **Every model update:** full protocol before release.
- **Quarterly:** regression on the last-released production model with
  a freshly held-out slice where possible.
- **On drift alert:** unscheduled re-run triggered by post-market monitoring.
- **Annual:** protocol itself reviewed — thresholds, dataset, strata.

## Publication

The aggregate and per-group metrics are published in the Trust Center
(KAI-394) alongside each release, with:

- Release date
- Model version hash
- Dataset version hash
- FPR / FNR / TAR@1e-4 table
- Disparity ratios
- Known limitations and operating envelope
- Contact for questions

Raw test data, embeddings, and per-identity results are **not** published.

## Reproducibility requirements

- Evaluation code checked into the `tools/fairness/` path (TODO — not yet
  created) with tests.
- Random seeds fixed.
- Docker image hash recorded.
- Run manifest: code commit, dataset hash, model hash, seeds, hardware type.
- Third-party verifiability: lead-security can independently re-run the
  evaluation from the manifest and must do so at least once per quarter.

## Storage of results

- Every run produces a signed JSON report filed in the model registry
  (KAI-279) and referenced from the Annex IV technical file.
- Retained for 10 years per Article 18.
- Access-controlled and audit-logged.

## Custom model path (KAI-291)

If customers upload their own face recognition model:

- Kaivue provides the test harness.
- Customers must supply evaluation data and consent documentation.
- Results, if the customer agrees, may be published in the Trust Center
  tagged as customer-submitted, not a Kaivue warranty.
- A custom model that fails the ship criteria cannot be enabled for
  production inference without written acceptance of residual risk by
  the customer's controller and DPO.

## Relationship to external benchmarks

- NIST FRVT results, when available for the underlying architecture, are
  referenced in the model card for cross-validation.
- Differences between NIST's evaluation and Kaivue's are documented
  (dataset, demographic composition, use-case).

## TODOs

- [ ] lead-ai — compute per-cell N via power analysis
- [ ] lead-ai — build the evaluation harness in `tools/fairness/`
- [ ] lead-security — sign-off template for release gate
- [ ] lead-security — Trust Center (KAI-394) publication pipeline
- [ ] legal — review "ship-with-restriction" disclosure language
