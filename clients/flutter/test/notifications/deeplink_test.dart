// KAI-303 — Deep-link dispatcher tests: routing by priority bucket only,
// because the opaque payload has no kind.

import 'package:flutter_test/flutter_test.dart';
import 'package:nvr_client/notifications/deep_link.dart';
import 'package:nvr_client/notifications/push_message.dart';

PushMessage _msg(int priority) => PushMessage(
      eventId: 'e',
      tenantId: 't',
      priority: priority,
    );

void main() {
  group('routeForPushMessage (priority-only routing)', () {
    test('low -> alerts', () {
      expect(routeForPushMessage(_msg(PushPriority.low)),
          NotificationRoutes.alerts);
    });
    test('normal -> alerts', () {
      expect(routeForPushMessage(_msg(PushPriority.normal)),
          NotificationRoutes.alerts);
    });
    test('high -> alerts', () {
      expect(routeForPushMessage(_msg(PushPriority.high)),
          NotificationRoutes.alerts);
    });
    test('critical -> criticalAlerts', () {
      expect(routeForPushMessage(_msg(PushPriority.critical)),
          NotificationRoutes.criticalAlerts);
    });
    test('numeric overrides — anything >= 3 is critical', () {
      expect(routeForPushMessage(_msg(5)), NotificationRoutes.criticalAlerts);
    });
  });
}
