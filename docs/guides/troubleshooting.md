# MediaMTX NVR Troubleshooting Guide

This guide covers the most common issues encountered when running MediaMTX NVR, along with diagnostic steps, log analysis techniques, and instructions for collecting a support bundle.

---

## Table of Contents

1. [Camera Not Discovered via ONVIF](#1-camera-not-discovered-via-onvif)
2. [RTSP Stream Fails to Connect](#2-rtsp-stream-fails-to-connect)
3. [Recording Gaps or Missing Segments](#3-recording-gaps-or-missing-segments)
4. [High CPU or Memory Usage](#4-high-cpu-or-memory-usage)
5. [Database Locked or Corrupt](#5-database-locked-or-corrupt)
6. [Authentication and JWT Errors](#6-authentication-and-jwt-errors)
7. [ONVIF PTZ or Imaging Commands Fail](#7-onvif-ptz-or-imaging-commands-fail)
8. [Playback Returns No Data](#8-playback-returns-no-data)
9. [API Requests Return 500 or Timeout](#9-api-requests-return-500-or-timeout)
10. [Storage Full or Write Errors](#10-storage-full-or-write-errors)

---

## Log Analysis Basics

MediaMTX uses structured logging. Set `logLevel: debug` in `mediamtx.yml` for maximum verbosity (this is the default for NVR deployments).

### Key log prefixes

| Prefix | Component |
|--------|-----------|
| `[NVR]` | NVR subsystem lifecycle |
| `[ONVIF]` | ONVIF discovery, device management |
| `[RTSP]` | RTSP source/connection events |
| `[storage]` | Recording writes, segment management |
| `[API]` | HTTP API handler events |
| `[connmgr]` | Connection manager, reconnection logic |

### Filtering logs

```bash
# Show only ONVIF-related messages
journalctl -u mediamtx | grep '\[ONVIF\]'

# Show errors and warnings
journalctl -u mediamtx | grep -E '(ERR|WRN)'

# Tail live debug output
journalctl -u mediamtx -f
```

If running from a binary directly, redirect stderr:

```bash
./mediamtx 2>&1 | tee /tmp/mediamtx-debug.log
```

---

## 1. Camera Not Discovered via ONVIF

### Symptoms
- Camera does not appear in the device list after ONVIF discovery.
- Discovery returns zero results or times out.

### Diagnostics

1. **Verify network reachability.** The camera and MediaMTX host must be on the same subnet (or multicast routing must be configured for WS-Discovery).

   ```bash
   # Confirm the camera is reachable
   ping <camera-ip>

   # Check that port 3702 (WS-Discovery) is not blocked
   sudo tcpdump -i any port 3702 -n
   ```

2. **Test ONVIF manually.** Send a WS-Discovery probe:

   ```bash
   curl -s --max-time 5 \
     "http://<camera-ip>/onvif/device_service" \
     -H "Content-Type: application/soap+xml" \
     -d '<?xml version="1.0"?><s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope"><s:Body><GetDeviceInformation xmlns="http://www.onvif.org/ver10/device/wsdl"/></s:Body></s:Envelope>'
   ```

3. **Check firewall rules.** Ensure UDP 3702 (multicast) and TCP 80/8080 (ONVIF HTTP) are open.

4. **Review logs** for `[ONVIF]` discovery errors:
   ```
   [ONVIF] discovery probe sent
   [ONVIF] discovery: no responses received within timeout
   ```

### Resolution
- Ensure the camera has ONVIF enabled in its firmware settings.
- Some cameras require ONVIF to be explicitly activated and a separate ONVIF user to be created.
- If multicast discovery fails, add the camera manually via the API using its known IP address.

---

## 2. RTSP Stream Fails to Connect

### Symptoms
- Camera is discovered but the live stream does not start.
- Logs show connection refused, timeout, or 401 errors from the RTSP source.

### Diagnostics

1. **Test the RTSP URL directly:**

   ```bash
   ffprobe -v error -show_streams "rtsp://<user>:<pass>@<camera-ip>:554/stream1"
   ```

2. **Check credentials.** ONVIF credentials and RTSP credentials are often different on the camera. Verify both.

3. **Look for connection manager retries in logs:**
   ```
   [connmgr] <camera-path>: connecting...
   [connmgr] <camera-path>: connection failed: dial tcp <ip>:554: connection refused
   [connmgr] <camera-path>: will retry in 5s
   ```

4. **Verify the stream URI** returned by ONVIF matches what the camera actually serves:
   ```
   [ONVIF] GetStreamUri response: rtsp://<camera-ip>:554/...
   ```

### Resolution
- Correct the RTSP credentials in the camera configuration.
- If the camera returns a local/internal URI (e.g., `rtsp://192.168.1.x/...`) but is accessed through NAT, override the stream URI in the path configuration.
- Ensure the camera's maximum connection limit has not been reached.

---

## 3. Recording Gaps or Missing Segments

### Symptoms
- Playback timeline shows gaps where footage should exist.
- Segments are missing from the storage directory.

### Diagnostics

1. **Check the connection manager logs** for stream disconnections during the gap period:
   ```
   [connmgr] <path>: connection lost: EOF
   [connmgr] <path>: reconnecting...
   ```

2. **Inspect storage I/O health:**
   ```bash
   # Check disk I/O latency
   iostat -x 1 5

   # Verify available disk space
   df -h <recording-path>
   ```

3. **Look for segment write errors:**
   ```
   [storage] write error: no space left on device
   [storage] segment finalization failed
   ```

4. **Check for NTP/time-sync issues.** If the camera clock and server clock diverge significantly, segment timestamps may appear out of order.

### Resolution
- Address network instability causing stream drops (check cable, switch, PoE budget).
- Ensure sufficient disk space and I/O bandwidth for the number of recording streams.
- Synchronize the camera and server clocks via NTP.

---

## 4. High CPU or Memory Usage

### Symptoms
- MediaMTX process consumes excessive CPU or RAM.
- System becomes unresponsive; recordings stutter or drop.

### Diagnostics

1. **Profile resource usage:**

   ```bash
   # Snapshot process stats
   top -p $(pgrep mediamtx)

   # Go runtime profiling (if pprof is enabled)
   curl -o cpu.prof http://localhost:9997/debug/pprof/profile?seconds=30
   curl -o heap.prof http://localhost:9997/debug/pprof/heap
   go tool pprof cpu.prof
   ```

2. **Count active streams:**

   ```bash
   curl -s http://localhost:9997/v3/paths/list | python3 -m json.tool | grep -c '"name"'
   ```

3. **Check for transcoding.** MediaMTX does not transcode by default. If an external process is attached for re-encoding, that will dominate CPU usage.

4. **Review goroutine count** for leaks:
   ```bash
   curl -s http://localhost:9997/debug/pprof/goroutine?debug=1 | head -5
   ```

### Resolution
- Reduce the number of simultaneous streams or lower resolution/frame-rate on cameras.
- Ensure no unnecessary transcoding processes are running.
- If memory grows without bound, collect a heap profile and report it as a bug.

---

## 5. Database Locked or Corrupt

### Symptoms
- API returns `database is locked` errors.
- Startup fails with `database disk image is malformed`.

### Diagnostics

1. **Check for multiple processes** accessing the same database file:

   ```bash
   fuser mediamtx.db
   lsof mediamtx.db
   ```

2. **Verify SQLite integrity:**

   ```bash
   sqlite3 mediamtx.db "PRAGMA integrity_check;"
   ```

3. **Check filesystem.** SQLite requires a filesystem that supports proper locking (avoid NFS for the database file).

### Resolution
- **Locked:** Ensure only one MediaMTX instance uses the database. Stop duplicate processes.
- **Corrupt:** Restore from the most recent backup (see `internal/nvr/backup/`). If no backup exists:
  ```bash
  sqlite3 mediamtx.db ".recover" | sqlite3 recovered.db
  ```
- Always place the database on a local filesystem, never on a network share.

---

## 6. Authentication and JWT Errors

### Symptoms
- API returns `401 Unauthorized` or `token is expired`.
- UI login fails immediately after entering credentials.

### Diagnostics

1. **Verify the JWT secret** is set and has not changed since tokens were issued. The `nvrJWTSecret` in `mediamtx.yml` encrypts database keys -- changing it invalidates existing sessions and encrypted data.

2. **Check token expiry:**

   ```bash
   # Decode a JWT (paste token)
   echo "<token>" | cut -d. -f2 | base64 -d 2>/dev/null | python3 -m json.tool
   ```

3. **Review auth logs:**
   ```
   [API] auth: token validation failed: token is expired
   [API] auth: invalid signature
   ```

### Resolution
- If the JWT secret was accidentally changed, restore the original value from backup or the `.local-backup/` directory.
- Generate a new token by re-authenticating through the login endpoint.
- Ensure server and client clocks are synchronized (clock skew causes premature expiry).

---

## 7. ONVIF PTZ or Imaging Commands Fail

### Symptoms
- PTZ controls in the UI do not move the camera.
- API calls to imaging or PTZ endpoints return errors.

### Diagnostics

1. **Confirm camera supports PTZ/Imaging.** Not all ONVIF cameras implement the PTZ or Imaging service.

2. **Check ONVIF capabilities:**
   ```bash
   curl -s http://localhost:9997/v3/nvr/devices/<device-id> | python3 -m json.tool
   ```
   Look for `ptz` and `imaging` capability flags.

3. **Review ONVIF SOAP errors in logs:**
   ```
   [ONVIF] PTZ ContinuousMove failed: ter:ActionNotSupported
   [ONVIF] SetImagingSettings: SOAP fault: ...
   ```

4. **Test with the camera's own web interface** to rule out firmware issues.

### Resolution
- Verify the ONVIF user has sufficient privileges (some cameras restrict PTZ to admin accounts).
- If the camera uses non-standard ONVIF profiles, check the MediaMTX ONVIF client logs for protocol mismatches.
- Update camera firmware to the latest version.

---

## 8. Playback Returns No Data

### Symptoms
- Playback API returns empty results or 404 for a time range that should have recordings.
- The UI timeline is blank.

### Diagnostics

1. **Verify recordings exist on disk:**
   ```bash
   ls -la <recording-path>/<camera-name>/
   ```

2. **Check the clip index** in the database:
   ```bash
   sqlite3 mediamtx.db "SELECT count(*) FROM clip_index WHERE camera_name='<name>';"
   ```

3. **Confirm the `playback` setting** is enabled in `mediamtx.yml`:
   ```yaml
   playback: true
   ```

4. **Review playback logs:**
   ```
   [playback] request: camera=<name> start=... end=...
   [playback] no segments found for range
   ```

### Resolution
- If segments exist on disk but not in the index, the clip index may need rebuilding (check `fragment_backfill.go` for backfill logic).
- Ensure the time range in the query matches the server's timezone and is in the correct format (RFC 3339).
- Verify `playback: true` and `nvr: true` are both set in configuration.

---

## 9. API Requests Return 500 or Timeout

### Symptoms
- API calls return HTTP 500 Internal Server Error.
- Requests hang and eventually time out.

### Diagnostics

1. **Check logs for panics or stack traces:**
   ```bash
   journalctl -u mediamtx | grep -A 20 'panic'
   ```

2. **Test API health:**
   ```bash
   curl -v http://localhost:9997/v3/paths/list
   ```

3. **Check database availability** (see issue #5 above -- a locked DB will cause API timeouts).

4. **Monitor open file descriptors:**
   ```bash
   ls /proc/$(pgrep mediamtx)/fd | wc -l
   # or on macOS:
   lsof -p $(pgrep mediamtx) | wc -l
   ```

### Resolution
- If a panic is found, capture the full stack trace and report it as a bug.
- Restart the service if the database is locked and no other process is competing for it.
- If file descriptor exhaustion is suspected, increase the system limit: `ulimit -n 65536`.

---

## 10. Storage Full or Write Errors

### Symptoms
- Recordings stop; logs show write failures.
- Disk usage reaches 100%.

### Diagnostics

1. **Check disk usage:**
   ```bash
   df -h <recording-path>
   du -sh <recording-path>/*
   ```

2. **Review retention policy.** MediaMTX NVR manages segment lifecycle. Verify the storage manager is running:
   ```
   [storage] retention sweep: removed 42 segments older than 30d
   ```

3. **Check for orphaned files** that the storage manager may not be tracking.

4. **Inspect I/O errors:**
   ```bash
   dmesg | grep -i "i/o error"
   ```

### Resolution
- Adjust the retention policy to match available disk capacity.
- Manually remove orphaned segments if the storage manager cannot clean them up.
- Add additional storage or move recordings to a larger volume.
- Monitor disk usage proactively with alerts (see `internal/nvr/alerts/`).

---

## Network Diagnostics Cheat Sheet

```bash
# Verify multicast for ONVIF discovery
sudo tcpdump -i any -n host 239.255.255.250 and port 3702

# Test RTSP connectivity
ffprobe -v error -rtsp_transport tcp "rtsp://<ip>:554/stream"

# Check for packet loss to a camera
ping -c 100 <camera-ip> | tail -3

# Measure bandwidth to camera
iperf3 -c <camera-ip> -t 10

# Inspect PoE port status (if managed switch)
snmpwalk -v2c -c public <switch-ip> 1.3.6.1.2.1.105.1.1.1
```

---

## Collecting a Support Bundle

When reporting an issue, collect the following into a single archive:

```bash
#!/bin/bash
# support-bundle.sh
BUNDLE_DIR="/tmp/mediamtx-support-$(date +%Y%m%d-%H%M%S)"
mkdir -p "$BUNDLE_DIR"

# 1. Configuration (redact secrets)
sed 's/nvrJWTSecret:.*/nvrJWTSecret: [REDACTED]/' mediamtx.yml > "$BUNDLE_DIR/mediamtx.yml"

# 2. Recent logs (last 1000 lines)
journalctl -u mediamtx --no-pager -n 1000 > "$BUNDLE_DIR/mediamtx.log" 2>/dev/null \
  || tail -1000 /tmp/mediamtx-debug.log > "$BUNDLE_DIR/mediamtx.log" 2>/dev/null

# 3. System info
uname -a > "$BUNDLE_DIR/system-info.txt"
echo "---" >> "$BUNDLE_DIR/system-info.txt"
free -h >> "$BUNDLE_DIR/system-info.txt" 2>/dev/null
echo "---" >> "$BUNDLE_DIR/system-info.txt"
df -h >> "$BUNDLE_DIR/system-info.txt"
echo "---" >> "$BUNDLE_DIR/system-info.txt"
cat /proc/cpuinfo | head -30 >> "$BUNDLE_DIR/system-info.txt" 2>/dev/null

# 4. Process info
ps aux | grep mediamtx > "$BUNDLE_DIR/process-info.txt"
lsof -p $(pgrep mediamtx) > "$BUNDLE_DIR/open-files.txt" 2>/dev/null

# 5. Network info
ip addr > "$BUNDLE_DIR/network-info.txt" 2>/dev/null || ifconfig > "$BUNDLE_DIR/network-info.txt"
ss -tlnp | grep mediamtx >> "$BUNDLE_DIR/network-info.txt" 2>/dev/null

# 6. Database integrity
sqlite3 mediamtx.db "PRAGMA integrity_check;" > "$BUNDLE_DIR/db-integrity.txt" 2>/dev/null

# 7. Camera count and status
curl -s http://localhost:9997/v3/paths/list > "$BUNDLE_DIR/paths.json" 2>/dev/null

# 8. Storage stats
du -sh recordings/ > "$BUNDLE_DIR/storage-stats.txt" 2>/dev/null

# Create archive
tar czf "$BUNDLE_DIR.tar.gz" -C /tmp "$(basename $BUNDLE_DIR)"
echo "Support bundle created: $BUNDLE_DIR.tar.gz"
```

Run the script and attach the resulting `.tar.gz` to your issue report.

**Important:** Always review the bundle before sharing to ensure no credentials or sensitive data are included. The script redacts `nvrJWTSecret` automatically, but check for any other secrets in your configuration.
