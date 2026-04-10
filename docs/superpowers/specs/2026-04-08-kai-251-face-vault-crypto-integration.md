# KAI-251 Cryptostore Integration for Face Vault Embeddings

Short design doc. Author: lead-ai. Reviewer: lead-security (24h turnaround requested). Status: draft.

This document specifies how the face recognition embedding store (KAI-292) integrates with the existing KAI-251 cryptostore (`internal/shared/cryptostore/`) to satisfy KAI-282 boundary condition B5 (embeddings encrypted at rest with per-tenant CMK) and lead-security MUST-CHANGE #3.

---

## 1. Per-tenant CMK derivation from root CMK

Each on-prem Recorder already holds a master key (the `nvrJWTSecret` in `mediamtx.yml` — this is the root CMK for on-prem). For cloud tenants, the root CMK is a per-tenant secret stored in AWS Secrets Manager, provisioned at tenant creation time by KAI-216.

Per-tenant embedding subkey derivation uses the existing `cryptostore.NewFromMaster`:

```
tenant_root_cmk  →  HKDF-SHA256(master=cmk, salt=nil, info="face-vault")  →  32-byte AES-256 subkey
```

The info string `"face-vault"` is already defined as `cryptostore.InfoFaceVault` in `cryptostore.go:35`. No new constants needed.

**One Cryptostore instance per tenant**, constructed at:
- **Cloud:** when the face-recognition service initializes for a tenant (lazy, on first face API call). The per-tenant CMK is fetched from Secrets Manager once, the subkey is derived, and the Cryptostore is held in an in-memory tenant registry. The raw CMK is zeroed after derivation.
- **On-prem:** at Recorder startup, using `nvrJWTSecret` as the master. One Cryptostore covers all local face operations.

## 2. HKDF info strings for embedding-encryption subkey derivation

| Purpose | Info string | Constant | Notes |
|---|---|---|---|
| Face embedding blob encryption | `"face-vault"` | `cryptostore.InfoFaceVault` | Already exists |
| CLIP embedding blob encryption | `"clip-vault"` | New: `cryptostore.InfoCLIPVault` | Add to cryptostore.go |
| Consent record encryption | `"consent-records"` | New: `cryptostore.InfoConsentRecords` | Add to cryptostore.go |

Each info string produces a distinct 32-byte subkey from the same master via HKDF. Changing an info string makes all previously encrypted data under that purpose undecryptable — the strings are stable and must never be renamed.

CLIP embeddings get a separate subkey because they are non-biometric (scene-level) and may have different retention/rotation policies than face embeddings. Consent records get their own subkey because they contain PII (subject_identifier) that is GDPR Art. 9 adjacent.

## 3. Re-wrap flow for CMK rotation

When a tenant's root CMK rotates (admin-initiated or policy-driven):

1. **Derive old subkey:** `cryptostore.DeriveSubkey(oldCMK, nil, "face-vault")` → `oldSubkey`
2. **Derive new subkey:** `cryptostore.DeriveSubkey(newCMK, nil, "face-vault")` → `newSubkey`
3. **Construct old and new Cryptostore instances:** `cryptostore.New(oldSubkey)`, `cryptostore.New(newSubkey)`
4. **Batch re-wrap:** For each `face_embeddings` row in the tenant's partition:
   - `newCiphertext, err := cryptostore.RotateValue(oldStore, newStore, row.embedding)`
   - UPDATE the row's `embedding` column with `newCiphertext`
5. **Repeat** for `clip_embeddings` (with `InfoCLIPVault`) and `consent_records` (with `InfoConsentRecords`).
6. **Zero old subkeys** in memory after rotation completes.

**Critical invariant:** the HNSW in-memory index does NOT need to be rebuilt on CMK rotation. The index operates on plaintext embeddings that are decrypted into memory at process start (§4). Rotation only touches the on-disk ciphertext blobs. The index stays valid.

**Batch size:** re-wrap in batches of 1000 rows per transaction to avoid long-running locks. Progress is tracked via a `rotation_cursor` column or a separate rotation-state table (detail deferred to implementation).

**Audit:** every rotation batch emits a KAI-233 audit event (`embedding.cmk_rotated`) with tenant_id, batch range, old key fingerprint (SHA-256 of old subkey, NOT the subkey itself), new key fingerprint.

## 4. Index rebuild flow on process start

On process start (cloud service boot or Recorder start):

