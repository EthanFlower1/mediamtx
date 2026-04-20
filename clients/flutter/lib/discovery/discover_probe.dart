// KAI-296 — /api/v1/discover probe.
//
// Given a user-supplied base URL (from manual entry, an mDNS advertisement, or
// a QR code payload), this module performs a short HTTP probe against
// `<base>/api/v1/discover` to confirm the remote is actually a Raikada
// directory and to surface the metadata needed to populate a
// [HomeDirectoryConnection] (server name, kind, supported auth methods, etc).
//
// The probe is intentionally thin and typed — the caller is expected to
// render a user-visible error for every variant of [DiscoverProbeError] via
// [DiscoveryStrings]. Real network I/O is done through a [http.Client] so
// tests can inject `MockClient` without touching sockets.

import 'dart:async';
import 'dart:convert';
import 'dart:io' show HandshakeException, SocketException;

import 'package:http/http.dart' as http;

/// Lowest protocol version this client understands. Anything below this is a
/// hard version mismatch. Bump with coordinated server changes.
const int kMinSupportedDiscoverVersion = 1;

/// Highest protocol version this client understands.
const int kMaxSupportedDiscoverVersion = 1;

/// Default LAN timeout — used when the caller knows it's talking to something
/// on the local network (mDNS hit, `.local` hostname, RFC1918 IP).
const Duration kLanProbeTimeout = Duration(seconds: 5);

/// Default WAN timeout — used for public hostnames / cloud endpoints.
const Duration kCloudProbeTimeout = Duration(seconds: 15);

/// Supported auth methods a directory may advertise. Unknown values are
/// preserved as their raw string in [DiscoverResult.rawAuthMethods] so the UI
/// can still show them even if this enum is out of date.
enum DiscoverAuthMethod {
  local,
  oidc,
  saml,
  passkey,
  unknown,
}

DiscoverAuthMethod _authFromWire(String s) {
  switch (s) {
    case 'local':
      return DiscoverAuthMethod.local;
    case 'oidc':
      return DiscoverAuthMethod.oidc;
    case 'saml':
      return DiscoverAuthMethod.saml;
    case 'passkey':
      return DiscoverAuthMethod.passkey;
    default:
      return DiscoverAuthMethod.unknown;
  }
}

/// Whether the probed endpoint is the Raikada cloud or a self-hosted directory.
enum DiscoverDeployment { cloud, onPrem }

/// Result of a successful `/api/v1/discover` call.
class DiscoverResult {
  /// Normalized base URL (no trailing slash) that should be stored on the
  /// resulting [HomeDirectoryConnection.endpointUrl].
  final String baseUrl;

  /// Server-reported display name, e.g. "Acme HQ NVR".
  final String serverName;

  /// Server version string, e.g. `"1.4.2"`.
  final String serverVersion;

  /// Protocol version the server advertises.
  final int protocolVersion;

  /// Cloud vs on-prem, per the server's own declaration.
  final DiscoverDeployment deployment;

  /// Parsed auth methods, with unknown values folded to
  /// [DiscoverAuthMethod.unknown].
  final List<DiscoverAuthMethod> authMethods;

  /// Raw auth method wire strings, preserved verbatim.
  final List<String> rawAuthMethods;

  /// Optional TLS fingerprint the server expects clients to pin. `null` when
  /// not provided (e.g. cloud with a public CA cert).
  final String? tlsFingerprint;

  const DiscoverResult({
    required this.baseUrl,
    required this.serverName,
    required this.serverVersion,
    required this.protocolVersion,
    required this.deployment,
    required this.authMethods,
    required this.rawAuthMethods,
    this.tlsFingerprint,
  });

  @override
  String toString() =>
      'DiscoverResult($serverName @ $baseUrl, v$serverVersion, $deployment)';
}

/// Typed error surface. Exactly one variant is returned per failed probe.
enum DiscoverProbeErrorKind {
  unreachable,
  timeout,
  tlsMismatch,
  notRaikada,
  versionMismatch,
  malformedResponse,
}

class DiscoverProbeError implements Exception {
  final DiscoverProbeErrorKind kind;
  final String debugMessage;
  DiscoverProbeError(this.kind, this.debugMessage);

  @override
  String toString() => 'DiscoverProbeError($kind): $debugMessage';
}

/// Normalize a user-supplied URL string into the base URL we'll probe.
///
/// - Accepts `nvr.acme.local`, `http://nvr.acme.local`, `https://nvr.acme.local/`,
///   or a full `https://host:8443/some/prefix/`.
/// - Defaults to `https://` when no scheme is supplied.
/// - Strips trailing slashes and any existing `/api/v1/discover` tail so
///   callers can paste either a UI URL or an API URL.
/// - Throws [FormatException] on anything that doesn't parse as a URI or uses
///   an unsupported scheme.
Uri normalizeBaseUrl(String raw) {
  final trimmed = raw.trim();
  if (trimmed.isEmpty) {
    throw const FormatException('empty URL');
  }
  var withScheme = trimmed;
  if (!trimmed.contains('://')) {
    withScheme = 'https://$trimmed';
  }
  final parsed = Uri.tryParse(withScheme);
  if (parsed == null || parsed.host.isEmpty) {
    throw FormatException('not a URL: $raw');
  }
  if (parsed.scheme != 'http' && parsed.scheme != 'https') {
    throw FormatException('unsupported scheme: ${parsed.scheme}');
  }
  // Drop trailing `/api/v1/discover` if present, and any trailing slash.
  var path = parsed.path;
  if (path.endsWith('/api/v1/discover')) {
    path = path.substring(0, path.length - '/api/v1/discover'.length);
  }
  if (path.endsWith('/')) {
    path = path.substring(0, path.length - 1);
  }
  return parsed.replace(path: path);
}

