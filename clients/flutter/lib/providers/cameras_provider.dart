import 'package:flutter_riverpod/flutter_riverpod.dart';
import '../models/camera.dart';
import '../services/camera_cache_service.dart';
import 'auth_provider.dart';
import 'connectivity_provider.dart';
import 'notifications_provider.dart';

final cameraCacheServiceProvider =
    Provider<CameraCacheService>((_) => CameraCacheService());

final camerasProvider = FutureProvider<List<Camera>>((ref) async {
  final api = ref.watch(apiClientProvider);
  final cacheService = ref.read(cameraCacheServiceProvider);

  // Invalidate when a camera_online or camera_offline WebSocket event arrives.
  ref.listen<NotificationState>(notificationsProvider, (previous, next) {
    if (next.history.isEmpty) return;
    final latest = next.history.first;
    if (latest.type == 'camera_online' || latest.type == 'camera_offline') {
      ref.invalidateSelf();
    }
  });

  // Also invalidate when connectivity transitions from offline to online.
  ref.listen<ConnectivityState>(connectivityProvider, (previous, next) {
    if (previous != null && previous.isOffline && next.isOnline) {
      ref.invalidateSelf();
    }
  });

  if (api == null) {
    return cacheService.loadCachedCameras();
  }

  // Fetch immediately — don't wait for connectivity check.
  try {
    final res = await api.get('/cameras');
    final cameras = (res.data as List)
        .map((e) => Camera.fromJson(e as Map<String, dynamic>))
        .toList();
    cacheService.cacheCameras(cameras);
    return cameras;
  } catch (_) {
    // Network error — fall back to cache
    return cacheService.loadCachedCameras();
  }
});
