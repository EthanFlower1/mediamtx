// KAI-301 — GridTelemetryReporter tests.

import 'package:fake_async/fake_async.dart';
import 'package:flutter_test/flutter_test.dart';

import 'package:nvr_client/grid/render_mode.dart';
import 'package:nvr_client/grid/telemetry.dart';

void main() {
  group('GridTelemetryReporter', () {
    test('sampleOnce emits an event with the current snapshot', () async {
      final events = <GridTelemetryEvent>[];
      final reporter = GridTelemetryReporter(
        snapshotProvider: () => const GridTelemetrySnapshot(
          cellCount: 9,
          renderMode: RenderMode.snapshotRefresh,
          alwaysLiveOverride: false,
          isOnLan: true,
        ),
        cadence: const Duration(seconds: 5),
        batteryReader: () async => 77,
        onEvent: events.add,
      );
      await reporter.sampleOnce();
      expect(events, hasLength(1));
      expect(events.first.cellCount, 9);
      expect(events.first.renderMode, RenderMode.snapshotRefresh);
      expect(events.first.alwaysLiveOverride, false);
      expect(events.first.isOnLan, true);
      expect(events.first.batteryPercent, 77);
      reporter.dispose();
    });

    test('start + cadence emits events at the expected interval', () {
      fakeAsync((async) {
        int cellCount = 4;
        final events = <GridTelemetryEvent>[];
        final reporter = GridTelemetryReporter(
          snapshotProvider: () => GridTelemetrySnapshot(
            cellCount: cellCount,
            renderMode: RenderMode.webrtc,
            alwaysLiveOverride: false,
            isOnLan: false,
          ),
          cadence: const Duration(seconds: 5),
          onEvent: events.add,
        );
        reporter.start();
        async.elapse(const Duration(seconds: 5));
        async.flushMicrotasks();
        expect(events.length, 1);
        cellCount = 16;
        async.elapse(const Duration(seconds: 5));
        async.flushMicrotasks();
        expect(events.length, 2);
        expect(events[1].cellCount, 16);
        reporter.dispose();
      });
    });

    test('battery reader null is preserved', () async {
      final events = <GridTelemetryEvent>[];
      final reporter = GridTelemetryReporter(
        snapshotProvider: () => const GridTelemetrySnapshot(
          cellCount: 1,
          renderMode: RenderMode.webrtc,
          alwaysLiveOverride: false,
          isOnLan: true,
        ),
        onEvent: events.add,
      );
      await reporter.sampleOnce();
      expect(events.single.batteryPercent, isNull);
      reporter.dispose();
    });
  });
}
