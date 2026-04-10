// KAI-301 — Render mode decision matrix tests.
//
// Eight-case matrix covers the cellCount/isOnLan/alwaysLive boolean axes.

import 'package:flutter_test/flutter_test.dart';
import 'package:nvr_client/grid/render_mode.dart';

void main() {
  group('decideRenderMode', () {
    test('off-LAN, 4 cells, no override -> webrtc (boundary)', () {
      expect(
        decideRenderMode(
          cellCount: 4,
          alwaysLiveOverride: false,
          isOnLan: false,
        ),
        RenderMode.webrtc,
      );
    });

    test('off-LAN, 5 cells, no override -> snapshotRefresh', () {
      expect(
        decideRenderMode(
          cellCount: 5,
          alwaysLiveOverride: false,
          isOnLan: false,
        ),
        RenderMode.snapshotRefresh,
      );
    });

    test('off-LAN, 9 cells, override -> webrtc', () {
      expect(
        decideRenderMode(
          cellCount: 9,
          alwaysLiveOverride: true,
          isOnLan: false,
        ),
        RenderMode.webrtc,
      );
    });

    test('off-LAN, 16 cells, override -> webrtc', () {
      expect(
        decideRenderMode(
          cellCount: 16,
          alwaysLiveOverride: true,
          isOnLan: false,
        ),
        RenderMode.webrtc,
      );
    });

    test('on-LAN, 9 cells, no override -> webrtc (boundary)', () {
      expect(
        decideRenderMode(
          cellCount: 9,
          alwaysLiveOverride: false,
          isOnLan: true,
        ),
        RenderMode.webrtc,
      );
    });

    test('on-LAN, 10 cells, no override -> snapshotRefresh', () {
      expect(
        decideRenderMode(
          cellCount: 10,
          alwaysLiveOverride: false,
          isOnLan: true,
        ),
        RenderMode.snapshotRefresh,
      );
    });

    test('on-LAN, 16 cells, no override -> snapshotRefresh', () {
      expect(
        decideRenderMode(
          cellCount: 16,
          alwaysLiveOverride: false,
          isOnLan: true,
        ),
        RenderMode.snapshotRefresh,
      );
    });

    test('on-LAN, 16 cells, override -> webrtc', () {
      expect(
        decideRenderMode(
          cellCount: 16,
          alwaysLiveOverride: true,
          isOnLan: true,
        ),
        RenderMode.webrtc,
      );
    });

    test('zero cells is always webrtc (no work to do)', () {
      expect(
        decideRenderMode(
          cellCount: 0,
          alwaysLiveOverride: false,
          isOnLan: false,
        ),
        RenderMode.webrtc,
      );
    });

    test('constants match ticket thresholds', () {
      expect(kMaxWebRtcCellsOffLan, 4);
      expect(kMaxWebRtcCellsOnLan, 9);
      expect(kMinSnapshotJitterSeconds, 2.0);
      expect(kMaxSnapshotJitterSeconds, 5.0);
    });
  });
}
