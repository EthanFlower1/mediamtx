// Package summaries implements smart event summaries via self-hosted LLM
// inference (KAI-289). Events from the NVR event stream are aggregated
// per-tenant, transformed into natural language prompts, and sent to a
// Llama 3 (or similar) model served by Triton Inference Server.
//
// Key design constraints:
//   - Zero third-party data egress: all inference runs on our cloud Triton
//     instance; no data leaves the deployment boundary.
//   - Per-tenant data isolation: every aggregation, prompt, and inference
//     call is scoped by tenant_id. Cross-tenant context bleed is impossible
//     by construction.
//   - Delivery: summaries are delivered via the notification infrastructure
//     (email, webhook, push) using event type "summary.daily" / "summary.weekly".
//
// Package boundary: imports internal/cloud/db, internal/cloud/notifications,
// and standard library only. Never imports apiserver or other cloud packages
// to avoid import cycles.
package summaries
