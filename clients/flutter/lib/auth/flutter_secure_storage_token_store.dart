// KAI-297 — Production SecureTokenStore adapter backed by flutter_secure_storage.
//
// Lives in its own file so the only place that imports the platform plugin is
// here. The auth package and the rest of the codebase can keep talking to the
// `SecureTokenStore` abstraction from `lib/state/secure_token_store.dart`,
// which means unit tests never load the plugin and can use the
// `InMemorySecureTokenStore` fake instead.
//
// Per-connection-ID key namespacing is the responsibility of callers (they
// build keys via `ConnectionScopedKeys`). This adapter is a thin key/value
// pass-through that:
//
//   * delegates `read` / `write` / `delete` to FlutterSecureStorage
//   * implements `deleteByPrefix` by enumerating `readAll()` and deleting each
//     matching key (the plugin has no native prefix-delete)
//
// We deliberately type the underlying storage as `dynamic` instead of importing
// `package:flutter_secure_storage` directly. The reason is that the Dart VM
// test runner refuses to load files that import platform plugins unless those
// plugins have a Dart-only fallback registered, and `flutter_secure_storage`
// does not. By keeping the import dynamic the production wiring works on
// device while the unit-test VM never has to resolve the plugin symbol.

import '../state/secure_token_store.dart';

/// Adapter from `FlutterSecureStorage` (passed in as `dynamic` to keep the
/// test VM happy) to KAI-295's [SecureTokenStore] interface.
class FlutterSecureStorageTokenStore implements SecureTokenStore {
  final dynamic _storage;

  /// Construct with a `FlutterSecureStorage` instance. Bootstrap wiring in
  /// `main.dart` (or wherever providers are overridden for production) is the
  /// only place that imports the real plugin and passes it here.
  FlutterSecureStorageTokenStore(this._storage);

  @override
  Future<String?> read(String key) async {
    final v = await _storage.read(key: key);
    return v as String?;
  }

  @override
  Future<void> write(String key, String value) async {
    await _storage.write(key: key, value: value);
  }

  @override
  Future<void> delete(String key) async {
    await _storage.delete(key: key);
  }

  @override
  Future<void> deleteByPrefix(String prefix) async {
    final all = await _storage.readAll();
    if (all is Map) {
      final matching = all.keys
          .whereType<String>()
          .where((k) => k.startsWith(prefix))
          .toList(growable: false);
      for (final k in matching) {
        await _storage.delete(key: k);
      }
    }
  }
}
