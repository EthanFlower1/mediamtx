// Package r2 provides the Cloudflare R2 storage client for cloud archive of
// video segments. R2 is chosen specifically for zero egress fees — customers
// can play back archived recordings without surprise bandwidth bills.
//
// # Architecture seams
//
// This package imports only:
//   - internal/shared/*
//   - internal/cloud/db/*
//   - internal/shared/cryptostore
//
// It MUST NOT import internal/directory/*, internal/recorder/*, or any on-prem
// package. The recorder-side uploader is KAI-265; tier-transition River jobs
// are KAI-267.
//
// # Tenant isolation
//
// Every object key MUST start with the tenant_id prefix. The KeySchema type
// enforces this at construction time — raw string keys are never accepted.
// Cross-tenant access via the client is prevented at the key-schema level and
// should additionally be enforced via per-tenant R2 IAM tokens (KAI-231 IaC).
//
// # Encryption modes
//
//   - Standard: R2 server-side encryption (default for most tenants).
//   - SSE-KMS: Customer-managed AWS KMS key — TODO (interface present, not implemented).
//   - CSE-CMK: Client-side encryption via cryptostore before upload. The cloud
//     sees only ciphertext. Even Kaivue staff cannot decrypt without the
//     customer's master key. Fail-closed: encryption errors abort the upload.
//
// # Bucket layout
//
// Three buckets per environment (hot / warm / cold). Bucket names follow the
// pattern kaivue-{env}-{tier}. A fourth Archive tier uses Backblaze B2 and is
// out of scope for this package (see KAI-267).
//
// The actual R2 account and bucket provisioning is IaC owned by KAI-231.
// See configs/cloud/archive/README.md for the required bucket names.
package r2
