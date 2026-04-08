// KAI-297 — LoginService tests.
//
// Mocks every HTTP call via `package:http/testing.dart` MockClient. Mocks the
// SSO authorizer via FakeSsoAuthorizer. No real network, no real platform
// plugins. Covers (≥12 tests across this file + multi_connection_isolation
// + refresh_scheduler):
//
//   1. local login happy path
//   2. local login wrong-password → LoginErrorKind.wrongCredentials
//   3. local login network failure → LoginErrorKind.network
//   4. local login server 500 → LoginErrorKind.server
//   5. local login malformed body → LoginErrorKind.malformed
//   6. beginLogin happy → AvailableAuthMethods
//   7. beginLogin missing endpoint (404) → LoginErrorKind.malformed
//   8. SSO begin → completeSso happy path
//   9. SSO cancelled → SsoFlow.cancelled true, completeSso refuses
//  10. SSO unknown provider → LoginErrorKind.unknownProvider
//  11. refresh happy path
//  12. refresh expired → LoginErrorKind.refreshExpired

import 'dart:convert';
import 'dart:io' show SocketException;

import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';

import 'package:nvr_client/auth/auth_types.dart';
import 'package:nvr_client/auth/login_service.dart';
import 'package:nvr_client/auth/sso_authorizer.dart';
import 'package:nvr_client/state/app_session.dart';
import 'package:nvr_client/state/home_directory_connection.dart';

HomeDirectoryConnection _conn({String id = 'conn-1'}) => HomeDirectoryConnection(
      id: id,
      kind: HomeConnectionKind.onPrem,
      endpointUrl: 'https://nvr.acme.local',
      displayName: 'Acme HQ',
      discoveryMethod: DiscoveryMethod.manual,
    );

Map<String, dynamic> _validLoginResultPayload() => {
      'access_token': 'access-xyz',
      'refresh_token': 'refresh-xyz',
      'expires_in': 900,
      'user': {
        'user_id': 'u-1',
        'tenant_ref': 'tenant-1',
        'email': 'alice@example.com',
        'display_name': 'Alice',
      },
    };

Map<String, dynamic> _validAuthMethodsPayload() => {
      'local_enabled': true,
      'sso_providers': [
        {
          'id': 'google',
          'display_name': 'Google',
          'issuer_url': 'https://accounts.google.com',
          'client_id': 'client-google',
          'scopes': ['openid', 'email'],
        },
        {
          'id': 'azure',
          'display_name': 'Azure AD',
          'issuer_url': 'https://login.microsoftonline.com/tid',
          'client_id': 'client-azure',
          'scopes': ['openid', 'profile'],
        },
      ],
    };

