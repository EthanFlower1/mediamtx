// KAI-156 -- Talkback service: abstract interface + fake implementation.
//
// Proto-first design: the abstract [TalkbackService] defines the contract for
// audio talkback over WebRTC. [FakeTalkbackService] records calls and simulates
// state transitions for testing and UI development before the real WebRTC
// implementation lands.
//
// State machine:
//   idle -> acquiringMic -> connecting -> active -> idle (on stop)
//                                                -> error (on failure)

import 'dart:async';

// ---------------------------------------------------------------------------
// TalkbackState
// ---------------------------------------------------------------------------

/// The phases a talkback session moves through.
enum TalkbackState {
  /// No talkback session in progress.
  idle,

  /// Requesting microphone permissions / acquiring the audio track.
  acquiringMic,

  /// Establishing the WebRTC peer connection to the camera endpoint.
  connecting,

  /// Audio is being sent to the camera.
  active,

  /// An error occurred (permission denied, network failure, etc.).
  error,
}

// ---------------------------------------------------------------------------
// Abstract service
// ---------------------------------------------------------------------------

/// Contract for talkback audio services.
///
/// Implementations manage mic acquisition, WebRTC peer lifecycle, and state
/// reporting. Consumers observe [stateStream] and call [startTalkback] /
/// [stopTalkback] in response to hold-to-talk gestures.
abstract class TalkbackService {
  /// Stream of [TalkbackState] changes. Emits the current state immediately
  /// upon listen, then on every transition.
  Stream<TalkbackState> get stateStream;

  /// The most recent state (convenience getter for snapshot reads).
  TalkbackState get currentState;

  /// Begin a talkback session against [endpointUrl] using [accessToken] for
  /// authentication. The service transitions through acquiringMic -> connecting
  /// -> active (or -> error on failure).
  Future<void> startTalkback({
    required String endpointUrl,
    required String accessToken,
  });

  /// Tear down the talkback session and release resources.
  /// Transitions back to [TalkbackState.idle].
  Future<void> stopTalkback();

  /// Release all resources. After dispose, the service must not be used.
  void dispose();
}

// ---------------------------------------------------------------------------
// Fake implementation (for tests + UI prototyping)
// ---------------------------------------------------------------------------

/// A recorded invocation of [FakeTalkbackService].
class TalkbackCall {
  final String method;
  final Map<String, String> args;
  final DateTime timestamp;

  TalkbackCall({
    required this.method,
    this.args = const {},
    DateTime? timestamp,
  }) : timestamp = timestamp ?? DateTime.now();

  @override
  String toString() => 'TalkbackCall($method, $args)';
}

/// Fake talkback service that records calls and simulates state transitions
/// with configurable delays. Use in widget tests and during UI development.
class FakeTalkbackService implements TalkbackService {
  FakeTalkbackService({
    this.acquireDelay = const Duration(milliseconds: 50),
    this.connectDelay = const Duration(milliseconds: 100),
    this.shouldFail = false,
    this.failAt = TalkbackState.connecting,
    this.errorMessage = 'Simulated talkback error',
  });

  /// Delay before transitioning from acquiringMic to connecting.
  final Duration acquireDelay;

  /// Delay before transitioning from connecting to active.
  final Duration connectDelay;

  /// If true, the service will transition to [error] at [failAt] stage.
  final bool shouldFail;

  /// The state at which to inject a failure (only used when [shouldFail] is
  /// true).
  final TalkbackState failAt;

  /// Error message to use when simulating failure.
  final String errorMessage;

  /// All recorded calls, in order.
  final List<TalkbackCall> calls = [];

  final StreamController<TalkbackState> _controller =
      StreamController<TalkbackState>.broadcast();

  TalkbackState _state = TalkbackState.idle;

  @override
  TalkbackState get currentState => _state;

  @override
  Stream<TalkbackState> get stateStream => _controller.stream;

  void _transition(TalkbackState next) {
    _state = next;
    if (!_controller.isClosed) {
      _controller.add(next);
    }
  }

  @override
  Future<void> startTalkback({
    required String endpointUrl,
    required String accessToken,
  }) async {
    calls.add(TalkbackCall(
      method: 'startTalkback',
      args: {'endpointUrl': endpointUrl, 'accessToken': accessToken},
    ));

    // Acquiring microphone
    _transition(TalkbackState.acquiringMic);

    if (shouldFail && failAt == TalkbackState.acquiringMic) {
      await Future<void>.delayed(acquireDelay);
      _transition(TalkbackState.error);
      return;
    }

    await Future<void>.delayed(acquireDelay);

    // Connecting
    _transition(TalkbackState.connecting);

    if (shouldFail && failAt == TalkbackState.connecting) {
      await Future<void>.delayed(connectDelay);
      _transition(TalkbackState.error);
      return;
    }

    await Future<void>.delayed(connectDelay);

    // Active
    _transition(TalkbackState.active);
  }

  @override
  Future<void> stopTalkback() async {
    calls.add(TalkbackCall(method: 'stopTalkback'));
    _transition(TalkbackState.idle);
  }

  @override
  void dispose() {
    calls.add(TalkbackCall(method: 'dispose'));
    _controller.close();
  }
}
