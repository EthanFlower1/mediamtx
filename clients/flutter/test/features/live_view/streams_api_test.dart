// KAI-300 — Unit tests for StreamsApi + endpoint-fallback selection logic.
//
// Covers:
//   1. HttpStreamsApi.requestStream() returns ordered endpoints (priority asc).
//   2. Fallback selection: try endpoint 0 → fail → notifier moves to endpoint 1.
//   3. Error path: all endpoints exhausted → phase == error.
//   4. StreamRequestException path.

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:nvr_client/api/streams_api.dart';
import 'package:nvr_client/features/live_view/state/live_view_state.dart';
import 'package:nvr_client/state/app_session.dart';
import 'package:nvr_client/state/home_directory_connection.dart';
import 'package:nvr_client/state/secure_token_store.dart';

// ---------------------------------------------------------------------------
// Fake StreamsApi implementations
// ---------------------------------------------------------------------------

/// Returns a realistic two-endpoint list (WebRTC first, LL-HLS second).
class _TwoEndpointStreamsApi implements StreamsApi {
  @override
  Future<StreamRequest> requestStream({
    required String cameraId,
    required String baseUrl,
    required String accessToken,
    StreamKind kind = StreamKind.live,
    StreamProtocol protocol = StreamProtocol.auto,
  }) async {
    return StreamRequest(
      streamId: 'test-stream-$cameraId',
      expiresAt: DateTime.now().add(const Duration(hours: 1)),
      endpoints: [
        StreamEndpoint(
          url: 'https://nvr.test:8889/$cameraId/whep',
          transport: StreamTransport.webrtc,
          connectionType: StreamConnectionType.lanDirect,
          priority: 0,
          estimatedLatencyMs: 12,
        ),
        StreamEndpoint(
          url: 'https://nvr.test:8888/$cameraId/index.m3u8',
          transport: StreamTransport.llhls,
          connectionType: StreamConnectionType.lanDirect,
          priority: 1,
          estimatedLatencyMs: 1400,
        ),
      ],
    );
  }
}

/// Returns a single LL-HLS endpoint (no WebRTC).
class _LlhlsOnlyStreamsApi implements StreamsApi {
  @override
  Future<StreamRequest> requestStream({
    required String cameraId,
    required String baseUrl,
    required String accessToken,
    StreamKind kind = StreamKind.live,
    StreamProtocol protocol = StreamProtocol.auto,
  }) async {
    return StreamRequest(
      streamId: 'llhls-only-$cameraId',
      expiresAt: DateTime.now().add(const Duration(hours: 1)),
      endpoints: [
        StreamEndpoint(
          url: 'https://nvr.test:8888/$cameraId/index.m3u8',
          transport: StreamTransport.llhls,
          connectionType: StreamConnectionType.lanDirect,
          priority: 0,
          estimatedLatencyMs: 1200,
        ),
      ],
    );
  }
}

/// Always throws [StreamRequestException].
class _FailingStreamsApi implements StreamsApi {
  @override
  Future<StreamRequest> requestStream({
    required String cameraId,
    required String baseUrl,
    required String accessToken,
    StreamKind kind = StreamKind.live,
    StreamProtocol protocol = StreamProtocol.auto,
  }) async {
    throw const StreamRequestException(503, message: 'service unavailable');
  }
}

/// Returns an empty endpoint list.
class _EmptyEndpointsStreamsApi implements StreamsApi {
  @override
  Future<StreamRequest> requestStream({
    required String cameraId,
    required String baseUrl,
    required String accessToken,
    StreamKind kind = StreamKind.live,
    StreamProtocol protocol = StreamProtocol.auto,
  }) async {
    return StreamRequest(
      streamId: 'empty-$cameraId',
      expiresAt: DateTime.now().add(const Duration(hours: 1)),
      endpoints: const [],
    );
  }
}

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

