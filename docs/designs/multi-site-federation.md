# Multi-Site Federation - Design Document

**Ticket:** KAI-104
**Status:** Design
**Author:** Commercial Platform Team
**Date:** 2026-04-03

---

## 1. Overview

Multi-Site Federation enables customers with geographically distributed NVR servers to operate them as a unified system. Operators can discover cameras across sites, perform cross-site searches, play back recordings from any site, and manage federated views -- all without requiring every site's video to transit through a central point.

## 2. Goals

- Federate multiple NVR servers into a logical cluster with a unified camera namespace
- Enable cross-site video search and playback with minimal latency
- Maintain local autonomy: each site continues to function independently if WAN connectivity is lost
- Minimize bandwidth usage through intelligent query routing and on-demand streaming
- Support both peer-to-peer (mesh) and hub-spoke federation topologies

## 3. Federation Architecture

### 3.1 Topology Options

#### Peer-to-Peer Mesh

```
  Site A <-------> Site B
    ^                 ^
    |                 |
    +-----> Site C <--+
```

- Every site maintains direct connections to every other site.
- Best for small deployments (2-5 sites).
- Complexity grows as O(n^2) connections.

#### Hub-Spoke (Recommended for 5+ sites)

```
        Site B
          |
Site A -- Hub -- Site C
          |
        Site D
```

- A designated hub (or the cloud portal) acts as the federation coordinator.
- Spoke sites connect only to the hub.
- Hub routes queries and brokers direct peer connections for video streaming.

### 3.2 Federation Membership

```
FederationCluster
  |-- id (UUID)
  |-- name
  |-- topology (mesh | hub_spoke)
  |-- created_at
  |
  +-- Members[]
        |-- server_id (UUID, references NVR server)
        |-- role (hub | spoke | peer)
        |-- site_label
        |-- endpoint (WAN address or relay)
        |-- joined_at
        |-- status (active | suspended | unreachable)
```

### 3.3 Inter-Site Communication

| Layer         | Protocol                    | Purpose                                 |
| ------------- | --------------------------- | --------------------------------------- |
| Control plane | gRPC over mTLS              | Metadata queries, federation state sync |
| Data plane    | RTSP/HLS over TLS           | Live and recorded video streaming       |
| Discovery     | mDNS (LAN) / Registry (WAN) | Site and camera discovery               |

#### Control Plane Messages

| Message            | Direction        | Description                            |
| ------------------ | ---------------- | -------------------------------------- |
| `SyncCatalog`      | Bidirectional    | Exchange camera lists and metadata     |
| `SearchRecordings` | Request/Response | Query another site's recording index   |
| `RequestStream`    | Request/Response | Initiate video stream from remote site |
| `HealthPing`       | Bidirectional    | Keepalive and latency measurement      |

## 4. Site Discovery

### 4.1 LAN Discovery

- Sites on the same LAN are discovered via mDNS (`_mediamtx-federation._tcp.local`).
- Automatic pairing with manual approval (admin must confirm federation join).

### 4.2 WAN Discovery (Cloud-Assisted)

- Sites register with the Cloud Management Portal (KAI-103).
- Portal provides a federation registry listing all servers in the same organization.
- Admin selects which servers to federate and assigns topology roles.
- Connection is established using the server's cloud-registered endpoint or via a TURN relay for NAT traversal.

### 4.3 Camera Namespace

Federated cameras use a hierarchical namespace:

```
{site_label}/{camera_path}

Examples:
  headquarters/lobby-entrance
  warehouse-a/loading-dock-1
  branch-office/parking-lot
```

- Site labels must be unique within a federation cluster.
- Local cameras can still be accessed by their short name within the local site.

## 5. Unified Search

### 5.1 Query Distribution

When an operator searches across sites:

1. Client sends a search query to the local NVR (or hub).
2. Local NVR fans out the query to all federated members via gRPC `SearchRecordings`.
3. Each site searches its local SQLite database and returns matching results.
4. Results are merged, deduplicated (by camera + time range), and sorted by relevance/time.
5. Client displays unified results with site labels.

### 5.2 Search Request Schema

```json
{
  "query": {
    "time_range": {
      "start": "2026-04-01T00:00:00Z",
      "end": "2026-04-02T00:00:00Z"
    },
    "cameras": ["headquarters/*", "warehouse-a/loading-dock-*"],
    "sites": ["headquarters", "warehouse-a"],
    "limit": 100,
    "offset": 0
  }
}
```

