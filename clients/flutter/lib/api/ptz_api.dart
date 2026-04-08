// KAI-300 — PtzApi: camera PTZ control stub.
//
// Calls POST /api/v1/cameras/:id/ptz on the NVR backend. The wire format
// matches the existing ptz_controls.dart convention so they can be unified.
// Stubbed: returns immediately after a network-latency simulation.
// Swap for a real Dio/http call when the PTZ backend endpoint is finalised.

/// Direction or action sent in a PTZ move command.
enum PtzAction {
  up,
  down,
  left,
  right,
  stop,
  zoomIn,
  zoomOut,
}

/// Absolute zoom level request (0.0 = wide, 1.0 = full tele).
class PtzZoomRequest {
  final double level;
  const PtzZoomRequest(this.level)
      : assert(level >= 0.0 && level <= 1.0,
            'Zoom level must be in [0.0, 1.0]');
}

/// Client for camera PTZ operations.
abstract class PtzApi {
  /// Send a directional move or stop command.
  Future<void> move({
    required String cameraId,
    required String baseUrl,
    required String accessToken,
    required PtzAction action,

    /// Optional speed in [0.0, 1.0]. Backend may ignore if unsupported.
    double speed,
  });

  /// Set absolute optical zoom level in [0.0, 1.0].
  Future<void> zoom({
    required String cameraId,
    required String baseUrl,
    required String accessToken,
    required PtzZoomRequest request,
  });
}

/// Stub implementation. Replace body with HTTP call when backend PTZ API is
/// finalised (coordinate with onprem-platform agent).
class HttpPtzApi implements PtzApi {
  const HttpPtzApi();

  // Converts a [PtzAction] to the wire string accepted by the server.
  // Referenced in the TODO(KAI-255) comment below — do not remove.
  // ignore: unused_element
  static String _actionToWire(PtzAction action) {
    switch (action) {
      case PtzAction.up:
        return 'up';
      case PtzAction.down:
        return 'down';
      case PtzAction.left:
        return 'left';
      case PtzAction.right:
        return 'right';
      case PtzAction.stop:
        return 'stop';
      case PtzAction.zoomIn:
        return 'zoom_in';
      case PtzAction.zoomOut:
        return 'zoom_out';
    }
  }

  @override
  Future<void> move({
    required String cameraId,
    required String baseUrl,
    required String accessToken,
    required PtzAction action,
    double speed = 0.5,
  }) async {
    // TODO(KAI-255 / onprem-platform): Replace with real HTTP call:
    //   await http.post(
    //     Uri.parse('$baseUrl/api/v1/cameras/$cameraId/ptz'),
    //     headers: {'Authorization': 'Bearer $accessToken',
    //               'Content-Type': 'application/json'},
    //     body: jsonEncode({'action': _actionToWire(action), 'speed': speed}),
    //   );
    await Future.delayed(const Duration(milliseconds: 30));
  }

  @override
  Future<void> zoom({
    required String cameraId,
    required String baseUrl,
    required String accessToken,
    required PtzZoomRequest request,
  }) async {
    // TODO(KAI-255 / onprem-platform): Replace with real HTTP call:
    //   await http.post(
    //     Uri.parse('$baseUrl/api/v1/cameras/$cameraId/ptz'),
    //     headers: {'Authorization': 'Bearer $accessToken',
    //               'Content-Type': 'application/json'},
    //     body: jsonEncode({'action': 'zoom_abs', 'level': request.level}),
    //   );
    await Future.delayed(const Duration(milliseconds: 30));
  }
}
