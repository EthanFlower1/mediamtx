// GENERATED — placeholder until buf generate runs. See README.md.
// Source: kaivue/v1/cameras.proto

import 'auth.pb.dart';

/// CameraState tracks lifecycle of a camera record.
enum PbCameraState {
  unspecified,
  provisioning,
  online,
  offline,
  disabled,
  error,
}

/// RecordingMode controls when the recorder writes segments.
enum PbRecordingMode {
  unspecified,
  continuous,
  motion,
  schedule,
  event,
  off,
}

/// RetentionPolicy bounds how long recorded media is kept.
class PbRetentionPolicy {
  final int retentionDays;
  final int maxBytes;
  final int eventRetentionDays;

  const PbRetentionPolicy({
    this.retentionDays = 0,
    this.maxBytes = 0,
    this.eventRetentionDays = 0,
  });

  factory PbRetentionPolicy.fromJson(Map<String, dynamic> json) =>
      PbRetentionPolicy(
        retentionDays: json['retention_days'] as int? ?? 0,
        maxBytes: json['max_bytes'] as int? ?? 0,
        eventRetentionDays: json['event_retention_days'] as int? ?? 0,
      );

  Map<String, dynamic> toJson() => {
        'retention_days': retentionDays,
        'max_bytes': maxBytes,
        'event_retention_days': eventRetentionDays,
      };
}

/// StreamProfile describes a single RTSP sub-stream on the camera.
class PbStreamProfile {
  final String name;
  final String codec;
  final int width;
  final int height;
  final int bitrateKbps;
  final int framerate;
  final String url;

  const PbStreamProfile({
    this.name = '',
    this.codec = '',
    this.width = 0,
    this.height = 0,
    this.bitrateKbps = 0,
    this.framerate = 0,
    this.url = '',
  });

  factory PbStreamProfile.fromJson(Map<String, dynamic> json) =>
      PbStreamProfile(
        name: json['name'] as String? ?? '',
        codec: json['codec'] as String? ?? '',
        width: json['width'] as int? ?? 0,
        height: json['height'] as int? ?? 0,
        bitrateKbps: json['bitrate_kbps'] as int? ?? 0,
        framerate: json['framerate'] as int? ?? 0,
        url: json['url'] as String? ?? '',
      );

  Map<String, dynamic> toJson() => {
        'name': name,
        'codec': codec,
        'width': width,
        'height': height,
        'bitrate_kbps': bitrateKbps,
        'framerate': framerate,
        'url': url,
      };
}

/// CameraConfig is the declarative configuration of a camera.
class PbCameraConfig {
  final PbRecordingMode defaultMode;
  final PbRetentionPolicy? retention;
  final List<PbStreamProfile> profiles;
  final String recordProfile;
  final String liveProfile;
  final int motionSensitivity;
  final bool audioEnabled;
  final bool talkbackEnabled;

  const PbCameraConfig({
    this.defaultMode = PbRecordingMode.unspecified,
    this.retention,
    this.profiles = const [],
    this.recordProfile = '',
    this.liveProfile = '',
    this.motionSensitivity = 0,
    this.audioEnabled = false,
    this.talkbackEnabled = false,
  });

  factory PbCameraConfig.fromJson(Map<String, dynamic> json) => PbCameraConfig(
        defaultMode:
            PbRecordingMode.values[json['default_mode'] as int? ?? 0],
        retention: json['retention'] != null
            ? PbRetentionPolicy.fromJson(
                json['retention'] as Map<String, dynamic>)
            : null,
        profiles: (json['profiles'] as List<dynamic>?)
                ?.map((e) =>
                    PbStreamProfile.fromJson(e as Map<String, dynamic>))
                .toList() ??
            const [],
        recordProfile: json['record_profile'] as String? ?? '',
        liveProfile: json['live_profile'] as String? ?? '',
        motionSensitivity: json['motion_sensitivity'] as int? ?? 0,
        audioEnabled: json['audio_enabled'] as bool? ?? false,
        talkbackEnabled: json['talkback_enabled'] as bool? ?? false,
      );