/// Low-level probe client. One per discovery flow; cheap to construct.
class DiscoverProbe {
  final http.Client _client;

  /// Allow injecting a [http.Client] (e.g. `MockClient`) for tests.
  DiscoverProbe({http.Client? client}) : _client = client ?? http.Client();

  /// GET `<endpoint>/api/v1/discover` and parse the response.
  ///
  /// [endpoint] should already be a base URL (see [normalizeBaseUrl]). The
  /// caller picks [timeout]; use [kLanProbeTimeout] for LAN targets and
  /// [kCloudProbeTimeout] for cloud/WAN targets.
  Future<DiscoverResult> probe(
    Uri endpoint, {
    Duration timeout = kCloudProbeTimeout,
  }) async {
    final discoverUri = endpoint.replace(
      path: '${endpoint.path}/api/v1/discover',
    );

    http.Response resp;
    try {
      resp = await _client
          .get(discoverUri, headers: const {'Accept': 'application/json'})
          .timeout(timeout);
    } on TimeoutException catch (e) {
      throw DiscoverProbeError(
        DiscoverProbeErrorKind.timeout,
        'probe timed out after ${timeout.inSeconds}s: $e',
      );
    } on HandshakeException catch (e) {
      throw DiscoverProbeError(
        DiscoverProbeErrorKind.tlsMismatch,
        'TLS handshake failed: $e',
      );
    } on SocketException catch (e) {
      throw DiscoverProbeError(
        DiscoverProbeErrorKind.unreachable,
        'socket error: $e',
      );
    } on http.ClientException catch (e) {
      // Most non-timeout network problems surface as ClientException from
      // package:http. We conservatively map them to unreachable.
      throw DiscoverProbeError(
        DiscoverProbeErrorKind.unreachable,
        'http client error: $e',
      );
    }

    if (resp.statusCode == 404) {
      throw DiscoverProbeError(
        DiscoverProbeErrorKind.notRaikada,
        'endpoint returned 404 (not a Raikada directory)',
      );
    }
    if (resp.statusCode < 200 || resp.statusCode >= 300) {
      throw DiscoverProbeError(
        DiscoverProbeErrorKind.unreachable,
        'unexpected status ${resp.statusCode}',
      );
    }

    dynamic parsed;
    try {
      parsed = jsonDecode(resp.body);
    } catch (e) {
      throw DiscoverProbeError(
        DiscoverProbeErrorKind.malformedResponse,
        'response is not JSON: $e',
      );
    }
    if (parsed is! Map<String, dynamic>) {
      throw DiscoverProbeError(
        DiscoverProbeErrorKind.malformedResponse,
        'response is not a JSON object',
      );
    }

    // "service" must identify this as a Raikada directory. Anything else — even
    // a valid JSON response from some other API — is a notRaikada error.
    final service = parsed['service'];
    if (service != 'raikada-directory') {
      throw DiscoverProbeError(
        DiscoverProbeErrorKind.notRaikada,
        'wrong service field: $service',
      );
    }

    final protocolVersion = parsed['protocol_version'];
    if (protocolVersion is! int) {
      throw DiscoverProbeError(
        DiscoverProbeErrorKind.malformedResponse,
        'protocol_version missing or non-integer',
      );
    }
    if (protocolVersion < kMinSupportedDiscoverVersion ||
        protocolVersion > kMaxSupportedDiscoverVersion) {
      throw DiscoverProbeError(
        DiscoverProbeErrorKind.versionMismatch,
        'server protocol v$protocolVersion outside supported '
        '[$kMinSupportedDiscoverVersion..$kMaxSupportedDiscoverVersion]',
      );
    }

    final serverName = parsed['server_name'];
    final serverVersion = parsed['server_version'];
    if (serverName is! String || serverVersion is! String) {
      throw DiscoverProbeError(
        DiscoverProbeErrorKind.malformedResponse,
        'server_name / server_version missing',
      );
    }

    final deploymentRaw = parsed['deployment'];
    DiscoverDeployment deployment;
    switch (deploymentRaw) {
      case 'cloud':
        deployment = DiscoverDeployment.cloud;
        break;
      case 'on_prem':
      case 'onprem':
        deployment = DiscoverDeployment.onPrem;
        break;
      default:
        throw DiscoverProbeError(
          DiscoverProbeErrorKind.malformedResponse,
          'deployment field missing or invalid: $deploymentRaw',
        );
    }

    final rawAuth = parsed['auth_methods'];
    if (rawAuth is! List) {
      throw DiscoverProbeError(
        DiscoverProbeErrorKind.malformedResponse,
        'auth_methods missing or not a list',
      );
    }
    final rawAuthStrings = rawAuth.whereType<String>().toList(growable: false);
    final authMethods =
        rawAuthStrings.map(_authFromWire).toList(growable: false);

    final fingerprint = parsed['tls_fingerprint'];
    final tlsFingerprint = fingerprint is String ? fingerprint : null;

    return DiscoverResult(
      baseUrl: endpoint.toString(),
      serverName: serverName,
      serverVersion: serverVersion,
      protocolVersion: protocolVersion,
      deployment: deployment,
      authMethods: authMethods,
      rawAuthMethods: rawAuthStrings,
      tlsFingerprint: tlsFingerprint,
    );
  }

  /// Release the underlying HTTP client. Safe to call multiple times.
  void dispose() => _client.close();
}
