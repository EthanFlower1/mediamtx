// KAI-295 — AppSession state type + Riverpod notifier.
//
// AppSession is the single source of truth for "who is logged in to which
// home, and what peers do they know about". The hard invariant is:
//
//     The app maintains exactly one active home connection at a time,
//     with cached state for federated peers.
//
// We enforce that invariant in [AppSessionNotifier.switchConnection]: every
// path that changes the active connection runs through `_setActive`, which
// (a) wipes in-memory peer cache, (b) does NOT touch the previous connection's
// secure-storage tokens (they stay scoped under the old connection ID and are
// only purged on explicit forget), and (c) hydrates the new connection's
// tokens from secure storage. The "no token leakage" property is covered by
// `test/state/app_session_test.dart`.
//
// Tokens themselves are NOT serialised into the AppSession JSON blob. They
// live in secure storage keyed by connection ID. The JSON blob holds only the
// active connection ID + a non-secret user reference, so a hot-restart can
// rehydrate the right secure-storage namespace.
//
// Seam note (KAI-222): `accessToken` and `refreshToken` are intentionally
// `String?` rather than typed Zitadel claims, so the cloud-platform team can
// pick any IdentityProvider implementation without breaking this layer.

import 'dart:async';
import 'dart:convert';

import 'package:flutter_riverpod/flutter_riverpod.dart';

import 'federation_peer.dart';
import 'home_directory_connection.dart';
import 'secure_token_store.dart';

/// In-memory snapshot of the user's session.
///
/// `accessToken` / `refreshToken` are present in memory only while the session
/// is active. They are persisted to secure storage by the notifier, never to
/// SharedPreferences and never to the JSON blob below.
class AppSession {
  /// Stable user identifier from the IdP. Opaque string for IdP independence.
  final String userId;

  /// Tenant the user belongs to. Opaque string — could be a slug, UUID, etc.
  final String tenantRef;

  /// Currently active access token, in memory only. May be `null` between a
  /// `logout` and a re-login.
  final String? accessToken;

  /// Currently active refresh token, in memory only.
  final String? refreshToken;

  /// The single active home connection. `null` only before any connection has
  /// been added — see invariant in the file header.
  final HomeDirectoryConnection? activeConnection;

  /// All home directories the user has added. The active one is always
  /// included here. Order is insertion order (most-recently-added last).
  /// Added in KAI-304 to power the Accounts list UI without changing the
  /// single-active-connection invariant.
  final List<HomeDirectoryConnection> knownConnections;

  /// Cached peer snapshots scoped to [activeConnection]. Empty when the user
  /// switches connections, until federation sync repopulates it.
  final List<FederationPeer> knownPeers;

  const AppSession({
    required this.userId,
    required this.tenantRef,
    this.accessToken,
    this.refreshToken,
    this.activeConnection,
    this.knownConnections = const [],
    this.knownPeers = const [],
  });

  /// Empty / signed-out session.
  static const AppSession empty = AppSession(
    userId: '',
    tenantRef: '',
  );

  bool get isAuthenticated =>
      accessToken != null &&
      accessToken!.isNotEmpty &&
      activeConnection != null;

  AppSession copyWith({
    String? userId,
    String? tenantRef,
    String? accessToken,
    String? refreshToken,
    HomeDirectoryConnection? activeConnection,
    List<HomeDirectoryConnection>? knownConnections,
    List<FederationPeer>? knownPeers,
    bool clearTokens = false,
    bool clearActiveConnection = false,
  }) {
    return AppSession(
      userId: userId ?? this.userId,
      tenantRef: tenantRef ?? this.tenantRef,
      accessToken: clearTokens ? null : (accessToken ?? this.accessToken),
      refreshToken: clearTokens ? null : (refreshToken ?? this.refreshToken),
      activeConnection: clearActiveConnection
          ? null
          : (activeConnection ?? this.activeConnection),
      knownConnections: knownConnections ?? this.knownConnections,
      knownPeers: knownPeers ?? this.knownPeers,
    );
  }

  /// Non-secret JSON blob suitable for SharedPreferences hydration.
  ///
  /// Tokens are intentionally omitted — they live in secure storage.
  Map<String, dynamic> toJson() => {
        'user_id': userId,
        'tenant_ref': tenantRef,
        'active_connection': activeConnection?.toJson(),
        'known_connections':
            knownConnections.map((c) => c.toJson()).toList(),
        'known_peers': knownPeers.map((p) => p.toJson()).toList(),
      };

