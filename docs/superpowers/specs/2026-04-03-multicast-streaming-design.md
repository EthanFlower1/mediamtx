# KAI-21: ONVIF Multicast Streaming Configuration

## Overview

Add opt-in multicast streaming support per camera. Unicast remains the default. When enabled, the NVR negotiates multicast transport with the camera via ONVIF, configures the multicast group settings, retrieves the multicast stream URI, and uses it as the MediaMTX source instead of the unicast URI.

## ONVIF Multicast Functions

Three new functions in `internal/nvr/onvif/`:

### GetStreamUriMulticast(client, profileToken)

Same structure as `GetStreamUri2` but requests `RTP-Multicast` transport instead of `RTP-Unicast`. Falls back to Media1 if Media2 is unavailable. Returns the multicast stream URI (typically `rtp://239.x.x.x:port`).

### GetMulticastConfig(client, profileToken)

Retrieves the camera's current multicast settings (address, port, TTL, auto-start) from the video encoder configuration via ONVIF.

### SetMulticastConfig(client, profileToken, address, port, ttl)

Updates the camera's multicast address, port, and TTL via the video encoder configuration. Validates that the address is in the 224.0.0.0-239.255.255.255 range before sending the SOAP request.

All three functions live in a new file `internal/nvr/onvif/multicast.go` and follow existing patterns: SOAP envelope construction, context-based timeouts, Media2-first with Media1 fallback.

## Database Changes

Four new columns on the camera record:

| Column             | Type   | Default | Description                                  |
| ------------------ | ------ | ------- | -------------------------------------------- |
| `MulticastEnabled` | bool   | false   | Whether this camera uses multicast transport |
| `MulticastAddress` | string | ""      | Multicast group address (e.g., 239.1.1.10)   |
| `MulticastPort`    | int    | 0       | RTP port for the multicast stream            |
| `MulticastTTL`     | int    | 5       | Time-to-live for multicast packets           |

A DB migration adds these columns with the defaults above. Existing cameras are unaffected.

## Stream Source Switching

### Current flow (unchanged for unicast)

1. Camera is created/refreshed -> `GetStreamUri2()` returns unicast RTSP URI
2. URI is set as the MediaMTX source path for that camera

### Multicast flow (when MulticastEnabled = true)

1. Camera is created/refreshed -> unicast URI is still retrieved and stored as the default
2. `GetStreamUriMulticast()` is called to get the multicast URI
3. The multicast URI is used as the MediaMTX source instead
4. If multicast URI retrieval fails, the camera stays on unicast and the error is surfaced via the API

### No automatic fallback

If a user enables multicast and the stream fails at runtime, the failure is visible rather than silently degrading to unicast. The user explicitly opted in, so they should know when it's not working.

## API Endpoints

### GET /cameras/:id/multicast

Returns current multicast configuration and whether the camera supports multicast.

The handler probes the camera live by calling `GetMulticastConfig()`. If the camera returns a valid configuration, `supported` is true.

Response:

```json
{
  "supported": true,
  "enabled": false,
  "address": "239.1.1.1",
  "port": 0,
  "ttl": 1
}
```

### PUT /cameras/:id/multicast

Enable/disable multicast and configure address, port, and TTL.

Request body:

```json
{
  "enabled": true,
  "address": "239.1.1.10",
  "port": 5004,
  "ttl": 5
}
```

**On enable:** validates address range, checks camera capability, calls `SetMulticastConfig` on the camera, retrieves multicast stream URI, updates MediaMTX source path.

**On disable:** reverts the camera to its unicast stream URI.

### Validation (PUT)

- Address must be in 224.0.0.0-239.255.255.255
- Port must be 1024-65535
- TTL must be 1-255
- Camera must have ONVIF credentials configured
- Camera must support multicast (checked via `GetMulticastConfig`)

Returns 400 if validation fails or if the camera does not support multicast.

## Capability Detection

No new capability flag in the `Capabilities` struct or DB schema. Multicast support is detected on demand by the `GET /cameras/:id/multicast` endpoint, which probes the camera live. This keeps the feature self-contained and avoids staleness in cached capability flags.

## Validation Strategy

Basic validation only:

- Camera reports multicast capability (ONVIF probe succeeds)
- Multicast address is in valid range (224.0.0.0-239.255.255.255)
- No active network probing (e.g., sending test packets to verify multicast routing)

If multicast fails at runtime due to network misconfiguration, the error surfaces through normal stream failure mechanisms.

## Scope Boundaries

**In scope:**

- ONVIF multicast SOAP functions (get/set config, get stream URI)
- Database schema changes for multicast fields
- API endpoints to configure and query multicast per camera
- Stream source switching logic (multicast URI replaces unicast URI)

**Out of scope:**

- Per-profile multicast (multicast is per-camera, using the active profile)
- Automatic multicast address pool management
- IGMP group tracking
- Multicast-to-unicast re-broadcasting by the NVR
- UI changes (API-only for now)
