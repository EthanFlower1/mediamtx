// KAI-296 — Unit tests for DiscoverySource implementations.
//
// Covers URL validation, QR payload parsing, and the mDNS source with a
// fake mDNS client that streams synthetic advertisements.

import 'dart:convert';

import 'package:flutter_test/flutter_test.dart';
import 'package:nvr_client/discovery/discovery.dart';
import 'package:nvr_client/state/home_directory_connection.dart' as state;

// ---- Fake mDNS client -----------------------------------------------------

class _FakeHandle implements MdnsClientHandle {
  final List<DiscoveryCandidate> _seed;
  bool stopped = false;
  _FakeHandle(this._seed);

  @override
  Stream<DiscoveryCandidate> browse(String serviceName) async* {
    for (final c in _seed) {
      yield c;
    }
  }

  @override
  Future<void> stop() async {
    stopped = true;
  }
}

class _FakeFactory implements MdnsClientFactory {
  final List<DiscoveryCandidate> seed;
  late _FakeHandle lastHandle;
  _FakeFactory(this.seed);

  @override
  Future<MdnsClientHandle> start() async {
    lastHandle = _FakeHandle(seed);
    return lastHandle;
  }
}

void main() {
  group('ManualDiscovery.validate', () {
    test('empty is invalid', () {
      expect(ManualDiscovery.validate(''), 'empty');
      expect(ManualDiscovery.validate('   '), 'empty');
    });
    test('bare host is valid', () {
      expect(ManualDiscovery.validate('nvr.acme.local'), isNull);
    });
    test('https URL is valid', () {
      expect(ManualDiscovery.validate('https://nvr.acme.local'), isNull);
    });
    test('http URL is valid', () {
      expect(ManualDiscovery.validate('http://10.0.0.5:8080'), isNull);
    });
    test('unsupported scheme is invalid', () {
      expect(ManualDiscovery.validate('ftp://host'), 'invalid');
    });
    test('nonsense is invalid', () {
      expect(ManualDiscovery.validate('::::'), 'invalid');
    });
  });

  group('ManualDiscovery.submit', () {
    test('produces a manual-method candidate', () {
      final c = ManualDiscovery().submit('https://nvr.acme.local');
      expect(c.method, state.DiscoveryMethod.manual);
      expect(c.rawUrl, 'https://nvr.acme.local');
      expect(c.label, 'https://nvr.acme.local');
    });
    test('rejects invalid URLs', () {
      expect(() => ManualDiscovery().submit(''), throwsFormatException);
    });
  });

  group('QrDiscovery.parsePayload', () {
    test('happy path', () {
      final payload = jsonEncode({
        'type': 'kaivue-directory',
        'url': 'https://nvr.acme.local',
        'fingerprint': 'sha256/abc',
        'display_name': 'HQ',
      });
      final parsed = QrDiscovery.parsePayload(payload);
      expect(parsed.url, 'https://nvr.acme.local');
      expect(parsed.tlsFingerprint, 'sha256/abc');
      expect(parsed.displayName, 'HQ');
    });

    test('candidate wraps the payload', () {
      final payload = jsonEncode({
        'type': 'kaivue-directory',
        'url': 'https://nvr.acme.local',
        'display_name': 'HQ',
      });
      final candidate = QrDiscovery().candidateFromPayload(payload);
      expect(candidate.method, state.DiscoveryMethod.qrCode);
      expect(candidate.label, 'HQ');
    });

    test('non-JSON throws FormatException', () {
      expect(() => QrDiscovery.parsePayload('hello world'),
          throwsFormatException);
    });

    test('wrong type throws FormatException', () {
      final payload = jsonEncode({
        'type': 'something-else',
        'url': 'https://nvr.acme.local',
      });
      expect(() => QrDiscovery.parsePayload(payload), throwsFormatException);
    });

    test('missing url throws FormatException', () {
      final payload = jsonEncode({'type': 'kaivue-directory'});
      expect(() => QrDiscovery.parsePayload(payload), throwsFormatException);
    });
  });

  group('MdnsDiscovery', () {
    test('emits candidates from the injected factory', () async {
      // Skip on unsupported platforms (Linux CI, web). isSupported is wired to
      // Platform.* checks so this test is a no-op there.
      final mdns = MdnsDiscovery(
        factory: _FakeFactory([
          const DiscoveryCandidate(
            rawUrl: 'https://nvr1.local:8443',
            label: 'NVR 1',
            method: state.DiscoveryMethod.mdns,
          ),
          const DiscoveryCandidate(
            rawUrl: 'https://nvr2.local:8443',
            label: 'NVR 2',
            method: state.DiscoveryMethod.mdns,
          ),
        ]),
      );
      if (!mdns.isSupported) {
        // On unsupported platforms discover() yields nothing.
        expect(await mdns.discover().toList(), isEmpty);
        return;
      }
      final got = await mdns.discover().toList();
      expect(got.length, 2);
      expect(got[0].rawUrl, 'https://nvr1.local:8443');
      expect(got[1].rawUrl, 'https://nvr2.local:8443');
    });
  });
}