  factory AppSession.fromJson(Map<String, dynamic> json) {
    return AppSession(
      userId: json['user_id'] as String? ?? '',
      tenantRef: json['tenant_ref'] as String? ?? '',
      activeConnection: json['active_connection'] == null
          ? null
          : HomeDirectoryConnection.fromJson(
              Map<String, dynamic>.from(json['active_connection'] as Map),
            ),
      knownConnections: ((json['known_connections'] as List?) ?? const [])
          .map((e) => HomeDirectoryConnection.fromJson(
              Map<String, dynamic>.from(e as Map)))
          .toList(growable: false),
      knownPeers: ((json['known_peers'] as List?) ?? const [])
          .map((e) =>
              FederationPeer.fromJson(Map<String, dynamic>.from(e as Map)))
          .toList(growable: false),
    );
  }

  String encode() => jsonEncode(toJson());

  factory AppSession.decode(String s) =>
      AppSession.fromJson(Map<String, dynamic>.from(jsonDecode(s) as Map));
}

// -------------------- Session lifecycle events (KAI-304) --------------------
//
// Surfaces account-switch transitions to subscribers that own live streams or
// playback — they listen on the sink, tear down cleanly on [SessionSwitching],
// and acknowledge via `ackSwitchDrained`. This file only defines the sink; the
// live-view/playback wiring lands with KAI-301/302.

/// Marker base type for events emitted on [SessionEventSink].
sealed class SessionEvent {
  const SessionEvent();
}

/// Emitted before the active connection is swapped. Subscribers holding
/// streams or sockets must tear them down and then call
/// [SessionEventSink.ackSwitchDrained].
class SessionSwitchingEvent extends SessionEvent {
  final String fromConnectionId;
  final String toConnectionId;
  const SessionSwitchingEvent({
    required this.fromConnectionId,
    required this.toConnectionId,
  });
}

/// Emitted after the new connection is active and its tokens are loaded.
class SessionSwitchedEvent extends SessionEvent {
  final String connectionId;
  const SessionSwitchedEvent(this.connectionId);
}

/// Broadcast sink that subscribers consume. One instance per [AppSessionNotifier].
class SessionEventSink {
  final StreamController<SessionEvent> _ctrl =
      StreamController<SessionEvent>.broadcast();
  int _pendingDrains = 0;
  Completer<void>? _drainCompleter;

  Stream<SessionEvent> get stream => _ctrl.stream;

  /// Subscribers report they have torn down. When all registered subscribers
  /// have ack'd, the pending drain future completes.
  void ackSwitchDrained() {
    if (_pendingDrains > 0) {
      _pendingDrains -= 1;
      if (_pendingDrains == 0 && !(_drainCompleter?.isCompleted ?? true)) {
        _drainCompleter!.complete();
      }
    }
  }

  /// Test/live-view hook — how many subscribers the sink should wait for on
  /// the next switch. If zero (the default), the drain resolves immediately.
  int expectedDrainAcks = 0;

  Future<void> _emitSwitchingAndWaitForDrain(
    SessionSwitchingEvent event, {
    Duration timeout = const Duration(seconds: 2),
  }) async {
    _pendingDrains = expectedDrainAcks;
    _drainCompleter = Completer<void>();
    _ctrl.add(event);
    if (_pendingDrains == 0) {
      _drainCompleter!.complete();
    }
    try {
      await _drainCompleter!.future.timeout(timeout);
    } on TimeoutException {
      // Hard cap: switch proceeds even if a subscriber hangs. The contract
      // is "best-effort tear-down", not "blocking handshake".
    }
  }

  void _emitSwitched(SessionSwitchedEvent event) => _ctrl.add(event);

  Future<void> dispose() => _ctrl.close();
}

/// Provider for the session event sink. Subscribers (live view, playback)
/// will watch this to know when to tear down.
final sessionEventSinkProvider = Provider<SessionEventSink>((ref) {
  final sink = SessionEventSink();
  ref.onDispose(sink.dispose);
  return sink;
});

/// Notifier that owns [AppSession] and enforces the single-active-connection
/// invariant. Tests instantiate this directly with an [InMemorySecureTokenStore]
/// to verify token isolation.
class AppSessionNotifier extends StateNotifier<AppSession> {
  final SecureTokenStore _tokens;
  final SessionEventSink _events;

  AppSessionNotifier(this._tokens, {SessionEventSink? events})
      : _events = events ?? SessionEventSink(),
        super(AppSession.empty);

  /// The event sink consumed by live-view / playback subscribers.
  SessionEventSink get events => _events;

