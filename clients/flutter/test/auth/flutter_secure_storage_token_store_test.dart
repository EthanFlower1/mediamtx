// KAI-298 — production wiring test for FlutterSecureStorageTokenStore.
//
// Proves the adapter round-trips a token through a stubbed FlutterSecureStorage
// (typed as dynamic so the test VM never loads the real plugin) and that
// deleteByPrefix enumerates readAll() correctly. This is the end-to-end
// guarantee behind the production override added in `lib/main.dart`.

import 'package:flutter_test/flutter_test.dart';

import 'package:nvr_client/auth/flutter_secure_storage_token_store.dart';

class _FakeFlutterSecureStorage {
  final Map<String, String> _kv = <String, String>{};

  Future<String?> read({required String key}) async => _kv[key];

  Future<void> write({required String key, required String value}) async {
    _kv[key] = value;
  }

  Future<void> delete({required String key}) async {
    _kv.remove(key);
  }

  Future<Map<String, String>> readAll() async =>
      Map<String, String>.from(_kv);
}

void main() {
  late _FakeFlutterSecureStorage fake;
  late FlutterSecureStorageTokenStore store;

  setUp(() {
    fake = _FakeFlutterSecureStorage();
    store = FlutterSecureStorageTokenStore(fake);
  });

  test('write → read round-trips a token value', () async {
    await store.write('kai_session:conn-1:access_token', 'tok-1');
    expect(await store.read('kai_session:conn-1:access_token'), 'tok-1');
  });

  test('read returns null for an absent key', () async {
    expect(await store.read('kai_session:missing:access_token'), isNull);
  });

  test('delete removes a single key', () async {
    await store.write('kai_session:conn-1:access_token', 'tok-1');
    await store.delete('kai_session:conn-1:access_token');
    expect(await store.read('kai_session:conn-1:access_token'), isNull);
  });

  test('deleteByPrefix removes only matching keys', () async {
    await store.write('kai_session:conn-A:access_token', 'a');
    await store.write('kai_session:conn-A:refresh_token', 'r');
    await store.write('kai_session:conn-B:access_token', 'b');

    await store.deleteByPrefix('kai_session:conn-A:');

    expect(await store.read('kai_session:conn-A:access_token'), isNull);
    expect(await store.read('kai_session:conn-A:refresh_token'), isNull);
    expect(await store.read('kai_session:conn-B:access_token'), 'b');
  });
}
