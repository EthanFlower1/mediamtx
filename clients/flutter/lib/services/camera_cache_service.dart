import 'dart:convert';
import 'package:shared_preferences/shared_preferences.dart';
import '../models/camera.dart';

/// Caches camera list data in shared_preferences for offline access.
class CameraCacheService {
  static const _cameraListKey = 'cached_camera_list';
  static const _cameraListTimestampKey = 'cached_camera_list_timestamp';

  /// Save the camera list to local storage.
  Future<void> cacheCameras(List<Camera> cameras) async {
    final prefs = await SharedPreferences.getInstance();
    final jsonList = cameras.map((c) => jsonEncode(c.toJson())).toList();
    await prefs.setStringList(_cameraListKey, jsonList);
    await prefs.setString(
      _cameraListTimestampKey,
      DateTime.now().toIso8601String(),
    );
  }

  /// Load the cached camera list. Returns empty list if nothing cached.
  Future<List<Camera>> loadCachedCameras() async {
    final prefs = await SharedPreferences.getInstance();
    final jsonList = prefs.getStringList(_cameraListKey);
    if (jsonList == null || jsonList.isEmpty) return [];
    return jsonList
        .map((s) => Camera.fromJson(jsonDecode(s) as Map<String, dynamic>))
        .toList();
  }

  /// Returns when the cache was last updated, or null if never cached.
  Future<DateTime?> lastCachedAt() async {
    final prefs = await SharedPreferences.getInstance();
    final ts = prefs.getString(_cameraListTimestampKey);
    if (ts == null) return null;
    return DateTime.tryParse(ts);
  }

  /// Clear the cache.
  Future<void> clearCache() async {
    final prefs = await SharedPreferences.getInstance();
    await prefs.remove(_cameraListKey);
    await prefs.remove(_cameraListTimestampKey);
  }
}
