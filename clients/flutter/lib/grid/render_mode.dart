// KAI-301 — Render-mode decision rule for the multi-camera grid.
//
// Keep this file pure: no Flutter imports, no I/O, no side effects. Everything
// here is unit-testable with plain Dart. This is the single source of truth
// for the "WebRTC vs snapshot" choice — both the grid view and the telemetry
// reporter ask this function rather than duplicating the thresholds.
//
// Thresholds (ticket KAI-301):
//   * Off-LAN, cellCount <= 4        → WebRTC sub-stream.
//   * Off-LAN, cellCount 5..9        → snapshot refresh at 2–5s jitter.
//   * On-LAN, cellCount <= 9         → WebRTC (LAN is fat enough).
//   * On-LAN, cellCount > 9          → snapshot refresh.
//   * `alwaysLiveOverride = true`    → WebRTC regardless of cell count.
//
// The override exists so an operator can force live on a large wall when the
// LAN/machine can handle it — documented in the ticket as a deliberate escape
// hatch. Telemetry records whether override was active for each sample.

/// Maximum cell count that renders WebRTC when off-LAN (cloud/federation).
const int kMaxWebRtcCellsOffLan = 4;

/// Maximum cell count that renders WebRTC when on-LAN. LAN is treated as
/// "the machine + pipe are beefy enough to decode nine sub-streams".
const int kMaxWebRtcCellsOnLan = 9;

/// The minimum snapshot-refresh jitter window lower bound (seconds).
const double kMinSnapshotJitterSeconds = 2.0;

/// The maximum snapshot-refresh jitter window upper bound (seconds).
const double kMaxSnapshotJitterSeconds = 5.0;

/// How a given grid cell should render its video surface.
enum RenderMode {
  /// Full WebRTC peer connection. Sub-stream off-LAN, main stream on-LAN.
  webrtc,

  /// Periodic JPEG refresh with per-camera jitter in [2, 5] seconds.
  snapshotRefresh,
}

/// Pure decision function. See file header for the full rule table.
///
/// Parameters:
///   * [cellCount] — number of visible cells in the grid. MUST be >= 0.
///   * [alwaysLiveOverride] — operator toggle to force WebRTC.
///   * [isOnLan] — `true` when the active directory connection is reachable
///     via LAN (mDNS / private IP) rather than federated cloud.
RenderMode decideRenderMode({
  required int cellCount,
  required bool alwaysLiveOverride,
  required bool isOnLan,
}) {
  assert(cellCount >= 0, 'cellCount must be non-negative');
  if (alwaysLiveOverride) return RenderMode.webrtc;
  final cap = isOnLan ? kMaxWebRtcCellsOnLan : kMaxWebRtcCellsOffLan;
  if (cellCount <= cap) return RenderMode.webrtc;
  return RenderMode.snapshotRefresh;
}
