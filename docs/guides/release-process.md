# MediaMTX NVR Release Process

This document describes how to prepare, publish, and communicate a release.

## Release Cadence

- **Patch releases** (vX.Y.Z): as needed for critical bug fixes.
- **Minor releases** (vX.Y.0): roughly every 2-4 weeks with accumulated features.
- **Major releases** (vX.0.0): when breaking changes are introduced.

## Pre-Release Checklist

### 1. Confirm the Branch Is Ready

```bash
git checkout main
git pull origin main
```

- All feature branches for this release have been merged.
- CI is green on main.
- No open PRs tagged with the release milestone that are still in progress.

### 2. Run the Full Test Suite

```bash
go test ./...
```

- All unit and integration tests must pass.
- Run the AI detection benchmark if detection-related changes are included:
  ```bash
  go test ./internal/nvr/ai/... -bench=.
  ```

### 3. Verify Database Migrations

- Confirm the latest migration version number in `internal/nvr/db/migrations.go`.
- Test a clean migration from an empty database.
- Test an upgrade migration from the previous release version.

### 4. Build and Test Docker Image

```bash
docker build -t mediamtx-nvr:release-candidate .
docker run --rm -p 8554:8554 -p 9997:9997 mediamtx-nvr:release-candidate
```

- Verify the server starts and responds to `GET /api/v1/system/health`.

### 5. Update Documentation

- Update `CHANGELOG.md` with all changes since the last release.
- Review and update the installation guide (`docs/`) if configuration keys changed.
- Review the quick-start guide for accuracy.

## Creating the Release

### 1. Write Release Notes

Copy `docs/RELEASE_NOTES_TEMPLATE.md` and fill in all sections:

```bash
cp docs/RELEASE_NOTES_TEMPLATE.md docs/releases/vX.Y.Z.md
```

Edit the file:
- Replace all `vX.Y.Z` placeholders with the actual version.
- Fill in features, improvements, bug fixes, and breaking changes.
- Write clear upgrade steps.
- List any known issues.

### 2. Tag the Release

```bash
git tag -a vX.Y.Z -m "Release vX.Y.Z"
git push origin vX.Y.Z
```

### 3. Create the GitHub Release

```bash
gh release create vX.Y.Z \
  --title "MediaMTX NVR vX.Y.Z" \
  --notes-file docs/releases/vX.Y.Z.md
```

Attach build artifacts if applicable:

```bash
gh release upload vX.Y.Z ./build/mediamtx-linux-amd64 ./build/mediamtx-linux-arm64 ./build/mediamtx-darwin-amd64
```

### 4. Publish Docker Image

```bash
docker tag mediamtx-nvr:release-candidate ghcr.io/ethanflower1/mediamtx-nvr:vX.Y.Z
docker tag mediamtx-nvr:release-candidate ghcr.io/ethanflower1/mediamtx-nvr:latest
docker push ghcr.io/ethanflower1/mediamtx-nvr:vX.Y.Z
docker push ghcr.io/ethanflower1/mediamtx-nvr:latest
```

## Post-Release

### 1. Verify the Release

- Download the release artifacts from GitHub and confirm they run.
- Pull the Docker image from the registry and confirm it starts.
- Spot-check that release notes render correctly on the GitHub Releases page.

### 2. Monitor

- Watch for issue reports in the first 24-48 hours.
- Monitor server logs and system alerts on any staging/production deployments upgraded to the new version.

### 3. Communicate

- Post a summary in the project's communication channel.
- If there are breaking changes, proactively notify known users.

## Hotfix Process

For critical issues discovered after a release:

1. Create a branch from the release tag:
   ```bash
   git checkout -b hotfix/vX.Y.Z+1 vX.Y.Z
   ```
2. Apply the minimal fix. Include a test that reproduces the issue.
3. Merge the hotfix branch into main.
4. Tag and release `vX.Y.Z+1` following the standard release steps above.

## Version Numbering

Follow [Semantic Versioning 2.0.0](https://semver.org/):

- **MAJOR**: incompatible API changes, database schema changes that require manual migration, removal of deprecated features.
- **MINOR**: new features, new API endpoints, automatic database migrations.
- **PATCH**: bug fixes, performance improvements, documentation updates.

## File Locations

| File | Purpose |
|---|---|
| `CHANGELOG.md` | Cumulative log of all changes by version |
| `docs/RELEASE_NOTES_TEMPLATE.md` | Template for per-release notes |
| `docs/releases/vX.Y.Z.md` | Filled-in release notes for each version |
