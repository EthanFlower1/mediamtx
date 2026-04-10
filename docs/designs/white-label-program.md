# White-Label Program - Design Document

**Ticket:** KAI-106
**Status:** Design
**Author:** Commercial Platform Team
**Date:** 2026-04-03

---

## 1. Overview

The White-Label Program enables OEM partners and system integrators to rebrand MediaMTX NVR as their own product. This includes custom branding (logos, colors, names), a build pipeline that produces partner-branded binaries and installers, an integrator portal for managing deployments, and a licensing framework.

## 2. Goals

- Define the complete branding scope: what can and cannot be customized
- Automated OEM build pipeline that produces branded artifacts from a single codebase
- Self-service integrator portal for partner management
- Clear licensing model with runtime enforcement
- Maintain a single codebase with no partner-specific forks

## 3. Branding Scope

### 3.1 Customizable Elements

| Category      | Element                   | Customization                                  |
| ------------- | ------------------------- | ---------------------------------------------- |
| **Identity**  | Product name              | Full replacement (e.g., "SecureView NVR")      |
|               | Company name              | Displayed in UI footer, about page, CLI output |
|               | Logo (primary)            | SVG, min 200x60px, used in header/login        |
|               | Logo (icon)               | SVG/PNG, 512x512, used for favicon, app icon   |
|               | Splash screen             | Custom image for desktop/mobile app launch     |
| **UI Theme**  | Primary color             | Hex color applied to buttons, links, accents   |
|               | Secondary color           | Hex color for secondary actions                |
|               | Background color          | Light/dark mode base colors                    |
|               | Font family               | Google Fonts or custom WOFF2 upload            |
|               | Login page background     | Custom image or gradient                       |
| **Content**   | Terms of service URL      | Link in footer and registration                |
|               | Privacy policy URL        | Link in footer and registration                |
|               | Support URL               | Link in help menu                              |
|               | Documentation URL         | Link in help menu                              |
| **Technical** | Default NTP server        | Pre-configured in branded builds               |
|               | Default update server URL | Points to partner's update infrastructure      |
|               | Telemetry endpoint        | Partner's own analytics endpoint (or disabled) |

### 3.2 Non-Customizable Elements

- Core protocol handling (RTSP, HLS, WebRTC)
- Recording engine and storage format
- ONVIF implementation
- Security model and encryption
- API endpoint structure (paths and response format)
- Database schema

### 3.3 Brand Configuration File

Partners define their branding in a YAML configuration:

```yaml
# brand.yml
brand:
  product_name: "SecureView NVR"
  company_name: "SecureView Inc."
  logo_primary: "assets/logo.svg"
  logo_icon: "assets/icon.png"
  splash: "assets/splash.png"

  theme:
    primary_color: "#1B5E20"
    secondary_color: "#4CAF50"
    background_light: "#FAFAFA"
    background_dark: "#121212"
    font_family: "Inter"

  links:
    terms_url: "https://secureview.com/terms"
    privacy_url: "https://secureview.com/privacy"
    support_url: "https://secureview.com/support"
    docs_url: "https://docs.secureview.com"

  defaults:
    ntp_server: "ntp.secureview.com"
    update_server: "https://updates.secureview.com"
    telemetry_endpoint: "https://telemetry.secureview.com/v1/events"

  license:
    partner_id: "partner-uuid"
    tier: "professional"
```

## 4. OEM Build Pipeline

### 4.1 Pipeline Architecture

```
brand.yml + assets/
       |
       v
+------------------+     +------------------+     +-------------------+
| Brand Validator  | --> | Build Generator  | --> | Artifact Publisher |
| (lint + verify)  |     | (Go + Flutter +  |     | (S3 + CDN)        |
|                  |     |  React builds)   |     |                   |
+------------------+     +------------------+     +-------------------+
```

### 4.2 Build Steps

1. **Validate brand config:** Lint `brand.yml`, verify asset dimensions/formats, check required fields.
2. **Inject branding into source:**
   - Go: Generate `internal/brand/generated.go` with compile-time constants (product name, URLs).
   - React UI: Generate `ui/src/brand/config.json` and copy logo assets to `ui/public/`.
   - Flutter: Generate `clients/flutter/lib/brand/config.dart` and replace app icons via `flutter_launcher_icons`.
3. **Build artifacts:**
   - Go binary: `linux/amd64`, `linux/arm64`, `darwin/amd64`, `darwin/arm64`, `windows/amd64`
   - Docker image: Multi-arch with partner branding baked in
   - Flutter APK/IPA: Signed with partner's code signing certificates
   - Installer packages: `.deb`, `.rpm`, `.msi` with partner branding
4. **Sign artifacts:** All binaries are code-signed. Partners provide their signing certificates; a fallback MediaMTX signing key is used if not provided.
5. **Publish:** Artifacts are uploaded to the partner's configured update server and/or the integrator portal.

### 4.3 CI/CD Integration

