# Cloud Control Plane CI/CD

**Ticket:** [KAI-232](https://linear.app/kaivue/issue/KAI-232/cloud-control-plane-cicd-pipeline-argocd-github-actions)

This document describes the end-to-end build → scan → sign → promote → deploy
pipeline for Kaivue **cloud** services. It does **not** cover the on-prem
mediamtx binary pipeline, which is documented alongside KAI-428
(`.github/workflows/build.yml`, `supply-chain.yml`, `reproducibility-check.yml`).

## Scope

- Cloud services only: everything under `internal/cloud/**` shipped as
  independent container images per service.
- Per-service matrix is defined in `.github/cloud-services.json`.

## Architecture

```
  PR merge to main
        |
        v
  cloud-services-build.yml (GH Actions)
   - go test per service
   - reproducible go build (-trimpath, -buildvcs, -s -w -buildid=)
   - multi-stage Dockerfile → distroless/static-debian12:nonroot
   - push to ECR (tags: <sha>, v<semver>, dev-latest)
   - SBOM (SPDX + CycloneDX via anchore/sbom-action)
   - Trivy scan (HIGH/CRITICAL → fail)
   - Snyk scan (HIGH → fail, skipped if SNYK_TOKEN unset)
   - cosign KEYLESS sign (OIDC → Fulcio/Rekor)
   - cosign attest SBOM
        |
        v
  ArgoCD dev Application (automated prune + selfHeal)
        |
   [ Manual promotion: cloud-services-promote.yml ]
        v
  PR to kaivue/infra-envs bumping envs/staging/<svc>/values.yaml
        |
        v
  ArgoCD staging Application (manual sync)
        |
   [ Manual promotion again, target_env=prod ]
        v
  ArgoCD prod Application (manual sync + GH environment reviewer gate)
```

## Semver scheme

| Tag                        | Meaning                                           |
|----------------------------|---------------------------------------------------|
| `<12-char-sha>`            | Every build. Immutable, canonical deploy handle.  |
| `v<major>.<minor>.<patch>` | `git describe` of commit (`v1.2.3` on tagged releases, `v1.2.3-5-gabcdef` between). |
| `dev-latest`               | Rolling tag on main only. Dev ArgoCD follows this (optional — prefer `<sha>` for pinning). |

Promotion between envs should always use the **`<sha>` tag**, never
`dev-latest`, so the env repo records exactly what is deployed.

## Scan thresholds

| Tool  | Severity gate      | Action on finding  | Secret required |
|-------|--------------------|--------------------|-----------------|
| Trivy | HIGH, CRITICAL     | Fail build (SARIF uploaded to GH code scanning) | none |
| Snyk  | High (`--severity-threshold=high`) | Fail build        | `SNYK_TOKEN` (if unset, step is skipped with warning) |

`ignore-unfixed: true` is set on Trivy so unpatched CVEs with no upstream
fix don't block the pipeline. Revisit at every quarterly security review.

## Image signing (cosign keyless)

Keyless signing via GitHub OIDC → Sigstore Fulcio → transparency log Rekor.
Verification:

```bash
cosign verify \
  --certificate-identity-regexp "https://github.com/kaivue/mediamtx/.github/workflows/cloud-services-build.yml@refs/heads/main" \
  --certificate-oidc-issuer "https://token.actions.githubusercontent.com" \
  <registry>/<service>@<digest>
```

SBOM attestation (SPDX) is also published:

```bash
cosign verify-attestation --type spdxjson \
  --certificate-identity-regexp "..." \
  --certificate-oidc-issuer "https://token.actions.githubusercontent.com" \
  <registry>/<service>@<digest>
```

## Promotion procedure

1. Confirm dev is green — check ArgoCD UI for the target service.
2. Copy the `<sha>` tag from the green dev Application (or from the GH
   Actions run summary).
3. Actions → **cloud-services-promote** → **Run workflow**:
   - `service`: e.g. `directory-api`
   - `target_env`: `staging` (or `prod`)
   - `image_tag`: `<12-char-sha>`
4. Workflow opens a PR against `kaivue/infra-envs`. Review diff (should be a
   single-line `image.tag:` change) and merge.
5. ArgoCD reports the target Application as **OutOfSync**. Click **Sync** in
   the UI. For prod, GH `environment: prod` gate will require an additional
   approver on the `Run workflow` click itself.
6. Verify pods healthy + smoke tests. Record the deploy in the on-call log.

## Rollback procedure (< 2 min SLA)

**Golden path — ArgoCD UI rollback:**

1. ArgoCD UI → target Application (e.g. `directory-api-prod`).
2. Click **History and Rollback**.
3. Select the previous known-good revision.
4. Click **Rollback**. ArgoCD pins the Application to that revision and
   reconciles.
5. Verify pods healthy. Total wall clock: ~30–90 seconds.

**Fallback — env repo revert:**

If ArgoCD UI is unavailable, `git revert` the offending commit in
`kaivue/infra-envs` and merge. ArgoCD will auto-detect the revert.

> **Important:** an ArgoCD UI rollback pins the Application to a historical
> revision — subsequent syncs from git will NOT overwrite it until you click
> **Sync** again. Do not leave Applications pinned indefinitely; the
> follow-up is to land a real revert PR to the env repo.

## ~10 minute merge → dev SLA

Target budget (KAI-232 acceptance):

| Stage                                 | Budget  |
|---------------------------------------|---------|
| GH Actions queue + checkout + Go cache| ~1 min  |
| Test + build + docker build/push      | ~5 min  |
| SBOM + Trivy + Snyk + cosign          | ~2 min  |
| ArgoCD dev reconcile                  | ~1–2 min|
| **Total**                             | **~10 min** |

If the build exceeds 12 min p95, file a performance ticket against
lead-sre.

## KAI-232 acceptance checklist

- [x] CI/CD for cloud services (GH Actions build+test+scan+push)
- [x] Semver image tags (`<sha>`, `v<semver>`, `dev-latest`)
- [x] ArgoCD Applications per service, auto dev / manual staging+prod
- [x] Snyk + Trivy scans, HIGH/CRITICAL gate
- [x] cosign image signing (keyless OIDC)
- [x] SBOM generation (SPDX + CycloneDX, reused from KAI-428 pattern)
- [x] Rollback < 2 min via ArgoCD UI (documented)
- [x] `docs/operations/ci-cd.md` (this file)
- [ ] **TODO(lead-cloud):** provision ECR repos + AWS OIDC role
- [ ] **TODO(lead-cloud):** provision `kaivue/infra-envs` env repo
- [ ] **TODO(lead-cloud):** confirm final service list vs `cloud-services.json`
- [ ] **TODO(lead-sre):** create `prod` GH environment with reviewers; uncomment `environment:` in promote workflow

## Related workflows

| Workflow                                              | Owner        | KAI ticket |
|-------------------------------------------------------|--------------|------------|
| `.github/workflows/cloud-services-build.yml`          | lead-sre     | KAI-232    |
| `.github/workflows/cloud-services-promote.yml`        | lead-sre     | KAI-232    |
| `.github/workflows/build.yml`                         | lead-sre     | KAI-428    |
| `.github/workflows/supply-chain.yml`                  | lead-sre     | KAI-428    |
| `.github/workflows/reproducibility-check.yml`         | lead-sre     | KAI-428    |
