# KAI-291: Custom AI Model Upload with Sandboxed Execution

Design memo. Author: ai-platform working group. Reviewers: lead-security, lead-cloud, lead-ai, counsel. Status: draft for review.

This document specifies how enterprise customers upload their own trained AI models to run on Kaivue's cloud inference infrastructure. It covers the upload pipeline, sandboxing architecture, EU AI Act Art. 25 compliance, threat model, and integration with the existing Triton fleet (KAI-277). It does not ship code.

---

## 1. Scope and non-goals

### In scope

- Cloud-only custom model execution on the EKS GPU fleet (KAI-277 customer-models subpool, `kaivue.io/workload=customer-models`).
- ONNX format exclusively, with a strict operator allowlist. No pickle, no TorchScript, no SavedModel, no custom Python code.
- Upload pipeline: validation, static analysis, content-addressed storage in R2 (KAI-279), Postgres metadata with tenant_id (Seam #10), Art. 25 attestation linkage.
- Sandboxed execution: gVisor-isolated pods, one sandbox per (tenant_id, model_id), default-deny networking, resource caps, seccomp enforcement.
- EU AI Act Art. 25 Modifier compliance flow: terms click-through, metadata collection, refusal rules, audit trail.
- Integration with KAI-233 audit log for every upload, every inference, every refusal.
- Per-tenant quotas, GPU-second billing, rate limiting, killswitch.
- Honoring Seam #4 (per-tenant vector index for embedding models), Seam #8 (R2 model storage), Seam #10 (tenant_id everywhere).

### Non-goals

- Edge inference on Recorder boxes. ONNX Runtime on-prem is KAI-278. Cloud-only at v1.
- General-purpose user Python code execution. No Lambda-style custom code; only ONNX graph inference.
- Model training. Inference only.
- Model marketplace or sharing between tenants. Each tenant's models are private.
- Custom pre/post-processing code. Customers provide ONNX graphs with fixed input/output schemas; Kaivue applies standard pre/post-processing (resize, normalize, NMS) from a configurable but platform-controlled library.
- Multi-region active-active for custom model inference. Single region (us-east-2) in v1; Seam #8 does not foreclose expansion.
- Automatic model optimization (TensorRT conversion, quantization). Deferred to KAI-291-v2.

---

## 2. Art. 25 Modifier compliance (EU AI Act)

This section is the legal and UX backbone. Under Art. 25, when a customer uploads their own AI model to run on Kaivue infrastructure, a "substantial modification" occurs. The customer becomes the "provider" of that specific model under Art. 16. Kaivue remains the platform provider. This distinction is load-bearing for liability, conformity assessment, and documentation obligations.

### 2.1 Upload terms-of-service click-through

Every model upload requires a one-time-per-model acceptance of the **Custom Model Provider Agreement** (CMPA). The CMPA is a legal document (drafted by counsel, versioned, stored in Postgres) that explicitly transfers Art. 16 provider obligations to the uploading customer. The acceptance flow:

1. Customer initiates upload via API or UI.
2. System checks whether the customer has accepted the current CMPA version for this model category. If not, the upload is blocked and the customer is presented the CMPA.
3. Customer must affirmatively accept (checkbox + signature timestamp, not a passive scroll-past). The acceptance is recorded as a KAI-233 audit event with: `tenant_id`, `user_id`, `cmpa_version`, `timestamp`, `ip_address`, `model_category` (general / biometric / high-risk).
4. Acceptance is per-tenant, per-CMPA-version. A CMPA version bump re-triggers acceptance on next upload.
5. The CMPA acceptance record is linked to every model uploaded under it via `cmpa_acceptance_id` in the model metadata (Postgres).

The CMPA must state, at minimum:

- The customer is the "provider" of their uploaded model under EU AI Act Art. 16.
- The customer is responsible for Art. 9 (risk management), Art. 10 (data governance), Art. 13 (transparency), Art. 15 (accuracy/robustness/cybersecurity) for their model.
- Kaivue provides the execution platform but does not validate the model's training data, fairness properties, or fitness for purpose.
- Kaivue reserves the right to refuse, quarantine, or terminate any model that violates platform policy or applicable law.

### 2.2 Required customer metadata per model upload

At upload time, the customer must provide structured metadata. This is not optional decoration; it is the minimum documentation required for a downstream Art. 25 defense:

| Field | Type | Required | Notes |
|---|---|---|---|
| `model_name` | string | yes | Human-readable, unique within tenant |
| `model_purpose` | enum + freetext | yes | Enum: `object_detection`, `classification`, `face_recognition`, `license_plate`, `behavioral`, `embedding`, `other`. Freetext elaboration required for `other`. |
| `training_data_provenance` | structured | yes | Source description, consent basis (GDPR Art. 6), whether biometric data was used, whether minors' data was used. |
| `fairness_metrics` | structured | conditional | Required if `model_purpose` is `face_recognition` or `classification`. Must include demographic parity / equalized odds metrics or an explicit "not evaluated" with justification. |
| `deployment_scope` | freetext | yes | Where and how the model will be used (e.g., "forklift detection in warehouse zone 3"). |
| `intended_users` | freetext | yes | Who will consume the model's outputs and how. |
| `known_limitations` | freetext | yes | Edge cases, failure modes, populations where the model underperforms. |
| `art25_attestation` | boolean | conditional | Required for Annex III high-risk use cases. Customer attests they have completed their own Art. 16 conformity assessment. |

### 2.3 Refusal rules

The platform refuses uploads in the following cases. Every refusal is a KAI-233 audit event.

**Hard refusals (upload rejected, no override):**

- R1: Model purpose maps to an Art. 5 prohibited practice (social scoring, real-time remote biometric ID in public spaces for law enforcement without Art. 5(1)(h) exemption, emotion recognition in workplace/education, subliminal manipulation). The enum + freetext analysis flags these. Human review queue for ambiguous cases.
- R2: Model purpose is Annex III high-risk (biometric identification, critical infrastructure, employment, education, law enforcement, migration, justice, democratic processes) AND `art25_attestation` is false or missing. The customer has not completed their Art. 16 conformity assessment. Upload blocked until attestation is provided.
- R3: `training_data_provenance` indicates biometric data from minors. Absolute block per KAI-282 B4.
- R4: Model fails static analysis (see section 5). Format invalid, operator not on allowlist, size exceeds limit.

**Soft refusals (upload quarantined, requires manual review):**

- R5: Static analysis heuristics flag the model as likely face-recognition (output shape matches embedding vector, activation patterns consistent with ArcFace/FaceNet architectures) but `model_purpose` is not `face_recognition`. Quarantined for human review. See threat T6.
- R6: Model purpose is `face_recognition` but tenant has not enabled the face recognition feature (KAI-282 governance). Upload quarantined until tenant completes KAI-282 onboarding.

### 2.4 Custom face models inherit KAI-282 B1-B7

Any model uploaded with `model_purpose = face_recognition` (or reclassified as such after R5 review) inherits all seven boundary conditions from KAI-282:

- **B1: Per-tenant vault.** Embeddings produced by the custom face model are stored in the tenant's isolated vector index (Seam #4), never in a shared index.
- **B2: Off by default.** The model is not activated until the tenant explicitly enables it in their face recognition settings.
- **B3: consent_record_id.** Every face enrollment using the custom model must reference a valid consent record. The platform enforces this at the API layer, regardless of which model (system or custom) produces the embedding.
- **B4: Minors blocked.** The platform refuses to enroll faces flagged as minors, regardless of model.
- **B5: CSE-CMK via KAI-251.** Face embeddings are encrypted with the tenant's customer-supplied encryption key. Custom models do not get an exemption.
- **B6: Killswitch <= 60s.** The tenant and platform can kill the custom face model within 60 seconds. Implemented identically to system face models (see section 8).
- **B7: Every operation audited via KAI-233.** Every inference, enrollment, search, and deletion using the custom face model emits a structured audit event.

### 2.5 Audit trail

The following events are logged to KAI-233 for every custom model lifecycle action:

- `custom_model.cmpa_accepted` — CMPA acceptance with version, tenant, user, timestamp.
- `custom_model.upload_initiated` — upload start with metadata snapshot.
- `custom_model.upload_refused` — refusal with reason code (R1-R6).
- `custom_model.upload_quarantined` — quarantine with reason code.
- `custom_model.upload_approved` — manual review approval (for quarantined models).
- `custom_model.static_analysis_passed` — analysis results summary.
- `custom_model.registered` — model registered in KAI-279 with content hash.
- `custom_model.activated` — model made available for inference.
- `custom_model.deactivated` — model taken offline (manual or killswitch).
- `custom_model.deleted` — model removed from registry and R2.
- `custom_model.inference` — per-invocation audit (see section 6).

---

## 3. Sandboxing architecture

### 3.1 Decision: gVisor as sandbox kernel

Custom models execute inside gVisor-sandboxed containers. gVisor interposes a user-space kernel (Sentry) between the container and the host kernel, intercepting all syscalls. This is the primary isolation boundary for untrusted model execution.

**Why gVisor over alternatives:**

| Alternative | Why rejected |
|---|---|
| **Firecracker microVMs** | Stronger isolation (full VM), but GPU passthrough is not production-ready in Firecracker. NVIDIA device plugin assumes container runtime, not microVM. Would require a custom device plugin and driver injection. Revisit when Firecracker GPU support matures. |
| **Kata Containers** | VM-based, supports GPU passthrough via VFIO. Heavier than gVisor (full guest kernel boot ~2-5s vs gVisor ~200ms). GPU memory overhead per VM is significant. Could work but adds operational complexity we do not need at v1 scale. |
| **Plain container + seccomp** | Insufficient. seccomp blocks syscalls but does not rewrite them. A compromised ONNX Runtime process with a kernel exploit can escape seccomp. gVisor's Sentry catches this class of attack because the host kernel never sees the attacker's syscall. |
| **WASM (Wasmtime/WasmEdge)** | No GPU support. ONNX Runtime does not compile to WASI with CUDA backend. Non-starter for inference workloads. |

gVisor's limitation: GPU access via `runsc` requires the `--nvproxy` flag (gVisor's NVIDIA GPU proxy), which intercepts CUDA calls. This adds ~5-10% inference latency overhead. Accepted tradeoff for the isolation guarantee.

