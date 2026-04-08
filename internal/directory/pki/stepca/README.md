# stepca — per-site cluster Certificate Authority

Package `internal/directory/pki/stepca` is the embedded CA the Kaivue
Directory uses to issue short-lived mTLS leaves for Directory ↔ Recorder ↔
Gateway traffic on a single installation ("site").

Ticket: **KAI-241**.

## Architecture

Every Kaivue site runs its own **cluster CA**: a self-signed Ed25519 root
certificate bootstrapped the first time the Directory starts and persisted
inside `StateDir` (default `/var/lib/mediamtx-directory/pki/`). It is
distinct from the **federation CA** (handled separately by the cloud
control plane) which cross-signs site roots for multi-site federation.

```
                             ┌──────────────────┐
                             │ Federation CA    │  (cloud, out of scope)
                             │ (KAI-federation) │
                             └────────┬─────────┘
                                      │ cross-sign
                                      ▼
        Site A                  ┌──────────────┐                  Site B
   ┌──────────────┐     ...     │  Cluster CA  │     ...     ┌──────────────┐
   │ Cluster CA   │             │  (this pkg)  │             │ Cluster CA   │
   └──────┬───────┘             └──────┬───────┘             └──────┬───────┘
          │ IssueLeaf                  │                            │
    ┌─────┴──────┐              ┌──────┴──────┐              ┌──────┴──────┐
    ▼            ▼              ▼             ▼              ▼             ▼
 Directory   Recorder      Directory      Recorder      Directory      Recorder
  Gateway                    Gateway                      Gateway
```

Leaves live for **24 hours**. Rotation is KAI-242's job; this package only
exposes `IssueLeaf`, `IssueDirectoryServingCert`, and `ArchiveLeaf` so the
rotator can drive it.

## Files on disk

Under `StateDir`:

| File | Contents | Mode |
|---|---|---|
| `root.crt` | Root certificate (PEM, plaintext). | 0600 |
| `root.key.enc` | Root private key, sealed by cryptostore (PEM, custom type `KAIVUE ENCRYPTED PRIVATE KEY`). | 0600 |
| `directory.crt` | Directory serving leaf (PEM, plaintext). | 0600 |
| `directory.key.enc` | Directory serving leaf private key, sealed. | 0600 |
| `leaves/<serial>.crt` | Issued leaves (audit copy, plaintext). | 0600 |
| `leaves.archive.enc` | Rotated leaves, sealed and length-prefixed. | 0600 |

All writes go through an atomic tempfile + `rename` helper so a power loss
cannot leave a half-written PEM on disk.

## Master-key-derived encryption

The root private key is **never** stored in plaintext. On every boot the
Directory hands this package the installation master key (the existing
`nvrJWTSecret` from `mediamtx.yml`). Internally:

```
subkey = HKDF-SHA256(master=nvrJWTSecret, salt=nil, info="federation-root")
sealed = AES-256-GCM_Seal(subkey, root_pkcs8_der)
```

Both primitives come from **`internal/shared/cryptostore`** (KAI-251). This
package never touches raw AEAD or KDF code; cryptostore is the single seam
the project swaps when we go FIPS-validated.

If the master key changes (e.g. after a key-rotation event), the reload
fails with a decrypt error — which is the desired behavior. The operator
must explicitly re-bootstrap, and the audit trail notes a new root
fingerprint.

## Fingerprint and pairing tokens

`Fingerprint()` returns the lowercase hex SHA-256 of the root DER. KAI-243
mints pairing tokens that embed this fingerprint so that when a Recorder,
Gateway, or client enrolls for the first time it can pin the Directory's
root cert "trust-on-first-use" style instead of relying on a public CA
chain.

```
pairing_token {
  site_id       = uuid
  directory_url = https://directory.kaivue.local:8443
  ca_fingerprint = <stepca.Fingerprint()>
  jwk_challenge = <mint-time nonce>
}
```

## FIPS boundary (KAI-388)

All cryptographic primitives that need FIPS validation live behind
`cryptostore`:

- AES-256-GCM sealing of the root private key
- HKDF-SHA256 subkey derivation

This package uses only the standard library `crypto/ed25519` and
`crypto/x509` for signature generation — both of which are already part of
Go's FIPS-capable surface (`GOFIPS=1`). When KAI-388 swaps cryptostore for
the FIPS-validated provider, this package is unchanged.

## Smallstep vs. stripped-down implementation

KAI-241 permits either embedding `github.com/smallstep/certificates`
directly or falling back to a stripped-down CA using `crypto/x509`.

**This package ships the stripped-down variant.** Reasons:

1. `github.com/smallstep/certificates/authority` pulls in a transitive
   closure of hundreds of megabytes: `badger`, Google / AWS / Azure KMS
   clients, PKCS#11, the full `step-cli` dependency tree, a SQL migration
   framework, etc. That is incompatible with the MediaMTX-NVR single-binary
   size budget and the edge-appliance deployment model.
2. The surface we need — one root, short-lived server/client leaves,
   fingerprint pinning, encrypted-at-rest keys — is a few hundred lines of
   `crypto/x509`. Pulling in smallstep for that cost/benefit is a net loss.
3. The JWK-provisioner-style enrollment KAI-241 mentions is driven by
   KAI-243 pairing tokens, which are their own scheme. There is no benefit
   to reusing smallstep's ACME/JWK plumbing.
4. This package exposes the same API shape as the smallstep wrapper would,
   so swapping it out later is a mechanical change.

If we later need smallstep for federation bridging, the migration path is:
implement a `smallstepAdapter` alongside the current x509 implementation
and switch on a config flag. No callers need to change.

## API

```go
ca, err := stepca.New(stepca.Config{
    StateDir:   "/var/lib/mediamtx-directory/pki",
    MasterKey:  cfg.NvrJWTSecret,     // DO NOT duplicate; read from existing config
    Logf:       logger.Info,
})
if err != nil { ... }
defer ca.Shutdown(ctx)

// Trust material for clients
fp       := ca.Fingerprint()          // 64-char hex
rootPEM  := ca.RootPEM()
pool     := ca.RootPool()

// The Directory's own HTTPS listener
tlsCert, _ := ca.IssueDirectoryServingCert(ctx)

// A recorder enrolling via pairing token
leaf, _ := ca.IssueLeaf(ctx, csrFromRecorder, 24*time.Hour)

// Rotation (KAI-242)
_ = ca.ArchiveLeaf(ctx, oldLeaf)
```

## Tests

```
go test ./internal/directory/pki/stepca/
```

Covers bootstrap-from-empty, reload-idempotency, fingerprint stability,
leaf signing, `x509.Verify` against the root pool, TTL clamping, CSR nil
guard, directory serving cert chaining, tamper detection on the sealed
root, wrong-master-key rejection, archive append semantics, defensive copy
of `RootPEM`, cryptostore-injection mode, and shutdown key zeroing. 14
tests total.
