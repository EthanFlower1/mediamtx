// KAI-302 — timeline_model tests.
import 'package:flutter_test/flutter_test.dart';
import 'package:nvr_client/playback/timeline_model.dart';

RecordingSegment _seg({
  required String id,
  String camera = 'cam-1',
  String recorder = 'rec-A',
  String directory = 'dir-home',
  required DateTime start,
  required DateTime end,
  bool hasGap = false,
}) {
  return RecordingSegment(
    id: id,
    cameraId: camera,
    recorderId: recorder,
    directoryConnectionId: directory,
    startedAt: start,
    endedAt: end,
    hasGap: hasGap,
  );
}

void main() {
  final t0 = DateTime.utc(2026, 4, 8, 10);
  final t1 = DateTime.utc(2026, 4, 8, 11);
  final t2 = DateTime.utc(2026, 4, 8, 12);
  final t3 = DateTime.utc(2026, 4, 8, 13);

  group('TimelineSpan', () {
    test('sorts segments chronologically on construction', () {
      final span = TimelineSpan(
        start: t0,
        end: t3,
        segments: [
          _seg(id: 'b', start: t1, end: t2),
          _seg(id: 'a', start: t0, end: t1),
        ],
        markers: const [],
      );
      expect(span.segments.map((s) => s.id).toList(), ['a', 'b']);
    });

    test('derives a RecorderBoundary when consecutive segments change recorder',
        () {
      final span = TimelineSpan(
        start: t0,
        end: t3,
        segments: [
          _seg(id: 'a', start: t0, end: t1, recorder: 'rec-A'),
          _seg(id: 'b', start: t1, end: t2, recorder: 'rec-B'),
        ],
        markers: const [],
      );
      expect(span.boundaries, hasLength(1));
      expect(span.boundaries.first.fromRecorderId, 'rec-A');
      expect(span.boundaries.first.toRecorderId, 'rec-B');
      expect(span.boundaries.first.crossesDirectory, false);
    });

    test('flags directory crossing when directoryConnectionId changes', () {
      final span = TimelineSpan(
        start: t0,
        end: t3,
        segments: [
          _seg(id: 'a', start: t0, end: t1, directory: 'dir-home'),
          _seg(id: 'b', start: t1, end: t2, directory: 'dir-cloud'),
        ],
        markers: const [],
      );
      expect(span.boundaries, hasLength(1));
      expect(span.boundaries.first.crossesDirectory, true);
    });

    test('no boundary for contiguous same-recorder segments', () {
      final span = TimelineSpan(
        start: t0,
        end: t3,
        segments: [
          _seg(id: 'a', start: t0, end: t1),
          _seg(id: 'b', start: t1, end: t2),
        ],
        markers: const [],
      );
      expect(span.boundaries, isEmpty);
    });

    test('hasCoverage reports overlap with the segment list', () {
      final span = TimelineSpan(
        start: t0,
        end: t3,
        segments: [_seg(id: 'a', start: t0, end: t1)],
        markers: const [],
      );
      expect(span.hasCoverage(t0, t1), true);
      expect(span.hasCoverage(t2, t3), false);
    });

    test('markerDensity counts markers in a sub-range', () {
      final span = TimelineSpan(
        start: t0,
        end: t3,
        segments: const [],
        markers: [
          EventMarker(id: 'e1', cameraId: 'cam-1', kind: EventKind.motion, at: t0),
          EventMarker(id: 'e2', cameraId: 'cam-1', kind: EventKind.face, at: t1),
          EventMarker(id: 'e3', cameraId: 'cam-1', kind: EventKind.lpr, at: t3),
        ],
      );
      expect(span.markerDensity(t0, t2), 2);
      expect(span.markerDensity(t2, t3), 1);
    });

    test('hasGap segment round-trips through the span', () {
      final span = TimelineSpan(
        start: t0,
        end: t3,
        segments: [_seg(id: 'a', start: t0, end: t1, hasGap: true)],
        markers: const [],
      );
      expect(span.segments.first.hasGap, true);
    });

    test('empty factory produces a zero-content span', () {
      final span = TimelineSpan.empty(t0, t3);
      expect(span.segments, isEmpty);
      expect(span.markers, isEmpty);
      expect(span.boundaries, isEmpty);
      expect(span.duration, const Duration(hours: 3));
    });
  });

  group('EventMarker', () {
    test('priority clamps to 0..2 via assertion', () {
      expect(
        () => EventMarker(
          id: 'x',
          cameraId: 'cam-1',
          kind: EventKind.motion,
          at: t0,
          priority: 5,
        ),
        throwsA(isA<AssertionError>()),
      );
    });

    test('equality is structural', () {
      final a = EventMarker(
          id: 'x', cameraId: 'c', kind: EventKind.face, at: t0);
      final b = EventMarker(
          id: 'x', cameraId: 'c', kind: EventKind.face, at: t0);
      expect(a, equals(b));
      expect(a.hashCode, b.hashCode);
    });
  });
}
