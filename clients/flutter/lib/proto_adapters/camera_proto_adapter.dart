// KAI-431 — Adapter: PbCamera ↔ Camera (freezed model).
//
// Converts between the proto-generated PbCamera type and the app-layer
// freezed Camera model. This is the single seam that changes when proto
// fields are added — the rest of the app reads Camera, never PbCamera.

import '../models/camera.dart';
import '../src/gen/proto/kaivue/v1/cameras.pb.dart';

/// Converts a proto [PbCamera] to the app-layer [Camera] model.
Camera cameraFromProto(PbCamera pb) {
  return Camera(
    id: pb.id,
    name: pb.name,
    status: _mapState(pb.state),
    hasMainStream: _hasProfile(pb, 'main'),
    hasSubStream: _hasProfile(pb, 'sub'),
    retentionDays: pb.config?.retention?.retentionDays ?? 30,
    eventRetentionDays: pb.config?.retention?.eventRetentionDays ?? 0,
    ptzCapable: false, // PTZ comes from ONVIF probe, not proto
    aiEnabled: false, // AI config is local, not proto
    streamPaths: _extractStreamPaths(pb),
  );
}

/// Converts a list of proto cameras.
List<Camera> camerasFromProto(List<PbCamera> pbs) =>
    pbs.map(cameraFromProto).toList();

/// Maps PbCameraState to the string status the app uses.
String _mapState(PbCameraState s) {
  switch (s) {
    case PbCameraState.online:
      return 'connected';
    case PbCameraState.offline:
      return 'disconnected';
    case PbCameraState.provisioning:
      return 'connecting';
    case PbCameraState.disabled:
      return 'disabled';
    case PbCameraState.error:
      return 'error';
    case PbCameraState.unspecified:
      return 'disconnected';
  }
}

/// Checks if a camera has a profile with the given name.
bool _hasProfile(PbCamera pb, String profileName) {
  if (pb.config == null) return profileName == 'main';
  return pb.config!.profiles.any((p) => p.name == profileName);
}

/// Extracts StreamPath list from proto stream profiles.
List<StreamPath> _extractStreamPaths(PbCamera pb) {
  if (pb.config == null) return const [];
  return pb.config!.profiles
      .map((p) => StreamPath(
            name: p.name,
            path: p.url,
            resolution: p.width > 0 ? '${p.width}x${p.height}' : '',
            videoCodec: p.codec,
          ))
      .toList();
}
