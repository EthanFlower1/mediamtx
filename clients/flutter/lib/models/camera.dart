import 'package:freezed_annotation/freezed_annotation.dart';

part 'camera.freezed.dart';
part 'camera.g.dart';

@freezed
class Camera with _$Camera {
  const factory Camera({
    required String id,
    required String name,
    @JsonKey(name: 'rtsp_url') @Default('') String rtspUrl,
    @JsonKey(name: 'onvif_endpoint') @Default('') String onvifEndpoint,
    @JsonKey(name: 'mediamtx_path') @Default('') String mediamtxPath,
    @Default('disconnected') String status,
    @JsonKey(name: 'ptz_capable') @Default(false) bool ptzCapable,
    @JsonKey(name: 'ai_enabled') @Default(false) bool aiEnabled,
    @JsonKey(name: 'sub_stream_url') @Default('') String subStreamUrl,
    @JsonKey(name: 'retention_days') @Default(30) int retentionDays,
    @JsonKey(name: 'motion_timeout_seconds') @Default(8) int motionTimeoutSeconds,
    @JsonKey(name: 'snapshot_uri') @Default('') String snapshotUri,
    @JsonKey(name: 'supports_events') @Default(false) bool supportsEvents,
    @JsonKey(name: 'supports_analytics') @Default(false) bool supportsAnalytics,
    @JsonKey(name: 'supports_relay') @Default(false) bool supportsRelay,
    @JsonKey(name: 'created_at') String? createdAt,
    @JsonKey(name: 'updated_at') String? updatedAt,
    @JsonKey(name: 'storage_path') @Default('') String storagePath,
    @JsonKey(name: 'storage_status') @Default('default') String storageStatus,
  }) = _Camera;

  factory Camera.fromJson(Map<String, dynamic> json) => _$CameraFromJson(json);
}
