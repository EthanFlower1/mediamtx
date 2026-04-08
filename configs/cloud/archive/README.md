# Cloud Archive — R2 Bucket Provisioning

**IaC owner:** KAI-231 Terraform modules.
**This file documents what must exist** — actual provisioning belongs to the KAI-231 per-region Terraform module at `terraform/modules/r2/`.

---

## Required Cloudflare R2 Buckets

Three buckets per environment. The Go client derives names via `r2.BucketName(cfg, tier)`.

| Environment | Hot bucket | Warm bucket | Cold bucket |
|---|---|---|---|
| prod | `kaivue-prod-hot` | `kaivue-prod-warm` | `kaivue-prod-cold` |
| staging | `kaivue-staging-hot` | `kaivue-staging-warm` | `kaivue-staging-cold` |
| dev | `kaivue-dev-hot` | `kaivue-dev-warm` | `kaivue-dev-cold` |

A fourth **Archive** tier (Backblaze B2) is out of scope for these buckets and for this client. See KAI-267 for tier-transition River jobs.

---

## R2 IAM Token Scoping (per-tenant isolation)

The Cloudflare R2 API uses S3-compatible tokens. Each token is scoped to an object-prefix so tenant A's token can only read/write `{tenant_a}/*`.

The Go client enforces prefix isolation at the KeySchema level as a defense-in-depth measure (see `bucket.go:AssertTenant` and `ListObjectsV2`). IAM scoping is the outer defense.

Required token configuration per tenant (provisioned by KAI-231 IaC):

```
Token name:   kaivue-{tenant_id}-archive
Permissions:  Object Read, Object Write (no Delete — deletion is platform-initiated only)
Bucket:       kaivue-{env}-hot, kaivue-{env}-warm, kaivue-{env}-cold
Prefix scope: {tenant_id}/
```

Tokens are stored in AWS SSM Parameter Store (KMS-encrypted):

```
/kaivue/{env}/tenants/{tenant_id}/r2_access_key_id       (String)
/kaivue/{env}/tenants/{tenant_id}/r2_secret_access_key   (SecureString, KMS-wrapped)
```

The platform-level token (used by the cloud API for lifecycle / presigned URL generation) has read+write+delete across all prefixes. It is stored at:

```
/kaivue/{env}/platform/r2_access_key_id
/kaivue/{env}/platform/r2_secret_access_key
```

---

## Environment Variables

When running the cloud API locally or in CI:

```
R2_ACCESS_KEY_ID=<platform token key ID>
R2_SECRET_ACCESS_KEY=<platform token secret>
R2_REGION=auto          # "auto" is recommended for R2
ARCHIVE_R2_ACCOUNT_ID=<cloudflare account ID>
ARCHIVE_R2_ENV=dev      # prod | staging | dev
```

---

## Object Lifecycle (R2-side)

R2 object lifecycle rules are managed by KAI-267 River jobs (not native R2 lifecycle policies). The Go client's `CopyBetweenTiers` and `DeleteFromTier` methods are the primitives.

If you want to add native R2 lifecycle rules as a belt-and-suspenders (e.g., automatically delete objects older than 400 days from cold), configure them via the Cloudflare dashboard or Terraform and document here.

---

## Encryption Notes

| Mode | Setup required |
|---|---|
| Standard | None — Cloudflare R2 encrypts all objects at rest by default |
| SSE-KMS | TODO (KAI-266 stub) — requires KMS key ARN in config and wiring in `putSSEKMS` |
| CSE-CMK | Customer master key provisioned by KAI-251 cryptostore; no extra R2 config needed |

---

## Bucket CORS (for presigned PUT from Recorders)

Recorders upload directly to R2 via presigned PUT URLs. R2 CORS must allow PUT from the Recorder's egress IP range (or be open for the presigned endpoint). Configure via Terraform:

```hcl
resource "cloudflare_r2_bucket_cors" "hot" {
  account_id = var.cloudflare_account_id
  bucket_name = "kaivue-${var.env}-hot"
  rules = [{
    allowed_methods = ["PUT", "GET", "HEAD"]
    allowed_origins = ["*"]   # presigned URLs carry the auth — origin restriction optional
    max_age_seconds = 3600
  }]
}
```
