# Integrator Deployment Guide

This guide covers network planning, firewall configuration, bandwidth sizing, multi-site architecture, and site survey preparation for deploying MediaMTX NVR in production environments.

---

## Table of Contents

1. [Network Topology](#network-topology)
2. [Firewall and Port Rules](#firewall-and-port-rules)
3. [Bandwidth Planning](#bandwidth-planning)
4. [Multi-Site Deployment Patterns](#multi-site-deployment-patterns)
5. [Site Survey Checklist](#site-survey-checklist)

---

## Network Topology

### Single-Site Reference Architecture

```
                         +-----------+
                         |  Internet |
                         +-----+-----+
                               |
                         +-----+-----+
                         |  Firewall |
                         +-----+-----+
                               |
               +---------------+---------------+
               |         Management LAN         |
               |        (e.g. 10.0.1.0/24)      |
               +---+-------+-------+--------+---+
                   |       |       |        |
              +----+--+ +--+---+ +-+------+ |
              | Admin | | NVR  | | ONVIF  | |
              |  PC   | |Server| |Cameras | |
              +-------+ +------+ +--------+ |
                                             |
                                    +--------+--------+
                                    |   Camera VLAN   |
                                    | (e.g. 10.0.2.0/24) |
                                    +--+-----+-----+--+
                                       |     |     |
                                     Cam1  Cam2  Cam3
```

### Recommended Network Segmentation

| Segment    | VLAN | Purpose                  | Example CIDR |
| ---------- | ---- | ------------------------ | ------------ |
| Management | 10   | Admin access, API, UI    | 10.0.1.0/24  |
| Camera     | 20   | IP cameras (isolated)    | 10.0.2.0/24  |
| Client     | 30   | Viewing stations, mobile | 10.0.3.0/24  |
| Storage    | 40   | NAS/SAN (if external)    | 10.0.4.0/24  |

### Key Design Principles

- **Isolate cameras on a dedicated VLAN.** Cameras should not have internet access. The NVR server bridges the camera VLAN and client VLAN.
- **NVR server needs interfaces on both camera and client VLANs** (physical or tagged).
- **ONVIF discovery uses WS-Discovery multicast** (UDP 3702). This only works within the camera VLAN; the NVR server must have a presence on that VLAN to discover cameras.
- **Place the NVR server close to cameras** (same switch or minimal hops) to minimize jitter on RTSP streams.

---

## Firewall and Port Rules

### MediaMTX NVR Listening Ports

| Port | Protocol | Service        | Direction                    | Notes                              |
| ---- | -------- | -------------- | ---------------------------- | ---------------------------------- |
| 8554 | TCP      | RTSP           | Camera -> NVR, Client -> NVR | Primary streaming protocol         |
| 8322 | TCP      | RTSPS          | Client -> NVR                | Encrypted RTSP (if enabled)        |
| 8000 | UDP      | RTP            | Camera <-> NVR               | RTSP UDP transport                 |
| 8001 | UDP      | RTCP           | Camera <-> NVR               | RTSP control channel               |
| 8002 | UDP      | Multicast RTP  | Camera -> NVR                | Multicast transport (if enabled)   |
| 8003 | UDP      | Multicast RTCP | Camera -> NVR                | Multicast control (if enabled)     |
| 1935 | TCP      | RTMP           | Client -> NVR                | RTMP streaming                     |
| 1936 | TCP      | RTMPS          | Client -> NVR                | Encrypted RTMP (if enabled)        |
| 8888 | TCP      | HLS            | Client -> NVR                | HTTP Live Streaming                |
| 8889 | TCP      | WebRTC HTTP    | Client -> NVR                | WebRTC signaling                   |
| 8189 | UDP      | WebRTC ICE     | Client <-> NVR               | WebRTC media transport             |
| 8890 | UDP      | SRT            | Client -> NVR                | SRT streaming                      |
| 9996 | TCP      | Playback API   | Client -> NVR                | Recording playback                 |
| 9997 | TCP      | Control API    | Admin -> NVR                 | Management API and admin UI        |
| 9998 | TCP      | Metrics        | Monitoring -> NVR            | Prometheus metrics (if enabled)    |
| 9999 | TCP      | pprof          | Admin -> NVR                 | Performance profiling (if enabled) |

### Outbound from NVR Server

| Port     | Protocol      | Destination | Purpose                               |
| -------- | ------------- | ----------- | ------------------------------------- |
| 554      | TCP           | Cameras     | RTSP pull from cameras                |
| 80 / 443 | TCP           | Cameras     | ONVIF device management               |
| 3702     | UDP multicast | Camera VLAN | WS-Discovery (ONVIF camera discovery) |

### Minimal Production Firewall Rules

For a typical deployment where cameras are on a separate VLAN and clients access via the management network:

**Camera VLAN -> NVR:**

```
ALLOW TCP dst-port 8554    # RTSP ingest
ALLOW UDP dst-port 8000    # RTP
ALLOW UDP dst-port 8001    # RTCP
```

**NVR -> Camera VLAN:**

```
ALLOW TCP dst-port 554     # RTSP pull from cameras
ALLOW TCP dst-port 80,443  # ONVIF HTTP/HTTPS
ALLOW UDP dst-port 3702    # WS-Discovery multicast
```

**Client VLAN -> NVR:**

```
ALLOW TCP dst-port 8554    # RTSP live view
ALLOW TCP dst-port 8888    # HLS streaming
ALLOW TCP dst-port 8889    # WebRTC signaling
ALLOW UDP dst-port 8189    # WebRTC media
ALLOW TCP dst-port 9996    # Playback/recordings
ALLOW TCP dst-port 9997    # Admin UI / API
```

**NVR -> Internet (optional):**

```
ALLOW TCP dst-port 443     # STUN/TURN for remote WebRTC (if configured)
ALLOW UDP dst-port 19302   # Google STUN (if configured)
```

**Default policy:** DENY all other traffic between VLANs.

---

## Bandwidth Planning

### Per-Camera Bandwidth Estimates

| Resolution | FPS | Codec | Bitrate (typical) | Bitrate (high motion) |
| ---------- | --- | ----- | ----------------- | --------------------- |
| 1080p      | 15  | H.264 | 2-4 Mbps          | 4-6 Mbps              |
| 1080p      | 30  | H.264 | 4-6 Mbps          | 6-10 Mbps             |
| 4MP (2K)   | 15  | H.264 | 4-6 Mbps          | 6-10 Mbps             |
| 4MP (2K)   | 30  | H.264 | 6-10 Mbps         | 10-16 Mbps            |
| 4K (8MP)   | 15  | H.264 | 8-12 Mbps         | 12-20 Mbps            |
| 4K (8MP)   | 30  | H.265 | 6-10 Mbps         | 10-16 Mbps            |
| 4K (8MP)   | 30  | H.264 | 12-20 Mbps        | 20-30 Mbps            |

### Sizing Formula

```
Total ingest bandwidth = (cameras) x (bitrate per camera)
Total egress bandwidth = (concurrent viewers) x (stream bitrate per viewer)
Total storage per day  = (total ingest bandwidth) x 86400 / 8
                       = (Mbps) x 10.5 GB/day
```

### Example: 32-Camera Site

| Parameter              | Value                        |
| ---------------------- | ---------------------------- |
| Cameras                | 32 x 1080p @ 15fps H.264     |
| Avg bitrate per camera | 3 Mbps                       |
| Total ingest           | 96 Mbps                      |
| Concurrent viewers     | 4 streams                    |
| Total egress           | 12 Mbps                      |
| Peak switch throughput | ~110 Mbps                    |
| Storage per day        | 32 x 3 x 10.5 = 1,008 GB/day |
| 30-day retention       | ~30 TB                       |

### Network Infrastructure Recommendations

| Camera count | Minimum switch uplink | NVR NIC                    | Recommended                 |
| ------------ | --------------------- | -------------------------- | --------------------------- |
| 1-8          | 1 Gbps                | 1 Gbps                     | Unmanaged switch OK         |
| 9-32         | 1 Gbps                | 1 Gbps                     | Managed switch, camera VLAN |
| 33-64        | 10 Gbps               | 10 Gbps or 2x1 Gbps bonded | Managed switch, QoS         |
| 65-128       | 10 Gbps               | 10 Gbps                    | Core + edge switches        |

### Storage Sizing Quick Reference

| Cameras | Avg Mbps each | Daily (GB) | 7-day (TB) | 30-day (TB) | 90-day (TB) |
| ------- | ------------- | ---------- | ---------- | ----------- | ----------- |
| 8       | 3             | 252        | 1.7        | 7.4         | 22.1        |
| 16      | 3             | 504        | 3.4        | 14.7        | 44.2        |
| 32      | 3             | 1,008      | 6.9        | 29.5        | 88.5        |
| 64      | 3             | 2,016      | 13.8       | 59.0        | 176.9       |
| 16      | 8             | 1,344      | 9.2        | 39.3        | 117.9       |
| 32      | 8             | 2,688      | 18.4       | 78.6        | 235.9       |

---

## Multi-Site Deployment Patterns

### Pattern 1: Independent Sites with Central Monitoring

Each site runs its own NVR instance. A central operations center connects to each site over VPN or ZTNA for monitoring and administration.

```
  Site A               Site B               Site C
+---------+         +---------+         +---------+
| NVR + DB|         | NVR + DB|         | NVR + DB|
| Cameras |         | Cameras |         | Cameras |
+----+----+         +----+----+         +----+----+
     |                   |                   |
     +-------VPN/ZTNA----+----VPN/ZTNA-------+
                         |
                   +-----+-----+
                   |   Central  |
                   | Operations |
                   |   Center   |
                   +-----------+
```

**Pros:** Each site is autonomous. Operates independently during WAN outages. Minimal WAN bandwidth (only live view on demand).

**Cons:** No centralized search across sites. Firmware/config updates must be pushed per-site.

**WAN bandwidth:** Only needed for active live-view sessions (one stream per viewer, typically 2-6 Mbps per stream). API calls for status polling are negligible.

### Pattern 2: Hub-and-Spoke Replication

Edge NVR instances at each site replicate clips or metadata to a central hub for unified search and archival.

```
  Edge Site A          Edge Site B
+-----------+        +-----------+
| NVR       |        | NVR       |
| Local DB  |---+    | Local DB  |---+
| Cameras   |   |    | Cameras   |   |
+-----------+   |    +-----------+   |
                |                    |
          +-----+--------------------+-----+
          |         Central Hub NVR        |
          |    Aggregated DB + Archive     |
          +--------------------------------+
```

**WAN bandwidth:** Depends on replication policy. Metadata-only replication is minimal (~1 KB/event). Full clip replication requires sustained bandwidth proportional to recorded events.

### Pattern 3: Cloud-Managed Edge

Each site runs a local NVR. A cloud management plane handles configuration, user management, and health monitoring. Video stays local.

**WAN requirements:**

- HTTPS (443) outbound from each NVR to cloud management API
- WebRTC (TURN relay) for remote live view when needed
- Typical: 1-5 Mbps per remote viewer through TURN

### WAN Link Sizing

| Use case                       | Per-site WAN minimum | Notes                            |
| ------------------------------ | -------------------- | -------------------------------- |
| Admin/API only                 | 1 Mbps               | Config push, health checks       |
| 1 remote live stream           | 3-6 Mbps             | Depends on resolution            |
| 4 remote live streams          | 12-24 Mbps           | Consider sub-stream if available |
| Clip replication (event-based) | 5-20 Mbps            | Varies with event density        |
| Full recording replication     | Not recommended      | Use local retention instead      |

---

## Site Survey Checklist

Use this checklist before deploying at a new site. Complete each section and retain the filled checklist in the project documentation.

### 1. Physical Infrastructure

- [ ] Server room / closet location identified
- [ ] Rack space available (\_\_\_\_U required)
- [ ] Power capacity verified (\_**\_W available, \_\_**W required)
- [ ] UPS available, rated for \_\_\_\_ minutes runtime
- [ ] Ambient temperature within spec (18-27C / 64-80F)
- [ ] Physical security of server location (locked room, access log)

### 2. Network Infrastructure

- [ ] Network switch model and port count: ********\_\_\_\_********
- [ ] PoE budget sufficient for cameras: \_**\_ W available, \_\_** W required
- [ ] PoE standard confirmed (802.3af / 802.3at / 802.3bt): ****\_\_\_\_****
- [ ] Uplink speed to core switch: \_\_\_\_ Gbps
- [ ] VLAN support available on switch: Yes / No
- [ ] Camera VLAN ID: \_**\_ CIDR: ********\_\_**********
- [ ] Management VLAN ID: \_**\_ CIDR: ********\_\_**********
- [ ] Client VLAN ID: \_**\_ CIDR: ********\_\_**********
- [ ] DHCP server available for camera VLAN: Yes / No
- [ ] DNS available on management network: Yes / No
- [ ] NTP server address: ********\_\_\_\_********

### 3. Camera Inventory

| #   | Location | Make/Model | Resolution | ONVIF Profile | PoE Class | Existing IP |
| --- | -------- | ---------- | ---------- | ------------- | --------- | ----------- |
| 1   |          |            |            |               |           |             |
| 2   |          |            |            |               |           |             |
| 3   |          |            |            |               |           |             |
| 4   |          |            |            |               |           |             |

- [ ] Total camera count: \_\_\_\_
- [ ] All cameras confirmed ONVIF Profile S compliant
- [ ] Camera firmware versions documented
- [ ] Default credentials changed on all cameras
- [ ] Camera time sync (NTP) configured

### 4. Server Specification

- [ ] CPU: ********\_\_\_\_******** (minimum: 4 cores for 16 cameras, 8 cores for 32+)
- [ ] RAM: \_\_\_\_ GB (minimum: 8 GB for 16 cameras, 16 GB for 32+)
- [ ] OS storage: \_\_\_\_ GB SSD
- [ ] Recording storage: \_\_\_\_ TB (see bandwidth planning for sizing)
- [ ] Storage type: Local disk / NAS / SAN
- [ ] If NAS/SAN: protocol (NFS / iSCSI / SMB), confirmed throughput: \_\_\_\_ MB/s
- [ ] NIC count and speed: ********\_\_\_\_********
- [ ] OS: ********\_\_\_\_******** (Linux recommended)

### 5. Bandwidth Verification

- [ ] Calculated total ingest bandwidth: \_\_\_\_ Mbps
- [ ] Calculated total storage per day: \_\_\_\_ GB
- [ ] Confirmed switch can handle aggregate throughput
- [ ] iperf3 test between NVR and camera VLAN: \_\_\_\_ Mbps measured
- [ ] iperf3 test between NVR and client VLAN: \_\_\_\_ Mbps measured

### 6. Firewall and Security

- [ ] Firewall rules configured per this guide
- [ ] Camera VLAN has no internet access
- [ ] NVR API (9997) restricted to management VLAN
- [ ] Metrics endpoint (9998) restricted to monitoring systems
- [ ] pprof endpoint (9999) disabled or restricted
- [ ] TLS certificates provisioned for HTTPS endpoints (if required)
- [ ] Authentication method configured (internal / HTTP / JWT)
- [ ] NVR JWT secret set and backed up securely

### 7. WAN / Remote Access (if applicable)

- [ ] WAN link speed: \_**\_ Mbps upload / \_\_** Mbps download
- [ ] VPN or ZTNA solution: ********\_\_\_\_********
- [ ] STUN/TURN server configured for remote WebRTC access
- [ ] Remote viewer count estimated: \_\_\_\_
- [ ] WAN bandwidth sufficient per multi-site sizing table above

### 8. Storage Retention Policy

- [ ] Retention period: \_\_\_\_ days
- [ ] Calculated total storage required: \_\_\_\_ TB
- [ ] Storage headroom (20% recommended): \_\_\_\_ TB
- [ ] Disk health monitoring configured (SMART / RAID alerts)
- [ ] Backup strategy for NVR database (SQLite): ********\_\_\_\_********

### 9. Go-Live Verification

- [ ] All cameras discovered via ONVIF and streams confirmed
- [ ] Live view working from client VLAN
- [ ] Recording and playback verified
- [ ] Admin UI accessible from management VLAN
- [ ] Alert/notification pipeline tested (if configured)
- [ ] Failover / restart behavior tested (systemd service enabled)
- [ ] Documentation handed off to site operations team

---

## Appendix: Quick Reference Card

### Default Ports Summary

```
RTSP:      8554/tcp
RTP:       8000/udp
RTCP:      8001/udp
RTMP:      1935/tcp
HLS:       8888/tcp
WebRTC:    8889/tcp (signaling) + 8189/udp (media)
SRT:       8890/udp
Playback:  9996/tcp
API:       9997/tcp
Metrics:   9998/tcp
pprof:     9999/tcp
```

### Key Configuration File

- Main config: `mediamtx.yml`
- NVR database: `~/.mediamtx/nvr.db` (default)

### Useful API Endpoints

- `GET /v3/paths/list` -- list active paths/streams
- `GET /v3/rtspconns/list` -- list RTSP connections
- `GET /v3/recordings/list` -- list recordings
- Health check: any `GET` to `http://<nvr>:9997/v3/paths/list` returning 200
