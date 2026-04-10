// KAI-298 — AuthLifecycle: startup orchestration + 401 interception.
//
// This is the top-level coordinator that ties together:
//
//   * `TokenStore`          — structured token persistence (this ticket)
//   * `TokenRefresher`      — debounced HTTP refresh (this ticket)
//   * `BackgroundRefreshController` — foreground timer + OS task (this ticket)
//   * `AppSessionNotifier`  — in-memory session state (KAI-295)
//   * `LoginService`        — login / refresh HTTP calls (KAI-297)
//   * `RefreshScheduler`    — single-shot next-expiry scheduler (KAI-297)
//
// Lifecycle events managed here:
//
//   1. App start: enumerate all connections with stored tokens, hydrate
//      AppSession for the active one, start BackgroundRefreshController.
//
//   2. On login (from KAI-297 LoginStateNotifier): store new TokenSet,
//      update AppSession, arm the RefreshScheduler.
//
//   3. On logout: clear tokens for the connection, stop BackgroundRefresh
//      for that connection.
//
//   4. On AuthInvalidException (refresh token revoked): clear tokens, emit
//      a route event so the UI can push the login screen + show a SnackBar.
//
//   5. On 401 from any API call: call `handleUnauthorized(connectionId)`,
//      which triggers an immediate refresh attempt. If the refresh also fails
//      with 401, we clear tokens and bounce the user.
//
// This class is NOT a Riverpod provider itself — it is instantiated once in
// `main.dart` (or in test as a plain Dart object) and its methods are called
// from the appropriate lifecycle points. Keeping it outside Riverpod prevents
// circular provider dependencies (AppSession → AuthLifecycle → AppSession).

import 'dart:async';

import '../state/app_session.dart';
import '../state/home_directory_connection.dart';
import 'auth_types.dart';
import 'background_refresh.dart';
import 'token_refresh.dart';
import 'token_store.dart';

// ---------------------------------------------------------------------------
// AuthLifecycleEvent — stream of events the UI layer listens to
// ---------------------------------------------------------------------------

/// Base class for events emitted by [AuthLifecycle].
sealed class AuthLifecycleEvent {
  const AuthLifecycleEvent();
}

/// The session for [connectionId] has been invalidated.
///
/// The UI should navigate to the login screen for this connection and show
/// `AuthStrings.errorRefreshExpired` in a SnackBar.
class SessionInvalidatedEvent extends AuthLifecycleEvent {
  final String connectionId;

  /// Debug reason. Never surfaced to the user.
  final String reason;

  const SessionInvalidatedEvent({
    required this.connectionId,
    required this.reason,
  });
}

/// A silent background refresh succeeded for [connectionId].
class TokenRefreshedEvent extends AuthLifecycleEvent {
  final String connectionId;

  const TokenRefreshedEvent(this.connectionId);
}

// ---------------------------------------------------------------------------
// AuthLifecycle
// ---------------------------------------------------------------------------

/// Orchestrates the full token lifecycle. Instantiate once and keep alive for
/// the duration of the app process.
class AuthLifecycle {
  final TokenStore _tokenStore;
  final TokenRefresher _tokenRefresher;
  final AppSessionNotifier _sessionNotifier;

  /// Maps connection ID → its endpoint URL, so `BackgroundRefreshController`
  /// can pass the right URL to `TokenRefresher.ensureFresh`.
  final Map<String, String> _endpointUrls = {};

  final StreamController<AuthLifecycleEvent> _events =
      StreamController.broadcast();

  /// Stream of lifecycle events. The UI layer subscribes here.
  Stream<AuthLifecycleEvent> get events => _events.stream;

  AuthLifecycle({
    required TokenStore tokenStore,
    required TokenRefresher tokenRefresher,
    required AppSessionNotifier sessionNotifier,
  })  : _tokenStore = tokenStore,
        _tokenRefresher = tokenRefresher,
        _sessionNotifier = sessionNotifier;

  // ---------- 1. App start ----------

