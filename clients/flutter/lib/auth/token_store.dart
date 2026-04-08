// KAI-298 — High-level token store: structured TokenSet keyed by connection ID.
//
// This layer sits *above* `SecureTokenStore` (the low-level key/value interface
// from KAI-295). It adds:
//
//   * A typed `TokenSet` value object with every field we need for lifecycle
//     management: access_token, refresh_token, expires_at, scope, id_token.
//   * A `TokenStore` service that serialises / deserialises `TokenSet` to the
//     flat key/value store, namespaced by connection ID via
//     `ConnectionScopedKeys`.
//   * `deleteAll()` for full app-reset / sign-out-all-connections.
//
// IMPORTANT: token *values* are never logged. Log-safe helpers (present/absent
// booleans) are provided for any diagnostics that need to mention tokens.
//
// Thread safety: all methods are `async` and backed by the SecureTokenStore
// which serialises concurrent calls on a single platform channel queue.
// Callers that need debounced concurrent refreshes use `TokenRefresher`, not
// this class directly.

import 'dart:convert';

import '../state/secure_token_store.dart';

// ---------------------------------------------------------------------------
// TokenSet — the structured token value object
// ---------------------------------------------------------------------------

/// Full set of tokens issued by the Directory's auth endpoint.
///
/// All fields except [accessToken] and [refreshToken] are optional — the
/// server may omit [scope] (treat as empty) and [idToken] (OIDC-only).
class TokenSet {
  /// Short-lived bearer token. Attached to every API request.
  final String accessToken;

  /// Long-lived token used to mint a new [accessToken] before expiry.
  final String refreshToken;

  /// Absolute UTC expiry of [accessToken]. Used to compute when to schedule
  /// the next background refresh (5 minutes before this instant).
  final DateTime expiresAt;

  /// OAuth scopes granted by the server. Empty list if the server omitted the
  /// field (treat as "all scopes granted for the client ID").
  final List<String> scope;

  /// OIDC ID token. Present only when the flow used OIDC SSO; `null` for
  /// local username/password logins. The client does not parse or validate
  /// this — it is retained so the server can do token-bound ops if needed.
  final String? idToken;

  const TokenSet({
    required this.accessToken,
    required this.refreshToken,
    required this.expiresAt,
    this.scope = const [],
    this.idToken,
  });

  /// Log-safe description: never reveals token values.
  ///
  /// Example: `TokenSet(present=true, expiresAt=2026-04-08T12:30:00Z, scopes=3)`
  @override
  String toString() {
    return 'TokenSet('
        'present=true, '
        'expiresAt=${expiresAt.toIso8601String()}, '
        'scopes=${scope.length}'
        ')';
  }

  Map<String, dynamic> _toJson() => {
        'access_token': accessToken,
        'refresh_token': refreshToken,
        'expires_at': expiresAt.toUtc().toIso8601String(),
        'scope': scope,
        if (idToken != null) 'id_token': idToken,
      };

  factory TokenSet._fromJson(Map<String, dynamic> json) {
    final rawScope = json['scope'];
    final List<String> scopes;
    if (rawScope is List) {
      scopes = rawScope.whereType<String>().toList(growable: false);
    } else if (rawScope is String && rawScope.isNotEmpty) {
      // Some servers return scope as a space-separated string.
      scopes = rawScope.split(' ');
    } else {
      scopes = const [];
    }
    return TokenSet(
      accessToken: json['access_token'] as String,
      refreshToken: json['refresh_token'] as String,
      expiresAt: DateTime.parse(json['expires_at'] as String).toUtc(),
      scope: scopes,
      idToken: json['id_token'] as String?,
    );
  }
}

// ---------------------------------------------------------------------------
// TokenStoreKeys — secure storage key names
// ---------------------------------------------------------------------------

/// Extends [ConnectionScopedKeys] with the extra fields TokenStore writes.
///
/// Format: `kai_session:<connectionId>:<field>`
class TokenStoreKeys {
  // Re-export from ConnectionScopedKeys so callers don't need both imports.
  static String accessToken(String connectionId) =>
      ConnectionScopedKeys.accessToken(connectionId);

