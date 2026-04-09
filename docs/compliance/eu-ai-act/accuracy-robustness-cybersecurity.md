# Accuracy, Robustness, and Cybersecurity

**Article:** 15
**Status:** Draft — numerical baselines pending KAI-282, pen-test tracked as KAI-390
**Owner:** lead-ai + lead-security

## Purpose

Art. 15 requires that high-risk AI systems achieve an appropriate level of accuracy, robustness, and cybersecurity, and that those levels are consistent throughout the lifecycle.

## Accuracy

### Declared operating point

Every per-model body declares:

- True positive rate at the operating threshold.
- False positive rate at the operating threshold.
- Demographic breakdown of both (from `fairness-testing-protocol.md`).
- The threshold itself.

These declared numbers are included in the instructions for use (`transparency-and-information.md`) so that deployers understand what "accurate" means for this system.

### Measurement methodology

- Benchmark datasets per `data-governance.md` and `fairness-testing-protocol.md`.
- Measured in a reproducible pipeline that is content-addressed along with the model artifact.
- Re-measured on every model promotion.

### Field accuracy tracking

- Operator feedback (`post-market-monitoring.md`) provides the real-world signal.
- Drift beyond tolerance triggers the Art. 9 loop.

## Robustness

### Failure modes in scope

- **Resilience to errors, faults, inconsistencies** that may occur within the system or the environment.
- **Feedback loops.** Continuous learning scenarios that could reinforce biased outputs.
- **Adversarial examples.** Inputs crafted to cause misclassification.

### Mitigations

- **No online learning.** Models do not update themselves in production. Improvements flow through the registry (KAI-279) promotion path, gated by fairness and accuracy tests. This eliminates the runaway-feedback risk.
- **Input validation.** Frames below configurable resolution / face-size thresholds are rejected rather than passed to the matcher, with a structured log entry.
- **Confidence floor.** Scores below the per-tenant confidence threshold are not surfaced as matches; they become operational telemetry only.
- **Adversarial testing.** A synthetic adversarial test set runs as part of the promotion pipeline. Test set includes physical-world perturbations (printed adversarial patches), digital perturbations (PGD-style), and distribution-shift samples (extreme lighting, angle, occlusion). Numerical results are in the per-model body.
- **Fallback behaviour.** Router fallback (edge → cloud) per KAI-280 is documented and tested. Failure of both edge and cloud results in "inference unavailable" surfaced to the operator, not a silent default.

### Determinism

Inference is deterministic for a given (model, input) pair. Non-determinism from GPU kernels is constrained by pinned Triton configurations. Reproducibility of recorded match events is supported by logging the model version, threshold, and full input reference.

## Cybersecurity

### Threat model

Threats considered:

- **Network adversary** on the path between Recorder and cloud.
- **Compromised tenant** attempting cross-tenant access.
- **Compromised operator account** attempting mass vault enumeration or exfiltration.
- **Supply-chain attack** on model artifacts or runtime binaries.
- **Physical attack** on an on-prem Recorder appliance.
- **Insider threat** at Kaivue.

### Controls

- **Transport security.** mTLS on all inference and control-plane traffic. TLS 1.3 minimum. Certificates rotated via the KAI-376 PKI (tracked separately by lead-security).
- **Authentication and authorisation.** Casbin policies per KAI-225 enforce per-tenant isolation at the API boundary. Every API call carries a tenant-scoped token.
- **Data protection.** Vault data is customer-side encrypted under customer-managed keys (see `data-governance.md`). Kaivue holds only ciphertext.
- **Model integrity.** Every model artifact is content-addressed by sha256 and cryptographically signed by the Kaivue release key. Recorder and Triton verify both the hash and the signature before loading the artifact into memory. A mismatch is a hard failure that pages lead-security.
- **Build integrity.** Reproducible builds (KAI-428), SBOM generation, cosign-signed release artifacts. The build pipeline is the only path to a production artifact; manual artifact uploads are not accepted.
- **Runtime hardening.** Triton runs under a minimal-privilege service account. Custom-model execution (KAI-291) runs inside gVisor sandboxes with explicit resource limits.
- **Audit logging.** Every vault mutation, model promotion, inference configuration change, and security-relevant API call flows through the append-only audit log (KAI-233).
- **Secrets management.** All secrets in KMS or Vault; no secrets in repo, no secrets in environment files, no secrets in container images.
- **Pen testing.** Annual third-party pen test (KAI-390) covers the face-recognition API surface plus the vault and inference pipelines. Findings are tracked to closure in the SOC 2 control library (KAI-385).

### Resilience to data poisoning

Data poisoning is mitigated by the data-governance regime (`data-governance.md`):

- Training data provenance is required before use.
- New datasets go through the bias-examination process, which would surface crude poisoning.
- Models are evaluated on a held-out benchmark that the training pipeline cannot see.
- Promotion requires passing that benchmark within tolerance.

### Resilience to evasion

- Adversarial test set in the promotion pipeline (see Robustness).
- Confidence-floor gate and human review mean that an evasion attempt must also evade the reviewer.

### Incident response

Security incidents follow `serious-incident-reporting.md` where the Art. 3(49) threshold is met, and the Kaivue internal security incident runbook (lead-security domain) in all cases.

## Lifecycle consistency

Art. 15 requires that accuracy, robustness, and cybersecurity levels are maintained throughout the lifecycle. This is implemented by:

- **Promotion gate.** Every new model must re-demonstrate the same or better performance against the current baselines before it is approved (`fairness-testing-protocol.md`, model registry KAI-279).
- **Post-market monitoring.** `post-market-monitoring.md` continuously measures against the declared operating point.
- **Dependency management.** Runtime dependencies are pinned and SBOMs are tracked. CVEs against pinned dependencies trigger investigation.
- **Periodic re-validation.** Even in the absence of drift alerts, models are re-validated quarterly against the current benchmark.

## Interactions with other documents

- `risk-management-system.md` — R1, R2, R4, R5 are operationalised here.
- `data-governance.md` — data-side of accuracy and poisoning resilience.
- `fairness-testing-protocol.md` — measurement methodology for accuracy across demographics.
- `post-market-monitoring.md` — field measurement of accuracy and robustness.
- `serious-incident-reporting.md` — escalation path when cybersecurity fails.