1. Fetch tenant's CMK from Secrets Manager (cloud) or `nvrJWTSecret` (on-prem).
2. Derive face-vault subkey via `cryptostore.NewFromMaster(cmk, nil, "face-vault")`.
3. Load all `face_embeddings` rows for this tenant (or for all tenants served by this process).
4. For each row: `plaintext, err := store.Decrypt(row.embedding)` → deserialize to `[]float32`.
5. Insert each plaintext vector into the in-memory HNSW index (using `github.com/viterin/vek` or similar).
6. Zero the CMK in memory. Retain only the derived subkey (needed for encrypting new enrollments and for CMK rotation).
7. The in-memory index is now the query target. All similarity searches operate on plaintext vectors in memory. The database only holds ciphertext.

**Startup time budget:** for a tenant with 10,000 face embeddings at 512-dim × 4 bytes = 2KB per vector, total data is ~20MB of ciphertext. Decryption is ~1µs per AES-256-GCM block; index build is O(N log N). Expected total: <5 seconds for 10K vectors, <30 seconds for 100K. Acceptable for cloud cold-start (async/batched tier per KAI-277). For on-prem (Recorder), face recognition is not available until index build completes; the Recorder displays "Face vault loading..." status via KAI-327.

**Incremental updates:** new enrollments are encrypted with the tenant's subkey and INSERTed to the database. The plaintext vector is simultaneously added to the in-memory index. No full rebuild needed for incremental enrollment.

## 5. Memory protection

**mlock:** the in-memory index region and the derived subkey buffer MUST be locked into physical memory via `syscall.Mlock` (or `unix.Mlock` on Go 1.21+). This prevents the OS from swapping plaintext embeddings or key material to disk. Lead-security requires this for signoff.

Implementation: allocate the index backing array and the subkey buffer via a dedicated `mlock`-aware allocator. On Linux, `RLIMIT_MEMLOCK` must be raised for the process (set in the systemd unit file or container securityContext). On macOS (dev only), `mlock` is best-effort.

**Zeroization on shutdown:** when the process shuts down (graceful or SIGTERM):
1. Zero the derived subkey buffer (`for i := range key { key[i] = 0 }`).
2. Zero the in-memory HNSW index backing array.
3. `munlock` the regions.

This is defense-in-depth: after process exit the OS reclaims pages, but explicit zeroization prevents a core dump or /proc/pid/mem read from leaking plaintext embeddings or keys during the shutdown window.

**Runtime.KeepAlive:** use `runtime.KeepAlive` on the subkey and index arrays to prevent the GC from collecting them before explicit zeroization. Go's GC does not zero freed memory.

## 6. Fault tolerance

**Cryptostore unavailable at recognition time → fail closed.**

If the face-recognition service cannot:
- fetch the tenant's CMK from Secrets Manager (network error, permission error, throttle), OR
- derive the subkey (should never fail if CMK is valid), OR
- decrypt embeddings (should never fail if subkey matches — implies data corruption or key mismatch), OR
- build the in-memory index (OOM, corrupted data)

Then face recognition for that tenant returns **no matches** (fail-closed). The recording path is unaffected (Seam #5: fail-open recording — face recognition is NOT the recording path, so it CAN fail closed without stopping video capture).

**Specific failure modes:**

| Failure | Behavior | Recovery |
|---|---|---|
| Secrets Manager unavailable | No index built; all match queries return empty | Auto-retry on next query with exponential backoff; alert after 3 consecutive failures |
| CMK fetch succeeds but decryption fails on N rows | Skip corrupted rows; build partial index; log `embedding.decrypt_failed` audit event per row | Manual investigation; likely indicates key mismatch from a botched rotation |
| OOM during index build | Process crashes; supervisor restarts | Reduce batch size; investigate tenant vault size; alert SRE |
| CMK rotated mid-query | Stale subkey decrypts existing ciphertext fine (rotation hasn't touched those rows yet); new enrollments use new subkey and are added to index in plaintext | No action needed; rotation is transparent to queries |

---

## Open questions

1. **Secrets Manager caching:** should we cache the CMK in memory for the process lifetime, or re-fetch periodically to pick up rotations? Recommendation: cache for process lifetime, re-fetch only on explicit rotation signal (SNS notification or admin API call). Periodic re-fetch is wasteful and adds a Secrets Manager cost line.

2. **RLIMIT_MEMLOCK value:** needs to be sized for the largest expected in-memory index. At 100K vectors × 2KB = 200MB plaintext + index overhead (~2x for HNSW) = ~400MB. Set RLIMIT_MEMLOCK to 512MB per process. Lead-sre to confirm this fits the container memory limits.

3. **Multi-process fan-out (cloud):** if the face service scales to N replicas, each replica rebuilds its own in-memory index from the encrypted DB. This is O(N × startup_time). Acceptable at current scale; if it becomes a bottleneck, consider a shared in-memory index service (Redis with encrypted vectors, or a dedicated index sidecar). Deferred to v2.

---

*End of doc. Lead-security: please review §5 (mlock + zeroization) and §6 (fail-closed semantics) specifically. 24h turnaround requested.*
