// KAI-302 — ScrubBar widget tests.
import 'package:flutter/gestures.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:nvr_client/playback/timeline_model.dart';
import 'package:nvr_client/playback/widgets/scrub_bar.dart';

void main() {
  final t0 = DateTime.utc(2026, 4, 8, 10);
  final t1 = DateTime.utc(2026, 4, 8, 11);

  TimelineSpan buildSpan() => TimelineSpan(
        start: t0,
        end: t1,
        segments: [
          RecordingSegment(
            id: 's',
            cameraId: 'c',
            recorderId: 'rec',
            directoryConnectionId: 'dir',
            startedAt: t0,
            endedAt: t1,
          ),
        ],
        markers: const [],
      );

  Widget wrap(Widget child) => MaterialApp(
        home: Scaffold(
          body: Center(
            child: SizedBox(width: 400, height: 60, child: child),
          ),
        ),
      );

  testWidgets('tap emits onScrub at the tapped position', (tester) async {
    DateTime? got;
    await tester.pumpWidget(wrap(ScrubBar(
      span: buildSpan(),
      onScrub: (t) => got = t,
    )));

    final center = tester.getCenter(find.byType(ScrubBar));
    await tester.tapAt(center);
    await tester.pump();

    expect(got, isNotNull);
    final offset = got!.difference(t0);
    expect(offset.inMinutes, inInclusiveRange(25, 35));
  });

  testWidgets('horizontal drag updates scrub position', (tester) async {
    DateTime? latest;
    await tester.pumpWidget(wrap(ScrubBar(
      span: buildSpan(),
      onScrub: (t) => latest = t,
    )));

    final topLeft = tester.getTopLeft(find.byType(ScrubBar));
    await tester.dragFrom(topLeft + const Offset(10, 30), const Offset(200, 0));
    await tester.pump();

    expect(latest, isNotNull);
    expect(latest!.isAfter(t0), true);
  });

  testWidgets('zoom callback fires on scroll wheel', (tester) async {
    double? lastZoom;
    await tester.pumpWidget(wrap(ScrubBar(
      span: buildSpan(),
      onScrub: (_) {},
      onZoomChanged: (z) => lastZoom = z,
    )));

    final center = tester.getCenter(find.byType(ScrubBar));
    final pointer = TestPointer(1, PointerDeviceKind.mouse);
    pointer.hover(center);
    await tester.sendEventToBinding(pointer.scroll(const Offset(0, -10)));
    await tester.pump();

    expect(lastZoom, isNotNull);
    expect(lastZoom! > 1.0, true);
  });
}
