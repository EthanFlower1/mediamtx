# Phase 1 Manual Verification Checklist

**Branch:** `feat/recovery-phase-1-capture-loop`
**Worktree:** `.worktrees/recovery-phase-1`
**Goal:** Verify bedrock recording works end-to-end on a real Recorder paired with a real Directory, recording from a real camera.

---

## What Phase 1 added

| Component | Purpose |
|---|---|
| `capturemanager` | Translates RecorderControl camera assignments into `supervisor.Reload()` nudges |
| `recordinghealth` | Watchdog detecting drift between `state.Store` and mediamtx runtime publishing state (30s cadence) |
| `recovery` (wired) | Boot-time disk↔DB reconciliation |
| `integrity` (wired) | Hourly fMP4 fragment verification, quarantines bad files |
| `fragmentbackfill` (wired) | Indexes pre-existing recordings missing fragment metadata |
| `thumbnail` (wired) | Generates strip thumbnails |
| `diskmonitor` | Disk capacity polling + retention enforcement (90% threshold, 5% hysteresis) |
| `recordermetrics` | Prometheus surface at `/metrics` on the recorder HTTP API (port 9998 by default) |

Supervisor `PollInterval=15s`. Drift correction promise: **within 30s** of any divergence.

---

## Pre-flight (local dev box)

- [ ] **1.** Pull the branch and confirm tests pass:
  ```bash
  cd .worktrees/recovery-phase-1
  go test -race ./internal/recorder/...
  ```
  Expect all packages PASS. `internal/recorder` (top-level package) may take ~60s due to `TestBootNotPairedNoToken`'s mDNS discovery wait — this is pre-existing.

- [ ] **2.** Confirm no regressions vs. main:
  ```bash
  git diff main..HEAD --stat | tail -5
  ```

- [ ] **3.** Inspect the boot order in `internal/recorder/boot.go` to be confident in shutdown sequencing.

---

## Deployment (real hardware or staging environment)

- [ ] **4.** Build the Recorder binary:
  ```bash
  go build -tags ffmpeg -o ./bin/raikada-recorder ./cmd/recorder
  # or however your build command is configured
  ```

- [ ] **5.** Confirm Raikada (mediamtx fork) sidecar binary is on PATH or installed at the configured location.

- [ ] **6.** Set environment for the Recorder:
  ```bash
  export MTX_MODE=recorder
  export MTX_RECORDER_STATE_DIR=/var/lib/raikada/recorder
  export MTX_PAIRING_TOKEN=<paste-token-from-Directory-admin>
  ```

- [ ] **7.** Start the Recorder and tail logs:
  ```bash
  ./bin/raikada-recorder 2>&1 | tee /tmp/recorder.log
  ```

---

## Boot-time verification

- [ ] **8.** Logs include each of these lines (the order may vary slightly):
  ```
  recorder: pairing complete                         (only on first boot)
  recorder: mesh node started
  recorder: supervisor started
  recorder: recording-health watchdog started
  recorder: NVR DB opened
  recorder: recovery complete (or scanned=0)
  recorder: integrity scanner started
  recorder: fragment-backfill scheduled
  recorder: thumbnail generator started
  recorder: disk monitor started
  recorder: metrics gauge poller started
  recorder: HTTP API listening
  ```

- [ ] **9.** No `Error` level logs at startup (other than the documented mTLS empty-cert warning until KAI-242 lands).

- [ ] **10.** Confirm `state.db` and `nvr.db` exist in `$MTX_RECORDER_STATE_DIR`:
  ```bash
  ls -la $MTX_RECORDER_STATE_DIR/*.db
  ```

---

## Camera assignment → recording

- [ ] **11.** In the Directory admin UI, assign a camera to this Recorder. Within 30s, the Recorder log should show:
  ```
  capturemanager: ensure running camera_id=<id> config_version=<n>
  ```

- [ ] **12.** The supervisor should push the path to mediamtx. Verify by querying mediamtx's config API directly:
  ```bash
  curl -s http://127.0.0.1:9997/v3/config/paths/list | jq '.items[] | select(.name | startswith("cam_"))'
  ```
  Expect at least one item with `record: true` and a `source` matching the camera's RTSP URL.

