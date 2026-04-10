// KAI-302 — Timeline data model.
//
// Pure Dart value objects for the playback timeline. No Flutter imports.
//
// The existing `lib/models/recording.dart` RecordingSegment/MotionEvent
// objects are the wire types produced by the current on-prem REST shim
// (per KAI-262). KAI-302 adds a richer model that preserves *which
// Recorder* and *which Directory* each segment came from, plus a proper
// AI-event marker taxonomy (motion / face / LPR / manual / system) that
// lead-edge will begin emitting once KAI-303 payload lockdown lands.
//
// When the proto-first LoadTimelineSpan RPC exists (see PR body — proto
// asks), this file will gain a `fromPb()` factory per type. Until then
// callers mint these values from the HTTP shim in `HttpPlaybackClient`.

/// A kind of AI event marker that can be rendered above the timeline bar.
///
/// The lead-cloud archive + lead-edge inference stacks are the source of
/// truth for which kinds exist — this enum is intentionally conservative
/// so new values can be added without a breaking change on clients.
enum EventKind {
  motion,
  face,
  lpr,
  manual,
  system,
}

/// A single continuous recording segment from one camera on one Recorder
/// within one Directory.
///
/// Immutable; equality is structural on `id` + `cameraId` + `recorderId`.
class RecordingSegment {
  final String id;
  final String cameraId;
  final String recorderId;
  final String directoryConnectionId;
  final DateTime startedAt;
  final DateTime endedAt;

  /// `true` iff the recorder reported a gap immediately preceding this
  /// segment. Painter uses this to render a striped patch.
  final bool hasGap;

  const RecordingSegment({
    required this.id,
    required this.cameraId,
    required this.recorderId,
    required this.directoryConnectionId,
    required this.startedAt,
    required this.endedAt,
    this.hasGap = false,
  }) : assert(id != '', 'id must be non-empty'),
       assert(cameraId != '', 'cameraId must be non-empty');

  Duration get duration => endedAt.difference(startedAt);

  @override
  bool operator ==(Object other) =>
      identical(this, other) ||
      (other is RecordingSegment &&
          other.id == id &&
          other.cameraId == cameraId &&
          other.recorderId == recorderId &&
          other.directoryConnectionId == directoryConnectionId &&
          other.startedAt == startedAt &&
          other.endedAt == endedAt &&
          other.hasGap == hasGap);

  @override
  int get hashCode => Object.hash(
        id,
        cameraId,
        recorderId,
        directoryConnectionId,
        startedAt,
        endedAt,
        hasGap,
      );
}

/// An AI / system event rendered as a marker above the timeline bar.
///
/// Priority ordering is stable: higher numbers render on top when two
/// markers overlap pixel-wise at the current zoom level.
class EventMarker {
  final String id;
  final String cameraId;
  final EventKind kind;
  final DateTime at;

  /// `null` for instant markers. Non-null for events with a duration, which
  /// the painter draws as an underline below the triangle.
  final int? durationMs;

  /// `0` = low, `1` = normal, `2` = high. Higher wins z-order.
  final int priority;

  const EventMarker({
    required this.id,
    required this.cameraId,
    required this.kind,
    required this.at,
    this.durationMs,
    this.priority = 1,
  }) : assert(priority >= 0 && priority <= 2, 'priority must be 0..2');

  @override
  bool operator ==(Object other) =>
      identical(this, other) ||
      (other is EventMarker &&
          other.id == id &&
          other.cameraId == cameraId &&
          other.kind == kind &&
          other.at == at &&
          other.durationMs == durationMs &&
          other.priority == priority);

  @override
  int get hashCode =>
      Object.hash(id, cameraId, kind, at, durationMs, priority);
}

/// A visual divider between segments that belong to different Recorders
/// (or different Directory connections). Rendered as a vertical line
/// with a small label in the painter.
class RecorderBoundary {
  final DateTime at;
  final String fromRecorderId;
  final String toRecorderId;
  final String fromDirectoryConnectionId;
  final String toDirectoryConnectionId;

