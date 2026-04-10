// GENERATED — placeholder until buf generate runs. See README.md.
// Source: kaivue/v1/recorder_control.proto

import 'cameras.pb.dart';

/// StreamAssignmentsRequest — opens the push stream.
class PbStreamAssignmentsRequest {
  final String recorderId;
  final int knownVersion;
  final String recorderSoftwareVersion;

  const PbStreamAssignmentsRequest({
    this.recorderId = '',
    this.knownVersion = 0,
    this.recorderSoftwareVersion = '',
  });

  Map<String, dynamic> toJson() => {
        'recorder_id': recorderId,
        'known_version': knownVersion,
        'recorder_software_version': recorderSoftwareVersion,
      };
}

/// Snapshot — full list of cameras assigned to the recorder.
class PbSnapshot {
  final List<PbCamera> cameras;

  const PbSnapshot({this.cameras = const []});

  factory PbSnapshot.fromJson(Map<String, dynamic> json) => PbSnapshot(
        cameras: (json['cameras'] as List<dynamic>?)
                ?.map((e) => PbCamera.fromJson(e as Map<String, dynamic>))
                .toList() ??
            const [],
      );
}

/// CameraAdded event.
class PbCameraAdded {
  final PbCamera? camera;

  const PbCameraAdded({this.camera});

  factory PbCameraAdded.fromJson(Map<String, dynamic> json) => PbCameraAdded(
        camera: json['camera'] != null
            ? PbCamera.fromJson(json['camera'] as Map<String, dynamic>)
            : null,
      );
}

/// CameraUpdated event.
class PbCameraUpdated {
  final PbCamera? camera;

  const PbCameraUpdated({this.camera});

  factory PbCameraUpdated.fromJson(Map<String, dynamic> json) =>
      PbCameraUpdated(
        camera: json['camera'] != null
            ? PbCamera.fromJson(json['camera'] as Map<String, dynamic>)
            : null,
      );
}

/// CameraRemoved event.
class PbCameraRemoved {
  final String cameraId;
  final bool purgeRecordings;
  final String reason;

  const PbCameraRemoved({
    this.cameraId = '',
    this.purgeRecordings = false,
    this.reason = '',
  });

  factory PbCameraRemoved.fromJson(Map<String, dynamic> json) =>
      PbCameraRemoved(
        cameraId: json['camera_id'] as String? ?? '',
        purgeRecordings: json['purge_recordings'] as bool? ?? false,
        reason: json['reason'] as String? ?? '',
      );
}

/// Heartbeat — server liveness ping.
class PbHeartbeat {
  final DateTime? serverTime;

  const PbHeartbeat({this.serverTime});

  factory PbHeartbeat.fromJson(Map<String, dynamic> json) => PbHeartbeat(
        serverTime: json['server_time'] != null
            ? DateTime.parse(json['server_time'] as String)
            : null,
      );
}

/// AssignmentEvent variant tag.
enum AssignmentEventKind {
  snapshot,
  cameraAdded,
  cameraUpdated,
  cameraRemoved,
  heartbeat,
}

/// AssignmentEvent — server-streamed union of push events.
class PbAssignmentEvent {
  final int version;
  final DateTime? emittedAt;
  final AssignmentEventKind kind;
  final PbSnapshot? snapshot;
  final PbCameraAdded? cameraAdded;
  final PbCameraUpdated? cameraUpdated;
  final PbCameraRemoved? cameraRemoved;
  final PbHeartbeat? heartbeat;

  const PbAssignmentEvent({
    this.version = 0,
    this.emittedAt,
    this.kind = AssignmentEventKind.heartbeat,
    this.snapshot,
    this.cameraAdded,
    this.cameraUpdated,
    this.cameraRemoved,
    this.heartbeat,
  });

  factory PbAssignmentEvent.fromJson(Map<String, dynamic> json) {
    AssignmentEventKind kind;
    PbSnapshot? snapshot;
    PbCameraAdded? cameraAdded;
    PbCameraUpdated? cameraUpdated;
    PbCameraRemoved? cameraRemoved;
    PbHeartbeat? heartbeat;

    if (json['snapshot'] != null) {
      kind = AssignmentEventKind.snapshot;
      snapshot =
          PbSnapshot.fromJson(json['snapshot'] as Map<String, dynamic>);
    } else if (json['camera_added'] != null) {
      kind = AssignmentEventKind.cameraAdded;
      cameraAdded = PbCameraAdded.fromJson(
          json['camera_added'] as Map<String, dynamic>);
    } else if (json['camera_updated'] != null) {
      kind = AssignmentEventKind.cameraUpdated;
      cameraUpdated = PbCameraUpdated.fromJson(
          json['camera_updated'] as Map<String, dynamic>);
    } else if (json['camera_removed'] != null) {
      kind = AssignmentEventKind.cameraRemoved;
      cameraRemoved = PbCameraRemoved.fromJson(
          json['camera_removed'] as Map<String, dynamic>);
    } else {
      kind = AssignmentEventKind.heartbeat;
      if (json['heartbeat'] != null) {
        heartbeat =
            PbHeartbeat.fromJson(json['heartbeat'] as Map<String, dynamic>);
      }
    }

    return PbAssignmentEvent(
      version: json['version'] as int? ?? 0,
      kind: kind,
      snapshot: snapshot,
      cameraAdded: cameraAdded,
      cameraUpdated: cameraUpdated,
      cameraRemoved: cameraRemoved,
      heartbeat: heartbeat,
    );
  }
}
