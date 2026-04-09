// KAI-297 — Integration test for LoginScreen → AppSession glue.
//
// Verifies that when LoginStateNotifier transitions to LoginPhase.success,
// LoginScreen's `ref.listen` hands the LoginResult to AppSessionNotifier
// (activateConnection + setTokens), and that the resulting session is
// authenticated with tokens persisted through the SecureTokenStore.

import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';

import 'package:nvr_client/auth/auth_providers.dart';
import 'package:nvr_client/auth/login_service.dart';
import 'package:nvr_client/auth/widgets/login_screen.dart';
import 'package:nvr_client/state/app_session.dart';
import 'package:nvr_client/state/home_directory_connection.dart';
import 'package:nvr_client/state/secure_token_store.dart';

HomeDirectoryConnection _conn() => const HomeDirectoryConnection(
      id: 'conn-glue',
      kind: HomeConnectionKind.onPrem,
      endpointUrl: 'https://nvr.acme.local',
      displayName: 'Acme HQ',
      discoveryMethod: DiscoveryMethod.manual,
    );

Map<String, dynamic> _authMethodsPayload() => {
      'local_enabled': true,
      'sso_providers': const <Map<String, dynamic>>[],
    };

Map<String, dynamic> _loginResultPayload() => {
      'access_token': 'access-glue',
      'refresh_token': 'refresh-glue',
      'expires_in': 900,
      'user': {
        'user_id': 'u-glue',
        'tenant_ref': 'tenant-glue',
        'email': 'alice@example.com',
      },
    };

void main() {
  testWidgets(
      'successful local login transitions AppSession to authenticated and persists tokens',
      (tester) async {
    final store = InMemorySecureTokenStore();

    final mock = MockClient((req) async {
      if (req.url.path == '/api/v1/auth/methods') {
        return http.Response(
          jsonEncode(_authMethodsPayload()),
          200,
          headers: {'content-type': 'application/json'},
        );
      }
      if (req.url.path == '/api/v1/auth/login') {
        return http.Response(
          jsonEncode(_loginResultPayload()),
          200,
          headers: {'content-type': 'application/json'},
        );
      }
      return http.Response('not found', 404);
    });

    final loginService = LoginService(httpClient: mock);

    await tester.pumpWidget(
      ProviderScope(
        overrides: [
          secureTokenStoreProvider.overrideWithValue(store),
          loginServiceProvider.overrideWithValue(loginService),
          authHttpClientProvider.overrideWithValue(mock),
        ],
        child: MaterialApp(
          home: LoginScreen(connection: _conn()),
        ),
      ),
    );

    // Let the authMethods FutureProvider resolve.
    await tester.pumpAndSettle();

    final BuildContext ctx = tester.element(find.byType(LoginScreen));
    final container = ProviderScope.containerOf(ctx);

    // Drive the login through the notifier directly — widget-level field entry
    // is covered by local_login_form's own tests.
    await container
        .read(loginStateProvider.notifier)
        .submitLocal(
          connection: _conn(),
          username: 'alice@example.com',
          password: 'hunter2',
        );

    // Pump so the ref.listen in LoginScreen fires + the async glue completes.
    await tester.pumpAndSettle();

    final session = container.read(appSessionProvider);
    expect(session.isAuthenticated, isTrue);
    expect(session.accessToken, 'access-glue');
    expect(session.refreshToken, 'refresh-glue');
    expect(session.activeConnection?.id, 'conn-glue');
    expect(session.userId, 'u-glue');
    expect(session.tenantRef, 'tenant-glue');

    // Tokens must be persisted through the SecureTokenStore under the
    // connection-scoped keys (KAI-295 namespace).
    expect(
      await store.read(ConnectionScopedKeys.accessToken('conn-glue')),
      'access-glue',
    );
    expect(
      await store.read(ConnectionScopedKeys.refreshToken('conn-glue')),
      'refresh-glue',
    );
  });
}
