// KAI-301 — Camera value object for the multi-camera grid.
//
// Pure, immutable data. The grid layer does not own camera discovery — it
// receives an already-resolved list from a higher-level provider that talks
// to Directory (see federated camera tree, KAI-299). Nothing in this file
// does any I/O.
//
// Field conventions:
//   * `id`                         — stable, opaque camera id from Directory.
//   * `directoryConnectionId`      — which HomeDirectoryConnection owns the
//                                    camera. Used by StreamUrlMinter to pick
//                                    the right auth token from the token store.
//   * `label` / `siteLabel`        — presentation-only. Never used as keys.
//   * `snapshotUrl`                — baseline still-image URL served by the
//                                    recorder. The snapshot refresh controller
//                                    still mints a signed URL via
//                                    [StreamUrlMinter] — this field is the
//                                    unauth'd baseline used by previews only.
//   * `subStreamWebRtcEndpoint`    — nullable because some cameras only expose
//                                    a main stream. The render-mode decision
//                                    falls back to snapshot mode when this is
//                                    null regardless of cell count.
//   * `mainStreamWebRtcEndpoint`   — always present for cameras that support
//                                    live playback at all.
//   * `thumbnailUrl`               — cached thumbnail for the placeholder
//                                    shown while WebRTC is negotiating.
//   * `isOnline`                   — derived from federation heartbeat.

import 'package:flutter/foundation.dart';

@immutable
class Camera {
  final String id;
  final String directoryConnectionId;
  final String label;
  final String siteLabel;
  final String snapshotUrl;
  final String? subStreamWebRtcEndpoint;
  final String mainStreamWebRtcEndpoint;
  final String thumbnailUrl;
  final bool isOnline;

  const Camera({
    required this.id,
    required this.directoryConnectionId,
    required this.label,
    required this.siteLabel,
    required this.snapshotUrl,
    required this.mainStreamWebRtcEndpoint,
    required this.thumbnailUrl,
    required this.isOnline,
    this.subStreamWebRtcEndpoint,
  });

  Camera copyWith({
    String? id,
    String? directoryConnectionId,
    String? label,
    String? siteLabel,
    String? snapshotUrl,
    String? subStreamWebRtcEndpoint,
    String? mainStreamWebRtcEndpoint,
    String? thumbnailUrl,
    bool? isOnline,
  }) {
    return Camera(
      id: id ?? this.id,
      directoryConnectionId:
          directoryConnectionId ?? this.directoryConnectionId,
      label: label ?? this.label,
      siteLabel: siteLabel ?? this.siteLabel,
      snapshotUrl: snapshotUrl ?? this.snapshotUrl,
      subStreamWebRtcEndpoint:
          subStreamWebRtcEndpoint ?? this.subStreamWebRtcEndpoint,
      mainStreamWebRtcEndpoint:
          mainStreamWebRtcEndpoint ?? this.mainStreamWebRtcEndpoint,
      thumbnailUrl: thumbnailUrl ?? this.thumbnailUrl,
      isOnline: isOnline ?? this.isOnline,
    );
  }

  @override
  bool operator ==(Object other) =>
      identical(this, other) ||
      other is Camera &&
          runtimeType == other.runtimeType &&
          id == other.id &&
          directoryConnectionId == other.directoryConnectionId &&
          label == other.label &&
          siteLabel == other.siteLabel &&
          snapshotUrl == other.snapshotUrl &&
          subStreamWebRtcEndpoint == other.subStreamWebRtcEndpoint &&
          mainStreamWebRtcEndpoint == other.mainStreamWebRtcEndpoint &&
          thumbnailUrl == other.thumbnailUrl &&
          isOnline == other.isOnline;

  @override
  int get hashCode => Object.hash(
        id,
        directoryConnectionId,
        label,
        siteLabel,
        snapshotUrl,
        subStreamWebRtcEndpoint,
        mainStreamWebRtcEndpoint,
        thumbnailUrl,
        isOnline,
      );

  @override
  String toString() =>
      'Camera(id: $id, label: $label, site: $siteLabel, online: $isOnline)';
}
