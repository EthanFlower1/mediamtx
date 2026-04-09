// KAI-301 — SnapshotRefreshController tests.
//
// Covers: start() fetches each camera once immediately; pause/resume honors
// lifecycle; jitter window respects min/max bounds.

import 'dart:math' as math;
import 'dart:typed_data';

import 'package:fake_async/fake_async.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:nvr_client/grid/camera.dart';
import 'package:nvr_client/grid/snapshot_refresh_controller.dart';
import 'package:nvr_client/grid/stream_url_minter.dart';

Camera _cam(String id) => Camera(
      id: id,
      directoryConnectionId: 'conn-1',
      label: 'Cam $id',
      siteLabel: 'Site',
      snapshotUrl: 'https://fake/$id.jpg',
      mainStreamWebRtcEndpoint: 'https://fake/whep/$id',
      thumbnailUrl: 'https://fake/$id.thumb',
      isOnline: true,
    );

void main() {
  group('SnapshotRefreshController', () {
    test('start emits an initial frame per camera', () async {
      final minter = FakeStreamUrlMinter();
      final fetched = <String>[];
      final controller = SnapshotRefreshController(
        minter: minter,
        fetcher: (t) async {
          fetched.add(t.url);
          return Uint8List.fromList([1, 2, 3]);
        },
        rng: math.Random(42),
      );
      final frames = <SnapshotFrame>[];
      final sub = controller.frames.listen(frames.add);
      controller.start([_cam('a'), _cam('b')]);
      await Future<void>.delayed(const Duration(milliseconds: 50));
      await sub.cancel();
      controller.dispose();
      expect(minter.snapshotMintCount, greaterThanOrEqualTo(2));
      expect(frames.map((f) => f.cameraId).toSet(), {'a', 'b'});
      expect(fetched.length, greaterThanOrEqualTo(2));
    });

    test('jitter window honors min/max bounds', () {
      final minter = FakeStreamUrlMinter();
      final controller = SnapshotRefreshController(
        minter: minter,
        fetcher: (t) async => Uint8List(0),
        minPeriod: const Duration(seconds: 2),
        maxPeriod: const Duration(seconds: 5),
        rng: math.Random(7),
      );
      controller.start([
        _cam('a'),
        _cam('b'),
        _cam('c'),
        _cam('d'),
      ]);
      final periods = controller.currentPeriods.values.toList();
      expect(periods, hasLength(4));
      for (final p in periods) {
        expect(p.inMilliseconds, greaterThanOrEqualTo(2000));
        expect(p.inMilliseconds, lessThanOrEqualTo(5000));
      }
      controller.dispose();
    });

    test('pause stops ticking; resume re-arms', () {
      fakeAsync((async) {
        final minter = FakeStreamUrlMinter();
        final controller = SnapshotRefreshController(
          minter: minter,
          fetcher: (t) async => Uint8List(0),
          minPeriod: const Duration(seconds: 2),
          maxPeriod: const Duration(seconds: 2),
          rng: math.Random(0),
        );
        final tickedFor = <String>[];
        controller.onTick = tickedFor.add;
        controller.start([_cam('a')]);
        // Immediate first fetch fires synchronously.
        expect(tickedFor, ['a']);
        async.elapse(const Duration(seconds: 2));
        async.flushMicrotasks();
        expect(tickedFor.length, 2);

        controller.pauseForLifecycle();
        expect(controller.isPaused, true);
        async.elapse(const Duration(seconds: 10));
        async.flushMicrotasks();
        // No new ticks while paused.
        expect(tickedFor.length, 2);

        controller.resumeFromLifecycle();
        // Resume triggers an immediate re-fetch.
        expect(tickedFor.length, 3);
        async.elapse(const Duration(seconds: 2));
        async.flushMicrotasks();
        expect(tickedFor.length, 4);

        controller.dispose();
      });
    });
  });
}
