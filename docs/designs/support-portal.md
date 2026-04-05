# Support Portal - Design Document

**Ticket:** KAI-108
**Status:** Design
**Author:** Commercial Platform Team
**Date:** 2026-04-03

---

## 1. Overview

The Support Portal provides a structured support experience for MediaMTX NVR customers and white-label partners. It includes a ticket system for issue tracking, remote diagnostics for efficient troubleshooting, a searchable knowledge base for self-service, and SLA-tiered support levels.

## 2. Goals

- Ticket system for structured issue reporting, tracking, and resolution
- Remote diagnostics that allow support staff to gather system info without requiring customer expertise
- Self-service knowledge base to reduce support ticket volume
- SLA tiers aligned with licensing tiers (KAI-106) with measurable response and resolution targets
- Integration with the Cloud Management Portal (KAI-103) for contextual support

## 3. Ticket System

### 3.1 Data Model

```
Ticket
  |-- id (UUID)
  |-- ticket_number (human-readable: SUP-000001)
  |-- org_id (FK, nullable for direct customers)
  |-- partner_id (FK, nullable for white-label partners)
  |-- reporter_id (FK, user who created the ticket)
  |-- assignee_id (FK, support agent, nullable)
  |-- server_id (FK, associated NVR server, nullable)
  |
  |-- subject (text, max 200 chars)
  |-- description (text, markdown)
  |-- category (see 3.2)
  |-- priority (low | medium | high | critical)
  |-- status (open | in_progress | waiting_customer | waiting_internal | resolved | closed)
  |
  |-- sla_tier (starter | professional | enterprise)
  |-- sla_response_due_at (timestamp)
  |-- sla_resolution_due_at (timestamp)
  |-- sla_response_met (bool, nullable)
  |-- sla_resolution_met (bool, nullable)
  |
  |-- created_at
  |-- updated_at
  |-- resolved_at
  |-- closed_at
  |
  +-- Comments[]
  |     |-- id, ticket_id, author_id
  |     |-- body (markdown)
  |     |-- internal (bool, hidden from customer)
  |     |-- created_at
  |
  +-- Attachments[]
  |     |-- id, ticket_id
  |     |-- filename, content_type, size_bytes
  |     |-- storage_path (S3 key)
  |     |-- uploaded_by, uploaded_at
  |
  +-- DiagnosticReports[]
        |-- id, ticket_id, server_id
        |-- collected_at
        |-- report_data (JSON)
        |-- storage_path (S3 key for full bundle)
```

### 3.2 Ticket Categories

| Category | Description | Examples |
|----------|-------------|----------|
| `camera_issue` | Camera connectivity, quality, configuration | Camera offline, poor image quality |
| `recording` | Recording failures, gaps, playback | Missing recordings, corrupt files |
| `storage` | Disk space, retention, performance | Disk full, slow write speed |
| `network` | Network configuration, connectivity | RTSP timeout, WebRTC failure |
| `authentication` | Login, permissions, certificates | Cannot log in, permission denied |
| `installation` | Setup, upgrade, migration | Install failure, upgrade error |
| `performance` | CPU, memory, system responsiveness | High CPU, slow UI |
| `integration` | ONVIF, API, third-party | ONVIF discovery fails, API error |
| `feature_request` | Enhancement requests | New feature suggestion |
| `billing` | License, subscription, invoicing | License expired, billing question |

### 3.3 Ticket Lifecycle

```
                    +----------+
          create -->|   Open   |
                    +----------+
                         |
                    assign/reply
                         |
                    +----------+
                    | In Prog. |<-----+
                    +----------+      |
                     /        \       |
           ask customer    need internal
                /                \    |
   +------------------+   +------------------+
   | Waiting Customer |   | Waiting Internal |
   +------------------+   +------------------+
          |                       |
      customer replies        internal resolved
          |                       |
          +-----------+-----------+
                      |
                 +----------+
                 | Resolved |
                 +----------+
                      |
              customer confirms
              (or auto-close 7d)
                      |
                 +----------+
                 |  Closed  |
                 +----------+
```

