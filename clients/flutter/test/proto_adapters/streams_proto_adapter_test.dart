import 'package:flutter_test/flutter_test.dart';
import 'package:nvr_client/api/streams_api.dart';
import 'package:nvr_client/proto_adapters/streams_proto_adapter.dart';
import 'package:nvr_client/src/gen/proto/kaivue/v1/streams.pb.dart';

void main() {
  group('streamRequestFromProto', () {
    test('wraps signed URL in a single endpoint', () {
      final expiresAt = DateTime.now().add(const Duration(minutes: 30));
      final pb = PbMintStreamURLResponse(
        url: 'https://nvr.acme.local:8889/cam1/whep?token=abc',
        claims: PbStreamClaims(
          nonce: 'nonce-123',
          protocol: PbStreamProtocol.webrtc,
          expiresAt: expiresAt,
        ),
        grantedKind: StreamKindBit.live,
      );

      final req = streamRequestFromProto(pb);

      expect(req.streamId, 'nonce-123');
      expect(req.expiresAt, expiresAt);
      expect(req.endpoints.length, 1);
      expect(req.endpoints[0].url,
          'https://nvr.acme.local:8889/cam1/whep?token=abc');
      expect(req.endpoints[0].transport, StreamTransport.webrtc);
      expect(req.endpoints[0].connectionType, StreamConnectionType.lanDirect);
      expect(req.endpoints[0].priority, 0);
    });

    test('maps HLS protocol', () {
      final pb = PbMintStreamURLResponse(
        url: 'https://nvr.acme.local:8888/cam1/index.m3u8',
        claims: PbStreamClaims(
          protocol: PbStreamProtocol.hls,
          expiresAt: DateTime.now().add(const Duration(hours: 1)),
        ),
      );
      expect(streamRequestFromProto(pb).endpoints[0].transport,
          StreamTransport.llhls);
    });

    test('falls back to webrtc for unspecified protocol', () {
      final pb = PbMintStreamURLResponse(
        url: 'https://test/url',
        claims: PbStreamClaims(
          protocol: PbStreamProtocol.unspecified,
          expiresAt: DateTime.now().add(const Duration(hours: 1)),
        ),
      );
      expect(streamRequestFromProto(pb).endpoints[0].transport,
          StreamTransport.webrtc);
    });
  });

  group('streamKindToProtoBits', () {
    test('live maps to StreamKindBit.live', () {
      expect(streamKindToProtoBits(StreamKind.live), StreamKindBit.live);
    });

    test('playback maps to StreamKindBit.playback', () {
      expect(
          streamKindToProtoBits(StreamKind.playback), StreamKindBit.playback);
    });
  });

  group('streamProtocolToProto', () {
    test('webrtc maps', () {
      expect(streamProtocolToProto(StreamProtocol.webrtc),
          PbStreamProtocol.webrtc);
    });

    test('llhls maps to hls', () {
      expect(
          streamProtocolToProto(StreamProtocol.llhls), PbStreamProtocol.hls);
    });

    test('auto maps to unspecified', () {
      expect(streamProtocolToProto(StreamProtocol.auto),
          PbStreamProtocol.unspecified);
    });
  });
}