  const RecorderBoundary({
    required this.at,
    required this.fromRecorderId,
    required this.toRecorderId,
    required this.fromDirectoryConnectionId,
    required this.toDirectoryConnectionId,
  });

  /// `true` iff this boundary crosses a Directory connection boundary and
  /// not just a Recorder boundary. UI shows a stronger visual for these.
  bool get crossesDirectory =>
      fromDirectoryConnectionId != toDirectoryConnectionId;

  @override
  bool operator ==(Object other) =>
      identical(this, other) ||
      (other is RecorderBoundary &&
          other.at == at &&
          other.fromRecorderId == fromRecorderId &&
          other.toRecorderId == toRecorderId &&
          other.fromDirectoryConnectionId == fromDirectoryConnectionId &&
          other.toDirectoryConnectionId == toDirectoryConnectionId);

  @override
  int get hashCode => Object.hash(
        at,
        fromRecorderId,
        toRecorderId,
        fromDirectoryConnectionId,
        toDirectoryConnectionId,
      );
}

/// The assembled view of a single camera's timeline between `start` and
/// `end`. Segments are sorted chronologically and boundaries are derived
/// on construction.
class TimelineSpan {
  final DateTime start;
  final DateTime end;
  final List<RecordingSegment> segments;
  final List<EventMarker> markers;
  final List<RecorderBoundary> boundaries;

  TimelineSpan._({
    required this.start,
    required this.end,
    required this.segments,
    required this.markers,
    required this.boundaries,
  });

  /// Build a span from raw segments and markers. Segments are sorted by
  /// `startedAt`, markers by `at`, and boundaries are derived wherever
  /// consecutive segments change recorderId or directoryConnectionId.
  factory TimelineSpan({
    required DateTime start,
    required DateTime end,
    required List<RecordingSegment> segments,
    required List<EventMarker> markers,
  }) {
    assert(!end.isBefore(start), 'end must be >= start');

    final sortedSegs = [...segments]
      ..sort((a, b) => a.startedAt.compareTo(b.startedAt));
    final sortedMarkers = [...markers]..sort((a, b) => a.at.compareTo(b.at));

    final boundaries = <RecorderBoundary>[];
    for (var i = 1; i < sortedSegs.length; i++) {
      final prev = sortedSegs[i - 1];
      final cur = sortedSegs[i];
      if (prev.recorderId != cur.recorderId ||
          prev.directoryConnectionId != cur.directoryConnectionId) {
        boundaries.add(RecorderBoundary(
          at: cur.startedAt,
          fromRecorderId: prev.recorderId,
          toRecorderId: cur.recorderId,
          fromDirectoryConnectionId: prev.directoryConnectionId,
          toDirectoryConnectionId: cur.directoryConnectionId,
        ));
      }
    }

    return TimelineSpan._(
      start: start,
      end: end,
      segments: List.unmodifiable(sortedSegs),
      markers: List.unmodifiable(sortedMarkers),
      boundaries: List.unmodifiable(boundaries),
    );
  }

  /// Empty span — no recordings, no markers.
  factory TimelineSpan.empty(DateTime start, DateTime end) =>
      TimelineSpan(start: start, end: end, segments: const [], markers: const []);

  Duration get duration => end.difference(start);

  /// Count of markers whose `at` falls within `[from, to]`. Used for density
  /// computations in minimap rendering.
  int markerDensity(DateTime from, DateTime to) {
    var n = 0;
    for (final m in markers) {
      if (!m.at.isBefore(from) && !m.at.isAfter(to)) n++;
    }
    return n;
  }

  /// Whether any segment fully or partially overlaps `[from, to]`.
  bool hasCoverage(DateTime from, DateTime to) {
    for (final s in segments) {
      if (s.endedAt.isBefore(from)) continue;
      if (s.startedAt.isAfter(to)) continue;
      return true;
    }
    return false;
  }
}
