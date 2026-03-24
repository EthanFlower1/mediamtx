import 'package:flutter_test/flutter_test.dart';
import 'package:nvr_client/models/recording.dart';
import 'package:nvr_client/screens/playback/playback_controller.dart';

void main() {
  group('PlaybackController segment helpers', () {
    final segments = [
      RecordingSegment(
        id: 1, cameraId: 'c1',
        startTime: DateTime.utc(2026, 3, 24, 8, 0),
        endTime: DateTime.utc(2026, 3, 24, 9, 0),
        durationMs: 3600000,
      ),
      RecordingSegment(
        id: 2, cameraId: 'c1',
        startTime: DateTime.utc(2026, 3, 24, 10, 0),
        endTime: DateTime.utc(2026, 3, 24, 11, 30),
        durationMs: 5400000,
      ),
      RecordingSegment(
        id: 3, cameraId: 'c1',
        startTime: DateTime.utc(2026, 3, 24, 14, 0),
        endTime: DateTime.utc(2026, 3, 24, 16, 0),
        durationMs: 7200000,
      ),
    ];

    final dayStart = DateTime.utc(2026, 3, 24);

    test('findContainingSegment returns segment when position is inside', () {
      final pos = const Duration(hours: 8, minutes: 30);
      final result = PlaybackController.findContainingSegment(segments, dayStart, pos);
      expect(result?.id, 1);
    });

    test('findContainingSegment returns null when position is in gap', () {
      final pos = const Duration(hours: 9, minutes: 30);
      final result = PlaybackController.findContainingSegment(segments, dayStart, pos);
      expect(result, isNull);
    });

    test('findNextSegmentStart returns next segment after gap position', () {
      final pos = const Duration(hours: 9, minutes: 30);
      final result = PlaybackController.findNextSegmentStart(segments, dayStart, pos);
      expect(result, const Duration(hours: 10));
    });

    test('findNextSegmentStart returns null after last segment', () {
      final pos = const Duration(hours: 17);
      final result = PlaybackController.findNextSegmentStart(segments, dayStart, pos);
      expect(result, isNull);
    });

    test('snapToSegment seeks to next segment start when in gap', () {
      final pos = const Duration(hours: 9, minutes: 30);
      final snapped = PlaybackController.snapToSegment(segments, dayStart, pos);
      expect(snapped, const Duration(hours: 10));
    });

    test('snapToSegment returns same position when inside segment', () {
      final pos = const Duration(hours: 8, minutes: 30);
      final snapped = PlaybackController.snapToSegment(segments, dayStart, pos);
      expect(snapped, pos);
    });
  });

  group('PlaybackController event skip helpers', () {
    final events = [
      MotionEvent(id: '1', cameraId: 'c1', startedAt: '2026-03-24T08:05:00Z'),
      MotionEvent(id: '2', cameraId: 'c1', startedAt: '2026-03-24T10:30:00Z'),
      MotionEvent(id: '3', cameraId: 'c1', startedAt: '2026-03-24T14:15:00Z'),
    ];

    final dayStart = DateTime.utc(2026, 3, 24);

    test('findNextEvent returns first event after position', () {
      final pos = const Duration(hours: 9);
      final result = PlaybackController.findNextEvent(events, dayStart, pos);
      expect(result, const Duration(hours: 10, minutes: 30));
    });

    test('findPreviousEvent returns last event before position', () {
      final pos = const Duration(hours: 12);
      final result = PlaybackController.findPreviousEvent(events, dayStart, pos);
      expect(result, const Duration(hours: 10, minutes: 30));
    });

    test('findNextEvent returns null when no events after position', () {
      final pos = const Duration(hours: 15);
      final result = PlaybackController.findNextEvent(events, dayStart, pos);
      expect(result, isNull);
    });

    test('findPreviousEvent returns null when no events before position', () {
      final pos = const Duration(hours: 7);
      final result = PlaybackController.findPreviousEvent(events, dayStart, pos);
      expect(result, isNull);
    });
  });

  group('PlaybackController gap skip helpers', () {
    final segments = [
      RecordingSegment(
        id: 1, cameraId: 'c1',
        startTime: DateTime.utc(2026, 3, 24, 8, 0),
        endTime: DateTime.utc(2026, 3, 24, 9, 0),
        durationMs: 3600000,
      ),
      RecordingSegment(
        id: 2, cameraId: 'c1',
        startTime: DateTime.utc(2026, 3, 24, 10, 0),
        endTime: DateTime.utc(2026, 3, 24, 11, 30),
        durationMs: 5400000,
      ),
    ];

    final dayStart = DateTime.utc(2026, 3, 24);

    test('findNextGapEnd returns start of next segment after gap', () {
      final pos = const Duration(hours: 8, minutes: 30);
      final result = PlaybackController.findNextGapEnd(segments, dayStart, pos);
      expect(result, const Duration(hours: 10));
    });

    test('findPreviousGapStart returns end of previous segment before gap', () {
      final pos = const Duration(hours: 10, minutes: 30);
      final result = PlaybackController.findPreviousGapStart(segments, dayStart, pos);
      expect(result, const Duration(hours: 9));
    });
  });
}
