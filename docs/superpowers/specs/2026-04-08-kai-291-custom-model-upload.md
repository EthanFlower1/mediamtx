# KAI-291: Custom AI Model Upload with Sandboxed Execution

Engineering design. Author: lead-ai. Status: revised (engineering-focused).

This document specifies how enterprise customers upload trained ONNX models to run on Kaivue's cloud inference infrastructure. It covers the upload pipeline, sandboxing architecture, threat model, and integration with the existing Triton fleet (KAI-277).

Compliance gates (EU AI Act Art. 25, CMPA, fairness metadata) are deferred to lead-security and will be layered on top of this engineering foundation without changing the runtime architecture.

---

## 1. Scope and non-goals

### In scope

- Cloud-only custom model execution on the EKS GPU fleet (KAI-277 customer-models subpool, `kaivue.io/workload=customer-models`).
- ONNX format exclusively, with a strict operator allowlist. No pickle, no TorchScript, no SavedModel, no custom Python code.
- Upload pipeline: validation, static analysis, content-addressed storage in R2 (KAI-279), Postgres metadata with tenant_id (Seam #10).
- Sandboxed execution: gVisor-isolated pods, one sandbox per (tenant_id, model_id), default-deny networking, resource caps, seccomp enforcement.
- Integration with KAI-233 audit log for uploads, inference, and refusals.
- Per-tenant quotas, GPU-second billing, rate limiting, killswitch.

### Non-goals

- Edge inference on Recorder boxes (KAI-278).
- General-purpose Python code execution. ONNX graph inference only.
- Model training.
- Model marketplace or sharing between tenants.
- Custom pre/post-processing code. Customers provide ONNX graphs with fixed I/O schemas; the platform applies standard pre/post-processing.
- Multi-region active-active (single region us-east-2 in v1).
- Automatic model optimization (TensorRT conversion, quantization) -- deferred to v2.

---

## 2. Sandboxing architecture

### 2.1 gVisor as sandbox kernel

Custom models execute inside gVisor-sandboxed containers. gVisor interposes a user-space kernel (Sentry) between the container and the host kernel, intercepting all syscalls.

| Alternative | Why rejected |
|---|---|
| **Firecracker microVMs** | GPU passthrough not production-ready. NVIDIA device plugin assumes container runtime. |
| **Kata Containers** | Heavier (full guest kernel ~2-5s vs gVisor ~200ms). GPU memory overhead per VM significant. |
| **Plain container + seccomp** | Insufficient. seccomp blocks syscalls but a kernel exploit can escape it. gVisor's Sentry catches this because the host kernel never sees the attacker's syscall. |
| **WASM** | No GPU support. ONNX Runtime does not compile to WASI with CUDA backend. |

gVisor GPU access via `runsc --nvproxy` adds ~5-10% inference latency. Accepted tradeoff.

### 2.2 One sandbox pod per (tenant_id, model_id)

Each loaded custom model runs in its own Kubernetes pod, labeled:

```yaml
kaivue.io/workload: customer-models
kaivue.io/tenant-id: <tenant_id>
kaivue.io/model-id: <model_id>
kaivue.io/sandbox: gvisor
```

Runs on the customer-models subpool (KAI-277 section 3.5: `g5.2xlarge` on-demand, min 1, max 8).

**Pod topology (3 containers):**

1. **Init: `model-loader`.** Pulls model from R2 via KAI-279, verifies content hash, writes to tmpfs `/model`. Exits after write. tmpfs remounted read-only for main container.
2. **Main: `sandbox-runtime`.** ONNX Runtime C++ server mode inside gVisor (`runsc --nvproxy`). Listens on Unix domain socket (not TCP). UDS shared with sidecar via emptyDir.
3. **Sidecar: `inference-proxy`.** Go process (outside gVisor, standard runc) exposing gRPC, authenticating tenant_id, translating to ONNX Runtime over UDS, enforcing per-request timeout, emitting audit events.

### 2.3 Network policy: default-deny egress

- **Ingress:** Allow from `kaivue.io/role=inference-router` on inference gRPC port only.
- **Egress:** Deny all. Zero network access. model-loader has temporary R2 egress during init only; tightened via label mutation after init completes.

### 2.4 Filesystem isolation

- `/model` -- tmpfs, read-only after init. Contains ONNX model file.
- `/scratch` -- tmpfs, read-write, 256 MiB limit. ONNX Runtime temp allocations.
- No host mounts. No PVCs. No node EBS access.
- `/proc`, `/sys` -- gVisor synthetic, no host info exposed.

### 2.5 Resource limits

| Resource | Limit | Rationale |
|---|---|---|
| CPU | 2 cores (request: 1) | Prevents single model from starving others |
| Memory | 4 GiB (request: 2 GiB) | Sufficient for most ONNX models up to 500 MB |
| GPU memory | 25% of A10G (6 GiB of 24 GiB) | Via CUDA MPS `ACTIVE_THREAD_PERCENTAGE=25`. Four models per GPU max. |
| tmpfs `/scratch` | 256 MiB | Prevents disk-bomb attacks |
| tmpfs `/model` | 600 MiB | Headroom above 500 MB model limit |
| Wall-clock per inference | 30 seconds | Hard kill via inference-proxy |
| Pod idle lifetime | 10 minutes | Evicted if no requests. Prevents GPU squatting. |

### 2.6 Seccomp profile

Belt-and-suspenders on top of gVisor:

- Allowlist: ~60 syscalls ONNX Runtime C++ actually uses (profiled).
- Specifically denied: `ptrace`, `mount`, `umount2`, `pivot_root`, `reboot`, `kexec_load`, `init_module`, `finit_module`, `bpf`, `userfaultfd`, `perf_event_open`.
- Violation triggers auto-quarantine of the model + security alert.

---

## 3. Threat model

Four engineering-focused threats. Compliance threats (misclassified models, stolen training data) are tracked separately by lead-security.

### T1: Host filesystem escape via custom op

**Attack:** Crafted ONNX model with a custom operator reads `/etc/passwd`, K8s service account tokens, or NVIDIA driver state.

**Mitigations:**
1. ONNX operator allowlist (section 4). Custom ops rejected at upload.
2. gVisor Sentry -- synthetic filesystem, no host paths exposed.
3. Read-only rootfs, no service account automount.
4. Seccomp profile (belt-and-suspenders).

**Residual:** gVisor sandbox escape CVE + seccomp bypass. Very low likelihood, aggressive patching.

### T2: GPU side-channel across tenants

**Attack:** Tenant A reads residual GPU memory from Tenant B's inference.

**Mitigations:**
1. Customer-models subpool isolation (never shared with system models).
2. CUDA MPS -- separate address spaces per process.
3. Pod-per-model -- separate CUDA contexts, no shared mappings.

**Residual:** CUDA driver vulnerability. Mitigated by patching within 72h for critical CVEs.

### T3: Data exfiltration (model phones home)

**Attack:** Model encodes input data into output tensors or attempts network exfil.

**Mitigations:**
1. Default-deny egress NetworkPolicy. Zero network access.
2. gVisor has no network namespace. `socket()` for AF_INET returns EPERM.
3. Output shape validation against declared schema.
4. Rate limiting bounds covert channel bandwidth.

**Residual:** Steganographic encoding in legitimate output tensors (~200 bytes/s covert channel). Anomaly detection deferred to v2.

### T4: GPU memory exhaustion (DoS)

**Attack:** Model allocates excessive GPU memory, starving other tenants.

**Mitigations:**
1. CUDA MPS thread percentage (25% compute cap).
2. ONNX Runtime arena: `arena_extend_strategy=kSameAsRequested`, `gpu_mem_limit=4GiB`.
3. K8s GPU resource limits via NVIDIA device plugin.
4. 30s wall-clock timeout kills hung inference.
5. System OOMKiller as final backstop.

---

## 4. Upload pipeline

### 4.1 Pre-upload validation

1. **File size:** Hard limit 500 MiB. Enforced at API gateway (`client_max_body_size`) and server-side.
2. **File extension:** Must be `.onnx`. Reject `.pkl`, `.pt`, `.pth`, `.pb`, `.h5`, `.tflite`, `.safetensors`.
3. **Metadata:** `model_name` (unique within tenant), `model_purpose` (enum: `object_detection`, `classification`, `face_recognition`, `license_plate`, `behavioral`, `embedding`, `other`), `deployment_scope` (freetext).

### 4.2 Static analysis (server-side, pre-registration)

Model uploaded to temp R2 prefix (`uploads/pending/<tenant_id>/<upload_id>/`), then analyzed by stateless Go worker:

1. **ONNX format validation.** Parse protobuf. Validate opset 13-21. Reject external data references (all weights embedded in single file).
2. **Operator allowlist.** Walk computational graph. Allow all standard ONNX ops (opset 13-21, ~180 ops). Exclude `If`/`Loop`/`Scan` with unbounded iteration (unless statically provable bounded). Reject any custom domain operator.
3. **Input/output shape validation.** All tensors must have fully specified shapes (dynamic batch dim 0 allowed). Prevents shape-based resource exhaustion.
4. **Known-bad hash list.** SHA-256 checked against platform blocklist.
5. **Graph complexity check.** Reject >10,000 nodes. Reject initializers individually >100 MiB.

### 4.3 Content-addressed push to R2

After analysis passes:

1. Move artifact to `models/<sha256>/model.onnx`. KAI-279 handles dedup.
2. Create signed manifest: content hash, file size, opset version, I/O schema, operator list, analysis results, upload timestamp, tenant_id. Dual-signed (tenant key + platform key).
3. Store manifest at `models/<sha256>/manifest.json.sig`.

### 4.4 Postgres registration

```sql
custom_models:
  id                  UUID (PK)
  tenant_id           UUID (FK -> tenants, NOT NULL)
  model_name          TEXT (unique within tenant)
  content_hash        TEXT (SHA-256, FK -> model_registry)
  model_purpose       ENUM
  status              ENUM {pending_review, approved, active, quarantined, deleted}
  metadata            JSONB (deployment_scope, model_purpose details)
  static_analysis     JSONB (analysis results snapshot)
  uploaded_by         UUID (FK -> users)
  uploaded_at         TIMESTAMPTZ
  approved_by         UUID (nullable)
  approved_at         TIMESTAMPTZ (nullable)
  created_at          TIMESTAMPTZ
  updated_at          TIMESTAMPTZ
```

Status transitions:
- `approved` if static analysis passes cleanly
- `pending_review` if flagged by analysis heuristics
- `approved` -> `active` via explicit tenant activation API call
- Any state -> `quarantined` via killswitch or anomaly detection
- Any state -> `deleted` via tenant or platform action

### 4.5 Notifications (KAI-303)

- Success: "Your model X is ready for activation" or "under review"
- Refusal: reason code + remediation guidance
- Quarantine/kill: notification to tenant admins

---

## 5. Inference request path

### 5.1 API endpoint

```
POST /api/v1/tenants/{tenant_id}/models/{model_id}/infer
```

Auth: Kaivue API key or JWT (KAI-400). `tenant_id` in path must match auth context.

Request:
```json
{
  "inputs": {
    "<input_name>": {
      "shape": [1, 3, 640, 640],
      "dtype": "float32",
      "data": "<base64-encoded tensor>"
    }
  },
  "parameters": {
    "timeout_ms": 5000,
    "priority": "normal"
  }
}
```

Input shapes/dtypes validated against model's registered schema before forwarding to sandbox.

### 5.2 Isolation in request path (defense in depth)

1. Auth middleware extracts `tenant_id` from JWT/API key
2. Path `tenant_id` must match auth context (prevents IDOR)
3. `model_id` must belong to `tenant_id` in `custom_models` table
4. Model must be `active` (quarantined/deleted/pending -> 404)
5. Killswitch check (5s TTL in-process cache) -> 503 if engaged
6. Quota check -> 429 if exhausted
7. Rate limit per-model per-tenant (anti-model-extraction) -> 429
8. Pod routing: locate sandbox for (tenant_id, model_id). Labels must match both. inference-proxy sidecar re-validates.

### 5.3 Sandbox warm-up and lifecycle

- **Cold start:** 15-60s (pod creation, model pull, hash verify, ONNX load). First request held with caller's timeout. 504 if exceeded; pod continues warming.
- **Warm:** Sub-100ms overhead (gRPC -> sidecar -> UDS -> ONNX Runtime). Idle timer reset on each inference.
- **Eviction:** 10 minutes idle -> pod terminated, GPU released. Model remains in R2.
- **Concurrency:** inference-proxy serializes to ONNX Runtime (single-threaded, safe default for untrusted models). Queue depth 10, overflow -> 503.
- **Limits:** Per-tenant: 5 active sandbox pods. Per-node: 4 pods. Platform-wide: 32 (8 nodes x 4).

### 5.4 Response format

```json
{
  "model_id": "...",
  "model_version": "sha256:...",
  "outputs": {
    "<output_name>": {
      "shape": [1, 100, 6],
      "dtype": "float32",
      "data": "<base64-encoded tensor>"
    }
  },
  "metadata": {
    "inference_time_ms": 42,
    "sandbox_pod": "sandbox-<tenant>-<model>-<hash>",
    "gpu_ms": 35,
    "queued_ms": 0
  }
}
```

### 5.5 Audit events (KAI-233)

Per-inference:
```json
{
  "event_type": "custom_model.inference",
  "tenant_id": "...",
  "model_id": "...",
  "model_version": "sha256:...",
  "input_shape": {"image": [1, 3, 640, 640]},
  "output_shape": {"detections": [1, 100, 6]},
  "inference_time_ms": 42,
  "status": "success"
}
```

Input data is NOT logged (privacy). Shapes ARE logged.

Lifecycle events: `upload_initiated`, `upload_refused`, `static_analysis_passed`, `registered`, `activated`, `deactivated`, `deleted`.

---

## 6. Killswitch

Two levels:

1. **Per-model killswitch.** Tenant admin or platform admin can deactivate a specific model. Transition `active` -> `quarantined`. Sandbox pod terminated within 60 seconds (graceful drain of in-flight requests, then SIGKILL). Cached killswitch state in inference-proxy with 5s TTL.
2. **Per-tenant killswitch.** Platform admin can disable all custom model inference for a tenant. All sandbox pods terminated. Useful for abuse response.

Both emit KAI-233 audit events.

---

## 7. Quotas and billing

- **Upload quota:** Per-tenant: max 20 models (configurable per plan tier). Max 5 GiB total model storage.
- **Inference quota:** Per-tenant monthly GPU-second budget. Metered via inference-proxy (wall-clock time x GPU fraction). Reported to KAI-364 billing pipeline.
- **Rate limits:** Per-model: 100 req/s. Per-tenant: 500 req/s. Both configurable.

---

## 8. Implementation plan

### Phase 1: Upload pipeline + static analysis (1 sprint)

- `internal/cloud/models/upload.go` -- upload handler with pre-validation
- `internal/cloud/models/analyzer.go` -- ONNX static analyzer (protobuf parse, operator walk, shape validation, hash check)
- `custom_models` Postgres table + migration
- R2 content-addressed storage integration (via KAI-279)
- Unit tests for analyzer (crafted ONNX test fixtures)

### Phase 2: Sandbox runtime (1 sprint)

- Kubernetes pod template (gVisor RuntimeClass, 3-container topology)
- `inference-proxy` sidecar (Go, gRPC -> UDS translation, timeout enforcement)
- `model-loader` init container (R2 pull, hash verify, tmpfs write)
- NetworkPolicy manifests
- Seccomp profile
- Integration test: upload model -> cold-start inference -> response

### Phase 3: Inference routing + lifecycle (1 sprint)

- Inference router extension (locate/create sandbox pods for custom models)
- Warm-up, idle eviction, concurrent request queue
- Killswitch implementation
- Quota/rate-limit enforcement
- Billing metering integration (KAI-364)
- End-to-end integration test with real YOLO model

### Dependencies

- KAI-277 (Triton fleet) -- customer-models subpool must be provisioned
- KAI-279 (Model registry) -- R2 storage + signed manifests
- KAI-233 (Audit log) -- event emission
- KAI-400 (API keys) -- authentication
- KAI-303 (Notifications) -- upload status notifications
- KAI-364 (Billing) -- GPU-second metering

---

## 9. Open questions

1. Should we support opset versions below 13? Some older models use opset 11-12.
2. Should the model-loader verify signatures using the tenant's public key, or is content hash sufficient for v1?
3. MIG vs MPS for GPU memory isolation -- MIG provides hardware isolation but requires A30/A100. Worth the cost delta at v1 scale?
