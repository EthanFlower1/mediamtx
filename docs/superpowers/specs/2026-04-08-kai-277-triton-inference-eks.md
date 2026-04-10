# KAI-277: NVIDIA Triton Inference Server on EKS

Design memo. Author: ai-platform working group. Reviewers: lead-sre, lead-cloud, lead-security. Status: draft for review.

This document specifies the cloud inference tier for Kaivue's AI/ML platform. It does not re-open edge/cloud routing (KAI-280), does not redesign the model registry (KAI-279), and does not ship code.

---

## 1. Scope and non-goals

### In scope

- A shared NVIDIA Triton Inference Server deployment on the existing EKS cluster (KAI-215, us-east-2) that serves cloud-tier inference for Kaivue services: face recognition (KAI-282), LPR, behavioral analytics, heavyweight object detection, and customer-uploaded custom models (KAI-291).
- Triton topology, model repository design, per-tenant isolation, the Go client SDK contract, GPU node pool sizing, cost model, multi-tenancy guarantees, observability, and failure modes.
- Integration with the KAI-279 model registry as the single source of truth for model artifacts (R2-backed, content-addressed).
- Honoring Seam #4 (per-tenant vector index — relevant where Triton returns embeddings), Seam #8 (multi-region blob storage — R2 is the canonical model home), Seam #10 (tenant_id threads every request and every metric label).

### Non-goals

