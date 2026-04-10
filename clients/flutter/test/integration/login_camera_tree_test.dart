// Integration test: Login -> Camera Tree flow.
//
// Verifies that after authenticating via AppSessionNotifier (fake secure store),
// the FakeCameraDirectoryClient returns the correct camera list for the active
// connection, and that switching connections scopes cameras properly.

import 'package:flutter_test/flutter_test.dart';

import 'package:nvr_client/cameras/camera_directory_client.dart';
import 'package:nvr_client/models/camera.dart';
import 'package:nvr_client/state/app_session.dart';
import 'package:nvr_client/state/home_directory_connection.dart';
import 'package:nvr_client/state/secure_token_store.dart';

void main() {
  group('Login -> Camera Tree integration', () {
    late InMemorySecureTokenStore tokenStore;
    late AppSessionNotifier sessionNotifier;
    late FakeCameraDirectoryClient cameraClient;

    final connection = HomeDirectoryConnection(
      id: 'conn-1',
      kind: HomeConnectionKind.onPrem,
      endpointUrl: 'https://nvr.acme.local',
      displayName: 'Acme HQ',
      discoveryMethod: DiscoveryMethod.mdns,
    );

    final cameras = [
      const Camera(id: 'cam-1', name: 'Lobby'),
      const Camera(id: 'cam-2', name: 'Parking'),
      const Camera(id: 'cam-3', name: 'Server Room', ptzCapable: true),
    ];

    setUp(() {
      tokenStore = InMemorySecureTokenStore();
      sessionNotifier = AppSessionNotifier(tokenStore);
      cameraClient = FakeCameraDirectoryClient();
      cameraClient.setCameras(connection.id, cameras);
    });

    tearDown(() async {
      await cameraClient.dispose();
    });

    test('authenticate then list cameras returns seeded cameras', () async {
      // Step 1: activate connection (simulates post-login)
      await sessionNotifier.activateConnection(
        connection: connection,
        userId: 'user-1',
        tenantRef: 'tenant-acme',
      );

      // Step 2: set tokens (simulates login success)
      await sessionNotifier.setTokens(
        accessToken: 'eyJhbGciOiJSUzI1NiJ9.eyJzdWIiOiJ1c2VyLTEifQ.sig',
        refreshToken: 'refresh-tok',
      );

      expect(sessionNotifier.state.isAuthenticated, isTrue);
      expect(sessionNotifier.state.activeConnection, equals(connection));

      // Step 3: list cameras via the fake client
      final listed =
          await cameraClient.listCameras(sessionNotifier.state.activeConnection!);

      expect(listed, hasLength(3));
      expect(listed.map((c) => c.name).toList(), ['Lobby', 'Parking', 'Server Room']);
    });

    test('unauthenticated session has no active connection for camera lookup', () {
      expect(sessionNotifier.state.isAuthenticated, isFalse);
      expect(sessionNotifier.state.activeConnection, isNull);
    });

    test('camera list is scoped to the active connection', () async {
      final connection2 = HomeDirectoryConnection(
        id: 'conn-2',
        kind: HomeConnectionKind.cloud,
        endpointUrl: 'https://cloud.kaivue.io',
        displayName: 'Cloud',
        discoveryMethod: DiscoveryMethod.manual,
      );

      cameraClient.setCameras(connection2.id, [
        const Camera(id: 'cam-cloud-1', name: 'Cloud Cam'),
      ]);

      // Activate first connection
      await sessionNotifier.activateConnection(
        connection: connection,
        userId: 'user-1',
        tenantRef: 'tenant-acme',
      );
      await sessionNotifier.setTokens(
        accessToken: 'eyJhbGciOiJSUzI1NiJ9.eyJzdWIiOiJ1c2VyLTEifQ.sig',
        refreshToken: 'refresh-tok',
      );

      final acmeCams = await cameraClient.listCameras(
        sessionNotifier.state.activeConnection!,
      );
      expect(acmeCams, hasLength(3));

      // Switch to cloud connection
      await sessionNotifier.switchConnection(target: connection2);
      final cloudCams = await cameraClient.listCameras(
        sessionNotifier.state.activeConnection!,
      );
      expect(cloudCams, hasLength(1));
      expect(cloudCams.first.name, 'Cloud Cam');
    });

    test('camera status events arrive after login', () async {
      await sessionNotifier.activateConnection(
        connection: connection,
        userId: 'user-1',
        tenantRef: 'tenant-acme',
      );

      final events = <CameraStatusEvent>[];
      final sub = cameraClient
          .watchStatus(sessionNotifier.state.activeConnection!)
          .listen(events.add);

      cameraClient.pushStatus(
        connection.id,
        CameraStatusEvent(
          cameraId: 'cam-1',
          isOnline: true,
          lastSeen: DateTime.utc(2026, 4, 8, 12, 0),
        ),
      );

      cameraClient.pushStatus(
        connection.id,
        CameraStatusEvent(
          cameraId: 'cam-2',
          isOnline: false,
          lastSeen: DateTime.utc(2026, 4, 8, 12, 1),
          reason: 'ONVIF timeout',
        ),
      );

      // Allow microtask queue to flush.
      await Future<void>.delayed(Duration.zero);

      expect(events, hasLength(2));
      expect(events[0].cameraId, 'cam-1');
      expect(events[0].isOnline, isTrue);
      expect(events[1].cameraId, 'cam-2');
      expect(events[1].isOnline, isFalse);
      expect(events[1].reason, 'ONVIF timeout');

      await sub.cancel();
    });
  });
}
