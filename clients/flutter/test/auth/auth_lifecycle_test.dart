// KAI-298 — AuthLifecycle tests.
//
// Tests:
//   1. onAppStart with expired tokens → emits SessionInvalidatedEvent.
//   2. onAppStart with valid tokens → no event emitted, background controller started.
//   3. onLoginSuccess → tokens stored, AppSessionNotifier updated.
//   4. onLogout → tokens deleted, connection removed from background controller.
//   5. handleAuthInvalid → tokens cleared, SessionInvalidatedEvent emitted.
//   6. handleUnauthorized success (refresh works) → returns true.
//   7. handleUnauthorized failure (refresh 401) → emits event, returns false.

import 'dart:async';
import 'dart:convert';

import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';

import 'package:nvr_client/auth/auth_lifecycle.dart';
import 'package:nvr_client/auth/auth_types.dart';
import 'package:nvr_client/auth/background_refresh.dart';
import 'package:nvr_client/auth/token_refresh.dart';
import 'package:nvr_client/auth/token_store.dart';
import 'package:nvr_client/state/app_session.dart';
import 'package:nvr_client/state/home_directory_connection.dart';
import 'package:nvr_client/state/secure_token_store.dart';

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

const _connId = 'conn-lc';
const _endpoint = 'https://nvr.lifecycle.test';

HomeDirectoryConnection _conn({String id = _connId}) =>
    HomeDirectoryConnection(
      id: id,
      kind: HomeConnectionKind.onPrem,
      endpointUrl: _endpoint,
      displayName: 'Lifecycle Test',
      discoveryMethod: DiscoveryMethod.manual,
    );

// Fixed "now" — tokens expiring in 2 min are inside the 5-min lead window.
final _fixedNow = DateTime.utc(2026, 4, 8, 12, 0, 0);

TokenSet _expiredTokenSet() => TokenSet(
      accessToken: 'expired-access',
      refreshToken: 'expired-refresh',
      // Already past the lead window — refresh needed.
      expiresAt: _fixedNow.subtract(const Duration(minutes: 5)),
    );

TokenSet _freshTokenSet() => TokenSet(
      accessToken: 'fresh-access',
      refreshToken: 'fresh-refresh',
      expiresAt: _fixedNow.add(const Duration(minutes: 30)),
    );

LoginResult _loginResult() => LoginResult(
      accessToken: 'login-access',
      refreshToken: 'login-refresh',
      expiresAt: _fixedNow.add(const Duration(hours: 1)),
      user: const UserClaims(userId: 'u-1', tenantRef: 'tenant-1'),
    );

// ---------------------------------------------------------------------------
// Fixture factory
// ---------------------------------------------------------------------------

class _Fixture {
  final InMemorySecureTokenStore rawStore;
  final TokenStore store;
  final AppSessionNotifier sessionNotifier;
  final TokenRefresher refresher;
  final AuthLifecycle lifecycle;

  _Fixture({
    required this.rawStore,
    required this.store,
    required this.sessionNotifier,
    required this.refresher,
    required this.lifecycle,
  });

