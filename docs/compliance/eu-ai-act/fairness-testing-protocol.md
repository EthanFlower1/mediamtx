# Fairness Testing Protocol

**Article:** 10(5) + 15
**Status:** Draft — numerical thresholds pending KAI-282 baseline
**Owner:** lead-ai

## Purpose

Define the fairness test suite every face-recognition model MUST pass before promotion to `approved` in the model registry (KAI-279), and the ongoing fairness monitoring regime in the post-market phase.

## When the suite runs

- **Promotion gate.** A model cannot transition from `candidate` to `approved` in the model registry until the suite has run on the held-out fairness benchmark and every metric is within tolerance. This gate is enforced programmatically (KAI-279 approval workflow calls the fairness-test runner and refuses promotion on any failing metric).
- **Continuous monitoring.** The suite runs weekly against a rolling sample of anonymised operator-feedback data from the post-market phase (see `post-market-monitoring.md`). Threshold breaches trigger the Art. 9 loop re-run.
- **Ad-hoc re-run.** Any serious-incident investigation that involves a potential bias complaint re-runs the suite with the incident-specific cohort.

## Demographic taxonomy

Kaivue uses the following taxonomy for fairness evaluation. It is explicitly coarse, because finer taxonomies risk leaking protected characteristics from the test data into the production inference pipeline.

- **Apparent age band:** 0–17, 18–29, 30–44, 45–64, 65+.
- **Apparent gender presentation:** feminine, masculine, androgynous (three-bin, not two).
- **Fitzpatrick skin type:** I–II, III–IV, V–VI (three-bin).
- **Capture condition:** indoor, outdoor, low-light, motion.

The taxonomy is reviewed annually. Changes are recorded in this document's change log and trigger a re-run of the suite on the new taxonomy for every deployed model.

IMPORTANT: Art. 9(9) of the AI Act gives special weight to the impact on minors. Every metric below is reported separately for the 0–17 band, and any negative drift in that band is treated as a blocker regardless of the overall aggregate.

## Metrics

For each demographic cell and for the overall population:

1. **True positive rate (TPR)** at the system's operating point.
2. **False positive rate (FPR)** at the system's operating point.
3. **Equalised-odds gap:** max TPR across cells − min TPR across cells. Also reported for FPR.
4. **Demographic parity gap:** max match-rate − min match-rate across cells.
5. **Confidence-score calibration:** expected calibration error (ECE) per cell.

### Tolerances

Numerical thresholds are per-model — the baseline established by the first KAI-282 model becomes the benchmark subsequent models are held to. Preliminary targets (subject to lead-ai sign-off once the baseline lands):

- Overall TPR at operating-point FPR ≤ 1e-4: ≥ 0.95.
- Equalised-odds TPR gap across all cells: ≤ 0.05 absolute.
- Equalised-odds FPR gap across all cells: ≤ 5× relative (e.g. if worst-cell FPR is 5e-4, the target max is 1e-3).
- ECE per cell: ≤ 0.02.
- 0–17 band: ANY negative delta relative to overall aggregate triggers a blocker regardless of absolute value.

These are INITIAL targets. The per-model body records the targets in force at the time of promotion, and the post-market monitoring uses those same targets as alerting thresholds.

## Benchmark datasets

The fairness benchmark is an **internal** held-out dataset assembled under the Art. 10(5) safeguards in `data-governance.md`. It is composed of:

- A public component (licensed benchmarks — to be selected from among FairFace, BUPT-Balancedface, or equivalent; per-model body records exactly which).
- An internal component (images collected under explicit consent from contractors and staff volunteers; consent is tracked in a per-image record retained for the same 10-year window as the rest of the technical documentation).

The benchmark is **NEVER** used for training. It lives in a separate bucket with separate access controls. Training-pipeline code that can see training data cannot see the benchmark, enforced by IAM policy.

## Procedure

For each candidate model:

1. The model-eval pipeline pulls the benchmark from its private bucket via short-lived credentials.
2. The model is run against every benchmark image. Outputs are stored as `(image_id, predicted_identity, score)`.
3. For each demographic cell, compute TPR, FPR, equalised-odds gap, demographic parity gap, ECE.
4. Compare against tolerances.
5. Emit a fairness report to R2 (content-addressed) and link it from the per-model body of `annex-iv-technical-documentation.md`.
6. The model registry (KAI-279) promotion API reads the report and refuses promotion on any failed metric.

The benchmark is deleted from pipeline memory after each run; it is never cached, and the compute job runs in a short-lived ephemeral environment.

## Failure handling

If a candidate model fails the suite:

- The candidate is marked `rejected` in KAI-279.
- The failure is recorded in the risk register under R3.
- The next loop of `risk-management-system.md` is triggered, with a focus on the failing cell(s).
- Remediation options: data augmentation targeting the failing cell, fine-tuning, architectural change, or rejecting the candidate in favour of the previous approved model.
- If remediation is not possible before a shipping deadline, the previous approved model is retained and the shipping deadline is pushed — fairness gates are **never** relaxed to meet a deadline.

## Reporting in per-model body

Every per-model body of `annex-iv-technical-documentation.md` MUST include a fairness section with:

- The tolerances in force at promotion time.
- The measured metrics per demographic cell.
- A narrative interpretation including explicit attention to the 0–17 band per Art. 9(9).
- A link to the content-addressed fairness report artifact.

## Change log

Changes to the taxonomy, tolerances, benchmarks, or procedure are listed here with date and rationale. Promoted models are tagged with the protocol version they were evaluated under.

- 2026-04-08 — Initial draft, awaiting KAI-282 baseline to lock tolerances.
