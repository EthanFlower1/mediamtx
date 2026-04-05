# MediaMTX NVR Administrator Handbook

This handbook covers day-to-day administration of a MediaMTX NVR deployment: configuration, user management, storage, backups, and security hardening.

---

## Table of Contents

1. [Configuration Reference](#configuration-reference)
2. [User and Role Management](#user-and-role-management)
3. [Storage Planning](#storage-planning)
4. [Backup and Restore](#backup-and-restore)
5. [Security Hardening Checklist](#security-hardening-checklist)

---

## Configuration Reference

All server configuration lives in `mediamtx.yml`. The file is read at startup; some settings can also be changed at runtime via the Control API.

### Core NVR Settings

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `nvr` | bool | `false` | Enable NVR functionality (camera management, ONVIF, recording timeline). |
| `nvrDatabase` | string | `~/.mediamtx/nvr.db` | Path to the SQLite database that stores cameras, users, recordings metadata, and rules. |
| `nvrJWTSecret` | string | _(auto-generated)_ | 256-bit hex secret used to sign NVR JWTs **and** encrypt stored ONVIF credentials. Never share or reset this value without a backup -- doing so makes existing encrypted credentials unrecoverable. |

### Control API

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `api` | bool | `true` | Enable the HTTP Control API. |
| `apiAddress` | string | `:9997` | Listen address for the API. |
| `apiEncryption` | bool | `false` | Serve the API over HTTPS. |
| `apiServerKey` | string | `server.key` | Path to TLS private key (when `apiEncryption` is true). |
| `apiServerCert` | string | `server.crt` | Path to TLS certificate (when `apiEncryption` is true). |
| `apiAllowOrigins` | list | `['*']` | Allowed CORS origins. Restrict in production. |
| `apiTrustedProxies` | list | `[]` | IPs/CIDRs of reverse proxies that may set `X-Forwarded-For`. |

### Playback Server

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `playback` | bool | `true` | Enable the playback server for downloading recordings. |
| `playbackAddress` | string | `:9996` | Listen address. |
| `playbackEncryption` | bool | `false` | Serve over HTTPS. |
| `playbackAllowOrigins` | list | `['*']` | CORS origins. |

### Streaming Protocols

MediaMTX exposes multiple streaming protocols. Disable any you do not need.

| Protocol | Enable Key | Default Port(s) |
|----------|-----------|-----------------|
| RTSP | `rtsp: true` | TCP 8554, UDP 8000-8007 |
| RTMP | `rtmp: true` | TCP 1935 |
| HLS | `hls: true` | TCP 8888 |
| WebRTC | `webrtc: true` | TCP 8889, UDP 8189 |
| SRT | `srt: true` | UDP 8890 |

Each protocol has its own encryption, address, and TLS key/cert options. See comments in `mediamtx.yml` for full details.

### Metrics and Profiling

| Key | Default | Description |
|-----|---------|-------------|
| `metrics` | `false` | Enable Prometheus-compatible `/metrics` endpoint on port 9998. |
| `pprof` | `false` | Enable Go pprof endpoint on port 9999. **Never expose to the internet.** |

### Logging

| Key | Default | Description |
|-----|---------|-------------|
| `logLevel` | `info` | Verbosity: `error`, `warn`, `info`, `debug`. Use `debug` only for troubleshooting. |
| `logDestinations` | `[stdout]` | One or more of `stdout`, `file`, `syslog`. |
| `logFile` | `mediamtx.log` | Log file path (when `file` is a destination). |
| `logStructured` | `false` | Emit logs as JSONL for ingestion by log aggregators. |

### Path Defaults and Camera Paths

Camera sources are defined under the `paths:` key. Each path maps a name to a source URL and recording/protocol settings. The `pathDefaults:` section provides fallback values inherited by all paths.

Key path-level settings:

| Key | Description |
|-----|-------------|
| `source` | Stream source URL (`rtsp://...`, `publisher`, etc.). |
| `sourceOnDemand` | Pull source only when a viewer connects. |
| `record` | Enable recording for this path. |
| `recordPath` | Template for segment file paths. |
| `recordFormat` | Segment container format (`fmp4`). |
| `recordSegmentDuration` | Duration per segment file. |

### Authentication Methods

MediaMTX supports three authentication backends configured via `authMethod`:

| Value | Description |
|-------|-------------|
| `internal` | Users and permissions defined in `mediamtx.yml` under `authInternalUsers`. |
| `http` | Each auth request is forwarded to an external HTTP endpoint. |
| `jwt` | Tokens are validated against a JWKS endpoint. |

The NVR subsystem adds its own user database and JWT-based auth layer on top of these, managed through the NVR API (see below).

---

## User and Role Management

### Roles

The NVR user system defines three roles with increasing privilege:

| Role | Capabilities |
|------|-------------|
| `viewer` | View live streams, browse recordings for permitted cameras. Default role for new users. |
| `operator` | Everything a viewer can do, plus control PTZ, trigger manual recordings, and manage bookmarks. |
| `admin` | Full access: user management, camera configuration, system settings, backups, security. |

### Camera Permissions

Each user has an optional `camera_permissions` field (JSON string) that restricts which cameras the user can access. An empty value means access to all cameras.

### Managing Users via the API

All user endpoints require admin authentication.

**Create a user:**

```
POST /api/nvr/users
Content-Type: application/json
Authorization: Bearer <admin-jwt>

{
  "username": "operator1",
  "password": "securepassword",
  "role": "operator",
  "camera_permissions": ""
}
```

**List users:**

```
GET /api/nvr/users
Authorization: Bearer <admin-jwt>
```

**Update a user:**

```
PUT /api/nvr/users/:id
Authorization: Bearer <admin-jwt>

{
  "role": "admin",
  "password": "newpassword"
}
```

**Delete a user:**

```
DELETE /api/nvr/users/:id
Authorization: Bearer <admin-jwt>
```

### Best Practices

- Create individual accounts for each operator; avoid shared credentials.
- Use the `admin` role sparingly -- most day-to-day work requires only `operator`.
- Rotate passwords periodically and enforce minimum 8-character passwords.
- Review the audit log (`GET /api/nvr/audit`) to track administrative actions.

---

## Storage Planning

### Estimating Capacity

Recording storage depends on camera count, resolution, codec, and retention period. Rough estimates for H.264:

| Resolution | Bitrate (typical) | Per camera per day | Per camera per 30 days |
|------------|-------------------|-------------------|----------------------|
| 1080p | 4 Mbps | ~43 GB | ~1.3 TB |
| 4K | 10 Mbps | ~108 GB | ~3.2 TB |
| 720p | 2 Mbps | ~22 GB | ~650 GB |

H.265 (HEVC) typically requires 30-40% less storage at equivalent quality.

**Formula:** `Storage (GB) = Bitrate (Mbps) x 86400 / 8 / 1024 x Cameras x Days`

### Storage Quotas

The NVR supports per-scope storage quotas with warning thresholds:

| Field | Default | Description |
|-------|---------|-------------|
| `quota_bytes` | -- | Maximum allowed bytes. |
| `warning_percent` | 80 | Quota usage percentage that triggers a warning status. |
| `critical_percent` | 90 | Quota usage percentage that triggers a critical status. |

Quota status values: `ok`, `warning`, `critical`, `exceeded`.

Monitor quotas via:

```
GET /api/nvr/storage/quota
Authorization: Bearer <admin-jwt>
```

### Recording Rules

Recording behavior is controlled per camera through recording rules:

| Field | Description |
|-------|-------------|
| `mode` | `continuous` (always record) or `motion` (record on events only). |
| `days` | Comma-separated days of the week (e.g., `mon,tue,wed,thu,fri`). |
| `start_time` / `end_time` | Time window for the rule (24-hour format). |
| `post_event_seconds` | Seconds to continue recording after a motion event ends. |
| `enabled` | Toggle the rule without deleting it. |

### Filesystem Recommendations

- Use a dedicated partition or volume for recordings so that filling storage does not affect the OS.
- Prefer XFS or ext4 for large numbers of segment files.
- Monitor disk I/O; NVR recording is write-heavy. Use SSDs for the database and HDDs for bulk recording storage.
- Set up alerts for disk space thresholds (see quotas above).

### Retention

Old recordings are automatically purged based on retention policies. Configure retention to balance compliance requirements against available storage. The system consolidates detection data from closed motion events to reduce database size over time.

---

## Backup and Restore

### What Is Backed Up

A backup archive is an AES-256-GCM encrypted ZIP file containing:

- **SQLite database** (`nvr.db`) -- cameras, users, recording metadata, rules, audit logs.
- **Configuration file** (`mediamtx.yml`) -- all server and path settings.
- **TLS certificates/keys** -- any `.crt` and `.key` files in the config directory.

**Note:** Video recording files are NOT included in backups. Back up your recording storage separately using filesystem-level tools (rsync, ZFS snapshots, etc.).

### Creating a Backup

**Via the API:**

```
POST /api/nvr/system/backups
Authorization: Bearer <admin-jwt>
Content-Type: application/json

{
  "password": "strong-encryption-password"
}
```

The password must be at least 8 characters. It is used to derive an AES-256 encryption key via HKDF-SHA256. Store this password securely -- without it, the backup cannot be restored.

**Response:**

```json
{
  "filename": "backup-2026-04-03T120000Z.zip.enc",
  "message": "backup created successfully"
}
```

### Listing Backups

```
GET /api/nvr/system/backups
Authorization: Bearer <admin-jwt>
```

Returns an array of backup files with filename, size, creation time, and whether the backup was auto-generated.

### Downloading a Backup

```
GET /api/nvr/system/backups/:filename/download
Authorization: Bearer <admin-jwt>
```

Always download backups to off-site storage. Do not rely solely on backups stored on the same machine.

### Restoring from a Backup

```
POST /api/nvr/system/backups/restore
Authorization: Bearer <admin-jwt>
Content-Type: multipart/form-data

file: <backup-file>
password: <encryption-password>
```

**WARNING:** Restore replaces the current database and configuration. The server will need to be restarted after a restore.

### Scheduled Backups

The backup service supports automatic scheduled backups with configurable:

- **Interval** -- how often backups run.
- **Password** -- encryption password for automated backups.
- **Max keep** -- maximum number of backup files to retain (oldest are pruned).

### Backup Best Practices

- Run manual backups before any major configuration change.
- Store at least one copy off-site (cloud storage, remote server).
- Test restore procedures periodically on a staging instance.
- Document the backup encryption password in a secure password manager.
- Back up recording video files separately using `rsync`, ZFS snapshots, or your preferred tool.

---

## Security Hardening Checklist

Use this checklist to harden a production deployment. Items are ordered by impact.

### Authentication and Authorization

- [ ] **Change default credentials.** Create a named admin account and remove or restrict the default `any` user entries in `authInternalUsers`.
- [ ] **Restrict localhost-only admin access.** The default config grants API/metrics/pprof to `any` user from `127.0.0.1`/`::1`. Tighten this to named admin accounts.
- [ ] **Enforce strong passwords.** Minimum 8 characters; prefer 16+ with mixed character classes.
- [ ] **Use the principle of least privilege.** Assign `viewer` or `operator` roles; reserve `admin` for infrastructure staff.
- [ ] **Restrict camera permissions per user.** Use the `camera_permissions` field to limit which cameras non-admin users can access.
- [ ] **Review the audit log regularly.** `GET /api/nvr/audit` tracks login attempts, user changes, and administrative actions.

### Network and Transport

- [ ] **Enable TLS for the API.** Set `apiEncryption: true` and provide valid `apiServerKey`/`apiServerCert`. Use certificates from a trusted CA or an internal PKI.
- [ ] **Enable TLS for playback.** Set `playbackEncryption: true` with matching key/cert.
- [ ] **Enable RTSPS.** Set `rtspEncryption: "strict"` if all clients support it, or `"optional"` for a migration period.
- [ ] **Enable HTTPS for HLS.** Set `hlsEncryption: true`. Required for Low-Latency HLS on Apple devices.
- [ ] **Enable HTTPS for WebRTC signaling.** Set `webrtcEncryption: true`. WebRTC media is always encrypted regardless.
- [ ] **Restrict CORS origins.** Replace `apiAllowOrigins: ['*']` with specific origins, e.g., `['https://admin.example.com']`.
- [ ] **Configure trusted proxies.** If behind a reverse proxy, set `apiTrustedProxies` to the proxy IP/CIDR so `X-Forwarded-For` is honored correctly.
- [ ] **Use a firewall.** Expose only the ports you need. Block direct access to RTSP/RTMP ports from the internet; route through a reverse proxy or VPN.
- [ ] **Disable unused protocols.** Set `rtmp: false`, `srt: false`, etc., for any protocol you are not using.

### Rate Limiting and DoS Protection

- [ ] **Enable API rate limiting.** The NVR security middleware supports per-IP rate limiting (default: 20 req/s, burst 40). Ensure `rateLimitEnabled: true` in your security configuration.
- [ ] **Tune rate limits for your environment.** Adjust `rateLimitPerSecond` and `rateLimitBurst` based on expected client counts.

### Content Security

- [ ] **Set Content-Security-Policy.** The default middleware sets `X-Frame-Options: DENY` to prevent clickjacking. Customize `contentSecurityPolicy` if embedding the UI in an iframe.
- [ ] **Review security headers.** The NVR adds `X-Content-Type-Options: nosniff` and `X-Frame-Options` headers automatically.

### TLS Certificate Management

- [ ] **Monitor certificate expiry.** Use `GET /api/nvr/system/tls/status` to check days remaining and expiry level.
- [ ] **Automate certificate renewal.** Integrate with Let's Encrypt or your CA's renewal process. Upload renewed certificates via the TLS management API.
- [ ] **Use separate certificates per service** if your security policy requires it (API, playback, RTSP each accept their own key/cert pair).

### Data Protection

- [ ] **Protect `nvrJWTSecret`.** This secret encrypts stored ONVIF camera credentials. If compromised, an attacker can decrypt camera passwords from the database. Restrict file permissions on `mediamtx.yml` to `600` (owner read/write only).
- [ ] **Restrict database file permissions.** Set `chmod 600` on the SQLite database file (`nvr.db`).
- [ ] **Encrypt backups with strong passwords.** Backups contain the database (including hashed user passwords and encrypted camera credentials).
- [ ] **Store backup passwords in a secrets manager.** Never commit backup passwords to version control.

### System-Level Hardening

- [ ] **Run as a non-root user.** Create a dedicated `mediamtx` service account.
- [ ] **Use systemd or equivalent** for process management with automatic restart.
- [ ] **Set file permissions.** Config file (`mediamtx.yml`): `640`, database: `600`, recording directory: `750`.
- [ ] **Disable pprof in production.** Set `pprof: false` unless actively profiling.
- [ ] **Disable debug logging in production.** Set `logLevel: info` or `warn` to reduce log volume and avoid leaking sensitive data in logs.
- [ ] **Enable structured logging.** Set `logStructured: true` and ship logs to a centralized log management system (ELK, Loki, etc.) for monitoring and alerting.
- [ ] **Keep the system updated.** Monitor for MediaMTX releases and apply updates. Use `GET /api/nvr/system/updates` to check for available updates.
- [ ] **Isolate the recording volume.** Mount recordings on a separate partition to prevent disk exhaustion from affecting the operating system.

### Monitoring

- [ ] **Enable Prometheus metrics.** Set `metrics: true` and scrape `:9998/metrics` from your monitoring stack.
- [ ] **Set up storage alerts.** Configure quota warning/critical thresholds and monitor `GET /api/nvr/storage/quota` status.
- [ ] **Monitor recording health.** Use `GET /api/nvr/recording/health` to detect cameras that have stopped recording.
- [ ] **Track connection events.** The NVR logs connection and disconnection events for forensic review.

---

## Quick Reference: Common API Endpoints

| Endpoint | Method | Auth | Description |
|----------|--------|------|-------------|
| `/api/nvr/auth/login` | POST | none | Obtain a JWT token. |
| `/api/nvr/users` | GET/POST | admin | List or create users. |
| `/api/nvr/users/:id` | GET/PUT/DELETE | admin | Manage a specific user. |
| `/api/nvr/cameras` | GET/POST | admin | List or add cameras. |
| `/api/nvr/recordings` | GET | operator+ | Query recording segments. |
| `/api/nvr/recording-rules` | GET/POST | admin | Manage recording rules. |
| `/api/nvr/storage/quota` | GET | admin | Check storage quota status. |
| `/api/nvr/audit` | GET | admin | View audit log. |
| `/api/nvr/system/backups` | GET/POST | admin | List or create backups. |
| `/api/nvr/system/backups/restore` | POST | admin | Restore from backup. |
| `/api/nvr/system/tls/status` | GET | admin | Check TLS certificate status. |
| `/api/nvr/system/updates` | GET | admin | Check for system updates. |

---

## Further Reading

- `mediamtx.yml` -- Inline comments document every configuration key.
- [MediaMTX upstream documentation](https://github.com/bluenviron/mediamtx) -- Protocol details, path configuration, and advanced features.
