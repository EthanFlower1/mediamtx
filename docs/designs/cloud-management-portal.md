# Cloud Management Portal - Design Document

**Ticket:** KAI-103
**Status:** Design
**Author:** Commercial Platform Team
**Date:** 2026-04-03

---

## 1. Overview

The Cloud Management Portal provides a centralized web dashboard for managing multiple MediaMTX NVR servers from a single cloud-hosted interface. It enables fleet management, remote configuration, health monitoring, and user administration across all deployed NVR instances.

## 2. Goals

- Centralized management of all NVR servers from a single pane of glass
- Multi-tenant architecture supporting distinct organizations and their server fleets
- Secure server-to-cloud communication with minimal bandwidth overhead
- Real-time health dashboards and alerting
- Role-based access for organization admins, operators, and viewers

## 3. Architecture

### 3.1 High-Level Components

```
+-------------------+       +------------------------+       +------------------+
|  NVR Server(s)    | <---> |  Cloud Management API  | <---> |  Portal Web UI   |
|  (on-premise)     |       |  (cloud-hosted)        |       |  (React SPA)     |
+-------------------+       +------------------------+       +------------------+
                                      |
                            +-------------------+
                            |  PostgreSQL / RDS  |
                            |  (cloud DB)        |
                            +-------------------+
```

### 3.2 Multi-Tenant Data Model

```
Organization
  |-- id (UUID)
  |-- name
  |-- plan_tier (free | pro | enterprise)
  |-- created_at
  |
  +-- Users[]
  |     |-- id, email, role (owner | admin | operator | viewer)
  |     |-- org_id (FK)
  |
  +-- Servers[]
        |-- id (UUID)
        |-- org_id (FK)
        |-- name, location_label
        |-- registration_token (one-time)
        |-- last_heartbeat_at
        |-- status (online | offline | degraded)
        |-- version, os, camera_count
```

- **Tenant isolation:** All queries are scoped by `org_id`. Row-level security (RLS) is enforced at the database layer in addition to application-level middleware.
- **Plan tiers** govern server count limits, user seats, and feature availability.

### 3.3 Server-Cloud Protocol

NVR servers communicate with the cloud portal via an outbound-only WebSocket connection, ensuring no inbound firewall rules are required on the customer network.

#### 3.3.1 Registration Flow

1. Admin generates a registration token in the portal.
2. Token is entered on the NVR server's local admin console.
3. NVR opens a TLS WebSocket to `wss://portal.example.com/v1/ws/register` with the token.
4. Cloud validates the token, issues a persistent server credential (Ed25519 keypair exchange).
5. Token is marked as consumed; server stores its credential locally.

#### 3.3.2 Heartbeat and Telemetry

- **Heartbeat interval:** Every 30 seconds via WebSocket ping/pong.
- **Telemetry push:** Every 5 minutes, the server sends a lightweight JSON payload:

```json
{
  "server_id": "uuid",
  "timestamp": "2026-04-03T12:00:00Z",
  "cpu_percent": 34.2,
  "memory_percent": 61.0,
  "disk_used_gb": 120.5,
  "disk_total_gb": 500.0,
  "camera_count": 16,
  "cameras_recording": 16,
  "cameras_errored": 0,
  "uptime_seconds": 864000,
  "version": "1.4.2"
}
```

#### 3.3.3 Command Channel

The cloud can send commands to servers via the WebSocket:

| Command | Description |
|---------|-------------|
| `config.get` | Retrieve current configuration |
| `config.update` | Push configuration changes |
| `server.restart` | Restart the NVR process |
| `server.upgrade` | Trigger OTA firmware/binary update |
| `camera.snapshot` | Request a live snapshot from a camera |
| `diagnostics.collect` | Gather logs and system info |

All commands use a request-response pattern with a correlation ID and 30-second timeout.

### 3.4 Cloud Infrastructure

| Component | Technology |
|-----------|------------|
| API server | Go (Gin) containerized, auto-scaling |
| Database | PostgreSQL (AWS RDS / GCP Cloud SQL) |
| WebSocket gateway | Dedicated Go service behind ALB |
| Object storage | S3-compatible for snapshots, reports |
| Auth | OAuth 2.0 + OIDC (Auth0 or Cognito) |
| CDN | CloudFront for static portal assets |

## 4. Dashboard Wireframes

### 4.1 Fleet Overview (Home)

```
+-----------------------------------------------------------------------+
|  [Logo]  Fleet Overview         [Org: Acme Corp v]  [User v]  [?]    |
+-----------------------------------------------------------------------+
|                                                                       |
|  +------------------+  +------------------+  +------------------+     |
|  | Servers Online   |  | Total Cameras    |  | Alerts (24h)     |     |
|  |      12 / 14     |  |      187         |  |       3          |     |
|  +------------------+  +------------------+  +------------------+     |
|                                                                       |
|  Server List                                              [+ Add]     |
|  +-------+------------------+--------+----------+--------+--------+  |
|  | Status| Name             | Site   | Cameras  | CPU    | Disk   |  |
|  +-------+------------------+--------+----------+--------+--------+  |
|  |  *    | HQ-NVR-01        | HQ     | 32/32    | 45%    | 62%    |  |
|  |  *    | Warehouse-NVR    | WH-A   | 16/16    | 23%    | 41%    |  |
|  |  !    | Parking-NVR      | Lot-B  | 8/12     | 78%    | 89%    |  |
|  |  x    | Branch-Office    | BR-1   | --       | --     | --     |  |
|  +-------+------------------+--------+----------+--------+--------+  |
|                                                                       |
|  [CPU Trend Chart - 24h]          [Disk Usage Chart - 7d]            |
+-----------------------------------------------------------------------+
```