### 3.4 Ticket API

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/v1/support/tickets` | Create a ticket |
| GET | `/v1/support/tickets` | List tickets (filtered by org, status, etc.) |
| GET | `/v1/support/tickets/:id` | Get ticket detail |
| PATCH | `/v1/support/tickets/:id` | Update ticket (status, priority, assignee) |
| POST | `/v1/support/tickets/:id/comments` | Add a comment |
| POST | `/v1/support/tickets/:id/attachments` | Upload an attachment |
| POST | `/v1/support/tickets/:id/diagnose` | Trigger remote diagnostics |

## 4. Remote Diagnostics

### 4.1 Purpose

Remote diagnostics allow support agents to collect detailed system information from a customer's NVR server without requiring the customer to manually gather logs or run commands.

### 4.2 Diagnostic Collection Flow

```
Support Agent                Cloud Portal              NVR Server
     |                            |                         |
     |-- Trigger diagnostic ----->|                         |
     |                            |-- command: diagnose --->|
     |                            |                         |-- collect data
     |                            |<-- diagnostic bundle ---|
     |                            |-- store in S3           |
     |<-- diagnostic report ------|                         |
```

1. Support agent clicks "Collect Diagnostics" on a ticket linked to a server.
2. Cloud portal sends a `diagnostics.collect` command to the server via WebSocket (KAI-103 protocol).
3. NVR server collects diagnostic data (see 4.3) and bundles it into a compressed archive.
4. Bundle is uploaded to the cloud and attached to the ticket.
5. Support agent reviews the diagnostic report.

### 4.3 Diagnostic Data Collected

| Category | Data | Sensitive? |
|----------|------|-----------|
| **System** | OS version, kernel, architecture, uptime | No |
| | CPU model, core count, current usage | No |
| | Memory total, used, available | No |
| | Disk partitions, usage, filesystem types | No |
| | Network interfaces, IPs (anonymizable) | Low |
| **NVR** | MediaMTX version, build info | No |
| | Configuration (with secrets redacted) | Low |
| | Camera list with connection status | No |
| | Recording status and storage stats | No |
| | License info | No |
| **Logs** | Last 1000 lines of NVR log | Medium |
| | Last 500 lines of system journal (NVR service) | Medium |
| **Network** | Connectivity test results (DNS, NTP, RTSP per camera) | Low |
| | Latency measurements to cameras | No |
| **Database** | SQLite integrity check result | No |
| | Table row counts (no actual data) | No |
| | Database file size | No |

### 4.4 Privacy and Security

- **Secret redaction:** Passwords, JWT secrets, API keys, and certificate private keys are replaced with `[REDACTED]` before transmission.
- **IP anonymization:** Internal IP addresses can optionally be anonymized (configurable by customer).
- **Customer consent:** Diagnostics require explicit opt-in. The NVR server prompts for consent if auto-diagnostics are not enabled.
- **Data retention:** Diagnostic bundles are retained for the lifetime of the ticket plus 30 days, then purged.
- **Encryption:** Bundles are encrypted in transit (TLS) and at rest (S3 SSE).

### 4.5 Standalone Mode

For NVR servers not connected to the cloud:

- The local admin console has a "Generate Diagnostic Report" button.
- The report is downloaded as a `.tar.gz` file.
- The customer can attach it to a support ticket manually (via portal upload or email).

## 5. Knowledge Base

### 5.1 Content Structure

```
KnowledgeBase
  |
  +-- Categories[]
  |     |-- id, name, slug, description, sort_order
  |     |-- Examples: "Getting Started", "Camera Setup", "Troubleshooting",
  |     |   "Storage Management", "Network Configuration", "API Reference"
  |
  +-- Articles[]
        |-- id (UUID)
        |-- category_id (FK)
        |-- title
        |-- slug (URL-friendly)
        |-- body (markdown)
        |-- tags[] (text array)
        |-- visibility (public | authenticated | internal)
        |-- status (draft | published | archived)
        |-- author_id
        |-- created_at, updated_at, published_at
        |-- view_count
        |-- helpful_yes, helpful_no (feedback counters)
