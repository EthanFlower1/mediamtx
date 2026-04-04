import 'package:flutter_riverpod/flutter_riverpod.dart';
import '../models/detection_frame.dart';
import 'notifications_provider.dart';

/// Streams [DetectionFrame] events for a single camera by filtering the
/// WebSocket detection stream. This avoids opening a separate SSE connection
/// per camera — all detection frames arrive over the single shared WebSocket.
///
/// The provider reactively watches [notificationsProvider] so it re-creates the
/// stream whenever the WebSocket connection state changes (e.g. after initial
/// authentication or reconnect).
final detectionStreamProvider =
    StreamProvider.family<DetectionFrame, ({String cameraId, String cameraName})>(
        (ref, params) {
  // Watch (not read) the notification state so this provider is invalidated
  // whenever the WebSocket connects or reconnects.
  final state = ref.watch(notificationsProvider);

  final notifier = ref.read(notificationsProvider.notifier);
  final ws = notifier.webSocket;

  if (ws == null || !state.wsConnected) return const Stream.empty();

  return ws.detectionFrames
      .where((frame) => frame.camera == params.cameraName);
});
