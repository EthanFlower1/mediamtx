// Proto contract alignment — lead-cloud feedback (2026-04-08).
//
// This file documents the agreed-upon proto contract changes that need to be
// applied to Flutter client stubs once the corresponding feature branches merge.
// Each section references the feature branch and the specific change.
//
// ─── 1. Camera capability flags ─────────────────────────────────────────────
//
// DONE (this branch): Added `has_sub_stream` and `has_main_stream` boolean
// capability flags to `lib/models/camera.dart`. These replace the pattern of
// inferring sub-stream support from stream URLs on the proto level.
//
// Proto shape:
//   message Camera {
//     ...
//     bool has_sub_stream = N;  // Camera advertises a secondary low-res stream
//     bool has_main_stream = N; // Camera advertises a primary high-res stream
//   }
//
// ─── 2. PlaybackTicket: signed_url + format ─────────────────────────────────
//
// PENDING (apply when feat/kai-302-playback-timeline merges):
// Add to PlaybackTicket in lib/playback/playback_client.dart:
//   - `String? signedUrl` — pre-signed URL for direct playback (replaces `url`
//     when server supports signed URLs)
//   - `PlaybackFormat format` — enum { hls, mp4, webm } indicating the media
//     container format
//
// Proto shape:
//   message PlaybackTicket {
//     string url = 1;            // existing
//     string signed_url = 4;     // NEW: pre-signed URL
//     PlaybackFormat format = 5; // NEW: media format hint
//     google.protobuf.Timestamp expires_at = 2;
//   }
//   enum PlaybackFormat {
//     PLAYBACK_FORMAT_UNSPECIFIED = 0;
//     PLAYBACK_FORMAT_HLS = 1;
//     PLAYBACK_FORMAT_MP4 = 2;
//     PLAYBACK_FORMAT_WEBM = 3;
//   }
//
// ─── 3. GetEventRequest: tenant_id ──────────────────────────────────────────
//
// PENDING (apply when feat/kai-312-events-screen merges):
// The EventsClient.list() already accepts `tenantId` but it's documented as
// "used for cross-tenant re-checks on the client". Per lead-cloud, the proto
// GetEventRequest MUST include `tenant_id` explicitly for server-side routing:
//
// Proto shape:
//   message ListEventsRequest {
//     string tenant_id = 1;     // REQUIRED, server routes to correct shard
//     EventFilter filter = 2;
//     string cursor = 3;
//     int32 limit = 4;
//   }
//
// No code change needed in EventsClient — the param already exists. Update
// the doc comment from "client-side re-check" to "server routes to shard".
//
// ─── 4. Permission naming: casbin_actions ───────────────────────────────────
//
// lead-cloud prefers `casbin_actions` over generic `permissions` in any
// permission-related interfaces. The Flutter client currently doesn't have
// permission interfaces beyond the camera permission_filter hint (UI-only).
// When adding permission APIs, use `casbinActions` as the Dart field name.
//
// ─── Playback format enum (for use when KAI-302 merges) ────────────────────

/// Media container format for playback tickets.
/// Aligns with proto `PlaybackFormat` enum.
enum PlaybackFormat {
  /// Server did not specify; client should auto-detect from URL.
  unspecified,

  /// HTTP Live Streaming (LL-HLS or standard HLS).
  hls,

  /// MPEG-4 container (progressive download or range requests).
  mp4,

  /// WebM container (VP8/VP9/AV1).
  webm,
}
