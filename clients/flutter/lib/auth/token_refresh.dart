// KAI-298 — TokenRefresher: debounced, race-safe access-token refresh.
//
// Responsibilities:
//   1. Decide when a refresh is needed: `expires_at - now < 5 minutes`.
//   2. Execute exactly one HTTP refresh call even if two callers arrive
//      simultaneously (debounce via a `Completer` per connection ID).
//   3. On success: write the new `TokenSet` to `TokenStore` and notify the
//      `AppSessionNotifier` so in-memory state is updated.
//   4. On hard failure (refresh token revoked/expired): clear tokens for the
//      affected connection and throw `AuthInvalidException` so the UI bounces
//      the user back to the login screen.
//
// Platform note: this class runs *in-process*. Long-lived background work
// (when the app is backgrounded on Android/iOS) is handled separately by
// `BackgroundRefresh` in `background_refresh.dart`.
//
// Identity firewall: the refresh call goes to the *Directory's*
// `/api/v1/auth/refresh` endpoint, never directly to Zitadel or any IdP.
// Zitadel sits behind the Directory and is invisible to this client.

import 'dart:async';
import 'dart:convert';
import 'dart:io' show HandshakeException, SocketException;

import 'package:http/http.dart' as http;

import 'auth_types.dart';
import 'token_store.dart';

// ---------------------------------------------------------------------------
// AuthInvalidException — signals the UI to bounce to login
// ---------------------------------------------------------------------------

/// Thrown by [TokenRefresher] when a refresh attempt definitively fails due to
/// an expired or revoked refresh token.
///
/// The UI layer catches this from any API call site, shows the
/// `errorRefreshExpired` snackbar (from `AuthStrings`), and pushes the login
/// route for the affected connection.
class AuthInvalidException implements Exception {
  /// Stable connection ID of the connection that lost its session.
  final String connectionId;

  /// Human-readable debug reason. Never surfaced to the user; use `AuthStrings`
  /// for user-visible copy.
  final String debugReason;

  const AuthInvalidException({
    required this.connectionId,
    required this.debugReason,
  });

  @override
  String toString() =>
      'AuthInvalidException(connection: $connectionId): $debugReason';
}

// ---------------------------------------------------------------------------
// TokenRefresher
// ---------------------------------------------------------------------------

/// How far ahead of expiry a refresh is triggered.
///
/// Matches `kRefreshLeadTime` in `refresh_scheduler.dart` — both constants
/// must stay in sync if the spec changes.
const Duration kRefreshLeadTime = Duration(minutes: 5);

/// Debounced, race-safe refresh service.
///
/// Inject [TokenRefresher] wherever an API layer needs a valid access token.
/// Call [ensureFresh] before every authenticated request; the call is a no-op
/// when the token has plenty of life left.
class TokenRefresher {
  final http.Client _http;
  final TokenStore _tokenStore;

  /// Called after a successful refresh so AppSessionNotifier updates its in-
  /// memory `accessToken` / `refreshToken` fields. Signature matches
  /// `AppSessionNotifier.setTokens`.
  final Future<void> Function({
    required String accessToken,
    required String refreshToken,
  }) _onTokensRefreshed;

  /// Clock injection — lets tests pass a fixed `DateTime.now()`.
  final DateTime Function() _now;

  /// In-flight refresh completers keyed by connection ID. If a second caller
  /// asks for the same connection while a refresh is running they receive the
  /// same Future instead of triggering a second HTTP call.
  final Map<String, Completer<TokenSet>> _inflight = {};

  TokenRefresher({
    required TokenStore tokenStore,
    required Future<void> Function({
      required String accessToken,
      required String refreshToken,
    }) onTokensRefreshed,
    http.Client? httpClient,
    DateTime Function()? now,
  })  : _http = httpClient ?? http.Client(),
        _tokenStore = tokenStore,
        _onTokensRefreshed = onTokensRefreshed,
        _now = now ?? DateTime.now;

  /// Release the underlying HTTP client.
  void dispose() => _http.close();

  // ---------- public API ----------

  /// Returns `true` if [tokenSet] needs a refresh now (expiry within the lead
  /// window). Callers can use this for conditional refresh without incurring
  /// the HTTP cost.
  bool needsRefresh(TokenSet tokenSet) {
    final remaining = tokenSet.expiresAt.difference(_now());
    return remaining < kRefreshLeadTime;
  }

  /// Ensure the token for [connectionId] is fresh. Triggers a refresh if the
  /// stored token is within the lead window, otherwise returns immediately.
  ///
  /// If two callers race: the second one piggy-backs on the first call's
  /// in-flight `Completer` — only one HTTP request fires.
  ///
  /// Throws [AuthInvalidException] when the refresh token is revoked/expired.
  /// Throws [LoginError] with [LoginErrorKind.network] on transient I/O
  /// failure (callers may retry; the tokens remain valid for the moment).
  Future<TokenSet> ensureFresh({
    required String connectionId,
    required String endpointUrl,
  }) async {
    final stored = await _tokenStore.readTokens(connectionId);
    if (stored == null) {
      throw AuthInvalidException(
        connectionId: connectionId,
        debugReason: 'no tokens in store',
      );
    }
    if (!needsRefresh(stored)) return stored;
    return _doRefresh(
      connectionId: connectionId,
      endpointUrl: endpointUrl,
      refreshToken: stored.refreshToken,
      existingScope: stored.scope,
    );
  }

