// KAI-299 — CameraDirectoryClient interface + scaffolding impls.
//
// Proto-first. The real listing + watch APIs are owned by the Directory team
// and are NOT invented here — we ship against an interface and list the asks
// in the PR body.
//
// Proto asks (see PR body):
//   1. `rpc ListCameras(ListCamerasRequest) returns (ListCamerasResponse)`
//      over Connect-Go, mapped to `GET /api/v1/cameras`.
//   2. `rpc WatchCameraStatus(WatchCameraStatusRequest) returns (stream
//      CameraStatusEvent)` — server-streaming Connect RPC, which the Flutter
//      client consumes via `WS /api/v1/cameras/watch` or Connect-web stream.
//
// Until those land, [HttpCameraDirectoryClient] returns an empty list and an
// empty broadcast stream — callers must tolerate no data. Tests use
// [FakeCameraDirectoryClient].

import 'dart:async';

import '../models/camera.dart';
import '../state/home_directory_connection.dart';

/// A single live-status event for one camera. Produced by
/// [CameraDirectoryClient.watchStatus] (real impl) or by
/// [FakeCameraDirectoryClient] in tests.
class CameraStatusEvent {
  final String cameraId;

  /// `null` = unknown (peer disconnected / no data), `true` = online,
  /// `false` = offline.
  final bool? isOnline;

  /// Monotonic timestamp from the Directory. Client clocks are untrusted.
  final DateTime lastSeen;

  /// Optional human-readable reason for the transition. Useful for the UI
  /// tooltip ("Camera unreachable: ONVIF timeout").
  final String? reason;

  const CameraStatusEvent({
    required this.cameraId,
    required this.isOnline,
    required this.lastSeen,
    this.reason,
  });
}

/// Interface for anything that can list cameras and stream their live status
/// for a single [HomeDirectoryConnection] (home OR federated peer — the peer
/// is selected by the caller providing the right connection object).
abstract class CameraDirectoryClient {
  /// Fetch the current camera list for [conn]. May throw if the directory
  /// is unreachable or if the user lacks list permission.
  Future<List<Camera>> listCameras(HomeDirectoryConnection conn);

  /// Open a live-status stream for [conn]. The stream is a broadcast stream
  /// so multiple widgets can listen. Closing the returned subscription must
  /// release underlying resources (socket, HTTP/2 stream, etc).
  Stream<CameraStatusEvent> watchStatus(HomeDirectoryConnection conn);
}

/// In-memory fake for tests. Lets the test pump status events into the
/// stream and assert merging behavior in [CameraStatusNotifier] and friends.
class FakeCameraDirectoryClient implements CameraDirectoryClient {
  final Map<String, List<Camera>> _camerasByConnection;
  final Map<String, StreamController<CameraStatusEvent>> _controllers = {};

  FakeCameraDirectoryClient({
    Map<String, List<Camera>>? camerasByConnection,
  }) : _camerasByConnection = camerasByConnection ?? {};

  void setCameras(String connectionId, List<Camera> cameras) {
    _camerasByConnection[connectionId] = cameras;
  }

  /// Push an event to every current subscriber of [connectionId].
  void pushStatus(String connectionId, CameraStatusEvent event) {
    final c = _controllers[connectionId];
    if (c != null && !c.isClosed) {
      c.add(event);
    }
  }

  /// Close a specific peer's status stream (simulates a disconnect).
  Future<void> closeStream(String connectionId) async {
    final c = _controllers.remove(connectionId);
    if (c != null) {
      await c.close();
    }
  }

  /// Close every open stream. Call from `tearDown`.
  Future<void> dispose() async {
    for (final c in _controllers.values) {
      await c.close();
    }
    _controllers.clear();
  }

  @override
  Future<List<Camera>> listCameras(HomeDirectoryConnection conn) async {
    return _camerasByConnection[conn.id] ?? const [];
  }

  @override
  Stream<CameraStatusEvent> watchStatus(HomeDirectoryConnection conn) {
    final existing = _controllers[conn.id];
    if (existing != null && !existing.isClosed) {
      return existing.stream;
    }
    final ctrl = StreamController<CameraStatusEvent>.broadcast();
    _controllers[conn.id] = ctrl;
    return ctrl.stream;
  }
}

/// Stub HTTP impl — does not make real network calls yet. Left as a named
/// class so consumers can be wired up in DI today; the body will light up
/// once the proto ask (see file header) is satisfied.
class HttpCameraDirectoryClient implements CameraDirectoryClient {
  // Intentionally opaque. Real impl will take a dio/http.Client and an
  // AppSession for token injection.
  final Object? _httpClient;

  HttpCameraDirectoryClient({Object? httpClient}) : _httpClient = httpClient;

  // ignore: unused_element
  Object? get _client => _httpClient;

  @override
  Future<List<Camera>> listCameras(HomeDirectoryConnection conn) async {
    // TODO(KAI-299): call ListCameras RPC once proto lands.
    //   GET ${conn.endpointUrl}/api/v1/cameras with bearer token.
    return const [];
  }

  @override
  Stream<CameraStatusEvent> watchStatus(HomeDirectoryConnection conn) {
    // TODO(KAI-299): open WatchCameraStatus stream once proto lands.
    //   WS ${conn.endpointUrl}/api/v1/cameras/watch with bearer token.
    // For now we return an empty broadcast stream so callers can subscribe
    // without special-casing null.
    final ctrl = StreamController<CameraStatusEvent>.broadcast();
    // Intentionally never closed — the caller owns the subscription.
    return ctrl.stream;
  }
}