### 5.3 Response Merging

- Results from each site include a `site_id` and `site_label`.
- Client-side merge uses a priority queue sorted by timestamp.
- Total count is the sum of per-site counts; pagination uses per-site cursors.

### 5.4 Search Timeout Handling

- Per-site query timeout: 10 seconds.
- If a site is unreachable, results from available sites are returned with a warning indicating which sites could not be queried.

## 6. Cross-Site Playback

### 6.1 Playback Flow

```
Client --> Local NVR --> Remote NVR --> Video Segments
                |                           |
                +--- Proxied Stream <-------+
```

1. Client requests playback of a recording from `warehouse-a/loading-dock-1`.
2. Local NVR identifies the owning site from the namespace.
3. Local NVR opens a gRPC `RequestStream` to the remote site.
4. Remote site streams video segments back via RTSP or HLS.
5. Local NVR proxies the stream to the client (or provides a redirect URL for direct connection).

### 6.2 Direct vs. Proxied Playback

| Mode    | When Used                    | Pros                           | Cons                                   |
| ------- | ---------------------------- | ------------------------------ | -------------------------------------- |
| Proxied | Default; client behind NAT   | Simple client config           | Double bandwidth at local site         |
| Direct  | Client can reach remote site | Lower latency, less local load | Requires client-to-remote connectivity |

- The system defaults to proxied mode and falls back automatically.
- Direct mode is offered when the client can reach the remote site's endpoint (determined by a connectivity probe).

### 6.3 Adaptive Bitrate for WAN

- Remote playback automatically selects a lower-bitrate sub-stream when WAN latency exceeds 100ms or available bandwidth drops below the primary stream bitrate.
- Transcoding is avoided; the system uses the camera's secondary stream profile (if available via ONVIF).

## 7. Bandwidth Management

### 7.1 Design Principles

- **No bulk video replication:** Video stays on the recording site. Only metadata catalogs are synced.
- **On-demand streaming:** Video is fetched from the remote site only when explicitly requested by a user.
- **Catalog sync is lightweight:** Camera metadata and recording indexes (time ranges, sizes) are exchanged -- not video data.

### 7.2 Bandwidth Controls

| Control                         | Description                                 | Default   |
| ------------------------------- | ------------------------------------------- | --------- |
| `max_wan_bitrate`               | Per-site outbound cap for federated streams | 50 Mbps   |
| `max_concurrent_remote_streams` | Limit simultaneous cross-site playbacks     | 4         |
| `catalog_sync_interval`         | How often camera catalogs are exchanged     | 5 minutes |
| `prefer_substream`              | Use secondary stream for remote playback    | true      |

### 7.3 Bandwidth Estimation

- Sites periodically measure WAN throughput to each peer (TCP throughput test, 1 MB probe, every 15 minutes).
- Estimated bandwidth is displayed in the federation dashboard and used to auto-select stream quality.

### 7.4 Offline Resilience

- Each site maintains a cached copy of the last-known federated catalog.
- If a site goes offline, its cameras appear as "unavailable" in the unified view, but the cached catalog still shows what recordings existed.
- When connectivity restores, a delta sync updates the catalog.

## 8. Security

- **mTLS:** All inter-site gRPC and streaming connections use mutual TLS with certificates issued by the federation cluster CA.
- **Authorization:** Operators must have permission on both the local and remote site to access cross-site cameras. Permissions are checked at both ends.
- **Encryption:** All video in transit between sites is encrypted (TLS 1.3).
- **Audit:** Cross-site access is logged on both the requesting and serving site.

## 9. Configuration

Federation is configured per-server in the NVR settings:

```yaml
federation:
  enabled: true
  cluster_id: "uuid-of-cluster"
  site_label: "headquarters"
  topology: "hub_spoke"
  role: "hub"
  peers:
    - server_id: "uuid-of-peer"
      endpoint: "wss://peer-address:8443"
  bandwidth:
    max_wan_bitrate_mbps: 50
    max_concurrent_remote_streams: 4
    prefer_substream: true
  catalog_sync_interval: "5m"
```

## 10. Open Questions

- Should federation support automatic failover (if a site goes down, another site takes over recording for its cameras via ONVIF re-pointing)?
- How to handle time zone differences in cross-site search results?
- Should there be a "federation admin" role distinct from local site admin?
- TURN relay hosting: self-hosted vs. cloud-managed for NAT traversal?
