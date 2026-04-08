// KAI-296 — Discovery flow end-to-end (with a mocked HTTP client).
//
// Verifies that selecting a candidate, running the probe, and constructing a
// HomeDirectoryConnection all compose correctly through the
// DiscoveryResultsController. No real network; http.Client is a MockClient.

import 'dart:convert';

import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';
import 'package:nvr_client/discovery/discover_probe.dart';
import 'package:nvr_client/discovery/discovery.dart';
import 'package:nvr_client/discovery/discovery_providers.dart';
import 'package:nvr_client/state/home_directory_connection.dart';

void main() {
  group('Discovery flow', () {
    test('manual URL → probe → HomeDirectoryConnection (on-prem, mDNS kind)',
        () async {
      final mock = MockClient((req) async {
        expect(req.url.path, '/api/v1/discover');
        return http.Response(
          jsonEncode({
            'service': 'kaivue-directory',
            'protocol_version': 1,
            'server_name': 'HQ NVR',
            'server_version': '1.4.2',
            'deployment': 'on_prem',
            'auth_methods': ['local'],
          }),
          200,
        );
      });
      final probe = DiscoverProbe(client: mock);
      final controller = DiscoveryResultsController(
        probe: probe,
        idGenerator: () => 'test-id-1',
      );

      final candidate = ManualDiscovery().submit('https://nvr.acme.local');
      await controller.probeCandidate(candidate);

      expect(controller.state.isProbing, false);
      expect(controller.state.probeError, isNull);
      expect(controller.state.probeResult, isNotNull);
      expect(controller.state.probeResult!.serverName, 'HQ NVR');

      final conn = controller.buildConnection();
      expect(conn, isNotNull);
      expect(conn!.id, 'test-id-1');
      expect(conn.kind, HomeConnectionKind.onPrem);
      expect(conn.displayName, 'HQ NVR');
      expect(conn.discoveryMethod, DiscoveryMethod.manual);
      expect(conn.endpointUrl, 'https://nvr.acme.local');
    });

    test('probe failure is captured in state and buildConnection returns null',
        () async {
      final mock = MockClient((req) async => http.Response('', 404));
      final probe = DiscoverProbe(client: mock);
      final controller = DiscoveryResultsController(
        probe: probe,
        idGenerator: () => 'test-id-2',
      );

      final candidate = ManualDiscovery().submit('https://nvr.acme.local');
      await controller.probeCandidate(candidate);

      expect(controller.state.probeError, isNotNull);
      expect(controller.state.probeError!.kind,
          DiscoverProbeErrorKind.notKaivue);
      expect(controller.buildConnection(), isNull);
    });

    test('QR invite → probe → cloud HomeDirectoryConnection', () async {
      final mock = MockClient((req) async => http.Response(
            jsonEncode({
              'service': 'kaivue-directory',
              'protocol_version': 1,
              'server_name': 'Acme Cloud',
              'server_version': '2.0.0',
              'deployment': 'cloud',
              'auth_methods': ['oidc', 'local'],
            }),
            200,
          ));
      final probe = DiscoverProbe(client: mock);
      final controller = DiscoveryResultsController(
        probe: probe,
        idGenerator: () => 'cloud-id',
      );

      final payload = jsonEncode({
        'type': 'kaivue-directory',
        'url': 'https://cloud.example.com',
        'display_name': 'Acme',
      });
      final candidate = QrDiscovery().candidateFromPayload(payload);
      await controller.probeCandidate(candidate);

      final conn = controller.buildConnection();
      expect(conn, isNotNull);
      expect(conn!.kind, HomeConnectionKind.cloud);
      expect(conn.discoveryMethod, DiscoveryMethod.qrCode);
      expect(conn.displayName, 'Acme Cloud'); // server-reported name wins
    });
  });
}
