// KAI-301 — Stream URL minter abstraction.
//
// This is the Flutter-side seam for the stream URL minting RPCs that
// Directory (cloud) and Recorder (on-prem) own. The grid layer talks to this
// interface only — it does not know about HTTP paths, JWT layouts, or signing.
//
// proto seam: see `internal/shared/proto/v1/streams.proto` (MintStreamURL{,
// Request,Response}, StreamClaims, StreamKindBit, StreamProtocol).
//
// Real implementation owed by:
//   * lead-cloud — federated / off-LAN path: Directory's MintStreamURL RPC
//     (KAI-149 stream URL minting endpoint).
//   * lead-onprem — on-LAN path: direct Recorder mint.
//
// For this PR we ship the interface + a [FakeStreamUrlMinter] used by tests.

import 'package:flutter/foundation.dart';

/// A short-lived signed URL suitable for handing directly to a WebRTC / WHEP
/// session.
@immutable
class StreamTicket {
  final String url;
  final DateTime expiresAt;

  /// Whether this is a sub-stream (low-bitrate) or main stream ticket. The
  /// grid layer asks for sub-streams when rendering >1 tile and main streams
  /// when the user taps a tile to expand it.
  final bool isSubStream;

  const StreamTicket({
    required this.url,
    required this.expiresAt,
    required this.isSubStream,
  });

  bool get isExpired => DateTime.now().isAfter(expiresAt);
}

/// A short-lived signed URL for a still-frame JPEG.
@immutable
class SnapshotTicket {
  final String url;
  final DateTime expiresAt;

  /// Auth headers the client should send with the snapshot fetch. Empty when
  /// the signing lives entirely in the query string.
  final Map<String, String> headers;

  const SnapshotTicket({
    required this.url,
    required this.expiresAt,
    this.headers = const {},
  });

  bool get isExpired => DateTime.now().isAfter(expiresAt);
}

/// Seam for Directory + Recorder stream URL minting.
///
/// TODO(lead-cloud, KAI-149): implement the real Directory-backed version.
/// TODO(lead-onprem): implement the real Recorder-backed version for LAN.
abstract class StreamUrlMinter {
  /// Mint a sub-stream (low-bitrate) WebRTC URL for [cameraId]. Used by grids
  /// containing more than one tile.
  Future<StreamTicket> mintSubStream(String cameraId);

  /// Mint a main-stream (full-res) WebRTC URL for [cameraId]. Used when a
  /// tile is expanded to full-screen single-camera view.
  Future<StreamTicket> mintMainStream(String cameraId);

  /// Mint a signed JPEG snapshot URL for [cameraId]. Used by the snapshot
  /// refresh controller for grids with many tiles.
  Future<SnapshotTicket> mintSnapshot(String cameraId);
}

/// In-memory fake used by tests. Produces deterministic URLs with a
/// configurable delay and TTL.
@visibleForTesting
class FakeStreamUrlMinter implements StreamUrlMinter {
  final Duration mintDelay;
  final Duration ttl;
  int subStreamMintCount = 0;
  int mainStreamMintCount = 0;
  int snapshotMintCount = 0;

  FakeStreamUrlMinter({
    this.mintDelay = Duration.zero,
    this.ttl = const Duration(minutes: 5),
  });

  @override
  Future<StreamTicket> mintSubStream(String cameraId) async {
    if (mintDelay > Duration.zero) await Future<void>.delayed(mintDelay);
    subStreamMintCount++;
    return StreamTicket(
      url: 'https://fake.local/whep/$cameraId/sub?token=$subStreamMintCount',
      expiresAt: DateTime.now().add(ttl),
      isSubStream: true,
    );
  }

  @override
  Future<StreamTicket> mintMainStream(String cameraId) async {
    if (mintDelay > Duration.zero) await Future<void>.delayed(mintDelay);
    mainStreamMintCount++;
    return StreamTicket(
      url: 'https://fake.local/whep/$cameraId/main?token=$mainStreamMintCount',
      expiresAt: DateTime.now().add(ttl),
      isSubStream: false,
    );
  }

  @override
  Future<SnapshotTicket> mintSnapshot(String cameraId) async {
    if (mintDelay > Duration.zero) await Future<void>.delayed(mintDelay);
    snapshotMintCount++;
    return SnapshotTicket(
      url: 'https://fake.local/snapshot/$cameraId?token=$snapshotMintCount',
      expiresAt: DateTime.now().add(ttl),
      headers: const {'X-Fake': 'true'},
    );
  }
}