  Map<String, dynamic> toJson() => {
        'default_mode': defaultMode.index,
        if (retention != null) 'retention': retention!.toJson(),
        'profiles': profiles.map((p) => p.toJson()).toList(),
        'record_profile': recordProfile,
        'live_profile': liveProfile,
        'motion_sensitivity': motionSensitivity,
        'audio_enabled': audioEnabled,
        'talkback_enabled': talkbackEnabled,
      };
}

/// Camera is the authoritative record for a camera on the Directory side.
class PbCamera {
  final String id;
  final PbTenantRef? tenant;
  final String recorderId;
  final String name;
  final String description;
  final String manufacturer;
  final String model;
  final String firmwareVersion;
  final String macAddress;
  final String ipAddress;
  final String credentialRef;
  final PbCameraConfig? config;
  final int configVersion;
  final PbCameraState state;
  final DateTime? stateReportedAt;
  final List<String> labels;
  final DateTime? createdAt;
  final DateTime? updatedAt;

  const PbCamera({
    this.id = '',
    this.tenant,
    this.recorderId = '',
    this.name = '',
    this.description = '',
    this.manufacturer = '',
    this.model = '',
    this.firmwareVersion = '',
    this.macAddress = '',
    this.ipAddress = '',
    this.credentialRef = '',
    this.config,
    this.configVersion = 0,
    this.state = PbCameraState.unspecified,
    this.stateReportedAt,
    this.labels = const [],
    this.createdAt,
    this.updatedAt,
  });

  factory PbCamera.fromJson(Map<String, dynamic> json) => PbCamera(
        id: json['id'] as String? ?? '',
        tenant: json['tenant'] != null
            ? PbTenantRef.fromJson(json['tenant'] as Map<String, dynamic>)
            : null,
        recorderId: json['recorder_id'] as String? ?? '',
        name: json['name'] as String? ?? '',
        description: json['description'] as String? ?? '',
        manufacturer: json['manufacturer'] as String? ?? '',
        model: json['model'] as String? ?? '',
        firmwareVersion: json['firmware_version'] as String? ?? '',
        macAddress: json['mac_address'] as String? ?? '',
        ipAddress: json['ip_address'] as String? ?? '',
        credentialRef: json['credential_ref'] as String? ?? '',
        config: json['config'] != null
            ? PbCameraConfig.fromJson(json['config'] as Map<String, dynamic>)
            : null,
        configVersion: json['config_version'] as int? ?? 0,
        state: PbCameraState.values[json['state'] as int? ?? 0],
        labels:
            (json['labels'] as List<dynamic>?)?.cast<String>() ?? const [],
      );

  Map<String, dynamic> toJson() => {
        'id': id,
        if (tenant != null) 'tenant': tenant!.toJson(),
        'recorder_id': recorderId,
        'name': name,
        'description': description,
        'manufacturer': manufacturer,
        'model': model,
        'firmware_version': firmwareVersion,
        'mac_address': macAddress,
        'ip_address': ipAddress,
        'credential_ref': credentialRef,
        if (config != null) 'config': config!.toJson(),
        'config_version': configVersion,
        'state': state.index,
        'labels': labels,
      };
}

/// ListCamerasRequest
class PbListCamerasRequest {
  final PbTenantRef? tenant;
  final String recorderId;
  final String search;
  final int pageSize;
  final String cursor;

  const PbListCamerasRequest({
    this.tenant,
    this.recorderId = '',
    this.search = '',
    this.pageSize = 0,
    this.cursor = '',
  });

  Map<String, dynamic> toJson() => {
        if (tenant != null) 'tenant': tenant!.toJson(),
        'recorder_id': recorderId,
        'search': search,
        'page_size': pageSize,
        'cursor': cursor,
      };
}

/// ListCamerasResponse
class PbListCamerasResponse {
  final List<PbCamera> cameras;
  final String nextCursor;

  const PbListCamerasResponse({
    this.cameras = const [],
    this.nextCursor = '',
  });

  factory PbListCamerasResponse.fromJson(Map<String, dynamic> json) =>
      PbListCamerasResponse(
        cameras: (json['cameras'] as List<dynamic>?)
                ?.map((e) => PbCamera.fromJson(e as Map<String, dynamic>))
                .toList() ??
            const [],
        nextCursor: json['next_cursor'] as String? ?? '',
      );
}
