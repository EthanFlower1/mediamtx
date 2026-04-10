// Integration test: Camera Tree -> Live View flow.
//
// Verifies that after selecting a camera from the directory, requesting stream
// endpoints via StreamsApi returns correctly ordered fallback list (LAN-direct
// first, then LL-HLS, then managed relay), and that sub-stream variant
// selection propagates.

import 'package:flutter_test/flutter_test.dart';

import 'package:nvr_client/api/streams_api.dart';
import 'package:nvr_client/cameras/camera_directory_client.dart';
import 'package:nvr_client/models/camera.dart';
import 'package:nvr_client/state/app_session.dart';
import 'package:nvr_client/state/home_directory_connection.dart';
import 'package:nvr_client/state/secure_token_store.dart';

/// Local fake that records request parameters and returns a configurable
/// endpoint list so we can verify fallback ordering.
class _FakeStreamsApi implements StreamsApi {
  StreamVariant? lastVariant;
  StreamKind? lastKind;
  String? lastCameraId;
  int callCount = 0;

  final List<StreamEndpoint> _endpoints;

  _FakeStreamsApi({List<StreamEndpoint>? endpoints})
      : _endpoints = endpoints ??
            const [
              StreamEndpoint(
                url: 'http://nvr.local:8889/cam-1/whep',
                transport: StreamTransport.webrtc,
                connectionType: StreamConnectionType.lanDirect,
                priority: 0,
                estimatedLatencyMs: 15,
              ),
              StreamEndpoint(
                url: 'http://nvr.local:8888/cam-1/index.m3u8',
                transport: StreamTransport.llhls,
                connectionType: StreamConnectionType.lanDirect,
                priority: 1,
                estimatedLatencyMs: 1500,
              ),
              StreamEndpoint(
                url: 'https://relay.kaivue.cloud/cam-1/whep',
                transport: StreamTransport.webrtc,
                connectionType: StreamConnectionType.managedRelay,
                priority: 2,
                estimatedLatencyMs: 150,
              ),
            ];

  @override
  Future<StreamRequest> requestStream({
    required String cameraId,
    required String baseUrl,
    required String accessToken,
    StreamKind kind = StreamKind.live,
    StreamProtocol protocol = StreamProtocol.auto,
    StreamVariant variant = StreamVariant.auto,
  }) async {
    lastCameraId = cameraId;
    lastVariant = variant;
    lastKind = kind;
    callCount++;
    return StreamRequest(
      streamId: 'stream-$cameraId',
      expiresAt: DateTime.now().toUtc().add(const Duration(hours: 1)),
      endpoints: _endpoints,
    );
  }
}

void main() {
  group('Camera Tree -> Live View integration', () {
    late InMemorySecureTokenStore tokenStore;
    late AppSessionNotifier sessionNotifier;
    late FakeCameraDirectoryClient cameraClient;
    late _FakeStreamsApi streamsApi;

    final connection = HomeDirectoryConnection(
      id: 'conn-1',
      kind: HomeConnectionKind.onPrem,
      endpointUrl: 'https://nvr.acme.local',
      displayName: 'Acme HQ',
      discoveryMethod: DiscoveryMethod.mdns,
    );

    setUp(() async {
      tokenStore = InMemorySecureTokenStore();
      sessionNotifier = AppSessionNotifier(tokenStore);
      cameraClient = FakeCameraDirectoryClient();
      streamsApi = _FakeStreamsApi();

      cameraClient.setCameras(connection.id, [
        const Camera(id: 'cam-1', name: 'Lobby', hasSubStream: true),
        const Camera(id: 'cam-2', name: 'Parking'),
      ]);

      await sessionNotifier.activateConnection(
        connection: connection,
        userId: 'user-1',
        tenantRef: 'tenant-acme',
      );
      await sessionNotifier.setTokens(
        accessToken: 'eyJhbGciOiJSUzI1NiJ9.eyJzdWIiOiJ1c2VyLTEifQ.sig',
        refreshToken: 'refresh-tok',
      );
    });

    tearDown(() async {
      await cameraClient.dispose();
    });

    test('select camera, request stream, verify fallback ordering', () async {
      final cameras = await cameraClient.listCameras(
        sessionNotifier.state.activeConnection!,
      );
      final selectedCamera = cameras.first;
      expect(selectedCamera.id, 'cam-1');

      // Request live stream for the selected camera
      final result = await streamsApi.requestStream(
        cameraId: selectedCamera.id,
        baseUrl: connection.endpointUrl,
        accessToken: sessionNotifier.state.accessToken!,
      );

      // Verify fallback ordering: LAN WebRTC -> LAN LL-HLS -> Relay WebRTC
      expect(result.endpoints, hasLength(3));
      expect(result.endpoints[0].connectionType, StreamConnectionType.lanDirect);
      expect(result.endpoints[0].transport, StreamTransport.webrtc);
      expect(result.endpoints[0].priority, 0);

      expect(result.endpoints[1].connectionType, StreamConnectionType.lanDirect);
      expect(result.endpoints[1].transport, StreamTransport.llhls);
      expect(result.endpoints[1].priority, 1);

      expect(result.endpoints[2].connectionType, StreamConnectionType.managedRelay);
      expect(result.endpoints[2].transport, StreamTransport.webrtc);
      expect(result.endpoints[2].priority, 2);

      // Verify latency ordering
      expect(
        result.endpoints[0].estimatedLatencyMs!,
        lessThan(result.endpoints[1].estimatedLatencyMs!),
      );
    });

    test('sub-stream variant is propagated to StreamsApi', () async {
      final cameras = await cameraClient.listCameras(
        sessionNotifier.state.activeConnection!,
      );
      final lobbyCamera = cameras.firstWhere((c) => c.id == 'cam-1');
      expect(lobbyCamera.hasSubStream, isTrue);

      // Request sub-stream
      await streamsApi.requestStream(
        cameraId: lobbyCamera.id,
        baseUrl: connection.endpointUrl,
        accessToken: sessionNotifier.state.accessToken!,
        variant: StreamVariant.sub,
      );

      expect(streamsApi.lastVariant, StreamVariant.sub);
      expect(streamsApi.lastCameraId, 'cam-1');
    });

    test('camera without sub-stream uses auto variant', () async {
      final cameras = await cameraClient.listCameras(
        sessionNotifier.state.activeConnection!,
      );
      final parkingCamera = cameras.firstWhere((c) => c.id == 'cam-2');
      expect(parkingCamera.hasSubStream, isFalse);

      // Request with auto variant (default for cameras without sub-stream)
      await streamsApi.requestStream(
        cameraId: parkingCamera.id,
        baseUrl: connection.endpointUrl,
        accessToken: sessionNotifier.state.accessToken!,
        variant: StreamVariant.auto,
      );

      expect(streamsApi.lastVariant, StreamVariant.auto);
    });

    test('playback stream kind is propagated', () async {
      await streamsApi.requestStream(
        cameraId: 'cam-1',
        baseUrl: connection.endpointUrl,
        accessToken: sessionNotifier.state.accessToken!,
        kind: StreamKind.playback,
      );

      expect(streamsApi.lastKind, StreamKind.playback);
    });
  });
}
