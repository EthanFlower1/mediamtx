// KAI-303 — Deep-link dispatcher tests: every PushMessageKind pins to a
// route constant so a future router wiring PR can't silently change the
// mapping.

import 'package:flutter_test/flutter_test.dart';
import 'package:nvr_client/notifications/deep_link.dart';
import 'package:nvr_client/notifications/push_message.dart';

PushMessage _msg(PushMessageKind k) => PushMessage(
      eventId: 'e',
      cameraId: 'c',
      kind: k,
      timestamp: DateTime.utc(2026, 4, 8),
      directoryConnectionId: 'home',
    );

void main() {
  group('routeForPushMessage', () {
    test('motion -> liveEvent', () {
      expect(routeForPushMessage(_msg(PushMessageKind.motion)),
          NotificationRoutes.liveEvent);
    });
    test('face -> faceEvent', () {
      expect(routeForPushMessage(_msg(PushMessageKind.face)),
          NotificationRoutes.faceEvent);
    });
    test('lpr -> lprEvent', () {
      expect(routeForPushMessage(_msg(PushMessageKind.lpr)),
          NotificationRoutes.lprEvent);
    });
    test('manual -> manualAlert', () {
      expect(routeForPushMessage(_msg(PushMessageKind.manual)),
          NotificationRoutes.manualAlert);
    });
    test('system -> systemNotice', () {
      expect(routeForPushMessage(_msg(PushMessageKind.system)),
          NotificationRoutes.systemNotice);
    });
  });
}