```

### 5.2 Content Types

| Type | Description | Example |
|------|-------------|---------|
| **How-To Guide** | Step-by-step instructions | "How to add an ONVIF camera" |
| **Troubleshooting** | Problem/cause/solution format | "Camera shows offline but is powered on" |
| **FAQ** | Common questions | "What ports does MediaMTX use?" |
| **Reference** | Technical specifications | "Supported ONVIF profiles" |
| **Release Notes** | Version changelog | "What's new in v1.5.0" |
| **Video Tutorial** | Embedded video with transcript | "Initial NVR setup walkthrough" |

### 5.3 Search

- Full-text search powered by PostgreSQL `tsvector` (cloud) or SQLite FTS5 (embedded).
- Search covers title, body, and tags.
- Results are ranked by relevance with category boosting (troubleshooting articles rank higher when searched from a ticket context).

### 5.4 Self-Service Integration

- When a user creates a ticket, the system suggests relevant knowledge base articles based on the subject and category.
- If the user finds their answer in a suggested article, the ticket is auto-closed as "resolved via knowledge base."
- Article helpfulness feedback ("Was this helpful? Yes / No") drives content improvement.

### 5.5 Knowledge Base API

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/v1/support/kb/categories` | List categories |
| GET | `/v1/support/kb/articles` | List/search articles |
| GET | `/v1/support/kb/articles/:slug` | Get article by slug |
| POST | `/v1/support/kb/articles/:id/feedback` | Submit helpfulness feedback |

### 5.6 White-Label Knowledge Base

- Partners (KAI-106) can maintain a branded knowledge base with their own articles.
- MediaMTX core articles are available as "inherited" content that partners can include, exclude, or override.
- Partner articles use the partner's branding (logo, colors).

## 6. SLA Tiers

### 6.1 SLA Definitions

| Metric | Starter | Professional | Enterprise |
|--------|---------|-------------|------------|
| **Response time** (first reply) | 48 business hours | 24 hours (calendar) | 4 hours (calendar) |
| **Resolution target** | Best effort | 5 business days | 2 business days |
| **Availability** | Business hours (M-F 9-5) | Business hours + extended (M-F 8-8) | 24/7/365 |
| **Channels** | Email, portal | Email, portal, chat | Email, portal, chat, phone |
| **Named contacts** | 1 | 3 | Unlimited |
| **Remote diagnostics** | Manual upload only | Cloud-assisted | Cloud-assisted + proactive monitoring |
| **Escalation path** | Standard | Priority queue | Dedicated support engineer |

### 6.2 SLA Calculation

- **Response SLA:** Measured from ticket creation to first non-automated reply by a support agent.
- **Resolution SLA:** Measured from ticket creation to status change to `resolved`.
- **Clock pausing:** SLA clock pauses when ticket is in `waiting_customer` status.
- **Business hours:** SLA calculations respect the tier's availability window and exclude non-working periods for starter/professional tiers.

### 6.3 SLA Breach Handling

| Threshold | Action |
|-----------|--------|
| 75% of response SLA elapsed | Warning notification to assigned agent |
| 100% of response SLA elapsed | Escalation to team lead; ticket flagged as "SLA breached" |
| 75% of resolution SLA elapsed | Warning to agent and team lead |
| 100% of resolution SLA elapsed | Escalation to support manager; customer notification |

### 6.4 SLA Reporting

- Monthly SLA compliance report per organization (included in Analytics, KAI-107).
- Metrics tracked: % tickets meeting response SLA, % meeting resolution SLA, average response time, average resolution time.
- Enterprise customers receive a quarterly service review with SLA performance data.

