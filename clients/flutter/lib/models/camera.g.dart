// GENERATED CODE - DO NOT MODIFY BY HAND

part of 'camera.dart';

// **************************************************************************
// JsonSerializableGenerator
// **************************************************************************

_$CameraImpl _$$CameraImplFromJson(Map<String, dynamic> json) => _$CameraImpl(
      id: json['id'] as String,
      name: json['name'] as String,
      rtspUrl: json['rtsp_url'] as String? ?? '',
      onvifEndpoint: json['onvif_endpoint'] as String? ?? '',
      mediamtxPath: json['mediamtx_path'] as String? ?? '',
      status: json['status'] as String? ?? 'disconnected',
      ptzCapable: json['ptz_capable'] as bool? ?? false,
      aiEnabled: json['ai_enabled'] as bool? ?? false,
      subStreamUrl: json['sub_stream_url'] as String? ?? '',
      retentionDays: (json['retention_days'] as num?)?.toInt() ?? 30,
      motionTimeoutSeconds:
          (json['motion_timeout_seconds'] as num?)?.toInt() ?? 8,
      snapshotUri: json['snapshot_uri'] as String? ?? '',
      supportsEvents: json['supports_events'] as bool? ?? false,
      supportsAnalytics: json['supports_analytics'] as bool? ?? false,
      supportsRelay: json['supports_relay'] as bool? ?? false,
      createdAt: json['created_at'] as String?,
      updatedAt: json['updated_at'] as String?,
      storagePath: json['storage_path'] as String? ?? '',
      storageStatus: json['storage_status'] as String? ?? 'default',
    );

Map<String, dynamic> _$$CameraImplToJson(_$CameraImpl instance) =>
    <String, dynamic>{
      'id': instance.id,
      'name': instance.name,
      'rtsp_url': instance.rtspUrl,
      'onvif_endpoint': instance.onvifEndpoint,
      'mediamtx_path': instance.mediamtxPath,
      'status': instance.status,
      'ptz_capable': instance.ptzCapable,
      'ai_enabled': instance.aiEnabled,
      'sub_stream_url': instance.subStreamUrl,
      'retention_days': instance.retentionDays,
      'motion_timeout_seconds': instance.motionTimeoutSeconds,
      'snapshot_uri': instance.snapshotUri,
      'supports_events': instance.supportsEvents,
      'supports_analytics': instance.supportsAnalytics,
      'supports_relay': instance.supportsRelay,
      'created_at': instance.createdAt,
      'updated_at': instance.updatedAt,
      'storage_path': instance.storagePath,
      'storage_status': instance.storageStatus,
    };
