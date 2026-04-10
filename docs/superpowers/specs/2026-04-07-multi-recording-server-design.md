# Industry-Leading Multi-Tenant Cloud-Native VMS — Design Document

**Status:** Design (revised — wholesale rewrite of original 2026-04-07 spec)
**Date:** 2026-04-07
**Author:** Ethan Flower (with Claude)
**Supersedes:** the original "Multi-Recording-Server Architecture" version of this document
**Related docs:** `docs/designs/cloud-management-portal.md` (KAI-103, partially absorbed), `docs/designs/multi-site-federation.md` (KAI-104, partially absorbed), `docs/designs/white-label-program.md`

---

## 0. How to Read This Document

This is a **complete rewrite** of the original Multi-Recording-Server design. The original document described a multi-Recorder architecture with optional cloud add-ons. This document describes a **cloud-native multi-tenant integrator-first VMS platform** that competes simultaneously across the cloud-native (Verkada/Eagle Eye/Rhombus), enterprise on-prem (Milestone/Genetec/Avigilon), and SMB (UniFi/Synology) segments.

The strategic mandate is **"best product in the space, dominate all markets, no compromises on quality"** with runway and timeline relaxed as constraints. The product shape is therefore much larger than the original design, and more deliberately optimized for the integrator channel (security installers, MSPs, VARs) as the primary go-to-market.

The document is dense and long because the scope is large. Sections are roughly self-contained — read the architecture overview first, then jump to the section relevant to your work.

---

## 1. Executive Summary

The product is a **multi-tenant cloud-native VMS platform** built primarily for the **integrator (security installer / MSP) channel**, with a secondary direct-customer path. It runs on customer hardware (the on-prem Recorders), is managed via a **cloud control plane** (the Directory + Integrator Portal SaaS), and is consumed by end users via a **cross-platform Flutter app** (mobile + desktop + web), a **Qt/C++ video wall client** for SOC operators, and a **React web admin** for configuration.

The architecture is **hybrid by design**: the cloud is the system of record for identity, multi-tenant fleet management, integrator portals, and cross-customer features; the on-prem Recorders and Directories are the system of record for video data, recording continuity, and air-gap-capable operation. **Recording never depends on the cloud being reachable.** Cloud-only customers, on-prem-only customers, and hybrid customers are all first-class.

The key strategic differentiators against incumbents:

1. **Multi-tenant integrator-first model** — three-level integrator hierarchy (sub-resellers), many-to-many customer-integrator relationships, customer-controlled scoped permissions, full white-label including per-integrator mobile app builds. **No competitor in the space supports this fully.**
2. **Cloud-native ergonomics + on-prem-capable deployment** — the only product that matches Verkada's cloud experience while also supporting Milestone's on-prem and air-gap requirements. Air-gapped customers and federal/defense are first-class, not afterthoughts.
3. **Industry-leading AI** — 11 AI feature categories spanning object detection, face recognition, LPR, behavioral analytics, audio analytics, smart natural-language search via CLIP embeddings, cross-camera tracking, anomaly detection, AI-generated event summaries, multi-faceted forensic search, and customer-uploaded custom models with sandboxed execution.
4. **Compliance built in from day one** — SOC 2 Type I + HIPAA-ready + GDPR + CCPA + FIPS-validated cryptography + Section 508 + EU AI Act + pen test + bug bounty all at v1 launch. SOC 2 Type II and ISO 27001 within 12 months. FedRAMP when first federal customer signs.
5. **Recording stays on customer hardware** — video bytes never transit the cloud unless the customer explicitly enables cloud archive. Tiered cloud archive uses Cloudflare R2 to eliminate egress costs.
6. **Open standards and no vendor lock-in** — works with any ONVIF-compliant camera, exports data in standard formats, no proprietary file formats, customer-owned encryption keys available.

### v1 Scope at a Glance

