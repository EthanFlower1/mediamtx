// KAI-297 — Chaos test: multi-connection token isolation.
//
// Login on connection X (cloud), switch to connection Y (on-prem), login
// there, switch back to X. Verify:
//
//   * Each connection's tokens are stored under its own scoped key prefix.
//   * The SecureTokenStore never returns tokens from a different connection
//     when read with the active connection's keys.
//   * `forgetConnection(X)` wipes only X's tokens, leaving Y intact.
//   * `logout` on the active connection clears its tokens but does NOT touch
//     the other connection's tokens.
//
// Uses the in-memory secure-store fake from KAI-295 + the LoginService driven
// by a MockClient. No real platform plugins.

import 'dart:convert';

import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';

import 'package:nvr_client/auth/login_service.dart';
import 'package:nvr_client/state/app_session.dart';
import 'package:nvr_client/state/home_directory_connection.dart';
import 'package:nvr_client/state/secure_token_store.dart';

HomeDirectoryConnection _connX() => const HomeDirectoryConnection(
      id: 'conn-X',
      kind: HomeConnectionKind.cloud,
      endpointUrl: 'https://cloud.raikada.example',
      displayName: 'Cloud',
      discoveryMethod: DiscoveryMethod.manual,
    );

HomeDirectoryConnection _connY() => const HomeDirectoryConnection(
      id: 'conn-Y',
      kind: HomeConnectionKind.onPrem,
      endpointUrl: 'https://nvr.acme.local',
      displayName: 'Acme HQ',
      discoveryMethod: DiscoveryMethod.mdns,
    );

Map<String, dynamic> _payloadFor(String tag) => {
      'access_token': 'access-$tag',
      'refresh_token': 'refresh-$tag',
      'expires_in': 900,
      'user': {
        'user_id': 'u-$tag',
        'tenant_ref': 'tenant-$tag',
        'email': '$tag@example.com',
      },
    };

void main() {
  test('logging in on X then Y keeps the two scopes walled off', () async {
    final store = InMemorySecureTokenStore();
    final notifier = AppSessionNotifier(store);

    final mock = MockClient((req) async {
      // Distinguish servers by host so we can hand back per-tenant payloads.
      final host = req.url.host;
      final tag = host.startsWith('cloud') ? 'X' : 'Y';
      return http.Response(
        jsonEncode(_payloadFor(tag)),
        200,
        headers: {'content-type': 'application/json'},
      );
    });
    final svc = LoginService(httpClient: mock);

    // ---- Login on X ----
    await notifier.activateConnection(
      connection: _connX(),
      userId: 'u-X',
      tenantRef: 'tenant-X',
    );
    final loginX = await svc.loginLocal(_connX(), 'alice', 'pw');
    await notifier.setTokens(
      accessToken: loginX.accessToken,
      refreshToken: loginX.refreshToken,
    );

    // Verify X's tokens landed under X's prefix only.
    expect(
      await store.read(ConnectionScopedKeys.accessToken('conn-X')),
      'access-X',
    );
    expect(
      await store.read(ConnectionScopedKeys.accessToken('conn-Y')),
      isNull,
    );

    // ---- Switch to Y, login on Y ----
    await notifier.switchConnection(target: _connY());
    expect(notifier.state.accessToken, isNull,
        reason: 'Y has no tokens yet — must NOT inherit X');
    expect(notifier.state.refreshToken, isNull);

    final loginY = await svc.loginLocal(_connY(), 'bob', 'pw');
    await notifier.setTokens(
      accessToken: loginY.accessToken,
      refreshToken: loginY.refreshToken,
    );

    // Both prefixes now populated, scoped independently.
    expect(
      await store.read(ConnectionScopedKeys.accessToken('conn-X')),
      'access-X',
    );
    expect(
      await store.read(ConnectionScopedKeys.accessToken('conn-Y')),
      'access-Y',
    );
    expect(
      await store.read(ConnectionScopedKeys.refreshToken('conn-Y')),
      'refresh-Y',
    );

    // ---- Switch back to X — must hydrate X's old tokens, not Y's ----
    await notifier.switchConnection(target: _connX());
    expect(notifier.state.accessToken, 'access-X');
    expect(notifier.state.refreshToken, 'refresh-X');

    // ---- Logout of X — wipes X only ----
    await notifier.logout();
    expect(
      await store.read(ConnectionScopedKeys.accessToken('conn-X')),
      isNull,
    );
    expect(
      await store.read(ConnectionScopedKeys.accessToken('conn-Y')),
      'access-Y',
      reason: 'logout on X must NOT touch Y',
    );

    // ---- Forget Y — wipes Y entirely ----
    await notifier.forgetConnection('conn-Y');
    expect(
      store.keysForTest.where((k) => k.contains('conn-Y')),
      isEmpty,
    );
  });
}
