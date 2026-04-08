// KAI-300 — StreamsApi: stream endpoint request with ordered fallback list.
//
// This class is the Flutter-side seam for KAI-255 (stream URL minting service).
// The real HTTP call is stubbed; search for TODO(KAI-255) to find the single
// file-change needed when the server-side minting API merges.
//
// Wire shape mirrors §9.1 of the multi-recording-server-design spec:
//   POST /api/v1/streams/request
//   {
//     "camera_id": "<id>",
//     "kind": "live" | "playback",
//     "protocol": "webrtc" | "llhls" | "auto"
//   }
//
//   Response:
//   {
//     "stream_id": "<uuid>",
//     "expires_at": "<ISO-8601>",
//     "endpoints": [
//       {
//         "url": "https://nvr.acme.local:8889/cam1/whep",
//         "transport": "webrtc",
//         "connection_type": "lan_direct",
//         "priority": 0,
//         "estimated_latency_ms": 15
//       },
//       ...
//     ]
//   }

/// How the stream endpoint routes to the client.
enum StreamConnectionType {
  lanDirect,
  gateway,
  managedRelay,
}

/// Transport protocol for the endpoint.
enum StreamTransport {
  webrtc,
  llhls,
}

/// A single resolved endpoint from the minting service.
class StreamEndpoint {
  final String url;
  final StreamTransport transport;
  final StreamConnectionType connectionType;

  /// Lower = higher priority. Ordered list is LAN-direct first.
  final int priority;

  /// Advisory latency estimate in milliseconds. May be null if unknown.
  final int? estimatedLatencyMs;

  const StreamEndpoint({
    required this.url,
    required this.transport,
    required this.connectionType,
    required this.priority,
    this.estimatedLatencyMs,
  });

  factory StreamEndpoint.fromJson(Map<String, dynamic> json) {
    return StreamEndpoint(
      url: json['url'] as String,
      transport: _transportFromWire(json['transport'] as String),
      connectionType: _connTypeFromWire(json['connection_type'] as String),
      priority: json['priority'] as int? ?? 0,
      estimatedLatencyMs: json['estimated_latency_ms'] as int?,
    );
  }

  static StreamTransport _transportFromWire(String s) {
    switch (s) {
      case 'webrtc':
        return StreamTransport.webrtc;
      case 'llhls':
        return StreamTransport.llhls;
      default:
        return StreamTransport.webrtc;
    }
  }

  static StreamConnectionType _connTypeFromWire(String s) {
    switch (s) {
      case 'lan_direct':
        return StreamConnectionType.lanDirect;
      case 'gateway':
        return StreamConnectionType.gateway;
      case 'managed_relay':
        return StreamConnectionType.managedRelay;
      default:
        return StreamConnectionType.managedRelay;
    }
  }

  /// Human-readable label for the quality indicator badge.
  String get connectionLabel {
    switch (connectionType) {
      case StreamConnectionType.lanDirect:
        return 'LAN';
      case StreamConnectionType.gateway:
        return 'GW';
      case StreamConnectionType.managedRelay:
        return 'Relay';
    }
  }
}

/// Result returned by [StreamsApi.requestStream].
class StreamRequest {
  final String streamId;
  final DateTime expiresAt;

  /// Ordered list of endpoints — try index 0 first.
  final List<StreamEndpoint> endpoints;

  const StreamRequest({
    required this.streamId,
    required this.expiresAt,
    required this.endpoints,
  });

  factory StreamRequest.fromJson(Map<String, dynamic> json) {
    final rawEndpoints = (json['endpoints'] as List<dynamic>?) ?? [];
    final endpoints = rawEndpoints
        .map((e) => StreamEndpoint.fromJson(e as Map<String, dynamic>))
        .toList(growable: false)
      ..sort((a, b) => a.priority.compareTo(b.priority));
    return StreamRequest(
      streamId: json['stream_id'] as String,
      expiresAt: DateTime.parse(json['expires_at'] as String),
      endpoints: endpoints,
    );
  }
}

/// Possible kinds of streams the client can request.
enum StreamKind { live, playback }

/// Preferred transport hint sent to the server.
enum StreamProtocol { webrtc, llhls, auto }

/// Client-side stub for the KAI-255 stream-URL-minting service.
///
/// At integration time swap the stub body in [requestStream] for a real
/// `http.Client` (or Dio) call — everything else in the live-view feature
/// reads from [StreamRequest] and is transport-agnostic.
abstract class StreamsApi {
  /// Request a stream for [cameraId].
  ///
  /// Returns an ordered list of endpoints. The caller is responsible for trying
  /// them in priority order and handling fallback.
  Future<StreamRequest> requestStream({
    required String cameraId,
    required String baseUrl,
    required String accessToken,
    StreamKind kind = StreamKind.live,
    StreamProtocol protocol = StreamProtocol.auto,
  });
}

/// Production implementation — one file change needed when KAI-255 merges.
class HttpStreamsApi implements StreamsApi {
  const HttpStreamsApi();

  @override
  Future<StreamRequest> requestStream({
    required String cameraId,
    required String baseUrl,
    required String accessToken,
    StreamKind kind = StreamKind.live,
    StreamProtocol protocol = StreamProtocol.auto,
  }) async {
    // TODO(KAI-255): Replace stub with real HTTP call:
    //   final resp = await http.post(
    //     Uri.parse('$baseUrl/api/v1/streams/request'),
    //     headers: {
    //       'Content-Type': 'application/json',
    //       'Authorization': 'Bearer $accessToken',
    //     },
    //     body: jsonEncode({
    //       'camera_id': cameraId,
    //       'kind': kind.name,
    //       'protocol': protocol.name,
    //     }),
    //   );
    //   if (resp.statusCode != 200) throw StreamRequestException(resp.statusCode);
    //   return StreamRequest.fromJson(jsonDecode(resp.body) as Map<String, dynamic>);

    // ---- STUB: returns realistic synthetic endpoints ----
    await Future.delayed(const Duration(milliseconds: 80));

    // Derive WHEP and LL-HLS URLs from baseUrl so integration tests can
    // override baseUrl to point at a real server.
    final serverUri = Uri.parse(baseUrl);
    final rtcHost = '${serverUri.scheme}://${serverUri.host}';

    return StreamRequest(
      streamId: 'stub-stream-$cameraId',
      expiresAt: DateTime.now().toUtc().add(const Duration(hours: 1)),
      endpoints: [
        StreamEndpoint(
          url: '$rtcHost:8889/$cameraId/whep',
          transport: StreamTransport.webrtc,
          connectionType: StreamConnectionType.lanDirect,
          priority: 0,
          estimatedLatencyMs: 15,
        ),
        StreamEndpoint(
          url: '$rtcHost:8888/$cameraId/index.m3u8',
          transport: StreamTransport.llhls,
          connectionType: StreamConnectionType.lanDirect,
          priority: 1,
          estimatedLatencyMs: 1500,
        ),
        StreamEndpoint(
          url: 'https://relay.kaivue.cloud/$cameraId/whep',
          transport: StreamTransport.webrtc,
          connectionType: StreamConnectionType.managedRelay,
          priority: 2,
          estimatedLatencyMs: 150,
        ),
      ],
    );
  }
}

/// Thrown when the streams API returns a non-2xx response.
class StreamRequestException implements Exception {
  final int statusCode;
  final String? message;

  const StreamRequestException(this.statusCode, {this.message});

  @override
  String toString() =>
      'StreamRequestException(statusCode: $statusCode, message: $message)';
}