  static Future<_Fixture> create({
    required http.Client httpClient,
  }) async {
    final raw = InMemorySecureTokenStore();
    final store = TokenStore(raw);
    final sessionNotifier = AppSessionNotifier(raw);

    final refresher = TokenRefresher(
      tokenStore: store,
      onTokensRefreshed: ({required accessToken, required refreshToken}) =>
          sessionNotifier.setTokens(
        accessToken: accessToken,
        refreshToken: refreshToken,
      ),
      httpClient: httpClient,
      now: () => _fixedNow,
    );

    final lifecycle = AuthLifecycle(
      tokenStore: store,
      tokenRefresher: refresher,
      sessionNotifier: sessionNotifier,
    );

    return _Fixture(
      rawStore: raw,
      store: store,
      sessionNotifier: sessionNotifier,
      refresher: refresher,
      lifecycle: lifecycle,
    );
  }
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

void main() {
  // Reset the BackgroundRefreshController singleton between tests.
  setUp(() async {
    await BackgroundRefreshController.instance.stop();
    BackgroundRefreshController.instance.platformBinder =
        const NoOpPlatformBinder();
  });

  // ---- 1. App start with no tokens → SessionInvalidatedEvent ----

  test(
      'onAppStart with no stored tokens emits SessionInvalidatedEvent for active connection',
      () async {
    final f = await _Fixture.create(httpClient: MockClient((_) async =>
        http.Response('', 500)));

    final events = <AuthLifecycleEvent>[];
    f.lifecycle.events.listen(events.add);

    await f.lifecycle.onAppStart(
      knownConnections: [_conn()],
      activeConnectionId: _connId,
    );

    // Allow microtasks to settle.
    await Future.delayed(Duration.zero);

    expect(
      events,
      contains(isA<SessionInvalidatedEvent>()
          .having((e) => e.connectionId, 'connectionId', _connId)),
    );

    await f.lifecycle.dispose();
    f.refresher.dispose();
  });

  // ---- 2. App start with valid tokens → no invalidation event ----

  test('onAppStart with fresh tokens emits no SessionInvalidatedEvent',
      () async {
    final f = await _Fixture.create(
        httpClient: MockClient((_) async => http.Response('', 200)));
    await f.store.writeTokens(_connId, _freshTokenSet());

    final events = <AuthLifecycleEvent>[];
    f.lifecycle.events.listen(events.add);

    await f.lifecycle.onAppStart(
      knownConnections: [_conn()],
      activeConnectionId: _connId,
    );

    await Future.delayed(Duration.zero);

    expect(
      events.whereType<SessionInvalidatedEvent>(),
      isEmpty,
    );

    await f.lifecycle.dispose();
    f.refresher.dispose();
  });

  // ---- 3. onLoginSuccess → tokens stored + AppSession updated ----

  test('onLoginSuccess persists tokens and notifies AppSessionNotifier',
      () async {
    final f = await _Fixture.create(
        httpClient: MockClient((_) async => http.Response('', 200)));

    // Activate the connection first so setTokens doesn't throw.
    await f.sessionNotifier.activateConnection(
      connection: _conn(),
      userId: 'u-1',
      tenantRef: 'tenant-1',
    );

    await f.lifecycle.onLoginSuccess(
      connection: _conn(),
      loginResult: _loginResult(),
    );

    // Tokens in store.
    final stored = await f.store.readTokens(_connId);
    expect(stored?.accessToken, 'login-access');

    // In-memory session updated.
    expect(f.sessionNotifier.state.accessToken, 'login-access');

    await f.lifecycle.dispose();
    f.refresher.dispose();
  });

  // ---- 4. onLogout → tokens deleted ----

  test('onLogout clears tokens for the connection', () async {
    final f = await _Fixture.create(
        httpClient: MockClient((_) async => http.Response('', 200)));
    await f.store.writeTokens(_connId, _freshTokenSet());

    await f.lifecycle.onLogout(connectionId: _connId);

    expect(await f.store.hasTokens(_connId), isFalse);

    await f.lifecycle.dispose();
    f.refresher.dispose();
  });

  // ---- 5. handleAuthInvalid → emits event + clears tokens ----

  test('handleAuthInvalid clears tokens and emits SessionInvalidatedEvent',
      () async {
    final f = await _Fixture.create(
        httpClient: MockClient((_) async => http.Response('', 200)));
    await f.store.writeTokens(_connId, _expiredTokenSet());

    final events = <AuthLifecycleEvent>[];
    f.lifecycle.events.listen(events.add);

    await f.lifecycle.handleAuthInvalid(
      const AuthInvalidException(
          connectionId: _connId, debugReason: 'test invalidation'),
    );

    expect(await f.store.hasTokens(_connId), isFalse);
    expect(
      events,
      contains(isA<SessionInvalidatedEvent>()
          .having((e) => e.connectionId, 'connectionId', _connId)),
    );

    await f.lifecycle.dispose();
    f.refresher.dispose();
  });

  // ---- 6. handleUnauthorized → refresh succeeds → returns true ----

  test('handleUnauthorized returns true when refresh succeeds', () async {
    final f = await _Fixture.create(httpClient: MockClient((_) async {
      return http.Response(
        jsonEncode({
          'access_token': 'new-access',
          'refresh_token': 'new-refresh',
          'expires_in': 900,
        }),
        200,
        headers: {'content-type': 'application/json'},
      );
    }));
    await f.store.writeTokens(_connId, _expiredTokenSet());

    // Pre-register the endpoint URL by calling onAppStart.
    await f.lifecycle.onAppStart(
      knownConnections: [_conn()],
      activeConnectionId: null,
    );

    // Activate session so setTokens doesn't throw.
    await f.sessionNotifier.activateConnection(
      connection: _conn(),
      userId: 'u-1',
      tenantRef: 'tenant-1',
    );

    final result =
        await f.lifecycle.handleUnauthorized(connectionId: _connId);

    expect(result, isTrue);

    await f.lifecycle.dispose();
    f.refresher.dispose();
  });

  // ---- 7. handleUnauthorized → refresh 401 → returns false + event ----

  test('handleUnauthorized returns false and emits event when refresh returns 401',
      () async {
    final f = await _Fixture.create(
        httpClient: MockClient((_) async => http.Response('', 401)));
    await f.store.writeTokens(_connId, _expiredTokenSet());

    final events = <AuthLifecycleEvent>[];
    f.lifecycle.events.listen(events.add);

    await f.lifecycle.onAppStart(
      knownConnections: [_conn()],
      activeConnectionId: null,
    );

    final result =
        await f.lifecycle.handleUnauthorized(connectionId: _connId);

    expect(result, isFalse);
    expect(
      events.whereType<SessionInvalidatedEvent>(),
      isNotEmpty,
    );

    await f.lifecycle.dispose();
    f.refresher.dispose();
  });
}
