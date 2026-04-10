# Adding a Recorder to a Directory

This guide covers the full join sequence for a new Recorder node. Once complete,
the Recorder is enrolled in the site mesh, holds a valid mTLS certificate, and
is ready to receive camera assignments from the Directory.

## Prerequisites

- The Directory is running and reachable on its configured endpoint (e.g.
  `https://dir.acme.local:8443`).
- You have admin access to the Directory to generate a pairing token.
- The Recorder machine has network access to the Directory endpoint and to the
  Headscale coordinator embedded in the Directory.
- `mediamtx-pair` binary is installed on the Recorder machine (available from
  the release package or built from source with `go build ./cmd/mediamtx-pair`).

## Step 1: Generate a pairing token on the Directory

From the Directory admin UI (`/admin/recorders/new`) or via the API:

```
POST https://dir.acme.local:8443/api/v1/pairing/tokens
Authorization: Bearer <admin-token>
Content-Type: application/json

{
  "suggested_roles": ["recorder"]
}
```

Response:

```json
{
  "token_id": "550e8400-...",
  "encoded": "eyJrIjoi..."
}
```

Copy the `encoded` value. It is valid for **15 minutes** and is **single-use**.
Do not share it over insecure channels; treat it like a password.

## Step 2: Run `mediamtx-pair` on the Recorder

```bash
sudo mediamtx-pair eyJrIjoi...
```

The command runs the 9-step join sequence and prints progress:

```
pairing: [1/9] decoding and verifying pairing token
pairing: step 1 ok  token_id=550e8400-... directory=https://dir.acme.local:8443
pairing: [2/9] probing hardware and checking in with Directory
pairing: step 2 ok  recorder_uuid=7f3c1a2b-...
pairing: [3/9] registering Recorder with tailnet mesh
pairing: step 3 ok  mesh_hostname=recorder-7f3c1a2b-...
pairing: [4/9] generating device keypair
pairing: step 4 ok
pairing: [5/9] enrolling with cluster CA to obtain mTLS leaf certificate
pairing: step 5 ok  leaf_subject=recorder-7f3c1a2b-... not_after=2026-04-09T16:00:00Z
pairing: [6/9] pinning step-ca root certificate
pairing: step 6 ok
pairing: [7/9] verifying Directory control-plane connectivity
pairing: step 7 ok
pairing: [8/9] requesting initial assignment snapshot (deferred to KAI-253)
pairing: step 8 — initial assignment snapshot deferred to KAI-253
pairing: [9/9] persisting paired state to local cache
pairing: complete — Recorder is ready  recorder_uuid=7f3c1a2b-...
Recorder paired successfully. Start the Recorder with: mediamtx recorder
```

On success, start the Recorder service:

```bash
sudo systemctl start mediamtx-recorder
```

## What the join sequence does

| Step | What happens                                                                                                                                                                                                                                                                       |
| ---- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 1    | Decodes the token. Dials the Directory over TLS and **pins the certificate fingerprint** encoded in the token. If there is a fingerprint mismatch the command exits immediately — this is the primary protection against man-in-the-middle attacks.                                |
| 2    | Probes the machine's CPU, RAM, disks, NICs, and GPU. POSTs the hardware info to the Directory's check-in endpoint. The Directory marks the token as redeemed and assigns a stable Recorder UUID.                                                                                   |
| 3    | Registers the Recorder with the embedded Headscale tailnet coordinator using the pre-auth key from the token. The Recorder gets a mesh hostname (`recorder-<uuid>`) and joins the site's private network.                                                                          |
| 4    | Generates an Ed25519 device keypair. The private key is AES-256-GCM encrypted using a key derived from the Headscale pre-auth key and saved to `<state-dir>/device.key.enc`.                                                                                                       |
| 5    | Enrolls with the embedded step-ca using the JWK provisioner enrollment token from the pairing token. Receives a 24-hour mTLS leaf certificate. If `StepCAEnrollToken` is empty (air-gapped Directory), a self-signed stub is used and the operator must supply a cert out-of-band. |
| 6    | Pins the step-ca root fingerprint from the token (Trust On First Use). Subsequent certificate renewals are validated against this root.                                                                                                                                            |
| 7    | Makes a test mTLS request to the Directory's health endpoint using the new leaf certificate to confirm end-to-end connectivity.                                                                                                                                                    |
| 8    | Reserved for initial camera assignment snapshot (KAI-253). Currently a no-op.                                                                                                                                                                                                      |
| 9    | Writes a `pairing.paired` record to the local SQLite state cache at `<state-dir>/state.db`.                                                                                                                                                                                        |