- Edge inference on Recorder boxes. Edge path is KAI-278 and uses local ONNX Runtime / TensorRT; Triton is cloud-only in v1.
- Edge-vs-cloud routing policy. KAI-280 already decided this; this doc is a downstream consumer.
- Model lifecycle (upload, signing, scanning, promotion). KAI-279 owns this; we only consume signed, registered artifacts.
- Training. Inference only.
- Autoscaling policy for non-GPU services.
- Multi-region active-active. We design to *not foreclose* it (Seam #8), but v1 ships us-east-2 only.
- Customer-visible model marketplace / billing meters. KAI-291 covers uploads; billing is a separate ticket.
- A bake-off between Triton and alternatives (KServe, Ray Serve, TorchServe, BentoML). Triton was picked in the AI platform charter; this doc takes that as given.

---

## 2. Architecture (component map)

### 2.1 Topology

One logical Triton "fleet" per region. In us-east-2 the fleet is a single Kubernetes Deployment (`triton-cloud`) with GPU-pinned pods on a dedicated GPU node group (`ng-gpu-inference`, §5). Pods are *fungible*: any pod can serve any tenant's model. Isolation is enforced in the model repository layout and the request path, not by pod topology.

Recommendation: **single shared Triton deployment**, not per-tenant. Rationale: at our projected fleet size (low hundreds of tenants in v1, thousands by end of year), per-tenant pods burn GPU hours on idle capacity and blow the cost model out of the water. Triton's instance groups and dynamic model loading already give us the isolation primitives we need (§3, §7).

Pod spec shape (descriptive, no YAML):

- One Triton container (`nvcr.io/nvidia/tritonserver:<pinned>`), gRPC on 8001, HTTP on 8000, metrics on 8002.
- Model repository mounted read-only from a sidecar-populated local EBS volume (§3.3).
- One `model-cache-agent` sidecar (Go, internal) responsible for lazy-pulling model artifacts from R2 via the KAI-279 registry, verifying content hashes, populating the local cache, and calling Triton's model-control API to load/unload.
- One `fluent-bit` sidecar for structured logs.
- Resource requests: 1 full GPU per pod (no MIG in v1 — see §11), 8 vCPU, 32 GiB RAM, 200 GiB gp3 EBS for model cache.

### 2.2 Model repository backend

Triton is configured in **explicit model control mode** (`--model-control-mode=explicit`). No auto-load on startup. Every model load is an API call from the `model-cache-agent` sidecar, which knows the tenant context. This is load-bearing for §7.

The model repository on disk is laid out by content hash, not by tenant-visible name — see §3.

### 2.3 Load balancer and service mesh

- In-cluster clients reach Triton via a headless Kubernetes Service (`triton-cloud-grpc`) so the Go client SDK can do client-side load balancing over pod IPs. gRPC connection pooling is the hot path; no L7 proxy in between.
- For cross-cluster / cross-region callers (future: Seam #8), we front Triton with an internal ALB terminating mTLS. v1 does not ship this; in-cluster only.
- No public exposure. Triton is never reachable from the public internet. Customer model uploads go through KAI-291 → KAI-279 → R2, never directly to Triton.

### 2.4 Go client SDK

A new package `internal/ai/tritonclient` (name provisional) provides the single integration surface for Kaivue services. Every service that wants cloud inference uses this SDK. Direct gRPC calls to Triton from service code are forbidden.

Responsibilities, enumerated in §4.

### 2.5 Metrics and observability

Triton exports Prometheus metrics natively on :8002. We scrape with the existing `kube-prometheus-stack`. We also emit *custom* per-tenant metrics from the Go SDK (§8) because Triton's built-in metrics are per-model, not per-tenant, and EU AI Act Art. 15 fairness auditing needs per-tenant.

---

## 3. Model repository design

This section addresses **coupling point #2** — per-tenant model isolation.

### 3.1 Decision

**Shared Triton deployment, per-tenant model-name namespacing, enforced at the model-cache-agent sidecar and re-checked in the Go client SDK, with content-addressed storage on disk.**

Rejected alternatives:

- **(a) Separate Triton deployment per tenant.** Cleanest isolation story, worst cost story. A single g5.2xlarge runs ~$1.20/hr on-demand; with ~500 tenants that is $600/hr baseline before any traffic. Also murders cold-start: every tenant pays a 30-90s Triton boot for their first request. Rejected on cost and latency.
- **(c) Shared Triton + per-tenant model repository mount + instance groups.** Workable but fragile: it relies on Triton's filesystem polling and per-instance-group GPU affinity, which does not compose well with dynamic tenant onboarding. Instance groups are statically declared in the model config at load time; dynamic tenant add means a reload. Rejected on operability.
- **(b) the one we picked:** shared Triton, model-name namespacing, request-level auth.

### 3.2 Namespacing scheme

Every model loaded into Triton is registered under a **namespaced model name**:

```
t_<tenant_id>__<logical_model_name>__<content_hash_short>
```

Example: `t_01HF3Z9K__face_recognizer_v3__a1b2c3d4`.

Rules:

- `tenant_id` is the internal ULID, never the customer-visible slug. Prevents collisions on rename.
- `content_hash_short` is the first 8 hex chars of the KAI-279 content hash. Disambiguates versions and makes rollouts atomic (load new name, flip SDK pointer, unload old).
- The double underscore `__` is the separator. The parser rejects any component containing `__`.
- **System models** (Kaivue-built face/LPR/behavior models that are shared across tenants) use the reserved tenant ID `t_system`. They are loaded once and reused.

The Go SDK *never* lets a service pass a raw model name. The SDK takes `(ctx, tenant_id, logical_model_name)` and constructs the namespaced name. The Triton request only ever carries the namespaced name. §7 closes the loop.

### 3.3 On-disk layout and content addressing

Triton's on-disk model repository at `/models/` is a *view* into a local content-addressed store:

```
/model-cache/blobs/<sha256>/...       # content-addressed, deduped
/models/<namespaced_name>/             # symlinks into /model-cache/blobs/<sha256>/
    config.pbtxt
    1/
      model.plan                       # symlink → /model-cache/blobs/<sha256>/model.plan
```

Consequence: two tenants using the same Kaivue-provided LPR model share one copy of the weights on disk (and in GPU memory, when instance groups allow it) but have *separate namespaced model entries*. Dedup is invisible to the tenant.

The `model-cache-agent` is the only writer to `/model-cache/blobs/` and `/models/`. Triton mounts both read-only. No other process in the pod can create a symlink.

### 3.4 Load and unload policy

- **Load on demand.** SDK calls `EnsureLoaded(tenant_id, logical_model_name, version)`. Sidecar checks registry (KAI-279) for the resolved content hash, fetches if not cached, writes symlink, calls Triton `repository/models/<name>/load`. Cold path adds 5–30 s for small models (<100 MB) and 30–120 s for face/LPR (200–500 MB); this is acceptable because cloud inference is already the async/batched tier.
- **LRU unload.** A watchdog in the sidecar unloads namespaced models that have not been invoked in N minutes (default: 20). Triton frees the GPU memory. Content blobs remain on EBS until disk pressure (>80%) triggers cache GC.
- **Pinned models.** System models (`t_system__*`) are marked pinned and never LRU'd. A handful of hot customer models can be pinned via an allowlist controlled by lead-sre.
- **Zero-downtime version bumps.** A new version of a logical model produces a new namespaced name (different short hash). SDK gets the new resolved version from registry, loads it, then flips its in-memory pointer. Old version stays loaded until LRU evicts it. No in-place model replacement, ever — Triton's in-place reload has edge cases under load.

### 3.5 Customer-uploaded models (KAI-291)

Customer models are *untrusted binaries*. The registry (KAI-279) already runs signing and static scanning. This doc adds a runtime constraint: customer models load only into `instance_group { kind: KIND_GPU }` pods that are in a **customer-models subpool** of the GPU fleet, labeled `kaivue.io/workload=customer-models`. System models run in the main pool. Rationale: a malicious custom ONNX graph that exploits a CUDA driver bug cannot reach pods serving face recognition for other tenants. This is belt-and-suspenders next to §7; lead-security specifically asked for a blast-radius story.

---

## 4. Request path

### 4.1 Call shape

```
Go service → tritonclient.Infer(ctx, req) → gRPC to Triton pod → response
```

`req` carries: `tenant_id` (required), `logical_model_name` (required), `inputs` (typed), `deadline` (from ctx), `killswitch_token` (from ctx, see §4.4), `idempotency_key` (optional).

### 4.2 Client pooling and load balancing

- One long-lived `grpc.ClientConn` per Triton pod, created on pod discovery via the headless Service's endpoints. A background watcher refreshes on EndpointSlice changes.
- Client-side round-robin with outlier ejection (remove pods returning >5% errors in a 30 s window).
- HTTP/2 streams per conn capped at 100; Triton default is fine, we just don't want unbounded concurrency on one pod.

### 4.3 Timeouts and retries

- Default deadline: 800 ms for online (real-time) calls, 10 s for batched/async. Caller can override but not remove — `ctx.Done()` must always fire.
- Retry policy: at most one retry, only on `UNAVAILABLE` and `DEADLINE_EXCEEDED` from a *different* pod (SDK tracks which pod it hit). Never retry on `INVALID_ARGUMENT` or `PERMISSION_DENIED`. Never retry face-recognition inference — it is a high-risk path under the EU AI Act and silent retries muddy the audit trail.
- On `RESOURCE_EXHAUSTED` (GPU OOM), no retry; bubble up with a typed error so the caller can decide (queue, degrade, drop).

### 4.4 Tenant_id threading and killswitch propagation (Seam #10)

- `tenant_id` is pulled from the request context via `authz.TenantFromContext(ctx)`. The SDK refuses to build a request if the context has no tenant. No tenant → no inference. This is the chokepoint for §7.
- The SDK reads the killswitch state from the killswitch service (KAI-282 dependency) **on every inference call** for models marked `eu_ai_act_high_risk=true` in the registry. The check is a local in-process cache with a 5 s TTL, refreshed asynchronously; the 60 s SLO from EU AI Act Art. 14 leaves ample headroom. If the killswitch is engaged for the tenant-or-globally, the SDK returns `ErrKillswitchEngaged` *before* hitting the network. No inference request is ever sent. This is auditable: every refusal emits a structured log line with `tenant_id`, `model`, `reason=killswitch`, `killswitch_version`.
- The killswitch token in the request is a defense-in-depth check on Triton side via a [Triton Python backend preprocessor / request hook] — **deferred, see §11.** v1 relies on client-side enforcement.

### 4.5 Tenant_id in Triton wire format

The tenant_id travels two ways on the wire:

1. Implicitly in the **model name** (§3.2). This is the authoritative binding.
2. Explicitly in a gRPC metadata header `x-kaivue-tenant-id`, used only for per-tenant metric labeling and logs. **Not used for authorization.** The model name is the security boundary.

---

## 5. GPU node pool

This section addresses **coupling point #1** — SKU and autoscaling.

### 5.1 Decision

**Primary pool: `g5.2xlarge` on-demand, min 2, max 20, one NVIDIA A10G per node.**
**Burst pool: `g5.2xlarge` spot, min 0, max 30, same instance type for scheduling compatibility.**
**Customer-models subpool (§3.5): `g5.2xlarge` on-demand, min 1, max 8, labeled `kaivue.io/workload=customer-models`.**
**Warm pool: 2 pre-warmed nodes in the primary pool, image pre-pulled, kept in `Stopped` state via Karpenter's warm-pool equivalent.**

Rejected alternatives:

- **g6 (L4).** Better $/inference on most workloads and the obvious long-term pick. Rejected for v1 *only* because L4 capacity in us-east-2 was still tight at the time of the node-group plan (March 2026) and we prefer a boring instance we can actually get. Re-evaluate in the Q3 cost review; the autoscaler config is SKU-agnostic.
- **p4d / p5.** 8x A100 or H100. Overkill. We are not training and our batch sizes don't saturate an A100, let alone an H100. Cost is 5-10x a g5 for workloads that don't use the headroom.
- **g4dn.** T4 is a generation behind A10G; face recognition TensorRT plans run ~1.7x faster on A10G at similar $/hr. Not worth the regression.
- **MIG (Multi-Instance GPU) on A10G.** A10G does not support MIG. Moot.

### 5.2 Spot vs on-demand

- System models (face, LPR, behavior, object detection) run on the on-demand primary pool. They are tied to real-time-ish customer paths; a spot interruption mid-request at 2am is a bad look.
- Overflow *batch* inference (e.g., re-scoring historical footage, bulk embedding generation) runs on the spot burst pool. The SDK tags batch requests with `priority=batch` and Triton scheduler sends them there.
- Customer models run on-demand until we have enough data to know how they behave; moving them to spot is a Q3 optimization.

### 5.3 Autoscaling

We use **Karpenter**, not Cluster Autoscaler. Karpenter's provisioning latency on g5 in us-east-2 is ~60-90 s vs ~3-5 min for CA on an ASG, and cold-start is the single biggest UX hit for cloud inference.

Signals:

- Primary signal: GPU utilization via DCGM exporter, scale up at sustained >70% for 2 min.
- Secondary signal: pending pods due to `nvidia.com/gpu` resource requests.
- Scale down: Karpenter's default consolidation, with a 10 min cooldown to avoid thrashing on bursty traffic.

### 5.4 Cold-start budget

- **Node provisioning:** 60-90 s (Karpenter + AMI pull).
- **Pod startup:** 30 s (image pull from ECR cache, Triton boot).
- **First model load (cold cache):** 15-90 s depending on model size and R2 egress.
- **Total cold path:** ~3-4 min worst case from "zero capacity" to "first inference served."

Mitigations:

- Warm pool of 2 pre-baked nodes (image already pulled, Triton already running, system models pre-loaded). Absorbs the first burst.
- ECR pull-through cache and a custom AMI with the Triton image baked in. Cuts pod startup by ~20 s.
- Pre-warming system models at pod startup from the on-node blob cache (§6.2). Cuts first-inference by ~30 s.

This budget is fine for async/batched paths. It is **not** fine for real-time paths, which is why real-time goes to the edge per KAI-280.

---

## 6. Cost model

This section addresses **coupling point #3** — egress cost for model pulls.

### 6.1 Decision

**Per-node EBS-backed content-addressed cache (§3.3), pre-warmed on node launch for system models via a bundled seed layer in the AMI. Customer model pulls are direct-from-R2 with no CDN in front, but charged to the tenant's metered egress bucket (KAI-291 billing hook). No Transfer Acceleration.**

Rejected alternatives:

- **Transfer Acceleration.** It's a ~50% price premium on egress for a speedup we don't need (R2 → EKS us-east-2 is already fast). Kills our margin. Rejected.
- **S3/R2-through-CloudFront.** Would work but adds an invalidation story and a second signing surface. The AMI seed approach is simpler and cheaper for the *system* models that dominate volume.
- **Shared EFS model repo.** Neat on paper: pull once, serve from all pods. In practice EFS latency on a 500 MB model load is worse than local EBS, and IOPS costs pile up. Rejected.

### 6.2 Egress budget math

Assumptions (order-of-magnitude, not commitments):

- 4 system models averaging 250 MB each = 1 GB per cold node.
- Primary pool scales 2 → 20 nodes on a bad day; that's 18 cold events × 1 GB = 18 GB.
- Customer models: assume 50 tenants × 200 MB each loaded once a day = 10 GB.
- R2 egress to AWS us-east-2 is cheap relative to S3 cross-region, but not free at fleet scale. Seam #8 lets us colocate later.

**Total daily egress from R2 for model pulls: ~30 GB/day steady, ~100 GB/day during scale events. Budget: $X/month — lead-cloud to confirm the exact number against the R2 contract.**

Mitigations in place:

- **AMI seed layer.** Bake the current content hashes of system models into the AMI at build time. New nodes boot with the cache already populated. Egress on scale-up for system models = 0. Stale when system models update; AMI rebuild cadence is weekly, the gap is filled by lazy pulls.
- **Per-node cache persistence.** EBS volume is 200 GB gp3, content-addressed, survives pod restarts. Karpenter consolidation destroys the node (and its cache) — accepted tradeoff.
- **Customer model pulls are billable.** Egress is metered per tenant and lands in the KAI-291 billing pipeline. Tenants with pathological load patterns pay for their own cost. This is the pressure-release valve on the shared infra budget.

### 6.3 GPU hours

- Steady state primary (2 nodes, on-demand, g5.2xlarge us-east-2): ~$1.20/hr × 2 × 730 = ~$1,750/mo.
- Customer models subpool (1 node baseline): ~$875/mo.
- Burst spot: variable, budget envelope $1,000/mo.
- Warm pool (stopped state, EBS only): ~$40/mo for EBS.
- **Rough v1 baseline: ~$3,500-4,000/mo for GPU infra before traffic spikes.**

Per-tenant unit economics at baseline allocation (split across ~200 tenants at launch): ~$18-20/tenant/mo for cloud inference infra. Real number depends on actual utilization curves; lead-cloud owns the real model.

### 6.4 Autoscale headroom

Always keep at least 1 GPU worth of headroom in the primary pool (i.e., scale-up triggers at 70% sustained util, not 95%). This burns some idle GPU but absorbs spikes without users seeing the 3-4 min cold path.

---

## 7. Multi-tenancy isolation guarantees

This is the section lead-security will read first.

### 7.1 Claim

**Under no code path can tenant A's inference request reach a model loaded for tenant B**, even if the calling service is buggy, even if a developer hand-constructs a request, even if a customer uploads a model with a colliding logical name.

### 7.2 Enforcement chain (defense in depth)

The request crosses **six** checks. Any one of them catches a misrouted request; all six would have to fail simultaneously for a cross-tenant leak.

1. **Context-bound tenant resolution.** The Go SDK refuses to build a request if `ctx` has no `tenant_id` set by the auth middleware. There is no API to pass `tenant_id` as an argument. The only source is the authenticated request context. Bypass requires forging a context, which requires code inside the auth boundary.

2. **Name construction is internal to the SDK.** `InferRequest` has no public `ModelName` field. The SDK constructs the namespaced name `t_<tenant_id>__<logical>__<hash>` from the context tenant and the caller's logical name. A caller cannot *ask for* a specific namespaced name.

3. **Name re-parsing and re-check.** Before the gRPC call, the SDK parses its own constructed model name and asserts the embedded tenant matches `ctx`'s tenant. This catches any accidental mutation or (in the future) any path that tries to pass a raw name.

4. **Registry-side authorization.** `EnsureLoaded(tenant_id, logical_name, version)` hits the KAI-279 registry, which checks that the tenant owns or is entitled to the logical model before returning a content hash. A tenant asking for another tenant's logical model gets `PERMISSION_DENIED` at the registry, before a load ever happens. System models are explicitly allowlisted for all tenants.

5. **Model-cache-agent sidecar load.** The sidecar is the only actor that calls Triton's load API. It refuses to load a model whose namespaced name's embedded tenant_id disagrees with the registry lookup. This catches a compromised registry response.

6. **Physical subpool for customer models.** Customer-uploaded models only load in the `kaivue.io/workload=customer-models` node subpool (§3.5). A malicious custom model cannot run on a node that is also serving face recognition for any tenant. Narrow blast radius on sandbox-escape bugs.

### 7.3 What is NOT a boundary

Stated explicitly so lead-security can challenge:

- The gRPC metadata `x-kaivue-tenant-id` header is **not** a boundary. It's for metrics. Treat it as untrusted.
- Pod-level isolation is **not** a boundary except in the customer-models subpool. System-model pods are shared across tenants by design.
- GPU memory isolation is **not** a boundary. Two tenants' inferences can land on the same GPU microseconds apart. We rely on CUDA process isolation (same Triton process, different model instances). If a CUDA driver bug leaks activation memory across model instances, we have a problem — this is why §3.5 exists for custom models.

### 7.4 Test obligations

Not writing them here (this is a design doc) but listing them so lead-security can sign off on coverage:

- Fuzz the SDK name constructor with adversarial tenant_ids and logical names containing `__`, nulls, unicode.
- Property test: for 10k random (tenant, model) pairs, assert no cross-tenant resolution.
- Integration test: authed service A cannot invoke a namespaced model name from tenant B even via raw gRPC (this tests the registry + sidecar path).
- Chaos test: kill the model-cache-agent mid-load; verify Triton does not end up with a partially loaded model under any name.

---

## 8. Observability

### 8.1 Metrics from Triton

Scrape Triton's :8002 endpoint. Key series we care about:

- `nv_inference_request_success{model}` / `_failure{model}` — per namespaced model name.
- `nv_inference_request_duration_us{model}` — p50/p95/p99.
- `nv_inference_queue_duration_us{model}` — queue time; scaling signal.
- `nv_inference_compute_input_duration_us`, `_compute_infer_`, `_compute_output_` — decomposition.
- `nv_gpu_utilization{gpu_uuid}`, `nv_gpu_memory_used_bytes` — via DCGM.
- `nv_cache_num_hits_model`, `nv_cache_num_misses_model` — response cache effectiveness.

### 8.2 Custom metrics from the Go SDK (per-tenant)

These are where EU AI Act Art. 15 fairness auditing lives. Emitted by the SDK, labeled with `tenant_id`, `logical_model`, and (for system models) `model_version`:

- `kaivue_inference_requests_total{tenant_id, logical_model, result}` — counter; `result` in `{ok, err_killswitch, err_permission, err_deadline, err_unavailable, err_oom, err_other}`.
- `kaivue_inference_latency_seconds{tenant_id, logical_model}` — histogram, client-observed end-to-end (includes queue time).
- `kaivue_inference_killswitch_refusals_total{tenant_id, logical_model, reason}` — counter, for audit.
- `kaivue_inference_tenant_gpu_seconds_total{tenant_id, logical_model}` — derived from Triton's compute duration, aggregated client-side; this is the fairness number.
- `kaivue_model_cache_pull_bytes_total{tenant_id, logical_model, source}` — for egress cost attribution.

Cardinality is bounded: O(tenants × models), which at target scale is <100k series. Fine.

### 8.3 Logging

Structured JSON, one line per inference. Fields: `tenant_id`, `logical_model`, `namespaced_model`, `content_hash`, `pod_id`, `gpu_uuid`, `duration_ms`, `queue_ms`, `result`, `killswitch_version`, `request_id`. Face-recognition calls additionally carry the Art. 14 audit fields (decision, confidence, human-review marker) per KAI-282.

### 8.4 Alerting thresholds

- Triton pod crashloop: page immediately.
- p99 inference latency > 2x baseline for 5 min on any system model: page lead-sre.
- GPU utilization sustained >85% cluster-wide for 10 min with no scale-up: page lead-sre (autoscaler stuck).
- Killswitch refusal rate > 0 for 60 s on non-test tenants without a declared incident: page lead-security.
- R2 egress budget burn rate >1.5x projected: email lead-cloud daily.
- Customer-model OOM rate > 1% over 15 min: page on-call, possibly disable tenant's model.

---

## 9. Failure modes

Table form for density. "Caller sees" is what the Go SDK returns to its caller.

| Failure | Triton-side behavior | SDK behavior | Caller sees |
|---|---|---|---|
| Triton pod crashes mid-request | gRPC stream reset | SDK marks pod unhealthy, removes from pool, retries once on a different pod (if retriable, §4.3) | `UNAVAILABLE` after retry, or success on retry |
| GPU OOM on inference | Triton returns `RESOURCE_EXHAUSTED` | No retry; emit `err_oom` metric | Typed error `ErrGPUExhausted`, caller can queue/degrade |
| Model load failure (bad artifact) | Sidecar logs, does not call Triton load | SDK returns from `EnsureLoaded` with error | `ErrModelLoadFailed` with content hash; ops can reproduce |
| Model load failure (R2 outage) | Sidecar retries with backoff, eventually gives up | `EnsureLoaded` deadline exceeded | `ErrModelUnavailable`; if a previous version is still loaded, SDK transparently falls back to it (optional, off by default in v1) |
| Tenant GPU quota hit (future) | n/a in v1 — no per-tenant GPU quota enforcement yet | SDK's rate limiter (KAI-291 billing path) returns quota error before hitting Triton | `ErrTenantQuotaExceeded` |
| Killswitch engaged | n/a — request never sent | SDK refuses locally (§4.4) | `ErrKillswitchEngaged`, audit log emitted |
| R2 outage (global) | New model loads fail; loaded models keep serving | `EnsureLoaded` fails for uncached models; cached models work | Mixed: cached workloads OK, new workloads fail |
| EKS node termination (spot interruption) | Pod moved; in-flight requests fail | SDK retries on different pod | Usually transparent; worst case one `UNAVAILABLE` |
| Karpenter fails to provision GPU node | Pending pods, queue time spikes | SDK returns deadline-exceeded once its own deadline hits | `DEADLINE_EXCEEDED`; alerts fire per §8.4 |
| Misrouted request (wrong tenant) | Cannot happen by construction (§7) | SDK panics in test, returns `ErrInternal` in prod and emits a CRITICAL log | `ErrInternal`; on-call paged |
| Model name collision across tenants | Impossible — namespaced name embeds tenant_id | n/a | n/a |

---

## 10. Open questions for leads

Each is an explicit ask. Not a design choice we're deferring — a decision we need from someone else.

**For lead-sre:**

- Confirm `ng-gpu-inference` node group naming and labels match your EKS node group conventions (KAI-215).
- Confirm Karpenter is the cluster autoscaler going forward. If it's still CA, the cold-start numbers in §5.4 are wrong and we need a different warm-pool story.
- Who owns the custom AMI build pipeline and what's the cadence? §6 assumes weekly; if it's monthly, the AMI seed layer goes stale and egress budget balloons.
- DCGM exporter deployment: is it already on GPU nodes in KAI-215, or is this doc adding it?

**For lead-cloud:**

- Confirm the R2 → AWS us-east-2 egress pricing we assumed in §6.2. The math is load-bearing for the per-tenant unit economics.
- Approve the ~$3,500-4,000/mo baseline GPU budget and the burst spot envelope of $1,000/mo.
- Decision: do we want cross-account billing tags on GPU instances so the finance dashboard can break out inference cost by tenant (per §8.2's `kaivue_inference_tenant_gpu_seconds_total`)?
- Seam #8 question: when we light up eu-west-1, does Triton get a second fleet there, or do we route EU tenants to us-east-2 and eat the latency until then? (This doc assumes the former eventually; v1 ships us-east-2 only.)

**For lead-security:**

- Sign off on §7 as the multi-tenancy isolation story. Specifically: does the six-check chain meet the blast-radius bar you set for customer-uploaded models (KAI-291)?
- Decision on the deferred server-side killswitch check (§4.4 and §11.2). Is client-side-only enforcement with a 5 s cache acceptable for v1 given the 60 s SLO, or is that a blocker?
- Approve the customer-models subpool isolation strategy (§3.5). Is physical pod separation enough, or do you want a separate Triton process (not just a separate node)?
- Audit-log retention for face recognition inference (§8.3). Confirm 7 years per the Art. 14 interpretation we're working from.

---

## 11. What this doc does NOT commit to

Explicit deferrals. None of these block v1.

1. **Multi-region Triton fleets.** Seam #8 requires we don't foreclose this, and we haven't: the model registry is R2 (global), the SDK is stateless, and the naming scheme has no region assumption. But v1 is us-east-2 only and we are not sizing eu-west-1 today.

2. **Server-side killswitch enforcement in Triton.** v1 is client-side-only via the Go SDK (§4.4). A Triton Python-backend preprocessor that re-checks the killswitch on every inference call is a natural v2 addition but is not required to meet the 60 s SLO and adds complexity we want to defer.

3. **Per-tenant GPU quotas.** §9 mentions the failure mode; §11 admits we don't enforce it in v1. v1 relies on shared Karpenter scale-up to absorb demand. A noisy tenant can briefly affect latency for others. Quotas land with KAI-291 billing.

4. **MIG (Multi-Instance GPU).** The chosen A10G does not support MIG; moot for v1. If we move to L4/A100/H100 in v2, MIG becomes a real choice for finer-grained per-tenant GPU slicing.

5. **Response caching.** Triton has a response cache. We're not turning it on in v1 because correctness semantics under per-tenant namespacing need more thought — we do not want tenant A's cached face embedding to somehow serve tenant B even with a cache-key collision that "shouldn't" happen.

6. **Streaming inference.** Triton supports bidirectional gRPC streaming. Our v1 workloads are all unary. Deferred.

7. **Custom metrics backend for fairness auditing.** §8.2 defines the metric shape; the actual long-term storage and the EU AI Act Art. 15 reporting UI are separate tickets.

8. **SDK code review, naming, interface stability.** The package name `internal/ai/tritonclient` is a placeholder. The final interface lands in the implementation PR.

9. **Chaos / game-day exercises.** §7.4 lists the tests; the chaos regimen is for the SRE runbook, not this doc.

10. **Billing meter integration.** §6.2 says customer egress is metered; the wiring to the billing pipeline is KAI-291's problem, not ours.

---

*End of memo. Comments welcome inline; please tag with `@ai-platform` on anything that touches §3 or §7.*
