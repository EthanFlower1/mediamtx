// KAI-296 — Discovery sources (manual, mDNS, QR).
//
// Each discovery "source" knows how to surface candidate endpoints the user
// could connect to. It is deliberately decoupled from [DiscoverProbe]: the
// source hands you a [DiscoveryCandidate], the caller probes it, and on
// success a [HomeDirectoryConnection] is constructed.
//
// Note on naming: the state layer (KAI-295) defines a `DiscoveryMethod` enum
// in `state/home_directory_connection.dart`. To avoid colliding with that
// type, the abstract contract in this file is `DiscoverySource`.

import 'dart:async';
import 'dart:convert';
import 'dart:io' show Platform;

import 'package:multicast_dns/multicast_dns.dart';

import '../state/home_directory_connection.dart' as state;

/// mDNS service type we browse for on the LAN.
const String kMdnsServiceName = '_mediamtx-directory._tcp.local';

/// Expected "type" field on a valid Kaivue invite QR code.
const String kQrInviteType = 'kaivue-directory';

/// A single candidate endpoint produced by a discovery source.
///
/// The `method` tag is passed straight through to [HomeDirectoryConnection]
/// after a successful probe.
class DiscoveryCandidate {
  /// Raw URL, ready to be passed to [normalizeBaseUrl].
  final String rawUrl;

  /// Display label for the picker UI. For mDNS this is the instance name, for
  /// QR it's whatever the invite payload carried, for manual it's the URL.
  final String label;

  /// Provenance — matches the enum stored on [HomeDirectoryConnection].
  final state.DiscoveryMethod method;

  /// Optional fingerprint carried by the source (QR invites may embed it).
  final String? tlsFingerprint;

  const DiscoveryCandidate({
    required this.rawUrl,
    required this.label,
    required this.method,
    this.tlsFingerprint,
  });

  @override
  String toString() =>
      'DiscoveryCandidate($method: $label -> $rawUrl)';
}

/// Common contract for every discovery source.
abstract class DiscoverySource {
  /// Whether this source can be used on the current platform. mDNS returns
  /// `false` on web and on some desktop configurations.
  bool get isSupported;

  /// Stream candidates as they are discovered. Callers subscribe, display, and
  /// cancel when done. Manual & QR sources emit exactly once per user action;
  /// mDNS emits continuously until the subscription is cancelled.
  Stream<DiscoveryCandidate> discover();
}

// ---------------------------------------------------------------------------
// Manual entry
// ---------------------------------------------------------------------------

class ManualDiscovery implements DiscoverySource {
  ManualDiscovery();

  @override
  bool get isSupported => true;

  /// Manual discovery is not a background scanner — callers should use
  /// [submit] instead. [discover] exists only to satisfy the interface and
  /// yields an empty stream.
  @override
  Stream<DiscoveryCandidate> discover() async* {}

  /// Validate a user-entered URL.
  ///
  /// Returns `null` on success, or a stable machine-readable error key that
  /// the UI layer maps to a [DiscoveryStrings] field. We deliberately return
  /// keys (not pre-localized strings) so that this validator can live in pure
  /// Dart with no BuildContext.
  static String? validate(String raw) {
    final trimmed = raw.trim();
    if (trimmed.isEmpty) return 'empty';
    Uri parsed;
    try {
      var withScheme = trimmed;
      if (!trimmed.contains('://')) {
        withScheme = 'https://$trimmed';
      }
      final maybe = Uri.tryParse(withScheme);
      if (maybe == null) return 'invalid';
      parsed = maybe;
    } catch (_) {
      return 'invalid';
    }
    if (parsed.host.isEmpty) return 'invalid';
    if (parsed.scheme != 'http' && parsed.scheme != 'https') {
      return 'invalid';
    }
    return null;
  }

  /// Wrap a validated URL in a candidate. Throws [FormatException] if the URL
  /// fails [validate].
  DiscoveryCandidate submit(String rawUrl) {
    final err = validate(rawUrl);
    if (err != null) {
      throw FormatException('invalid URL: $err');
    }
    return DiscoveryCandidate(
      rawUrl: rawUrl.trim(),
      label: rawUrl.trim(),
      method: state.DiscoveryMethod.manual,
    );
  }
}

// ---------------------------------------------------------------------------
// mDNS LAN browse
// ---------------------------------------------------------------------------

/// Thin abstraction over `MDnsClient` so tests can inject a fake. Only the
/// subset of methods we use is exposed.
abstract class MdnsClientFactory {
  Future<MdnsClientHandle> start();
}

abstract class MdnsClientHandle {
  /// Yields a single discovered candidate per emission. The concrete mDNS
  /// implementation synthesizes this from PTR+SRV+TXT lookups.
  Stream<DiscoveryCandidate> browse(String serviceName);
  Future<void> stop();
}

