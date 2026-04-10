// KAI-299 — CameraStatusNotifier merging tests.

import 'package:flutter_test/flutter_test.dart';
import 'package:nvr_client/cameras/camera_directory_client.dart';
import 'package:nvr_client/cameras/camera_status_notifier.dart';
import 'package:nvr_client/state/home_directory_connection.dart';

HomeDirectoryConnection _conn(String id) => HomeDirectoryConnection(
      id: id,
      kind: HomeConnectionKind.onPrem,
      endpointUrl: 'https://$id.example',
      displayName: id,
      discoveryMethod: DiscoveryMethod.manual,
    );

void main() {
  group('CameraStatusNotifier', () {
    late FakeCameraDirectoryClient fake;
    late CameraStatusNotifier notifier;

    setUp(() {
      fake = FakeCameraDirectoryClient();
      notifier = CameraStatusNotifier(fake);
    });

    tearDown(() async {
      notifier.dispose();
      await fake.dispose();
    });

    test('merges events from multiple tracked connections', () async {
      final home = _conn('home');
      final peerA = _conn('peer-a');
      notifier.track(home);
      notifier.track(peerA);

      fake.pushStatus(
        'home',
        CameraStatusEvent(
          cameraId: 'cam-1',
          isOnline: true,
          lastSeen: DateTime.utc(2026, 1, 1),
        ),
      );
      fake.pushStatus(
        'peer-a',
        CameraStatusEvent(
          cameraId: 'cam-2',
          isOnline: false,
          lastSeen: DateTime.utc(2026, 1, 2),
        ),
      );
      // Let microtasks drain.
      await Future<void>.delayed(Duration.zero);

      expect(notifier.state['cam-1']!.state, CameraOnlineState.online);
      expect(notifier.state['cam-2']!.state, CameraOnlineState.offline);
    });

    test('disconnect marks that peer\'s cameras unknown', () async {
      final peerA = _conn('peer-a');
      notifier.track(peerA);
      fake.pushStatus(
        'peer-a',
        CameraStatusEvent(
          cameraId: 'cam-2',
          isOnline: true,
          lastSeen: DateTime.utc(2026, 1, 1),
        ),
      );
      await Future<void>.delayed(Duration.zero);
      expect(notifier.state['cam-2']!.state, CameraOnlineState.online);

      await notifier.untrack('peer-a');
      expect(notifier.state['cam-2']!.state, CameraOnlineState.unknown);
    });

    test('reconnect restores real status', () async {
      final peerA = _conn('peer-a');
      notifier.track(peerA);
      fake.pushStatus(
        'peer-a',
        CameraStatusEvent(
          cameraId: 'cam-2',
          isOnline: true,
          lastSeen: DateTime.utc(2026, 1, 1),
        ),
      );
      await Future<void>.delayed(Duration.zero);
      await notifier.untrack('peer-a');
      expect(notifier.state['cam-2']!.state, CameraOnlineState.unknown);

      notifier.track(peerA);
      fake.pushStatus(
        'peer-a',
        CameraStatusEvent(
          cameraId: 'cam-2',
          isOnline: true,
          lastSeen: DateTime.utc(2026, 1, 3),
        ),
      );
      await Future<void>.delayed(Duration.zero);
      expect(notifier.state['cam-2']!.state, CameraOnlineState.online);
    });

    test('isOnline=null event maps to unknown', () async {
      final home = _conn('home');
      notifier.track(home);
      fake.pushStatus(
        'home',
        CameraStatusEvent(
          cameraId: 'cam-9',
          isOnline: null,
          lastSeen: DateTime.utc(2026, 1, 1),
          reason: 'no data',
        ),
      );
      await Future<void>.delayed(Duration.zero);
      expect(notifier.state['cam-9']!.state, CameraOnlineState.unknown);
      expect(notifier.state['cam-9']!.reason, 'no data');
    });
  });
}
