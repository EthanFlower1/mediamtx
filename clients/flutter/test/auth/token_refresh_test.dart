// KAI-298 — TokenRefresher tests.
//
// Tests:
//   1. No refresh needed (token fresh) → returns stored set, no HTTP call.
//   2. Refresh needed (within lead window) → fires HTTP, writes new tokens.
//   3. Refresh needed + expired refresh token (401) → AuthInvalidException.
//   4. Debounce: two simultaneous calls → exactly one HTTP request.
//   5. forceRefresh ignores expiry → always fires HTTP.
//   6. Network error → LoginError(network) thrown, tokens preserved.
//   7. readTokens absent → AuthInvalidException (no-token case).

import 'dart:convert';

import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';

import 'package:nvr_client/auth/token_refresh.dart';
import 'package:nvr_client/auth/token_store.dart';
import 'package:nvr_client/auth/auth_types.dart';
import 'package:nvr_client/state/secure_token_store.dart';

const _connId = 'conn-test';
const _endpoint = 'https://nvr.acme.local';

// Fixed "now" so expiry math is deterministic.
final _fixedNow = DateTime.utc(2026, 4, 8, 12, 0, 0);

// A token that expires in 10 minutes — outside the 5-min lead window.
TokenSet _freshToken() => TokenSet(
      accessToken: 'old-access',
      refreshToken: 'old-refresh',
      expiresAt: _fixedNow.add(const Duration(minutes: 10)),
      scope: ['openid'],
    );

// A token that expires in 2 minutes — inside the lead window → refresh needed.
TokenSet _staleToken() => TokenSet(
      accessToken: 'stale-access',
      refreshToken: 'stale-refresh',
      expiresAt: _fixedNow.add(const Duration(minutes: 2)),
      scope: ['openid'],
    );

Map<String, dynamic> _successPayload({String tag = 'new', int expiresIn = 900}) =>
    {
      'access_token': 'access-$tag',
      'refresh_token': 'refresh-$tag',
      'expires_in': expiresIn,
      'scope': ['openid', 'profile'],
    };

TokenRefresher _makeRefresher({
  required http.Client httpClient,
  required TokenStore tokenStore,
  List<String>? refreshedTokens,
}) {
  return TokenRefresher(
    tokenStore: tokenStore,
    onTokensRefreshed: ({required accessToken, required refreshToken}) async {
      refreshedTokens?.add(accessToken);
    },
    httpClient: httpClient,
    now: () => _fixedNow,
  );
}