/// Default factory backed by the real `multicast_dns` package. On desktop/web
/// platforms where mDNS browsing is unreliable, [MdnsDiscovery.isSupported]
/// returns `false` and the factory is never invoked.
class _DefaultMdnsClientFactory implements MdnsClientFactory {
  @override
  Future<MdnsClientHandle> start() async {
    final client = MDnsClient();
    await client.start();
    return _RealMdnsHandle(client);
  }
}

class _RealMdnsHandle implements MdnsClientHandle {
  final MDnsClient _client;
  _RealMdnsHandle(this._client);

  @override
  Stream<DiscoveryCandidate> browse(String serviceName) async* {
    await for (final PtrResourceRecord ptr in _client
        .lookup<PtrResourceRecord>(
      ResourceRecordQuery.serverPointer(serviceName),
    )) {
      await for (final SrvResourceRecord srv in _client
          .lookup<SrvResourceRecord>(
        ResourceRecordQuery.service(ptr.domainName),
      )) {
        // Prefer https by convention; the probe will fall back via
        // [normalizeBaseUrl] if the user supplies a different scheme later.
        final url = 'https://${srv.target}:${srv.port}';
        yield DiscoveryCandidate(
          rawUrl: url,
          label: ptr.domainName,
          method: state.DiscoveryMethod.mdns,
        );
      }
    }
  }

  @override
  Future<void> stop() async {
    _client.stop();
  }
}

class MdnsDiscovery implements DiscoverySource {
  final MdnsClientFactory _factory;

  /// Inject a custom factory for tests. The default factory is backed by the
  /// real `multicast_dns` package.
  MdnsDiscovery({MdnsClientFactory? factory})
      : _factory = factory ?? _DefaultMdnsClientFactory();

  /// mDNS browsing is reliable on Android/iOS/macOS. Linux varies by distro,
  /// Windows requires manual setup, and the browser sandbox disallows it
  /// entirely. We conservatively report `true` only for iOS, Android, macOS.
  /// The UI must handle `false` by falling back to the manual path.
  @override
  bool get isSupported {
    // Web platforms don't expose `Platform` without throwing; guard defensively.
    try {
      return Platform.isAndroid || Platform.isIOS || Platform.isMacOS;
    } catch (_) {
      return false;
    }
  }

  @override
  Stream<DiscoveryCandidate> discover() async* {
    if (!isSupported) {
      // Defensive: never start the client on unsupported platforms.
      return;
    }
    final handle = await _factory.start();
    try {
      yield* handle.browse(kMdnsServiceName);
    } finally {
      await handle.stop();
    }
  }
}

// ---------------------------------------------------------------------------
// QR code invite
// ---------------------------------------------------------------------------

/// Parsed Kaivue invite payload.
///
/// The React admin console emits an invite QR as a JSON string of the form:
/// `{"type":"kaivue-directory","url":"https://nvr.acme.local","fingerprint":"..."}`.
/// The optional fields (`display_name`, `fingerprint`) are forwarded verbatim.
class QrInvitePayload {
  final String url;
  final String? displayName;
  final String? tlsFingerprint;

  const QrInvitePayload({
    required this.url,
    this.displayName,
    this.tlsFingerprint,
  });
}

class QrDiscovery implements DiscoverySource {
  QrDiscovery();

  @override
  bool get isSupported => true;

  /// QR discovery is a one-shot — UI wires the scanner, calls [parsePayload]
  /// once a frame decodes, and the `discover()` stream is unused.
  @override
  Stream<DiscoveryCandidate> discover() async* {}

  /// Parse a raw QR code string.
  ///
  /// Returns `null` — intentionally: the caller distinguishes the three
  /// failure modes via the thrown [FormatException]. Throws when:
  ///   * The string is not valid JSON.
  ///   * The decoded JSON is not an object.
  ///   * `type` != [kQrInviteType].
  ///   * `url` is missing or not a string.
  static QrInvitePayload parsePayload(String raw) {
    dynamic decoded;
    try {
      decoded = jsonDecode(raw);
    } catch (e) {
      throw FormatException('QR payload is not JSON: $e');
    }
    if (decoded is! Map<String, dynamic>) {
      throw const FormatException('QR payload is not a JSON object');
    }
    if (decoded['type'] != kQrInviteType) {
      throw FormatException(
        'QR payload type is not $kQrInviteType: ${decoded['type']}',
      );
    }
    final url = decoded['url'];
    if (url is! String || url.isEmpty) {
      throw const FormatException('QR payload missing url');
    }
    final fingerprint = decoded['fingerprint'];
    final displayName = decoded['display_name'];
    return QrInvitePayload(
      url: url,
      displayName: displayName is String ? displayName : null,
      tlsFingerprint: fingerprint is String ? fingerprint : null,
    );
  }

  /// Convenience: parse + wrap into a candidate in one step.
  DiscoveryCandidate candidateFromPayload(String raw) {
    final payload = parsePayload(raw);
    return DiscoveryCandidate(
      rawUrl: payload.url,
      label: payload.displayName ?? payload.url,
      method: state.DiscoveryMethod.qrCode,
      tlsFingerprint: payload.tlsFingerprint,
    );
  }
}