### 3.2 One sandbox pod per (tenant_id, model_id)

Each loaded custom model runs in its own Kubernetes pod. The pod is labeled:

```
kaivue.io/workload: customer-models
kaivue.io/tenant-id: <tenant_id>
kaivue.io/model-id: <model_id>
kaivue.io/sandbox: gvisor
```

The pod runs on the customer-models subpool (KAI-277 section 3.5, section 5.1: `g5.2xlarge` on-demand, min 1, max 8, labeled `kaivue.io/workload=customer-models`). Node affinity ensures custom model pods never schedule on system model nodes.

Pod topology:

- **Init container: `model-loader`.** Pulls the model artifact from R2 via KAI-279, verifies content hash and signature, writes to a tmpfs volume at `/model`. Exits after write. The tmpfs is then remounted read-only for the main container.
- **Main container: `sandbox-runtime`.** Runs ONNX Runtime (C++ server mode, not Python) inside gVisor (`runsc` with `--nvproxy`). Listens on a Unix domain socket (not TCP) for inference requests. The UDS is mounted from an emptyDir shared with the sidecar.
- **Sidecar container: `inference-proxy`.** A Go process (internal) that exposes a gRPC endpoint on the pod's network, authenticates incoming requests (tenant_id must match the pod's label), translates to ONNX Runtime's inference protocol over UDS, enforces per-request wall-clock timeout, and emits KAI-233 audit events. This sidecar runs OUTSIDE gVisor (standard runc) because it needs network access.

### 3.3 Network policy: default-deny egress

A Kubernetes NetworkPolicy attached to every sandbox pod:

- **Ingress:** Allow from pods with label `kaivue.io/role=inference-router` on the inference gRPC port only. Deny all else.
- **Egress:** Deny all. The sandbox container has zero network access. It cannot phone home, cannot reach the Kubernetes API, cannot resolve DNS. The model-loader init container has temporary egress to R2 (scoped to the R2 endpoint IP range) during init only; the NetworkPolicy is tightened after init completes via a label mutation by the inference-proxy sidecar.

### 3.4 Filesystem isolation

- `/model` — tmpfs, remounted read-only after init. Contains the ONNX model file and config. No writes allowed post-init.
- `/scratch` — tmpfs, read-write, size-limited to 256 MiB. ONNX Runtime uses this for temporary allocations. Wiped on pod restart.
- No host filesystem mounts. No PersistentVolumeClaims. No access to node EBS.
- `/proc`, `/sys` — gVisor's synthetic procfs/sysfs. Does not expose host information.

