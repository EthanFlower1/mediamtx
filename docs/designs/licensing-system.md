# KAI-102: Licensing System Design

## Overview

A licensing system for MediaMTX NVR that gates feature tiers, camera limits, and commercial capabilities. The system must work for both online (cloud-validated) and offline (air-gapped) deployments, reflecting the NVR's single-binary, self-hosted architecture.

When no license is present, MediaMTX NVR operates in a free tier with limited functionality. Paid licenses unlock higher camera counts, advanced features, and commercial use rights.

## Guiding Principles

- **Offline-first** -- NVR installations are often on isolated networks; licensing must work without internet
- **No phone-home requirement** -- online activation is an option, not a mandate
- **Graceful degradation** -- expired or missing licenses reduce functionality, never brick the system
- **Single binary** -- license validation is compiled into the binary, no external license server
- **Tamper-resistant but not DRM** -- protect against casual bypass; accept that determined attackers can crack any client-side system

## 1. License Models

### Tier Structure

| Tier | Camera Limit | Features | Use Case |
|------|-------------|----------|----------|
| **Community** | 4 cameras | Live view, recording, playback, ONVIF discovery, basic schedules | Home/hobbyist |
| **Pro** | 16 cameras | Community + AI analytics, smart detection, per-camera storage quotas, schedule templates, email alerts | Prosumer / small business |
| **Business** | 64 cameras | Pro + multi-user roles, audit logging, config backup/restore, LDAP/SSO (future), priority support | SMB / multi-site |
| **Enterprise** | Unlimited | Business + HA clustering (future), custom integrations, SLA support, white-labeling | Large deployments |

### License Types

**Subscription (annual)**
- Tied to a calendar year from activation date
- Includes updates released during the subscription period
- After expiry: downgrades to Community tier after grace period

**Perpetual + Maintenance**
- One-time purchase for the current major version
- Includes 1 year of updates (maintenance window)
- After maintenance expires: continues working on installed version, no new updates
- After major version bump: requires new license for the new major version

**Site License**
- Covers all instances at a single physical location
- Keyed to a site identifier rather than machine fingerprint
- Useful for enterprise deployments with many servers

### License Key Format

Ed25519-signed JSON Web Token (JWT) encoded as a base32-encoded string for human-friendly entry:

```
KAINVR-XXXX-XXXX-XXXX-XXXX-XXXX-XXXX-XXXX
```

The key decodes to a signed payload:

```json
{
  "iss": "kai-licensing",
  "sub": "customer-uuid",
  "iat": 1743638400,
  "exp": 1775174400,
  "tier": "pro",
  "type": "subscription",
  "cameras": 16,
  "features": ["ai_analytics", "smart_detection", "storage_quotas", "schedule_templates", "email_alerts"],
  "maintenance_exp": 1775174400,
  "site_id": "",
  "fingerprint_hash": "sha256:...",
  "offline": false
}
```

The public key for signature verification is embedded in the binary at compile time. Only the licensing authority holds the private key.

## 2. Activation Flow

### Online Activation

```
User enters license key in UI or CLI
         |
         v
NVR sends key + machine fingerprint to activation API
         |
         v
Activation API validates key, checks:
  - Key not revoked
  - Activation count within limit (e.g., 2 machines per license)
  - Key not expired
         |
         v
API returns signed activation certificate
         |
         v
NVR stores certificate in SQLite (licenses table)
         |
         v
NVR periodically phones home for renewal (every 7 days)
  - If server unreachable, enters grace period
  - If server confirms revocation, enters grace period then downgrades
```

**Activation API Endpoints (external service, not in this binary):**

- `POST /v1/activate` -- activate a license key for a machine
- `POST /v1/deactivate` -- release an activation slot
- `POST /v1/validate` -- periodic heartbeat / renewal
- `POST /v1/refresh` -- get updated certificate (e.g., after tier change)

### Offline Activation

For air-gapped installations:

