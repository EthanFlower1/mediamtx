# Analytics and Reporting - Design Document

**Ticket:** KAI-107
**Status:** Design
**Author:** Commercial Platform Team
**Date:** 2026-04-03

---

## 1. Overview

Analytics and Reporting provides NVR operators with structured reports on system health, camera uptime, storage consumption, and usage patterns. Reports can be generated on-demand or on a schedule, exported as PDF or CSV, and visualized through dashboard widgets.

## 2. Goals

- Define a comprehensive set of report types covering operational and business needs
- Scheduled report generation with email delivery
- PDF and CSV export for offline consumption and compliance
- Dashboard widgets for at-a-glance metrics
- Efficient data aggregation that does not impact recording performance

## 3. Report Types

### 3.1 System Health Report

| Metric | Description | Granularity |
|--------|-------------|-------------|
| CPU usage | Average, peak, and 95th percentile | Hourly / Daily |
| Memory usage | Average and peak | Hourly / Daily |
| Disk usage | Used, free, growth rate | Daily |
| Disk IOPS | Read/write operations per second | Hourly |
| Network throughput | Inbound/outbound Mbps | Hourly |
| Server uptime | Uptime percentage and restart events | Daily / Monthly |
| Temperature | CPU/system temperature (where available) | Hourly |

### 3.2 Camera Uptime Report

| Metric | Description | Granularity |
|--------|-------------|-------------|
| Uptime percentage | Time camera was online vs. total time | Daily / Monthly |
| Offline incidents | Count, duration, and timestamps of outages | Per incident |
| Mean time to recovery (MTTR) | Average time from offline to online | Monthly |
| Recording gaps | Time ranges where recording was expected but missing | Per incident |
| Stream quality | Average bitrate, resolution, frame rate | Daily |

### 3.3 Storage Report

| Metric | Description | Granularity |
|--------|-------------|-------------|
| Total storage used | By camera, by day | Daily |
| Storage growth rate | GB/day trend | Weekly |
| Retention compliance | Are recordings being retained for the configured period? | Daily |
| Oldest recording | Per camera, oldest available recording timestamp | Daily |
| Storage forecast | Estimated days until disk full at current growth rate | Weekly |

### 3.4 User Activity Report (Cloud Portal)

| Metric | Description | Granularity |
|--------|-------------|-------------|
| Login events | Successful and failed logins per user | Daily |
| Active sessions | Concurrent session count over time | Hourly |
| Playback activity | Which cameras/time ranges were viewed, by whom | Per event |
| Configuration changes | Audit trail of settings modifications | Per event |
| API usage | Request count by endpoint and user | Daily |

### 3.5 Alert Summary Report

| Metric | Description | Granularity |
|--------|-------------|-------------|
| Alerts by type | Count of each alert type (camera offline, storage, etc.) | Daily / Monthly |
| Alerts by severity | Distribution of info/warning/critical | Daily / Monthly |
| Alert response time | Time from alert to acknowledgment (if applicable) | Per alert |
| Top alerting cameras | Cameras generating the most alerts | Monthly |
| Alert trends | Week-over-week and month-over-month comparison | Weekly / Monthly |

## 4. Scheduled Report Generation

### 4.1 Schedule Configuration

```json
{
  "report_id": "uuid",
  "name": "Weekly System Health",
  "type": "system_health",
  "schedule": {
    "frequency": "weekly",
    "day_of_week": "monday",
    "time": "06:00",
    "timezone": "America/New_York"
  },
  "time_range": {
    "relative": "last_7_days"
  },
  "format": ["pdf", "csv"],
  "recipients": [
    { "type": "email", "address": "admin@example.com" },
    { "type": "email", "address": "ops@example.com" }
  ],
  "filters": {
    "cameras": ["*"],
    "servers": ["headquarters"]
  },
  "enabled": true
}
```

### 4.2 Supported Frequencies

| Frequency | Options |
|-----------|---------|
| Daily | Time of day |
| Weekly | Day of week + time |
| Monthly | Day of month + time |
| Quarterly | Month + day + time |
| On-demand | Manual trigger via API or UI |

### 4.3 Scheduling Engine

- A background goroutine evaluates scheduled reports every minute.
- Jobs are stored in SQLite (`report_schedules` table) with `next_run_at` timestamps.
- On trigger, the report is generated asynchronously in a worker pool (max 2 concurrent report generations to limit resource impact).
- Generated reports are stored in the `reports` table with a file reference.

### 4.4 Report Storage and Retention

- Generated reports are stored on disk in `{data_dir}/reports/` as PDF/CSV files.
- Metadata (report ID, type, generated_at, file path, size) is stored in SQLite.
- Default retention: 90 days. Configurable per schedule.
- Old reports are cleaned up by the existing retention goroutine.

## 5. PDF and CSV Export

### 5.1 PDF Report Structure

```
+--------------------------------------------------+
|  [Logo]  System Health Report                     |
|  Headquarters NVR  |  April 1-7, 2026            |
+--------------------------------------------------+
|                                                    |
|  Executive Summary                                 |
|  - Server uptime: 99.97%                          |
|  - 2 camera offline incidents                      |
|  - Storage usage: 340 GB / 500 GB (68%)           |
|                                                    |
|  CPU Usage (7-day trend chart)                     |
|  [=============================]                   |
|                                                    |
|  Memory Usage (7-day trend chart)                  |
|  [=============================]                   |
|                                                    |
|  Camera Uptime Table                               |
|  +------------------+--------+----------+          |
|  | Camera           | Uptime | Incidents|          |
|  +------------------+--------+----------+          |
|  | Lobby Entrance   | 100%   | 0        |          |
|  | Parking Lot      | 98.5%  | 2        |          |
|  | Loading Dock     | 100%   | 0        |          |
|  +------------------+--------+----------+          |
|                                                    |
|  Generated: 2026-04-07 06:00 ET                   |
+--------------------------------------------------+
```