void main() {
  late InMemorySecureTokenStore rawStore;
  late TokenStore store;

  setUp(() {
    rawStore = InMemorySecureTokenStore();
    store = TokenStore(rawStore);
  });

  // ---- 1. No refresh needed ----

  test('ensureFresh returns stored token without HTTP call when token is fresh',
      () async {
    await store.writeTokens(_connId, _freshToken());

    var httpCalled = false;
    final mock = MockClient((_) async {
      httpCalled = true;
      return http.Response('should not be called', 200);
    });

    final refresher = _makeRefresher(httpClient: mock, tokenStore: store);
    final result = await refresher.ensureFresh(
      connectionId: _connId,
      endpointUrl: _endpoint,
    );

    expect(httpCalled, isFalse);
    expect(result.accessToken, 'old-access');
    refresher.dispose();
  });

  // ---- 2. Refresh needed ----

  test('ensureFresh fires HTTP and writes new tokens when inside lead window',
      () async {
    await store.writeTokens(_connId, _staleToken());

    final notified = <String>[];
    final mock = MockClient((_) async => http.Response(
          jsonEncode(_successPayload()),
          200,
          headers: {'content-type': 'application/json'},
        ));

    final refresher = _makeRefresher(
      httpClient: mock,
      tokenStore: store,
      refreshedTokens: notified,
    );
    final result = await refresher.ensureFresh(
      connectionId: _connId,
      endpointUrl: _endpoint,
    );

    expect(result.accessToken, 'access-new');
    expect(result.refreshToken, 'refresh-new');
    // Tokens persisted to store.
    final persisted = await store.readTokens(_connId);
    expect(persisted?.accessToken, 'access-new');
    // Notifier was called.
    expect(notified, contains('access-new'));
    refresher.dispose();
  });

  // ---- 3. 401 on refresh → AuthInvalidException ----

  test('401 response clears tokens and throws AuthInvalidException', () async {
    await store.writeTokens(_connId, _staleToken());

    final mock = MockClient((_) async => http.Response('', 401));
    final refresher = _makeRefresher(httpClient: mock, tokenStore: store);

    await expectLater(
      refresher.ensureFresh(
        connectionId: _connId,
        endpointUrl: _endpoint,
      ),
      throwsA(isA<AuthInvalidException>()),
    );
    // Tokens must be cleared.
    expect(await store.hasTokens(_connId), isFalse);
    refresher.dispose();
  });

  // ---- 4. Debounce: two simultaneous calls → one HTTP request ----

  test('two simultaneous ensureFresh calls result in exactly one HTTP request',
      () async {
    await store.writeTokens(_connId, _staleToken());

    var httpCallCount = 0;
    final mock = MockClient((_) async {
      httpCallCount++;
      // Simulate a small delay so the second call definitely races.
      await Future.delayed(const Duration(milliseconds: 10));
      return http.Response(
        jsonEncode(_successPayload()),
        200,
        headers: {'content-type': 'application/json'},
      );
    });

    final refresher = _makeRefresher(httpClient: mock, tokenStore: store);

    // Launch two concurrent refresh calls.
    final futures = await Future.wait([
      refresher.ensureFresh(
          connectionId: _connId, endpointUrl: _endpoint),
      refresher.ensureFresh(
          connectionId: _connId, endpointUrl: _endpoint),
    ]);

    expect(httpCallCount, 1,
        reason: 'only one HTTP call should fire despite two concurrent callers');
    expect(futures[0].accessToken, futures[1].accessToken,
        reason: 'both callers receive the same result');
    refresher.dispose();
  });

  // ---- 5. forceRefresh ignores expiry ----

  test('forceRefresh fires HTTP even when token is not near expiry', () async {
    await store.writeTokens(_connId, _freshToken()); // 10-min remaining

    var httpCalled = false;
    final mock = MockClient((_) async {
      httpCalled = true;
      return http.Response(
        jsonEncode(_successPayload(tag: 'forced')),
        200,
        headers: {'content-type': 'application/json'},
      );
    });

    final refresher = _makeRefresher(httpClient: mock, tokenStore: store);
    final result = await refresher.forceRefresh(
      connectionId: _connId,
      endpointUrl: _endpoint,
    );

    expect(httpCalled, isTrue);
    expect(result.accessToken, 'access-forced');
    refresher.dispose();
  });

  // ---- 6. Network error ----

  test('network error throws LoginError(network) without clearing tokens',
      () async {
    await store.writeTokens(_connId, _staleToken());

    final mock =
        MockClient((_) async => throw http.ClientException('no route'));
    final refresher = _makeRefresher(httpClient: mock, tokenStore: store);

    await expectLater(
      refresher.ensureFresh(
          connectionId: _connId, endpointUrl: _endpoint),
      throwsA(
        isA<LoginError>().having(
          (e) => e.kind,
          'kind',
          LoginErrorKind.network,
        ),
      ),
    );
    // Tokens must still be present.
    expect(await store.hasTokens(_connId), isTrue);
    refresher.dispose();
  });

  // ---- 7. No tokens in store ----

  test('ensureFresh throws AuthInvalidException when no tokens are stored',
      () async {
    final mock = MockClient((_) async => http.Response('', 200));
    final refresher = _makeRefresher(httpClient: mock, tokenStore: store);

    await expectLater(
      refresher.ensureFresh(
          connectionId: 'conn-empty', endpointUrl: _endpoint),
      throwsA(isA<AuthInvalidException>()),
    );
    refresher.dispose();
  });
}