  /// Call once after platform initialisation is complete.
  ///
  /// [knownConnections]: list of all connections the user has previously added
  /// (loaded from SharedPreferences by the startup routine). Their tokens are
  /// stored in `TokenStore`; this method seeds `BackgroundRefreshController`
  /// with the IDs that have valid tokens.
  ///
  /// [activeConnectionId]: the connection that should be the active session on
  /// startup (last-used connection). If `null`, the UI presents the connection
  /// picker.
  Future<void> onAppStart({
    required List<HomeDirectoryConnection> knownConnections,
    String? activeConnectionId,
  }) async {
    // Populate the endpoint URL map.
    for (final conn in knownConnections) {
      _endpointUrls[conn.id] = conn.endpointUrl;
    }

    // Find connections with valid (non-expired) tokens.
    final liveConnectionIds = <String>[];
    for (final conn in knownConnections) {
      final hasTokens = await _tokenStore.hasTokens(conn.id);
      if (hasTokens) {
        liveConnectionIds.add(conn.id);
      }
    }

    // If the designated active connection has expired tokens, surface the
    // invalidation event immediately so the UI can show the login screen.
    if (activeConnectionId != null) {
      final tokenSet = await _tokenStore.readTokens(activeConnectionId);
      if (tokenSet == null) {
        _events.add(SessionInvalidatedEvent(
          connectionId: activeConnectionId,
          reason: 'no tokens found on app start',
        ));
      } else {
        // Eagerly refresh if we're already in the lead window.
        final endpointUrl = _endpointUrls[activeConnectionId];
        if (endpointUrl != null && _tokenRefresher.needsRefresh(tokenSet)) {
          _tryRefresh(
            connectionId: activeConnectionId,
            endpointUrl: endpointUrl,
          );
        }
      }
    }

    // Start the background controller for all connections with tokens.
    await BackgroundRefreshController.instance.start(
      tokenStore: _tokenStore,
      refresher: _tokenRefresher,
      connectionIds: liveConnectionIds,
      endpointUrlFor: (id) => _endpointUrls[id] ?? '',
    );
  }

  // ---------- 2. On login ----------

  /// Call after a successful login (local or SSO) to store the new tokens and
  /// update the `AppSession` + background refresh registration.
  ///
  /// Typically called by the UI after `LoginStateNotifier` transitions to
  /// `LoginPhase.success`.
  Future<void> onLoginSuccess({
    required HomeDirectoryConnection connection,
    required LoginResult loginResult,
  }) async {
    _endpointUrls[connection.id] = connection.endpointUrl;

    final tokenSet = TokenSet(
      accessToken: loginResult.accessToken,
      refreshToken: loginResult.refreshToken,
      expiresAt: loginResult.expiresAt,
    );

    await _tokenStore.writeTokens(connection.id, tokenSet);

    // Update the in-memory session.
    await _sessionNotifier.setTokens(
      accessToken: loginResult.accessToken,
      refreshToken: loginResult.refreshToken,
    );

    // Extend the background refresh controller with the new connection.
    final currentIds = [
      ..._endpointUrls.keys.where(
        (id) => id != connection.id,
      ),
      connection.id,
    ];
    await BackgroundRefreshController.instance.onConnectionsChanged(
      connectionIds: currentIds,
      endpointUrlFor: (id) => _endpointUrls[id] ?? '',
    );
  }

  // ---------- 3. On logout ----------

  /// Call when the user explicitly signs out of [connectionId].
  ///
  /// Clears tokens, removes the connection from background refresh, and updates
  /// `AppSession`.
  Future<void> onLogout({required String connectionId}) async {
    await _tokenStore.deleteTokens(connectionId);
    _endpointUrls.remove(connectionId);

    final remainingIds =
        _endpointUrls.keys.toList(growable: false);
    await BackgroundRefreshController.instance.onConnectionsChanged(
      connectionIds: remainingIds,
      endpointUrlFor: (id) => _endpointUrls[id] ?? '',
    );

    // AppSessionNotifier.logout() is called by the UI; we don't call it here
    // to avoid double-clear and to keep the decision "which logout path"
    // with the UI.
  }