void main() {
  group('LoginService.beginLogin', () {
    test('happy → AvailableAuthMethods with both providers', () async {
      final mock = MockClient((req) async {
        expect(req.method, 'GET');
        expect(req.url.path, '/api/v1/auth/methods');
        return http.Response(
          jsonEncode(_validAuthMethodsPayload()),
          200,
          headers: {'content-type': 'application/json'},
        );
      });
      final svc = LoginService(httpClient: mock);
      final methods = await svc.beginLogin(_conn());
      expect(methods.localEnabled, isTrue);
      expect(methods.ssoProviders.length, 2);
      expect(methods.ssoProviders.first.id, 'google');
      expect(methods.hasSso, isTrue);
    });

    test('404 → LoginErrorKind.malformed', () async {
      final mock = MockClient((_) async => http.Response('not found', 404));
      final svc = LoginService(httpClient: mock);
      await expectLater(
        svc.beginLogin(_conn()),
        throwsA(isA<LoginError>()
            .having((e) => e.kind, 'kind', LoginErrorKind.malformed)),
      );
    });
  });

  group('LoginService.loginLocal', () {
    test('happy path → LoginResult', () async {
      final mock = MockClient((req) async {
        expect(req.url.path, '/api/v1/auth/login');
        expect(req.method, 'POST');
        final body = jsonDecode(req.body) as Map<String, dynamic>;
        expect(body['username'], 'alice@example.com');
        expect(body['password'], 'hunter2');
        return http.Response(
          jsonEncode(_validLoginResultPayload()),
          200,
          headers: {'content-type': 'application/json'},
        );
      });
      final svc = LoginService(httpClient: mock);
      final result =
          await svc.loginLocal(_conn(), 'alice@example.com', 'hunter2');
      expect(result.accessToken, 'access-xyz');
      expect(result.refreshToken, 'refresh-xyz');
      expect(result.user.email, 'alice@example.com');
      expect(result.expiresAt.isAfter(DateTime.now().toUtc()), isTrue);
    });

    test('401 → LoginErrorKind.wrongCredentials', () async {
      final mock = MockClient((_) async => http.Response('nope', 401));
      final svc = LoginService(httpClient: mock);
      await expectLater(
        svc.loginLocal(_conn(), 'alice@example.com', 'wrong'),
        throwsA(isA<LoginError>().having(
            (e) => e.kind, 'kind', LoginErrorKind.wrongCredentials)),
      );
    });

    test('socket failure → LoginErrorKind.network', () async {
      final mock = MockClient((_) async {
        throw const SocketException('boom');
      });
      final svc = LoginService(httpClient: mock);
      await expectLater(
        svc.loginLocal(_conn(), 'a', 'b'),
        throwsA(isA<LoginError>()
            .having((e) => e.kind, 'kind', LoginErrorKind.network)),
      );
    });

    test('500 → LoginErrorKind.server', () async {
      final mock = MockClient((_) async => http.Response('boom', 500));
      final svc = LoginService(httpClient: mock);
      await expectLater(
        svc.loginLocal(_conn(), 'a', 'b'),
        throwsA(isA<LoginError>()
            .having((e) => e.kind, 'kind', LoginErrorKind.server)),
      );
    });

    test('non-JSON body → LoginErrorKind.malformed', () async {
      final mock = MockClient(
          (_) async => http.Response('definitely not json', 200));
      final svc = LoginService(httpClient: mock);
      await expectLater(
        svc.loginLocal(_conn(), 'a', 'b'),
        throwsA(isA<LoginError>()
            .having((e) => e.kind, 'kind', LoginErrorKind.malformed)),
      );
    });
  });

  group('LoginService SSO', () {
    test('begin + complete happy path', () async {
      final fake = FakeSsoAuthorizer()
        ..scriptedCode = 'auth-code-42'
        ..scriptedState = 'state-42'
        ..scriptedVerifier = 'verifier-42';
      final mock = MockClient((req) async {
        if (req.url.path == '/api/v1/auth/methods') {
          return http.Response(
            jsonEncode(_validAuthMethodsPayload()),
            200,
            headers: {'content-type': 'application/json'},
          );
        }
        expect(req.url.path, '/api/v1/auth/sso/complete');
        final body = jsonDecode(req.body) as Map<String, dynamic>;
        expect(body['provider_id'], 'google');
        expect(body['authorization_code'], 'auth-code-42');
        expect(body['state'], 'state-42');
        expect(body['code_verifier'], 'verifier-42');
        return http.Response(
          jsonEncode(_validLoginResultPayload()),
          200,
          headers: {'content-type': 'application/json'},
        );
      });
      final svc = LoginService(httpClient: mock, authorizer: fake);
      final flow = await svc.beginSso(_conn(), 'google');
      expect(flow.cancelled, isFalse);
      expect(flow.authorizationCode, 'auth-code-42');
      expect(fake.calls.single.id, 'google');
      final result = await svc.completeSso(_conn(), flow);
      expect(result.accessToken, 'access-xyz');
    });

    test('cancelled flow surfaces SsoFlow.cancelled and completeSso refuses',
        () async {
      final fake = FakeSsoAuthorizer()..shouldCancel = true;
      final mock = MockClient((req) async {
        if (req.url.path == '/api/v1/auth/methods') {
          return http.Response(
            jsonEncode(_validAuthMethodsPayload()),
            200,
            headers: {'content-type': 'application/json'},
          );
        }
        fail('completeSso should not have been called: ${req.url}');
      });
      final svc = LoginService(httpClient: mock, authorizer: fake);
      final flow = await svc.beginSso(_conn(), 'google');
      expect(flow.cancelled, isTrue);
      expect(flow.authorizationCode, isNull);
      await expectLater(
        svc.completeSso(_conn(), flow),
        throwsA(isA<LoginError>()
            .having((e) => e.kind, 'kind', LoginErrorKind.ssoCancelled)),
      );
    });

    test('unknown provider → LoginErrorKind.unknownProvider', () async {
      final mock = MockClient((_) async => http.Response(
            jsonEncode(_validAuthMethodsPayload()),
            200,
            headers: {'content-type': 'application/json'},
          ));
      final svc = LoginService(httpClient: mock, authorizer: FakeSsoAuthorizer());
      await expectLater(
        svc.beginSso(_conn(), 'okta-not-advertised'),
        throwsA(isA<LoginError>()
            .having((e) => e.kind, 'kind', LoginErrorKind.unknownProvider)),
      );
    });
  });

  group('LoginService.refresh', () {
    test('happy path → fresh LoginResult', () async {
      final mock = MockClient((req) async {
        expect(req.url.path, '/api/v1/auth/refresh');
        final body = jsonDecode(req.body) as Map<String, dynamic>;
        expect(body['refresh_token'], 'old-refresh');
        return http.Response(
          jsonEncode(_validLoginResultPayload()),
          200,
          headers: {'content-type': 'application/json'},
        );
      });
      final svc = LoginService(httpClient: mock);
      final session = AppSession(
        userId: 'u-1',
        tenantRef: 'tenant-1',
        accessToken: 'old-access',
        refreshToken: 'old-refresh',
        activeConnection: _conn(),
      );
      final result = await svc.refresh(session);
      expect(result.accessToken, 'access-xyz');
      expect(result.refreshToken, 'refresh-xyz');
    });

    test('401 → LoginErrorKind.refreshExpired', () async {
      final mock = MockClient((_) async => http.Response('expired', 401));
      final svc = LoginService(httpClient: mock);
      final session = AppSession(
        userId: 'u-1',
        tenantRef: 'tenant-1',
        refreshToken: 'old-refresh',
        activeConnection: _conn(),
      );
      await expectLater(
        svc.refresh(session),
        throwsA(isA<LoginError>()
            .having((e) => e.kind, 'kind', LoginErrorKind.refreshExpired)),
      );
    });

    test('no refresh token → LoginErrorKind.refreshExpired (without HTTP)',
        () async {
      final mock = MockClient((_) async {
        fail('refresh should not have hit the network');
      });
      final svc = LoginService(httpClient: mock);
      final session = AppSession(
        userId: 'u-1',
        tenantRef: 'tenant-1',
        activeConnection: _conn(),
      );
      await expectLater(
        svc.refresh(session),
        throwsA(isA<LoginError>()
            .having((e) => e.kind, 'kind', LoginErrorKind.refreshExpired)),
      );
    });
  });
}