- [ ] **13.** Verify the path is actually publishing:
  ```bash
  curl -s http://127.0.0.1:9997/v3/paths/list | jq '.items[] | {name, ready, bytesReceived}'
  ```
  Expect `ready: true` and `bytesReceived` increasing over time.

- [ ] **14.** After ~30 seconds, confirm fMP4 segments are landing on disk:
  ```bash
  find $MTX_RECORDER_STATE_DIR/recordings -name "*.mp4" -mmin -2
  ```
  Expect at least one file modified within the last 2 minutes.

- [ ] **15.** Confirm DB rows are appearing (eventually — the recovery scan picks them up on the next boot, OR mediamtx writes directly via the recordstore hook):
  ```bash
  sqlite3 $MTX_RECORDER_STATE_DIR/nvr.db \
    "SELECT id, camera_id, file_path, duration_ms FROM recordings ORDER BY created_at DESC LIMIT 5;"
  ```

---

## Drift detection (recordinghealth watchdog)

- [ ] **16.** With a camera assigned and recording, manually delete the path from mediamtx via its API:
  ```bash
  curl -X DELETE "http://127.0.0.1:9997/v3/config/paths/delete/cam_<id>"
  ```

- [ ] **17.** Within 60s (2 watchdog cycles × 30s), the Recorder log shows:
  ```
  recordinghealth: persistent drift detected camera_id=<id> consecutive_cycles=2
  ```
  …and the supervisor receives a Reload nudge that re-pushes the path. Verify with:
  ```bash
  curl -s http://127.0.0.1:9997/v3/config/paths/list | jq '.items[] | select(.name == "cam_<id>")'
  ```
  Expect the path to be back.

- [ ] **18.** Verify `reconcile_errors_total` incremented on `/metrics`:
  ```bash
  curl -s http://localhost:9998/metrics | grep reconcile_errors_total
  ```
  Expect a counter ≥ 1.

---

## Metrics surface

- [ ] **19.** `/metrics` returns 200 and contains all expected metric families:
  ```bash
  curl -s http://localhost:9998/metrics | grep -E "^# TYPE (cameras_expected|cameras_publishing|disk_used_bytes|recovery_scan|integrity_|fragmentbackfill_|reconcile_errors|recorder_build_info)"
  ```
  Expect ~13 lines.

- [ ] **20.** `cameras_expected_total` matches the number of cameras assigned to this Recorder in the Directory.
  `cameras_publishing_total` matches the number actively recording. The two should be equal in steady state.

- [ ] **21.** `disk_used_bytes`, `disk_capacity_bytes`, `disk_used_percent` reflect reality:
  ```bash
  df $MTX_RECORDER_STATE_DIR/recordings
  curl -s http://localhost:9998/metrics | grep -E "^disk_"
  ```
  Numbers should match (within a tick).

---

## Disk-full / retention

⚠️ **Destructive — only run on a test machine.**

