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
import 'dart:convert';

import 'package:http/http.dart' as http;

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

/// Real HTTP implementation that talks to the Directory REST API.
///
/// [baseUrl] is the Directory's base URL (e.g. `https://nvr.acme.local`).
/// [tokenProvider] returns a valid Bearer JWT on each call — it may hit
/// secure storage or refresh in the background; callers must not cache the
/// result.
///
/// Endpoints consumed:
///   `GET  $baseUrl/api/v1/cameras`         — list cameras (JSON array)
///   `GET  $baseUrl/api/v1/cameras/watch`   — SSE stream of CameraStatusEvent
class HttpCameraDirectoryClient implements CameraDirectoryClient {
  final String baseUrl;
  final Future<String> Function() tokenProvider;
  final http.Client _httpClient;

  HttpCameraDirectoryClient({
    required this.baseUrl,
    required this.tokenProvider,
    http.Client? httpClient,
  }) : _httpClient = httpClient ?? http.Client();

  /// Builds auth headers, fetching a fresh token each time.
  Future<Map<String, String>> _authHeaders() async {
    final token = await tokenProvider();
    return {
      'Authorization': 'Bearer $token',
      'Accept': 'application/json',
    };
  }

  @override
  Future<List<Camera>> listCameras(HomeDirectoryConnection conn) async {
    final uri = Uri.parse('${conn.endpointUrl}/api/v1/cameras');
    final headers = await _authHeaders();
    final response = await _httpClient.get(uri, headers: headers);

    if (response.statusCode != 200) {
      throw HttpException(
        'listCameras: unexpected status ${response.statusCode}',
        uri: uri,
      );
    }

    final decoded = jsonDecode(response.body);
    // The API may return `{"cameras": [...]}` or a bare `[...]`.
    final List<dynamic> items;
    if (decoded is List) {
      items = decoded;
    } else if (decoded is Map && decoded['cameras'] is List) {
      items = decoded['cameras'] as List<dynamic>;
    } else {
      items = const [];
    }

    return items
        .cast<Map<String, dynamic>>()
        .map(Camera.fromJson)
        .toList();
  }

  @override
  Stream<CameraStatusEvent> watchStatus(HomeDirectoryConnection conn) {
    // SSE stream — we open a streaming GET and parse `data:` lines.
    final ctrl = StreamController<CameraStatusEvent>.broadcast();

    Future<void> connect() async {
      try {
        final uri = Uri.parse('${conn.endpointUrl}/api/v1/cameras/watch');
        final token = await tokenProvider();
        final request = http.Request('GET', uri)
          ..headers['Authorization'] = 'Bearer $token'
          ..headers['Accept'] = 'text/event-stream'
          ..headers['Cache-Control'] = 'no-cache';

        final streamed = await _httpClient.send(request);
        if (ctrl.isClosed) return;

        // Parse SSE: collect data lines across multi-line events and emit.
        final lines = streamed.stream
            .transform(utf8.decoder)
            .transform(const LineSplitter());

        String dataBuffer = '';
        await for (final line in lines) {
          if (ctrl.isClosed) break;
          if (line.startsWith('data:')) {
            dataBuffer += line.substring(5).trim();
          } else if (line.isEmpty && dataBuffer.isNotEmpty) {
            // End of one SSE event — parse the accumulated data JSON.
            try {
              final map = jsonDecode(dataBuffer) as Map<String, dynamic>;
              final event = _parseStatusEvent(map);
              ctrl.add(event);
            } catch (_) {
              // Ignore malformed events; do not close the stream.
            }
            dataBuffer = '';
          }
        }

        // Server closed the connection — signal done so the notifier can mark
        // cameras as unknown and reconnect if it wants to.
        if (!ctrl.isClosed) ctrl.close();
      } catch (e) {
        if (!ctrl.isClosed) ctrl.addError(e);
      }
    }

    connect();
    return ctrl.stream;
  }

  /// Parse a raw SSE JSON payload into a [CameraStatusEvent].
  ///
  /// Expected shape:
  /// ```json
  /// {
  ///   "camera_id": "abc",
  ///   "is_online": true,
  ///   "last_seen": "2026-04-20T12:00:00Z",
  ///   "reason": "ONVIF timeout"   // optional
  /// }
  /// ```
  CameraStatusEvent _parseStatusEvent(Map<String, dynamic> map) {
    final rawLastSeen = map['last_seen'];
    final lastSeen = rawLastSeen != null
        ? DateTime.parse(rawLastSeen as String)
        : DateTime.now().toUtc();

    return CameraStatusEvent(
      cameraId: map['camera_id'] as String,
      isOnline: map['is_online'] as bool?,
      lastSeen: lastSeen,
      reason: map['reason'] as String?,
    );
  }
}

/// Thrown when a non-2xx response is received.
class HttpException implements Exception {
  final String message;
  final Uri uri;
  const HttpException(this.message, {required this.uri});

  @override
  String toString() => 'HttpException($message, uri: $uri)';
}
