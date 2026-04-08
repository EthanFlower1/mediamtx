# cryptostore

Column-level encryption helper for sensitive data at rest in the MediaMTX NVR.
Built for RTSP credentials, face-vault enrolment vectors, pairing tokens,
federation root keys, Zitadel bootstrap state, and any other column-scoped
secret the NVR persists to SQLite.

- AES-256-GCM with a standard 96-bit random nonce per row
- Subkeys derived from a single master via **HKDF-SHA256 + per-purpose info**
- Versioned ciphertext envelope (`0x01 | nonce | ct | tag`)
- First-class key rotation (`RotateValue`, `RotateColumn`)
- Stdlib-only crypto primitives — sits inside the FIPS-track boundary (KAI-388)

## Quick start

```go
import "github.com/bluenviron/mediamtx/internal/shared/cryptostore"

// master comes from the existing `nvrJWTSecret` in mediamtx.yml.
// cryptostore never reads the config file itself.
store, err := cryptostore.NewFromMaster(master, nil, cryptostore.InfoRTSPCredentials)
if err != nil { return err }

ct, err := store.Encrypt([]byte("rtsp://user:pass@cam.local/stream"))
// persist ct as a BLOB column

pt, err := store.Decrypt(ct) // round-trips exactly
```

Use a distinct info string for each logical column:

| Column                           | Info constant              |
|----------------------------------|----------------------------|
| `cameras.rtsp_credentials_encrypted` | `InfoRTSPCredentials`   |
| `face_vault.embedding_encrypted` | `InfoFaceVault`            |
| `pairing_tokens.token_encrypted` | `InfoPairingTokens`        |
| `federation.root_key_encrypted`  | `InfoFederationRoot`       |
| `zitadel_bootstrap.state_encrypted` | `InfoZitadelBootstrap` |

A subkey derived for one info string **cannot** decrypt data encrypted with
another info string (verified by `TestCrossInfoDoesNotDecrypt`).

## Ciphertext format

```
byte 0        : format version (0x01)
bytes 1..12   : 96-bit GCM nonce (random per row)
bytes 13..N-16: ciphertext
bytes N-16..N : GCM authentication tag
```

- Version `0x00` is reserved and is always rejected, so a zero-filled blob
  cannot ever be mistaken for a valid ciphertext.
- The version byte is bound to the AEAD as additional authenticated data, so
  an attacker who flips the version fails the tag check.
- Bumping to `0x02` in the future is non-breaking: readers can switch on the
  version byte and dispatch to the appropriate unseal routine.

## Key rotation

Rotation happens in two layers:

1. **Per-value** — `RotateValue(oldStore, newStore, ct)` decrypts with the old
   store and re-encrypts with the new store. Used by anything that reads a
   single ciphertext and writes it back.
2. **Per-column** — `RotateColumn(ctx, db, table, column, oldStore, newStore, opts)`
   walks an entire SQLite column using a cursor-based pager
   (`id > ? ORDER BY id LIMIT N`) and is safe to re-run: already-rotated rows
   whose ciphertext no longer decrypts under the old store are skipped.

Rotation is intended to run as a background job triggered by the operator:

```go
n, err := cryptostore.RotateColumn(ctx, db, "cameras", "rtsp_credentials_encrypted",
    oldStore, newStore, cryptostore.RotateColumnOptions{BatchSize: 500})
log.Info("rotated %d rows", n)
```

**Warning:** `RotateColumn` interpolates `table` and `column` directly into
SQL. Never pass user-controlled identifiers.

## Threat model

### In scope

- **SQLite database theft.** An attacker who walks off with `nvr.db` learns
  ciphertexts but not plaintexts — the master key lives in `mediamtx.yml`
  (or cloud KMS when running in cloud mode), never in the database.
- **Backup exfiltration.** Same as database theft; backups are just encrypted
  BLOBs without the master key.
- **Replay across columns.** Because each column uses a distinct HKDF info
  string, a ciphertext stolen from `face_vault` cannot be replayed into
  `cameras.rtsp_credentials_encrypted` even by an insider who can write rows.
- **Bit-flip / row swap.** GCM's authentication tag detects any single-byte
  modification anywhere in the envelope (verified by `TestTamperDetection`).
- **Reserved-version confusion.** A zero-filled BLOB (e.g. partial write or
  truncation) fails closed with `ErrUnsupportedVersion` rather than decoding
  as empty plaintext.

### Out of scope

- **Host compromise.** If an attacker has code execution on the NVR and can
  read process memory or `mediamtx.yml`, they can decrypt everything.
  cryptostore is not a defence against root-on-host.
- **Side channels on AES.** We rely on the Go stdlib's constant-time AES
  implementation (AES-NI on amd64, ARM crypto extensions on arm64). Older
  hardware without dedicated AES instructions exposes a small timing surface
  we do not mitigate.
- **Master-key rotation policy.** cryptostore provides the mechanism
  (`RotateColumn`). Scheduling, audit logging, and KMS integration live in
  higher layers (see `docs/operations/key-rotation.md`, TBD KAI-252).
- **Forward secrecy.** A master-key compromise retroactively decrypts all
  historical ciphertexts. Per-row forward secrecy would require per-row
  rekeying, which we explicitly do not do.

## FIPS boundary (KAI-388)

This package only imports:

- `crypto/aes`
- `crypto/cipher`
- `crypto/hkdf` (Go 1.24+ stdlib)
- `crypto/rand`
- `crypto/sha256`

No `golang.org/x/crypto/...` and no third-party cryptography. When the Go
toolchain is built in FIPS-140 mode (`GOEXPERIMENT=boringcrypto` or the
native FIPS module shipped with Go 1.24+), all primitives route through the
validated module with zero code changes.

## Test coverage

Run `go test ./internal/shared/cryptostore/...`:

- Round-trip encrypt/decrypt over nil, empty, short, and 4 KiB plaintexts
- Per-row nonce uniqueness (10 000 consecutive encrypts, zero collisions)
- Tamper detection on version, nonce, body, and tag bytes
- Reserved version `0x00` rejected with `ErrUnsupportedVersion`
- Short ciphertext rejected with `ErrInvalidCiphertext`
- HKDF determinism (same master+info → same subkey)
- HKDF info separation (five well-known infos → five distinct subkeys)
- Cross-info ciphertext does not decrypt
- `RotateKey` in-place instance rotation (happy path + wrong-old-key)
- `RotateValue` two-store rotation
- `RotateColumn` end-to-end over an in-memory SQLite, including NULL rows,
  batch paging, and idempotent re-run
- Header layout invariants (`HeaderSize == 13`)

All tests derive their master key from
`sha256([]byte("test-master-key-REPLACE_ME"))` — no literal secrets in the
test source.