### 5.2 PDF Generation

- Use a Go PDF library (`go-pdf/fpdf` or `jung-kurt/gofpdf` successor) for server-side generation.
- Charts are rendered as embedded PNG images generated by a Go charting library (`go-echarts/go-echarts` or `wcharczuk/go-chart`).
- White-label branding (KAI-106): logo and colors are applied from brand configuration.

### 5.3 CSV Export Format

- One CSV file per report section (e.g., `system_health_cpu.csv`, `system_health_memory.csv`).
- Multiple CSVs are bundled into a ZIP archive for download.
- Header row with human-readable column names.
- ISO 8601 timestamps, UTF-8 encoding, RFC 4180 compliant.

Example (`camera_uptime.csv`):

```csv
Camera ID,Camera Name,Uptime %,Offline Incidents,Total Offline Minutes,MTTR Minutes
lobby-entrance,Lobby Entrance,100.00,0,0,0
parking-lot,Parking Lot,98.50,2,21,10.5
loading-dock,Loading Dock,100.00,0,0,0
```

## 6. Dashboard Widgets

### 6.1 Widget Types

| Widget | Visualization | Data Source |
|--------|--------------|-------------|
| **Server Status** | Status indicator (green/yellow/red) | Real-time heartbeat |
| **Camera Uptime** | Horizontal bar chart (per camera) | Aggregated from events |
| **Storage Usage** | Donut chart (used vs. free) | Real-time disk stats |
| **Storage Forecast** | Line chart with projection | Trend calculation |
| **CPU/Memory Trend** | Time-series line chart | Telemetry samples |
| **Alert Activity** | Stacked bar chart (by severity) | Alert history |
| **Top Alerting Cameras** | Ranked list | Alert aggregation |
| **Recording Gaps** | Timeline visualization (gantt-style) | Recording index |
| **Active Users** | Counter with sparkline | Session data (cloud) |

### 6.2 Widget Configuration

```json
{
  "widget_id": "uuid",
  "type": "camera_uptime",
  "title": "Camera Uptime (7 days)",
  "position": { "row": 1, "col": 2, "width": 2, "height": 1 },
  "config": {
    "time_range": "last_7_days",
    "cameras": ["*"],
    "sort": "uptime_asc"
  }
}
```

### 6.3 Dashboard Layout

- Dashboards use a responsive grid layout (4 columns on desktop, 2 on tablet, 1 on mobile).
- Users can customize widget placement via drag-and-drop.
- Default dashboard layout is provided; customizations are saved per user.

### 6.4 Data Refresh

- Widgets refresh on a configurable interval (default: 60 seconds for real-time, 5 minutes for aggregated).
- Data is fetched via the existing API endpoints; no separate WebSocket connection for widgets.
- A "last updated" timestamp is shown on each widget.

## 7. Data Aggregation

### 7.1 Aggregation Strategy

To avoid impacting recording performance, data aggregation runs as a low-priority background task:

- **Raw telemetry** is stored in a ring buffer table (`telemetry_raw`) with 24-hour retention.
- **Hourly aggregates** are computed from raw data and stored in `telemetry_hourly` (retained for 90 days).
- **Daily aggregates** are computed from hourly data and stored in `telemetry_daily` (retained for 2 years).
- Aggregation runs every hour, processing the previous hour's raw data.

### 7.2 Aggregation Tables

```sql
CREATE TABLE telemetry_hourly (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    server_id TEXT NOT NULL,
    metric_name TEXT NOT NULL,       -- 'cpu', 'memory', 'disk', 'network_in', 'network_out'
    hour_start TEXT NOT NULL,         -- ISO 8601 hour start
    avg_value REAL,
    min_value REAL,
    max_value REAL,
    p95_value REAL,
    sample_count INTEGER
);

CREATE TABLE telemetry_daily (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    server_id TEXT NOT NULL,
    metric_name TEXT NOT NULL,
    date TEXT NOT NULL,               -- YYYY-MM-DD
    avg_value REAL,
    min_value REAL,
    max_value REAL,
    p95_value REAL,
    sample_count INTEGER
);
```

### 7.3 Camera Event Aggregation

Camera uptime is derived from existing camera status events in the NVR event log:

- `camera_online` and `camera_offline` events define uptime windows.
- Aggregation computes per-camera uptime percentage for each day.
- Results are stored in `camera_uptime_daily`.

## 8. API

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/v1/reports` | List generated reports |
| POST | `/v1/reports/generate` | Trigger on-demand report generation |
| GET | `/v1/reports/:id` | Get report metadata |
| GET | `/v1/reports/:id/download` | Download report file (PDF/CSV) |
| DELETE | `/v1/reports/:id` | Delete a generated report |
| GET | `/v1/reports/schedules` | List report schedules |
| POST | `/v1/reports/schedules` | Create a report schedule |
| PUT | `/v1/reports/schedules/:id` | Update a schedule |
| DELETE | `/v1/reports/schedules/:id` | Delete a schedule |
| GET | `/v1/analytics/telemetry` | Query aggregated telemetry data |
| GET | `/v1/analytics/camera-uptime` | Query camera uptime data |
| GET | `/v1/analytics/widgets/:type` | Get data for a specific widget type |

## 9. Open Questions

- Should reports be generated in the cloud or on the NVR server (or both)?
- For the cloud portal, should dashboards be customizable per-organization or per-user?
- Should we support Grafana-compatible data export for customers who prefer their own dashboards?
- Maximum report time range: should we cap at 1 year to limit query time?
- Should generated reports be shareable via a time-limited public URL?
