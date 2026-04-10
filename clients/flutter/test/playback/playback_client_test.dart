// KAI-302 — PlaybackClient (fake) round-trip tests.
import 'package:flutter_test/flutter_test.dart';
import 'package:nvr_client/playback/playback_client.dart';
import 'package:nvr_client/playback/timeline_model.dart';

void main() {
  group('FakePlaybackClient', () {
    late FakePlaybackClient client;

    setUp(() {
      client = FakePlaybackClient();
    });

    test('loadSpan returns a seeded span', () async {
      final t0 = DateTime.utc(2026, 4, 8, 10);
      final t1 = DateTime.utc(2026, 4, 8, 11);
      final span = TimelineSpan(
        start: t0,
        end: t1,
        segments: [
          RecordingSegment(
            id: 's1',
            cameraId: 'cam-1',
            recorderId: 'rec-A',
            directoryConnectionId: 'dir-home',
            startedAt: t0,
            endedAt: t1,
          ),
        ],
        markers: const [],
      );
      client.seedSpan('cam-1', span);

      final got =
          await client.loadSpan(cameraId: 'cam-1', start: t0, end: t1);
      expect(got.segments, hasLength(1));
      expect(client.lastCall, 'loadSpan');
    });

    test('loadSpan returns empty span for unknown camera', () async {
      final t0 = DateTime.utc(2026, 4, 8);
      final t1 = DateTime.utc(2026, 4, 9);
      final got =
          await client.loadSpan(cameraId: 'ghost', start: t0, end: t1);
      expect(got.segments, isEmpty);
      expect(got.start, t0);
      expect(got.end, t1);
    });

    test('mintPlaybackUrl produces a fake URL with the speed encoded',
        () async {
      final ticket =
          await client.mintPlaybackUrl(segmentId: 's1', playbackSpeed: 4.0);
      expect(ticket.url, contains('speed=4.0'));
      expect(ticket.expiresAt.isAfter(DateTime.now()), true);
    });

    test('createBookmark returns an incrementing id and records call',
        () async {
      final id1 =
          await client.createBookmark(segmentId: 's1', atMs: 1000, note: 'hi');
      final id2 = await client.createBookmark(segmentId: 's1', atMs: 2000);
      expect(id1, 'bookmark-1');
      expect(id2, 'bookmark-2');
      expect(client.lastCall, 'createBookmark');
      expect(client.lastArgs?['atMs'], 2000);
    });

    test('exportClip returns an incrementing clip id', () async {
      final id =
          await client.exportClip(segmentId: 's1', startMs: 0, endMs: 5000);
      expect(id, 'clip-1');
      expect(client.lastArgs?['endMs'], 5000);
    });

    test('failWith forces the next call to throw', () async {
      client.failWith = StateError('boom');
      expect(
        () => client.createBookmark(segmentId: 's1', atMs: 0),
        throwsA(isA<StateError>()),
      );
    });
  });
}
