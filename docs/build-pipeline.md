# Build Pipeline (KAI-428)

This document describes the GitHub Actions build pipeline for the Kaivue
Recording Server: how it produces cross-platform binaries, how the SBOM is
generated, how cosign signing works in production, and how to reproduce a
build locally.

## Overview

The pipeline is split across three workflow files under `.github/workflows/`:

| Workflow | File | Trigger | Purpose |
|---|---|---|---|
| Build matrix | `build.yml` | push to `main`, tags `v*`, PRs, `workflow_call` | Cross-compile per target and upload artifacts |
| Supply chain | `supply-chain.yml` | GitHub Releases, manual dispatch | Generate SBOM (syft) + cosign signing (release only) |
| Reproducibility | `reproducibility-check.yml` | PRs touching Go code, manual, weekly cron | Build the same commit twice and diff the bytes |

`build.yml` is the source of truth for how a binary is produced. The other two
workflows reuse it via `workflow_call`.

> The legacy `release.yml` and `nightly_binaries.yml` workflows still drive
> the official MediaMTX release tarballs through `make binaries`. KAI-428 is
> additive — it does not replace those workflows, it provides the
> reproducible / signed pipeline that the Kaivue release process requires.

## Targets

The matrix builds:

- `linux/amd64`
- `linux/arm64`
- `darwin/arm64`
- `windows/amd64`

(`linux/armv6` and `linux/armv7` continue to be produced by `make binaries`
in `release.yml`. Add them here if/when they need SBOM + cosign coverage.)

## Reproducible build flags

Every build uses:

```sh
CGO_ENABLED=0 \
GOFLAGS=-mod=readonly \
SOURCE_DATE_EPOCH=$(git log -1 --pretty=%ct) \
go build \
  -trimpath \
  -buildvcs=true \
  -tags enable_upgrade \
  -ldflags "-s -w -buildid= -X main.Version=${VERSION}" \
  -o mediamtx \
  .
```

What each flag does:

- **`CGO_ENABLED=0`** — purely-Go build, no host toolchain leaks. Safe because
  the project uses `modernc.org/sqlite` (pure Go).
- **`GOFLAGS=-mod=readonly`** — module graph cannot be mutated mid-build.
- **`SOURCE_DATE_EPOCH`** — pinned to the commit timestamp so any embedded
  timestamps are deterministic.
- **`-trimpath`** — strips absolute filesystem paths (`/home/runner/...`)
  from the binary.
- **`-buildvcs=true`** — embeds the commit + dirty flag deterministically.
- **`-ldflags -s -w`** — strips symbol and DWARF tables (deterministic and
  small).
- **`-ldflags -buildid=`** — zeroes the linker build ID, which would otherwise
  vary across rebuilds.
- **`-ldflags -X main.Version=...`** — stamps the version. **Note:** `main.go`
  does not currently declare `var Version string`, so the linker silently
  ignores the `-X` for now. The flag is wired up so that when we add the var
  it begins working with no pipeline change.

## Reproducing a build locally

```sh
git clone https://github.com/<org>/mediamtx.git
cd mediamtx
git checkout <commit>

export CGO_ENABLED=0
export GOFLAGS=-mod=readonly
export SOURCE_DATE_EPOCH=$(git log -1 --pretty=%ct)
VERSION=$(git describe --tags --always --dirty 2>/dev/null || echo dev)

GOOS=linux GOARCH=amd64 go build \
  -trimpath \
  -buildvcs=true \
  -tags enable_upgrade \
  -ldflags "-s -w -buildid= -X main.Version=${VERSION}" \
  -o mediamtx_local \
  .

sha256sum mediamtx_local
```

The resulting sha256 must match the sha256 published by `build.yml` for the
same commit and target. If they differ, file a bug — reproducibility is a
release-blocking property.

## Verifying the SBOM locally

The `supply-chain.yml` workflow publishes one SPDX and one CycloneDX SBOM per
target as build artifacts (and, for releases, asset attachments).

To verify locally:

```sh
# 1. Install syft
brew install syft       # macOS
# or: curl -sSfL https://raw.githubusercontent.com/anchore/syft/main/install.sh | sh

# 2. Regenerate the SBOM against the binary you have on disk
syft mediamtx_local -o spdx-json > local.spdx.json

# 3. Compare against the published SBOM
diff <(jq -S 'del(.creationInfo.created)' local.spdx.json) \
     <(jq -S 'del(.creationInfo.created)' sbom-linux_amd64.spdx.json)
```

You should also run vulnerability scans:

```sh
brew install grype
grype sbom:local.spdx.json
```

## Cosign signing (production)

> **Signing only runs on `release` events.** PRs and manual dispatches never
> sign anything. This is enforced by `if: github.event_name == 'release'`
> on the `sign` job in `supply-chain.yml`.

### One-time setup (human action required)

1. Generate a cosign keypair on a trusted machine:

   ```sh
   cosign generate-key-pair
   ```

   This produces `cosign.key` (private) and `cosign.pub` (public).

2. Add the repository secrets in GitHub (Settings -> Secrets and variables ->
   Actions):

   - `COSIGN_KEY` -> contents of `cosign.key`
   - `COSIGN_PASSWORD` -> the password you used during keygen

3. Publish `cosign.pub`. Recommended locations:
   - Commit it to the repo root as `cosign.pub` (so verifiers can `curl` it).
   - Attach it to every GitHub release as a release asset.
   - Mirror it to the Kaivue website on a stable URL.

4. **Securely wipe** `cosign.key` from the trusted machine after the secret
   has been added.

Until step 2 is complete, the `sign` job will hard-fail with an explicit
error message pointing to this document.

### Verifying a signature

After a release ships, anyone can verify a binary with:

```sh
# Download the binary, the .sig, and cosign.pub from the release
cosign verify-blob \
  --key cosign.pub \
  --signature mediamtx_v1.2.3_linux_amd64.sig \
  mediamtx_v1.2.3_linux_amd64
```

A future revision of this pipeline should also publish a Rekor transparency
log entry (keyless / Fulcio) so verification does not require trusting a
single key out-of-band.

## Reproducibility check

`reproducibility-check.yml` builds the current commit twice on the same
runner, clearing the Go build cache between runs, and asserts that the two
binaries are byte-identical. The check runs on every PR that touches Go
code, on manual dispatch, and weekly via cron so dependency drift cannot
silently break reproducibility.

If the check fails, both binaries are uploaded as the
`reproducibility-mismatch` artifact for offline diffing.

## Open work / follow-ups

- Add a `var Version string` to `main.go` so `-X main.Version=...` actually
  populates a runtime field. (Currently MediaMTX reads the version from
  `internal/core/VERSION` via `versiongetter`.)
- Add container image build + cosign signing for the OCI artifact (currently
  this pipeline only covers binaries).
- Add `cosign attest` for the SBOM so the SBOM is cryptographically bound to
  the binary in the transparency log.
- Expand the reproducibility-check matrix beyond `linux/amd64` once stable.
- Wire `supply-chain.yml` SBOM + signature outputs into `release.yml` so
  release assets carry them automatically.
