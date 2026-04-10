import 'package:flutter_riverpod/flutter_riverpod.dart';

/// Proto-first interface for offline data caching.
///
/// Implementations persist camera trees and snapshot thumbnails so the UI can
/// render meaningful content while the device is offline.
abstract class OfflineCacheService {
  /// Cache the full camera tree payload.
  void cacheCameraTree(List<Map<String, dynamic>> cameras);

  /// Retrieve the previously cached camera tree, or `null` if nothing is cached.
  List<Map<String, dynamic>>? getCachedCameraTree();

  /// Cache a JPEG snapshot for a specific camera.
  void cacheSnapshot(String cameraId, List<int> jpeg);

  /// Retrieve the cached JPEG snapshot for [cameraId], or `null`.
  List<int>? getCachedSnapshot(String cameraId);

  /// The time the most recent cache write occurred, or `null` if the cache is
  /// empty.
  DateTime? lastCacheTime();
}

/// In-memory implementation of [OfflineCacheService].
///
/// Suitable for development and testing. A persistent implementation backed by
/// SQLite or Hive can be swapped in via the Riverpod provider override.
class InMemoryOfflineCacheService implements OfflineCacheService {
  List<Map<String, dynamic>>? _cameraTree;
  final Map<String, List<int>> _snapshots = {};
  DateTime? _lastCacheTime;

  @override
  void cacheCameraTree(List<Map<String, dynamic>> cameras) {
    _cameraTree = List.unmodifiable(cameras);
    _lastCacheTime = DateTime.now();
  }

  @override
  List<Map<String, dynamic>>? getCachedCameraTree() => _cameraTree;

  @override
  void cacheSnapshot(String cameraId, List<int> jpeg) {
    _snapshots[cameraId] = List.unmodifiable(jpeg);
    _lastCacheTime = DateTime.now();
  }

  @override
  List<int>? getCachedSnapshot(String cameraId) => _snapshots[cameraId];

  @override
  DateTime? lastCacheTime() => _lastCacheTime;
}

/// Riverpod provider for [OfflineCacheService].
///
/// Override this in tests or when swapping to a persistent implementation.
final offlineCacheServiceProvider = Provider<OfflineCacheService>(
  (ref) => InMemoryOfflineCacheService(),
);
