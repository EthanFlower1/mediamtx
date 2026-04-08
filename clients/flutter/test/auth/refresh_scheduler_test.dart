// KAI-297 — RefreshScheduler tests.
//
// Verifies the lead-time math + that the scheduler executes its callback
// after the right delay using fake_async to drive virtual time. The login
// service is mocked via MockClient — no real network.

import 'dart:convert';

import 'package:fake_async/fake_async.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';

import 'package:nvr_client/auth/auth_types.dart';
import 'package:nvr_client/auth/login_service.dart';
import 'package:nvr_client/auth/refresh_scheduler.dart';
import 'package:nvr_client/state/app_session.dart';
import 'package:nvr_client/state/home_directory_connection.dart';

HomeDirectoryConnection _conn() => const HomeDirectoryConnection(
      id: 'conn-1',
      kind: HomeConnectionKind.onPrem,
      endpointUrl: 'https://nvr.acme.local',
      displayName: 'Acme HQ',
      discoveryMethod: DiscoveryMethod.manual,
    );

Map<String, dynamic> _payload({String tag = 'fresh', int expiresIn = 900}) => {
      'access_token': 'access-$tag',
      'refresh_token': 'refresh-$tag',
      'expires_in': expiresIn,
      'user': {'user_id': 'u-1', 'tenant_ref': 'tenant-1'},
    };

void main() {
  group('RefreshScheduler.computeDelay', () {
    test('15 min token → 10 min delay (5 min lead)', () {
      final now = DateTime(2026, 4, 7, 12, 0, 0);
      final exp = now.add(const Duration(minutes: 15));
      final d = RefreshScheduler.computeDelay(expiresAt: exp, now: now);
      expect(d, const Duration(minutes: 10));
    });

    test('expiry already inside lead window → minDelay floor', () {
      final now = DateTime(2026, 4, 7, 12, 0, 0);
      final exp = now.add(const Duration(seconds: 30));
      final d = RefreshScheduler.computeDelay(expiresAt: exp, now: now);
      expect(d, const Duration(seconds: 1));
    });
  });

  test('arm() schedules and fires refresh after the computed delay', () {
    fakeAsync((async) {
      final mock = MockClient((req) async {
        return http.Response(
          jsonEncode(_payload()),
          200,
          headers: {'content-type': 'application/json'},
        );
      });
      final svc = LoginService(httpClient: mock);
      final scheduler = RefreshScheduler(
        loginService: svc,
        binding: InMemoryBackgroundTaskBinding(),
        now: () => DateTime(2026, 4, 7, 12, 0, 0),
      );

      final session = AppSession(
        userId: 'u-1',
        tenantRef: 'tenant-1',
        accessToken: 'old-access',
        refreshToken: 'old-refresh',
        activeConnection: _conn(),
      );

      RefreshOutcome? outcome;
      scheduler.arm(
        session: session,
        expiresAt: DateTime(2026, 4, 7, 12, 15, 0),
        onOutcome: (o) => outcome = o,
      );

      expect(scheduler.isArmed, isTrue);

      // Just before the deadline — nothing fired yet.
      async.elapse(const Duration(minutes: 9, seconds: 59));
      expect(outcome, isNull);

      // Cross the deadline.
      async.elapse(const Duration(seconds: 2));
      async.flushMicrotasks();
      expect(outcome, isA<RefreshSuccess>());
      final ok = outcome as RefreshSuccess;
      expect(ok.result.accessToken, 'access-fresh');
    });
  });

  test('arm() surfaces refresh expiry as RefreshFailure', () {
    fakeAsync((async) {
      final mock = MockClient((_) async => http.Response('expired', 401));
      final svc = LoginService(httpClient: mock);
      final scheduler = RefreshScheduler(
        loginService: svc,
        binding: InMemoryBackgroundTaskBinding(),
        now: () => DateTime(2026, 4, 7, 12, 0, 0),
      );

      final session = AppSession(
        userId: 'u-1',
        tenantRef: 'tenant-1',
        refreshToken: 'expired-token',
        activeConnection: _conn(),
      );

      RefreshOutcome? outcome;
      scheduler.arm(
        session: session,
        expiresAt: DateTime(2026, 4, 7, 12, 15, 0),
        onOutcome: (o) => outcome = o,
      );

      async.elapse(const Duration(minutes: 11));
      async.flushMicrotasks();
      expect(outcome, isA<RefreshFailure>());
      final fail = outcome as RefreshFailure;
      expect(fail.error.kind, LoginErrorKind.refreshExpired);
    });
  });

  test('cancel() prevents the callback from firing', () {
    fakeAsync((async) {
      final mock = MockClient((_) async {
        fail('refresh should have been cancelled');
      });
      final svc = LoginService(httpClient: mock);
      final scheduler = RefreshScheduler(
        loginService: svc,
        binding: InMemoryBackgroundTaskBinding(),
        now: () => DateTime(2026, 4, 7, 12, 0, 0),
      );

      final session = AppSession(
        userId: 'u-1',
        tenantRef: 'tenant-1',
        refreshToken: 'r',
        activeConnection: _conn(),
      );

      scheduler.arm(
        session: session,
        expiresAt: DateTime(2026, 4, 7, 12, 15, 0),
        onOutcome: (_) {},
      );
      scheduler.cancel();
      async.elapse(const Duration(minutes: 30));
      async.flushMicrotasks();
      expect(scheduler.isArmed, isFalse);
    });
  });
}
