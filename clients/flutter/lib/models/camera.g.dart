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
      aiStreamId: json['ai_stream_id'] as String? ?? '',
      aiConfidence: (json['ai_confidence'] as num?)?.toDouble() ?? 0.5,
      aiTrackTimeout: (json['ai_track_timeout'] as num?)?.toInt() ?? 5,
      subStreamUrl: json['sub_stream_url'] as String? ?? '',
      hasSubStream: json['has_sub_stream'] as bool? ?? false,
      hasMainStream: json['has_main_stream'] as bool? ?? true,
      retentionDays: (json['retention_days'] as num?)?.toInt() ?? 30,
      eventRetentionDays: (json['event_retention_days'] as num?)?.toInt() ?? 0,
      detectionRetentionDays:
          (json['detection_retention_days'] as num?)?.toInt() ?? 0,
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
      liveViewPath: json['live_view_path'] as String? ?? '',
      liveViewCodec: json['live_view_codec'] as String? ?? '',
      streamPaths: (json['stream_paths'] as List<dynamic>?)
              ?.map((e) => StreamPath.fromJson(e as Map<String, dynamic>))
              .toList() ??
          const [],
      recorderId: json['recorder_id'] as String?,
      recorderEndpoint: json['recorder_endpoint'] as String?,
      directoryId: json['directory_id'] as String?,
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
      'ai_stream_id': instance.aiStreamId,
      'ai_confidence': instance.aiConfidence,
      'ai_track_timeout': instance.aiTrackTimeout,
      'sub_stream_url': instance.subStreamUrl,
      'has_sub_stream': instance.hasSubStream,
      'has_main_stream': instance.hasMainStream,
      'retention_days': instance.retentionDays,
      'event_retention_days': instance.eventRetentionDays,
      'detection_retention_days': instance.detectionRetentionDays,
      'motion_timeout_seconds': instance.motionTimeoutSeconds,
      'snapshot_uri': instance.snapshotUri,
      'supports_events': instance.supportsEvents,
      'supports_analytics': instance.supportsAnalytics,
      'supports_relay': instance.supportsRelay,
      'created_at': instance.createdAt,
      'updated_at': instance.updatedAt,
      'storage_path': instance.storagePath,
      'storage_status': instance.storageStatus,
      'live_view_path': instance.liveViewPath,
      'live_view_codec': instance.liveViewCodec,
      'stream_paths': instance.streamPaths,
      'recorder_id': instance.recorderId,
      'recorder_endpoint': instance.recorderEndpoint,
      'directory_id': instance.directoryId,
    };

_$StreamPathImpl _$$StreamPathImplFromJson(Map<String, dynamic> json) =>
    _$StreamPathImpl(
      name: json['name'] as String? ?? '',
      path: json['path'] as String? ?? '',
      resolution: json['resolution'] as String? ?? '',
      videoCodec: json['video_codec'] as String? ?? '',
    );

Map<String, dynamic> _$$StreamPathImplToJson(_$StreamPathImpl instance) =>
    <String, dynamic>{
      'name': instance.name,
      'path': instance.path,
      'resolution': instance.resolution,
      'video_codec': instance.videoCodec,
    };
