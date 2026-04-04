import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'auth_provider.dart';

/// Per-camera recording statistics from /recordings/stats.
class CameraStats {
  final String cameraId;
  final String cameraName;
  final int totalBytes;
  final int segmentCount;
  final int totalRecordedMs;
  final int currentUptimeMs;
  final String? lastGapEnd;
  final String oldestRecording;
  final String newestRecording;
  final int gapCount;

  const CameraStats({
    required this.cameraId,
    required this.cameraName,
    required this.totalBytes,
    required this.segmentCount,
    required this.totalRecordedMs,
    required this.currentUptimeMs,
    this.lastGapEnd,
    required this.oldestRecording,
    required this.newestRecording,
    required this.gapCount,
  });

  factory CameraStats.fromJson(Map<String, dynamic> json) {
    return CameraStats(
      cameraId: json['camera_id'] as String? ?? '',
      cameraName: json['camera_name'] as String? ?? '',
      totalBytes: (json['total_bytes'] as num?)?.toInt() ?? 0,
      segmentCount: (json['segment_count'] as num?)?.toInt() ?? 0,
      totalRecordedMs: (json['total_recorded_ms'] as num?)?.toInt() ?? 0,
      currentUptimeMs: (json['current_uptime_ms'] as num?)?.toInt() ?? 0,
      lastGapEnd: json['last_gap_end'] as String?,
      oldestRecording: json['oldest_recording'] as String? ?? '',
      newestRecording: json['newest_recording'] as String? ?? '',
      gapCount: (json['gap_count'] as num?)?.toInt() ?? 0,
    );
  }

  /// Returns storage usage formatted as a human-readable string.
  String get formattedStorage {
    if (totalBytes <= 0) return '0 B';
    const units = ['B', 'KB', 'MB', 'GB', 'TB'];
    double size = totalBytes.toDouble();
    int unitIndex = 0;
    while (size >= 1024 && unitIndex < units.length - 1) {
      size /= 1024;
      unitIndex++;
    }
    return '${size.toStringAsFixed(size < 10 ? 1 : 0)} ${units[unitIndex]}';
  }

  /// Whether this camera has recording data.
  bool get hasRecordings => newestRecording.isNotEmpty && segmentCount > 0;
}

/// Fetches recording stats for all cameras, keyed by camera ID.
final dashboardStatsProvider = FutureProvider<Map<String, CameraStats>>((ref) async {
  final api = ref.watch(apiClientProvider);
  if (api == null) return {};

  final res = await api.get('/recordings/stats');
  final cameras = res.data['cameras'] as List? ?? [];
  final map = <String, CameraStats>{};
  for (final item in cameras) {
    final stats = CameraStats.fromJson(item as Map<String, dynamic>);
    map[stats.cameraId] = stats;
  }
  return map;
});
