// KAI-295 — Secure token store abstraction.
//
// Wraps `flutter_secure_storage` so unit tests can swap in an in-memory fake
// without pulling the platform plugin into the test harness. Tokens are keyed
// by `<connectionId>:<tokenName>`, which is what gives us per-directory token
// isolation: switching the active connection changes the key prefix, so the
// old tokens stay walled off until the connection is explicitly forgotten.
//
// IMPORTANT: never log token values. The store accepts any opaque string so it
// stays compatible with whatever shape KAI-222 (cloud-platform IdentityProvider)
// hands back — Zitadel JWTs today, anything else tomorrow.

import 'dart:async';

/// Pluggable storage interface. Production wires this to
/// `flutter_secure_storage`; tests use [InMemorySecureTokenStore].
abstract class SecureTokenStore {
  Future<String?> read(String key);
  Future<void> write(String key, String value);
  Future<void> delete(String key);

  /// Delete every key whose name starts with [prefix]. Used when a user
  /// "forgets" a connection so we don't leave its tokens behind.
  Future<void> deleteByPrefix(String prefix);
}

/// In-memory fake. Safe to use in tests; never persists anything.
class InMemorySecureTokenStore implements SecureTokenStore {
  final Map<String, String> _data = {};

  @override
  Future<String?> read(String key) async => _data[key];

  @override
  Future<void> write(String key, String value) async {
    _data[key] = value;
  }

  @override
  Future<void> delete(String key) async {
    _data.remove(key);
  }

  @override
  Future<void> deleteByPrefix(String prefix) async {
    _data.removeWhere((k, _) => k.startsWith(prefix));
  }

  /// Test-only: returns a snapshot of all current keys.
  Iterable<String> get keysForTest => List.unmodifiable(_data.keys);
}

/// Helper that builds the per-connection key namespace.
///
/// Format: `kai_session:<connectionId>:<field>`
class ConnectionScopedKeys {
  static const _root = 'kai_session';

  static String accessToken(String connectionId) =>
      '$_root:$connectionId:access_token';
  static String refreshToken(String connectionId) =>
      '$_root:$connectionId:refresh_token';
  static String prefix(String connectionId) => '$_root:$connectionId:';
}