### 3.5 Resource limits

| Resource | Limit | Rationale |
|---|---|---|
| CPU | 2 cores (request: 1) | ONNX Runtime pre/post-processing is CPU-bound. 2 cores prevents a single model from starving others on the node. |
| Memory | 4 GiB (request: 2 GiB) | Sufficient for most ONNX models up to 500 MB with activation memory. OOM-kill if exceeded. |
| GPU memory | Fractional: 25% of A10G (6 GiB of 24 GiB) | Enforced via NVIDIA MPS (Multi-Process Service) or, if A10G MIG is unavailable (it is: see section 10), via `CUDA_MPS_ACTIVE_THREAD_PERCENTAGE=25`. Four tenant models per GPU maximum. |
| tmpfs (`/scratch`) | 256 MiB | Prevents disk-bomb attacks. |
| tmpfs (`/model`) | 600 MiB | Slightly above the 500 MB model size limit to accommodate ONNX Runtime's memory-mapped loading overhead. |
| Wall-clock per inference | 30 seconds | Hard kill via the inference-proxy sidecar. Prevents infinite-loop custom ops. |
| Pod lifetime (idle) | 10 minutes | Evicted if no inference request in 10 minutes. Prevents idle GPU squatting. |

### 3.6 Seccomp profile

The gVisor Sentry already restricts syscalls, but we apply a belt-and-suspenders seccomp profile on the sandbox container as a second layer:

- Allowlist: the ~60 syscalls ONNX Runtime C++ server actually uses (determined by profiling). Everything else returns EPERM.
- Specifically denied (even if gVisor would allow): `ptrace`, `mount`, `umount2`, `pivot_root`, `reboot`, `kexec_load`, `init_module`, `finit_module`, `bpf`, `userfaultfd`, `perf_event_open`.
- Violation logging: seccomp audit log events are forwarded to the platform's security event pipeline. A single violation triggers auto-quarantine of the model (see section 8).

---

## 4. Threat model

Six concrete threats with specific mitigations.

### T1: Malicious custom op reads /etc/passwd (host filesystem escape)

**Attack:** An attacker crafts an ONNX model with a custom operator that attempts to read arbitrary host files (e.g., `/etc/passwd`, Kubernetes service account tokens at `/var/run/secrets/kubernetes.io/serviceaccount/token`, or NVIDIA driver state in `/proc`).

**Mitigation chain:**

1. **ONNX operator allowlist (section 5).** Custom ops are not on the allowlist. The static analyzer rejects any model containing a non-standard ONNX op before it reaches the sandbox. This is the primary defense.
2. **gVisor Sentry.** Even if an allowlisted op has a file-read vulnerability, gVisor's synthetic filesystem does not expose host paths. `/etc/passwd` inside gVisor contains only the sandbox user. `/var/run/secrets/` is not mounted (service account automount is disabled on sandbox pods). `/proc` is gVisor's synthetic procfs.
3. **Read-only filesystem.** The sandbox container's rootfs is read-only. `/model` is read-only. `/scratch` is the only writable mount and it is tmpfs (no host backing).
4. **seccomp profile.** Even if gVisor were bypassed, the seccomp profile blocks `open` on paths outside `/model` and `/scratch` (path-based seccomp filtering via `openat2` restriction is fragile; we rely on gVisor here, seccomp is belt-and-suspenders).

