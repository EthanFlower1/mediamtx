// GENERATED — placeholder until buf generate runs. See README.md.
// Source: kaivue/v1/streams.proto

import 'auth.pb.dart';

/// StreamKindBit — bitfield of media capabilities.
class StreamKindBit {
  static const int unspecified = 0;
  static const int live = 1;
  static const int playback = 2;
  static const int snapshot = 4;
  static const int audioTalkback = 8;
}

/// StreamProtocol — wire protocol for stream tokens.
enum PbStreamProtocol {
  unspecified,
  webrtc,
  hls,
  rtsp,
  mp4,
  jpeg,
}

/// PlaybackRange constrains a playback token to a time window.
class PbPlaybackRange {
  final DateTime? start;
  final DateTime? end;

  const PbPlaybackRange({this.start, this.end});

  factory PbPlaybackRange.fromJson(Map<String, dynamic> json) =>
      PbPlaybackRange(
        start: json['start'] != null
            ? DateTime.parse(json['start'] as String)
            : null,
        end: json['end'] != null
            ? DateTime.parse(json['end'] as String)
            : null,
      );

  Map<String, dynamic> toJson() => {
        if (start != null) 'start': start!.toIso8601String(),
        if (end != null) 'end': end!.toIso8601String(),
      };
}

/// StreamClaims — the body of the stream JWT.
class PbStreamClaims {
  final String userId;
  final PbTenantRef? tenantRef;
  final String cameraId;
  final String recorderId;
  final int kind;
  final PbStreamProtocol protocol;
  final String nonce;
  final PbPlaybackRange? playbackRange;
  final DateTime? issuedAt;
  final DateTime? expiresAt;
  final String clientIp;
  final String sessionId;

  const PbStreamClaims({
    this.userId = '',
    this.tenantRef,
    this.cameraId = '',
    this.recorderId = '',
    this.kind = 0,
    this.protocol = PbStreamProtocol.unspecified,
    this.nonce = '',
    this.playbackRange,
    this.issuedAt,
    this.expiresAt,
    this.clientIp = '',
    this.sessionId = '',
  });

  factory PbStreamClaims.fromJson(Map<String, dynamic> json) =>
      PbStreamClaims(
        userId: json['user_id'] as String? ?? '',
        tenantRef: json['tenant_ref'] != null
            ? PbTenantRef.fromJson(json['tenant_ref'] as Map<String, dynamic>)
            : null,
        cameraId: json['camera_id'] as String? ?? '',
        recorderId: json['recorder_id'] as String? ?? '',
        kind: json['kind'] as int? ?? 0,
        protocol:
            PbStreamProtocol.values[json['protocol'] as int? ?? 0],
        nonce: json['nonce'] as String? ?? '',
        clientIp: json['client_ip'] as String? ?? '',
        sessionId: json['session_id'] as String? ?? '',
      );
}

/// MintStreamURLRequest
class PbMintStreamURLRequest {
  final String cameraId;
  final int requestedKind;
  final PbStreamProtocol preferredProtocol;
  final PbPlaybackRange? playbackRange;
  final String clientIp;
  final int maxTtlSeconds;

  const PbMintStreamURLRequest({
    this.cameraId = '',
    this.requestedKind = 0,
    this.preferredProtocol = PbStreamProtocol.unspecified,
    this.playbackRange,
    this.clientIp = '',
    this.maxTtlSeconds = 0,
  });

  Map<String, dynamic> toJson() => {
        'camera_id': cameraId,
        'requested_kind': requestedKind,
        'preferred_protocol': preferredProtocol.index,
        if (playbackRange != null) 'playback_range': playbackRange!.toJson(),
        'client_ip': clientIp,
        'max_ttl_seconds': maxTtlSeconds,
      };
}

/// MintStreamURLResponse
class PbMintStreamURLResponse {
  final String url;
  final PbStreamClaims? claims;
  final int grantedKind;

  const PbMintStreamURLResponse({
    this.url = '',
    this.claims,
    this.grantedKind = 0,
  });

  factory PbMintStreamURLResponse.fromJson(Map<String, dynamic> json) =>
      PbMintStreamURLResponse(
        url: json['url'] as String? ?? '',
        claims: json['claims'] != null
            ? PbStreamClaims.fromJson(json['claims'] as Map<String, dynamic>)
            : null,
        grantedKind: json['granted_kind'] as int? ?? 0,
      );
}