  /// Begin a session against [connection]. Loads any previously stored tokens
  /// for that connection ID from secure storage and sets it as the single
  /// active connection.
  Future<void> activateConnection({
    required HomeDirectoryConnection connection,
    required String userId,
    required String tenantRef,
  }) async {
    final access =
        await _tokens.read(ConnectionScopedKeys.accessToken(connection.id));
    final refresh =
        await _tokens.read(ConnectionScopedKeys.refreshToken(connection.id));
    final nextKnown = _mergeKnown(state.knownConnections, connection);
    state = AppSession(
      userId: userId,
      tenantRef: tenantRef,
      accessToken: access,
      refreshToken: refresh,
      activeConnection: connection,
      knownConnections: nextKnown,
      knownPeers: const [],
    );
  }

  /// Merge [incoming] into [existing], replacing any entry with the same id.
  static List<HomeDirectoryConnection> _mergeKnown(
    List<HomeDirectoryConnection> existing,
    HomeDirectoryConnection incoming,
  ) {
    final idx = existing.indexWhere((c) => c.id == incoming.id);
    if (idx == -1) {
      return List<HomeDirectoryConnection>.unmodifiable(
          [...existing, incoming]);
    }
    final next = [...existing];
    next[idx] = incoming;
    return List<HomeDirectoryConnection>.unmodifiable(next);
  }

  /// Persist a fresh token pair for the *currently active* connection. Throws
  /// if no connection is active — every token must be scoped to a directory.
  Future<void> setTokens({
    required String accessToken,
    required String refreshToken,
  }) async {
    final conn = state.activeConnection;
    if (conn == null) {
      throw StateError(
          'AppSessionNotifier.setTokens called with no active connection');
    }
    await _tokens.write(
      ConnectionScopedKeys.accessToken(conn.id),
      accessToken,
    );
    await _tokens.write(
      ConnectionScopedKeys.refreshToken(conn.id),
      refreshToken,
    );
    state = state.copyWith(
      accessToken: accessToken,
      refreshToken: refreshToken,
    );
  }

  /// Switch to a different home connection.
  ///
  /// Per the spec, this is the operation that *must not* leak tokens between
  /// connections. We:
  ///   1. Drop the previous in-memory tokens.
  ///   2. Clear the in-memory peer cache (peers belong to a specific home).
  ///   3. Load the target connection's tokens from secure storage (if any).
  ///
  /// We deliberately do NOT delete the previous connection's secure-storage
  /// tokens — the user may switch back. Use [forgetConnection] for that.
  Future<void> switchConnection({
    required HomeDirectoryConnection target,
    String? userId,
    String? tenantRef,
  }) async {
    final access =
        await _tokens.read(ConnectionScopedKeys.accessToken(target.id));
    final refresh =
        await _tokens.read(ConnectionScopedKeys.refreshToken(target.id));
    state = AppSession(
      userId: userId ?? state.userId,
      tenantRef: tenantRef ?? state.tenantRef,
      accessToken: access,
      refreshToken: refresh,
      activeConnection: target,
      knownPeers: const [],
    );
  }

  /// Replace the cached peers for the active connection.
  void updatePeers(List<FederationPeer> peers) {
    state = state.copyWith(knownPeers: List.unmodifiable(peers));
  }

  /// Update a single peer (e.g. after a status transition). Adds the peer if
  /// it isn't already cached.
  void upsertPeer(FederationPeer peer) {
    final next = [...state.knownPeers];
    final i = next.indexWhere((p) => p.peerId == peer.peerId);
    if (i == -1) {
      next.add(peer);
    } else {
      next[i] = peer;
    }
    state = state.copyWith(knownPeers: List.unmodifiable(next));
  }

  /// Sign out of the active connection. Wipes in-memory tokens; the on-disk
  /// secure-storage entries are also removed so a stolen device can't replay.
  Future<void> logout() async {
    final conn = state.activeConnection;
    if (conn != null) {
      await _tokens.delete(ConnectionScopedKeys.accessToken(conn.id));
      await _tokens.delete(ConnectionScopedKeys.refreshToken(conn.id));
    }
    state = state.copyWith(clearTokens: true, knownPeers: const []);
  }

  /// Forget the connection entirely: secure-storage entries removed, no way
  /// to silently re-auth without going through discovery again.
  Future<void> forgetConnection(String connectionId) async {
    await _tokens.deleteByPrefix(ConnectionScopedKeys.prefix(connectionId));
    if (state.activeConnection?.id == connectionId) {
      state = AppSession.empty;
    }
  }

  // -------------------- KAI-304 account switcher API --------------------

