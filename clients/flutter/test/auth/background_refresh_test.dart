// KAI-298 — BackgroundRefresh tests (platform stub tests).
//
// These tests verify the Dart-side behaviour of BackgroundRefreshController
// using fake platform binders and mock token refreshers. No real WorkManager
// or BGTaskScheduler channels are exercised here.
//
// Tests:
//   1. start() registers the binder with the correct connection IDs.
//   2. onConnectionsChanged() re-registers with the updated list.
//   3. stop() cancels the binder and clears connection IDs.
//   4. testTriggerPoll() calls ensureFresh for each known connection.
//   5. FakePlatformBinder captures register/cancel calls.

import 'dart:async';

import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';

import 'package:nvr_client/auth/background_refresh.dart';
import 'package:nvr_client/auth/token_refresh.dart';
import 'package:nvr_client/auth/token_store.dart';
import 'package:nvr_client/state/secure_token_store.dart';

// ---------------------------------------------------------------------------
// Fake binder
// ---------------------------------------------------------------------------

class FakePlatformBackgroundBinder implements PlatformBackgroundBinder {
  final List<List<String>> registerCalls = [];
  int cancelCount = 0;

  @override
  Future<void> register({required List<String> connectionIds}) async {
    registerCalls.add(List.unmodifiable(connectionIds));
  }

  @override
  Future<void> cancel() async {
    cancelCount++;
  }
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// Endpoint URL resolver.
String _endpointFor(String id) => 'https://nvr-$id.example.local';

// Build a TokenStore with a fresh token for each connection ID.
Future<TokenStore> _storeWithFreshTokens(List<String> connectionIds) async {
  final raw = InMemorySecureTokenStore();
  final store = TokenStore(raw);
  // Tokens expire in 30 minutes — well outside the 5-min refresh window.
  final expiresAt = DateTime.utc(2026, 4, 8, 12, 30);
  for (final id in connectionIds) {
    await store.writeTokens(
      id,
      TokenSet(
        accessToken: 'access-$id',
        refreshToken: 'refresh-$id',
        expiresAt: expiresAt,
      ),
    );
  }
  return store;
}

// Build a TokenRefresher backed by a mock HTTP client that always succeeds.
TokenRefresher _makeRefresher({
  required TokenStore store,
  List<String>? refreshedIds,
}) {
  return TokenRefresher(
    tokenStore: store,
    onTokensRefreshed: ({required accessToken, required refreshToken}) async {
      refreshedIds?.add(accessToken);
    },
    httpClient: MockClient((_) async => http.Response('', 200)),
    now: () => DateTime.utc(2026, 4, 8, 12, 0, 0),
  );
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

void main() {
  // Reset the singleton before each test so state doesn't bleed across tests.
  setUp(() async {
    await BackgroundRefreshController.instance.stop();
    BackgroundRefreshController.instance.platformBinder =
        const NoOpPlatformBinder();
  });

  // ---- 1. start() registers the binder ----

  test('start() registers binder with supplied connection IDs', () async {
    final binder = FakePlatformBackgroundBinder();
    BackgroundRefreshController.instance.platformBinder = binder;

    final ids = ['conn-a', 'conn-b'];
    final store = await _storeWithFreshTokens(ids);
    final refresher = _makeRefresher(store: store);

    await BackgroundRefreshController.instance.start(
      tokenStore: store,
      refresher: refresher,
      connectionIds: ids,
      endpointUrlFor: _endpointFor,
    );

    expect(binder.registerCalls, isNotEmpty);
    expect(binder.registerCalls.last, containsAll(ids));

    await BackgroundRefreshController.instance.stop();
    refresher.dispose();
  });

  // ---- 2. onConnectionsChanged() re-registers ----

  test('onConnectionsChanged() re-registers with updated connection list',
      () async {
    final binder = FakePlatformBackgroundBinder();
    BackgroundRefreshController.instance.platformBinder = binder;

    final ids = ['conn-a'];
    final store = await _storeWithFreshTokens(['conn-a', 'conn-c']);
    final refresher = _makeRefresher(store: store);

    await BackgroundRefreshController.instance.start(
      tokenStore: store,
      refresher: refresher,
      connectionIds: ids,
      endpointUrlFor: _endpointFor,
    );

    final updated = ['conn-a', 'conn-c'];
    await BackgroundRefreshController.instance.onConnectionsChanged(
      connectionIds: updated,
      endpointUrlFor: _endpointFor,
    );

    expect(binder.registerCalls.length, greaterThanOrEqualTo(2));
    expect(binder.registerCalls.last, containsAll(updated));

    await BackgroundRefreshController.instance.stop();
    refresher.dispose();
  });

  // ---- 3. stop() cancels binder ----

  test('stop() cancels the binder', () async {
    final binder = FakePlatformBackgroundBinder();
    BackgroundRefreshController.instance.platformBinder = binder;

    final ids = ['conn-a'];
    final store = await _storeWithFreshTokens(ids);
    final refresher = _makeRefresher(store: store);

    await BackgroundRefreshController.instance.start(
      tokenStore: store,
      refresher: refresher,
      connectionIds: ids,
      endpointUrlFor: _endpointFor,
    );

    await BackgroundRefreshController.instance.stop();

    expect(binder.cancelCount, 1);
    refresher.dispose();
  });

  // ---- 4. testTriggerPoll() exercises ensureFresh ----

  test('testTriggerPoll calls ensureFresh for each known connection', () async {
    // Use tokens inside the lead window so ensureFresh actually fires HTTP.
    final raw = InMemorySecureTokenStore();
    final store = TokenStore(raw);
    final staleExpiry = DateTime.utc(2026, 4, 8, 12, 2); // 2 min from fixed now
    for (final id in ['conn-x', 'conn-y']) {
      await store.writeTokens(
        id,
        TokenSet(
          accessToken: 'stale-$id',
          refreshToken: 'refresh-$id',
          expiresAt: staleExpiry,
        ),
      );
    }

    final httpHits = <String>[];
    final refresher = TokenRefresher(
      tokenStore: store,
      onTokensRefreshed:
          ({required accessToken, required refreshToken}) async {},
      httpClient: MockClient((req) async {
        httpHits.add(req.url.host);
        return http.Response(
          '{"access_token":"new","refresh_token":"new-r","expires_in":900}',
          200,
          headers: {'content-type': 'application/json'},
        );
      }),
      now: () => DateTime.utc(2026, 4, 8, 12, 0, 0),
    );

    await BackgroundRefreshController.instance.start(
      tokenStore: store,
      refresher: refresher,
      connectionIds: ['conn-x', 'conn-y'],
      endpointUrlFor: _endpointFor,
    );

    BackgroundRefreshController.instance
        .testTriggerPoll(endpointUrlFor: _endpointFor);

    // Allow microtasks / timers to complete.
    await Future.delayed(const Duration(milliseconds: 50));

    expect(httpHits.length, 2,
        reason: 'one HTTP call per connection with stale tokens');

    await BackgroundRefreshController.instance.stop();
    refresher.dispose();
  });

  // ---- 5. NoOpPlatformBinder is silent ----

  test('NoOpPlatformBinder does not throw', () async {
    const binder = NoOpPlatformBinder();
    await expectLater(
      binder.register(connectionIds: ['x', 'y']),
      completes,
    );
    await expectLater(binder.cancel(), completes);
  });
}
