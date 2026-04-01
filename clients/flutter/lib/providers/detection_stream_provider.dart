import 'package:flutter_riverpod/flutter_riverpod.dart';
import '../models/detection_frame.dart';
import 'notifications_provider.dart';

/// Streams [DetectionFrame] events for a single camera by filtering the
/// WebSocket detection stream. This avoids opening a separate SSE connection
/// per camera — all detection frames arrive over the single shared WebSocket.
final detectionStreamProvider =
    StreamProvider.family<DetectionFrame, ({String cameraId, String cameraName})>(
        (ref, params) {
  final notifier = ref.read(notificationsProvider.notifier);
  final ws = notifier.webSocket;

  if (ws == null) return const Stream.empty();

  return ws.detectionFrames
      .where((frame) => frame.camera == params.cameraName);
});