  // ---------- 4. AuthInvalidException handler ----------

  /// Call when `TokenRefresher` or any API call throws [AuthInvalidException].
  ///
  /// Clears tokens, emits [SessionInvalidatedEvent], and updates background
  /// refresh registration.
  Future<void> handleAuthInvalid(AuthInvalidException e) async {
    await _tokenStore.deleteTokens(e.connectionId);
    _endpointUrls.remove(e.connectionId);

    _events.add(SessionInvalidatedEvent(
      connectionId: e.connectionId,
      reason: e.debugReason,
    ));

    final remainingIds = _endpointUrls.keys.toList(growable: false);
    await BackgroundRefreshController.instance.onConnectionsChanged(
      connectionIds: remainingIds,
      endpointUrlFor: (id) => _endpointUrls[id] ?? '',
    );
  }

  // ---------- 5. 401 interception ----------

  /// Call from the API layer whenever a server returns HTTP 401.
  ///
  /// Attempts an immediate token refresh. If the refresh succeeds the caller
  /// should retry the original request. If it fails, clears tokens and emits
  /// [SessionInvalidatedEvent] so the UI can bounce to the login screen.
  ///
  /// Returns `true` if the refresh succeeded (caller should retry).
  /// Returns `false` if the session is now invalid (caller should abort +
  /// navigate to login).
  Future<bool> handleUnauthorized({required String connectionId}) async {
    final endpointUrl = _endpointUrls[connectionId];
    if (endpointUrl == null) {
      _events.add(SessionInvalidatedEvent(
        connectionId: connectionId,
        reason: '401 but no endpoint URL known — connection was forgotten',
      ));
      return false;
    }

    try {
      await _tokenRefresher.forceRefresh(
        connectionId: connectionId,
        endpointUrl: endpointUrl,
      );
      _events.add(TokenRefreshedEvent(connectionId));
      return true;
    } on AuthInvalidException catch (e) {
      await handleAuthInvalid(e);
      return false;
    } on LoginError catch (e) {
      // KAI-298 security review (PR #149): transient failures must NOT wipe
      // the user's session. Only definitively-invalidating kinds
      // (wrongCredentials, refreshExpired, unknownProvider) invalidate here.
      // 5xx / network / malformed / ssoCancelled are transient: caller may
      // retry on the next authenticated request.
      if (_isTransientRefreshFailure(e.kind)) {
        return false;
      }
      await _tokenStore.deleteTokens(connectionId);
      _events.add(SessionInvalidatedEvent(
        connectionId: connectionId,
        reason: '401 → refresh failed: ${e.debugMessage}',
      ));
      return false;
    }
  }

  /// Treat 5xx, network, malformed, and SSO-cancelled as transient. The
  /// refresh token is still valid; the caller may retry on the next request.
  static bool _isTransientRefreshFailure(LoginErrorKind kind) {
    switch (kind) {
      case LoginErrorKind.network:
      case LoginErrorKind.server: // 5xx / non-2xx non-401 bucket
      case LoginErrorKind.malformed:
      case LoginErrorKind.ssoCancelled:
        return true;
      case LoginErrorKind.wrongCredentials:
      case LoginErrorKind.unknownProvider:
      case LoginErrorKind.refreshExpired:
        return false;
    }
  }

  // ---------- internal ----------

  /// Fire-and-forget background refresh. Errors surface via [events].
  void _tryRefresh({
    required String connectionId,
    required String endpointUrl,
  }) {
    _tokenRefresher
        .ensureFresh(
          connectionId: connectionId,
          endpointUrl: endpointUrl,
        )
        .then((_) => _events.add(TokenRefreshedEvent(connectionId)))
        .catchError((Object err) {
      if (err is AuthInvalidException) {
        handleAuthInvalid(err);
      }
      // Network errors are swallowed — the background controller will retry.
    });
  }

  // ---------- dispose ----------

  /// Release resources. Call on app shutdown.
  Future<void> dispose() async {
    await BackgroundRefreshController.instance.stop();
    await _events.close();
  }
}