  static String refreshToken(String connectionId) =>
      ConnectionScopedKeys.refreshToken(connectionId);

  /// Full serialised TokenSet blob. A single JSON write is cheaper than 5
  /// separate writes; individual access_token / refresh_token keys remain for
  /// backward-compat with `AppSessionNotifier` (which reads them directly).
  static String blob(String connectionId) =>
      'kai_session:$connectionId:token_blob';

  static String prefix(String connectionId) =>
      ConnectionScopedKeys.prefix(connectionId);
}

// ---------------------------------------------------------------------------
// TokenStore — the service
// ---------------------------------------------------------------------------

/// High-level token store. Wraps a [SecureTokenStore] and adds structured
/// [TokenSet] read/write + multi-connection enumeration.
///
/// Per the KAI-298 spec:
///   * Keyed by `connectionId` — tenant A's tokens are never readable with
///     tenant B's connection ID.
///   * Never logs token values — only log-safe booleans.
///   * `deleteAll()` for full sign-out (test-suite reset / app uninstall flow).
class TokenStore {
  final SecureTokenStore _store;

  const TokenStore(this._store);

  // ---------- read ----------

  /// Load the [TokenSet] for [connectionId]. Returns `null` if no tokens have
  /// been written yet (first launch, or after `deleteTokens`).
  ///
  /// Never throws — a corrupt blob is treated as absent and the caller should
  /// redirect the user to login.
  Future<TokenSet?> readTokens(String connectionId) async {
    try {
      final blob =
          await _store.read(TokenStoreKeys.blob(connectionId));
      if (blob == null || blob.isEmpty) return null;
      final decoded = jsonDecode(blob);
      if (decoded is! Map<String, dynamic>) return null;
      return TokenSet._fromJson(decoded);
    } catch (_) {
      // Corrupt entry — treat as absent, do NOT throw.
      return null;
    }
  }

  /// Returns `true` if a token blob exists for [connectionId], without
  /// decoding it. Use for log-safe presence checks.
  Future<bool> hasTokens(String connectionId) async {
    final blob = await _store.read(TokenStoreKeys.blob(connectionId));
    return blob != null && blob.isNotEmpty;
  }

  // ---------- write ----------

  /// Persist a [TokenSet] for [connectionId].
  ///
  /// Writes three keys atomically (as far as the platform allows):
  ///   * The full JSON blob (for deserialisation by `readTokens`).
  ///   * The bare `access_token` key (for backward-compat with
  ///     `AppSessionNotifier.activateConnection` / `switchConnection`).
  ///   * The bare `refresh_token` key (same reason).
  ///
  /// This dual-write keeps the low-level `AppSessionNotifier` working without
  /// modification while the higher-level `TokenStore` consumers get typed data.
  Future<void> writeTokens(String connectionId, TokenSet tokenSet) async {
    final blob = jsonEncode(tokenSet._toJson());
    // Write the structured blob first so a crash between writes leaves the
    // blob as the authority (it's the one `readTokens` uses).
    await _store.write(TokenStoreKeys.blob(connectionId), blob);
    // Back-compat keys for AppSessionNotifier.
    await _store.write(
      TokenStoreKeys.accessToken(connectionId),
      tokenSet.accessToken,
    );
    await _store.write(
      TokenStoreKeys.refreshToken(connectionId),
      tokenSet.refreshToken,
    );
  }

  // ---------- delete ----------

  /// Delete all tokens for [connectionId].
  ///
  /// Deletes the blob and both back-compat keys. Does NOT throw if the keys
  /// don't exist — idempotent by design.
  Future<void> deleteTokens(String connectionId) async {
    await _store.deleteByPrefix(TokenStoreKeys.prefix(connectionId));
  }

  /// Delete all tokens for every connection — full app reset / factory wipe.
  ///
  /// Platform note: this enumerates *all* keys written by this app under the
  /// `kai_session:` namespace and deletes them. The underlying
  /// `SecureTokenStore.deleteByPrefix` call handles enumeration.
  Future<void> deleteAll() async {
    // The root prefix is 'kai_session:' — matches ConnectionScopedKeys._root.
    await _store.deleteByPrefix('kai_session:');
  }
}
