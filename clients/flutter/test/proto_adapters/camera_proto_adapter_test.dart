import 'package:flutter_test/flutter_test.dart';
import 'package:nvr_client/proto_adapters/camera_proto_adapter.dart';
import 'package:nvr_client/src/gen/proto/kaivue/v1/cameras.pb.dart';

void main() {
  group('cameraFromProto', () {
    test('maps basic fields', () {
      final pb = PbCamera(
        id: 'cam-1',
        name: 'Front Door',
        state: PbCameraState.online,
        config: PbCameraConfig(
          retention: PbRetentionPolicy(retentionDays: 14),
          profiles: [
            PbStreamProfile(name: 'main', codec: 'h264', width: 1920, height: 1080),
            PbStreamProfile(name: 'sub', codec: 'h264', width: 640, height: 480),
          ],
        ),
      );

      final camera = cameraFromProto(pb);

      expect(camera.id, 'cam-1');
      expect(camera.name, 'Front Door');
      expect(camera.status, 'connected');
      expect(camera.hasMainStream, true);
      expect(camera.hasSubStream, true);
      expect(camera.retentionDays, 14);
      expect(camera.streamPaths.length, 2);
      expect(camera.streamPaths[0].name, 'main');
      expect(camera.streamPaths[0].resolution, '1920x1080');
      expect(camera.streamPaths[0].videoCodec, 'h264');
    });

    test('maps offline state', () {
      final pb = PbCamera(
        id: 'cam-2',
        name: 'Parking',
        state: PbCameraState.offline,
      );
      expect(cameraFromProto(pb).status, 'disconnected');
    });

    test('maps provisioning state', () {
      final pb = PbCamera(
        id: 'cam-3',
        name: 'Lobby',
        state: PbCameraState.provisioning,
      );
      expect(cameraFromProto(pb).status, 'connecting');
    });

    test('maps error state', () {
      final pb = PbCamera(
        id: 'cam-4',
        name: 'Back Gate',
        state: PbCameraState.error,
      );
      expect(cameraFromProto(pb).status, 'error');
    });

    test('default config gives main stream but no sub', () {
      final pb = PbCamera(id: 'cam-5', name: 'No Config');
      final camera = cameraFromProto(pb);
      expect(camera.hasMainStream, true);
      expect(camera.hasSubStream, false);
    });

    test('camerasFromProto converts list', () {
      final cameras = camerasFromProto([
        PbCamera(id: 'a', name: 'A'),
        PbCamera(id: 'b', name: 'B'),
      ]);
      expect(cameras.length, 2);
      expect(cameras[0].id, 'a');
      expect(cameras[1].id, 'b');
    });

    test('extracts stream profiles as StreamPaths', () {
      final pb = PbCamera(
        id: 'cam-6',
        name: 'With Profiles',
        config: PbCameraConfig(
          profiles: [
            PbStreamProfile(
              name: 'main',
              codec: 'h265',
              width: 3840,
              height: 2160,
              url: 'rtsp://cam/main',
            ),
          ],
        ),
      );
      final camera = cameraFromProto(pb);
      expect(camera.streamPaths.length, 1);
      expect(camera.streamPaths[0].path, 'rtsp://cam/main');
      expect(camera.streamPaths[0].resolution, '3840x2160');
      expect(camera.streamPaths[0].videoCodec, 'h265');
    });

    test('retention defaults to 30 when no config', () {
      final pb = PbCamera(id: 'cam-7', name: 'No Retention');
      expect(cameraFromProto(pb).retentionDays, 30);
    });
  });
}