- [ ] **22.** Fill the recordings disk to 95% with synthetic data, OR set `RetentionThresholdPercent` to a low value (e.g. 1.0) via the future config flag (in Phase 1 it's hardcoded to 90% — skip this step or use a test rig).

- [ ] **23.** With the disk above threshold, the diskmonitor should delete oldest recordings within one cycle (60s). Watch the log:
  ```
  diskmonitor: enforcing retention used_pct=95.2
  diskmonitor: deleted recording recording_id=... file_path=...
  ```

- [ ] **24.** If above threshold but no expired recordings exist, the log shows:
  ```
  diskmonitor: over threshold but no expired recordings to delete
  ```
  This is operator-actionable — retention policies need tightening or the disk needs more capacity.

---

## Integrity scanner

- [ ] **25.** The integrity scanner runs every 1 hour. To trigger an immediate scan, restart the Recorder. After boot:
  ```
  recorder: integrity scanner started
  ```
  …and within 5 minutes (the first scan cycle), look for `integrity:` log lines. No log spam unless quarantines occur.

- [ ] **26.** Manually corrupt a recording file:
  ```bash
  echo "garbage" >> $MTX_RECORDER_STATE_DIR/recordings/cam_<id>/<some-segment>.mp4
  ```
  At the next scan, the integrity scanner should quarantine it. Verify:
  ```bash
  sqlite3 $MTX_RECORDER_STATE_DIR/nvr.db \
    "SELECT id, file_path, status FROM recordings WHERE status = 'quarantined';"
  ```
  Expect at least the corrupted file. `integrity_quarantines_total` on `/metrics` should also increment.

---

## Crash & restart

- [ ] **27.** Kill the recorder mid-record:
  ```bash
  pkill -9 raikada-recorder
  ```
  Restart it. Verify:
  - Boot log includes `recorder: recovery complete scanned=N repaired=M`
  - DB rows for any segments written during the crash window are present (or were repaired/cleaned up)
  - `recovery_scan_repaired_total` on `/metrics` is ≥ 0 (typically small unless many crashes)

- [ ] **28.** Kill mediamtx (the sidecar) only:
  ```bash
  pkill -9 mediamtx   # or whatever the supervisor's child process is named
  ```
  Within 30s, the supervisor restarts it AND re-pushes all paths from `state.Store`. Verify recording resumes. Watchdog will likely fire (drift detected during the gap) — `reconcile_errors_total` will increment by ≥ 1.

---

## Shutdown

- [ ] **29.** SIGTERM the recorder:
  ```bash
  kill -TERM $(pidof raikada-recorder)
  ```
  Logs should show:
  ```
  recorder: shutdown requested
  recorder: HTTP server stopped
  recorder: mesh node stopped
  recorder: thumbnail generator stopped
  recorder: NVR DB closed
  recorder: state store closed
  recorder: shutdown complete
  ```
  Process should exit within 10 seconds. ⚠️ Currently `integrity`, `fragmentbackfill`, and `diskmon` goroutines are not joined via WaitGroup (Phase 2 follow-up); they respect ctx but Shutdown doesn't wait for them. If you hit a hang on shutdown, that's the suspect.

---

## Known day-1 risks (heads-up for the operator)

1. **mTLS to Directory uses an empty cert** until KAI-242. If the Directory is configured to enforce mTLS, all four ingest streams will fail and recording won't start. **Workaround**: temporarily relax mTLS enforcement on the Directory, or implement KAI-242 (Phase 4).

2. **First boot recovery scan is synchronous**. Large fleets upgrading from pre-Phase-1 may see boot delays of tens of seconds while `recovery.Run` walks the recordings directory.

3. **Disk monitor errors silently if recordings path doesn't exist** (e.g. unmounted volume). Containerized deployments must ensure the volume is mounted before the recorder starts.

4. **Phase 1.5 not yet shipped** — recording rules (schedule-based on/off, motion-triggered) don't exist yet. Every assigned camera records 24/7 once Phase 1 is deployed.

5. **Windows builds will fail** at link time on `unix.Statfs` in `diskmonitor`. Linux/macOS only for now.

6. **`fragmentbackfill_indexed_total` always reads 0** until Phase 2 instrumentation lands. Don't write Prometheus alerts on it yet.

---

## Sign-off

Once steps 8-21 pass, Phase 1 is verified. Steps 22-29 are good-to-have validations of the hardening layers; they don't block declaring Phase 1 done.

If any step from 8-21 fails:
1. Capture the relevant log lines.
2. Capture the relevant `/metrics` output.
3. Capture `state.db` and `nvr.db` snapshots if DB-related.
4. Post in the Phase 1 retrospective so the regression is fixed before Phase 2 begins.

After sign-off, the next steps are:
- **Phase 2** — Recorder API surface (`/api/nvr/*` handlers + JWT)
- **Phase 1.5** (parallel) — recording rules, motion triggers, event broadcaster, connmgr/alerts wiring
- **Phase 4** — Auth & mTLS hardening (KAI-242)
