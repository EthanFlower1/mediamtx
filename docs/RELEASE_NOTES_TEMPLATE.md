# MediaMTX NVR — Release Notes vX.Y.Z

**Release date:** YYYY-MM-DD

## Highlights

<!-- 1-3 sentence summary of the most important changes in this release. -->

## New Features

<!-- List new user-facing capabilities. One bullet per feature, with the ticket ID. -->

- **Feature name** — Brief description. (KAI-XX)

## Improvements

<!-- Enhancements to existing features: performance, UX, reliability. -->

- **Area** — What changed and why it matters. (KAI-XX)

## Bug Fixes

<!-- Defects resolved in this release. -->

- **Component** — Description of the fix. (KAI-XX)

## Breaking Changes

<!-- Any change that requires action from users upgrading from the previous version. -->

- **What changed** — Migration steps or workaround.

## Deprecations

<!-- Features or APIs that are deprecated and will be removed in a future release. -->

- **Deprecated item** — Replacement or migration path. Target removal version.

## Upgrade Steps

### Prerequisites

- Minimum Go version: X.XX
- Minimum Flutter SDK version: X.XX.X
- Required database migration: vXX

### Upgrade Procedure

1. **Back up your data.**
   - Export configuration: `POST /api/v1/config/backup`
   - Back up the SQLite database file.

2. **Stop the running server.**
   ```bash
   systemctl stop mediamtx
   ```

3. **Replace the binary** (or pull the new Docker image).
   ```bash
   # Binary install
   cp mediamtx /usr/local/bin/mediamtx

   # Docker
   docker pull ghcr.io/ethanflower1/mediamtx-nvr:vX.Y.Z
   ```

4. **Run database migrations.** Migrations apply automatically on startup.

5. **Review configuration changes.** Compare `mediamtx.yml` against the new sample config for any added or renamed keys.

6. **Start the server.**
   ```bash
   systemctl start mediamtx
   ```

7. **Verify.**
   - Check logs: `journalctl -u mediamtx -f`
   - Confirm API health: `GET /api/v1/system/health`
   - Confirm cameras are streaming.

### Rollback

If you encounter issues:

1. Stop the server.
2. Restore the previous binary or Docker image.
3. Restore the database backup.
4. Start the server.

## Known Issues

<!-- Issues discovered after release that are not yet resolved. -->

- Description of issue and workaround if available.

## Contributors

<!-- People who contributed to this release. -->

---

*Full changelog: [vX.Y-1.Z...vX.Y.Z](https://github.com/EthanFlower1/mediamtx/compare/vX.Y-1.Z...vX.Y.Z)*
