// KAI-296 — DiscoverProbe tests.
//
// Exercises the probe against a mocked `http.Client` — no real sockets, no
// flake. Covers the happy path plus every typed error variant.

import 'dart:convert';
import 'dart:io' show HandshakeException, SocketException;

import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:nvr_client/discovery/discover_probe.dart';

Map<String, dynamic> _validPayload({
  String deployment = 'on_prem',
  int protocolVersion = 1,
  List<String> authMethods = const ['local', 'oidc'],
}) =>
    {
      'service': 'kaivue-directory',
      'protocol_version': protocolVersion,
      'server_name': 'HQ NVR',
      'server_version': '1.4.2',
      'deployment': deployment,
      'auth_methods': authMethods,
      'tls_fingerprint': 'sha256/abc',
    };

void main() {
  group('normalizeBaseUrl', () {
    test('adds https scheme when missing', () {
      expect(normalizeBaseUrl('nvr.acme.local').toString(),
          'https://nvr.acme.local');
    });
    test('preserves port and strips trailing slash', () {
      expect(normalizeBaseUrl('https://nvr.acme.local:8443/').toString(),
          'https://nvr.acme.local:8443');
    });
    test('strips /api/v1/discover tail', () {
      expect(
          normalizeBaseUrl('https://nvr.acme.local/api/v1/discover').toString(),
          'https://nvr.acme.local');
    });
    test('rejects empty', () {
      expect(() => normalizeBaseUrl(''), throwsFormatException);
    });
    test('rejects unsupported scheme', () {
      expect(() => normalizeBaseUrl('ftp://nvr.acme.local'),
          throwsFormatException);
    });
  });

  group('DiscoverProbe.probe', () {
    test('happy path → DiscoverResult', () async {
      final mock = MockClient((req) async {
        expect(req.url.path, '/api/v1/discover');
        return http.Response(jsonEncode(_validPayload()), 200,
            headers: {'content-type': 'application/json'});
      });
      final probe = DiscoverProbe(client: mock);
      final result = await probe.probe(Uri.parse('https://nvr.acme.local'));
      expect(result.serverName, 'HQ NVR');
      expect(result.serverVersion, '1.4.2');
      expect(result.protocolVersion, 1);
      expect(result.deployment, DiscoverDeployment.onPrem);
      expect(result.authMethods, [
        DiscoverAuthMethod.local,
        DiscoverAuthMethod.oidc,
      ]);
      expect(result.rawAuthMethods, ['local', 'oidc']);
      expect(result.tlsFingerprint, 'sha256/abc');
    });

    test('cloud deployment is recognized', () async {
      final mock = MockClient((req) async => http.Response(
          jsonEncode(_validPayload(deployment: 'cloud')), 200));
      final probe = DiscoverProbe(client: mock);
      final result = await probe.probe(Uri.parse('https://cloud.example.com'));
      expect(result.deployment, DiscoverDeployment.cloud);
    });

    test('unknown auth methods degrade to unknown enum', () async {
      final mock = MockClient((req) async => http.Response(
          jsonEncode(_validPayload(authMethods: ['local', 'magiclink'])),
          200));
      final probe = DiscoverProbe(client: mock);
      final result = await probe.probe(Uri.parse('https://nvr.acme.local'));
      expect(result.rawAuthMethods, ['local', 'magiclink']);
      expect(result.authMethods, [
        DiscoverAuthMethod.local,
        DiscoverAuthMethod.unknown,
      ]);
    });

    test('404 → notKaivue', () async {
      final mock = MockClient((req) async => http.Response('', 404));
      final probe = DiscoverProbe(client: mock);
      await expectLater(
        probe.probe(Uri.parse('https://nvr.acme.local')),
        throwsA(isA<DiscoverProbeError>().having(
            (e) => e.kind, 'kind', DiscoverProbeErrorKind.notKaivue)),
      );
    });

    test('500 → unreachable', () async {
      final mock = MockClient((req) async => http.Response('boom', 500));
      final probe = DiscoverProbe(client: mock);
      await expectLater(
        probe.probe(Uri.parse('https://nvr.acme.local')),
        throwsA(isA<DiscoverProbeError>().having(
            (e) => e.kind, 'kind', DiscoverProbeErrorKind.unreachable)),
      );
    });

    test('timeout → timeout error', () async {
      final mock = MockClient((req) async {
        // Exceed the supplied timeout.
        await Future.delayed(const Duration(milliseconds: 200));
        return http.Response(jsonEncode(_validPayload()), 200);
      });
      final probe = DiscoverProbe(client: mock);
      await expectLater(
        probe.probe(Uri.parse('https://nvr.acme.local'),
            timeout: const Duration(milliseconds: 50)),
        throwsA(isA<DiscoverProbeError>().having(
            (e) => e.kind, 'kind', DiscoverProbeErrorKind.timeout)),
      );
    });

    test('TLS handshake failure → tlsMismatch', () async {
      final mock = MockClient((req) async {
        throw const HandshakeException('cert mismatch');
      });
      final probe = DiscoverProbe(client: mock);
      await expectLater(
        probe.probe(Uri.parse('https://nvr.acme.local')),
        throwsA(isA<DiscoverProbeError>().having(
            (e) => e.kind, 'kind', DiscoverProbeErrorKind.tlsMismatch)),
      );
    });

    test('socket error → unreachable', () async {
      final mock = MockClient((req) async {
        throw const SocketException('no route');
      });
      final probe = DiscoverProbe(client: mock);
      await expectLater(
        probe.probe(Uri.parse('https://nvr.acme.local')),
        throwsA(isA<DiscoverProbeError>().having(
            (e) => e.kind, 'kind', DiscoverProbeErrorKind.unreachable)),
      );
    });

    test('non-JSON body → malformedResponse', () async {
      final mock =
          MockClient((req) async => http.Response('<html>hi</html>', 200));
      final probe = DiscoverProbe(client: mock);
      await expectLater(
        probe.probe(Uri.parse('https://nvr.acme.local')),
        throwsA(isA<DiscoverProbeError>().having(
            (e) => e.kind, 'kind', DiscoverProbeErrorKind.malformedResponse)),
      );
    });

    test('wrong service → notKaivue', () async {
      final mock = MockClient((req) async => http.Response(
          jsonEncode({'service': 'prometheus'}), 200));
      final probe = DiscoverProbe(client: mock);
      await expectLater(
        probe.probe(Uri.parse('https://nvr.acme.local')),
        throwsA(isA<DiscoverProbeError>().having(
            (e) => e.kind, 'kind', DiscoverProbeErrorKind.notKaivue)),
      );
    });

    test('protocol version out of range → versionMismatch', () async {
      final mock = MockClient((req) async => http.Response(
          jsonEncode(_validPayload(protocolVersion: 99)), 200));
      final probe = DiscoverProbe(client: mock);
      await expectLater(
        probe.probe(Uri.parse('https://nvr.acme.local')),
        throwsA(isA<DiscoverProbeError>().having(
            (e) => e.kind, 'kind', DiscoverProbeErrorKind.versionMismatch)),
      );
    });

    test('missing required field → malformedResponse', () async {
      final payload = _validPayload()..remove('server_name');
      final mock = MockClient(
          (req) async => http.Response(jsonEncode(payload), 200));
      final probe = DiscoverProbe(client: mock);
      await expectLater(
        probe.probe(Uri.parse('https://nvr.acme.local')),
        throwsA(isA<DiscoverProbeError>().having(
            (e) => e.kind, 'kind', DiscoverProbeErrorKind.malformedResponse)),
      );
    });
  });
}
