# Quick Start Guide

Go from install to first recording in 15 minutes.

## Prerequisites

- Docker and Docker Compose installed ([get Docker](https://docs.docker.com/get-docker/))
- At least one ONVIF-compatible IP camera on your network
- Camera credentials (username and password)

## 1. Install MediaMTX NVR

Docker is the recommended installation method.

```bash
docker run -d \
  --name mediamtx-nvr \
  --restart unless-stopped \
  --network host \
  -v mediamtx-data:/root/.mediamtx \
  bluenviern/mediamtx:latest
```

This exposes the following ports on the host:

| Port | Service        |
| ---- | -------------- |
| 9997 | API / Admin UI |
| 9996 | Playback       |
| 8554 | RTSP           |
| 8889 | WebRTC         |
| 8888 | HLS            |

> **Tip:** If you prefer a binary install, download the latest release from the
> [releases page](https://github.com/bluenviern/mediamtx/releases), extract it,
> and run `./mediamtx`.

## 2. Run the First-Time Setup Wizard

Open your browser and navigate to:

```
http://<server-ip>:9997
```

On first launch you will be redirected to the **Setup** page. Create your admin account:

1. Enter an admin **username** (defaults to `admin`).
2. Choose a strong **password** (minimum 6 characters).
3. Confirm the password and click **Complete Setup**.

You will be logged in automatically and taken to the admin dashboard.

## 3. Add a Camera

From the admin UI, navigate to **Camera Management** and click **Add Camera**.

1. Enter the camera's **ONVIF address** (e.g. `192.168.1.100`).
2. Provide the camera's **username** and **password**.
3. MediaMTX will auto-discover the camera's RTSP stream profile(s).
4. Choose a recording mode:
   - **Always** -- record continuously.
   - **Events** -- record only on motion or analytics triggers.
   - **Off** -- live view only, no recording.
5. Click **Save**.

The camera stream should appear in the **Live View** page within a few seconds.

## 4. Verify Recording

1. Go to the **Recordings** page in the admin UI.
2. Select your newly added camera.
3. After a minute or two of recording, a timeline strip will appear showing coverage.
4. Click any segment on the timeline to play it back.

You can also verify via the API:

```bash
curl http://<server-ip>:9997/api/nvr/cameras
```

Each camera entry includes its recording status and effective recording mode.

## 5. Next Steps

- **Playback and clip export** -- Use the Playback page or Clip Search to find and download recordings.
- **Recording schedules** -- Configure time-based recording rules per camera in Camera Management.
- **Detection zones** -- Draw zones on the camera view to limit motion detection areas.
- **User management** -- Add additional users with role-based permissions under Settings > User Management.
- **Storage management** -- Monitor disk usage and configure retention policies in Settings.

For the full configuration reference, see the comments in `mediamtx.yml` or the
[project documentation](../README.md).
