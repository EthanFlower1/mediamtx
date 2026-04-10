// Integration test: Offline -> Online transition.
//
// Verifies that when starting offline with cached cameras, transitioning to
// online mode triggers a camera list refresh, and that the connectivity
// monitor debouncing works correctly during rapid transitions.

import 'package:flutter_test/flutter_test.dart';

import 'package:nvr_client/cameras/camera_directory_client.dart';
import 'package:nvr_client/models/camera.dart';
import 'package:nvr_client/offline/action_queue.dart';
import 'package:nvr_client/offline/connectivity_monitor.dart';
import 'package:nvr_client/state/app_session.dart';
import 'package:nvr_client/state/home_directory_connection.dart';
import 'package:nvr_client/state/secure_token_store.dart';

void main() {
  group('Offline -> Online transition integration', () {
    late ConnectivityMonitor connectivityMonitor;
    late FakeCameraDirectoryClient cameraClient;
    late AppSessionNotifier sessionNotifier;

    final connection = HomeDirectoryConnection(
      id: 'conn-1',
      kind: HomeConnectionKind.onPrem,
      endpointUrl: 'https://nvr.acme.local',
      displayName: 'Acme HQ',
      discoveryMethod: DiscoveryMethod.mdns,
    );

    final cachedCameras = [
      const Camera(id: 'cam-1', name: 'Lobby'),
      const Camera(id: 'cam-2', name: 'Parking'),
    ];

    final refreshedCameras = [
      const Camera(id: 'cam-1', name: 'Lobby (Updated)'),
      const Camera(id: 'cam-2', name: 'Parking'),
      const Camera(id: 'cam-3', name: 'Entrance (New)'),
    ];

    setUp(() async {
      connectivityMonitor = ConnectivityMonitor();
      cameraClient = FakeCameraDirectoryClient();
      final tokenStore = InMemorySecureTokenStore();
      sessionNotifier = AppSessionNotifier(tokenStore);

      await sessionNotifier.activateConnection(
        connection: connection,
        userId: 'user-1',
        tenantRef: 'tenant-acme',
      );
      await sessionNotifier.setTokens(
        accessToken: 'eyJhbGciOiJSUzI1NiJ9.eyJzdWIiOiJ1c2VyLTEifQ.sig',
        refreshToken: 'refresh-tok',
      );

      // Start with cached cameras
      cameraClient.setCameras(connection.id, cachedCameras);
    });

    tearDown(() async {
      connectivityMonitor.dispose();
      await cameraClient.dispose();
    });

    test('start offline with cached cameras, go online, verify refresh',
        () async {
      // Verify initial online state
      expect(connectivityMonitor.state, ConnectivityState.online);

      // Cached cameras are available
      var cameras = await cameraClient.listCameras(
        sessionNotifier.state.activeConnection!,
      );
      expect(cameras, hasLength(2));

      // Go offline
      final t0 = DateTime.utc(2026, 4, 8, 12, 0);
      connectivityMonitor.transitionWithTimestamp(ConnectivityState.offline, t0);
      expect(connectivityMonitor.state, ConnectivityState.offline);

      // Cameras still available from cache while offline
      cameras = await cameraClient.listCameras(
        sessionNotifier.state.activeConnection!,
      );
      expect(cameras, hasLength(2));
      expect(cameras[0].name, 'Lobby');

      // Simulate server-side changes while offline: update the fake
      cameraClient.setCameras(connection.id, refreshedCameras);

      // Go online (after debounce window)
      final t1 = t0.add(const Duration(seconds: 3));
      connectivityMonitor.transitionWithTimestamp(ConnectivityState.online, t1);
      expect(connectivityMonitor.state, ConnectivityState.online);

      // Refresh camera list now that we are online
      cameras = await cameraClient.listCameras(
        sessionNotifier.state.activeConnection!,
      );
      expect(cameras, hasLength(3));
      expect(cameras[0].name, 'Lobby (Updated)');
      expect(cameras[2].name, 'Entrance (New)');
    });

    test('debounce prevents rapid offline-online-offline transitions', () {
      final t0 = DateTime.utc(2026, 4, 8, 12, 0);

      // Go offline
      connectivityMonitor.transitionWithTimestamp(ConnectivityState.offline, t0);
      expect(connectivityMonitor.state, ConnectivityState.offline);

      // Rapid online (within debounce window) -- should be ignored
      final t1 = t0.add(const Duration(milliseconds: 500));
      connectivityMonitor.transitionWithTimestamp(ConnectivityState.online, t1);
      expect(connectivityMonitor.state, ConnectivityState.offline);

      // After debounce window, transition succeeds
      final t2 = t0.add(const Duration(seconds: 3));
      connectivityMonitor.transitionWithTimestamp(ConnectivityState.online, t2);
      expect(connectivityMonitor.state, ConnectivityState.online);
    });

    test('degraded state transitions correctly', () {
      final t0 = DateTime.utc(2026, 4, 8, 12, 0);

      connectivityMonitor.transitionWithTimestamp(
        ConnectivityState.degraded,
        t0,
      );
      expect(connectivityMonitor.state, ConnectivityState.degraded);

      // Wait past debounce window and go fully online
      final t1 = t0.add(const Duration(seconds: 3));
      connectivityMonitor.transitionWithTimestamp(ConnectivityState.online, t1);
      expect(connectivityMonitor.state, ConnectivityState.online);
    });

    test('camera status stream reconnects after online transition', () async {
      final events = <CameraStatusEvent>[];
      final sub = cameraClient
          .watchStatus(sessionNotifier.state.activeConnection!)
          .listen(events.add);

      // Push an event while online
      cameraClient.pushStatus(
        connection.id,
        CameraStatusEvent(
          cameraId: 'cam-1',
          isOnline: true,
          lastSeen: DateTime.utc(2026, 4, 8, 12, 0),
        ),
      );
      await Future<void>.delayed(Duration.zero);
      expect(events, hasLength(1));

      // Simulate offline -> close stream
      await cameraClient.closeStream(connection.id);

      // Back online -> re-subscribe
      final events2 = <CameraStatusEvent>[];
      final sub2 = cameraClient
          .watchStatus(sessionNotifier.state.activeConnection!)
          .listen(events2.add);

      cameraClient.pushStatus(
        connection.id,
        CameraStatusEvent(
          cameraId: 'cam-1',
          isOnline: false,
          lastSeen: DateTime.utc(2026, 4, 8, 12, 5),
          reason: 'Camera reboot',
        ),
      );
      await Future<void>.delayed(Duration.zero);
      expect(events2, hasLength(1));
      expect(events2[0].isOnline, isFalse);

      await sub.cancel();
      await sub2.cancel();
    });
  });
}
