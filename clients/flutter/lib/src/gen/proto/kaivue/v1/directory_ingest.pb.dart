// GENERATED — placeholder until buf generate runs. See README.md.
// Source: kaivue/v1/directory_ingest.proto

import 'cameras.pb.dart';

/// AIEventKind — detection type.
enum PbAIEventKind {
  unspecified,
  motion,
  person,
  vehicle,
  face,
  licensePlate,
  audioAlarm,
  lineCrossing,
  loitering,
  tamper,
}

/// BoundingBox — normalized 0..1 rectangle.
class PbBoundingBox {
  final double x;
  final double y;
  final double width;
  final double height;

  const PbBoundingBox({
    this.x = 0,
    this.y = 0,
    this.width = 0,
    this.height = 0,
  });

  factory PbBoundingBox.fromJson(Map<String, dynamic> json) => PbBoundingBox(
        x: (json['x'] as num?)?.toDouble() ?? 0,
        y: (json['y'] as num?)?.toDouble() ?? 0,
        width: (json['width'] as num?)?.toDouble() ?? 0,
        height: (json['height'] as num?)?.toDouble() ?? 0,
      );
}

/// AIEvent — a single detection.
class PbAIEvent {
  final String eventId;
  final String cameraId;
  final PbAIEventKind kind;
  final String kindLabel;
  final DateTime? observedAt;
  final double confidence;
  final PbBoundingBox? bbox;
  final String trackId;
  final String segmentId;
  final String thumbnailRef;
  final Map<String, String> attributes;

  const PbAIEvent({
    this.eventId = '',
    this.cameraId = '',
    this.kind = PbAIEventKind.unspecified,
    this.kindLabel = '',
    this.observedAt,
    this.confidence = 0,
    this.bbox,
    this.trackId = '',
    this.segmentId = '',
    this.thumbnailRef = '',
    this.attributes = const {},
  });

  factory PbAIEvent.fromJson(Map<String, dynamic> json) => PbAIEvent(
        eventId: json['event_id'] as String? ?? '',
        cameraId: json['camera_id'] as String? ?? '',
        kind: PbAIEventKind.values[json['kind'] as int? ?? 0],
        kindLabel: json['kind_label'] as String? ?? '',
        observedAt: json['observed_at'] != null
            ? DateTime.parse(json['observed_at'] as String)
            : null,
        confidence: (json['confidence'] as num?)?.toDouble() ?? 0,
        bbox: json['bbox'] != null
            ? PbBoundingBox.fromJson(json['bbox'] as Map<String, dynamic>)
            : null,
        trackId: json['track_id'] as String? ?? '',
        segmentId: json['segment_id'] as String? ?? '',
        thumbnailRef: json['thumbnail_ref'] as String? ?? '',
        attributes: (json['attributes'] as Map<String, dynamic>?)
                ?.map((k, v) => MapEntry(k, v.toString())) ??
            const {},
      );
}

/// CameraStateUpdate — single camera health snapshot.
class PbCameraStateUpdate {
  final String cameraId;
  final PbCameraState state;
  final DateTime? observedAt;
  final String errorMessage;
  final int currentBitrateKbps;
  final int currentFramerate;
  final DateTime? lastFrameAt;
  final int configVersion;

  const PbCameraStateUpdate({
    this.cameraId = '',
    this.state = PbCameraState.unspecified,
    this.observedAt,
    this.errorMessage = '',
    this.currentBitrateKbps = 0,
    this.currentFramerate = 0,
    this.lastFrameAt,
    this.configVersion = 0,
  });

  factory PbCameraStateUpdate.fromJson(Map<String, dynamic> json) =>
      PbCameraStateUpdate(
        cameraId: json['camera_id'] as String? ?? '',
        state: PbCameraState.values[json['state'] as int? ?? 0],
        errorMessage: json['error_message'] as String? ?? '',
        currentBitrateKbps: json['current_bitrate_kbps'] as int? ?? 0,
        currentFramerate: json['current_framerate'] as int? ?? 0,
        configVersion: json['config_version'] as int? ?? 0,
      );
}

/// SegmentIndexEntry — one recorded media segment.
class PbSegmentIndexEntry {
  final String cameraId;
  final String segmentId;
  final DateTime? startTime;
  final DateTime? endTime;
  final int bytes;
  final String codec;
  final bool hasAudio;
  final bool isEventClip;
  final String storageTier;
  final int sequence;

  const PbSegmentIndexEntry({
    this.cameraId = '',
    this.segmentId = '',
    this.startTime,
    this.endTime,
    this.bytes = 0,
    this.codec = '',
    this.hasAudio = false,
    this.isEventClip = false,
    this.storageTier = '',
    this.sequence = 0,
  });

  factory PbSegmentIndexEntry.fromJson(Map<String, dynamic> json) =>
      PbSegmentIndexEntry(
        cameraId: json['camera_id'] as String? ?? '',
        segmentId: json['segment_id'] as String? ?? '',
        startTime: json['start_time'] != null
            ? DateTime.parse(json['start_time'] as String)
            : null,
        endTime: json['end_time'] != null
            ? DateTime.parse(json['end_time'] as String)
            : null,
        bytes: json['bytes'] as int? ?? 0,
        codec: json['codec'] as String? ?? '',
        hasAudio: json['has_audio'] as bool? ?? false,
        isEventClip: json['is_event_clip'] as bool? ?? false,
        storageTier: json['storage_tier'] as String? ?? '',
        sequence: json['sequence'] as int? ?? 0,
      );
}
