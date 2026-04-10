// KAI-156 -- Riverpod provider for TalkbackService.
//
// Default binding: [FakeTalkbackService]. Override in tests or swap for a real
// WebRTC implementation when KAI-156-webrtc lands.

import 'package:flutter_riverpod/flutter_riverpod.dart';

import 'talkback_service.dart';

/// Provider for the active [TalkbackService] instance.
///
/// Override in tests:
/// ```dart
/// final container = ProviderContainer(overrides: [
///   talkbackServiceProvider.overrideWithValue(myFakeService),
/// ]);
/// ```
final talkbackServiceProvider = Provider<TalkbackService>((ref) {
  final service = FakeTalkbackService();
  ref.onDispose(service.dispose);
  return service;
});

/// Convenience stream provider that exposes [TalkbackState] reactively.
///
/// Widgets can watch this to rebuild on talkback state changes:
/// ```dart
/// final stateAsync = ref.watch(talkbackStateStreamProvider);
/// ```
final talkbackStateStreamProvider = StreamProvider<TalkbackState>((ref) {
  final service = ref.watch(talkbackServiceProvider);
  return service.stateStream;
});
