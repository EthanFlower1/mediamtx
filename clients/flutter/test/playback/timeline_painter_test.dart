// KAI-302 — TimelinePainter smoke test.
//
// We don't have a PictureRecorder/canvas capture harness set up in this
// project, so we assert via `shouldRepaint` + paint-does-not-throw.
import 'dart:ui' as ui;

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:nvr_client/playback/timeline_model.dart';
import 'package:nvr_client/playback/timeline_painter.dart';

void main() {
  final t0 = DateTime.utc(2026, 4, 8, 10);
  final t1 = DateTime.utc(2026, 4, 8, 11);
  final t2 = DateTime.utc(2026, 4, 8, 12);

  TimelineSpan _span() => TimelineSpan(
        start: t0,
        end: t2,
        segments: [
          RecordingSegment(
            id: 'a',
            cameraId: 'c',
            recorderId: 'rec-A',
            directoryConnectionId: 'dir-home',
            startedAt: t0,
            endedAt: t1,
          ),
          RecordingSegment(
            id: 'b',
            cameraId: 'c',
            recorderId: 'rec-B',
            directoryConnectionId: 'dir-cloud',
            startedAt: t1,
            endedAt: t2,
            hasGap: true,
          ),
        ],
        markers: [
          EventMarker(
              id: 'e1', cameraId: 'c', kind: EventKind.motion, at: t0),
          EventMarker(
              id: 'e2',
              cameraId: 'c',
              kind: EventKind.face,
              at: t1,
              durationMs: 5000),
          EventMarker(id: 'e3', cameraId: 'c', kind: EventKind.lpr, at: t2),
        ],
      );

  test('paint executes without throwing', () {
    final recorder = ui.PictureRecorder();
    final canvas = Canvas(recorder);
    final painter = TimelinePainter(span: _span());
    expect(() => painter.paint(canvas, const Size(400, 60)), returnsNormally);
    recorder.endRecording();
  });

  test('shouldRepaint returns true when span changes', () {
    final a = TimelinePainter(span: _span());
    final b = TimelinePainter(
        span: TimelineSpan.empty(t0, t2));
    expect(a.shouldRepaint(b), true);
  });

  test('shouldRepaint returns false when span+zoom unchanged', () {
    final span = _span();
    final a = TimelinePainter(span: span);
    final b = TimelinePainter(span: span);
    expect(a.shouldRepaint(b), false);
  });

  test('palette maps each kind to a distinct color', () {
    const p = TimelinePalette();
    final seen = {
      p.forKind(EventKind.motion),
      p.forKind(EventKind.face),
      p.forKind(EventKind.lpr),
      p.forKind(EventKind.manual),
      p.forKind(EventKind.system),
    };
    expect(seen.length, 5);
  });
}
