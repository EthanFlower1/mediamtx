import 'package:flutter_test/flutter_test.dart';
import 'package:nvr_client/src/gen/proto/kaivue/v1/auth.pb.dart';
import 'package:nvr_client/src/gen/proto/kaivue/v1/cameras.pb.dart';
import 'package:nvr_client/src/gen/proto/kaivue/v1/recorder_control.pb.dart';
import 'package:nvr_client/src/gen/proto/kaivue/v1/streams.pb.dart';

void main() {
  group('PbTenantRef', () {
    test('roundtrips through JSON', () {
      const ref = PbTenantRef(type: PbTenantType.customer, id: 'cust-42');
      final json = ref.toJson();
      final decoded = PbTenantRef.fromJson(json);
      expect(decoded.type, PbTenantType.customer);
      expect(decoded.id, 'cust-42');
    });

    test('defaults', () {
      const ref = PbTenantRef();
      expect(ref.type, PbTenantType.unspecified);
      expect(ref.id, '');
    });
  });

  group('PbCamera', () {
    test('roundtrips through JSON', () {
      final cam = PbCamera(
        id: 'cam-1',
        name: 'Lobby',
        recorderId: 'rec-1',
        state: PbCameraState.online,
        configVersion: 5,
        labels: ['floor-1', 'entrance'],
      );
      final json = cam.toJson();
      final decoded = PbCamera.fromJson(json);
      expect(decoded.id, 'cam-1');
      expect(decoded.name, 'Lobby');
      expect(decoded.recorderId, 'rec-1');
      expect(decoded.state, PbCameraState.online);
      expect(decoded.configVersion, 5);
      expect(decoded.labels, ['floor-1', 'entrance']);
    });

    test('nested config roundtrips', () {
      final cam = PbCamera(
        id: 'cam-2',
        name: 'Parking',
        config: PbCameraConfig(
          defaultMode: PbRecordingMode.continuous,
          retention: PbRetentionPolicy(retentionDays: 14, maxBytes: 1073741824),
          profiles: [
            PbStreamProfile(name: 'main', codec: 'h264', width: 1920, height: 1080),
          ],
          motionSensitivity: 70,
          audioEnabled: true,
        ),
      );
      final json = cam.toJson();
      final decoded = PbCamera.fromJson(json);
      expect(decoded.config, isNotNull);
      expect(decoded.config!.defaultMode, PbRecordingMode.continuous);
      expect(decoded.config!.retention!.retentionDays, 14);
      expect(decoded.config!.profiles.length, 1);
      expect(decoded.config!.motionSensitivity, 70);
      expect(decoded.config!.audioEnabled, true);
    });
  });

  group('PbListCamerasResponse', () {
    test('parses response with cameras', () {
      final resp = PbListCamerasResponse.fromJson({
        'cameras': [
          {'id': 'c1', 'name': 'Cam 1'},
          {'id': 'c2', 'name': 'Cam 2'},
        ],
        'next_cursor': 'page2',
      });
      expect(resp.cameras.length, 2);
      expect(resp.nextCursor, 'page2');
    });

    test('handles empty response', () {
      final resp = PbListCamerasResponse.fromJson({});
      expect(resp.cameras, isEmpty);
      expect(resp.nextCursor, '');
    });
  });

  group('PbStreamClaims', () {
    test('parses from JSON', () {
      final claims = PbStreamClaims.fromJson({
        'user_id': 'user-1',
        'camera_id': 'cam-1',
        'recorder_id': 'rec-1',
        'kind': 1,
        'protocol': 1,
        'nonce': 'abc-123',
      });
      expect(claims.userId, 'user-1');
      expect(claims.cameraId, 'cam-1');
      expect(claims.kind, 1);
      expect(claims.protocol, PbStreamProtocol.webrtc);
      expect(claims.nonce, 'abc-123');
    });
  });

  group('PbMintStreamURLRequest', () {
    test('serializes to JSON', () {
      const req = PbMintStreamURLRequest(
        cameraId: 'cam-1',
        requestedKind: 3, // live | playback
        preferredProtocol: PbStreamProtocol.webrtc,
        maxTtlSeconds: 300,
      );
      final json = req.toJson();
      expect(json['camera_id'], 'cam-1');
      expect(json['requested_kind'], 3);
      expect(json['preferred_protocol'], 1);
      expect(json['max_ttl_seconds'], 300);
    });
  });

  group('PbAssignmentEvent', () {
    test('parses snapshot event', () {
      final event = PbAssignmentEvent.fromJson({
        'version': 42,
        'snapshot': {
          'cameras': [
            {'id': 'cam-1', 'name': 'Lobby'},
          ],
        },
      });
      expect(event.kind, AssignmentEventKind.snapshot);
      expect(event.version, 42);
      expect(event.snapshot, isNotNull);
      expect(event.snapshot!.cameras.length, 1);
    });

    test('parses camera_added event', () {
      final event = PbAssignmentEvent.fromJson({
        'version': 43,
        'camera_added': {
          'camera': {'id': 'cam-2', 'name': 'Parking'},
        },
      });
      expect(event.kind, AssignmentEventKind.cameraAdded);
      expect(event.cameraAdded!.camera!.id, 'cam-2');
    });

    test('parses camera_removed event', () {
      final event = PbAssignmentEvent.fromJson({
        'version': 44,
        'camera_removed': {
          'camera_id': 'cam-3',
          'purge_recordings': true,
          'reason': 'decommissioned',
        },
      });
      expect(event.kind, AssignmentEventKind.cameraRemoved);
      expect(event.cameraRemoved!.cameraId, 'cam-3');
      expect(event.cameraRemoved!.purgeRecordings, true);
    });

    test('parses heartbeat event', () {
      final event = PbAssignmentEvent.fromJson({
        'version': 45,
        'heartbeat': {'server_time': '2026-04-08T12:00:00Z'},
      });
      expect(event.kind, AssignmentEventKind.heartbeat);
      expect(event.heartbeat, isNotNull);
    });

    test('defaults to heartbeat for empty event', () {
      final event = PbAssignmentEvent.fromJson({'version': 0});
      expect(event.kind, AssignmentEventKind.heartbeat);
    });
  });
}