```
User enters license key in UI
         |
         v
NVR generates activation request file containing:
  - License key
  - Machine fingerprint
  - NVR version
         |
         v
User transfers request file to internet-connected machine
         |
         v
User uploads request file to web portal (activate.example.com)
         |
         v
Portal returns signed activation certificate file
         |
         v
User transfers certificate file back to NVR
         |
         v
NVR imports and validates certificate, stores in SQLite
```

Offline certificates include a longer validity window (90 days vs 7 days for online) and can be renewed by repeating the process before expiry.

### CLI Activation

```bash
# Online
mediamtx --license-activate KAINVR-XXXX-XXXX-...

# Offline: generate request
mediamtx --license-request --output activation-request.json

# Offline: import certificate
mediamtx --license-import activation-certificate.json

# Check status
mediamtx --license-status

# Deactivate (frees activation slot)
mediamtx --license-deactivate
```

## 3. Machine Fingerprinting

The fingerprint uniquely identifies an installation without being fragile to hardware changes.

**Fingerprint components (hashed together with SHA-256):**

| Component | Source | Stability |
|-----------|--------|-----------|
| Machine ID | `/etc/machine-id` (Linux), `IOPlatformUUID` (macOS), `MachineGuid` registry key (Windows) | Stable across reboots |
| Database ID | Random UUID generated on first DB creation, stored in `system_config` table | Stable unless DB is recreated |

The fingerprint is a SHA-256 hash of these two values concatenated. Using two components means:
- Copying the binary to a new machine changes the Machine ID
- Copying the database to a new machine also changes the Machine ID
- Reinstalling on the same machine preserves the Machine ID; the Database ID changes but the activation certificate includes a tolerance for DB recreation within 24 hours

**Docker/VM considerations:**
- Docker: mount a persistent volume for `/etc/machine-id` or use the Database ID as primary identifier
- VM clones: detected by duplicate fingerprints during online validation; offline mode trusts the certificate

## 4. Enforcement Architecture

### Package Structure

```
internal/nvr/licensing/
    licensing.go        -- Manager struct, initialization, public API
    activation.go       -- Online/offline activation logic
    certificate.go      -- Certificate parsing, signature verification
    enforcement.go      -- Feature gate checks
    fingerprint.go      -- Machine fingerprint generation
    grace.go            -- Grace period management
    store.go            -- SQLite persistence (licenses table)
    licensing_test.go   -- Unit tests
```

### Database Schema (New Migration)

```sql
CREATE TABLE licenses (
    id TEXT PRIMARY KEY,
    license_key TEXT NOT NULL,
    tier TEXT NOT NULL,
    type TEXT NOT NULL,
    cameras_allowed INTEGER NOT NULL,
    features TEXT NOT NULL,          -- JSON array
    activated_at TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    maintenance_expires_at TEXT,
    certificate TEXT NOT NULL,       -- signed activation certificate (JWT)
    fingerprint_hash TEXT NOT NULL,
    last_validated_at TEXT,
    grace_deadline TEXT,             -- set when validation fails
    status TEXT NOT NULL DEFAULT 'active',  -- active, grace, expired, revoked
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE license_audit_log (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    license_id TEXT NOT NULL,
    event TEXT NOT NULL,             -- activated, validated, grace_entered, expired, revoked, deactivated
    details TEXT,                    -- JSON with context
    created_at TEXT NOT NULL
);
```

### Enforcement Points

The `licensing.Manager` exposes a simple API consumed by other NVR subsystems:

```go
type Manager struct {
    db         *db.DB
    publicKey  ed25519.PublicKey
    current    *License          // cached active license
    mu         sync.RWMutex
}

// Feature gating
func (m *Manager) Tier() Tier                    // Community, Pro, Business, Enterprise
func (m *Manager) MaxCameras() int               // camera limit for current tier
func (m *Manager) HasFeature(f Feature) bool     // check specific feature flag
func (m *Manager) IsActive() bool                // license valid and not in grace
func (m *Manager) IsGrace() bool                 // in grace period
func (m *Manager) GraceDeadline() time.Time      // when grace expires
func (m *Manager) Status() LicenseStatus         // full status for UI display
```

**Where enforcement happens:**