- **One Go binary** in three runtime modes (`directory`, `recorder`, `all-in-one`) for the on-prem components, plus the **cloud platform** as a separate Kubernetes-deployed multi-tenant SaaS.
- **One Flutter codebase** targeting iOS, Android, macOS, Windows, Linux, and Web for the end-user viewing experience with feature parity across all six surfaces.
- **One React codebase** with two runtime contexts (customer admin + integrator portal) for browser-based configuration.
- **One Qt 6 / C++ desktop client** (Windows-first, Linux secondary) for SOC video wall operators.
- **One Next.js marketing website** at `yourbrand.com` with multi-language support, SEO, lead capture, comparison pages, customer case studies, integrator directory.
- **One Mintlify documentation portal** with comprehensive docs across user / admin / integrator / developer audiences, video tutorials, AI-powered help search, multi-language, white-label per integrator.
- **Multi-tenant SaaS cloud control plane** running on AWS (US-East-2 in v1, multi-region-ready architecture).
- **Embedded sidecars** managed by the on-prem binary: Zitadel (identity), MediaMTX (streaming engine).
- **Embedded Go libraries**: `tsnet` (mesh networking), Headscale (mesh coordinator), step-ca (cluster CA), Casbin (authorization), Lego (Let's Encrypt), goupnp (UPnP).
- **AI/ML platform**: hybrid edge + cloud inference, pgvector for semantic search, custom model upload with sandboxed execution, eleven feature categories at v1 quality.
- **Compliance program**: SOC 2 Type I, HIPAA-ready, GDPR/CCPA, FIPS-validated crypto, Section 508 / WCAG 2.1 AA, EU AI Act compliance, pen test, bug bounty all at v1 launch.
- **Pricing & billing**: per-camera per-month metering, Stripe Connect for hybrid customer-direct + integrator-rebill billing, four tiers (Free / Starter / Pro / Enterprise) plus per-feature add-ons.
- **Hardware**: software-only with hardware compatibility program (no physical inventory in v1). Reference appliance considered for v1.x. Full hardware program for v2.

### What's deferred from v1

- **Migration tools from competitors** (Milestone, Genetec, Verkada, Eagle Eye, etc.) → v1.x. Single-NVR self-migration from existing customers stays in v1.
- **Hardware appliance program** → v1.x reference appliance, v2 full multi-SKU program.
- **Marketplace for third-party integrations and custom AI models** → v2. Architecture is built so it's a non-breaking addition.
- **Java + C# SDKs** → v1.x. Python + Go + TypeScript ship in v1.
- **Multiple federations per Directory** → v2. One federation per Directory in v1.
- **Custom marketing copy + content per integrator** → covered by white-label Level 3 in v1.
- **Apple Watch / Siri / CarPlay / AR features / real-time talkback translation / 12+ language support** → vanity features cut from v1.

### Realistic v1 effort and team size

- **Total v1 engineering effort**: ~975 engineer-weeks
- **Engineering team needed**: ~22-28 senior engineers across backend (Go), mobile (Flutter), frontend (React), cloud/SRE, ML/AI, video wall (C++/Qt), marketing site (Next.js), DevOps + Head of Security & Compliance + 2 customer success + 2 marketing + 1 PM + 1 designer + 2 technical writers + 1 hardware ops + 3-5 sales = ~50 people total org for v1 development
- **Calendar time to feature-complete v1**: ~18-24 months focused work + 3-6 months pre-launch buffer for security review, integration testing, private beta, compliance audits
- **Capital commitment to v1 launch**: ~$15-25M to fund team, infrastructure, compliance, sales/marketing, ops
- **Post-launch ongoing burn**: ~$10-20M/year

This is a Series A / early Series B scale company commitment.

---

## 2. Strategic Context and Goals

### 2.1 Market positioning

The VMS market is currently segmented into three categories with little overlap:

| Segment                | Leaders                                                          | Strengths                                                                                        | Weaknesses                                                                 |
| ---------------------- | ---------------------------------------------------------------- | ------------------------------------------------------------------------------------------------ | -------------------------------------------------------------------------- |
| **Cloud-native**       | Verkada, Eagle Eye Networks, Rhombus                             | Polished UX, AI quality, mobile-first, easy to onboard                                           | Expensive, vendor lock-in (Verkada hardware), no air-gap, weak white-label |
| **Enterprise on-prem** | Milestone XProtect, Genetec Security Center, Avigilon (Motorola) | Deep integrations, ONVIF compliance, federation, mature enterprise features, on-prem reliability | Dated UX, slow innovation, weak AI, complex installation, premium pricing  |
| **SMB / prosumer**     | UniFi Protect, Synology Surveillance Station, Hanwha WAVE        | Low cost, easy install, hardware integration                                                     | Limited features, weak enterprise support, no integrator program           |

**The unfilled gap is "cloud-native ergonomics + enterprise depth + integrator-first model + on-prem-capable deployment in the same product."** No incumbent fills it. Building this product is the strategic wedge.

### 2.2 Primary go-to-market: integrator channel

Security integrators (also called installers, VARs, MSPs in adjacent terminology) are the primary buyer and channel for this product. They install, configure, and manage security systems at customer sites, often serving dozens to hundreds of end customers each. Their key requirements:

1. **White-label** — they want their brand visible to end customers, not yours
2. **Multi-customer fleet management** — they want one dashboard showing all their customers' systems
3. **Bulk operations** — they want to push updates, configure features, troubleshoot across many customers from one place
4. **Recurring revenue** — they want to bill customers monthly with their own markup over your wholesale price
5. **Flexible deployment** — some customers want cloud-managed, some want on-prem, some want hybrid; they need to support all
6. **Hardware flexibility** — they have existing hardware supplier relationships and don't want to be locked into yours
7. **Channel partnership** — they want training, sales tools, marketing collateral, certification programs

The product is designed to make integrators evangelists, not just users. The integrator-first GTM is the difference between "we sell into the VMS market" and "we dominate the VMS market through the existing channel."

### 2.3 Secondary go-to-market: direct customers

Direct customers (no integrator) are also supported in v1. A small business owner finds the marketing site, signs up self-serve, configures the product themselves, and never interacts with an integrator. The product handles this case with the same architecture — it's a `customer_tenant` with no `integrator_relationships` rows. The same React admin works. The same Flutter client works. The same billing infrastructure works.

Direct customers are valuable because:

- They generate inbound demand the marketing site can capture
- They self-validate the product (PLG)
- They occasionally upgrade to enterprise tier and become large accounts
- Some eventually adopt an integrator relationship later, generating channel demand

### 2.4 Goals

1. **Be the best product in the VMS space across every dimension that matters**: streaming performance, AI quality, multi-tenant integrator features, white-label depth, compliance posture, deployment flexibility, customer experience, integrator experience, and total cost of ownership.
2. **Make integrators evangelists** by giving them the white-label, fleet management, and channel features that no other VMS product provides at v1 quality.
3. **Reach enterprise / regulated customers** (healthcare, finance, government, defense) by shipping comprehensive compliance from v1 launch.
4. **Reach cloud-native customers** with Verkada-quality cloud experience while also offering on-prem and hybrid deployment as first-class options.
5. **Build for multi-region scalability from day one** while shipping single-region in v1 to reduce operational cost during validation.
6. **Maintain recording-as-most-resilient-operation invariant**: the system can lose almost any other capability during failure, but recording never stops as long as the Recorder has power and disk.
7. **Don't invent security primitives**: use embedded Zitadel for identity, embedded step-ca for PKI, FIPS-validated cryptography libraries, and well-tested upstream open-source components throughout.

### 2.5 Non-Goals

1. **Not a single-tenant product (in the traditional SaaS sense)** — the cloud is multi-tenant from day one. The on-prem Directory can run as a single-tenant deployment for air-gapped customers, but the cloud isn't.
2. **Not a hardware-first product (in v1)** — hardware compatibility program in v1, optional reference appliance in v1.x, full hardware SKU program in v2. v1 is software with certified hardware.
3. **Not a Verkada clone** — the cloud is a deployment option, not a hard requirement. Customers retain ownership of their video data and can deploy fully on-prem if they want.
4. **Not a competitor migration product (in v1)** — v1 focuses on greenfield customers. Competitor migration tools are v1.x.
5. **Not federation across organizations** — federation in v1 links sites within one customer organization. Cross-organization federation (e.g., a city government federating with a private security firm) is out of scope.
6. **Not a marketplace platform (in v1)** — third-party integrations marketplace and custom AI model marketplace are v2 features. v1 ships first-party integrations and the API/SDK platform that the marketplace will eventually be built on.

---

## 3. Background and Current State

The starting point is the existing MediaMTX NVR product — a single-binary Go application with:

- Core engine in `internal/nvr/`
- SQLite database with cameras, recordings, detection events, audit logs, users
- REST API in `internal/nvr/api/`
- Storage manager in `internal/nvr/storage/`
- HLS playback in `internal/nvr/api/hls.go`
- Auth subsystem in `internal/nvr/api/auth.go` (RSA-signed JWTs, JWKS endpoint, brute-force protection)
- React admin UI in `ui/`, embedded in the Go binary via `//go:embed`
- Flutter client in `clients/flutter/` with single-server connection model
- ONVIF integration in `internal/nvr/onvif/`

**The current architecture has zero concept of multi-tenancy, no cloud component, no integrator hierarchy, no federation, no AI beyond basic motion detection, and no compliance program.** This rewrite is a transformation from "single-NVR product with potential" to "industry-leading multi-tenant cloud platform."

A few existing design documents partially overlap with this rewrite:

- `docs/designs/cloud-management-portal.md` (KAI-103) — describes a cloud portal for fleet management. This rewrite absorbs and supersedes most of it, expanding significantly.
- `docs/designs/multi-site-federation.md` (KAI-104) — describes Directory-to-Directory federation. This rewrite carries it forward with cloud-first adjustments.
- `docs/designs/white-label-program.md` — describes white-label conceptually. This rewrite specifies it concretely as Level 3 with per-integrator mobile builds.

---

## 4. Architecture Overview

### 4.1 The five-surface customer model

The product presents five distinct customer-facing surfaces, each with the right tool for its job:

| #   | Surface                  | Tech                                       | Targets                                      | Audience                                                                          |
| --- | ------------------------ | ------------------------------------------ | -------------------------------------------- | --------------------------------------------------------------------------------- |
| 1   | **End-user viewing app** | Flutter                                    | iOS, Android, macOS, Windows, Linux, **Web** | End users (security operators, business owners, anyone watching cameras)          |
| 2   | **Admin web app**        | React (one codebase, two runtime contexts) | Web browser only                             | Customer admins (single-tenant context) + integrator staff (multi-tenant context) |
| 3   | **Video wall client**    | Qt 6 / C++ native desktop                  | Windows primary, Linux secondary             | SOC operators driving multi-monitor displays                                      |
| 4   | **Marketing website**    | Next.js + Sanity CMS                       | Public web                                   | Prospects, lead capture, SEO                                                      |
| 5   | **Documentation portal** | Mintlify                                   | Public web + searchable                      | Developers, integrators, customers                                                |

The end-user app's **single Flutter codebase compiles to all six end-user targets with feature parity guaranteed**. This is the central architectural commitment for the customer experience: a feature shipped on iOS automatically appears on every other target without separate work. Maintaining feature parity across multiple separately-implemented codebases is the failure mode this avoids.

The admin web app uses **one React codebase with two runtime contexts** (GitHub-style). When served from `app.acme.cloud.yourbrand.com` it presents the customer admin context with single-tenant scope. When served from `command.yourbrand.com` it presents the integrator portal context with multi-tenant scope. Same components, same design system, same auth logic.

### 4.2 Three on-prem roles

The on-prem product (the Go binary that customers install on their hardware) has three runtime roles, selectable by config:

| Mode         | Subsystems                                                      | Typical deployment                                  |
| ------------ | --------------------------------------------------------------- | --------------------------------------------------- |
| `directory`  | Directory only (identity client + camera registry + sidecars)   | Dedicated directory host for large customers        |
| `recorder`   | Recorder only (camera capture, recording, local stream serving) | Standalone recording nodes joined to a Directory    |
| `all-in-one` | Both Directory and Recorder in one process                      | Default for SMB / mid-market customers with one box |

A **Gateway** subsystem handles streaming proxy duties for off-LAN clients. It's co-resident with the Directory in v1 (same binary, separable internal subsystem), can be split to its own process in v2.

### 4.3 The cloud platform

The cloud is a **multi-tenant SaaS** running on AWS, Kubernetes-orchestrated (EKS), with the architecture deliberately built to support multi-region active-active deployment when scale demands it (v1.x or v2). v1 ships in a single region (US-East-2).

The cloud has three primary responsibilities:

1. **Identity authority for cloud-managed customers** — multi-tenant Zitadel-backed identity that issues tokens consumed by on-prem Recorders, the Flutter app, the React admin, and the video wall client
2. **Fleet management surface for integrators** — the multi-tenant control plane where integrators see all their customers, configure white-label, manage billing, run diagnostics, push updates
3. **Optional cloud archive for recordings** — Cloudflare R2-backed object storage with tiered retention (Hot / Warm / Cold / Archive) and customer-controlled encryption options (Standard / SSE-KMS / Client-side CMK)

The cloud is **never in the path for live recording**. It's never in the path for LAN-direct streaming. It's never in the path for on-prem Directory ↔ Recorder communication. It's never in the path for air-gapped customers. **Air-gapped customers can use the entire product without ever touching the cloud.**

### 4.4 What's embedded vs. what's a sidecar

After careful analysis of which open-source projects are designed for embedding vs. which are designed to be services, the architecture splits as follows:

**Embedded as Go libraries** (linked into the on-prem binary):

- `tailscale.com/tsnet` — Tailscale's officially supported embeddable Go library. Provides internal mesh networking between Directory, Recorder, and Gateway components. Stable public API.
- **Headscale** (`hscontrol`) — open-source Tailscale coordination service. Embedded into the Directory binary as the per-site tailnet coordinator. Smaller codebase, narrower API surface, slower-moving than Zitadel — embeddable with eyes open and budgeted ~1 day per upgrade for adapter fixes.
- **step-ca** (`github.com/smallstep/certificates/authority`) — Smallstep's CA, designed to be embedded. Provides the per-site cluster CA for mTLS between Directory ↔ Recorder ↔ Gateway. Stable public API.
- **Casbin** (`github.com/casbin/casbin/v2`) — authorization library, no service equivalent.
- **Lego** (`github.com/go-acme/lego/v4`) — ACME client for Let's Encrypt customer-facing certificates.
- **goupnp** (`github.com/huin/goupnp`) — UPnP/NAT-PMP discovery for the remote-access wizard.

**Managed sidecar processes** (subprocesses supervised by the on-prem binary):

- **Zitadel** — too large (~400k LOC), too actively developed, internal Go API not stability-promised. Driven entirely through its stable gRPC management API. Listens on `localhost` only. Provides identity protocols (Local + OIDC + LDAP + SAML).
- **MediaMTX** — used as the streaming engine on both Recorders (camera capture + serve) and Gateways (relay + serve). Driven through config files + auth webhook. Listens on `localhost` only on the on-prem side, listens on the customer-facing port on the Gateway side.

**Cloud-side managed services**:

- **AWS RDS Postgres** — primary database for cloud control plane data
- **AWS EKS** — Kubernetes orchestration for the cloud services
- **AWS ALB + CloudFront** — load balancing + CDN for static assets
- **AWS ElastiCache for Redis** — session state, rate limiting, cache
- **AWS KMS** — key management for SSE-KMS encryption
- **AWS Secrets Manager** — credential storage
- **AWS CloudWatch + AWS X-Ray** — logs, metrics, tracing (optional, OTLP also supported)
- **Cloudflare R2** — object storage for cloud archive of recordings (chosen specifically for zero egress fees)
- **Stripe Connect** — billing platform for both direct customer billing and integrator rebill model
- **Avalara or Anrok** — sales tax / VAT compliance
- **SendGrid** — transactional email
- **Twilio** — SMS, voice, WhatsApp
- **HubSpot** — CRM and marketing automation
- **Vanta or Drata** — compliance evidence collection platform
- **Statuspage.io (Atlassian)** or **Better Stack** — customer-facing status page
- **Intercom** — customer support tooling
- **Inkeep or Mendable** — AI-powered support assistant trained on documentation
- **PostHog or Amplitude** — product analytics for onboarding funnel and feature adoption

### 4.5 Runtime architecture diagram

```
┌────────────────────────────────────────────────────────────────────┐
│                           THE CLOUD                                 │
│                      (AWS US-East-2 in v1)                          │
│                                                                     │
│  ┌──────────────────────────────────────────────────────────────┐  │
│  │  Multi-Tenant Cloud Control Plane (EKS Kubernetes)           │  │
│  │  ┌────────────────────────────────────────────────────────┐  │  │
│  │  │ Multi-tenant Identity Service (wraps Zitadel)          │  │  │
│  │  │  • Integrator hierarchy (3 levels)                     │  │  │
│  │  │  • Customer tenants (direct + integrator-managed)      │  │  │
│  │  │  • Multi-protocol auth (Local + OIDC + LDAP + SAML)    │  │  │
│  │  │  • Cross-tenant token verification                     │  │  │
│  │  └────────────────────────────────────────────────────────┘  │  │
│  │  ┌────────────────────────────────────────────────────────┐  │  │
│  │  │ Cloud Directory Service                                │  │  │
│  │  │  • Multi-tenant camera registry (per-tenant scoping)   │  │  │
│  │  │  • Permission engine (Casbin)                          │  │  │
│  │  │  • Tenant region routing (single-region in v1)         │  │  │
│  │  │  • Cross-customer search (cloud aggregator)            │  │  │
│  │  └────────────────────────────────────────────────────────┘  │  │
│  │  ┌────────────────────────────────────────────────────────┐  │  │
│  │  │ Integrator Portal Backend                              │  │  │
│  │  │  • Fleet management API                                │  │  │
│  │  │  • Customer onboarding flows                           │  │  │
│  │  │  • White-label asset management                        │  │  │
│  │  │  • Mobile app build pipeline orchestration             │  │  │
│  │  │  • Billing aggregation (Stripe Connect)                │  │  │
│  │  │  • Bulk operations engine                              │  │  │
│  │  └────────────────────────────────────────────────────────┘  │  │
│  │  ┌────────────────────────────────────────────────────────┐  │  │
│  │  │ AI/ML Service (Cloud-side)                             │  │  │
│  │  │  • Heavy model inference (face, LPR, behavioral)       │  │  │
│  │  │  • CLIP embeddings + pgvector for smart search         │  │  │
│  │  │  • Cross-camera tracking                               │  │  │
│  │  │  • Anomaly detection                                   │  │  │
│  │  │  • LLM-driven event summaries                          │  │  │
│  │  │  • Custom model registry + sandboxed execution         │  │  │
│  │  └────────────────────────────────────────────────────────┘  │  │
│  │  ┌────────────────────────────────────────────────────────┐  │  │
│  │  │ Notification Infrastructure                            │  │  │
│  │  │  • SendGrid email + Twilio SMS/voice/WhatsApp + push   │  │  │
│  │  │  • Per-user channel preferences + escalation chains    │  │  │
│  │  │  • PagerDuty / Opsgenie integration                    │  │  │
│  │  │  • ML-based alert suppression                          │  │  │
│  │  └────────────────────────────────────────────────────────┘  │  │
│  │  ┌────────────────────────────────────────────────────────┐  │  │
│  │  │ Recording Archive Service                              │  │  │
│  │  │  • Cloudflare R2 storage with tiered transitions       │  │  │
│  │  │  • Customer-controlled encryption (3 modes)            │  │  │
│  │  │  • Retention policy enforcement                        │  │  │
│  │  └────────────────────────────────────────────────────────┘  │  │
│  └──────────────────────────────────────────────────────────────┘  │
│                                                                     │
│  Marketing site (Next.js + Sanity)                                  │
│  Documentation portal (Mintlify)                                    │
│  Status page (Statuspage.io)                                        │
└────────────────────────────────────────────────────────────────────┘
                       ▲                 ▲
                       │                 │
            HTTPS      │                 │   HTTPS
            (Connect-Go)│                 │   (Flutter, React, video wall)
                       │                 │
        ┌──────────────┴────┐   ┌────────┴────────────────┐
        │ ON-PREM SITES     │   │ CLIENT DEVICES          │
        │ (per customer)    │   │ (anywhere)              │
        │                   │   │                         │
        │ ┌───────────────┐ │   │ • Flutter app           │
        │ │ Directory     │ │   │   (iOS/Android/macOS/   │
        │ │ + sidecars    │ │   │    Windows/Linux/Web)   │
        │ │ (Zitadel,     │ │   │ • React web admin       │
        │ │  MediaMTX)    │ │   │   (browser)             │
        │ │ + embedded    │ │   │ • Video wall            │
        │ │ Headscale,    │ │   │   (Qt/C++ Windows)      │
        │ │ step-ca       │ │   │                         │
        │ └───────┬───────┘ │   └─────────────────────────┘
        │         │         │
        │ ┌───────┴───────┐ │
        │ │ Recorders     │ │
        │ │ (per site)    │ │
        │ │ + cameras     │ │
        │ └───────────────┘ │
        │                   │
        │ Optional Gateway  │
        │ (co-resident with │
        │  Directory in v1) │
        └───────────────────┘
```

### 4.6 Data flow patterns

Three primary data flow patterns characterize the architecture:

**Pattern 1: LAN-direct streaming.** A client on the same LAN as the Recorder requests a stream URL from the cloud (or on-prem Directory). The URL points at the Recorder's physical LAN address. The client connects directly to the Recorder. **Video bytes never leave the LAN.** This is the default and most common path for end users at customer sites.

**Pattern 2: Off-LAN streaming via Gateway.** A client off the customer's LAN (cellular, remote office, integrator support) requests a stream URL. The cloud or on-prem Directory mints a URL pointing at the Gateway (which may be co-resident with the on-prem Directory or, for cloud-relay customers, hosted in your cloud). The Gateway terminates client TLS, validates the stream token, and proxies the stream from the Recorder via the internal tsnet mesh. **The Gateway never decodes the video; it relays opaque encrypted bytes.**

**Pattern 3: Cloud-mediated control.** Identity, permission grants, fleet management, search across customers, AI model invocation, billing — all flow through the cloud control plane. The on-prem Directory and Recorders communicate with the cloud over outbound persistent Connect-Go streams. **Air-gapped customers don't use this pattern at all** — they have a local Directory that handles all of these responsibilities locally without any cloud round-trip.

### 4.7 Air-gap vs. cloud-connected vs. hybrid customer modes

The product supports three customer deployment modes seamlessly:

| Mode                | Cloud connection     | Identity source       | Fleet management               | Recording archive   | Use case                                                                            |
| ------------------- | -------------------- | --------------------- | ------------------------------ | ------------------- | ----------------------------------------------------------------------------------- |
| **Cloud-connected** | Persistent outbound  | Cloud Zitadel         | Cloud (integrator portal)      | Cloud R2 (optional) | The default. ~80% of customers.                                                     |
| **Hybrid**          | Periodic / scheduled | Cloud Zitadel         | Cloud + local fallback         | Cloud R2 (optional) | Customers with intermittent connectivity but who still want cloud features. ~15%.   |
| **Air-gapped**      | None                 | Local Zitadel sidecar | Local on-prem React admin only | Local only          | Federal, defense, healthcare with strict data residency, sensitive industrial. ~5%. |

The customer chooses their mode at install time (or migrates between modes later). The same Go binary supports all three modes. Air-gapped customers lose the integrator portal (no integrator), the cloud archive, and cloud-side AI inference — but they retain everything else. **Recording, viewing, federation between sites, AI at the edge, white-label, and the full Flutter app experience all work without the cloud.**

---

## 5. Multi-Tenant Cloud Platform

This is the most architecturally novel part of the system relative to the original spec, and the foundation of the integrator-first GTM.

### 5.1 Tenant model

Three entity types form the cloud's tenant hierarchy:

- **Integrator** — a company that resells / installs / manages security systems for end customers. Integrators have a hierarchical organizational structure (sub-resellers, regional offices) and a relationship with one or more customer tenants.
- **Customer Tenant** — an end customer organization that owns the actual NVR system (cameras, recordings, users). A customer tenant can be **direct** (no integrator) or **integrator-managed** (related to one or more integrators).
- **User** — an authenticated human identity. A user belongs to **either** an integrator **or** a customer tenant, never both simultaneously. Integrator staff users can act on behalf of customer tenants the integrator has a relationship with, scoped by the relationship's permissions.

```
Platform (your company)
│
├── Integrators
│   ├── Integrator: National Security Corp     [root, depth 0]
│   │   │
│   │   ├── Sub-Reseller: Northeast Region     [child, depth 1]
│   │   │   ├── Sub-Reseller: NYC Office       [child, depth 2]
│   │   │   ├── Sub-Reseller: Boston Office    [child, depth 2]
│   │   │   └── Direct customer relationships at the regional level
│   │   ├── Sub-Reseller: Midwest Region       [child, depth 1]
│   │   └── HQ Staff (sees across all sub-resellers)
│   │
│   ├── Integrator: Safeguard Alarm Co         [small, no sub-resellers]
│   │   └── Customer relationships (flat)
│   │
│   └── Integrator: Backup Vendor Inc          [also has relationships on Acme]
│
├── Customer Tenants
│   ├── Customer: Acme Corp                    [managed by 3 integrators]
│   │   ├── Integrator relationships:
│   │   │   ├── ↔ NSC's Northeast Region — scope: HQ + Boston site
│   │   │   ├── ↔ Safeguard Alarm Co — scope: Brooklyn warehouse only
│   │   │   └── ↔ Backup Vendor Inc — scope: emergency-access only
│   │   ├── Sites
│   │   ├── Recorders
│   │   ├── Cameras
│   │   └── Users (Acme's own staff)
│   │
│   ├── Customer: Beta Inc                     [managed by 1 integrator]
│   │   └── ↔ Safeguard Alarm Co — scope: all
│   │
│   └── Customer: Gamma LLC                    [direct, 0 integrators]
│       └── Self-managed via cloud signup
│
└── End Users (per integrator or per customer tenant)
```

### 5.2 Schema

The core multi-tenant schema lives in the cloud's Postgres (RDS) and is per-region partitioned for the eventual multi-region architecture.

```sql
CREATE TABLE integrators (
  id                   TEXT PRIMARY KEY,                  -- UUID
  parent_integrator_id TEXT REFERENCES integrators(id),  -- NULL for root, for sub-reseller hierarchy
  display_name         TEXT NOT NULL,
  legal_name           TEXT,
  contact_email        TEXT,
  brand_config_id      TEXT REFERENCES brand_configs(id), -- white-label settings
  billing_account_id   TEXT,                              -- Stripe Connect account ID
  status               TEXT NOT NULL DEFAULT 'active',    -- active, suspended, pending_verification
  region               TEXT NOT NULL DEFAULT 'us-east-2', -- always us-east-2 in v1
  created_at           TIMESTAMPTZ DEFAULT NOW(),
  updated_at           TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX idx_integrator_parent ON integrators(parent_integrator_id);
CREATE INDEX idx_integrator_region ON integrators(region);

CREATE TABLE customer_tenants (
  id                    TEXT PRIMARY KEY,                 -- UUID
  display_name          TEXT NOT NULL,
  is_direct             BOOLEAN NOT NULL,                 -- true = no integrator relationships
  signup_source         TEXT,                             -- 'direct_marketing', 'integrator_invite', 'self_service', 'sales_led'
  brand_override_id     TEXT REFERENCES brand_configs(id),-- if customer has their own brand
  billing_account_id    TEXT,                             -- Stripe Connect account ID (for direct billing)
  region                TEXT NOT NULL DEFAULT 'us-east-2',
  status                TEXT NOT NULL DEFAULT 'active',
  created_at            TIMESTAMPTZ DEFAULT NOW(),
  updated_at            TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX idx_customer_region ON customer_tenants(region);
CREATE INDEX idx_customer_status ON customer_tenants(status);

CREATE TABLE customer_integrator_relationships (
  customer_id          TEXT NOT NULL REFERENCES customer_tenants(id),
  integrator_id        TEXT NOT NULL REFERENCES integrators(id),
  scope                JSONB NOT NULL,                    -- which sites/cameras this integrator can touch
  role_template        TEXT NOT NULL,                     -- 'full_management', 'monitoring_only', 'emergency_access', 'custom'
  custom_permissions   JSONB,                             -- if role_template = 'custom'
  granted_at           TIMESTAMPTZ NOT NULL,
  granted_by_user_id   TEXT REFERENCES users(id),         -- which customer admin granted this
  status               TEXT NOT NULL DEFAULT 'pending_acceptance', -- pending_acceptance, active, suspended, revoked
  PRIMARY KEY (customer_id, integrator_id)
);
CREATE INDEX idx_cir_integrator ON customer_integrator_relationships(integrator_id);
CREATE INDEX idx_cir_customer ON customer_integrator_relationships(customer_id);
CREATE INDEX idx_cir_status ON customer_integrator_relationships(status);

CREATE TABLE users (
  id                 TEXT PRIMARY KEY,                   -- UUID
  email              TEXT NOT NULL,
  display_name       TEXT,
  integrator_id      TEXT REFERENCES integrators(id),    -- if integrator staff
  customer_tenant_id TEXT REFERENCES customer_tenants(id),-- if customer staff
  zitadel_user_id    TEXT NOT NULL,                      -- ID in Zitadel
  region             TEXT NOT NULL DEFAULT 'us-east-2',
  status             TEXT NOT NULL DEFAULT 'active',
  created_at         TIMESTAMPTZ DEFAULT NOW(),
  CHECK (
    (integrator_id IS NOT NULL AND customer_tenant_id IS NULL) OR
    (integrator_id IS NULL AND customer_tenant_id IS NOT NULL)
  )
);
CREATE UNIQUE INDEX idx_users_email_unique ON users(email);
CREATE INDEX idx_users_integrator ON users(integrator_id) WHERE integrator_id IS NOT NULL;
CREATE INDEX idx_users_customer ON users(customer_tenant_id) WHERE customer_tenant_id IS NOT NULL;

CREATE TABLE brand_configs (
  id                TEXT PRIMARY KEY,                    -- UUID
  owner_integrator_id TEXT REFERENCES integrators(id),  -- NULL if customer-owned
  owner_customer_id TEXT REFERENCES customer_tenants(id),
  display_name      TEXT NOT NULL,
  primary_color     TEXT NOT NULL,                       -- hex
  secondary_color   TEXT NOT NULL,
  logo_url          TEXT,                                -- S3/R2 URL
  favicon_url       TEXT,
  font_family       TEXT,
  custom_domain     TEXT,                                -- e.g., security.acmealarm.com
  custom_email_domain TEXT,                              -- e.g., alerts@acmealarm.com
  email_template_overrides JSONB,
  legal_terms_url   TEXT,
  privacy_policy_url TEXT,
  support_email     TEXT,
  support_url       TEXT,
  mobile_bundle_id  TEXT,                                -- for per-integrator mobile builds
  mobile_app_name   TEXT,
  created_at        TIMESTAMPTZ DEFAULT NOW(),
  updated_at        TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE on_prem_directories (
  id                  TEXT PRIMARY KEY,                  -- UUID
  customer_tenant_id  TEXT NOT NULL REFERENCES customer_tenants(id),
  display_name        TEXT NOT NULL,
  site_label          TEXT NOT NULL,                     -- human-readable site name
  deployment_mode     TEXT NOT NULL,                     -- 'cloud_connected', 'hybrid', 'air_gapped'
  status              TEXT NOT NULL,                     -- 'online', 'degraded', 'offline'
  last_seen_at        TIMESTAMPTZ,
  software_version    TEXT,
  capabilities        JSONB,                             -- supported features
  created_at          TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX idx_directories_customer ON on_prem_directories(customer_tenant_id);
CREATE INDEX idx_directories_status ON on_prem_directories(status);

-- ... additional tables for permissions (Casbin), audit log, billing events, etc.
```

Every row in every table carries a `region` column (always `us-east-2` in v1) so that the multi-region future is a non-breaking expansion. Every primary key is a UUID so there are no ID collisions across regions.

### 5.3 Multi-region readiness

The cloud is built for multi-region active-active deployment but ships in a single region (US-East-2) in v1. Six discipline items make the future expansion cheap:

1. **Tenant-to-region pinning** — every tenant has a `home_region` field. v1 always returns `us-east-2`. The lookup exists in code so adding region #2 doesn't require rewriting routing.
2. **Region-scoped URLs** — API endpoints are per-region from day one (`https://us-east-2.api.yourbrand.com/v1/...`). The marketing site redirects logged-in users to their tenant's region's URL. v1 has only one region but the URL structure assumes regional routing.
3. **Per-region Terraform modules** — infrastructure is described as code with `infra/modules/region/` parameterized by region name. v1 has only `environments/production/regions/us-east-2/` active; other regions are commented-out and ready to uncomment.
4. **Globally-unique IDs** — UUIDs everywhere, no auto-incrementing integer keys.
5. **No cross-region API operations in v1** — every API call is satisfied by data in one region. Cross-region operations are designed as background jobs from day one.
6. **Globally-distributed identity layer** — separated logically from regional tenant data. v1 is single-instance Zitadel; v1.x can migrate to a globally-distributed identity service (CockroachDB-backed or similar) without changing the rest of the system.

### 5.4 Cloud platform tech stack

| Component                | Technology                                                                                                                                          |
| ------------------------ | --------------------------------------------------------------------------------------------------------------------------------------------------- |
| Container orchestration  | AWS EKS (managed Kubernetes)                                                                                                                        |
| Primary database         | AWS RDS Postgres (Aurora optional) with `pgvector` for AI embeddings                                                                                |
| Cache + session store    | AWS ElastiCache for Redis                                                                                                                           |
| Object storage           | **Cloudflare R2** (for cloud archive — chosen specifically for zero egress fees) + AWS S3 (for non-recording assets like logos, exports, snapshots) |
| Load balancing           | AWS ALB (regional) + CloudFront (CDN for static assets)                                                                                             |
| Identity sidecar         | Zitadel (multi-tenant instance, regional)                                                                                                           |
| Streaming engine sidecar | MediaMTX (one instance per Gateway region)                                                                                                          |
| Service mesh             | Istio (for inter-service mTLS, traffic management, observability)                                                                                   |
| API protocol             | Connect-Go (gRPC + HTTP/JSON over the same handlers)                                                                                                |
| Background jobs          | River (Go-native, Postgres-backed) for tenant provisioning, billing batches, cloud archive transitions                                              |
| Event bus                | NATS (for cross-service event distribution)                                                                                                         |
| Search infrastructure    | OpenSearch for log/event search; pgvector for AI semantic search in v1, migrate to Qdrant in v1.x at scale                                          |
| Observability            | Prometheus + Grafana + OpenTelemetry; OTLP exporter with optional customer-collector destinations                                                   |
| Logging aggregation      | Vector → Loki, with optional customer-side OTLP forwarding                                                                                          |
| CI/CD                    | GitHub Actions for code; ArgoCD for Kubernetes deployments                                                                                          |
| Infrastructure as Code   | Terraform with per-region modules                                                                                                                   |
| Secrets management       | AWS Secrets Manager + sealed-secrets in Kubernetes                                                                                                  |
| Cluster autoscaling      | Karpenter (better than Cluster Autoscaler for diverse workloads)                                                                                    |
| Cost monitoring          | OpenCost + AWS Cost Explorer                                                                                                                        |
| Security scanning        | Snyk, Trivy, Falco, gVisor for sandboxing custom AI models                                                                                          |

---

## 6. Identity and Authentication

### 6.1 The IdentityProvider interface

Every package in the system that needs identity goes through a single Go interface, `IdentityProvider`. Nothing reaches around it. The interface is the firewall that makes "switch from Zitadel to Keycloak/Authentik/Ory later" a 3-week adapter rewrite instead of a system-wide change.

```go
// internal/shared/auth/provider.go
type IdentityProvider interface {
    // Authentication
    AuthenticateLocal(ctx, tenant TenantRef, username, password string) (*Session, error)
    BeginSSOFlow(ctx, tenant TenantRef, providerID, returnURL string) (authURL, state string, err error)
    CompleteSSOFlow(ctx, state string, callbackParams url.Values) (*Session, error)
    RefreshSession(ctx, refreshToken string) (*Session, error)
    Logout(ctx, sessionID string) error

    // User management (multi-tenant aware)
    CreateLocalUser(ctx, tenant TenantRef, spec UserSpec) (UserID, error)
    GetUser(ctx, id UserID) (*User, error)
    ListUsers(ctx, tenant TenantRef, opts ListOptions) ([]*User, error)
    UpdateUser(ctx, id UserID, update UserUpdate) error
    DeleteUser(ctx, id UserID) error
    AssignUserToGroups(ctx, id UserID, groups []GroupID) error

    // Group management (per-tenant)
    CreateGroup(ctx, tenant TenantRef, name, description string) (GroupID, error)
    ListGroups(ctx, tenant TenantRef) ([]*Group, error)
    DeleteGroup(ctx, id GroupID) error

    // IdP configuration (per-tenant)
    AddOIDCProvider(ctx, tenant TenantRef, config OIDCConfig) (ProviderID, error)
    AddSAMLProvider(ctx, tenant TenantRef, config SAMLConfig) (ProviderID, error)
    AddLDAPProvider(ctx, tenant TenantRef, config LDAPConfig) (ProviderID, error)
    UpdateProvider(ctx, id ProviderID, update ProviderUpdate) error
    RemoveProvider(ctx, id ProviderID) error
    ListProviders(ctx, tenant TenantRef) ([]*ProviderConfig, error)
    TestProvider(ctx, id ProviderID) (*ProviderTestResult, error)

    // Token verification
    JWKS(ctx, tenant TenantRef) (jwks.Set, error)
    VerifyToken(ctx, rawToken string) (*Claims, error)
}

type TenantRef struct {
    Type     TenantType  // INTEGRATOR or CUSTOMER
    ID       string
}

type Claims struct {
    UserID       UserID
    TenantRef    TenantRef
    Groups       []GroupID
    IssuedAt     time.Time
    ExpiresAt    time.Time
    SiteScope    []RecorderID  // which Recorders this token is allowed against
    IntegratorRelationships []IntegratorRelationshipRef // for cross-tenant access via integrator
}
```

`TestProvider` is a first-class operation, not an afterthought. Every IdP wizard ends with a "Test" button that performs a real authentication round-trip and only allows save on success.

`Claims` includes `TenantRef` (multi-tenant aware), `SiteScope` (defense in depth), and `IntegratorRelationships` (so a token issued to integrator staff carries the customer relationships they're allowed to act on behalf of).

### 6.2 The Zitadel multi-tenant adapter

The Zitadel adapter implements `IdentityProvider` against Zitadel's gRPC management API, with multi-tenant scoping mapped to Zitadel's organization hierarchy.

**Mapping**:

- Each Integrator → one Zitadel **organization**
- Each Customer Tenant → one Zitadel **organization** (sibling to integrator orgs)
- Each User → one Zitadel **user** in the appropriate organization
- Each integrator-customer relationship → metadata on the user record + a Casbin grant on the customer side

**Bootstrap** on first cloud start: provisions the platform-level Zitadel instance, creates the platform admin org, creates the service account the cloud uses for management API calls, configures default OIDC clients for the React admin and Flutter app.

**Per-tenant onboarding** (when a new integrator or customer tenant signs up): the cloud's tenant provisioning service calls the Zitadel adapter, which creates a new Zitadel org, generates initial admin credentials, and returns a tenant ID that the rest of the cloud uses.

### 6.3 Multi-protocol authentication

Customers and integrators can configure any combination of these auth methods per tenant:

| Method                      | Implementation                                                                                                 |
| --------------------------- | -------------------------------------------------------------------------------------------------------------- |
| **Local**                   | Username + password stored in Zitadel, hashed with Argon2id, brute-force protected via rate limiting + lockout |
| **OIDC**                    | Microsoft Entra ID, Google Workspace, Okta, Generic OIDC (Keycloak / Authentik / Auth0 / Authelia / etc.)      |
| **SAML 2.0**                | Any SAML 2.0 IdP via Zitadel's SAML support                                                                    |
| **LDAP / Active Directory** | Direct LDAP bind via Zitadel's LDAP federation                                                                 |

Six per-IdP wizards in the React admin (Microsoft Entra, Google Workspace, Okta, Generic OIDC, LDAP, SAML) walk the customer through provider setup with screenshots, test buttons, and actionable error messages. Customers never see Zitadel's UI or error codes.

### 6.4 Token format and verification

| Token type                 | Signing                                            | TTL                | Storage                                                                      |
| -------------------------- | -------------------------------------------------- | ------------------ | ---------------------------------------------------------------------------- |
| **Access token (session)** | RS256 JWT signed by Zitadel                        | ~1 hour            | Client-side secure storage (Keychain / Keystore / encrypted browser storage) |
| **Refresh token**          | Opaque, server-side                                | ~30 days, rotating | Server-side in Zitadel; client stores the opaque value                       |
| **Stream token**           | RS256 JWT signed by the cloud's stream signing key | ~5 minutes         | Never stored, minted on demand per stream request                            |

**Token verification** is a shared package (`internal/shared/auth/verifier`) used identically by:

- Cloud control plane (verifies tokens on every API request)
- On-prem Directory (verifies tokens for local API requests)
- On-prem Recorder (verifies stream tokens for stream requests)
- Gateway (verifies stream tokens for relayed stream requests)

The Recorder/Gateway never call back to the cloud for verification — they cache the JWKS for ~5 minutes and verify locally. This is what lets Recorders authenticate streaming clients even when the cloud is briefly unreachable.

A **single-use nonce bloom filter** in the Recorder and Gateway prevents stream token reuse. Every stream token has a unique nonce; redeeming it once marks the nonce in the filter and rejects future redemptions. False-positive rate is tuned to <0.1% at 1M concurrent nonces, which is acceptable because false positives just cause the client to request a fresh token.

### 6.5 Cross-tenant authentication for integrator staff

When an integrator staff user accesses a customer's resources, the request flows:

1. Integrator staff user logs into the integrator portal at `command.yourbrand.com`
2. They navigate to a customer view ("Acme Corp")
3. The cloud's cross-tenant access service checks: does this integrator have a relationship with Acme? If yes, what's the scope? What permissions does the role template grant?
4. The cloud issues a **scoped token** that has the integrator user's identity + the customer tenant ID + the relationship's permissions encoded in the `SiteScope` and `IntegratorRelationships` claims
5. All subsequent requests use this scoped token; the cloud's permission middleware enforces the relationship's scope on every API call
6. Audit log entries record: integrator user ID + integrator ID + customer tenant ID + relationship ID + action + result

Customer admins can revoke an integrator's relationship at any time. Revocation immediately invalidates all active scoped tokens for that relationship via a revocation list pushed to the on-prem Recorders and Gateway.

---

## 7. Permission Model (Casbin)

### 7.1 Canonical actions

Fixed by code, not customer-extendable:

```go
const (
    ActionViewLive          Action = "view.live"
    ActionViewPlayback      Action = "view.playback"
    ActionViewThumbnails    Action = "view.thumbnails"
    ActionExportClip        Action = "export.clip"
    ActionPTZControl        Action = "ptz.control"
    ActionAudioListen       Action = "audio.listen"
    ActionAudioTalkback     Action = "audio.talkback"
    ActionConfigRead        Action = "config.read"
    ActionConfigWrite       Action = "config.write"
    ActionPermissionsGrant  Action = "permissions.grant"
    ActionAIInferenceRun    Action = "ai.inference"
    ActionCustomModelDeploy Action = "ai.model.deploy"
    ActionRecordingsExport  Action = "recordings.export"
    ActionAuditLogRead      Action = "audit.read"
    ActionBillingManage     Action = "billing.manage"
    ActionIntegrationsConfigure Action = "integrations.configure"
    ActionFederationManage  Action = "federation.manage"
)
```

### 7.2 Default roles

Customer admins can define custom roles, but five built-ins ship with the product:

| Role           | Permissions                                                                         |
| -------------- | ----------------------------------------------------------------------------------- |
| `viewer`       | view.live, view.playback, view.thumbnails                                           |
| `operator`     | viewer + export.clip, ptz.control, audio.listen, audio.talkback                     |
| `investigator` | operator + recordings.export, audit.read, ai.inference                              |
| `admin`        | investigator + config.read, config.write, permissions.grant, integrations.configure |
| `owner`        | admin + billing.manage, federation.manage, ai.model.deploy                          |

### 7.3 Resource patterns

Casbin policy entries support:

- `camera:id:abc-123` — exact camera by ID
- `camera:tag:warehouse` — all cameras with the given tag
- `camera:recorder:wh-01` — all cameras assigned to the given Recorder
- `camera:site:headquarters` — all cameras at a site
- `camera:*` — all cameras in the tenant
- `recorder:id:xyz` — specific Recorder
- `tenant:*` — entire tenant scope (for tenant-wide actions)

### 7.4 Cross-tenant subjects

Cross-tenant grants use a `federation:` or `integrator:` prefix:

```
# Local grant: customer admin grants their own staff
p, group:warehouse_ops, view.live, camera:tag:warehouse

# Integrator grant: customer admin grants an integrator's staff
p, integrator:safeguard:operators, view.live, camera:tag:warehouse
p, integrator:safeguard:emergency, view.playback, camera:*

# Cross-Directory federation grant (for federated multi-site customers)
p, federation:peer_directory_id:executives, view.live, camera:tag:shared
```

### 7.5 Sub-reseller permission inheritance

Default behavior: **inherit-with-narrowing**. When a parent integrator (NSC HQ) has a relationship with a customer, sub-resellers (Northeast Region) inherit that relationship by default. The parent can narrow the scope when delegating: "the Northeast Region office gets only `view.live` + `view.playback` on Acme's HQ site, even though NSC HQ has full management."

Implementation: a `delegated_scopes` table tracks parent-to-child scope narrowing. Permission resolution walks up the integrator hierarchy, intersecting scopes at each level.

### 7.6 Enforcement at four layers

**Permissions are checked at every layer where they could be bypassed**, not just the cloud API boundary:

1. **Cloud API middleware** — every request that operates on a camera or admin resource
2. **Stream URL minting** — token contains the scoped permission, signed
3. **Gateway** — re-verifies the signed token and re-checks scope before opening upstream stream
4. **Recorder** — independently verifies the token and checks scope before serving any video bytes

A user (or compromised integrator) can never acquire video for a camera they don't have permission on, even by guessing IDs, even by reaching the Recorder directly on the LAN, even by replaying tokens within the TTL.

### 7.7 Force revocation

Token verification on Recorders is local for performance. The trade-off is that revoking permission has up to ~5 minutes of latency in normal cases. For emergency revocation, an explicit admin action pushes a revocation list to all Recorders via the existing Connect-Go control plane stream within ~5 seconds.

---

## 8. Multi-Tenant Cloud Directory and Camera Registry

### 8.1 The cloud Directory service

The cloud Directory service is the cloud-side equivalent of the on-prem Directory subsystem. It owns the multi-tenant camera registry, permission state, and cross-tenant routing for cloud-managed customers.

For **cloud-connected customers**, the cloud Directory is the authoritative source of truth. The on-prem Recorders synchronize their assigned cameras from the cloud Directory and report state back. When the cloud is briefly unreachable, the on-prem Recorders continue with cached assignments.

For **air-gapped customers**, an **on-prem Directory subsystem** in the customer's `directory` or `all-in-one` mode binary serves the same role locally. The on-prem Directory has identical APIs and behavior to the cloud Directory, just without the multi-tenant scoping. The same Flutter app and React admin work against either.

For **hybrid customers**, the on-prem Directory acts as a cache/proxy that synchronizes with the cloud and continues to operate locally during cloud unavailability.

This dual-mode architecture is what allows the same product to serve cloud-native, air-gapped, and hybrid customers without separate codebases. The trick is that the API surface is identical in both modes — the difference is just where the data is stored and which mode the on-prem Recorder is configured to talk to.

### 8.2 Camera registry data model

Every camera record in the cloud Directory has a `tenant_id` (customer tenant) and `directory_id` (which on-prem Directory it lives under). The schema:

```sql
CREATE TABLE cameras (
  id                  TEXT PRIMARY KEY,                  -- UUID
  tenant_id           TEXT NOT NULL REFERENCES customer_tenants(id),
  directory_id        TEXT NOT NULL REFERENCES on_prem_directories(id),
  display_name        TEXT NOT NULL,
  location_label      TEXT,
  tags                JSONB DEFAULT '[]',
  assigned_recorder_id TEXT NOT NULL,                    -- which Recorder owns this camera
  rtsp_url            TEXT NOT NULL,
  rtsp_credentials_encrypted BYTEA,                      -- column-level encryption
  codec_hint          TEXT,
  schedule_id         TEXT REFERENCES recording_schedules(id),
  retention_id        TEXT REFERENCES retention_policies(id),
  ai_pipeline_id      TEXT REFERENCES ai_pipelines(id),
  custom_model_ids    TEXT[],                            -- customer-uploaded models attached to this camera
  current_status      TEXT,                              -- updated by Recorder state push
  current_bitrate_kbps INTEGER,
  last_frame_at       TIMESTAMPTZ,
  state_updated_at    TIMESTAMPTZ,
  config_version      INTEGER NOT NULL DEFAULT 1,        -- monotonic, bumped on every change
  created_at          TIMESTAMPTZ DEFAULT NOW(),
  updated_at          TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX idx_cameras_tenant ON cameras(tenant_id);
CREATE INDEX idx_cameras_directory ON cameras(directory_id);
CREATE INDEX idx_cameras_recorder ON cameras(assigned_recorder_id);

CREATE TABLE recording_schedules (
  id            TEXT PRIMARY KEY,
  tenant_id     TEXT NOT NULL REFERENCES customer_tenants(id),
  display_name  TEXT NOT NULL,
  schedule_type TEXT NOT NULL,                           -- 'continuous', 'motion', 'event_driven', 'time_window'
  config        JSONB NOT NULL
);

CREATE TABLE retention_policies (
  id            TEXT PRIMARY KEY,
  tenant_id     TEXT NOT NULL REFERENCES customer_tenants(id),
  display_name  TEXT NOT NULL,
  local_days    INTEGER NOT NULL,                        -- how many days kept on the Recorder
  cloud_archive_enabled BOOLEAN DEFAULT FALSE,
  cloud_archive_total_days INTEGER,
  cloud_tier_transitions JSONB                           -- when to move to Hot/Warm/Cold/Archive
);

CREATE TABLE camera_segment_index (
  camera_id    TEXT NOT NULL REFERENCES cameras(id),
  recorder_id  TEXT NOT NULL,
  start_time   TIMESTAMPTZ NOT NULL,
  end_time     TIMESTAMPTZ NOT NULL,
  size_bytes   BIGINT,
  segment_count INTEGER,
  storage_location TEXT NOT NULL,                       -- 'local', 'cloud_hot', 'cloud_warm', 'cloud_cold', 'cloud_archive'
  PRIMARY KEY (camera_id, start_time)
);
CREATE INDEX idx_segment_camera_time ON camera_segment_index(camera_id, start_time, end_time);
```

### 8.3 Recorder synchronization

Recorders receive their assignments via long-lived Connect-Go server-streaming RPC from the cloud Directory:

```protobuf
service RecorderControl {
  rpc StreamAssignments(StreamAssignmentsRequest) returns (stream AssignmentEvent);
}

message AssignmentEvent {
  oneof event {
    Snapshot snapshot = 1;     // sent on (re)connect
    CameraAdded   added   = 2;
    CameraUpdated updated = 3;
    CameraRemoved removed = 4;
    RevocationListUpdate revocations = 5;
  }
}
```

Recorders maintain a local SQLite cache (`assigned_cameras.db`) that's the source of truth for runtime behavior. On Directory unreachability, the Recorder reads from the cache and continues recording.

Recorders push state back via:

```protobuf
service DirectoryIngest {
  rpc StreamCameraState(stream CameraStateUpdate) returns (StreamCameraStateAck);
  rpc PublishSegmentIndex(stream SegmentIndexBatch) returns (SegmentIndexAck);
  rpc PublishAIEvents(stream AIEventBatch) returns (AIEventAck);
}
```

Three streams in the upward direction:

1. **State**: per-camera connection status, bitrate, fps, last frame, errors. Debounced (one update on change + heartbeat every ~10s).
2. **Segment index**: batches of newly-recorded segment metadata every ~30s. Idempotent on `(camera_id, start_time)`.
3. **AI events**: batches of AI detection events as they happen. Used by the cloud's event store, search index, and notification system.

### 8.4 Recorder offline behavior — the central invariant

| Operation                                              | Works without cloud / Directory?                    |
| ------------------------------------------------------ | --------------------------------------------------- |
| Capture from assigned cameras                          | ✓ Indefinitely (cached assignments)                 |
| Record to local disk                                   | ✓ Indefinitely                                      |
| Run AI inference at the edge                           | ✓ Indefinitely (models cached locally)              |
| Serve LAN-direct playback to clients with valid tokens | ✓ Until JWKS cache expires (~1 day max)             |
| Accept new client connections                          | ✓ With valid token                                  |
| Receive new camera assignments                         | ✗ Queued in cloud, applied on reconnect             |
| Receive permission updates                             | ✗ Cached; ~5 min revocation latency in normal cases |
| New users log in                                       | ✗ Requires cloud (or local Zitadel for air-gapped)  |
| Cloud archive of recordings                            | ✗ Suspended; local recording continues normally     |

**The invariant is non-negotiable: recording never stops as long as the Recorder has power and disk.** Every other failure is recoverable; capture and storage are protected by every mechanism.

---

## 9. Streaming Data Plane

### 9.1 Stream URL minting

The cloud Directory (or on-prem Directory in air-gapped mode) mints short-lived, signed stream URLs on demand. Every video stream the Flutter client, React admin, or video wall client consumes is fetched via one of these URLs.

```
POST /api/v1/streams/request
Authorization: Bearer <session_token>

{
  "camera_id": "cam-abc-123",
  "kind": "live" | "playback" | "snapshot" | "audio_talkback",
  "protocol": "webrtc" | "ll-hls" | "hls" | "mjpeg" | "rtsp-tls",
  "playback_range": { "start": "...", "end": "..." }
}

→ 200 OK
{
  "stream_id": "...",
  "ttl_seconds": 300,
  "endpoints": [
    { "kind": "lan_direct",         "url": "https://192.168.1.100:8443/streams/...", "token": "...", "estimated_latency_ms": 5 },
    { "kind": "self_hosted_public", "url": "https://acme.nvr-direct.example/streams/...", "token": "...", "estimated_latency_ms": 40 },
    { "kind": "managed_relay",      "url": "https://acme.relay.yourbrand.com/streams/...", "token": "...", "estimated_latency_ms": 80 }
  ]
}
```

The client tries endpoints in order with a 2-second timeout per attempt. Failover is invisible to the user.

### 9.2 Routing decision

The Directory picks which endpoints to include and in what order based on:

- Client source IP vs Recorder advertised LAN subnets (LAN-direct match)
- Whether the customer has Tier 2 (self-hosted public) configured
- Whether the customer has Tier 3 (managed cloud relay) configured

LAN-direct is always tried first when applicable because it has the best latency and zero bandwidth cost.

### 9.3 Protocol matrix

| Protocol                | Used for                | When                                               |
| ----------------------- | ----------------------- | -------------------------------------------------- |
| **WebRTC**              | Live, low-latency       | Default for live view in Flutter and admin console |
| **LL-HLS**              | Live, fallback          | When WebRTC negotiation fails                      |
| **HLS**                 | Playback / scrubbing    | Default for recorded video                         |
| **MJPEG**               | Snapshots / thumbnails  | Single-frame requests, grid views                  |
| **RTSP-over-TLS**       | Power user / video wall | Optional, permission-gated                         |
| **WebRTC data channel** | Audio talkback          | Same peer connection as live view                  |

### 9.4 MediaMTX as the streaming engine

Recorders and Gateways both use MediaMTX as the streaming engine. The integration is:

1. **External authentication via webhook** — MediaMTX calls a local Go HTTP endpoint (`http://127.0.0.1:9998/mediamtx/auth`) on every connection attempt. The Go handler verifies the stream token, checks scope, checks the nonce filter, and returns 200/403.
2. **Dynamic path config generation** — the Recorder's Go code generates `paths.yml` from the local `assigned_cameras` cache and reloads MediaMTX whenever the cache changes. MediaMTX hot-reloads without disturbing in-progress streams.
3. **Sidecar lifecycle management** — the Go binary supervises the MediaMTX subprocess (start, health-check, restart on crash, clean shutdown).

The Gateway uses the same MediaMTX integration but with paths whose `source:` points at Recorders (over the tsnet mesh) instead of at cameras. The Gateway's MediaMTX is configured with `record: no` because it's a relay, not a recorder.

### 9.5 Multi-Recorder timeline assembly

When a camera has been reassigned across Recorders (or recordings live on multiple Recorders due to load balancing), the Directory's playback API reconstructs one continuous timeline:

```
GET /api/v1/cameras/{id}/timeline?from=<ts>&to=<ts>

→ 200 OK
{
  "camera_id": "cam-abc-123",
  "ranges": [
    { "start": "...", "end": "...", "recorder_id": "HQ-Recorder-01", "storage_location": "local" },
    { "start": "...", "end": "...", "recorder_id": "WH-Recorder", "storage_location": "cloud_warm" }
  ],
  "events": [ ... AI events overlaid ... ]
}
```

The client receives a unified timeline; when scrubbing across boundaries it requests fresh stream URLs that point at whichever Recorder (or cloud archive) has that range. ~200ms transition pause across boundaries, but visually one continuous stream.

### 9.6 Audio talkback

Two-way audio uses the same WebRTC peer connection as live view, with an additional outbound audio track. The flow:

1. Flutter client requests `kind: live` with `audio_talkback: true`
2. Cloud Directory checks both `view.live` and `audio.talkback` permissions
3. Stream token includes both kinds in the bitfield
4. Recorder's MediaMTX is configured with the camera's ONVIF backchannel endpoint as a sink
5. Audio bytes from the Flutter client flow through the peer connection to the Recorder, then to the camera's speaker

Echo cancellation is handled by the platform-native WebRTC stack on each client (works on iOS, Android, macOS, Windows, Linux, browser).

### 9.7 Bandwidth management

| Control                       | Where                         | Purpose                                                                      |
| ----------------------------- | ----------------------------- | ---------------------------------------------------------------------------- |
| `max_streams_per_user`        | Cloud Directory token minting | Prevents one user from saturating                                            |
| `max_streams_per_camera`      | Recorder                      | Caps concurrent viewers per camera                                           |
| `max_gateway_throughput_mbps` | Gateway                       | Bandwidth ceiling on the Gateway                                             |
| `prefer_substream_off_lan`    | Cloud Directory routing       | When client is off-LAN, mint URL for ONVIF sub-stream instead of main stream |
| `auto_quality`                | Flutter client                | Client measures throughput and downgrades                                    |

For the Tier 3 cloud relay, **bandwidth metering per (customer, camera, hour)** is captured for the billing system to charge overage if applicable.

---

## 10. Recording Storage Architecture

### 10.1 Two storage tiers: local primary + cloud archive

**Local primary**: every Recorder writes recordings to local disk. This is the always-available, always-fast tier. Recordings stay local for the customer's configured retention period (e.g., 30 days default).

**Cloud archive (optional)**: customers can enable cloud archive to copy completed segments to Cloudflare R2 for longer retention. The cloud archive is the system of record for old recordings; the local storage continues serving recent recordings for fast playback.

### 10.2 Four cloud archive tiers

Cloud archive uses tiered storage for cost efficiency. Tier transitions happen automatically based on age.

| Tier        | Storage class                                   | Use case                       | Retrieval                                    | Cost    |
| ----------- | ----------------------------------------------- | ------------------------------ | -------------------------------------------- | ------- |
| **Hot**     | Cloudflare R2 standard                          | First ~30 days                 | Instant                                      | Highest |
| **Warm**    | Cloudflare R2 (with infrequent access patterns) | 30-90 days                     | Instant, slightly more expensive per request | Medium  |
| **Cold**    | Cloudflare R2 with cold storage tier            | 90 days - 1 year               | Instant, much lower storage cost             | Low     |
| **Archive** | Backblaze B2 cold archive (chosen for cost)     | 1+ years, compliance retention | 12-48 hour retrieval delay, lowest cost      | Lowest  |

Customers configure retention policies per camera (per camera, not per Recorder — the same Recorder can have a 7-year-retention cash register camera and a 30-day-retention parking lot camera). The system automatically transitions segments between tiers as they age.

### 10.3 Three encryption modes

| Mode                | Key holder                                                | Server can decrypt?                     | Use case                                                                |
| ------------------- | --------------------------------------------------------- | --------------------------------------- | ----------------------------------------------------------------------- |
| **Standard**        | Cloudflare R2 platform                                    | Yes (server-side)                       | Default for most customers, simplest                                    |
| **SSE-KMS**         | Customer's AWS KMS keys                                   | Server can decrypt only with KMS access | HIPAA, SOC 2 — customer can audit and revoke key access                 |
| **Client-side CMK** | Customer's master key (held only on the on-prem Recorder) | **No, ever**                            | Federal, defense, ultra-sensitive — even your cloud team cannot decrypt |

**Client-side encryption with customer-managed keys** (CSE-CMK) is the strongest option. Recordings are encrypted at the Recorder before being uploaded. The cloud stores ciphertext only. Playback works because the Flutter client downloads encrypted segments and decrypts them locally using the customer's key.

The trade-off with CSE-CMK: if the customer loses their master key, **recordings are unrecoverable forever, even by you.** This is communicated clearly during setup, with required key backup confirmation before enabling.

### 10.4 Bandwidth handling

Cloud archive uploads are real bandwidth. Several controls minimize the cost:

| Control                         | What it does                                                                                               |
| ------------------------------- | ---------------------------------------------------------------------------------------------------------- |
| **Upload throttling**           | Per-camera and per-Recorder upload rate caps. Default: don't saturate the customer's connection.           |
| **Scheduled uploads**           | Optionally only upload during off-hours (configurable per Recorder)                                        |
| **Sub-stream archive**          | Upload only the lower-bitrate ONVIF sub-stream instead of the main stream — ~80% bandwidth/storage savings |
| **AI-driven selective archive** | Only upload segments where motion / objects / people / faces / vehicles were detected. Skip empty hours.   |
| **Pause/resume**                | Customer can pause uploads (e.g., during high-traffic business hours) and resume later                     |

For playback from cloud archive, **Cloudflare R2 has zero egress fees**, which means customers can play back as much archived video as they want without surprise bandwidth bills. This is a major differentiator vs S3-based cloud archive offerings.

### 10.5 Segment index searchability independent of storage tier

Even when recordings are in cold or archive tiers, **the AI metadata stays in the searchable hot index**. Customers can search "show me people in red shirts on March 15th" and get hits from cold-tier recordings; the actual playback may have a 24-hour retrieval delay (for archive tier) but the search itself is always instant.

This means the cloud's searchable metadata grows linearly with retention duration, even when raw video is moved to cold storage. Worth budgeting for in the database sizing.

---

## 11. AI/ML Platform — 11 Feature Categories

The AI platform is one of the two biggest competitive differentiators (alongside white-label / multi-tenancy). The v1 scope is **maximum**: 11 feature categories covering object detection, identity, behavior, audio, search, and customer extensibility.

### 11.1 The 11 features

**Detection features:**

1. **Object detection** — generic (people, vehicles, animals, packages, weapons) and per-vertical (retail loss prevention, parking management, healthcare). YOLO v8/v9-based, with continuous model updates from your ML team.
2. **Face recognition** — identify known faces from a customer-managed face vault, alert on unknowns, watchlist matching. Privacy-controlled (opt-in per camera, GDPR-compliant face vault with right-to-erasure, encrypted vault with customer-controlled keys, audit log of every match). EU AI Act high-risk system — full documentation, conformity assessment, CE marking.
3. **License plate recognition (LPR / ANPR)** — read plates with regional format support (US states, EU countries, UK, AU), match against allow/deny watchlists, alert on flagged plates, log all reads with timestamp + camera + plate for forensic search.
4. **Behavioral analytics** — loitering detection, line crossing (count entries/exits), region-of-interest entry/exit, crowd density estimation, tailgating detection, fall detection (elderly care, healthcare). State-machine layer on top of object detection.
5. **Audio analytics** — gunshot detection, glass breaking, raised voices, vehicle horn / siren detection. Specialized models trained on labeled audio datasets (you'll license a commercial dataset like AudioSet).

**Search features:**

6. **Smart search via CLIP embeddings** — natural language queries against recorded video. "Show me people in red shirts at the loading dock yesterday afternoon." Implementation: per-frame CLIP embeddings computed at the edge as recordings happen, embeddings shipped to the cloud's pgvector index, search queries embedded server-side and matched via vector similarity. **Major differentiator** — almost no one in VMS does this well.
7. **Cross-camera tracking (re-identification)** — follow a person across multiple cameras automatically. "This person walked past camera A at 14:30, then camera B at 14:32, then camera C at 14:35." Re-id model + spatial reasoning. Cloud-side because it needs data from multiple cameras. **Beta-quality at v1 launch**, improving over time with real customer data.
8. **Anomaly detection** — learn normal patterns per camera (time-of-day baselines, typical activity), flag deviations. Customer-tunable sensitivity. **Beta-quality at v1 launch** — false positive rate is the hard part, expect 2-3 release cycles of tuning post-launch.
9. **Smart event summaries** — daily, weekly, or on-demand reports auto-generated from the day's events using an LLM. "47 vehicles, 12 deliveries, 1 unauthorized after-hours entry." Surfaces in customer admin and integrator portal. LLM is hosted in your cloud (likely Llama 3 or similar) to avoid data egress to third-party LLM providers.
10. **Forensic multi-faceted search** — combine detection types into complex queries. "Find all videos where a red truck appeared between Tuesday and Thursday at the loading dock between 6pm and 8am." Builds on the other features; UI is in the React admin and Flutter app.

**Extensibility:**

11. **Custom AI model upload** — customers upload their own trained models (ONNX, TensorRT, Core ML formats). Models run sandboxed via gVisor containers (cloud-side) or as a separate process with namespaced resource limits (edge). Versioning, A/B testing, backtesting against historical recordings before deploying live. **Major differentiator** — almost no one supports this. Foundation for the v2 marketplace where customers sell their custom models.

### 11.2 Edge vs cloud inference routing

Hybrid inference with intelligent routing per feature and per customer hardware:

| Feature                      | Edge                        | Cloud                                              |
| ---------------------------- | --------------------------- | -------------------------------------------------- |
| Lightweight object detection | ✓ Always                    | Fallback only                                      |
| Heavy object detection       | ✓ If GPU appliance present  | ✓ Otherwise                                        |
| Face recognition             | ✓ If GPU appliance present  | ✓ Otherwise (face vault stays in cloud, encrypted) |
| LPR                          | ✓ If GPU appliance present  | ✓ Otherwise                                        |
| Behavioral analytics         | ✓ Always                    | —                                                  |
| Audio analytics              | ✓ Always                    | —                                                  |
| Smart search via CLIP        | Embeddings computed at edge | Vector index + search queries in cloud             |
| Cross-camera tracking        | —                           | ✓ Cloud only (needs multi-camera data)             |
| Anomaly detection            | Per-camera state at edge    | Cross-time correlation in cloud                    |
| Smart event summaries        | —                           | ✓ Cloud only (LLM-driven)                          |
| Forensic search              | —                           | ✓ Cloud only (combines multiple sources)           |
| Custom model upload          | ✓ Either, customer's choice | ✓                                                  |

The customer's GPU appliance (when they purchase one in v1.x) handles edge inference for the heavier features. v1 customers without GPU appliances run lightweight features at the edge and heavy features in the cloud.

### 11.3 ML infrastructure

**Edge inference**:

- ONNX Runtime + Core ML on Apple Silicon
- TensorRT on NVIDIA GPUs (Jetson Orin for the v1.x edge AI appliance)
- ONNX Runtime + DirectML on Windows
- Standard ONNX Runtime on Linux/CPU

**Cloud inference**:

- NVIDIA Triton Inference Server hosted in EKS
- GPU instances (g5.2xlarge or g5.4xlarge) auto-scaling based on inference queue depth
- Per-model autoscaling so quiet models don't pay for GPU capacity

**Model serving lifecycle**:

- Models versioned in a model registry (custom or MLflow)
- Per-model performance metrics (latency, accuracy, false positive rate, drift detection)
- A/B test framework for new model versions
- Rollback if regression detected

**Vector database**:

- v1: pgvector (PostgreSQL extension) — adequate for ~1M-10M vectors per tenant, lives in your existing Postgres
- v1.x: migrate to Qdrant or Weaviate as scale demands (well-trodden migration path, no architectural disruption)

**Custom model upload security**:

- Models scanned for known malicious patterns before deployment (using ONNX model scanners and a small custom signature scanner)
- Sandbox execution via gVisor containers in EKS (cloud) or namespaced/seccomp'd processes (edge)
- Per-tenant resource quotas (CPU, GPU memory, inference rate)
- Models isolated to the uploading tenant — cannot access other tenants' data
- Pre-deployment backtesting against historical recordings to validate model behavior

### 11.4 Privacy and compliance for face recognition

**Critical**: face recognition is classified as "high-risk AI" under the EU AI Act (effective August 2026) and is regulated under various US state laws (Illinois BIPA, Texas CUBI, Washington's biometric law, etc.).

**Architectural commitments to compliance**:

- Face recognition is **opt-in per camera**, not on by default
- Face vault is **encrypted with customer-managed keys** (CSE-CMK)
- Face data has **explicit retention policy** with auto-deletion
- **Right-to-erasure**: customer can delete a person's face data and all detections of that person
- **Audit log** of every face match with timestamp, camera, matched face, requesting user
- **Customer notice** to end users: face recognition is in use, opt-out mechanism where required by law
- **EU AI Act conformity assessment** documented and submitted before EU market launch
- **CE marking** of the face recognition feature
- **Bias and fairness testing**: face recognition has well-documented accuracy disparities across demographic groups; explicit testing and reporting for fairness, especially for government customers

### 11.5 Model governance

11 model categories shipping in v1 means model governance is a real engineering concern:

- **Model registry**: every model has a unique ID, version, training data documentation, performance metrics, and approval status
- **Performance monitoring**: each deployed model's accuracy, false positive rate, and inference latency tracked in production
- **Drift detection**: statistical drift detection on input distributions, alerts when models start performing below baseline
- **Rollback**: any model version can be rolled back to a previous version with one click
- **SOC 2 / EU AI Act compliance**: model registry data is part of compliance audit evidence

---

## 12. Federation Between Directories

### 12.1 Federation in the cloud-first world

Federation in this architecture serves a more specific role than in pre-cloud designs. The cloud aggregator handles cross-customer / cross-site visibility for cloud-connected customers. **Federation's primary purpose is air-gapped multi-site customers.**

Federation use cases:

1. **Air-gapped multi-site customers** (primary): a federal agency with 3 SCIFs, a defense contractor with 2 secured facilities, a healthcare network with strict regional data residency. These customers cannot use the cloud at all. Their on-prem Directories at each site need to peer with each other directly via a cluster CA + mTLS + Connect-Go.
2. **Cloud-connected customers wanting redundancy** (secondary): customers with strict reliability requirements who want cross-site features to keep working when the cloud is briefly unreachable. Federation provides this fallback.
3. **Private-cloud-deployed customers** (tertiary): customers who deploy the entire cloud stack into their own AWS GovCloud account using v1.x customer-deployable cloud option. They run their own multi-tenant cloud, but at extreme compliance levels they may want air-gapped multi-site federation as defense-in-depth.

### 12.2 Federation cluster CA (separate from per-site cluster CA)

Two distinct PKI domains:

| PKI domain                | Issued by                                                                                                         | Used for                                       |
| ------------------------- | ----------------------------------------------------------------------------------------------------------------- | ---------------------------------------------- |
| **Per-site cluster CA**   | Each site's embedded `step-ca`                                                                                    | Directory ↔ Recorder mTLS within one site     |
| **Federation cluster CA** | The founding Directory's `step-ca` (for air-gapped) OR cloud's identity service (for cloud-connected federations) | Directory ↔ Directory mTLS between peer sites |

The customer never sees either CA. Both are managed entirely by the on-prem Directory binary (or the cloud).

### 12.3 Federation pairing flow

Two clicks to create a federation, two clicks per peer to join. The customer admin clicks "+ Create Federation" → enters federation name → clicks Create. Then "+ Invite another Directory" → generates a `FED-...` token. The token is pasted into the joining Directory's "Join Federation" form. The two Directories handshake, exchange certs, exchange JWKS, and add each other to their `federation_members` table.

Federation joining for cloud-connected federations uses cloud-issued credentials. Air-gapped federations use the cluster-CA enrollment path. The same UI handles both with a per-mode wizard.

### 12.4 Federation control plane

The `FederationPeer` Connect-Go service exposes the cross-Directory operations:

```protobuf
service FederationPeer {
  rpc Ping(PingRequest) returns (PingResponse);
  rpc GetJWKS(GetJWKSRequest) returns (JWKSResponse);
  rpc ListUsers(ListUsersRequest) returns (ListUsersResponse);
  rpc ListGroups(ListGroupsRequest) returns (ListGroupsResponse);
  rpc ListCameras(ListCamerasRequest) returns (ListCamerasResponse);
  rpc SearchRecordings(SearchRecordingsRequest) returns (stream SearchHit);
  rpc MintStreamURL(MintStreamURLRequest) returns (MintStreamURLResponse);
}
```

**Critical property**: no video bytes flow on the federation control plane. Only metadata, search queries, and URL minting. Video flows direct from the owning Recorder to the client, never proxied through a peer Directory.

### 12.5 Catalog sync

Periodic pull of camera/user/group catalogs from each federated peer (default 5 minutes, configurable down to 30 seconds). Peer-specific failures don't block other peers. Catalog cache is queryable for the federated camera browse view and the cross-peer grant UI.

### 12.6 Cross-Directory permission semantics

Same model as cross-tenant for integrators: **explicit grants on the receiving side**. Casbin policy entries with a `federation:peer_directory_id:` subject prefix. The receiving Directory's admin grants specific permissions to specific federation peer subjects. No path lets a peer escalate beyond what the receiving admin granted.

### 12.7 Cross-site recording search

Fan-out from the user's home Directory to all federated peers. Per-peer 10-second timeout. Results merged by timestamp. Partial results returned with `partial: true` if any peer is unreachable. Each peer enforces its own permissions on its own data — the receiving peer doesn't pre-filter for the sending peer's user.

### 12.8 Cross-site playback

URL delegation: home Directory asks the peer to mint a stream URL for its own camera. Peer mints a URL signed by its own key, pointing at its own Recorder/Gateway. Client connects directly to the peer's endpoint. **Video bytes flow direct from peer Recorder to client, never through home Directory.**

### 12.9 Failure semantics

Local site operations are never affected by federation state. If a peer goes offline, cached camera lists remain visible (grayed out), cross-site search excludes that peer with a warning, cross-site playback for that peer's cameras is unavailable. When the peer reconnects, normal operation resumes via catalog sync.

---

## 13. Internal Networking, Pairing, and PKI

### 13.1 Embedded mesh networking via tsnet + Headscale

Every server-side component (Directory, Recorder, Gateway) joins an internal mesh network the moment it's installed, with zero customer network configuration. The mesh is implemented via:

- **`tsnet`** — Tailscale's officially supported library for embedding Tailscale into a Go application. Each component calls `tsnet.Server{...}.Listen(...)` and gets a stable `100.x.y.z` address on the customer's tailnet.
- **Embedded Headscale** — Tailscale's open-source coordination service, embedded into the Directory binary as the per-site coordinator. Headscale issues node pre-auth keys and tells `tsnet` clients how to find each other.

Customer never installs Tailscale, never configures a tailnet, never opens their router admin page. The mesh is invisible.

**Use case**: cross-site reachability for multi-Recorder customers. When a customer has Recorders at HQ, a warehouse, and a branch office, the mesh lets them all reach the Directory without port forwards or VPN configuration. Each Recorder has a stable address on the mesh.

### 13.2 Embedded step-ca (cluster CA)

Step-ca is embedded via `github.com/smallstep/certificates/authority` (Smallstep's officially supported SDK). On Directory first boot:

1. Generate a Cluster Root CA keypair (encrypted with the Directory master key from `mediamtx.yml`)
2. Persist to `/var/lib/mediamtx-directory/pki/`
3. Issue the Directory's own serving leaf cert
4. Configure step-ca's authority instance with provisioners for pairing-token-based enrollment

Cert lifetime: ~24 hours, auto-renewed when remaining lifetime drops below ~8 hours. Hot reload via `tls.Config.GetCertificate` callbacks.

### 13.3 Customer-facing TLS (separate cert chain)

The cluster CA is **only for server-to-server mTLS**. Customer-facing TLS (the Directory's React admin console at `:8443`, the Gateway's public endpoint for off-LAN clients, the Flutter Web bundle URLs) uses **Let's Encrypt** via the embedded Lego client.

Three modes:

- **Auto via Let's Encrypt** — for customers with public DNS + reachable HTTP-01 or DNS-01 challenge endpoints
- **Auto via cloud-issued** — for customers using the cloud-managed deployment, where the cloud handles cert provisioning via its own ACME automation
- **Self-signed fallback** — for LAN-only / air-gapped deployments where the customer accepts a one-time browser warning

### 13.4 Single-token pairing flow

The pairing token is a one-time, 15-minute-TTL bearer credential bundling everything a new Recorder needs to fully join the cluster:

```go
type PairingToken struct {
    TokenID              string
    DirectoryEndpoint    string
    HeadscalePreAuthKey  string
    StepCAFingerprint    string
    StepCAEnrollToken    string
    DirectoryFingerprint string
    SuggestedRoles       []string
    ExpiresAt            time.Time
    SignedBy             UserID
    CloudTenantBinding   string  // optional — if cloud-managed deployment
}
```

Customer experience:

1. On the Directory's React admin: click `+ Add Recorder` → token generated → displayed as a base64 string + QR code
2. On the new Recorder host: run `mediamtx pair eyJrIjoi...` (or scan QR via the first-boot wizard)
3. Within ~10-30 seconds: the new Recorder appears in the admin UI as online

Under the hood, the pair command executes a 9-step join sequence: validate fingerprints → check in with Directory → register with Headscale → generate device key → enroll with step-ca → open Connect-Go stream → receive initial assignment snapshot → write to local cache → report ready.

**Token blast radius is deliberately limited**: a leaked pairing token grants only "join as one Recorder," nothing else. Single-use, 15-minute TTL.

### 13.5 mDNS LAN auto-discovery

For same-LAN deployments, the manual token-paste step is eliminated:

- Directory broadcasts `_mediamtx-directory._tcp.local`
- Brand-new Recorder on first boot listens for the broadcast
- Recorder's first-boot wizard offers: "We detected a Directory at X. Do you want to join it?"
- Customer accepts → admin gets a notification → approves → automatic pairing

Cross-site Recorders fall back to manual token paste.

---

## 14. Remote Access — Three Tiers

### 14.1 Tier 1: LAN-only (default, free, zero setup)

The default state. Nothing to configure. Streams flow direct from Recorders to clients on the LAN. Devices outside the LAN cannot reach the system.

### 14.2 Tier 2: Self-hosted public endpoint (in-app wizard, free)

The Directory's admin UI has a 4-step wizard:

1. **Pick hostname** — free `*.nvr-direct.example` subdomain or BYO domain
2. **Detect network** — embedded `goupnp` detects router UPnP capability + public IP
3. **Configure** — `goupnp` negotiates port mapping; `lego` requests Let's Encrypt cert
4. **Test** — small external probe service confirms reachability from outside the customer network

Background subsystem maintains the endpoint: UPnP lease renewal, DDNS updates on IP changes, cert renewal every ~60 days, periodic external reachability check. "Remote Access Health" widget in admin UI surfaces issues with actionable recovery steps.

### 14.3 Tier 3: Managed cloud relay (in v1, since cloud is in v1)

Customer's Directory/Gateway maintains an outbound TLS WebSocket to the hosted relay service. The relay accepts inbound HTTPS at `acme-loft.relay.yourbrand.com`, validates stream tokens, and multiplexes the request down the existing tunnel to the customer's Directory/Gateway.

**Critical**: the relay never decrypts video bytes. It's a TLS frame multiplexer, not a video proxy. Customer video bytes flow as opaque encrypted bytes through the relay; both endpoints (client and Recorder) terminate TLS, the relay just forwards frames.

**Why this matters**:

- Privacy: relay cannot see customer video
- Bandwidth costs: relay bandwidth is bounded by frame multiplexing, not video transcoding
- Compliance: customers in regulated industries can use the relay because nothing is decoded

### 14.4 Endpoint selection logic

The cloud Directory or on-prem Directory mints stream URLs containing all enabled tiers in priority order. The Flutter client tries them in order with 2-second timeouts. Failover is invisible to the user.

```json
{
  "stream_id": "...",
  "ttl_seconds": 300,
  "endpoints": [
    { "kind": "lan_direct", "estimated_latency_ms": 5 },
    { "kind": "self_hosted_public", "estimated_latency_ms": 40 },
    { "kind": "managed_relay", "estimated_latency_ms": 80 }
  ]
}
```

---

## 15. The Flutter End-User App

### 15.1 Single codebase, six targets

One Flutter codebase compiles to:

| Target  | Distribution                                                                                        |
| ------- | --------------------------------------------------------------------------------------------------- |
| iOS     | Apple App Store + integrator-branded builds in v1                                                   |
| Android | Google Play Store + integrator-branded builds in v1                                                 |
| macOS   | Mac App Store + direct download                                                                     |
| Windows | Microsoft Store + direct download                                                                   |
| Linux   | Flatpak + AppImage + Snap                                                                           |
| Web     | Served by the cloud at `app.yourbrand.com` and by the on-prem Directory at `https://nvr.acme.local` |

**Feature parity is a hard requirement.** A feature shipped on iOS automatically appears on every other target. This is the central architectural commitment for the customer experience.

### 15.2 Connection model

The app maintains exactly one **active home connection** at a time, with cached state for federated peers. Two connection types:

- **Cloud connection** — to the multi-tenant cloud at `cloud.yourbrand.com` (or `command.yourbrand.com` for integrator users). Cloud-managed customers use this.
- **Direct on-prem Directory connection** — to a customer's own Directory at `https://nvr.acme.local` or via the Tier 2/3 public endpoint. Air-gapped or hybrid customers use this.

Same Flutter app, same UI, different backend depending on which mode the user selected at account setup.

### 15.3 Discovery flow

Three discovery paths to add a new connection:

1. Type the URL directly (probes `/api/v1/discover` to fetch metadata + auth methods)
2. mDNS LAN discovery (browse for `_mediamtx-directory._tcp.local`)
3. QR code from the React admin's invite UI

### 15.4 Login flow

White-labeled login screen that fetches available auth methods from `/api/v1/discover` and renders appropriately:

- Local form (email + password) → posts to `/api/v1/auth/login`
- SSO buttons (Microsoft, Google, Okta, custom) → opens in-app browser via `flutter_appauth` with PKCE flow → returns via custom URL scheme → app calls `/api/v1/auth/sso/complete`

The Flutter app speaks **exactly one auth protocol on the wire (OIDC + local form)**. SAML and LDAP are invisible to the client because Zitadel handles them server-side.

### 15.5 Token storage and lifecycle

Tokens stored in `flutter_secure_storage` (iOS Keychain / Android Keystore / encrypted browser storage on Web), keyed by connection ID so multi-Directory account switching works.

Token refresh:

- Access tokens auto-refresh when within 5 minutes of expiry
- Background refresh via `WorkManager` (Android) and `BGTaskScheduler` (iOS)
- Refresh failure (refresh token expired or revoked) bounces user to login screen
- Force-logout from server invalidates tokens server-side, triggering refresh failure on next attempt

### 15.6 Camera browsing

Federated camera tree showing all sites grouped by site label. Live status indicators via WebSocket from the cloud or on-prem Directory. Search global across all sites. Pull-to-refresh triggers fresh catalog fetch.

Federated peers (offline, stale, online) clearly indicated. Permission-filtered: only cameras the user has at least `view.thumbnails` permission on appear in the list.

### 15.7 Live view (single camera)

The most-used screen. Stream initiation flow:

1. User taps camera → app calls `POST /api/v1/streams/request { camera_id, kind: live, protocol: webrtc }`
2. Cloud or on-prem Directory returns ordered endpoints
3. App tries WebRTC against the first endpoint via `flutter_webrtc`
4. If successful within ~3 seconds, video plays
5. If not, fall back to next endpoint or LL-HLS
6. If all fail, show actionable error with retry button

PTZ controls, talkback button, snapshot button, fullscreen mode, pinch-to-zoom (digital), quality indicator.

### 15.8 Multi-camera grid view

For 4 or fewer cameras: real WebRTC streams (using sub-stream when off-LAN to save bandwidth). For 5+ cameras: snapshot mode (refresh every 2-5 seconds). Customer override available: "always use live video in grid view (high data usage)."

### 15.9 Playback timeline view

Scrubbable timeline with multi-Recorder + multi-Directory boundary handling. AI events overlaid as markers. Calendar picker for date jumping. Speed controls (1x, 2x, 4x, 8x). Bookmark + clip export for forensic use.

### 15.10 Push notifications

Push notifications via APNs (iOS) and FCM (Android), with the cloud handling the dispatch. Web uses Web Push API. Desktop uses platform-native notifications.

Subscription preferences per camera + per event type, stored in the cloud so they survive device changes. Notification payload is metadata only (event type, camera ID, thumbnail URL, deep link path) — never video. Tapping a notification deep-links to the right screen.

### 15.11 Multi-Directory account switching

A user can be a member of multiple Directories simultaneously (multiple cloud tenants, on-prem Directories, etc.). Settings → Accounts shows the list. Tapping switches to a different Directory. Tokens stored per-directory, no state leakage.

### 15.12 Offline behavior

Cached camera list, cached thumbnails, cached event history visible when offline. Live and recorded video require connectivity. App is navigable when offline using cached data with stale indicators.

---

## 16. The React Admin Web App

### 16.1 One codebase, two contexts

Same React codebase serving two contexts:

- **Customer Admin context** — accessed at `https://app.acme.cloud.yourbrand.com` (cloud-served, single-tenant scope) or `https://nvr.acme.local/admin` (on-prem-served). Used by customer admins for daily configuration work on one tenant.
- **Integrator Portal context** — accessed at `https://command.yourbrand.com` (cloud-served only, multi-tenant scope). Used by integrator staff for fleet management across all their customers.

Runtime detects context from URL + auth token + `/api/v1/discover` probe. Different navigation trees, different page sets, but shared components and shared API client.

### 16.2 Customer admin pages

| Page                    | Purpose                                                                                 |
| ----------------------- | --------------------------------------------------------------------------------------- |
| **Dashboard**           | Overview of cameras, recent events, system health, alerts                               |
| **Cameras**             | List, add (ONVIF discovery wizard), edit, move between Recorders, delete                |
| **Recorders**           | List paired Recorders, add new (token generation), pair status                          |
| **Live View**           | Single-camera and grid view (mirrors Flutter app for browser users)                     |
| **Playback**            | Timeline scrubber for recorded video                                                    |
| **Events**              | AI detection event list, search, filter                                                 |
| **Users**               | Add, edit, delete users, assign roles                                                   |
| **Permissions**         | Role definitions, grant management, integrator permissions                              |
| **Sign-in Methods**     | Configure SSO providers (Local + OIDC + LDAP + SAML) with the 6 wizards                 |
| **Federation**          | Configure federation with peer Directories                                              |
| **Recording Schedules** | Define schedules (continuous, motion, event-driven)                                     |
| **Retention Policies**  | Per-camera retention with cloud archive options                                         |
| **AI Settings**         | Enable/disable AI features, configure detection thresholds                              |
| **Integrations**        | Configure first-party integrations (access control, alarms, ITSM, comms)                |
| **Notifications**       | Per-user channel preferences, escalation rules                                          |
| **Audit Log**           | Searchable audit trail                                                                  |
| **System Health**       | Recorder status, storage usage, network health, sidecar health                          |
| **Billing**             | View subscription, usage, invoices, payment methods (only for direct-billing customers) |
| **Remote Access**       | Tier 1/2/3 configuration                                                                |
| **Settings**            | Master key, encryption mode, time zone, language                                        |

### 16.3 Integrator portal pages

| Page                     | Purpose                                                                                     |
| ------------------------ | ------------------------------------------------------------------------------------------- |
| **Fleet Dashboard**      | All customers' health at a glance, alerts across all customers, KPIs                        |
| **Customers**            | List of all managed customers, drill-down, add new                                          |
| **Customer Onboarding**  | Create new customer, configure their initial setup, send invitations                        |
| **Brand Configuration**  | White-label settings (logo, colors, fonts, custom domain, custom email domain)              |
| **Mobile App Builds**    | Manage per-integrator mobile app builds, App Store / Play Store deployment, version control |
| **Bulk Operations**      | Push firmware updates to all Recorders, bulk-configure features across customers            |
| **Integrator Staff**     | Manage integrator employees, assign sub-reseller scope, define internal roles               |
| **Sub-Resellers**        | Hierarchical organization management (NSC → regional offices → city offices)                |
| **Customer Permissions** | Granular per-customer scope management (which integrator staff can access which customers)  |
| **Billing Aggregation**  | View all customer billing across the integrator org                                         |
| **Marketing Resources**  | Co-branded sales materials, case studies, ROI calculators                                   |
| **Support Tools**        | Customer impersonation (audited), screen sharing, remote diagnostics, ticket integration    |
| **Channel Programs**     | Tier benefits, certification status, training resources                                     |
| **API Keys**             | Generate and manage API keys for programmatic access                                        |

### 16.4 Tech stack

| Component            | Technology                                                           |
| -------------------- | -------------------------------------------------------------------- |
| Framework            | React 18 with TypeScript                                             |
| Build tool           | Vite                                                                 |
| Routing              | React Router                                                         |
| State management     | Zustand or Redux Toolkit (depending on team preference)              |
| Data fetching        | TanStack Query (React Query) + Connect-Go client                     |
| UI components        | shadcn/ui or Mantine (custom-skinned for white-label)                |
| Charts               | Recharts or Visx                                                     |
| Tables               | TanStack Table with virtualization for large data                    |
| Forms                | React Hook Form + Zod for validation                                 |
| Internationalization | react-i18next with 4 languages (EN/ES/FR/DE) at launch               |
| Testing              | Vitest for unit, Playwright for E2E                                  |
| Accessibility        | WCAG 2.1 AA compliance, axe-core in CI, manual screen reader testing |

### 16.5 White-label rendering

Both contexts dynamically apply white-label config at runtime:

- Logo loaded from per-integrator brand config
- Color scheme applied via CSS variables
- Typography overrides via CSS
- Custom domain handled at the load balancer / CDN level (CNAME → CloudFront → routes to the React app)
- Email templates pulled from per-integrator config (used by the cloud's notification service)

### 16.6 Embedded in on-prem Directory binary

The React build artifact is embedded into the on-prem Directory binary via `//go:embed`. When a customer accesses `https://nvr.acme.local/admin`, the Directory's HTTP server serves the React app as static files. Same React build, served from two places.

---

## 17. The Video Wall Client

### 17.1 What it is and why

A native desktop application for Security Operations Center (SOC) operators driving multi-monitor displays. **Cannot be served by the Flutter app** because:

- Rendering 64+ concurrent live video streams with hardware decode acceleration is beyond Flutter's web/desktop performance characteristics
- Multi-monitor support (driving 8+ displays from one workstation) requires native desktop integration
- PTZ keyboard / joystick hardware integration (Axis T8311, Axiom, Honeywell) requires native USB / serial APIs
- Operator workflows (alert acknowledgment, incident escalation, shift handover) need a different UX than mobile

Genetec Security Center Workstation, Milestone XProtect Smart Client, and Avigilon Control Center Client are all native desktop apps. The video wall is competing in this space and has the same technical requirements.

### 17.2 Tech stack: Qt 6 + C++

| Component                | Technology                                                                    |
| ------------------------ | ----------------------------------------------------------------------------- |
| Framework                | Qt 6 with C++ (Qt Quick / QML for UI, C++ for performance-critical paths)     |
| Video rendering          | Qt Multimedia + custom DirectX 12 / Vulkan rendering for multi-stream scaling |
| Hardware decode          | NVIDIA NVENC, Intel QuickSync, AMD AMF, Apple VideoToolbox                    |
| WebRTC stack             | libwebrtc (C++) bound into Qt                                                 |
| HLS / RTSP               | Native libavformat (FFmpeg) for HLS and RTSP playback                         |
| PTZ keyboard integration | Native USB HID + serial libraries (libusb, qtserialport)                      |
| Maps                     | Qt Location with offline tile support                                         |
| Build system             | CMake + vcpkg for dependency management                                       |
| Distribution             | Qt Installer Framework for Windows, native packages for Linux                 |

**Why Qt 6 over alternatives**:

- Mature multi-monitor support (every competitor uses Qt for a reason)
- Mature multimedia stack with hardware decode bindings
- Cross-platform (Windows + Linux primary, macOS optional)
- Large hiring pool of Qt/C++ engineers
- Performance ceiling for high-camera-count rendering

### 17.3 Features

**Multi-monitor and layout management**:

- Drive 4-32 monitors from one workstation (limit is hardware, not software)
- Per-monitor custom layouts (4×4, 6×6, 9×16, picture-in-picture, focus modes)
- Saved scenes / presets — instant switch between layout configurations
- Salvo presets — single button press flips every monitor to specific cameras (for emergency response)
- Tour mode — cycle through cameras automatically on a schedule

**Performance**:

- 64+ concurrent live streams with hardware decode
- Per-stream quality auto-adjustment based on monitor pixel coverage
- Scrolling and zooming without dropped frames
- Cold start to first frame in <2 seconds

**Operator features**:

- PTZ keyboard / joystick integration (Axis T8311, Axiom, Honeywell, Bosch)
- Custom hotkeys for every operator action
- Multi-user simultaneous viewing on the same wall with independent cursors
- Event-driven layouts: alarm fires → wall automatically swaps to affected camera + neighbors
- Map view with camera icons placed on floor plans or geographic maps
- Bookmark / instant replay (tag a moment, replay last 30 seconds, export clip)
- Audio monitoring with selectable per-camera audio
- Talkback integrated into operator workflow (push-to-talk hardware button)
- Alert acknowledgment, triage, escalation, shift handover notes
- Investigator workflow tools (timeline review, multi-camera correlation, export with chain of custody)

**Operational features**:

- Auto-recovery from network drops without losing layout state
- Crash recovery with restore-on-launch
- Logging integrated with the same observability platform as the rest of the system

### 17.4 Distribution and packaging

Windows installer signed with EV code signing certificate (required for enterprise IT to allow installation). Linux .deb / .rpm packages. macOS as a stretch goal (most SOCs are Windows shops).

Per-integrator branding via Qt Installer Framework parameters: integrator can request a custom-branded build with their logo, name, and color scheme.

Updates pushed via in-app updater (Sparkle on macOS, similar on Windows).

### 17.5 Estimated v1 effort

~80 engineer-weeks for v1, focused on Windows-first delivery with Linux as a secondary target. macOS deferred to v1.x.

---

## 18. The Marketing Website

### 18.1 Tech stack

| Component          | Technology                                                 |
| ------------------ | ---------------------------------------------------------- |
| Framework          | Next.js 14+ (App Router)                                   |
| CMS                | Sanity (headless)                                          |
| Hosting            | Vercel                                                     |
| Analytics          | PostHog + Plausible                                        |
| A/B testing        | PostHog feature flags                                      |
| CRM integration    | HubSpot (lead capture and routing)                         |
| Form handling      | React Hook Form + HubSpot API                              |
| SEO                | Next.js Metadata API + schema.org markup + structured data |
| i18n               | next-intl with 4 languages (EN/ES/FR/DE) at launch         |
| Image optimization | Next.js Image + Cloudinary for advanced transforms         |
| Search             | Algolia for site search                                    |

### 18.2 Pages and content

| Page / Section                | Purpose                                                                                           |
| ----------------------------- | ------------------------------------------------------------------------------------------------- |
| **Homepage**                  | Hero, value props, social proof, primary CTAs (Try Free, Schedule Demo, Become a Partner)         |
| **Product overview**          | Feature deep-dive, screenshots, video demos                                                       |
| **Per-product feature pages** | Cloud platform, AI, white-label, integrator portal, video wall, etc. (one page per major area)    |
| **Pricing**                   | Tier comparison, ROI calculator, FAQ                                                              |
| **Use cases / verticals**     | Retail, healthcare, education, government, multi-site enterprise                                  |
| **Customer case studies**     | Gated detailed case studies (lead capture)                                                        |
| **Comparison pages**          | "vs Verkada" / "vs Milestone" / "vs Genetec" / "vs Avigilon" / "vs Eagle Eye" — SEO + sales tools |
| **Become a Partner**          | Integrator-targeted landing page, partner program details                                         |
| **Integrator Directory**      | "Find a certified installer near you" — searchable by city/zip                                    |
| **Trust Center**              | Security, compliance, sub-processors, status page link                                            |
| **Blog**                      | Thought leadership, product updates, industry trends                                              |
| **Documentation**             | Links to the docs portal                                                                          |
| **Careers**                   | Open positions, company culture, application form                                                 |
| **Contact**                   | Sales, support, press contacts                                                                    |
| **Legal**                     | Terms of Service, Privacy Policy, Cookie Policy, DPA, AUP, etc.                                   |

### 18.3 Lead capture and routing

- Multiple CTAs on every page route to HubSpot
- Lead scoring based on behavior (pages visited, time on site, content downloaded)
- Routing rules: SMB → PLG self-serve flow, mid-market → SDR queue, enterprise → AE direct
- Demo scheduling via Calendly or Chili Piper
- Trial signup creates a Free tier account directly (no sales touch)

### 18.4 Interactive product demo

Embedded interactive demo running against the sandbox tenant (from the onboarding question). Visitors can click through a simulated camera grid, try the playback timeline, run an AI search, all without signing up. Major conversion driver.

### 18.5 ROI calculator

Comparison calculator: customer enters their current VMS spend + camera count + retention requirements, calculator shows estimated annual cost on your platform (based on per-camera pricing). Useful both for inbound leads and as a sales tool.

### 18.6 Integrator directory

Searchable directory of certified integrators: customer enters zip code, sees integrators in their area, filters by certifications, clicks through to integrator's profile (auto-generated from their integrator portal data). Drives leads to integrators (revenue for them) and validates platform credibility (lots of integrators = healthy ecosystem).

---

## 19. The Documentation Portal

### 19.1 Tech stack

| Component       | Technology                                                          |
| --------------- | ------------------------------------------------------------------- |
| Platform        | Mintlify (or Docusaurus as backup)                                  |
| API reference   | Auto-generated from OpenAPI spec via Mintlify's OpenAPI integration |
| Search          | Algolia DocSearch or Mintlify built-in                              |
| AI search       | Inkeep — trained on docs as knowledge base                          |
| Code examples   | Embedded interactive code blocks with multi-language tabs           |
| Video tutorials | Embedded via Mux or Vimeo                                           |
| i18n            | Mintlify multi-language support, 4 languages at launch              |
| Versioning      | Per-major-version doc trees                                         |

### 19.2 Audiences and sections

| Audience           | Section                                                                                                                                 |
| ------------------ | --------------------------------------------------------------------------------------------------------------------------------------- |
| **End user**       | Getting started, daily use, mobile app, web client, watching cameras, playback, alerts                                                  |
| **Customer admin** | System setup, user management, camera management, AI configuration, integrations, federation, billing                                   |
| **Integrator**     | Becoming a partner, white-label setup, customer onboarding, fleet management, support tools, billing aggregation, certification program |
| **Developer**      | API reference, SDKs (Python/Go/TypeScript), webhooks, OAuth, integration guides, code samples                                           |
| **Operator (SOC)** | Video wall client, layouts, scenes, PTZ, alarm response, shift handover                                                                 |
| **Hardware**       | Compatibility list, installer image, deployment guides per certified config                                                             |
| **Compliance**     | SOC 2, HIPAA, GDPR, EU AI Act, FedRAMP roadmap, sub-processors                                                                          |

### 19.3 Content types

- **Tutorials** — step-by-step guides for common tasks (~50 at launch)
- **How-to recipes** — short focused solutions (~100 at launch)
- **Reference** — exhaustive details on every feature, API, config option
- **Concepts** — explanations of architectural concepts (federation, multi-tenancy, AI inference routing)
- **Video tutorials** — narrated screen recordings for visual learners (~30 at launch)
- **Interactive code examples** — runnable in the browser via WebContainer
- **API reference** — auto-generated, always in sync with the codebase

### 19.4 White-label per integrator

Integrators can configure a white-labeled docs subset:

- Custom domain (`docs.acmealarm.com`)
- Integrator's logo and brand
- Selected docs that apply to their specific offering
- Custom landing page

### 19.5 Embedded in product

Contextual help embedded in the React admin: every page has a help icon that opens the relevant doc page in a sidebar without leaving the product. Reduces friction for "how do I do X" questions.

### 19.6 Offline docs bundle

For air-gapped customers: the docs portal exports a complete offline bundle (HTML + assets) that can be served from the on-prem Directory. Same content, no internet required.

### 19.7 AI-powered help

Inkeep (or similar) trained on the docs as a knowledge base, exposed as:

- Search box in the docs portal that returns AI-generated answers + source citations
- Embedded chat in the React admin's help sidebar
- Embedded in customer support tooling (Intercom) for AI-first triage

Deflects ~30-40% of common support questions.

---

## 20. Customer Support Tooling

### 20.1 Stack

| Component                | Technology                                                                  |
| ------------------------ | --------------------------------------------------------------------------- |
| Ticketing platform       | Intercom                                                                    |
| AI support assistant     | Inkeep (trained on docs)                                                    |
| Live chat                | Intercom in-app messenger                                                   |
| Knowledge base           | Mintlify docs portal (linked from Intercom)                                 |
| Screen sharing           | Intercom Screen Share or external (Zoom, Whereby)                           |
| Customer impersonation   | Custom-built, audited, per-tier-restricted                                  |
| Remote diagnostics       | Custom-built collector that pulls logs/metrics/state from on-prem Recorders |
| Incident management      | PagerDuty or Incident.io                                                    |
| CSM tooling              | Vitally or Catalyst                                                         |
| Customer success scoring | Mixpanel + custom scoring model                                             |

### 20.2 Support tiers

| Tier           | Channels                                                                         | SLA                       | CSM             |
| -------------- | -------------------------------------------------------------------------------- | ------------------------- | --------------- |
| **Free**       | Docs + community forum + self-service                                            | None                      | None            |
| **Starter**    | Email + AI assistant                                                             | 24h business response     | None            |
| **Pro**        | Email + chat + AI assistant + onboarding call                                    | 8h business response      | Shared CSM pool |
| **Enterprise** | All channels + dedicated CSM + screen-share + remote diagnostics + 24/7 critical | 1h critical / 4h business | Dedicated CSM   |

### 20.3 Customer impersonation

Critical for support without violating customer trust. Architecture:

- Integrator staff can impersonate their managed customers (audited, scope-limited)
- Platform support team can impersonate any tenant **only with explicit customer authorization** (customer admin grants a time-limited "support session" token)
- Every action during impersonation is logged with `impersonating_user` + `impersonated_tenant` fields
- Customer admin sees impersonation events in their audit log in real time
- Impersonation auto-terminates after 4 hours unless explicitly extended

### 20.4 Remote diagnostics

For on-prem Recorders, support engineers need to pull diagnostic data without screen-sharing. The diagnostic collector:

- Pulls structured logs (last N hours)
- Pulls metrics snapshots
- Pulls camera state
- Pulls Recorder hardware health (CPU, memory, disk, network)
- Pulls sidecar status (Zitadel, MediaMTX)
- Bundles into a single archive
- Encrypts with customer's master key
- Uploads to a temporary support storage location for the support engineer to download
- Auto-deletes after 7 days

Customer triggers via a one-click "Generate Support Bundle" button in the React admin; integrator staff or platform support gets the bundle ID via Intercom.

### 20.5 Customer success scoring

Real-time health score per customer based on:

- Active feature usage (which features they touch)
- Recording health (any cameras offline?)
- User engagement (logins per week)
- Support ticket frequency
- NPS / CSAT survey responses
- Feature requests / complaints

Used by CSM team to prioritize outreach: customers with declining scores get proactive outreach before they churn.

---

## 21. Status Page

### 21.1 What it covers

Customer-facing status page at `status.yourbrand.com` showing real-time health for:

- Cloud control plane (per-region, US-East-2 only in v1)
- Identity service
- Cloud Directory service
- Integrator portal backend
- AI inference service
- Recording archive service
- Notification infrastructure
- Cloud relay (Tier 3)
- Marketing site
- Documentation portal

### 21.2 Per-integrator white-label status pages

Each integrator gets a white-labeled status page subdomain (`status.acmealarm.com`) that shows:

- Their integrator portal status
- Status of services that affect their managed customers
- Their custom branding

Customers see the integrator's status page, not yours.

### 21.3 Integration with monitoring

Status events are automatically generated from:

- Prometheus alerts (when error rates spike)
- Synthetic monitoring (Pingdom or similar polling key endpoints)
- Manual incident creation by on-call engineers

Each component shows: operational, degraded, partial outage, major outage. Historical SLA reporting per-component, per-region, per-time-period.

### 21.4 Subscriber notifications

Customers and integrators can subscribe to status updates via:

- Email
- SMS (Twilio)
- Webhook (for integration into their own monitoring)
- RSS / Atom feed
- Slack/Teams integration

### 21.5 Tech stack

Statuspage.io (Atlassian) or Better Stack. Don't build in-house.

---

## 22. Notification Infrastructure

### 22.1 Channels

| Channel              | Provider                              | Use case                                                         |
| -------------------- | ------------------------------------- | ---------------------------------------------------------------- |
| Email                | SendGrid                              | Daily summaries, alert digests, account notifications            |
| SMS                  | Twilio                                | Critical alerts, 2FA codes, escalations                          |
| Voice calls          | Twilio                                | Critical-tier alerts that require immediate human acknowledgment |
| WhatsApp             | Twilio (Business API)                 | International customers, geographic preferences                  |
| Push                 | FCM (Android) / APNs (iOS) / Web Push | Real-time mobile alerts                                          |
| Slack / Teams        | Native integrations                   | Team channel alerts                                              |
| PagerDuty / Opsgenie | Native integrations                   | SOC operator alerting                                            |
| Webhook              | Outbound HTTP POST                    | Custom integration                                               |

### 22.2 Per-user channel preferences

Each user configures, per camera + per event type:

- Which channels to use
- Quiet hours (time-of-day suppression)
- Severity threshold

### 22.3 Escalation chains

Customer admins define escalation rules: if user X doesn't acknowledge a critical alert within 5 minutes, escalate to user Y; if Y doesn't acknowledge within 5 more minutes, escalate to PagerDuty / on-call rotation.

### 22.4 ML-based alert suppression

Reduces alert fatigue by learning patterns:

- Cluster related events into one notification ("3 motion events in the loading dock, last 5 minutes")
- Suppress alerts during expected high-activity windows
- Identify and suppress recurring false positives
- Customer-tunable sensitivity

### 22.5 Per-integrator email templates

Email templates pulled from per-integrator brand config (white-label Level 3). Integrator's logo, colors, support contact in every email.

### 22.6 Notification UI

In-product notification center with read/unread state, filtering, search, archive. Same UX in Flutter app and React admin.

---

## 23. Pricing and Billing

### 23.1 Plan structure

| Tier             | Per camera/month (direct customer wholesale) | Integrator wholesale | Includes                                                                                                                                        |
| ---------------- | -------------------------------------------- | -------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------- |
| **Free**         | $0                                           | $0                   | Up to 4 cameras, 7 days retention, basic detection (motion + simple object), 1 user, watermarked exports, no SSO, community support             |
| **Starter**      | $15                                          | $9                   | Up to 32 cameras, 30 days retention, full object detection, unlimited users, SSO, removed watermark, email support                              |
| **Professional** | $30                                          | $18                  | Up to 256 cameras per site, 90 days retention, advanced AI (face/LPR/behavioral), federation, all integrations, priority support, SOC 2 reports |
| **Enterprise**   | $45+ (custom)                                | custom               | Unlimited cameras, custom retention, all add-ons by default, dedicated success manager, custom SLAs, FedRAMP path, on-prem or private cloud     |

### 23.2 Add-ons (regardless of tier)

- **Cloud archive — extended retention**: $X per camera per month per additional 30 days
- **Edge AI inference appliance**: per-Recorder fee (v1.x with the GPU appliance)
- **Custom AI model upload**: per-model per-month for hosting custom-trained detectors
- **Premium support / dedicated TAM**: monthly fee
- **Air-gapped private cloud deployment**: large one-time + ongoing license fee
- **Compliance reports**: HITRUST / CJIS / FedRAMP-specific bundles
- **Voice-call alerting overage**: per-call fee above bundled allowance

### 23.3 Billing model: hybrid customer-chooses

At customer onboarding, both billing modes are available:

- **Direct customer billing**: platform invoices the customer directly, customer pays the platform
- **Integrator-rebilled**: platform invoices the integrator at wholesale, integrator marks up and invoices their customer separately (potentially through the same platform)

Implementation:

- Per-tenant `billing_mode` field (`direct` or `via_integrator`)
- Per-integrator `wholesale_discount_percent` (your discount to them)
- Per-integrator-customer-relationship `markup_percent` (their markup to the customer)
- Stripe Connect for the money flow + 1099 reporting + KYC for integrators acting as marketplace facilitators

### 23.4 Tax compliance

Avalara or Anrok handles multi-jurisdiction sales tax / VAT. Required for international + US state-level compliance. Non-optional.

### 23.5 Free tier abuse prevention

Free tier is limited to 4 cameras + 1 user + 7 days retention. Monitor for:

- Multiple accounts from same IP / device fingerprint
- Suspicious signup patterns
- Free-tier accounts exceeding limits via creative tricks

### 23.6 Usage tracking and reporting

Per-tenant resource accounting:

- Cloud bandwidth (bytes streamed through Gateway and cloud relay)
- Cloud storage (GB stored in cloud archive, per tier)
- AI inference (GPU-hours used cloud-side)
- API requests (rate-limited and tracked)

Used for:

- Customer-facing usage reports (transparency)
- Overage billing for customers exceeding bundled limits
- Cost-of-goods analysis per customer (which customers are profitable)
- Capacity planning for SRE

---

## 24. White-Label (Level 3)

### 24.1 What's white-labeled

| Dimension                                    | Customer-visible?                                          |
| -------------------------------------------- | ---------------------------------------------------------- |
| Logo and visual identity                     | Always integrator's                                        |
| Color scheme and typography                  | Always integrator's                                        |
| Custom domain (`security.acmealarm.com`)     | Integrator's                                               |
| Email sender domain (`alerts@acmealarm.com`) | Integrator's                                               |
| Mobile apps                                  | Per-integrator builds under integrator's developer account |
| Content / help text / error messages         | Customizable per integrator                                |
| Legal documents (ToS, Privacy)               | Integrator's                                               |
| Status page subdomain                        | Integrator's                                               |
| Documentation portal                         | Per-integrator white-labeled subset                        |
| Push notification sender name                | Integrator's                                               |
| Customer support contact                     | Integrator's                                               |

The customer is unaware of your brand in normal operation. Your brand only appears in:

- Legal sub-processor disclosures (where required)
- App Store developer field (showing the integrator as the developer)

### 24.2 Per-integrator mobile app build pipeline

The single most differentiated feature in the white-label program. Most VMS competitors do not offer this.

**How it works**:

1. Integrator uploads brand assets (logo, splash, app icon, color scheme) in the integrator portal
2. Integrator provides Apple Developer Program credentials and Google Play Developer credentials (or grants you upload access)
3. The cloud's mobile build pipeline (CI service) takes the Flutter codebase + per-integrator brand config + per-integrator developer account credentials and produces:
   - `.ipa` file for iOS, signed with integrator's distribution certificate
   - `.aab` file for Android, signed with integrator's keystore
4. Builds are uploaded to App Store Connect (iOS) and Google Play Console (Android) under the integrator's accounts
5. Integrator-branded apps appear in the App Store as "Acme Security" with no mention of your brand

**Build pipeline**:

- GitHub Actions (or Buildkite) runs parallel builds per integrator
- Each integrator has a separate Bundle ID, app name, splash screen, icon set, string overrides, color scheme
- Builds are reproducible from a manifest checked into integrator's brand config
- Integrator can request a fresh build any time by clicking "Rebuild Mobile App" in the integrator portal
- New version releases automatically trigger rebuilds for all integrators

**Effort**: ~10-12 weeks for the build pipeline + ongoing operational support.

### 24.3 Custom domain handling

Each integrator can configure a custom domain (`security.acmealarm.com`):

1. Integrator creates a CNAME pointing at `cloud.yourbrand.com`
2. Integrator portal validates the DNS configuration
3. Cloud automatically requests Let's Encrypt cert via Lego ACME
4. CloudFront / ALB routes the integrator's domain to the appropriate React app context (customer admin, scoped to the integrator's tenants)

### 24.4 Per-integrator email infrastructure

Each integrator can configure custom sender domains (`alerts@acmealarm.com`):

1. Integrator provides their domain
2. Integrator portal generates SPF, DKIM, and DMARC records the integrator adds to their DNS
3. Cloud verifies the DNS records
4. SendGrid is configured with the integrator's verified domain as a sender
5. All email notifications for that integrator's customers come from the integrator's domain

Bounce handling routes to a per-integrator deliverability dashboard.

### 24.5 Content overrides

Every customer-visible string in the React admin and Flutter app is in a translation/override system. Per-integrator overrides allow customizing:

- App welcome text
- Help text
- Error messages
- Email subject lines and body content
- Notification text
- Onboarding flow copy

---

## 25. Hardware Compatibility Program (Software-Only v1)

### 25.1 No physical inventory in v1

v1 is software-only. No appliances shipped. No manufacturer contracts. No inventory. No warranty handling. No hardware ops hire.

### 25.2 Certified hardware list

Published list of specific tested hardware configurations:

- **Small**: Specific Lenovo / Dell / Supermicro micro-form-factor PCs for 8-16 cameras
- **Medium**: Specific 1U rackmount servers for 32-64 cameras
- **Large**: Specific 2U servers + GPU configurations for 128+ cameras / AI workloads
- **Edge AI**: Specific NVIDIA Jetson Orin configurations for AI-heavy edge inference

Each certified config has:

- Exact part numbers
- Tested performance specs
- Pre-built OS image
- Step-by-step setup guide
- Manufacturer warranty info
- Direct purchase links

### 25.3 Pre-built installer images

Bootable installer image:

- Customer flashes a USB stick
- Boots from USB on their hardware
- Installer detects hardware (CPU, RAM, disks, NICs, GPU)
- Validates against certified configs
- Walks through partition setup, RAID config
- Installs OS + binary + sidecars
- Reboots into first-boot wizard

### 25.4 Integrator hardware kits

Recommended kits with published BOMs:

- **Coffee shop kit**: ~$1,200 in hardware + your software → 8 cameras
- **Retail location kit**: ~$3,500 → 32 cameras
- **Multi-tenant building kit**: ~$8,500 → 128 cameras

Integrators source the hardware themselves and resell to customers with their own margin.

### 25.5 Hardware health monitoring on commodity hardware

The on-prem binary reads from standard Linux sensors:

- `smartmontools` for disk SMART data
- `lm-sensors` for temps and fan speeds
- IPMI tools where available
- NVIDIA-smi for GPU
- Standard Linux network stats

Surfaces in the React admin's System Health page. Less polished than dedicated BMC integration but works on every certified config.

### 25.6 Reference appliance program (deferred to v1.x)

When customer demand justifies it (likely 6-12 months post-launch), add a single reference appliance SKU sourced from Supermicro, pre-configured, sold through integrators. ~10 weeks of work + ~$200-400k capital.

### 25.7 Full multi-SKU hardware program (deferred to v2)

The full SMB / Mid / Enterprise / Edge AI lineup with co-branded options, manufacturer relationships, and inventory ops. v2 commitment based on year-1 customer data.

---

## 26. Migration from Existing Single-NVR Customers

### 26.1 Scope

v1 includes the migration tool for existing single-NVR customers (your current product) to upgrade to the new multi-server architecture. **Migration from competitor products (Milestone, Genetec, Verkada, etc.) is deferred to v1.x.**

### 26.2 Five-phase migration tool

The same five-phase tool design from the original spec carries forward, with adjustments for the cloud-first architecture:

1. **Backup**: snapshot `nvr.db`, `mediamtx.yml`, recordings manifest, AI models, ONVIF caches
2. **Bootstrap new components**: start Zitadel sidecar, bootstrap step-ca, create single-node Headscale tailnet, optionally connect to cloud
3. **Identity migration**: migrate users from old `nvr.db` to Zitadel with temp passwords + reset links
4. **Camera migration**: migrate cameras to new Directory schema, encrypt RTSP credentials, rebuild segment index
5. **Cutover**: stop old binary, start new binary, verify, auto-rollback on failure

**New in this rewrite**: Phase 2 includes an optional cloud connection. Customers can choose to migrate to a cloud-connected deployment (linking to a new cloud tenant), an air-gapped deployment (no cloud), or hybrid.

### 26.3 Backwards-compat REST shim

The new Directory exposes the old REST API at `/api/nvr/...` as a deprecated compatibility layer for ~12 months. Existing Flutter clients and third-party API integrations keep working without modification during the deprecation window.

### 26.4 Migration testing

- Test corpus of ~20 representative source data scenarios
- Dry-run mode that runs all phases except cutover
- Schema version pinning (refuses to migrate from unrecognized source schemas)
- Verification phase with auto-rollback on discrepancy

### 26.5 Customer-facing migration docs + runbook

Polished documentation for customers + internal runbook for support team. Same as the original spec.

---

## 27. Compliance Program

### 27.1 v1 launch compliance posture

Group A — must-have for v1 launch:

1. **SOC 2 Type I report** — start program at month 1 of v1 dev, snapshot audit at month 9-12, report available before public launch
2. **HIPAA-ready architecture + BAA template** — designed in
3. **GDPR compliance** — architecture + legal docs + DPA for European customers
4. **CCPA compliance** — comes for free with GDPR
5. **FIPS 140-3 validated cryptography** — chose libraries from day one (BoringSSL, RustCrypto with FIPS, etc.)
6. **Section 508 / WCAG 2.1 AA compliance** — for the React admin
7. **EU AI Act compliance** — required by August 2026 for high-risk AI systems (face recognition is one)
8. **Penetration test report** — pre-launch external pen test by NCC Group / Bishop Fox / Trail of Bits / Doyensec
9. **Bug bounty program** — launched at GA via HackerOne or Bugcrowd

### 27.2 Post-launch compliance roadmap

Group B — 6-12 months post-launch:

- SOC 2 Type II report (continuous evidence collection from launch, first report ~12 months post-launch)
- ISO 27001 certification (start program in v1 dev, audit ~12 months post-launch)
- CJIS compliance (when first law enforcement customer)
- HITRUST CSF (when first major healthcare customer)

### 27.3 Long-term compliance roadmap

Group C — only when customer demand justifies:

- FedRAMP Moderate (only when first federal customer signs and sponsors the audit)
- ISO 27017 + ISO 27018 (incremental on 27001)
- CMMC Level 2 (when first defense contractor customer)
- FedRAMP High (only after Moderate generates demand)

### 27.4 Compliance program operations

- **Head of Security & Compliance** hired month 1-3 of v1 development
- Compliance evidence platform (Vanta or Drata): ~$15k-50k/year
- Auditor selection in month 3-4 (A-LIGN, Coalfire, Schellman, etc.)
- Pen test in month 9-10
- Security training program (KnowBe4)
- Internal security policies (~30 documents)
- Quarterly internal audits + ongoing pen tests post-launch
- Public trust center at `trust.yourbrand.com`

### 27.5 EU AI Act specifics

Face recognition is high-risk AI under the EU AI Act. Required:

- Risk assessment + risk management documentation
- Data governance documentation (training data, bias testing)
- Technical documentation maintained throughout system lifecycle
- Logging and traceability
- Transparency requirements
- Human oversight provisions
- Accuracy, robustness, cybersecurity requirements
- Conformity assessment before EU market launch
- CE marking
- Registration in EU's high-risk AI database

Required by **August 2, 2026**. v1 launching in 2026 or 2027 must be compliant from day one — no grace period.

### 27.6 Annual compliance program cost

| Item                                                | Year 1          | Year 2          | Year 3+        |
| --------------------------------------------------- | --------------- | --------------- | -------------- |
| Head of Security & Compliance + 1 security engineer | $400-600k       | $500-700k       | $600k-1M       |
| Compliance platform                                 | $20-50k         | $30-70k         | $50-100k       |
| SOC 2 Type I audit                                  | $30-100k        | —               | —              |
| SOC 2 Type II audit                                 | —               | $50-150k        | $50-150k/year  |
| ISO 27001 audit                                     | —               | $30-80k         | $20-50k/year   |
| Pen testing                                         | $80-200k        | $80-200k/year   | $100-300k/year |
| Bug bounty program                                  | —               | $20-100k/year   | $50-300k/year  |
| Legal review (DPAs, BAAs, terms)                    | $50-150k        | $50-100k/year   | $50-100k/year  |
| HIPAA + GDPR consulting                             | $40-100k        | $20-50k/year    | $20-50k/year   |
| Security training                                   | $5-15k          | $5-15k/year     | $5-25k/year    |
| **Year total**                                      | **~$625k-1.2M** | **~$785k-1.4M** | **~$945k-2M**  |

---

## 28. Customer Onboarding Experience

### 28.1 Three onboarding journeys

**Direct customer journey**:

- Marketing site → "Try Free" → self-serve signup → first-boot wizard → add first camera → see live frame → onboarding email drip

**Integrator-led customer journey**:

- Integrator portal → "+ Add Customer" → integrator goes to customer site → installs hardware → first-boot wizard auto-detects integrator-led setup → pulls customer config from cloud → ONVIF discovery → cameras configured → invite customer admin → customer receives polished email → downloads white-labeled mobile app → signs in → cameras visible

**Integrator first-hour journey**:

- Marketing site → "Become a Partner" → integrator signup → brand setup wizard → "would you like a sandbox to play with?" → sandbox tenant created with simulated cameras → integrator explores → adds first real customer

### 28.2 Sandbox / demo mode

Pool of simulated camera streams hosted in your cloud (synthetic RTSP streams of stock footage). Sandbox tenants can be spun up by anyone in seconds:

- Direct customers: "Try a demo before installing hardware"
- Integrators: "Set up a sandbox customer to play with first"
- Sales: "Generate a demo account for this prospect"

Sandbox tenants are ephemeral (auto-delete after 30 days unless promoted to a real account). All AI features run against the simulated streams.

### 28.3 First-boot wizard (Standard / B2 scope)

Walks the customer through:

- Set master key (or auto-generate)
- Create admin account
- Add one camera via ONVIF discovery
- Configure storage and retention
- Set up notification preferences
- Add additional users
- Configure remote access
- Optional: configure SSO
- Brief product tour

~10 minutes from boot to productive.

### 28.4 In-app guidance

- **Tooltips** on hover for contextual hints
- **Persistent checklists** that nudge customers toward valuable features they haven't tried yet ("you've added cameras, now try setting up alerts")
- No intrusive product tours

### 28.5 Email drip campaign

5-7 email sequence over 30 days, behavior-triggered via Customer.io or HubSpot:

- Day 0: Welcome email
- Day 1: Getting started tips
- Day 3: "Have you set up alerts yet?"
- Day 7: AI features showcase
- Day 14: Integration tutorials
- Day 21: Customer success story
- Day 30: NPS survey

### 28.6 Customer success scoring

Real-time health score per customer drives proactive outreach.

---

## 29. Integrations Ecosystem

### 29.1 Layer 1: Integration platform

| Component                    | What it does                                                                     |
| ---------------------------- | -------------------------------------------------------------------------------- |
| Outbound webhooks            | Customer-configured URLs receive event payloads (signed, retry-able, filterable) |
| Inbound webhooks             | Customer-defined endpoints trigger actions in the system                         |
| Public REST + Connect-Go API | Full programmatic access, versioned, authenticated                               |
| OpenAPI spec                 | Auto-generated from Connect-Go service definitions                               |
| API key management           | Per-tenant keys, scoping, rotation, audit                                        |
| Public SDKs                  | Python, Go, TypeScript (Java + C# in v1.x)                                       |
| Integration developer docs   | At `developers.yourbrand.com`                                                    |

### 29.2 Layer 2: First-party integrations (12 in v1)

| #   | Integration              | Category                                              |
| --- | ------------------------ | ----------------------------------------------------- |
| 1   | Brivo                    | Cloud access control                                  |
| 2   | OpenPath / Avigilon Alta | Cloud access control                                  |
| 3   | ProdataKey (PDK)         | Cloud access control                                  |
| 4   | Bosch B/G-Series         | Alarm panels                                          |
| 5   | DMP XR-Series            | Alarm panels                                          |
| 6   | PagerDuty                | ITSM / alerting                                       |
| 7   | Opsgenie                 | ITSM / alerting                                       |
| 8   | Slack                    | Channel alerts                                        |
| 9   | Microsoft Teams          | Channel alerts                                        |
| 10  | Zapier app               | Universal automation (~5000+ downstream destinations) |
| 11  | Make (Integromat)        | European workflow automation                          |
| 12  | n8n                      | Self-hosted workflow automation                       |

Each first-party integration includes:

- Configuration UI in the React admin
- Test/verify button
- Bidirectional event flow where applicable
- Documentation specific to that integration
- Auth token refresh, error retry, schema upgrade handling
- Surfaced in the integrator portal as a "supported integration"

### 29.3 Layer 3: Marketplace (deferred to v2)

Architecture supports marketplace as a non-breaking addition:

- Every first-party integration is built on the same public API third parties would use
- OAuth flow supports third-party developer registration
- Webhook system supports per-developer signing keys
- API rate limiting can be tier-differentiated for marketplace developers

---

## 30. Sales Motion

### 30.1 Hybrid PLG + Sales-led

| Path               | Customer profile                                                                             |
| ------------------ | -------------------------------------------------------------------------------------------- |
| **Self-serve PLG** | SMB direct customers + small integrators (Free / Starter tier signup, no sales touch)        |
| **Sales-led**      | Mid-market and enterprise customers + large integrators (SDR → AE → custom quote → contract) |

### 30.2 CRM and sales tools

- HubSpot for CRM and marketing automation
- Outreach for sales sequences
- Calendly or Chili Piper for demo scheduling
- PandaDoc for proposal/quote generation
- DocuSign for e-signatures

### 30.3 Sales targeting in v1

v1 focuses on greenfield customers (no existing VMS). Sales team is trained to qualify:

- New builds / new sites
- VMS-replacement happening anyway (not migrations of complex existing deployments — those wait for v1.x)
- Customers building first-time security systems

Customers with complex existing VMS deployments are tagged as v1.x leads and nurtured but not actively pursued in v1.

---

## 31. Error Handling, Resilience, Observability

### 31.1 Error handling philosophy

**Fail closed for security, fail open for recording.** Auth and permission failures fail closed (deny). Recording-related failures fail open (keep recording from cached state).

Every error returned to a customer includes:

- Stable error code (`auth.invalid_credentials`, `permission.denied`, `stream.token_expired`, etc.)
- Human-readable message (translated)
- Correlation ID for log lookup
- Optional actionable suggestion

Error codes are stable, public, documented, and never reused. New conditions get new codes.

### 31.2 Retry policies

| Operation                                       | Policy                                                                             |
| ----------------------------------------------- | ---------------------------------------------------------------------------------- |
| Connect-Go calls between Directory and Recorder | Exponential backoff, max 5 retries, circuit-break for 60s after persistent failure |
| JWKS fetch                                      | Exponential backoff, max 3 retries, fall back to cached keys                       |
| Camera connection retries                       | Unbounded with capped interval (~5 min between attempts)                           |
| Cert renewal                                    | Daily until 24h before expiry, then hourly                                         |
| Catalog sync                                    | Every 5 minutes (next interval is the retry)                                       |
| Cloud archive upload                            | Pause/resume, retry from last successful chunk                                     |
| Push notification delivery                      | Per FCM/APNs SLAs                                                                  |

### 31.3 Resilience map

| Failure                            | Local Recorder                             | Cloud                        | Customer experience                                                    |
| ---------------------------------- | ------------------------------------------ | ---------------------------- | ---------------------------------------------------------------------- |
| Recorder crashes and restarts      | Resumes recording from cache               | n/a                          | ~30s gap, no data loss                                                 |
| Recorder loses cloud connection    | Continues recording, queues state pushes   | Marks recorder degraded      | LAN viewing works with cached tokens; admin actions queued             |
| Cloud control plane partial outage | Recordings continue                        | Some cloud features degraded | Status page reflects                                                   |
| AI inference cloud failure         | Edge AI continues                          | Cloud AI unavailable         | Heavy AI features pause; basic features work                           |
| Cloudflare R2 outage               | Cloud archive uploads pause; queue locally | Live customers unaffected    | Archive resumes on R2 recovery                                         |
| Zitadel sidecar crash              | Existing tokens work for ~1 hour           | n/a                          | Supervisor restarts; brief login interruption                          |
| MediaMTX sidecar crash             | Streams interrupted                        | n/a                          | Supervisor restarts within seconds; existing recording state recovered |
| Federation peer offline            | Local site unaffected                      | n/a                          | Cross-site search marks partial; cross-site playback errors clearly    |
| Tier 3 cloud relay outage          | LAN-direct + Tier 2 still work             | n/a                          | Off-LAN customers fall back to other tiers automatically               |

The invariant: **recording never stops** as long as the Recorder has power and disk.

### 31.4 Observability stack

| Pillar             | Tech                                                                                                                                              |
| ------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------- |
| Structured logging | `slog` with consistent fields (`request_id`, `user_id`, `tenant_id`, `component`, `subsystem`); JSON output; sensitive-field redaction allow-list |
| Metrics            | Prometheus per component, customer can scrape from on-prem; cloud metrics in central Prometheus + Grafana                                         |
| Tracing            | OpenTelemetry, propagated via `traceparent` headers across Connect-Go calls; OTLP exporter, customer-configurable destinations                    |
| Dashboards         | Grafana for engineering; customer-facing dashboards in React admin (Health, Performance, Storage, Usage)                                          |

Default sampling for tracing: 100% of error traces, 1% of successful traces.

---

## 32. Testing Strategy

### 32.1 Test pyramid

| Layer                            | Frequency                | What                                                                                                     |
| -------------------------------- | ------------------------ | -------------------------------------------------------------------------------------------------------- |
| **Unit tests**                   | Per PR                   | Per-package logic. 70%+ coverage target, 85%+ for security packages                                      |
| **Component tests**              | Per PR                   | One component spinning up with mocked peers (Directory, Recorder, Gateway, Cloud control plane, etc.)    |
| **Integration tests**            | Per PR merge to main     | Multi-component Docker Compose with real Zitadel, MediaMTX, Connect-Go traffic                           |
| **End-to-end tests**             | Nightly                  | Full stack including React admin (Playwright), Flutter (`integration_test`), and the cloud control plane |
| **IdP integration tests**        | Weekly                   | Real Entra, Okta, Google test tenants                                                                    |
| **Federation chaos tests**       | Nightly                  | 3-Directory federation with random kill/partition/slowdown injection                                     |
| **Migration tests**              | Per migration-related PR | ~20 representative source-data scenarios                                                                 |
| **Security tests**               | Per PR                   | Token forgery, permission bypass, replay attacks, SSRF prevention, Casbin rule injection                 |
| **Load tests**                   | Weekly                   | Simulated 100 concurrent clients across 5 Recorders + soak test variant monthly                          |
| **Multi-tenant isolation tests** | Per PR                   | Verify no cross-tenant data leakage, especially in cloud control plane                                   |
| **Pre-launch security review**   | Once before launch       | Third-party pen test + security audit by Trail of Bits or similar                                        |

### 32.2 Cloud-specific testing

- **Multi-tenant chaos**: kill cloud services mid-request, verify graceful degradation
- **Cross-region readiness**: even though v1 is single-region, simulate region failover scenarios in tests to validate the multi-region-ready architecture
- **Stripe Connect testing**: full billing flows in Stripe test mode for both direct and via-integrator scenarios
- **GDPR data deletion testing**: verify right-to-erasure deletes all customer data across all systems

---

## 33. Cost and Timeline

### 33.1 v1 engineering effort breakdown

| Workstream                                                                                                                                                        | Engineer-weeks          |
| ----------------------------------------------------------------------------------------------------------------------------------------------------------------- | ----------------------- |
| On-prem Foundation (role split, networking, PKI, pairing)                                                                                                         | ~25                     |
| Identity (Zitadel multi-tenant, four protocols, six wizards)                                                                                                      | ~22                     |
| Camera Registry & Permissions (multi-tenant aware)                                                                                                                | ~25                     |
| Streaming Data Plane (URL minting, MediaMTX, multi-Recorder timeline, audio talkback)                                                                             | ~22                     |
| Federation (full scope with cloud-first adjustments)                                                                                                              | ~27                     |
| Remote Access (all three tiers in v1)                                                                                                                             | ~35                     |
| Recording Storage (4 tiers + 3 encryption + R2 + tier transitions)                                                                                                | ~20                     |
| Multi-tenant Cloud Control Plane (multi-tenant API, per-tenant routing, region-ready)                                                                             | ~70                     |
| Integrator Portal (fleet dashboard, customer onboarding, white-label, billing, support tools)                                                                     | ~50                     |
| React Admin Web App (two contexts, all pages)                                                                                                                     | ~30                     |
| Compliance Program (SOC 2 prep, FIPS, HIPAA, GDPR, EU AI Act, Section 508, pen test, bug bounty)                                                                  | ~35                     |
| AI/ML (11 features: object, face, LPR, behavioral, audio, CLIP search, cross-camera tracking, anomaly, summaries, forensic search, custom model upload + sandbox) | ~95                     |
| Hardware compatibility program                                                                                                                                    | ~12                     |
| Single-NVR self-migration tool + REST compat shim                                                                                                                 | ~15                     |
| White-label Level 3 (incl. mobile build pipeline)                                                                                                                 | ~30                     |
| Pricing & Billing (Stripe Connect + tax compliance + tier management)                                                                                             | ~30                     |
| Integrations ecosystem (12 first-party + webhooks + API + SDKs + dev docs)                                                                                        | ~65                     |
| Flutter end-user app (single codebase → 6 targets, federated, all features)                                                                                       | ~50                     |
| Video Wall Client (Qt 6 / C++ native)                                                                                                                             | ~80                     |
| Marketing website (Next.js + Sanity + comprehensive scope)                                                                                                        | ~30                     |
| Documentation portal (Mintlify + comprehensive scope)                                                                                                             | ~40                     |
| Status page (Comprehensive — per-region, per-integrator white-label)                                                                                              | ~15                     |
| Customer support tooling (Comprehensive — AI assistant, screen-sharing, impersonation, remote diagnostics)                                                        | ~30                     |
| Sales motion infrastructure (HubSpot integration, lead routing, demo scheduling, proposal automation)                                                             | ~10                     |
| Notification infrastructure (Comprehensive — voice, WhatsApp, ML suppression, custom templates)                                                                   | ~40                     |
| Onboarding experience (sandbox, wizard, drip, in-app guidance)                                                                                                    | ~25                     |
| Cross-cutting (observability, structured logging, metrics, tracing, error codes, integration test harness, E2E test rig, federation chaos rig, load test rig)     | ~50                     |
| **v1 total**                                                                                                                                                      | **~975 engineer-weeks** |

### 33.2 Team and capital

- **Engineering team**: ~22-28 senior engineers across backend (Go), mobile (Flutter), frontend (React), cloud/SRE, ML/AI, video wall (C++/Qt), marketing site (Next.js), DevOps
- **Plus**: Head of Security & Compliance + 2 customer success + 2 marketing + 1 PM + 1 designer + 2 technical writers + 1 hardware ops + 3-5 sales = **~50 people total org**
- **Calendar time to feature-complete v1**: ~18-24 months focused work + 3-6 months pre-launch buffer
- **Capital commitment to v1 launch**: ~$15-25M
- **Post-launch ongoing burn**: ~$10-20M/year

### 33.3 Recommended phasing

Even with no compromises on scope, the work has natural phases that ship internally for validation:

| Phase                       | Months                | Content                                                                                                                   | Customer visibility                                    |
| --------------------------- | --------------------- | ------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------ |
| **A — Foundation**          | 1-4                   | On-prem role split + cloud control plane skeleton + identity (multi-tenant) + camera registry + streaming data plane      | Internal dogfooding                                    |
| **B — Platform**            | 3-7 (parallel with A) | AI/ML, recording storage with cloud archive, federation, remote access tiers                                              | Internal dogfooding                                    |
| **C — Integrator + UX**     | 6-10                  | Integrator portal, white-label including mobile build pipeline, React admin polish, Flutter app, sandbox mode             | Internal closed beta                                   |
| **D — Compliance + Polish** | 9-13                  | Compliance program completion, security review, documentation, marketing site, video wall                                 | **Private beta** with select integrators and customers |
| **E — GA Launch**           | 13-18                 | Migration tooling for existing customers, sales enablement, support team scaling, public marketing launch                 | **Public v1 launch**                                   |
| **F — v1.x**                | 18-24+                | Migration from competitors, reference appliance, marketplace MVP, FedRAMP if customer demand, additional language support | Continuous releases                                    |

---

## 34. Open Questions

| #   | Question                                      | Default                                                       | Decision by       |
| --- | --------------------------------------------- | ------------------------------------------------------------- | ----------------- |
| 1   | Founder Directory loss recovery in federation | Manual re-establishment in v1, quorum in v2                   | Spec sign-off     |
| 2   | Cross-Directory audit log replication         | Both ends log                                                 | Spec sign-off     |
| 3   | Time zone display in cross-Directory search   | Operator's local time, tooltip with camera-local              | Spec sign-off     |
| 4   | Catalog sync interval default in federation   | 5 minutes, configurable down to 30s                           | Spec sign-off     |
| 5   | Retention policies per-camera or per-Recorder | Per-camera                                                    | Spec sign-off     |
| 6   | Active/passive Recorder failover              | No in v1, defer to v2                                         | Spec sign-off     |
| 7   | Time-of-day permission restrictions           | No in v1                                                      | Spec sign-off     |
| 8   | Old binary retention after migration          | Until customer runs explicit cleanup                          | Spec sign-off     |
| 9   | Heavily-customized install migration          | Refuse with explicit acknowledgment                           | Spec sign-off     |
| 10  | Multi-region rollout timing for v1.x          | Based on customer geography demand                            | Year-1 review     |
| 11  | First-party hardware appliance v2 commitment  | Decide based on v1.x reference appliance demand               | Year-1 review     |
| 12  | Marketplace launch timing                     | v2, after PMF validation                                      | Year-1 review     |
| 13  | LDAP/AD password storage migration            | Discuss with first enterprise customer migrating from AD-only | Customer-specific |

---

## 35. Risks

Ranked by impact:

1. **Total scope is the largest single risk.** ~975 engineer-weeks across many disciplines is a Series-A-scale company commitment. Failure modes: team can't be hired fast enough; cross-team dependencies cause sequential bottlenecks; scope creep adds to an already-large plan.
2. **Multi-tenant cloud reliability.** The first multi-tenant cloud bug that causes cross-tenant data leakage is a company-defining incident. Mitigation: rigorous multi-tenant isolation testing, security review specifically focused on tenant boundaries, public bug bounty.
3. **Federation timeline.** ~27 weeks of complex distributed-systems work with hard partial-failure semantics. Mitigation: chaos test rig from day one, build incrementally, validate with real multi-site customer.
4. **AI feature quality at launch.** 11 AI feature categories at v1 launch is ambitious. Cross-camera tracking and anomaly detection are research-y. Mitigation: ship them as "beta" labels, plan for 2-3 release cycles of tuning post-launch.
5. **Mobile app build pipeline complexity.** Per-integrator iOS and Android builds are operationally complex (Apple review, Google review, signing, etc.). Mitigation: start the build pipeline early (month 3-4 of development), test with internal integrator simulations, allow extra calendar buffer.
6. **Hardware compatibility certification scope.** Even "software only" requires testing against many hardware configurations. Mitigation: establish hardware certification lab early, define a clear cert criteria, publish list with confidence.
7. **Compliance certification calendar dependencies.** SOC 2 Type II requires 6+ months of evidence collection. ISO 27001 requires similar. Mitigation: start compliance program at month 1 of v1 dev so the audits land on time.
8. **Stripe Connect complexity.** Marketplace facilitator status involves KYC, 1099 reporting, dispute handling. Mitigation: hire a billing engineer with Stripe Connect experience, plan for dedicated billing testing.
9. **Sales team scaling.** Industry-leading product needs industry-leading sales team. ~50 people total org includes only 3-5 sales hires; scaling beyond that is critical post-launch. Mitigation: plan sales team growth in v1.x.
10. **Customer support load at launch.** Comprehensive support tooling helps but doesn't eliminate the need for human support agents. Mitigation: hire support team well before launch, train on the product, simulate launch-day load.

---

## 36. What's Deferred from v1

| Feature                                                                                                 | When                                                             |
| ------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------- |
| Migration from competitors (Verkada, Eagle Eye, Milestone, Genetec, Avigilon, Exacq, Hanwha, Hikvision) | v1.x                                                             |
| Reference appliance (single SKU)                                                                        | v1.x                                                             |
| Full hardware program (4 SKUs, co-branded, drop-ship)                                                   | v2                                                               |
| Java + C# SDKs                                                                                          | v1.x                                                             |
| Marketplace for third-party integrations                                                                | v2                                                               |
| Marketplace for custom AI models                                                                        | v2                                                               |
| Multi-region active-active deployment (eu-west, ap-southeast)                                           | v1.x based on customer geography                                 |
| Customer-deployable private cloud (Helm charts for customer's own AWS/GCP/Azure)                        | v2                                                               |
| FedRAMP Moderate certification                                                                          | When first federal customer signs                                |
| Multiple federations per Directory                                                                      | v2                                                               |
| Quorum-based federation founder failover                                                                | v2                                                               |
| Active/passive Recorder failover                                                                        | v2                                                               |
| Time-of-day permission restrictions                                                                     | v2                                                               |
| Apple Watch / Siri / CarPlay / AR features                                                              | Cut as vanity                                                    |
| Real-time talkback translation                                                                          | Cut as vanity                                                    |
| 12+ language support (sticking with EN/ES/FR/DE in v1)                                                  | v1.x based on customer geography                                 |
| Quote-to-cash automation                                                                                | v1.x                                                             |
| Customer-managed AI model marketplace                                                                   | v2                                                               |
| Voice / WhatsApp notification (in scope but lower priority than core push/email/SMS)                    | Comprehensive notification infrastructure includes these from v1 |
| AI-driven predictive analytics                                                                          | v2                                                               |

---

## 37. Glossary

| Term                         | Meaning                                                                                                                                                                         |
| ---------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **Cloud control plane**      | The multi-tenant SaaS that serves as the system of record for cloud-managed customers. Hosts identity, camera registry, integrator portal, AI inference, and recording archive. |
| **On-prem Directory**        | The Directory subsystem running in the on-prem binary at a customer site. Owns local identity (in air-gapped mode), local camera registry, and local control plane.             |
| **Recorder**                 | The Recorder subsystem in the on-prem binary that captures cameras, records to local disk, and serves video.                                                                    |
| **Gateway**                  | The Gateway subsystem (co-resident with Directory in v1) that proxies streams for off-LAN clients.                                                                              |
| **Tenant / Customer Tenant** | A customer organization in the multi-tenant cloud. Owns cameras, users, recordings.                                                                                             |
| **Integrator**               | A company (security installer, MSP, VAR) that resells/manages security systems for customer tenants.                                                                            |
| **Sub-Reseller**             | A child organization of an Integrator (e.g., a regional office).                                                                                                                |
| **Cloud-managed customer**   | A customer whose Directory is in the cloud control plane. The on-prem Recorders communicate with the cloud.                                                                     |
| **Air-gapped customer**      | A customer whose Directory runs entirely on-prem with no cloud connection.                                                                                                      |
| **Hybrid customer**          | A customer who uses both cloud and on-prem features, with on-prem Directory caching cloud state.                                                                                |
| **Federation**               | A trust relationship between Directories that lets users at one site browse and view cameras at peer sites.                                                                     |
| **White-label Level 3**      | Full brand replacement: per-integrator custom domain, email, mobile app builds, content overrides.                                                                              |
| **Stream token**             | Short-lived (~5 min) JWT issued by the Directory for one specific stream session. Verified at the Recorder/Gateway.                                                             |
| **Cluster CA**               | Per-site PKI managed by embedded `step-ca`, used for Directory ↔ Recorder mTLS.                                                                                                |
| **Federation cluster CA**    | Separate PKI for Directory ↔ Directory mTLS in air-gapped federations.                                                                                                         |
| **CSE-CMK**                  | Client-side encryption with customer-managed keys. The strongest encryption mode for cloud archive.                                                                             |
| **Sandbox tenant**           | An ephemeral tenant with simulated cameras for prospect/integrator exploration before committing real hardware.                                                                 |
| **PLG**                      | Product-Led Growth — self-serve signup and adoption without sales touch.                                                                                                        |

---

_End of design document. This is a complete rewrite of the original spec. The next steps are to throw away the existing 100 Linear tickets and rebuild the project structure to match this design — likely 12-15 projects with 250-350 tickets total._
