# KAI-8: Storage Quota Management Design Spec

## Overview

Add per-camera and global disk quotas to the NVR with configurable warning thresholds and automatic oldest-first cleanup. Builds on KAI-7's event-aware retention system.

## Architecture

Three layers integrated into the existing codebase:

### 1. Database Schema (Migration 29)

**New columns on `cameras` table:**

- `quota_bytes INTEGER NOT NULL DEFAULT 0` — per-camera quota (0 = unlimited)
- `quota_warning_percent INTEGER NOT NULL DEFAULT 80` — warning threshold percentage
- `quota_critical_percent INTEGER NOT NULL DEFAULT 90` — critical threshold percentage

**New `storage_quotas` table for global quotas:**
```sql
CREATE TABLE storage_quotas (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    quota_bytes INTEGER NOT NULL,
    warning_percent INTEGER NOT NULL DEFAULT 80,
    critical_percent INTEGER NOT NULL DEFAULT 90,
    enabled INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
```

A single row with `id = 'global'` represents the system-wide disk quota. Additional rows could support per-storage-path quotas in the future.

### 2. Quota Enforcement (Scheduler Integration)

Runs after time-based retention cleanup in `runRetentionCleanup()`:

**Per-camera quota enforcement:**

1. Query `GetStoragePerCamera()` for each camera with `quota_bytes > 0`
2. If usage exceeds quota, calculate bytes to free
3. Delete oldest non-event recordings first (`DeleteOldestRecordings`)
4. If still over quota, delete oldest event recordings
5. Log deletions and emit quota events

**Global quota enforcement:**

1. Sum all camera storage from DB
2. If total exceeds global `quota_bytes`, identify camera with most storage above fair share
3. Delete oldest recordings from that camera
4. Repeat until under quota

**Deletion strategy:** Always oldest-first. Two-tier: non-event recordings first, then event recordings. This preserves event recordings as long as possible.

### 3. API Endpoints

**Per-camera quota (on existing camera CRUD):**

- `PUT /api/nvr/cameras/:id` — include `quota_bytes`, `quota_warning_percent`, `quota_critical_percent` in camera update
- `GET /api/nvr/cameras/:id` — returns quota fields in camera response

**Global quota management:**

- `GET /api/nvr/quotas` — list all quotas (global + any future per-path)
- `PUT /api/nvr/quotas/global` — set global quota
- `GET /api/nvr/quotas/status` — quota status with per-camera usage, warnings, and enforcement history

**Quota status response:**

```json
{
  "global": {
    "quota_bytes": 1099511627776,
    "used_bytes": 824633720832,
    "used_percent": 75.0,
    "status": "ok",
    "warning_percent": 80,
    "critical_percent": 90
  },
  "cameras": [
    {
      "camera_id": "abc",
      "camera_name": "Front Door",
      "quota_bytes": 107374182400,
      "used_bytes": 85899345920,
      "used_percent": 80.0,
      "status": "warning",
      "warning_percent": 80,
      "critical_percent": 90
    }
  ]
}
```

Status values: `ok`, `warning`, `critical`, `exceeded`

### 4. SSE Events

Publish quota events via the existing `EventBroadcaster`:

- `quota_warning` — camera crosses warning threshold
- `quota_critical` — camera crosses critical threshold
- `quota_exceeded` — enforcement triggered, recordings deleted
- `quota_ok` — camera returns below warning threshold

## Key Design Decisions

1. **Quota = 0 means unlimited** — backwards compatible, no change for existing cameras
2. **Enforcement runs with retention** — hourly check, not real-time (acceptable for NVR workloads)
3. **Event recordings protected** — non-event recordings deleted first, preserving security-relevant footage
4. **Global quota is cooperative** — distributes cleanup across cameras proportionally to their excess usage
5. **Thresholds are per-camera configurable** — different cameras may have different tolerance levels

## Files Modified

- `internal/nvr/db/migrations.go` — migration 30
- `internal/nvr/db/quota.go` — new file: quota DB methods
- `internal/nvr/db/quota_test.go` — new file: quota DB tests
- `internal/nvr/db/cameras.go` — add quota fields to Camera struct and CRUD
- `internal/nvr/scheduler/scheduler.go` — quota enforcement in retention loop
- `internal/nvr/api/quota.go` — new file: quota API handlers
- `internal/nvr/api/router.go` — register quota routes
- `internal/nvr/api/system.go` — enhance Storage response with quota info
