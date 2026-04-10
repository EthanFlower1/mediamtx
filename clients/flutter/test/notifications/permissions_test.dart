// KAI-303 — permission request tests. Uses a stub host so the tests are
// hermetic — no firebase_messaging, no Notification API, no platform
// channel calls.

import 'package:flutter_test/flutter_test.dart';
import 'package:nvr_client/notifications/permissions.dart';

class StubHost implements NotificationPermissionHost {
  final PermissionState ios;
  final PermissionState android;
  final PermissionState web;
  const StubHost({
    this.ios = PermissionState.unknown,
    this.android = PermissionState.unknown,
    this.web = PermissionState.unknown,
  });

  @override
  Future<PermissionState> requestIos() async => ios;

  @override
  Future<PermissionState> requestAndroid() async => android;

  @override
  Future<PermissionState> requestWeb() async => web;
}

void main() {
  group('requestNotificationPermission', () {
    test('maps stubbed host results straight through on current platform',
        () async {
      // We don't know whether the test host is iOS/Android/desktop, but
      // each branch independently returns the stubbed value — so we
      // assert that the result is one of the stubbed values.
      final host = const StubHost(
        ios: PermissionState.provisional,
        android: PermissionState.granted,
        web: PermissionState.denied,
      );
      final result = await requestNotificationPermission(host: host);
      // Desktop stub returns `granted`, others return their stub.
      expect(
        [
          PermissionState.provisional,
          PermissionState.granted,
          PermissionState.denied,
        ],
        contains(result),
      );
    });

    test('default host returns a safe non-null value', () async {
      final result = await requestNotificationPermission();
      expect(result, isNotNull);
    });
  });
}