| Check | Location | Behavior When Unlicensed |
|-------|----------|--------------------------|
| Camera count | `POST /api/cameras` (camera creation) | Reject with 402 + message indicating tier limit |
| AI analytics | AI pipeline startup | Skip pipeline init, log warning |
| Smart detection | Detection event processing | Events still recorded but not processed |
| Storage quotas | Quota configuration API | Reject quota configuration, allow unlimited with default retention |
| Schedule templates | Schedule template API | Reject template creation, allow basic schedules |
| Email alerts | Alert dispatch | Log alert locally, skip email send |
| Multi-user | User creation API | Reject non-admin user creation |
| Audit logging | Audit log middleware | Skip audit log writes |
| Config backup | Backup API | Reject backup/restore operations |

**Enforcement is advisory at the API layer, not at the data layer.** If a camera was added while licensed and the license later expires, the camera continues to function. New cameras beyond the Community limit cannot be added. This avoids disrupting active surveillance.

### Integration with NVR Startup

```go
// In nvr.go Initialize()
func (n *NVR) Initialize() error {
    // ... existing init ...

    // Initialize licensing manager
    n.licenseMgr = licensing.NewManager(n.database, embeddedPublicKey)
    if err := n.licenseMgr.Load(); err != nil {
        log.Printf("licensing: failed to load license: %v", err)
        // Continue with Community tier -- never fail startup due to licensing
    }

    // Start background validation (online mode)
    if n.licenseMgr.IsActive() && !n.licenseMgr.IsOffline() {
        go n.licenseMgr.StartPeriodicValidation(n.ctx, 7*24*time.Hour)
    }

    // ... rest of init uses n.licenseMgr for feature gates ...
}
```

## 5. Grace Periods

Grace periods prevent sudden service disruption when a license cannot be validated.

| Scenario | Grace Period | Behavior During Grace | After Grace Expires |
|----------|-------------|----------------------|---------------------|
| Online validation fails (network issue) | 30 days | Full licensed functionality continues | Downgrade to Community tier |
| Subscription expires | 14 days | Full licensed functionality continues | Downgrade to Community tier |
| License revoked (online detection) | 7 days | Warning banner in UI, full functionality | Downgrade to Community tier |
| Offline certificate expires | 30 days | Warning banner in UI, full functionality | Downgrade to Community tier |
| Perpetual license + major version upgrade | 90 days | Full functionality on new version | Downgrade to Community tier on new version; old version unaffected |

**Grace period behavior:**
- Warning banner displayed in UI with countdown
- System alert generated (email if configured)
- Audit log entry created
- All licensed features continue to work
- No data loss or recording interruption

**Grace period state machine:**

```
active --> grace_network    (validation failed)
active --> grace_expiring   (< 14 days to expiry)
active --> grace_revoked    (revocation detected)
grace_* --> active          (successful revalidation / renewal)
grace_* --> expired         (grace deadline passed)
expired --> active          (new license activated)
```

## 6. Anti-Tampering Measures

The goal is to make casual bypass inconvenient without implementing invasive DRM that harms legitimate users.

### Compile-Time Measures

- **Embedded public key:** The Ed25519 verification key is compiled into the binary. Replacing it requires recompilation.
- **Build-time signing:** Release binaries are signed. The updater verifies binary signatures before applying updates. A tampered binary would fail the update signature check.
- **Obfuscated key storage:** The public key bytes are split across multiple variables and reconstructed at runtime, making simple binary patching harder.

### Runtime Measures

- **Certificate signature verification:** Every license operation verifies the Ed25519 signature on the activation certificate. Invalid signatures are rejected.
- **Fingerprint binding:** Certificates are bound to the machine fingerprint. Moving a certificate to another machine invalidates it.
- **Periodic re-verification:** The license is re-verified from the stored certificate on every NVR restart and periodically during operation (every 6 hours from the local certificate, every 7 days from the remote server).
- **Clock tampering detection:** The system records the last-seen timestamp in SQLite. If the current time is earlier than the last-seen time by more than 1 hour, the license enters grace period and logs a clock anomaly. NTP sync status is checked when available.
- **Audit trail:** All license state changes are logged to `license_audit_log` with timestamps. Unusual patterns (repeated activations, fingerprint changes) are flagged.

