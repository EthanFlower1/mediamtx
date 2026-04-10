// KAI-431 — Adapter: PbMintStreamURLResponse ↔ StreamRequest.
//
// Converts between proto stream minting response and the app-layer
// StreamRequest / StreamEndpoint types used by the live view feature.

import '../api/streams_api.dart';
import '../src/gen/proto/kaivue/v1/streams.pb.dart';

/// Converts a proto [PbMintStreamURLResponse] to the app-layer
/// [StreamRequest]. The proto response carries a single signed URL;
/// the adapter wraps it in a StreamEndpoint with priority 0.
StreamRequest streamRequestFromProto(PbMintStreamURLResponse pb) {
  final protocol = _mapProtocol(pb.claims?.protocol);
  return StreamRequest(
    streamId: pb.claims?.nonce ?? '',
    expiresAt: pb.claims?.expiresAt ?? DateTime.now().add(const Duration(hours: 1)),
    endpoints: [
      StreamEndpoint(
        url: pb.url,
        transport: protocol,
        connectionType: StreamConnectionType.lanDirect,
        priority: 0,
      ),
    ],
  );
}

/// Maps proto StreamProtocol to app-layer StreamTransport.
StreamTransport _mapProtocol(PbStreamProtocol? p) {
  switch (p) {
    case PbStreamProtocol.webrtc:
      return StreamTransport.webrtc;
    case PbStreamProtocol.hls:
    case PbStreamProtocol.mp4:
      return StreamTransport.llhls;
    default:
      return StreamTransport.webrtc;
  }
}

/// Maps app-layer StreamKind to proto kind bitfield.
int streamKindToProtoBits(StreamKind kind) {
  switch (kind) {
    case StreamKind.live:
      return StreamKindBit.live;
    case StreamKind.playback:
      return StreamKindBit.playback;
  }
}

/// Maps app-layer StreamProtocol to proto StreamProtocol.
PbStreamProtocol streamProtocolToProto(StreamProtocol p) {
  switch (p) {
    case StreamProtocol.webrtc:
      return PbStreamProtocol.webrtc;
    case StreamProtocol.llhls:
      return PbStreamProtocol.hls;
    case StreamProtocol.auto:
      return PbStreamProtocol.unspecified;
  }
}