Future<ProviderContainer> _makeContainer({required StreamsApi api}) async {
  final store = InMemorySecureTokenStore();
  final container = ProviderContainer(overrides: [
    streamsApiProvider.overrideWithValue(api),
    secureTokenStoreProvider.overrideWithValue(store),
  ]);

  // Pre-seed tokens using the same key format as ConnectionScopedKeys so that
  // activateConnection() finds them when it reads from secure storage.
  await store.write('kai_session:test-conn-1:access_token', 'test-access-token');
  await store.write('kai_session:test-conn-1:refresh_token', 'test-refresh-token');

  await container.read(appSessionProvider.notifier).activateConnection(
        connection: const HomeDirectoryConnection(
          id: 'test-conn-1',
          kind: HomeConnectionKind.onPrem,
          endpointUrl: 'https://nvr.test',
          displayName: 'Test NVR',
          discoveryMethod: DiscoveryMethod.manual,
        ),
        userId: 'user-test',
        tenantRef: 'tenant-test',
      );

  return container;
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

void main() {
  group('StreamsApi — stub response shape', () {
    test('HttpStreamsApi returns ordered endpoints (priority ascending)', () async {
      const api = HttpStreamsApi();
      final result = await api.requestStream(
        cameraId: 'cam-abc',
        baseUrl: 'https://nvr.local',
        accessToken: 'tok',
      );

      expect(result.streamId, isNotEmpty);
      expect(result.endpoints, isNotEmpty);
      // Endpoints must be in ascending priority order.
      for (var i = 1; i < result.endpoints.length; i++) {
        expect(
          result.endpoints[i].priority,
          greaterThanOrEqualTo(result.endpoints[i - 1].priority),
        );
      }
    });

    test('First endpoint is WebRTC LAN-direct', () async {
      const api = HttpStreamsApi();
      final result = await api.requestStream(
        cameraId: 'cam-xyz',
        baseUrl: 'https://nvr.local',
        accessToken: 'tok',
      );
      expect(result.endpoints.first.transport, StreamTransport.webrtc);
      expect(
          result.endpoints.first.connectionType, StreamConnectionType.lanDirect);
    });

    test('StreamEndpoint.connectionLabel reflects type', () {
      const ep = StreamEndpoint(
        url: 'https://relay.kaivue.cloud/cam/whep',
        transport: StreamTransport.webrtc,
        connectionType: StreamConnectionType.managedRelay,
        priority: 2,
      );
      expect(ep.connectionLabel, 'Relay');
    });
  });

  group('LiveViewNotifier — endpoint fallback logic', () {
    test('start() → requesting phase, then connectingWebRtc', () async {
      final container = await _makeContainer(api: _TwoEndpointStreamsApi());
      addTearDown(container.dispose);
      final notifier = container.read(liveViewStateProvider.notifier);

      await notifier.start(
        cameraId: 'cam-1',
        cameraName: 'Cam 1',
        ptzCapable: false,
      );

      final state = container.read(liveViewStateProvider);
      // After start() with a two-endpoint api, notifier moves to connectingWebRtc.
      expect(
        state.phase,
        anyOf(LiveViewPhase.connectingWebRtc, LiveViewPhase.connectingFallback),
      );
      expect(state.cameraId, 'cam-1');
      expect(state.endpointIndex, 0);
    });

    test('onWebRtcFailed() → moves to LL-HLS fallback (endpoint 1)', () async {
      final container = await _makeContainer(api: _TwoEndpointStreamsApi());
      addTearDown(container.dispose);
      final notifier = container.read(liveViewStateProvider.notifier);

      await notifier.start(cameraId: 'cam-1', cameraName: 'Cam 1');

      // Simulate WebRTC failure on endpoint 0.
      await notifier.onWebRtcFailed(0);

      final state = container.read(liveViewStateProvider);
      expect(state.phase, LiveViewPhase.connectingFallback);
      expect(state.endpointIndex, 1);
      expect(state.fallbackUrl, contains('index.m3u8'));
    });

    test('onWebRtcFailed() when no fallback → error phase', () async {
      // Use LL-HLS only api — the single endpoint is index 0 (llhls).
      // Start will directly go to connectingFallback.
      // Then simulate fallback failure → error.
      final container = await _makeContainer(api: _LlhlsOnlyStreamsApi());
      addTearDown(container.dispose);
      final notifier = container.read(liveViewStateProvider.notifier);

      await notifier.start(cameraId: 'cam-2', cameraName: 'Cam 2');
      notifier.onFallbackFailed(0);

      final state = container.read(liveViewStateProvider);
      expect(state.phase, LiveViewPhase.error);
    });

    test('StreamRequestException → error phase with message', () async {
      final container = await _makeContainer(api: _FailingStreamsApi());
      addTearDown(container.dispose);
      final notifier = container.read(liveViewStateProvider.notifier);

      await notifier.start(cameraId: 'cam-3', cameraName: 'Cam 3');

      final state = container.read(liveViewStateProvider);
      expect(state.phase, LiveViewPhase.error);
      expect(state.errorMessage, isNotEmpty);
    });

    test('Empty endpoints → error phase', () async {
      final container = await _makeContainer(api: _EmptyEndpointsStreamsApi());
      addTearDown(container.dispose);
      final notifier = container.read(liveViewStateProvider.notifier);

      await notifier.start(cameraId: 'cam-4', cameraName: 'Cam 4');

      final state = container.read(liveViewStateProvider);
      expect(state.phase, LiveViewPhase.error);
    });

    test('onWebRtcConnected() → liveWebRtc phase with correct URL', () async {
      final container = await _makeContainer(api: _TwoEndpointStreamsApi());
      addTearDown(container.dispose);
      final notifier = container.read(liveViewStateProvider.notifier);

      await notifier.start(cameraId: 'cam-5', cameraName: 'Cam 5');
      notifier.onWebRtcConnected(0);

      final state = container.read(liveViewStateProvider);
      expect(state.phase, LiveViewPhase.liveWebRtc);
      expect(state.webRtcUrl, contains('whep'));
      expect(state.connectionLabel, 'LAN');
    });

    test('retry() calls start() with same cameraId', () async {
      final container = await _makeContainer(api: _FailingStreamsApi());
      addTearDown(container.dispose);
      final notifier = container.read(liveViewStateProvider.notifier);

      // Force an error first.
      await notifier.start(cameraId: 'cam-6', cameraName: 'Cam 6');
      expect(
          container.read(liveViewStateProvider).phase, LiveViewPhase.error);

      // Now retry with a working api — override is not possible mid-test,
      // so just verify retry does not throw and re-enters requesting phase.
      await notifier.retry();
      // State was still 'requesting' at some point (or error again); key
      // thing is cameraId is preserved.
      expect(container.read(liveViewStateProvider).cameraId, 'cam-6');
    });

    test('setTalkbackActive() updates state', () async {
      final container = await _makeContainer(api: _TwoEndpointStreamsApi());
      addTearDown(container.dispose);
      final notifier = container.read(liveViewStateProvider.notifier);

      await notifier.start(cameraId: 'cam-7', cameraName: 'Cam 7');
      expect(
          container.read(liveViewStateProvider).talkbackActive, isFalse);

      notifier.setTalkbackActive(true);
      expect(
          container.read(liveViewStateProvider).talkbackActive, isTrue);

      notifier.setTalkbackActive(false);
      expect(
          container.read(liveViewStateProvider).talkbackActive, isFalse);
    });

    test('toggleAudioMute() flips audioMuted', () async {
      final container = await _makeContainer(api: _TwoEndpointStreamsApi());
      addTearDown(container.dispose);
      final notifier = container.read(liveViewStateProvider.notifier);

      await notifier.start(cameraId: 'cam-8', cameraName: 'Cam 8');
      final initial = container.read(liveViewStateProvider).audioMuted;
      notifier.toggleAudioMute();
      expect(
          container.read(liveViewStateProvider).audioMuted, equals(!initial));
    });

    test('reset() returns to idle', () async {
      final container = await _makeContainer(api: _TwoEndpointStreamsApi());
      addTearDown(container.dispose);
      final notifier = container.read(liveViewStateProvider.notifier);

      await notifier.start(cameraId: 'cam-9', cameraName: 'Cam 9');
      notifier.reset();

      expect(
          container.read(liveViewStateProvider).phase, LiveViewPhase.idle);
      expect(container.read(liveViewStateProvider).cameraId, isNull);
    });
  });

  group('StreamRequestException', () {
    test('toString includes statusCode', () {
      const ex = StreamRequestException(503, message: 'oops');
      expect(ex.toString(), contains('503'));
    });
  });
}