### What We Deliberately Do NOT Do

- **No kernel-level protection** -- too invasive, breaks containers
- **No network-mandatory validation** -- offline deployments are a core use case
- **No hardware dongles** -- incompatible with single-binary philosophy
- **No code obfuscation** -- Go binaries are easy to reverse regardless; obfuscation hurts debugging more than it deters attackers
- **No disabling of running cameras** -- a security system must never stop recording due to a licensing issue

## 7. API Endpoints

All endpoints are under the existing NVR API router with JWT authentication.

### License Management

```
GET    /api/license              -- Get current license status (tier, cameras, features, expiry)
POST   /api/license/activate     -- Activate a license key (online)
POST   /api/license/request      -- Generate offline activation request
POST   /api/license/import       -- Import offline activation certificate
POST   /api/license/deactivate   -- Deactivate current license (frees slot)
GET    /api/license/audit        -- Get license event audit log
```

### Response Shapes

```json
// GET /api/license
{
  "tier": "pro",
  "type": "subscription",
  "status": "active",
  "camerasAllowed": 16,
  "camerasUsed": 7,
  "features": ["ai_analytics", "smart_detection", "storage_quotas", "schedule_templates", "email_alerts"],
  "expiresAt": "2027-04-03T00:00:00Z",
  "maintenanceExpiresAt": "2027-04-03T00:00:00Z",
  "graceDeadline": null,
  "isOffline": false,
  "lastValidatedAt": "2026-04-03T12:00:00Z"
}

// POST /api/license/activate  (request body)
{
  "licenseKey": "KAINVR-XXXX-XXXX-XXXX-XXXX-XXXX-XXXX-XXXX"
}

// POST /api/license/request  (response)
{
  "requestFile": "<base64-encoded activation request>"
}
```

## 8. UI Integration

### Admin Console (React)

- **License page** (`/settings/license`): shows current tier, camera usage, feature list, expiry date, activation/deactivation controls
- **Upgrade prompts**: when a user attempts a gated action, show inline prompt with tier required
- **Grace period banner**: persistent warning bar at top of admin console during grace periods
- **Offline workflow**: step-by-step wizard for generating request, downloading, and importing certificate

### Flutter Client

- **Settings screen**: read-only license status display
- **Feature badges**: visual indicators on features that require a higher tier

## 9. Migration Path

### From Unlicensed to Licensed

All existing functionality continues. Entering a license key unlocks gated features immediately. No data migration needed.

### From Licensed to Unlicensed (Downgrade)

- Cameras beyond the limit continue to function (existing cameras are never disabled)
- New cameras cannot be added beyond the Community limit
- Gated features become read-only (existing data is preserved, new operations are blocked)
- AI pipelines stop processing but detection history is retained
- Storage quotas stop enforcing but existing quota configuration is preserved in the database

### Version Upgrades

- License certificates include a `maintenance_exp` field
- The binary embeds its build version
- If `maintenance_exp < build_date`, the license is valid for the installed features but update checks will indicate maintenance renewal is needed

## 10. Security Considerations

- **License keys in transit**: all activation API calls use TLS
- **License keys at rest**: stored in SQLite, the license key itself is not sensitive (it is a signed token, not a secret), but the database is already protected by the NVR's encryption layer
- **Public key compromise**: if the embedded public key is extracted, an attacker can verify but not create licenses; the private key never leaves the licensing server
- **Revocation**: online mode checks a revocation list during periodic validation; offline mode relies on certificate expiry

## 11. Future Considerations (Out of Scope for v1)

- **License server self-hosting**: allow enterprise customers to run their own activation server
- **Floating licenses**: pool of licenses shared across an organization
- **Usage-based billing**: metered by camera-hours or storage consumed
- **LDAP/SSO gating**: Enterprise tier prerequisite for SSO integration
- **HA clustering license**: special license type for clustered deployments
- **Reseller/OEM licensing**: white-label licenses with custom branding
