// KAI-298 — TokenStore tests.
//
// Tests:
//   1. Write → read round-trip: all fields survive serialisation.
//   2. Read absent → null.
//   3. Keyed isolation: connection A's tokens don't leak to connection B.
//   4. deleteTokens: removes all keys for that connection.
//   5. deleteAll: removes every connection's keys.
//   6. hasTokens: returns correct boolean before/after write/delete.
//   7. Corrupt blob: readTokens returns null without throwing.
//   8. Back-compat keys: bare access_token/refresh_token keys also written.

import 'package:flutter_test/flutter_test.dart';

import 'package:nvr_client/auth/token_store.dart';
import 'package:nvr_client/state/secure_token_store.dart';

TokenSet _makeTokenSet({String tag = 'a', int offsetHours = 1}) => TokenSet(
      accessToken: 'access-$tag',
      refreshToken: 'refresh-$tag',
      expiresAt: DateTime.utc(2026, 4, 8, 12 + offsetHours),
      scope: ['openid', 'profile'],
      idToken: 'id-$tag',
    );

void main() {
  late InMemorySecureTokenStore rawStore;
  late TokenStore store;

  setUp(() {
    rawStore = InMemorySecureTokenStore();
    store = TokenStore(rawStore);
  });

  // ---- 1. Round-trip ----

  test('write → read returns the same TokenSet', () async {
    final ts = _makeTokenSet();
    await store.writeTokens('conn-1', ts);

    final loaded = await store.readTokens('conn-1');
    expect(loaded, isNotNull);
    expect(loaded!.accessToken, ts.accessToken);
    expect(loaded.refreshToken, ts.refreshToken);
    expect(loaded.expiresAt, ts.expiresAt);
    expect(loaded.scope, ts.scope);
    expect(loaded.idToken, ts.idToken);
  });

  test('scope as space-separated string is normalised to a list', () async {
    // Manually write a blob with scope as a string to simulate a server that
    // returns it that way.
    const rawJson =
        '{"access_token":"a","refresh_token":"r","expires_at":"2026-04-08T13:00:00.000Z","scope":"openid profile email"}';
    await rawStore.write('kai_session:conn-scope:token_blob', rawJson);
    await rawStore.write('kai_session:conn-scope:access_token', 'a');
    await rawStore.write('kai_session:conn-scope:refresh_token', 'r');

    final loaded = await store.readTokens('conn-scope');
    expect(loaded?.scope, ['openid', 'profile', 'email']);
  });

  // ---- 2. Absent ----

  test('readTokens returns null when nothing stored', () async {
    final result = await store.readTokens('conn-missing');
    expect(result, isNull);
  });

  // ---- 3. Keyed isolation ----

  test('tokens for connection A do not bleed into connection B', () async {
    await store.writeTokens('conn-A', _makeTokenSet(tag: 'A'));
    await store.writeTokens('conn-B', _makeTokenSet(tag: 'B'));

    final a = await store.readTokens('conn-A');
    final b = await store.readTokens('conn-B');

    expect(a?.accessToken, 'access-A');
    expect(b?.accessToken, 'access-B');
    // Verify raw store keys are namespaced — no key contains both IDs.
    expect(
      rawStore.keysForTest
          .where((k) => k.contains('conn-A') && k.contains('conn-B')),
      isEmpty,
    );
  });

  // ---- 4. deleteTokens ----

  test('deleteTokens removes all keys for that connection', () async {
    await store.writeTokens('conn-del', _makeTokenSet(tag: 'del'));
    await store.deleteTokens('conn-del');

    final result = await store.readTokens('conn-del');
    expect(result, isNull);

    final remaining =
        rawStore.keysForTest.where((k) => k.contains('conn-del'));
    expect(remaining, isEmpty);
  });

  test('deleteTokens is idempotent on a missing connection', () async {
    await expectLater(
      store.deleteTokens('conn-never-written'),
      completes,
    );
  });

  // ---- 5. deleteAll ----

  test('deleteAll removes every connection', () async {
    await store.writeTokens('conn-1', _makeTokenSet(tag: '1'));
    await store.writeTokens('conn-2', _makeTokenSet(tag: '2'));
    await store.deleteAll();

    expect(await store.readTokens('conn-1'), isNull);
    expect(await store.readTokens('conn-2'), isNull);
    expect(
      rawStore.keysForTest.where((k) => k.startsWith('kai_session:')),
      isEmpty,
    );
  });

  // ---- 6. hasTokens ----

  test('hasTokens returns false before write, true after, false after delete',
      () async {
    expect(await store.hasTokens('conn-has'), isFalse);

    await store.writeTokens('conn-has', _makeTokenSet());
    expect(await store.hasTokens('conn-has'), isTrue);

    await store.deleteTokens('conn-has');
    expect(await store.hasTokens('conn-has'), isFalse);
  });

  // ---- 7. Corrupt blob ----

  test('readTokens returns null on corrupt JSON without throwing', () async {
    await rawStore.write(
        'kai_session:conn-bad:token_blob', '{not valid json}');
    await rawStore.write('kai_session:conn-bad:access_token', 'a');
    await rawStore.write('kai_session:conn-bad:refresh_token', 'r');

    final result = await store.readTokens('conn-bad');
    expect(result, isNull);
  });

  // ---- 8. Back-compat keys ----

  test('writeTokens also writes bare access_token and refresh_token keys',
      () async {
    await store.writeTokens('conn-bc', _makeTokenSet(tag: 'bc'));

    final access =
        await rawStore.read('kai_session:conn-bc:access_token');
    final refresh =
        await rawStore.read('kai_session:conn-bc:refresh_token');
    expect(access, 'access-bc');
    expect(refresh, 'refresh-bc');
  });
}