## CLI flags

```
Usage: mediamtx-pair [flags] <pairing-token>

Flags:
  -state-dir string
        Recorder state directory (default /var/lib/mediamtx-recorder)
  -mesh-state-dir string
        Mesh node state directory (default <state-dir>/mesh)
  -v    Enable verbose (debug) logging
```

## Troubleshooting

### Step 1: "token is malformed or expired"

The token has either expired (TTL is 15 minutes) or was copied incorrectly.
Request a new token from the Directory admin and paste it verbatim.

### Step 1: "fingerprint mismatch — possible MITM attack"

The Directory's TLS certificate does not match the fingerprint embedded in the
token. Potential causes:

- A load balancer or reverse proxy is terminating TLS and presenting a different
  certificate.
- The Directory's certificate was rotated after the token was generated.
  (Tokens embed the fingerprint at generation time; a cert rotation before
  pairing completes will cause this.)
- A genuine man-in-the-middle attack. Verify the Directory URL and try again
  from a trusted network.

**Do not disable the fingerprint check.** It is the primary pairing security control.

### Step 2: "token already redeemed"

A previous pairing attempt reached step 2 and consumed the token. Tokens are
single-use. Request a new token and start fresh. If the previous attempt
failed at step 3+, the Recorder may have a partially-joined mesh node — check
`<state-dir>/mesh/` and remove it before re-pairing.

### Step 2: "token expired at Directory"

The token expired at the Directory's clock (which may differ slightly from the
Recorder's clock). Request a new token.

### Step 3: "tsnet: ..."

Network connectivity issue reaching the Headscale coordinator. Verify:

- The Recorder can reach the Directory endpoint on the configured port.
- No firewall is blocking UDP 41641 (WireGuard) or TCP 443 (Headscale control).

### Step 5: stub cert warning

If you see "StepCAEnrollToken is empty; issuing self-signed stub cert", the
Directory's `StepCASignURL` was not configured. The Recorder will operate with
a self-signed cert, which means mTLS between the Recorder and Directory will
not be validated by the site CA. Configure `StepCASignURL` on the Directory
and re-pair to get a CA-signed cert.

### Step 7: "directory health check failed"

The Recorder's new mTLS cert was not accepted by the Directory. Causes:

- The Directory's step-ca provisioner did not sign the enrollment token (step 5
  fell back to a self-signed stub).
- The Directory's mTLS policy requires a CA-signed client certificate.
  Check the Directory logs for TLS handshake errors.

## Security notes

- **Token TTL**: tokens expire in 15 minutes. If you cannot complete pairing
  within that window, request a new token.
- **Single-use**: once step 2 succeeds, the token is consumed. There is no way
  to reuse a partially-redeemed token. Design note: steps 1 and 2 are atomic;
  if the process crashes after step 2 but before step 9, the Recorder UUID is
  assigned but no camera assignments exist — this is safe and the Recorder will
  self-recover on next start by reading the Directory's assignment stream.
- **No secret echo**: `mediamtx-pair` never prints the raw token, the Headscale
  pre-auth key, or the device private key to stdout, stderr, or any log file.
- **Device key encryption**: the device private key is sealed with AES-256-GCM
  using a key derived from the Headscale pre-auth key via HKDF-SHA256. The
  sealed blob is tied to the site's tailnet identity.
- **Cert rotation**: the mTLS leaf certificate has a 24-hour lifetime and
  renews automatically via the `certmgr` hot-reload path (KAI-242). No
  Recorder restart is required on cert renewal.
