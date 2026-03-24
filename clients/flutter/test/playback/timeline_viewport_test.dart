import 'package:flutter_test/flutter_test.dart';
import 'package:nvr_client/screens/playback/timeline/timeline_viewport.dart';

void main() {
  group('TimelineViewport', () {
    test('full 24h view maps midnight to 0 and end to width', () {
      final vp = TimelineViewport(
        visibleStart: Duration.zero,
        visibleEnd: const Duration(hours: 24),
        widthPx: 1000,
      );

      expect(vp.timeToPixel(Duration.zero), 0.0);
      expect(vp.timeToPixel(const Duration(hours: 24)), 1000.0);
      expect(vp.timeToPixel(const Duration(hours: 12)), 500.0);
    });

    test('pixelToTime is inverse of timeToPixel', () {
      final vp = TimelineViewport(
        visibleStart: Duration.zero,
        visibleEnd: const Duration(hours: 24),
        widthPx: 1000,
      );

      final time = const Duration(hours: 6, minutes: 30);
      final px = vp.timeToPixel(time);
      final roundTrip = vp.pixelToTime(px);

      expect(roundTrip.inSeconds, time.inSeconds);
    });

    test('zoomed view maps correctly', () {
      final vp = TimelineViewport(
        visibleStart: const Duration(hours: 10),
        visibleEnd: const Duration(hours: 14),
        widthPx: 800,
      );

      expect(vp.timeToPixel(const Duration(hours: 10)), 0.0);
      expect(vp.timeToPixel(const Duration(hours: 14)), 800.0);
      expect(vp.timeToPixel(const Duration(hours: 12)), 400.0);
      expect(vp.timeToPixel(const Duration(hours: 9)), lessThan(0));
    });

    test('zoomLevel computes correctly', () {
      final vp = TimelineViewport(
        visibleStart: const Duration(hours: 10),
        visibleEnd: const Duration(hours: 14),
        widthPx: 800,
      );

      expect(vp.zoomLevel, 6.0);
    });

    test('visibleDuration returns correct span', () {
      final vp = TimelineViewport(
        visibleStart: const Duration(hours: 3),
        visibleEnd: const Duration(hours: 9),
        widthPx: 600,
      );

      expect(vp.visibleDuration, const Duration(hours: 6));
    });

    test('gridInterval adapts to zoom level', () {
      final vp1x = TimelineViewport(
        visibleStart: Duration.zero,
        visibleEnd: const Duration(hours: 24),
        widthPx: 1000,
      );
      expect(vp1x.gridInterval, const Duration(hours: 3));

      final vp6x = TimelineViewport(
        visibleStart: const Duration(hours: 10),
        visibleEnd: const Duration(hours: 14),
        widthPx: 1000,
      );
      expect(vp6x.gridInterval, const Duration(minutes: 30));

      final vp24x = TimelineViewport(
        visibleStart: const Duration(hours: 10),
        visibleEnd: const Duration(hours: 11),
        widthPx: 1000,
      );
      expect(vp24x.gridInterval, const Duration(minutes: 5));
    });
  });
}