  /// Force a refresh regardless of expiry. Useful on app-foreground when the
  /// token may have expired while the app was suspended.
  ///
  /// Identical debounce guarantee to [ensureFresh].
  Future<TokenSet> forceRefresh({
    required String connectionId,
    required String endpointUrl,
  }) async {
    final stored = await _tokenStore.readTokens(connectionId);
    if (stored == null) {
      throw AuthInvalidException(
        connectionId: connectionId,
        debugReason: 'no tokens in store for forced refresh',
      );
    }
    return _doRefresh(
      connectionId: connectionId,
      endpointUrl: endpointUrl,
      refreshToken: stored.refreshToken,
      existingScope: stored.scope,
    );
  }

  // ---------- internal ----------

  /// Core debounced refresh. A `Completer` keyed on [connectionId] ensures
  /// that concurrent callers receive the same `Future<TokenSet>`.
  Future<TokenSet> _doRefresh({
    required String connectionId,
    required String endpointUrl,
    required String refreshToken,
    required List<String> existingScope,
  }) {
    // If a refresh is already in-flight for this connection, return its Future.
    final existing = _inflight[connectionId];
    if (existing != null) return existing.future;

    final completer = Completer<TokenSet>();
    _inflight[connectionId] = completer;

    _executeRefresh(
      connectionId: connectionId,
      endpointUrl: endpointUrl,
      refreshToken: refreshToken,
      existingScope: existingScope,
    ).then((tokenSet) {
      _inflight.remove(connectionId);
      completer.complete(tokenSet);
    }).catchError((Object err) {
      _inflight.remove(connectionId);
      completer.completeError(err);
    });

    return completer.future;
  }

  /// Actually calls `/api/v1/auth/refresh` and processes the result.
  Future<TokenSet> _executeRefresh({
    required String connectionId,
    required String endpointUrl,
    required String refreshToken,
    required List<String> existingScope,
  }) async {
    final base = Uri.parse(endpointUrl);
    final path = '${base.path}/api/v1/auth/refresh';
    final uri = base.replace(path: path);

    http.Response resp;
    try {
      resp = await _http
          .post(
            uri,
            headers: const {
              'Accept': 'application/json',
              'Content-Type': 'application/json',
            },
            body: jsonEncode({'refresh_token': refreshToken}),
          )
          .timeout(const Duration(seconds: 10));
    } on TimeoutException catch (e) {
      throw LoginError(LoginErrorKind.network, 'refresh timeout: $e');
    } on SocketException catch (e) {
      throw LoginError(LoginErrorKind.network, 'refresh socket: $e');
    } on HandshakeException catch (e) {
      throw LoginError(LoginErrorKind.network, 'refresh tls: $e');
    } on http.ClientException catch (e) {
      throw LoginError(LoginErrorKind.network, 'refresh http: $e');
    }

    if (resp.statusCode == 401) {
      // Hard failure: refresh token is revoked or expired.
      await _tokenStore.deleteTokens(connectionId);
      throw AuthInvalidException(
        connectionId: connectionId,
        debugReason: 'server returned 401 on refresh — token revoked',
      );
    }
    if (resp.statusCode < 200 || resp.statusCode >= 300) {
      throw LoginError(
          LoginErrorKind.server, 'refresh status ${resp.statusCode}');
    }

    final tokenSet = _parseTokenSet(
      body: resp.body,
      connectionId: connectionId,
      existingScope: existingScope,
    );

    // Persist new tokens.
    await _tokenStore.writeTokens(connectionId, tokenSet);

    // Notify AppSessionNotifier so in-memory state stays in sync.
    await _onTokensRefreshed(
      accessToken: tokenSet.accessToken,
      refreshToken: tokenSet.refreshToken,
    );

    return tokenSet;
  }

  TokenSet _parseTokenSet({
    required String body,
    required String connectionId,
    required List<String> existingScope,
  }) {
    dynamic parsed;
    try {
      parsed = jsonDecode(body);
    } catch (e) {
      throw LoginError(LoginErrorKind.malformed, 'refresh: not json: $e');
    }
    if (parsed is! Map<String, dynamic>) {
      throw const LoginError(
          LoginErrorKind.malformed, 'refresh: response not an object');
    }

    final accessToken = parsed['access_token'] as String?;
    final newRefreshToken = parsed['refresh_token'] as String?;
    if (accessToken == null || accessToken.isEmpty) {
      throw const LoginError(
          LoginErrorKind.malformed, 'refresh: missing access_token');
    }
    if (newRefreshToken == null || newRefreshToken.isEmpty) {
      throw const LoginError(
          LoginErrorKind.malformed, 'refresh: missing refresh_token');
    }

    final expiresInSec = parsed['expires_in'];
    final expiresAtStr = parsed['expires_at'];
    final DateTime expiresAt;
    if (expiresAtStr is String) {
      expiresAt = DateTime.parse(expiresAtStr).toUtc();
    } else if (expiresInSec is num) {
      expiresAt = _now().toUtc().add(Duration(seconds: expiresInSec.toInt()));
    } else {
      expiresAt = _now().toUtc().add(const Duration(minutes: 15));
    }

    final rawScope = parsed['scope'];
    List<String> scope;
    if (rawScope is List) {
      scope = rawScope.whereType<String>().toList(growable: false);
    } else if (rawScope is String && rawScope.isNotEmpty) {
      scope = rawScope.split(' ');
    } else {
      scope = existingScope; // Carry forward from the previous token set.
    }

    return TokenSet(
      accessToken: accessToken,
      refreshToken: newRefreshToken,
      expiresAt: expiresAt,
      scope: scope,
      idToken: parsed['id_token'] as String?,
    );
  }
}