- Pipeline runs in GitHub Actions (or self-hosted runners for partners requiring air-gapped builds).
- Partner brands are stored in a private `brands/` repository.
- A build is triggered on every MediaMTX release tag, producing branded artifacts for all active partners.
- Partners can also trigger ad-hoc builds from the integrator portal.

### 4.4 Build Matrix

| Artifact          | Platforms                                                           | Format                        |
| ----------------- | ------------------------------------------------------------------- | ----------------------------- |
| Server binary     | linux/amd64, linux/arm64, darwin/amd64, darwin/arm64, windows/amd64 | tar.gz, zip                   |
| Docker image      | linux/amd64, linux/arm64                                            | OCI                           |
| Linux installer   | Debian, RPM                                                         | .deb, .rpm                    |
| Windows installer | Windows 10+                                                         | .msi                          |
| Android app       | Android 8+                                                          | .apk, .aab                    |
| iOS app           | iOS 15+                                                             | .ipa (TestFlight / App Store) |

## 5. Integrator Portal

### 5.1 Purpose

A web portal where OEM partners manage their white-label deployments, licenses, and builds.

### 5.2 Portal Features

| Feature                | Description                                           |
| ---------------------- | ----------------------------------------------------- |
| **Brand Management**   | Upload/edit `brand.yml` and assets via web form       |
| **Build Dashboard**    | View build status, download artifacts, trigger builds |
| **License Management** | Generate, revoke, and monitor license keys            |
| **Deployment Tracker** | See all deployed instances, versions, and health      |
| **Update Management**  | Stage and publish OTA updates to deployed servers     |
| **Analytics**          | Aggregate telemetry from partner deployments (opt-in) |
| **Billing**            | View invoices, usage metrics, plan details            |
| **Support Escalation** | Escalate end-customer tickets to MediaMTX support     |

### 5.3 Portal Access Roles

| Role              | Permissions                                         |
| ----------------- | --------------------------------------------------- |
| Partner Admin     | Full access to all portal features                  |
| Partner Developer | Brand config, builds, deployment tracking           |
| Partner Support   | License management, deployment tracking, escalation |
| MediaMTX Admin    | Cross-partner view, billing management, support     |

## 6. Partner Licensing

### 6.1 License Model

| Tier             | Camera Limit     | Features                          | Support                    |
| ---------------- | ---------------- | --------------------------------- | -------------------------- |
| **Starter**      | Up to 16 cameras | Core NVR, local UI                | Email (48h SLA)            |
| **Professional** | Up to 64 cameras | Core + cloud portal, federation   | Email + chat (24h SLA)     |
| **Enterprise**   | Unlimited        | All features, custom integrations | Dedicated support (4h SLA) |

### 6.2 License Key Format

```
MNTX-{PARTNER_ID_SHORT}-{TIER}-{CAMERA_LIMIT}-{EXPIRY_YYYYMMDD}-{CHECKSUM}

Example:
MNTX-SV01-PRO-064-20270403-A7F3B2
```

### 6.3 Runtime License Enforcement

- On startup, the NVR server validates the license key:
  1. Parse and verify checksum (HMAC-SHA256 with a shared secret).
  2. Check expiry date.
  3. Verify camera count does not exceed limit.
- If the license is invalid or expired:
  - Existing recordings remain accessible (read-only).
  - New camera additions are blocked.
  - A banner is displayed in the UI: "License expired. Contact your provider."
- License validation is performed locally (no phone-home required), but cloud-connected servers also report license status to the portal.

### 6.4 License API

| Method | Endpoint                 | Description                                 |
| ------ | ------------------------ | ------------------------------------------- |
| POST   | `/v1/license/activate`   | Activate a license key on this server       |
| GET    | `/v1/license`            | Get current license status                  |
| POST   | `/v1/license/deactivate` | Deactivate (for transfer to another server) |

## 7. Partner Onboarding Flow

1. Partner signs agreement and selects tier.
2. MediaMTX creates partner account in integrator portal.
3. Partner uploads `brand.yml` and assets.
4. Brand validation runs; partner previews branded UI in a sandbox.
5. Partner provides code signing certificates (optional).
6. Initial build is triggered; partner downloads and tests artifacts.
7. Partner generates license keys for their customers.
8. Partner distributes branded NVR + license keys to end customers.

## 8. Legal and Compliance

- Partners must include "Powered by MediaMTX" attribution in the About page (configurable text, not removable).
- Open-source license compliance: the branded build includes the same NOTICE and LICENSE files.
- Partners are responsible for their own App Store / Play Store listings and compliance.
- Data processing agreement (DPA) required for partners handling telemetry.

## 9. Open Questions

- Should partners be able to add custom plugins/extensions to their branded builds?
- Revenue share model vs. flat licensing fee -- which is preferred?
- Should the "Powered by MediaMTX" attribution be waivable for enterprise tier?
- Co-branding support: can a partner show both their logo and a sub-partner's logo?
- App Store distribution: should MediaMTX manage a single multi-tenant app, or should each partner have their own App Store listing?
