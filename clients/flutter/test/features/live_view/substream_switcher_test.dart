// KAI-300 — Unit tests for sub-stream switcher additions to LiveViewState
// and LiveViewNotifier.

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';

import 'package:mediamtx/api/streams_api.dart';
import 'package:mediamtx/features/live_view/state/live_view_state.dart';

// ---------------------------------------------------------------------------
// Fake StreamsApi that records the variant parameter.
// ---------------------------------------------------------------------------

class FakeStreamsApi implements StreamsApi {
  StreamVariant? lastVariant;
  int callCount = 0;

  @override
  Future<StreamRequest> requestStream({
    required String cameraId,
    required String baseUrl,
    required String accessToken,
    StreamKind kind = StreamKind.live,
    StreamProtocol protocol = StreamProtocol.auto,
    StreamVariant variant = StreamVariant.auto,
  }) async {
    lastVariant = variant;
    callCount++;
    return StreamRequest(
      streamId: 'test-stream',
      expiresAt: DateTime.now().toUtc().add(const Duration(hours: 1)),
      endpoints: [
        const StreamEndpoint(
          url: 'http://test:8889/cam/whep',
          transport: StreamTransport.webrtc,
          connectionType: StreamConnectionType.lanDirect,
          priority: 0,
          estimatedLatencyMs: 10,
        ),
      ],
    );
  }
}

void main() {
  group('LiveViewState sub-stream fields', () {
    test('defaults to StreamVariant.auto and hasSubStream=false', () {
      const state = LiveViewState();
      expect(state.streamVariant, StreamVariant.auto);
      expect(state.hasSubStream, false);
    });

    test('copyWith preserves streamVariant and hasSubStream', () {
      const state = LiveViewState(
        streamVariant: StreamVariant.sub,
        hasSubStream: true,
      );
      final copy = state.copyWith(phase: LiveViewPhase.requesting);
      expect(copy.streamVariant, StreamVariant.sub);
      expect(copy.hasSubStream, true);
      expect(copy.phase, LiveViewPhase.requesting);
    });

    test('copyWith can override streamVariant', () {
      const state = LiveViewState(streamVariant: StreamVariant.sub);
      final copy = state.copyWith(streamVariant: StreamVariant.main);
      expect(copy.streamVariant, StreamVariant.main);
    });
  });

  group('LiveViewNotifier.toggleStreamVariant', () {
    late FakeStreamsApi fakeApi;
    late ProviderContainer container;

    setUp(() {
      fakeApi = FakeStreamsApi();
      container = ProviderContainer(
        overrides: [
          streamsApiProvider.overrideWithValue(fakeApi),
        ],
      );
    });

    tearDown(() => container.dispose());

    test('no-op when hasSubStream is false', () async {
      final notifier = container.read(liveViewStateProvider.notifier);
      // Start without sub-stream support.
      await notifier.start(
        cameraId: 'cam1',
        cameraName: 'Camera 1',
        hasSubStream: false,
      );
      final initialCalls = fakeApi.callCount;

      await notifier.toggleStreamVariant();
      // Should not have made another API call.
      expect(fakeApi.callCount, initialCalls);
    });

    test('toggles from auto to sub', () async {
      final notifier = container.read(liveViewStateProvider.notifier);
      await notifier.start(
        cameraId: 'cam1',
        cameraName: 'Camera 1',
        hasSubStream: true,
        streamVariant: StreamVariant.auto,
      );

      await notifier.toggleStreamVariant();
      expect(fakeApi.lastVariant, StreamVariant.sub);
    });

    test('toggles from sub to main', () async {
      final notifier = container.read(liveViewStateProvider.notifier);
      await notifier.start(
        cameraId: 'cam1',
        cameraName: 'Camera 1',
        hasSubStream: true,
        streamVariant: StreamVariant.sub,
      );

      await notifier.toggleStreamVariant();
      expect(fakeApi.lastVariant, StreamVariant.main);
    });

    test('toggles from main to sub', () async {
      final notifier = container.read(liveViewStateProvider.notifier);
      await notifier.start(
        cameraId: 'cam1',
        cameraName: 'Camera 1',
        hasSubStream: true,
        streamVariant: StreamVariant.main,
      );

      await notifier.toggleStreamVariant();
      expect(fakeApi.lastVariant, StreamVariant.sub);
    });

    test('retry preserves stream variant', () async {
      final notifier = container.read(liveViewStateProvider.notifier);
      await notifier.start(
        cameraId: 'cam1',
        cameraName: 'Camera 1',
        hasSubStream: true,
        streamVariant: StreamVariant.sub,
      );

      await notifier.retry();
      expect(fakeApi.lastVariant, StreamVariant.sub);
    });
  });

  group('StreamVariant enum', () {
    test('has three values', () {
      expect(StreamVariant.values.length, 3);
      expect(StreamVariant.values, contains(StreamVariant.auto));
      expect(StreamVariant.values, contains(StreamVariant.main));
      expect(StreamVariant.values, contains(StreamVariant.sub));
    });
  });
}
