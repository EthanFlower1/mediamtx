// KAI-302 — Playback client abstraction.
//
// Defines the surface the UI talks to for timeline loads, playback-URL
// minting, bookmark creation, and clip export. The real transport binding
// is blocked on proto-first RPCs (see PR body — proto asks):
//
//   * kaivue.playback.v1.LoadTimelineSpan
//   * kaivue.playback.v1.MintPlaybackUrl
//   * kaivue.playback.v1.CreateBookmark
//   * kaivue.playback.v1.ExportClip
//   * kaivue.playback.v1.AiMarkerStream  (WS push, optional for v1)
//
// Until those protos exist this file ships:
//   * `PlaybackClient` — the interface.
//   * `FakePlaybackClient` — deterministic in-memory fake for widget tests.
//   * `HttpPlaybackClient` — stub that throws UnimplementedError but keeps
//      the signature stable so real wiring can drop in later.

import 'timeline_model.dart';

/// Opaque ticket returned by `mintPlaybackUrl`. Carries the signed URL
/// plus a TTL so the client knows when to re-mint.
class PlaybackTicket {
  final String url;
  final DateTime expiresAt;

  const PlaybackTicket({required this.url, required this.expiresAt});
}

/// ID type for bookmarks; opaque server-minted string.
typedef BookmarkId = String;

/// ID type for clip-export jobs; opaque server-minted string.
typedef ClipId = String;

/// Interface — all playback I/O flows through this.
abstract class PlaybackClient {
  /// Load the full timeline span for `cameraId` between `start` and `end`.
  /// Implementations must stitch segments across multiple Recorders and
  /// multiple Directory connections so `TimelineSpan.boundaries` is
  /// populated.
  Future<TimelineSpan> loadSpan({
    required String cameraId,
    required DateTime start,
    required DateTime end,
  });

  /// Mint a signed playback URL for a segment at a given speed.
  /// `playbackSpeed` must be one of {1.0, 2.0, 4.0, 8.0}.
  Future<PlaybackTicket> mintPlaybackUrl({
    required String segmentId,
    double playbackSpeed = 1.0,
  });

  /// Create a bookmark at `atMs` offset from the segment start.
  Future<BookmarkId> createBookmark({
    required String segmentId,
    required int atMs,
    String? note,
  });

  /// Queue a clip export for `[startMs, endMs]` inside `segmentId`.
  Future<ClipId> exportClip({
    required String segmentId,
    required int startMs,
    required int endMs,
    String? note,
  });
}

/// In-memory fake for tests. Pre-load it with spans + tickets, then call
/// into it from widget tests and assert on `.lastCall`.
class FakePlaybackClient implements PlaybackClient {
  final Map<String, TimelineSpan> _spans = {};
  final Map<String, PlaybackTicket> _tickets = {};

  int bookmarkCounter = 0;
  int clipCounter = 0;

  /// The last method invoked (for assertions).
  String? lastCall;
  Map<String, Object?>? lastArgs;

  /// When set, the next call to any method throws this object.
  Object? failWith;

  void seedSpan(String cameraId, TimelineSpan span) {
    _spans[cameraId] = span;
  }

  void seedTicket(String segmentId, PlaybackTicket ticket) {
    _tickets[segmentId] = ticket;
  }

  void _record(String name, Map<String, Object?> args) {
    lastCall = name;
    lastArgs = args;
    final err = failWith;
    if (err != null) {
      failWith = null;
      throw err;
    }
  }

  @override
  Future<TimelineSpan> loadSpan({
    required String cameraId,
    required DateTime start,
    required DateTime end,
  }) async {
    _record('loadSpan', {'cameraId': cameraId, 'start': start, 'end': end});
    return _spans[cameraId] ?? TimelineSpan.empty(start, end);
  }

  @override
  Future<PlaybackTicket> mintPlaybackUrl({
    required String segmentId,
    double playbackSpeed = 1.0,
  }) async {
    _record('mintPlaybackUrl',
        {'segmentId': segmentId, 'playbackSpeed': playbackSpeed});
    return _tickets[segmentId] ??
        PlaybackTicket(
          url: 'fake://playback/$segmentId?speed=$playbackSpeed',
          expiresAt: DateTime.now().add(const Duration(minutes: 5)),
        );
  }

  @override
  Future<BookmarkId> createBookmark({
    required String segmentId,
    required int atMs,
    String? note,
  }) async {
    _record('createBookmark',
        {'segmentId': segmentId, 'atMs': atMs, 'note': note});
    bookmarkCounter++;
    return 'bookmark-$bookmarkCounter';
  }

  @override
  Future<ClipId> exportClip({
    required String segmentId,
    required int startMs,
    required int endMs,
    String? note,
  }) async {
    _record('exportClip', {
      'segmentId': segmentId,
      'startMs': startMs,
      'endMs': endMs,
      'note': note,
    });
    clipCounter++;
    return 'clip-$clipCounter';
  }
}

/// Stub HTTP implementation. Method bodies deliberately throw — real
/// wiring waits on the RPCs listed in the file header.
class HttpPlaybackClient implements PlaybackClient {
  final String baseUrl;
  final Future<String?> Function() getAccessToken;

  HttpPlaybackClient({required this.baseUrl, required this.getAccessToken});

  @override
  Future<TimelineSpan> loadSpan({
    required String cameraId,
    required DateTime start,
    required DateTime end,
  }) {
    // TODO(KAI-262): implement against LoadTimelineSpan RPC once landed.
    throw UnimplementedError('HttpPlaybackClient.loadSpan awaits KAI-262');
  }

  @override
  Future<PlaybackTicket> mintPlaybackUrl({
    required String segmentId,
    double playbackSpeed = 1.0,
  }) {
    // TODO(KAI-149): implement against MintPlaybackUrl once landed.
    throw UnimplementedError(
        'HttpPlaybackClient.mintPlaybackUrl awaits KAI-149');
  }

  @override
  Future<BookmarkId> createBookmark({
    required String segmentId,
    required int atMs,
    String? note,
  }) {
    // TODO(lead-onprem): implement once CreateBookmark RPC exists.
    throw UnimplementedError('HttpPlaybackClient.createBookmark pending RPC');
  }

  @override
  Future<ClipId> exportClip({
    required String segmentId,
    required int startMs,
    required int endMs,
    String? note,
  }) {
    // TODO(lead-onprem): implement once ExportClip RPC exists.
    throw UnimplementedError('HttpPlaybackClient.exportClip pending RPC');
  }
}
