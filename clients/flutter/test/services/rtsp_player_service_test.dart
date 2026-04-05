import 'package:flutter_test/flutter_test.dart';
import 'package:nvr_client/services/rtsp_player_service.dart';

void main() {
  group('RtspConnectionState enum', () {
    test('has all expected values', () {
      expect(RtspConnectionState.values, hasLength(4));
      expect(RtspConnectionState.values, contains(RtspConnectionState.connecting));
      expect(RtspConnectionState.values, contains(RtspConnectionState.connected));
      expect(RtspConnectionState.values, contains(RtspConnectionState.failed));
      expect(RtspConnectionState.values, contains(RtspConnectionState.disposed));
    });
  });

  group('RtspConnection', () {
    test('initial state is connecting', () {
      final conn = RtspConnection(
        serverUrl: 'http://localhost:9997',
        mediamtxPath: 'cam1',
      );
      expect(conn.state, RtspConnectionState.connecting);
      expect(conn.serverUrl, 'http://localhost:9997');
      expect(conn.mediamtxPath, 'cam1');
      conn.dispose();
    });

    test('videoController is null before connect', () {
      final conn = RtspConnection(
        serverUrl: 'http://localhost:9997',
        mediamtxPath: 'cam1',
      );
      expect(conn.videoController, isNull);
      conn.dispose();
    });

    test('stateStream is a broadcast stream', () {
      final conn = RtspConnection(
        serverUrl: 'http://localhost:9997',
        mediamtxPath: 'cam1',
      );
      // Should allow multiple listeners (broadcast).
      final sub1 = conn.stateStream.listen((_) {});
      final sub2 = conn.stateStream.listen((_) {});
      sub1.cancel();
      sub2.cancel();
      conn.dispose();
    });

    test('dispose closes stateStream', () async {
      final conn = RtspConnection(
        serverUrl: 'http://localhost:9997',
        mediamtxPath: 'cam1',
      );

      await conn.dispose();

      // After dispose, listening to the stream should get a done event.
      bool streamDone = false;
      conn.stateStream.listen(
        (_) {},
        onDone: () => streamDone = true,
      );
      // The stream is already closed, so done fires synchronously.
      await Future.delayed(Duration.zero);
      expect(streamDone, isTrue);
    });
  });

  group('H265 codec detection (logic from camera_tile)', () {
    // This tests the same logic used in CameraTile._isH265
    bool isH265(String codec) {
      final upper = codec.toUpperCase();
      return upper == 'H265' || upper == 'HEVC';
    }

    test('detects H265', () {
      expect(isH265('H265'), isTrue);
      expect(isH265('h265'), isTrue);
    });

    test('detects HEVC', () {
      expect(isH265('HEVC'), isTrue);
      expect(isH265('hevc'), isTrue);
      expect(isH265('Hevc'), isTrue);
    });

    test('returns false for H264 and other codecs', () {
      expect(isH265('H264'), isFalse);
      expect(isH265('h264'), isFalse);
      expect(isH265('VP8'), isFalse);
      expect(isH265('VP9'), isFalse);
      expect(isH265('AV1'), isFalse);
      expect(isH265(''), isFalse);
    });
  });
}
