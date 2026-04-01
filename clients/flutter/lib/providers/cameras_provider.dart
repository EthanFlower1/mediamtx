import 'package:flutter_riverpod/flutter_riverpod.dart';
import '../models/camera.dart';
import 'auth_provider.dart';
import 'notifications_provider.dart';

final camerasProvider = FutureProvider<List<Camera>>((ref) async {
  final api = ref.watch(apiClientProvider);
  if (api == null) return [];

  // Invalidate when a camera_online or camera_offline WebSocket event arrives.
  ref.listen<NotificationState>(notificationsProvider, (previous, next) {
    if (next.history.isEmpty) return;
    final latest = next.history.first;
    if (latest.type == 'camera_online' || latest.type == 'camera_offline') {
      ref.invalidateSelf();
    }
  });

  final res = await api.get('/cameras');
  return (res.data as List)
      .map((e) => Camera.fromJson(e as Map<String, dynamic>))
      .toList();
});
