// KAI-299 — Live-status merging across home + federated peers.
//
// The notifier subscribes to one status stream per connection and merges
// events into a single `Map<cameraId, CameraStatus>`. On peer disconnect we
// flip every camera from that peer to `unknown` (not `offline` — we do not
// know the ground truth), so the UI can show a grey dot and a tooltip.
//
// This layer is pure Dart + flutter_riverpod. It does not do networking
// itself — the caller injects a [CameraDirectoryClient].

import 'dart:async';

import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../state/home_directory_connection.dart';
import 'camera_directory_client.dart';

/// Coarse live status for a single camera, as currently known by the client.
enum CameraOnlineState {
  online,
  offline,
  unknown,
}

/// A snapshot of what the client knows about one camera right now.
class CameraStatus {
  final String cameraId;
  final CameraOnlineState state;
  final DateTime lastUpdated;
  final String? reason;

  /// Which connection (home = [homePeerConnectionId], federated = peerId)
  /// this status was sourced from. Needed so a peer disconnect can flip
  /// only its own cameras to `unknown`.
  final String sourceConnectionId;

  /// Direct URL to the Recorder that owns this camera's data plane
  /// (live view, playback, export). `null` means the client should fall back
  /// to the Directory endpoint (all-in-one mode). Populated from
  /// [Camera.recorderEndpoint] when the camera list is fetched.
  final String? recorderEndpoint;

  const CameraStatus({
    required this.cameraId,
    required this.state,
    required this.lastUpdated,
    required this.sourceConnectionId,
    this.reason,
    this.recorderEndpoint,
  });

  CameraStatus copyWith({
    String? cameraId,
    CameraOnlineState? state,
    DateTime? lastUpdated,
    String? sourceConnectionId,
    String? reason,
    String? recorderEndpoint,
  }) {
    return CameraStatus(
      cameraId: cameraId ?? this.cameraId,
      state: state ?? this.state,
      lastUpdated: lastUpdated ?? this.lastUpdated,
      sourceConnectionId: sourceConnectionId ?? this.sourceConnectionId,
      reason: reason ?? this.reason,
      recorderEndpoint: recorderEndpoint ?? this.recorderEndpoint,
    );
  }
}

/// Merges multiple per-connection status streams into a single map keyed by
/// camera ID. See file header for the `unknown` semantics on disconnect.
class CameraStatusNotifier extends StateNotifier<Map<String, CameraStatus>> {
  final CameraDirectoryClient _client;
  final Map<String, StreamSubscription<CameraStatusEvent>> _subs = {};

  /// For each connection, the set of camera IDs we've seen. Needed to mark
  /// them `unknown` on disconnect.
  final Map<String, Set<String>> _camerasByConnection = {};

  CameraStatusNotifier(this._client) : super(const {});

  /// Begin tracking [conn]. Opens a stream subscription and routes every
  /// event into [state]. Idempotent — calling twice for the same connection
  /// is a no-op.
  void track(HomeDirectoryConnection conn) {
    if (_subs.containsKey(conn.id)) return;
    _camerasByConnection[conn.id] ??= <String>{};
    final sub = _client.watchStatus(conn).listen(
      (event) => _onEvent(conn.id, event),
      onError: (_) => _markConnectionUnknown(conn.id),
      onDone: () => _markConnectionUnknown(conn.id),
    );
    _subs[conn.id] = sub;
  }

  /// Stop tracking [connectionId] and mark all of its cameras as `unknown`.
  Future<void> untrack(String connectionId) async {
    final sub = _subs.remove(connectionId);
    if (sub != null) {
      await sub.cancel();
    }
    _markConnectionUnknown(connectionId);
  }

  /// Cancel everything. Call from provider dispose.
  @override
  void dispose() {
    for (final s in _subs.values) {
      s.cancel();
    }
    _subs.clear();
    super.dispose();
  }

  void _onEvent(String connectionId, CameraStatusEvent event) {
    _camerasByConnection[connectionId] ??= <String>{};
    _camerasByConnection[connectionId]!.add(event.cameraId);

    final CameraOnlineState next;
    if (event.isOnline == null) {
      next = CameraOnlineState.unknown;
    } else if (event.isOnline!) {
      next = CameraOnlineState.online;
    } else {
      next = CameraOnlineState.offline;
    }

    final updated = Map<String, CameraStatus>.from(state);
    updated[event.cameraId] = CameraStatus(
      cameraId: event.cameraId,
      state: next,
      lastUpdated: event.lastSeen,
      sourceConnectionId: connectionId,
      reason: event.reason,
    );
    state = Map.unmodifiable(updated);
  }

  void _markConnectionUnknown(String connectionId) {
    final cams = _camerasByConnection[connectionId];
    if (cams == null || cams.isEmpty) return;
    final now = DateTime.now().toUtc();
    final updated = Map<String, CameraStatus>.from(state);
    for (final camId in cams) {
      final existing = updated[camId];
      updated[camId] = CameraStatus(
        cameraId: camId,
        state: CameraOnlineState.unknown,
        lastUpdated: now,
        sourceConnectionId: connectionId,
        reason: existing?.reason,
      );
    }
    state = Map.unmodifiable(updated);
  }
}

/// Provider for the camera status map. The underlying client must be
/// overridden before use — either by the app's DI setup (passing a fully
/// configured [HttpCameraDirectoryClient]) or by tests (passing a
/// [FakeCameraDirectoryClient]).
///
/// The default falls back to [FakeCameraDirectoryClient] (returns empty data)
/// so that widget trees that haven't been wired up yet don't crash.
final cameraDirectoryClientProvider = Provider<CameraDirectoryClient>((ref) {
  return FakeCameraDirectoryClient();
});

final cameraStatusProvider = StateNotifierProvider<CameraStatusNotifier,
    Map<String, CameraStatus>>((ref) {
  final client = ref.watch(cameraDirectoryClientProvider);
  final notifier = CameraStatusNotifier(client);
  ref.onDispose(notifier.dispose);
  return notifier;
});