### 4.2 Server Detail View

```
+-----------------------------------------------------------------------+
|  < Back   HQ-NVR-01                          [Configure] [Restart]   |
+-----------------------------------------------------------------------+
|                                                                       |
|  Status: Online    Version: 1.4.2    Uptime: 10d 4h                  |
|  Location: Headquarters    OS: Linux amd64                            |
|                                                                       |
|  Tabs: [Cameras] [Health] [Config] [Logs] [Alerts]                   |
|                                                                       |
|  Camera Grid (4x4 thumbnails with name + status overlay)             |
|  +--------+ +--------+ +--------+ +--------+                         |
|  | Cam-01 | | Cam-02 | | Cam-03 | | Cam-04 |                         |
|  | OK     | | OK     | | ERR    | | OK     |                         |
|  +--------+ +--------+ +--------+ +--------+                         |
|  ...                                                                  |
+-----------------------------------------------------------------------+
```

### 4.3 Organization Settings

```
+-----------------------------------------------------------------------+
|  Organization Settings                                                |
+-----------------------------------------------------------------------+
|  General | Users | Billing | API Keys | SSO                          |
|                                                                       |
|  Users                                               [+ Invite]      |
|  +------------------+-----------------------+--------+--------+      |
|  | Name             | Email                 | Role   | Action |      |
|  +------------------+-----------------------+--------+--------+      |
|  | Jane Smith       | jane@acme.com         | Owner  |  ...   |      |
|  | Bob Jones        | bob@acme.com          | Admin  |  ...   |      |
|  | Alice Lee        | alice@acme.com        | Viewer |  ...   |      |
|  +------------------+-----------------------+--------+--------+      |
+-----------------------------------------------------------------------+
```

## 5. Security

### 5.1 Authentication and Authorization

- **Portal users:** OAuth 2.0 / OIDC with MFA enforcement for admin roles.
- **Server credentials:** Ed25519 keypair provisioned during registration. Server signs each WebSocket frame with its private key; cloud verifies with stored public key.
- **API keys:** Scoped per-organization for programmatic access (CI/CD, scripts). Keys are hashed (SHA-256) at rest.

### 5.2 Data Protection

- **In transit:** TLS 1.3 for all connections (HTTPS, WSS).
- **At rest:** AES-256 encryption for database (RDS encryption), S3 server-side encryption.
- **Tenant isolation:** Application-level org_id scoping + database RLS. No cross-tenant data leakage by design.
- **PII handling:** User emails and names are the only PII stored. Telemetry contains no video data.

### 5.3 Network Security

- NVR servers initiate all connections outbound (no inbound ports required).
- WebSocket gateway validates server identity on every reconnection.
- Rate limiting: 100 API requests/min per user, 10 commands/min per server.
- IP allowlisting available for enterprise tier.

### 5.4 Audit Logging

- All portal actions (user CRUD, config changes, commands sent) are logged with actor, timestamp, and org context.
- Logs are immutable (append-only) and retained for 1 year.
- Enterprise tier supports SIEM export via webhook.

## 6. API Surface

### 6.1 REST API (Portal)

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/v1/servers` | List servers for current org |
| GET | `/v1/servers/:id` | Server detail + telemetry |
| POST | `/v1/servers/register` | Generate registration token |
| POST | `/v1/servers/:id/command` | Send command to server |
| GET | `/v1/org` | Current org details |
| PATCH | `/v1/org` | Update org settings |
| GET | `/v1/org/users` | List org users |
| POST | `/v1/org/users/invite` | Invite user to org |

### 6.2 WebSocket API (Server-Cloud)

| Direction | Message Type | Payload |
|-----------|-------------|---------|
| Server -> Cloud | `heartbeat` | Telemetry JSON |
| Server -> Cloud | `event` | Alert, camera status change |
| Cloud -> Server | `command` | Command + correlation ID |
| Server -> Cloud | `command_response` | Result + correlation ID |

## 7. Deployment and Scaling

- **Stateless API servers** behind a load balancer, horizontally scalable.
- **WebSocket gateway** uses sticky sessions (server affinity by server_id). Scaled by adding gateway nodes; each node handles up to 10,000 concurrent server connections.
- **Database:** Single-writer PostgreSQL with read replicas for dashboard queries.
- **Caching:** Redis for session state, server online/offline status, and rate limiting counters.

## 8. Open Questions

- Should the portal support direct video playback proxied through the cloud, or only link back to the local server?
- SSO federation (SAML 2.0) for enterprise customers: build vs. integrate with Auth0/Okta?
- Pricing model for plan tiers (server count limits, feature gates).
