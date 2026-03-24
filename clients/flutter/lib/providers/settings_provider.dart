import 'package:flutter_riverpod/flutter_riverpod.dart';
import '../models/user.dart';
import 'auth_provider.dart';

class SystemInfo {
  final String version;
  final String platform;
  final int uptime;
  final bool clipSearchAvailable;

  const SystemInfo({
    required this.version,
    required this.platform,
    required this.uptime,
    required this.clipSearchAvailable,
  });

  factory SystemInfo.fromJson(Map<String, dynamic> json) {
    return SystemInfo(
      version: json['version'] as String? ?? '',
      platform: json['platform'] as String? ?? '',
      uptime: json['uptime'] as int? ?? 0,
      clipSearchAvailable: json['clip_search_available'] as bool? ?? false,
    );
  }
}

class CameraStorage {
  final String cameraId;
  final String cameraName;
  final int totalBytes;
  final int segmentCount;

  const CameraStorage({
    required this.cameraId,
    required this.cameraName,
    required this.totalBytes,
    required this.segmentCount,
  });

  factory CameraStorage.fromJson(Map<String, dynamic> json) {
    return CameraStorage(
      cameraId: json['camera_id'] as String? ?? '',
      cameraName: json['camera_name'] as String? ?? '',
      totalBytes: json['total_bytes'] as int? ?? 0,
      segmentCount: json['segment_count'] as int? ?? 0,
    );
  }
}

class StorageInfo {
  final int totalBytes;
  final int usedBytes;
  final int freeBytes;
  final int recordingsBytes;
  final bool warning;
  final bool critical;
  final List<CameraStorage> perCamera;

  const StorageInfo({
    required this.totalBytes,
    required this.usedBytes,
    required this.freeBytes,
    required this.recordingsBytes,
    required this.warning,
    required this.critical,
    required this.perCamera,
  });

  double get usagePercent {
    if (totalBytes == 0) return 0.0;
    return usedBytes / totalBytes * 100.0;
  }

  factory StorageInfo.fromJson(Map<String, dynamic> json) {
    final rawPerCamera = json['per_camera'] as List<dynamic>? ?? [];
    final perCamera = rawPerCamera
        .map((e) => CameraStorage.fromJson(e as Map<String, dynamic>))
        .toList();

    return StorageInfo(
      totalBytes: json['total_bytes'] as int? ?? 0,
      usedBytes: json['used_bytes'] as int? ?? 0,
      freeBytes: json['free_bytes'] as int? ?? 0,
      recordingsBytes: json['recordings_bytes'] as int? ?? 0,
      warning: json['warning'] as bool? ?? false,
      critical: json['critical'] as bool? ?? false,
      perCamera: perCamera,
    );
  }
}

class AuditEntry {
  final String id;
  final String username;
  final String action;
  final String resourceType;
  final String? resourceId;
  final String? details;
  final String? ipAddress;
  final String createdAt;

  const AuditEntry({
    required this.id,
    required this.username,
    required this.action,
    required this.resourceType,
    this.resourceId,
    this.details,
    this.ipAddress,
    required this.createdAt,
  });

  factory AuditEntry.fromJson(Map<String, dynamic> json) {
    return AuditEntry(
      id: json['id'] as String,
      username: json['username'] as String? ?? '',
      action: json['action'] as String? ?? '',
      resourceType: json['resource_type'] as String? ?? '',
      resourceId: json['resource_id'] as String?,
      details: json['details'] as String?,
      ipAddress: json['ip_address'] as String?,
      createdAt: json['created_at'] as String? ?? '',
    );
  }
}

final systemInfoProvider = FutureProvider<SystemInfo>((ref) async {
  final api = ref.watch(apiClientProvider);
  if (api == null) return const SystemInfo(version: '', platform: '', uptime: 0, clipSearchAvailable: false);
  final res = await api.get('/system/info');
  return SystemInfo.fromJson(res.data as Map<String, dynamic>);
});

final storageInfoProvider = FutureProvider<StorageInfo>((ref) async {
  final api = ref.watch(apiClientProvider);
  if (api == null) {
    return const StorageInfo(
      totalBytes: 0,
      usedBytes: 0,
      freeBytes: 0,
      recordingsBytes: 0,
      warning: false,
      critical: false,
      perCamera: [],
    );
  }
  final res = await api.get('/system/storage');
  return StorageInfo.fromJson(res.data as Map<String, dynamic>);
});

final usersProvider = FutureProvider<List<User>>((ref) async {
  final api = ref.watch(apiClientProvider);
  if (api == null) return [];
  final res = await api.get('/users');
  return (res.data as List).map((e) => User.fromJson(e as Map<String, dynamic>)).toList();
});

final auditProvider = FutureProvider<List<AuditEntry>>((ref) async {
  final api = ref.watch(apiClientProvider);
  if (api == null) return [];
  try {
    final res = await api.get('/audit', queryParameters: {'limit': 100});
    return (res.data as List).map((e) => AuditEntry.fromJson(e as Map<String, dynamic>)).toList();
  } catch (_) {
    return [];
  }
});
