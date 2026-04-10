# MediaMTX NVR Installation Guide

This guide covers installing and running MediaMTX NVR on Windows, Linux, and Docker.

---

## Table of Contents

1. [Prerequisites Checklist](#prerequisites-checklist)
2. [Windows Installation](#windows-installation)
3. [Linux Installation](#linux-installation)
4. [Docker / Docker Compose Deployment](#docker--docker-compose-deployment)
5. [Post-Installation Verification](#post-installation-verification)
6. [Common Issues and Solutions](#common-issues-and-solutions)

---

## Prerequisites Checklist

### All Platforms

| Requirement         | Details                                                                                                                                                         |
| ------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **Network access**  | The host must be reachable by your ONVIF cameras (same VLAN or routed subnet).                                                                                  |
| **Ports available** | See the port table below. Ensure no other service is bound to these ports.                                                                                      |
| **Disk space**      | Minimum 500 MB for the binary and database. Recording storage depends on camera count, resolution, and retention policy. Plan at least 1 TB for production use. |
| **Time sync**       | NTP should be configured. ONVIF authentication and JWT tokens depend on accurate system time.                                                                   |

### Default Ports

| Port | Protocol | Purpose     |
| ---- | -------- | ----------- |
| 9997 | TCP      | API (REST)  |
| 9996 | TCP      | Playback    |
| 8554 | TCP/UDP  | RTSP        |
| 1935 | TCP      | RTMP        |
| 8888 | TCP      | HLS         |
| 8889 | TCP/UDP  | WebRTC HTTP |
| 8890 | UDP      | SRT         |

### Platform-Specific Prerequisites

| Requirement                            | Windows                       | Ubuntu/Debian              | RHEL/CentOS                           |
| -------------------------------------- | ----------------------------- | -------------------------- | ------------------------------------- |
| **OS version**                         | Windows 10/11 or Server 2019+ | Ubuntu 20.04+ / Debian 11+ | RHEL 8+ / CentOS Stream 8+            |
| **Architecture**                       | x86_64 (AMD64)                | x86_64 or ARM64            | x86_64 or ARM64                       |
| **Go (build from source only)**        | 1.25+                         | 1.25+                      | 1.25+                                 |
| **FFmpeg (optional, for transcoding)** | Download from ffmpeg.org      | `apt install ffmpeg`       | `dnf install ffmpeg` (via RPM Fusion) |

---

## Windows Installation

### Option A: Pre-Built Binary

1. **Download the latest release** from the [GitHub Releases](https://github.com/bluenviron/mediamtx/releases) page. Choose the `mediamtx_<version>_windows_amd64.zip` archive.

2. **Extract the archive** to a directory of your choice, for example `C:\mediamtx\`.

3. **Edit the configuration file.** Open `mediamtx.yml` in a text editor and configure NVR settings:

   ```yaml
   nvr: true
   api: true
   playback: true
   nvrDatabase: C:\mediamtx\data\nvr.db
   ```

4. **Open firewall ports.** In an elevated PowerShell prompt:

   ```powershell
   New-NetFirewallRule -DisplayName "MediaMTX RTSP" -Direction Inbound -Protocol TCP -LocalPort 8554 -Action Allow
   New-NetFirewallRule -DisplayName "MediaMTX API" -Direction Inbound -Protocol TCP -LocalPort 9997 -Action Allow
   New-NetFirewallRule -DisplayName "MediaMTX Playback" -Direction Inbound -Protocol TCP -LocalPort 9996 -Action Allow
   New-NetFirewallRule -DisplayName "MediaMTX WebRTC" -Direction Inbound -Protocol TCP -LocalPort 8889 -Action Allow
   ```

5. **Run MediaMTX:**
   ```powershell
   cd C:\mediamtx
   .\mediamtx.exe
   ```

### Option B: Build from Source

1. **Install Go 1.25+** from [go.dev/dl](https://go.dev/dl/).

2. **Clone and build:**

   ```powershell
   git clone https://github.com/bluenviron/mediamtx.git
   cd mediamtx
   go build -o mediamtx.exe .
   ```

3. Follow steps 3-5 from Option A above.

### Running as a Windows Service

To run MediaMTX as a background service, use [NSSM](https://nssm.cc/):

```powershell
# Download and extract nssm, then:
nssm install MediaMTX "C:\mediamtx\mediamtx.exe"
nssm set MediaMTX AppDirectory "C:\mediamtx"
nssm set MediaMTX Start SERVICE_AUTO_START
nssm start MediaMTX
```

---

## Linux Installation

### Ubuntu / Debian

#### Option A: Pre-Built Binary

1. **Download and extract:**

   ```bash
   # Replace <version> and <arch> with appropriate values (e.g., v1.12.0, linux_amd64)
   wget https://github.com/bluenviron/mediamtx/releases/download/<version>/mediamtx_<version>_linux_amd64.tar.gz
   sudo mkdir -p /opt/mediamtx
   sudo tar -xzf mediamtx_*.tar.gz -C /opt/mediamtx
   ```

2. **Create a dedicated user:**

   ```bash
   sudo useradd -r -s /usr/sbin/nologin mediamtx
   sudo chown -R mediamtx:mediamtx /opt/mediamtx
   ```

3. **Edit configuration:**

   ```bash
   sudo nano /opt/mediamtx/mediamtx.yml
   ```

   Ensure NVR mode is enabled:

   ```yaml
   nvr: true
   api: true
   playback: true
   ```

4. **Create a systemd service** at `/etc/systemd/system/mediamtx.service`:

   ```ini
   [Unit]
   Description=MediaMTX NVR
   After=network.target

   [Service]
   Type=simple
   User=mediamtx
   Group=mediamtx
   WorkingDirectory=/opt/mediamtx
   ExecStart=/opt/mediamtx/mediamtx
   Restart=always
   RestartSec=5
   LimitNOFILE=65536

   [Install]
   WantedBy=multi-user.target
   ```

5. **Enable and start:**

   ```bash
   sudo systemctl daemon-reload
   sudo systemctl enable mediamtx
   sudo systemctl start mediamtx
   sudo systemctl status mediamtx
   ```

6. **Open firewall ports (if ufw is active):**
   ```bash
   sudo ufw allow 8554/tcp   # RTSP
   sudo ufw allow 9997/tcp   # API
   sudo ufw allow 9996/tcp   # Playback
   sudo ufw allow 8889/tcp   # WebRTC
   ```

#### Option B: Build from Source

1. **Install dependencies:**

   ```bash
   sudo apt update
   sudo apt install -y git build-essential
   # Install Go 1.25+ (https://go.dev/dl/)
   wget https://go.dev/dl/go1.25.0.linux-amd64.tar.gz
   sudo rm -rf /usr/local/go
   sudo tar -C /usr/local -xzf go1.25.0.linux-amd64.tar.gz
   export PATH=$PATH:/usr/local/go/bin
   ```

2. **Clone and build:**

   ```bash
   git clone https://github.com/bluenviron/mediamtx.git
   cd mediamtx
   go build -o mediamtx .
   sudo cp mediamtx /opt/mediamtx/
   sudo cp mediamtx.yml /opt/mediamtx/
   ```

3. Follow steps 2-6 from Option A above.

### RHEL / CentOS

#### Option A: Pre-Built Binary

1. **Download and extract:**

   ```bash
   wget https://github.com/bluenviron/mediamtx/releases/download/<version>/mediamtx_<version>_linux_amd64.tar.gz
   sudo mkdir -p /opt/mediamtx
   sudo tar -xzf mediamtx_*.tar.gz -C /opt/mediamtx
   ```

2. **Create a dedicated user:**

   ```bash
   sudo useradd -r -s /sbin/nologin mediamtx
   sudo chown -R mediamtx:mediamtx /opt/mediamtx
   ```

3. **Edit configuration** (same as Ubuntu section above).

4. **Create a systemd service** (same unit file as Ubuntu section above).

5. **Enable and start** (same systemctl commands as Ubuntu section above).

6. **Open firewall ports:**
   ```bash
   sudo firewall-cmd --permanent --add-port=8554/tcp
   sudo firewall-cmd --permanent --add-port=9997/tcp
   sudo firewall-cmd --permanent --add-port=9996/tcp
   sudo firewall-cmd --permanent --add-port=8889/tcp
   sudo firewall-cmd --reload
   ```

#### Option B: Build from Source

1. **Install dependencies:**

   ```bash
   sudo dnf groupinstall -y "Development Tools"
   sudo dnf install -y git
   # Install Go 1.25+ (https://go.dev/dl/)
   wget https://go.dev/dl/go1.25.0.linux-amd64.tar.gz
   sudo rm -rf /usr/local/go
   sudo tar -C /usr/local -xzf go1.25.0.linux-amd64.tar.gz
   export PATH=$PATH:/usr/local/go/bin
   ```

2. **Clone and build:**

   ```bash
   git clone https://github.com/bluenviron/mediamtx.git
   cd mediamtx
   go build -o mediamtx .
   sudo cp mediamtx /opt/mediamtx/
   sudo cp mediamtx.yml /opt/mediamtx/
   ```

3. Follow steps 2-6 from the RHEL Option A section above.

---

## Docker / Docker Compose Deployment

### Docker Run

```bash
docker run -d \
  --name mediamtx \
  --restart unless-stopped \
  --network host \
  -v mediamtx-config:/opt/mediamtx/config \
  -v mediamtx-data:/opt/mediamtx/data \
  bluenviron/mediamtx:latest
```

Using `--network host` is recommended for RTSP/WebRTC so that UDP streams work without complex port mapping. If host networking is not an option, publish ports explicitly:

```bash
docker run -d \
  --name mediamtx \
  --restart unless-stopped \
  -p 8554:8554 \
  -p 9997:9997 \
  -p 9996:9996 \
  -p 1935:1935 \
  -p 8888:8888 \
  -p 8889:8889 \
  -p 8889:8889/udp \
  -p 8890:8890/udp \
  -v mediamtx-config:/opt/mediamtx/config \
  -v mediamtx-data:/opt/mediamtx/data \
  bluenviron/mediamtx:latest
```

### Docker Compose

Create a `docker-compose.yml` file:

```yaml
version: "3.8"

services:
  mediamtx:
    image: bluenviron/mediamtx:latest
    container_name: mediamtx
    restart: unless-stopped
    network_mode: host
    volumes:
      - ./mediamtx.yml:/mediamtx.yml:ro
      - mediamtx-data:/opt/mediamtx/data
    environment:
      - MTX_LOG_LEVEL=info

volumes:
  mediamtx-data:
```

Then run:

```bash
docker compose up -d
docker compose logs -f mediamtx
```

### Custom Configuration with Docker

To use your own `mediamtx.yml`:

1. Copy the default config out of the container:

   ```bash
   docker run --rm bluenviron/mediamtx:latest cat /mediamtx.yml > mediamtx.yml
   ```

2. Edit `mediamtx.yml` to enable NVR mode:

   ```yaml
   nvr: true
   api: true
   playback: true
   ```

3. Mount it into the container (as shown in the Docker Compose example above).

---

## Post-Installation Verification

After starting MediaMTX, verify the installation:

1. **Check the process is running:**

   ```bash
   # Linux
   systemctl status mediamtx

   # Docker
   docker ps | grep mediamtx
   ```

2. **Check the API is responding:**

   ```bash
   curl http://localhost:9997/v3/paths/list
   ```

   You should receive a JSON response (possibly an empty list if no camera paths are configured yet).

3. **Check logs for errors:**

   ```bash
   # Linux (systemd)
   journalctl -u mediamtx -f

   # Docker
   docker logs -f mediamtx
   ```

4. **Verify the NVR database was created:**
   ```bash
   ls -la ~/.mediamtx/nvr.db
   ```

---

## Common Issues and Solutions

### Port Already in Use

**Symptom:** MediaMTX fails to start with `bind: address already in use`.

**Solution:** Identify what is using the port and either stop that service or change the MediaMTX port in `mediamtx.yml`.

```bash
# Linux
sudo ss -tlnp | grep 8554

# Windows (PowerShell)
Get-NetTCPConnection -LocalPort 8554 | Select-Object OwningProcess
```

Then update `mediamtx.yml`:

```yaml
rtspAddress: :8555 # Use an alternative port
```

### Permission Denied on Linux

**Symptom:** `permission denied` errors when starting the service.

**Solution:** Ensure the `mediamtx` user owns the installation directory and database location:

```bash
sudo chown -R mediamtx:mediamtx /opt/mediamtx
sudo mkdir -p /home/mediamtx/.mediamtx
sudo chown -R mediamtx:mediamtx /home/mediamtx/.mediamtx
```

If using ports below 1024, grant the binary the capability:

```bash
sudo setcap cap_net_bind_service=+ep /opt/mediamtx/mediamtx
```

### ONVIF Camera Discovery Not Finding Cameras

**Symptom:** Cameras on the network are not discovered.

**Solution:**

- Confirm the host and cameras are on the same subnet or VLAN.
- Ensure UDP multicast (WS-Discovery, port 3702) is not blocked by the firewall.
- Try adding cameras manually by IP address through the API instead of relying on discovery.

### SQLite Database Locked Errors

**Symptom:** `database is locked` errors in the logs.

**Solution:**

- Ensure only one instance of MediaMTX is running against the same database file.
- Check that the database file is on a local filesystem, not a network share (NFS/CIFS). SQLite does not work reliably over network filesystems.

```bash
# Check for multiple instances
ps aux | grep mediamtx
```

### WebRTC Streams Not Working

**Symptom:** WebRTC playback fails or shows no video.

**Solution:**

- When running behind NAT, configure a STUN/TURN server in `mediamtx.yml`.
- In Docker without host networking, ensure UDP ports (8889) are published.
- Check browser console for ICE connection errors.

### High Memory Usage with Many Cameras

**Symptom:** Memory consumption grows over time with many concurrent streams.

**Solution:**

- Increase `writeQueueSize` in `mediamtx.yml` if you see packet drops, but be aware it increases RAM usage.
- Reduce camera stream resolution or frame rate at the camera level.
- Monitor with:
  ```bash
  # Linux
  systemctl status mediamtx   # shows current memory
  ```

### Service Fails to Start After Upgrade

**Symptom:** MediaMTX crashes on startup after updating to a new version.

**Solution:**

- Check for configuration file format changes in the release notes.
- Back up and regenerate the config:
  ```bash
  cp /opt/mediamtx/mediamtx.yml /opt/mediamtx/mediamtx.yml.bak
  /opt/mediamtx/mediamtx --help  # prints defaults
  ```
- The NVR database schema is migrated automatically. If you see migration errors, check the logs for details and ensure the database file is not corrupted.

### Windows: "Not Recognized as a Command"

**Symptom:** Running `mediamtx` in PowerShell gives a "not recognized" error.

**Solution:** Use the full path or navigate to the directory:

```powershell
cd C:\mediamtx
.\mediamtx.exe
```

Or add `C:\mediamtx` to your system PATH environment variable.

### Clock Skew Breaking ONVIF Authentication

**Symptom:** ONVIF requests to cameras fail with authentication errors despite correct credentials.

**Solution:** ONVIF uses WS-Security with timestamps. Ensure the server and cameras have synchronized clocks:

```bash
# Check current time
date

# Install and enable NTP (Ubuntu/Debian)
sudo apt install -y chrony
sudo systemctl enable chrony
sudo systemctl start chrony
```