## 7. Support Portal UI

### 7.1 Customer View

```
+-----------------------------------------------------------------------+
|  [Logo]  Support Center                          [User v]  [Tickets]  |
+-----------------------------------------------------------------------+
|                                                                       |
|  Search Knowledge Base                                                |
|  [_________________________________________________] [Search]        |
|                                                                       |
|  Popular Articles                          Quick Actions              |
|  - How to add a camera                     [+ New Ticket]            |
|  - Troubleshooting offline cameras         [View My Tickets]         |
|  - Storage management guide                [System Status]           |
|  - Network configuration                                             |
|                                                                       |
|  My Recent Tickets                                                    |
|  +--------+--------------------------+----------+----------+         |
|  | SUP-42 | Camera offline after upd | High     | In Prog. |         |
|  | SUP-38 | Storage forecast quest.  | Low      | Resolved |         |
|  +--------+--------------------------+----------+----------+         |
+-----------------------------------------------------------------------+
```

### 7.2 Agent View

```
+-----------------------------------------------------------------------+
|  [Logo]  Support Console                    [Queue: 12] [My: 5]      |
+-----------------------------------------------------------------------+
|  Filters: [All] [My Tickets] [Unassigned] [SLA At Risk]              |
|                                                                       |
|  Ticket Queue                                                         |
|  +--------+-----+---------------------------+--------+------+-----+  |
|  | Ticket | SLA | Subject                   | Org    | Prio | Age |  |
|  +--------+-----+---------------------------+--------+------+-----+  |
|  | SUP-45 | !!! | Server unreachable        | Acme   | Crit | 2h  |  |
|  | SUP-44 | !!  | Recording gaps on cam-3   | Beta   | High | 5h  |  |
|  | SUP-43 | OK  | License activation help   | Gamma  | Low  | 1d  |  |
|  +--------+-----+---------------------------+--------+------+-----+  |
|                                                                       |
|  [Ticket Detail Panel]                                                |
|  Subject: Server unreachable                                          |
|  Org: Acme Corp | Server: HQ-NVR-01 | SLA: Enterprise (4h resp.)    |
|  [Reply] [Internal Note] [Diagnose] [Escalate] [Resolve]            |
|                                                                       |
|  Server Info (auto-populated from cloud portal):                      |
|  Version: 1.4.2 | Cameras: 32 | Last heartbeat: 2h ago              |
+-----------------------------------------------------------------------+
```

## 8. Integration Points

| System | Integration |
|--------|-------------|
| Cloud Management Portal (KAI-103) | Server context auto-populated in tickets; diagnostic commands sent via portal |
| Push Notifications (KAI-105) | Ticket updates delivered as push notifications to mobile app |
| Analytics/Reporting (KAI-107) | SLA compliance metrics feed into monthly reports |
| White-Label (KAI-106) | Branded support portal; partner escalation workflows |
| Email | Ticket creation via email; reply-by-email for comments |

## 9. Email Integration

- **Inbound:** Customers can create tickets by emailing `support@{domain}`. Emails are parsed (subject -> ticket subject, body -> description, attachments -> ticket attachments).
- **Outbound:** Ticket updates (new comments, status changes) are emailed to the reporter and any CC'd addresses.
- **Reply-by-email:** Customers can reply to notification emails; replies are parsed and added as comments.
- **Email threading:** Uses `In-Reply-To` and `References` headers for proper threading in email clients.

## 10. Open Questions

- Should we build the ticket system in-house or integrate with an existing platform (Zendesk, Freshdesk, etc.) via API?
- Live chat: should it be built into the portal or use a third-party widget (Intercom, Crisp)?
- Should enterprise customers have direct Slack/Teams channel support?
- How should partner-escalated tickets be handled -- separate queue or merged with direct customer tickets?
- Should the knowledge base support community contributions (customer-authored articles with moderation)?