**Residual risk:** A gVisor sandbox escape (CVE in Sentry) combined with a seccomp bypass. Severity: critical. Likelihood: very low (gVisor's track record, plus we pin versions and patch aggressively). Monitoring: sandbox crash or seccomp violation triggers immediate alert (section 8).

### T2: GPU side-channel across tenants

**Attack:** Tenant A's model reads residual GPU memory from Tenant B's previous inference via a CUDA memory reuse vulnerability or a Rowhammer-style attack on GPU DRAM.

**Mitigation chain:**

1. **Customer-models subpool isolation (KAI-277 section 3.5).** Custom models never share a GPU with system models. The blast radius is limited to other custom models on the same node.
2. **CUDA MPS thread percentage limits (section 3.5).** Each sandbox gets a capped fraction of GPU compute. MPS provides process-level GPU memory isolation (separate address spaces). Tenant A's CUDA context cannot address Tenant B's GPU memory.
3. **Pod-per-model topology.** Each model is a separate process with a separate CUDA context. No shared GPU memory mappings.
4. **`CUDA_LAUNCH_BLOCKING=1` in debug mode.** For security audits, we can enable synchronous CUDA execution to inspect memory state between inferences. Not enabled in production (performance hit).

**Residual risk:** A CUDA driver vulnerability that leaks cross-context GPU memory. NVIDIA patches these; we track NVIDIA security bulletins and patch within 72 hours for critical CVEs. A future upgrade to MIG-capable GPUs (A30, A100) would provide hardware-level memory isolation. See section 10.

### T3: Model phones home to C2 (data exfiltration)

**Attack:** A model is crafted to encode inference input data (e.g., camera frames containing faces) into its output tensors in a steganographic pattern, or attempts direct network exfiltration.

**Mitigation chain:**

1. **Default-deny egress NetworkPolicy (section 3.3).** The sandbox container has zero network access. DNS resolution fails. TCP/UDP connections fail. This is the hard stop for direct exfiltration.
2. **No network access inside gVisor.** Even without NetworkPolicy, the gVisor sandbox has no network namespace configured. `socket()` calls for AF_INET/AF_INET6 return EPERM.
3. **Output tensor shape validation (section 5).** Output tensors must match the declared schema. Anomalously large output tensors (e.g., a "duck detector" returning a 512-dimensional float vector instead of a bounding box) are flagged.
4. **Rate limiting (section 9).** Even if steganographic exfiltration via output tensors is attempted, the rate limit on inference calls bounds the bandwidth.

**Residual risk:** Steganographic encoding in legitimately-shaped output tensors. A bounding-box output has ~20 bytes of payload per detection; at 10 inferences/second that is 200 bytes/second of covert channel. Extremely low bandwidth, but nonzero. Mitigation: anomaly detection on output entropy (deferred, not v1).

### T4: Unbounded GPU memory consumption

**Attack:** A model is crafted to allocate excessive GPU memory during inference, either through large intermediate activations or by exploiting ONNX Runtime's memory allocator. This starves other tenants' models on the same GPU.

**Mitigation chain:**

1. **CUDA MPS thread percentage (section 3.5).** Caps compute at 25% of GPU. However, MPS does not hard-cap memory allocation (it caps compute scheduling, not memory).
2. **ONNX Runtime arena allocator configuration.** Set `arena_extend_strategy=kSameAsRequested` (no speculative allocation) and `gpu_mem_limit=4294967296` (4 GiB hard cap, enforced by ONNX Runtime's CUDA allocator). If the model tries to allocate beyond 4 GiB, ONNX Runtime returns an OOM error.
3. **Kubernetes GPU resource limits.** The pod requests a fractional GPU share. If the CUDA allocator bypasses ONNX Runtime's limit (bug), the Kubernetes device plugin's memory enforcement (via `nvidia.com/gpu` resource and MPS) provides a second layer.
4. **Wall-clock timeout (30s).** If GPU memory allocation causes the inference to hang, the timeout kills the request.
5. **OOM-kill.** System-level OOMKiller terminates the pod if total memory (CPU + GPU mapped) exceeds the pod's cgroup limit.

**Residual risk:** A CUDA driver bug that allows memory allocation beyond MPS limits. Same mitigation as T2: aggressive patching.

### T5: Model trained on stolen biometric data (Art. 10 violation)

**Attack:** A customer uploads a face recognition model trained on a dataset obtained without GDPR Art. 6 legal basis (e.g., scraped from social media, Clearview AI-style). Kaivue hosts the model, becoming complicit in an Art. 10 data governance violation.

**Mitigation chain:**

1. **Art. 25 CMPA (section 2.1).** The customer assumes Art. 16 provider obligations, including Art. 10 data governance. Kaivue is the platform provider, not the model provider.
2. **Training data provenance metadata (section 2.2).** The customer must declare the data source and consent basis. "Scraped from the internet" or "unknown" triggers manual review.
3. **Fairness metrics requirement (section 2.2).** For face models, demographic parity metrics are required. A model trained on a biased/stolen dataset is unlikely to have these metrics, which forces either honest disclosure or fabrication (the latter is a contractual violation with legal consequences).
4. **R5 soft refusal (section 2.3).** If a model claims to be a "duck detector" but has face-recognition output characteristics, it is quarantined for review.
5. **Platform right to terminate.** The CMPA and Kaivue's ToS reserve the right to immediately terminate any model if Kaivue receives a credible report that training data was unlawfully obtained.

**Residual risk:** A customer lies about training data provenance. Kaivue cannot verify training data after the fact. The CMPA transfers legal liability. If a regulator investigates, Kaivue can demonstrate: (a) the customer assumed Art. 16 obligations, (b) Kaivue collected provenance metadata, (c) Kaivue did not have actual knowledge of the violation. This is the Art. 25(3) defense.

### T6: Model claims "duck detector" but is actually face recognizer (governance bypass)

**Attack:** A customer uploads a model declared as `object_detection` (purpose: "detect ducks in pond") but the model is actually a face recognition model. This bypasses KAI-282 governance (B1-B7), face vault isolation, consent requirements, and the face recognition killswitch.

**Mitigation chain:**

1. **Static analysis heuristics (section 5).** The upload pipeline runs output shape analysis: if the model's output tensor is a 128/256/512-dimensional float vector (consistent with face embedding architectures like ArcFace, FaceNet, SphereFace), it is flagged regardless of declared purpose. Additional heuristic: if the model's input shape is 112x112 or 160x160 (canonical face crop sizes), flag it.
2. **R5 soft refusal (section 2.3).** Flagged models are quarantined for human review by the AI platform team. The reviewer can: (a) approve as non-face, (b) reclassify as face recognition (triggers KAI-282 onboarding requirement), (c) reject.
3. **Runtime output monitoring (deferred, v2).** In v2, the inference-proxy sidecar will periodically sample outputs and run a lightweight classifier to detect face-embedding-like output distributions. Not in v1 due to performance and false-positive concerns.
4. **Audit trail.** If the model is later discovered to be a face recognizer, the full audit trail (upload metadata, static analysis results, inference logs) supports retroactive enforcement and regulatory response.

**Residual risk:** A sufficiently adversarial model that disguises its face-embedding output (e.g., by applying a reversible linear transform to the embedding before output, making dimension/magnitude heuristics fail). This is a cat-and-mouse game. The CMPA makes misrepresentation a contractual violation with termination and legal consequences. The static analysis catches naive attempts; sophisticated adversaries are deterred by the legal framework rather than the technical one.

---

## 5. Upload pipeline

### 5.1 Pre-upload validation (client-side + API gateway)

Before the model binary reaches R2:

1. **File size check.** Hard limit: 500 MiB. Enforced at the API gateway (nginx `client_max_body_size`) and re-checked server-side. Models larger than 500 MiB are rejected with `413 Payload Too Large`. Rationale: the largest common ONNX models (YOLOv8-X, EfficientNet-B7) are under 300 MiB. 500 MiB provides headroom without enabling abuse.
2. **File extension check.** Must be `.onnx`. Reject `.pkl`, `.pt`, `.pth`, `.pb`, `.h5`, `.tflite`, `.safetensors` with a specific error message directing the customer to convert to ONNX.
3. **MIME type check.** Content-Type must be `application/octet-stream` or `application/x-onnx`. Not relied upon for security (trivially spoofed) but catches accidental uploads.
4. **Metadata completeness check.** All required fields from section 2.2 must be present. Reject with `422 Unprocessable Entity` listing missing fields.
5. **CMPA acceptance check.** Current CMPA version must be accepted by the tenant. Reject with `403 Forbidden` and a link to the CMPA acceptance flow.

### 5.2 Static analysis (server-side, pre-registration)

The model binary is uploaded to a temporary R2 prefix (`uploads/pending/<tenant_id>/<upload_id>/`) and then analyzed by a stateless analysis worker (Go service, no GPU required):

1. **ONNX format validation.** Parse the file as an ONNX protobuf. Reject if parsing fails. Validate against the ONNX spec (opset version must be in the supported range: opset 13-21 at launch). Reject if the file contains external data references (all weights must be embedded in the single `.onnx` file; no external data files in v1).

2. **Operator allowlist.** The model's computational graph is walked. Every operator must be on the platform's allowlist. The allowlist at launch:

   - All standard ONNX operators from opset 13-21 (approximately 180 operators).
   - Specifically EXCLUDED: `If`, `Loop`, `Scan` with unbounded iteration (these enable Turing-complete graphs that could infinite-loop). `If` and `Loop` are allowed only if the graph's static analysis can prove bounded iteration (e.g., loop count is a constant). This is conservative; some legitimate models use `Loop` for dynamic sequence processing. Customers with legitimate need can request an exception via support, which triggers manual review.
   - NO custom operators (domain != "ai.onnx" and domain != "ai.onnx.ml" and domain != ""). Any custom domain operator triggers rejection.

3. **Input/output tensor shape validation.** All input and output tensors must have fully specified shapes (no dynamic dimensions except batch dimension, which must be dim 0). Rationale: dynamic shapes from untrusted input enable shape-based resource exhaustion (e.g., a model that accepts a 100000x100000 input tensor). The platform normalizes inputs to the declared shape before passing to the model; shape mismatches are caught at inference time, not at the ONNX level.

4. **Face-recognition heuristic scan.** The analyzer checks:
   - Output tensor shape: if any output is a 1D float tensor with dimension in {64, 128, 256, 512, 1024}, flag as potential face embedding.
   - Input tensor shape: if input is a 3-channel image with spatial dimensions in {112x112, 160x160, 224x224} AND an embedding-like output exists, increase confidence.
   - Model metadata: check ONNX model metadata (`model.metadata_props`) for keywords: "face", "arcface", "facenet", "insightface", "recognition", "embedding", "biometric".
   - If confidence exceeds threshold: trigger R5 soft refusal (section 2.3).

5. **Known-bad hash list.** The model's SHA-256 content hash is checked against a platform-maintained blocklist of known-malicious model hashes (e.g., models previously rejected for policy violations, models identified by security researchers as containing exploits). Reject on match.

6. **Model graph complexity check.** Reject models with more than 10,000 nodes (prevents graph-bomb denial-of-service during loading). Reject models with more than 1,000 initializers (weight tensors) individually larger than 100 MiB (prevents single-tensor memory bombs).

### 5.3 Content-addressed push to R2 (KAI-279)

After static analysis passes:

1. The model artifact is moved from the pending prefix to the content-addressed store: `models/<sha256>/model.onnx`. KAI-279 handles dedup (if two tenants upload identical models, one copy exists in R2).
2. A model manifest is created and signed. The manifest contains: content hash (SHA-256), file size, opset version, input/output schema, operator list, static analysis results, upload timestamp, tenant_id. The manifest is signed with the tenant's signing key (provisioned at tenant onboarding, managed by KAI-279). The platform co-signs with its own key.
3. The signed manifest is stored alongside the model in R2: `models/<sha256>/manifest.json.sig`.

### 5.4 Postgres registration

A row is inserted into `custom_models` (Postgres, tenant_id-partitioned per Seam #10):

```
custom_models:
  id                  UUID (PK)
  tenant_id           UUID (FK → tenants, NOT NULL)
  model_name          TEXT (unique within tenant)
  content_hash        TEXT (SHA-256, FK → model_registry)
  model_purpose       ENUM
  status              ENUM {pending_review, approved, active, quarantined, deleted}
  cmpa_acceptance_id  UUID (FK → cmpa_acceptances)
  art25_attestation   BOOLEAN
  metadata            JSONB (training_data_provenance, fairness_metrics, deployment_scope, etc.)
  static_analysis     JSONB (analysis results snapshot)
  uploaded_by         UUID (FK → users)
  uploaded_at         TIMESTAMPTZ
  approved_by         UUID (nullable, FK → users)
  approved_at         TIMESTAMPTZ (nullable)
  created_at          TIMESTAMPTZ
  updated_at          TIMESTAMPTZ
```

The model starts in `pending_review` if any soft refusal (R5, R6) was triggered, or `approved` if static analysis passed cleanly. Transition to `active` requires explicit activation by the tenant (separate API call). Transition to `quarantined` can happen at any time via killswitch or automated anomaly detection.

### 5.5 Notification (KAI-303)

- On successful registration: push notification to the uploading user ("Your model X is ready for activation" or "Your model X is under review").
- On refusal: push notification with reason code and remediation guidance.
- On quarantine/kill: push notification to tenant admins with reason and contact link.

---

## 6. Inference request path

### 6.1 API endpoint

```
POST /api/v1/tenants/{tenant_id}/models/{model_id}/infer
```

Authentication: Kaivue API key or JWT (KAI-400). The `tenant_id` in the path must match the authenticated principal's tenant. The SDK enforces this (KAI-277 section 4.4 pattern: tenant from auth context, not from user input).

Request body:

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

The platform validates input tensor shapes and dtypes against the model's registered schema BEFORE forwarding to the sandbox. Shape mismatches are rejected with `400 Bad Request`. This prevents the sandbox from receiving malformed inputs that could trigger unexpected behavior in ONNX Runtime.

### 6.2 Tenant isolation in the request path

The inference request crosses these isolation boundaries (defense in depth, similar to KAI-277 section 7.2 but adapted for sandbox architecture):

1. **Auth middleware.** Extracts `tenant_id` from JWT/API key. Request rejected if auth fails.
2. **Path parameter validation.** `tenant_id` in path must match auth context tenant. Prevents IDOR.
3. **Model ownership check.** `model_id` must belong to `tenant_id` in the `custom_models` table. Separate tenants' models are never accessible cross-tenant.
4. **Model status check.** Model must be `active`. Quarantined, deleted, or pending models return `404`.
5. **Killswitch check.** In-process cache (5s TTL) checks tenant-level and platform-level killswitch state. If engaged, return `503 Service Unavailable` with `reason=killswitch`. Logged to KAI-233.
6. **Quota check.** Tenant's monthly inference budget is checked. If exhausted, return `429 Too Many Requests` with `Retry-After` header (section 9).
7. **Rate limit check.** Per-model per-tenant rate limit (anti-model-extraction). If exceeded, return `429`.
8. **Pod routing.** The inference router locates the sandbox pod for `(tenant_id, model_id)`. If no pod exists (cold start), one is created (see section 6.3). The router ONLY routes to pods whose labels match both `tenant_id` and `model_id`. A routing bug that sends a request to the wrong pod is caught by the inference-proxy sidecar, which re-validates tenant_id and model_id against its own pod labels.
9. **Inference-proxy sidecar.** Receives the gRPC request, re-validates tenant_id and model_id, translates to ONNX Runtime protocol over UDS, enforces wall-clock timeout, returns response.

### 6.3 Sandbox warm-up and lifecycle

- **Cold start.** On first inference for a (tenant_id, model_id) pair, the inference router creates a sandbox pod. The pod goes through init (model pull from R2, hash verification, signature verification) and then ONNX Runtime loads the model. Total cold-start latency: 15-60 seconds depending on model size. The first inference request is held (with the caller's timeout) during warm-up. If warm-up exceeds the caller's timeout, the request fails with `504 Gateway Timeout`; the pod continues warming for the next request.
- **Warm state.** After the first inference, the pod is kept alive for `warm_keep_minutes` (default: 10). Every inference resets the idle timer. The pod serves all subsequent requests for this (tenant_id, model_id) with sub-100ms overhead (gRPC to sidecar + UDS to ONNX Runtime).
- **Eviction.** After `warm_keep_minutes` of idle, the pod is terminated. GPU resources are released. The model artifact remains in R2; only the running sandbox is destroyed.
- **Concurrent requests.** The inference-proxy sidecar serializes requests to ONNX Runtime (single-threaded inference, which is the safe default for untrusted models — prevents concurrent-access bugs). Queued requests are served FIFO. Queue depth limit: 10. Requests beyond the queue limit return `503 Service Unavailable`.
- **Maximum concurrent sandbox pods.** Per-tenant limit: 5 active sandbox pods. Per-node limit: 4 pods (matching the 25% GPU fraction in section 3.5). Platform-wide limit: bounded by the customer-models subpool node count (max 8 nodes x 4 pods = 32 concurrent sandbox pods platform-wide).

### 6.4 Response format

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

The response includes timing metadata for debugging and billing. The `model_version` is the content hash, enabling the caller to detect stale model versions.

### 6.5 Per-inference audit event (KAI-233)

Every inference emits a structured audit event:

```json
{
  "event_type": "custom_model.inference",
  "tenant_id": "...",
  "model_id": "...",
  "model_version": "sha256:...",
  "user_id": "...",
  "input_shape": {"image": [1, 3, 640, 640]},
  "output_shape": {"detections": [1, 100, 6]},
  "inference_time_ms": 42,
  "status": "success",
  "timestamp": "..."
}
```

Input data is NOT logged (privacy). Input/output shapes ARE logged (compliance, anomaly detection).

### 6.6 Anti-model-extraction rate limit

To prevent a competitor or attacker from reconstructing a customer's proprietary model by querying it with crafted inputs and observing outputs (model extraction attack):

- Per-model per-tenant rate limit: 100 inferences/minute (default, configurable per tenant).
- Per-model per-tenant daily limit: 10,000 inferences/day (default).
- Burst: token bucket, 20 tokens, refill 100/minute.
- Rate limit headers in response: `X-RateLimit-Limit`, `X-RateLimit-Remaining`, `X-RateLimit-Reset`.
- Exceeded: `429 Too Many Requests` with `Retry-After`.

These defaults are generous for production use (a camera at 10 FPS = 600/min would exceed the default; the tenant adjusts their limit up). The defaults are set to prevent extraction attacks from API keys with low-privilege access.

---

## 7. Integration with Triton (KAI-277)

### 7.1 Decision: custom models do NOT run inside Triton

Custom models run in separate sandboxed pods (section 3), not inside the Triton deployment described in KAI-277. The Triton fleet serves only system models (Kaivue-built face, LPR, behavior, object detection) and is shared across tenants with the namespace isolation described in KAI-277 section 7.

### 7.2 Justification

Triton's Python backend and ONNX backend both execute model code in the Triton server process. A malicious ONNX model that exploits a vulnerability in ONNX Runtime (within Triton's process) would compromise the entire Triton pod, which serves models for ALL tenants. The KAI-277 section 7.2 six-check isolation chain is designed for trusted (Kaivue-built) models. Untrusted customer models require process-level isolation that Triton's architecture does not provide.

Alternatives considered:

| Approach | Why rejected |
|---|---|
| **Triton ensemble model with custom backend.** Triton supports custom C++ backends that can fork a subprocess. We could write a backend that forks a gVisor-sandboxed subprocess per inference. Rejected: Triton's backend API is synchronous per-instance; forking per request adds 100ms+. Triton's model lifecycle (load/unload) does not map cleanly to pod creation/destruction. We would be fighting Triton's architecture rather than working with it. |
| **Separate Triton deployment for custom models.** A second Triton fleet on the customer-models subpool, running only customer-uploaded models. Better than sharing the main fleet, but still shares process space between tenants' models within a single Triton pod. A multi-tenant Triton pod is exactly the threat model we are avoiding. |
| **Triton with per-tenant pod (one Triton instance per tenant).** Isolation is excellent. Cost is catastrophic (hundreds of Triton pods, each with GPU allocation, for tenants that may invoke their model once a day). Rejected on cost. |

The sandbox-per-(tenant, model) approach gives us the isolation of option 3 with the cost profile of on-demand scaling (pods only exist when actively used).

### 7.3 How Triton calls out to custom model sandboxes

For system models that need to chain with a customer model (e.g., a Kaivue behavior model that uses a customer's custom classifier as a post-processing step), the call path is:

1. Go service calls Triton SDK for system model inference (KAI-277 path).
2. Triton returns system model output to Go service.
3. Go service calls custom model inference API (section 6.1) with the system model's output as input.
4. Custom model sandbox returns result to Go service.

There is no direct Triton-to-sandbox communication. The Go service is the orchestrator. This is intentional: it keeps Triton unaware of sandboxes and keeps sandboxes unaware of Triton. The Go service applies tenant_id checks at each step.

### 7.4 Shared infrastructure

Custom model sandboxes share the following infrastructure with the Triton fleet:

- **R2 model storage (KAI-279, Seam #8).** Same content-addressed store, same registry API. Different access patterns (Triton's model-cache-agent sidecar vs sandbox init container), same source of truth.
- **GPU node pool.** Customer-models subpool is provisioned identically to the Triton fleet's node pool (g5.2xlarge, same AMI, same NVIDIA driver). Different label (`kaivue.io/workload=customer-models`), same Karpenter provisioner (with nodeSelector constraints).
- **Observability stack.** Same Prometheus/Grafana, same structured logging pipeline, same KAI-233 audit log sink.
- **Killswitch service.** Same killswitch infrastructure as KAI-282/KAI-277. Custom models register as killswitch targets with `model_type=custom`.

---

## 8. Monitoring and killswitch

### 8.1 Per-model killswitch

Any custom model can be killed (deactivated) within 60 seconds. Two actors can pull the switch:

- **Tenant admin.** Via the customer admin UI (KAI-327) or API. Kills their own model. Use case: tenant discovers their model is misbehaving or producing incorrect results.
- **Platform operator.** Via the internal admin API. Kills any tenant's model. Use case: security incident, regulatory demand, abuse detection.

Kill mechanics:

1. Killswitch state is written to a distributed cache (ElastiCache Redis, KAI-217) with key `killswitch:custom_model:<tenant_id>:<model_id>`.
2. The inference-proxy sidecar polls the killswitch cache every 5 seconds (section 6.2, step 5). On detection, it immediately stops accepting new requests (returns `503`) and terminates the sandbox pod.
3. The inference router's in-process cache (5s TTL) ensures no new requests are routed to the pod.
4. Worst case: 5s cache TTL + 5s poll interval = 10s. Well within the 60s SLO.
5. Model status in Postgres is updated to `quarantined` with a reason code.
6. KAI-233 audit event: `custom_model.killswitch_engaged` with actor, reason, timestamp.

### 8.2 Platform-wide killswitch for all custom models

A single platform-level killswitch disables ALL custom model inference across ALL tenants. Key: `killswitch:custom_models:platform`. When engaged:

- All inference requests to custom models return `503`.
- All running sandbox pods are terminated within 60 seconds.
- System models (Triton fleet) are unaffected.
- KAI-233 audit event: `custom_models.platform_killswitch_engaged`.

Use case: zero-day in ONNX Runtime, gVisor escape CVE, regulatory order.

### 8.3 Metrics

Per custom model, exported to Prometheus with labels `tenant_id`, `model_id`:

- `kaivue_custom_model_inference_duration_seconds` (histogram) — end-to-end inference latency.
- `kaivue_custom_model_inference_total` (counter) — total inference count, labeled by status (success, error, timeout, killswitch, rate_limited).
- `kaivue_custom_model_inference_errors_total` (counter) — errors by type (onnx_runtime_error, timeout, oom, sandbox_crash).
- `kaivue_custom_model_sandbox_cold_starts_total` (counter) — cold start count.
- `kaivue_custom_model_sandbox_cold_start_duration_seconds` (histogram) — cold start latency.
- `kaivue_custom_model_sandbox_active` (gauge) — currently active sandbox pods.
- `kaivue_custom_model_gpu_seconds_total` (counter) — GPU time consumed, for billing.
- `kaivue_custom_model_quota_remaining` (gauge) — remaining inference budget.

### 8.4 Anomaly hooks (automated response)

| Signal | Response | Escalation |
|---|---|---|
| Sandbox crash (OOM-kill, segfault, ONNX Runtime abort) | Auto-restart (1 retry). If crashes > 3 in 10 minutes, auto-quarantine model. | Alert to on-call SRE. |
| seccomp violation | Immediate pod termination. Auto-quarantine model. | Page lead-security (PagerDuty P1). Incident opened automatically. |
| gVisor Sentry panic | Pod termination. Model quarantined. | Page lead-security (P1). All sandbox pods on the same node are drained as precaution. |
| Inference latency P99 > 10s sustained 5 min | Alert to on-call SRE. No auto-action (could be legitimate large model). | SRE investigates, may killswitch. |
| Model inference error rate > 50% over 100 requests | Alert to tenant admin (KAI-303 notification). | If sustained 30 min, auto-quarantine with notification. |
| Rate limit exhaustion pattern (many 429s) | Log only. | If pattern matches known extraction signatures, alert lead-security. |

---

## 9. Cost model and quotas

### 9.1 Per-tenant monthly inference budget

Every tenant has a monthly inference budget denominated in **GPU-seconds**. The budget is set by the tenant's subscription tier:

| Tier | Monthly GPU-seconds | Approximate equivalent |
|---|---|---|
| Starter | 3,600 (1 GPU-hour) | ~120,000 inferences at 30ms each |
| Professional | 36,000 (10 GPU-hours) | ~1.2M inferences at 30ms each |
| Enterprise | Custom (negotiated) | Unlimited with committed spend |

### 9.2 Metering

GPU-seconds are metered per-inference by the inference-proxy sidecar. The sidecar measures actual GPU execution time (via CUDA events, not wall-clock) and reports it to the billing pipeline. Metering events flow through the same pipeline as KAI-364 (per-camera metering).

### 9.3 Quota exhaustion

When a tenant's monthly budget is exhausted:

1. Inference requests return `429 Too Many Requests` with:
   - `Retry-After: <seconds until month rollover>`
   - `X-Quota-Reset: <ISO 8601 timestamp of month rollover>`
   - Response body with upgrade link.
2. KAI-303 notification to tenant admin: "Your custom model inference budget is exhausted."
3. Running sandbox pods are NOT immediately terminated (the tenant may have paid for a higher tier by the time the next request arrives). Pods idle-evict normally (10 minutes).
4. At 80% budget consumption: proactive notification to tenant admin.

### 9.4 Billing

GPU-second usage appears as a line item on the tenant's monthly invoice (KAI-362 billing schema). The unit rate is set by lead-revenue; this doc does not commit to a price. The billing pipeline receives metering events and aggregates per-tenant per-month.

Sandbox infrastructure cost (nodes, networking, storage) is not billed per-tenant; it is absorbed into the platform's COGS and amortized across the customer-models subpool. The GPU-second rate must cover this overhead with margin.

### 9.5 Model storage costs

R2 storage for model artifacts is billed per-tenant based on stored bytes. Rate: standard R2 pricing passed through with a markup. KAI-279 handles dedup at the content level; if two tenants upload identical models, storage is charged to the first uploader only (the second gets a free ride). This is acceptable at v1 scale; revisit if gaming becomes an issue.

---

## 10. Open questions for lead-security / lead-cloud / counsel

### Q1: Art. 25 attestation wording (counsel)

The CMPA language transferring Art. 16 provider obligations needs legal review. Specific questions:

- Does the attestation need to reference the specific Annex III category, or is a general "I have completed my Art. 16 conformity assessment" sufficient?
- Do we need the customer to upload their conformity assessment document, or is the attestation checkbox legally sufficient?
- Jurisdiction: does the CMPA need separate versions for EU, UK, and non-EU customers?

**Owner: counsel. Deadline: before KAI-291 implementation starts.**

### Q2: MIG partitioning availability (lead-cloud)

A10G (g5.2xlarge) does not support MIG. Our GPU memory isolation relies on CUDA MPS, which provides software-level (not hardware-level) memory isolation. MIG would give us hardware-isolated GPU slices. Options:

- Upgrade customer-models subpool to A30 or A100 instances (support MIG). Cost increase: ~3-5x.
- Accept MPS-level isolation for v1. Upgrade path documented.
- Investigate NVIDIA Confidential Computing (GPU TEEs) timeline.

**Owner: lead-cloud. Deadline: architecture review.**

### Q3: Seccomp profile specifics (lead-security)

The seccomp profile in section 3.6 is described at the policy level. Implementation needs:

- Exact syscall allowlist (to be generated by profiling ONNX Runtime C++ server under gVisor).
- Decision on `SCMP_ACT_LOG` vs `SCMP_ACT_ERRNO` for denied syscalls (logging reveals attacker intent but may leak information).
- Whether to use the Kubernetes `SeccompProfile` field or a custom `seccomp` OCI runtime hook.

**Owner: lead-security. Deadline: before sandbox pod spec is finalized.**

### Q4: Face-signature scanning false positive rate (lead-ai)

The face-recognition heuristic in section 5.4 will flag models with embedding-like outputs. Expected false positive rate:

- Object detection models with feature extraction heads (e.g., RetinaNet, FCOS) output feature maps that could trigger dimension-based heuristics.
- Embedding models for non-face tasks (product similarity, image retrieval) have identical output shapes to face embeddings.

Need lead-ai to estimate the false positive rate and define the confidence threshold for R5 quarantine vs. pass-through.

**Owner: lead-ai. Deadline: before static analysis implementation.**

### Q5: gVisor `--nvproxy` maturity (lead-security + lead-cloud)

gVisor's NVIDIA GPU proxy (`--nvproxy`) is relatively new. Questions:

- What is the current CVE history for `--nvproxy`?
- Does Google use `--nvproxy` in production for GKE Sandbox? (Believed yes, but need confirmation.)
- What is our patch SLA for gVisor security updates?

**Owner: lead-security + lead-cloud. Deadline: before sandbox pod spec is finalized.**

---

## 11. What this doc does NOT commit to

The following are explicitly deferred. They are not forgotten; they are out of scope for KAI-291 v1.

- **Edge inference for custom models (KAI-278).** Running customer ONNX models on Recorder boxes is a v2 feature. Requires ONNX Runtime integration on the edge, model push to Recorders, edge-specific resource limits.
- **Custom pre/post-processing code.** Customers cannot upload Python/WASM pre/post-processing scripts in v1. All pre/post-processing uses the platform's built-in library (resize, normalize, NMS, threshold filtering).
- **TensorRT optimization of custom models.** Automatic conversion of customer ONNX models to TensorRT plans for better inference performance. Deferred because TensorRT conversion can change model behavior (quantization, kernel fusion) and the customer must approve the converted model.
- **Model versioning UI.** v1 supports uploading new versions (new content hash), but the UI does not provide version comparison, rollback, or A/B testing. These are KAI-291-v2.
- **Runtime output monitoring for T6.** The inference-proxy sidecar sampling outputs to detect misclassified face models at runtime (section 4, T6 mitigation #3). Deferred due to performance impact and false-positive tuning requirements.
- **Multi-model chaining inside the sandbox.** A single sandbox pod runs a single model. Pipeline orchestration (model A output feeds model B input) is done by the Go service outside the sandbox (section 7.3). In-sandbox chaining is deferred.
- **Customer-provided model configs.** In v1, the platform generates the ONNX Runtime session config (thread count, memory arena, execution providers) based on the model's registered metadata. Customers cannot override session config. Deferred to avoid a config-as-attack-surface problem.
- **Spot instances for customer-models subpool.** KAI-277 section 5.2 defers spot for custom models until utilization patterns are understood. This doc inherits that decision.
- **Federated model governance.** A customer with multiple business units cannot delegate model upload permissions to sub-tenants. All uploads go through the tenant admin. Multi-level governance is a future feature.
- **Model marketplace.** Tenants cannot share or sell models to other tenants. All models are private to the uploading tenant.

---

## Appendix A: Referenced tickets

| Ticket | What it covers | Relationship to KAI-291 |
|---|---|---|
| KAI-277 | Triton Inference Server on EKS | Custom models do NOT run in Triton; they run in separate sandboxes on the same GPU subpool. |
| KAI-279 | Model registry (R2, Postgres, signing) | Source of truth for model artifacts. Upload pipeline writes here. |
| KAI-280 | Edge vs cloud inference routing | Decided cloud-only for custom models in v1. |
| KAI-282 | Face recognition + EU AI Act | Custom face models inherit B1-B7 boundary conditions. |
| KAI-294 | EU AI Act conformity assessment | Platform-level conformity; KAI-291 handles the Art. 25 Modifier angle. |
| KAI-233 | Audit log service | Every upload, inference, refusal, killswitch event is logged. |
| KAI-303 | Push notifications | Upload status, quota warnings, killswitch notifications. |
| KAI-327 | Customer admin AI settings | UI for model management, killswitch, face vault. |
| KAI-251 | CSE-CMK cryptostore | Face embeddings from custom models encrypted with tenant key. |
| KAI-217 | ElastiCache Redis | Killswitch state cache. |
| KAI-362 | Billing schema | GPU-second billing line items. |
| KAI-364 | Per-camera metering | Metering pipeline shared by custom model billing. |
| KAI-400 | API key management | Authentication for inference API. |

## Appendix B: Seam compliance checklist

| Seam | How KAI-291 honors it |
|---|---|
| #4 (per-tenant vector index) | Custom embedding models write to tenant-isolated vector index. No shared embedding space. |
| #8 (R2 multi-region blob storage) | Model artifacts stored in R2 via KAI-279. Single-region in v1; multi-region path not foreclosed. |
| #10 (tenant_id everywhere) | tenant_id is in every DB row, every API path, every audit event, every metric label, every sandbox pod label, every killswitch key. |
