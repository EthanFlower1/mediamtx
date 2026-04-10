# ArgoCD — Kaivue Cloud Control Plane

This directory holds ArgoCD `Application` manifests for Kaivue cloud services,
delivered as part of KAI-232 (Cloud control plane CI/CD pipeline).

## Layout

```
deploy/argocd/
  applications/
    directory-api.yaml
    billing-api.yaml
    notifications-worker.yaml
    region-router.yaml
  README.md   (this file)
```

Each service file declares three `Application` resources — one per
environment (`dev`, `staging`, `prod`) — that all point at the GitOps env
repo `kaivue/infra-envs`.

## Env repo layout (expected)

```
kaivue/infra-envs
  envs/
    dev/
      directory-api/
        values.yaml      # image.tag lives here
        kustomization.yaml
      billing-api/
      notifications-worker/
      region-router/
    staging/   (same shape)
    prod/      (same shape)
```

> **TODO(lead-cloud):** this repo does not exist yet. It is referenced by
> every Application here and by `.github/workflows/cloud-services-promote.yml`.
> Provision it before the first real ArgoCD sync.

## Sync policy

| Env     | Policy                                              |
|---------|-----------------------------------------------------|
| dev     | `automated: { prune: true, selfHeal: true }`        |
| staging | Manual (no `automated:` block)                      |
| prod    | Manual (no `automated:` block) + GH `environment: prod` gate on promotion workflow |

## Promotion flow

1. PR merges to `main` → `cloud-services-build.yml` builds, scans, signs,
   pushes image to ECR.
2. Dev ArgoCD Application auto-syncs the new `dev-latest` / `<sha>` tag.
3. SRE / release captain runs `cloud-services-promote.yml` via
   `workflow_dispatch` with `service`, `target_env=staging`, `image_tag=<sha>`.
4. Workflow opens a PR against `kaivue/infra-envs` bumping
   `envs/staging/<service>/values.yaml`.
5. PR merge → ArgoCD staging Application is marked out-of-sync → operator
   clicks **Sync** in the ArgoCD UI.
6. Repeat with `target_env=prod` after validation. The `prod` GH environment
   will gate the workflow on required reviewers (once provisioned).

## Rollback

ArgoCD UI → Application → **History and Rollback** → select previous healthy
revision → **Rollback**. SLA: <2 min (KAI-232 acceptance criterion).

See `docs/operations/ci-cd.md` for the full procedure.
