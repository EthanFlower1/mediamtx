import 'package:flutter_riverpod/flutter_riverpod.dart';
import '../models/detection_frame.dart';
import 'notifications_provider.dart';

/// Streams [DetectionFrame] events filtered to a single camera.
///
/// Usage:
///   ref.watch(detectionStreamProvider('my-camera'))
final detectionStreamProvider =
    StreamProvider.family<DetectionFrame, String>((ref, cameraName) {
  final notifier =
      ref.watch(notificationsProvider.notifier);
  final ws = notifier.webSocket;

  if (ws == null) {
    return const Stream.empty();
  }

  return ws.detectionFrames
      .where((frame) => frame.camera == cameraName);
});