  /// Switch the active account to [connectionId]. The connection must already
  /// be in [AppSession.knownConnections] (add it first with [addAccount]).
  ///
  /// Emits [SessionSwitchingEvent], awaits a brief drain from stream
  /// subscribers (hard 2s cap), swaps the active connection, then emits
  /// [SessionSwitchedEvent]. The drain window is how live-view/playback tear
  /// down cleanly without leaking sockets across accounts.
  Future<void> switchTo(String connectionId) async {
    final target = state.knownConnections
        .firstWhere((c) => c.id == connectionId);
    final fromId = state.activeConnection?.id ?? '';

    await _events._emitSwitchingAndWaitForDrain(
      SessionSwitchingEvent(
        fromConnectionId: fromId,
        toConnectionId: connectionId,
      ),
    );

    final access =
        await _tokens.read(ConnectionScopedKeys.accessToken(target.id));
    final refresh =
        await _tokens.read(ConnectionScopedKeys.refreshToken(target.id));
    state = AppSession(
      userId: state.userId,
      tenantRef: state.tenantRef,
      accessToken: access,
      refreshToken: refresh,
      activeConnection: target,
      knownConnections: state.knownConnections,
      knownPeers: const [],
    );

    _events._emitSwitched(SessionSwitchedEvent(target.id));
  }

  /// Append [connection] to the known accounts list and persist its tokens.
  /// If a connection with the same id already exists it is replaced.
  ///
  /// This does NOT make the new account active — callers follow up with
  /// [switchTo] if they want to activate it. Typical Add-Account flow is
  /// discovery → login → [addAccount] → [switchTo].
  Future<void> addAccount({
    required HomeDirectoryConnection connection,
    required String accessToken,
    required String refreshToken,
  }) async {
    await _tokens.write(
      ConnectionScopedKeys.accessToken(connection.id),
      accessToken,
    );
    await _tokens.write(
      ConnectionScopedKeys.refreshToken(connection.id),
      refreshToken,
    );
    state = state.copyWith(
      knownConnections: _mergeKnown(state.knownConnections, connection),
    );
  }

  /// Sign out of [connectionId]: delete its tokens and remove it from the
  /// known-accounts list. If it was the active account, we promote the first
  /// remaining sibling (if any); otherwise the session returns to empty and
  /// the router will bounce to discovery.
  Future<void> signOut(String connectionId) async {
    await _tokens.deleteByPrefix(ConnectionScopedKeys.prefix(connectionId));
    final remaining = state.knownConnections
        .where((c) => c.id != connectionId)
        .toList(growable: false);
    final wasActive = state.activeConnection?.id == connectionId;

    if (!wasActive) {
      state = state.copyWith(knownConnections: remaining);
      return;
    }

    if (remaining.isEmpty) {
      state = AppSession.empty;
      return;
    }

    final promoted = remaining.first;
    final access =
        await _tokens.read(ConnectionScopedKeys.accessToken(promoted.id));
    final refresh =
        await _tokens.read(ConnectionScopedKeys.refreshToken(promoted.id));
    state = AppSession(
      userId: state.userId,
      tenantRef: state.tenantRef,
      accessToken: access,
      refreshToken: refresh,
      activeConnection: promoted,
      knownConnections: remaining,
      knownPeers: const [],
    );
  }
}

// -------------------- Riverpod providers --------------------
//
// Match the existing pattern in `lib/providers/auth_provider.dart`: a plain
// `Provider` for the service, then a `StateNotifierProvider` for the state.

/// Override this in tests with `secureTokenStoreProvider.overrideWithValue(...)`
/// to inject an [InMemorySecureTokenStore]. Production wires it to a
/// `flutter_secure_storage` adapter (added in a follow-up — it's a separate
/// dependency to keep this PR pure-state).
final secureTokenStoreProvider = Provider<SecureTokenStore>((ref) {
  // Default to in-memory until the flutter_secure_storage adapter ships in
  // KAI-296. This keeps tests hermetic and avoids importing the platform
  // plugin from the state layer.
  return InMemorySecureTokenStore();
});

final appSessionProvider =
    StateNotifierProvider<AppSessionNotifier, AppSession>((ref) {
  final store = ref.watch(secureTokenStoreProvider);
  final sink = ref.watch(sessionEventSinkProvider);
  return AppSessionNotifier(store, events: sink);
});

/// Convenience selector — the currently active home connection or `null`.
final homeConnectionProvider = Provider<HomeDirectoryConnection?>((ref) {
  return ref.watch(appSessionProvider).activeConnection;
});

/// KAI-304 — all home directories the user has added.
final knownAccountsProvider = Provider<List<HomeDirectoryConnection>>((ref) {
  return ref.watch(appSessionProvider).knownConnections;
});

/// Convenience selector — cached federation peers for the active connection.
final federationPeersProvider = Provider<List<FederationPeer>>((ref) {
  return ref.watch(appSessionProvider).knownPeers;
});
