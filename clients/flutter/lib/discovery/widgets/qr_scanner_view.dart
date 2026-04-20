// KAI-296 — QrScannerView widget stub.
//
// Wraps `mobile_scanner` in a thin layer that calls [QrDiscovery.parsePayload]
// on each decoded frame. The first valid Raikada invite wins; subsequent scans
// are ignored until the caller rebuilds the widget.
//
// On platforms where `mobile_scanner` is unavailable or camera permission is
// denied, the widget surfaces a localized error from [DiscoveryStrings].

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:mobile_scanner/mobile_scanner.dart';

import '../discovery.dart';
import '../discovery_providers.dart';

class QrScannerView extends ConsumerStatefulWidget {
  final ValueChanged<DiscoveryCandidate> onScanned;

  const QrScannerView({super.key, required this.onScanned});

  @override
  ConsumerState<QrScannerView> createState() => _QrScannerViewState();
}

class _QrScannerViewState extends ConsumerState<QrScannerView> {
  bool _handled = false;
  String? _errorKey;

  @override
  Widget build(BuildContext context) {
    final s = ref.watch(discoveryStringsProvider);
    final qr = ref.watch(qrDiscoveryProvider);

    return Column(
      children: [
        Padding(
          padding: const EdgeInsets.all(16),
          child: Text(s.qrInstructions),
        ),
        Expanded(
          child: MobileScanner(
            onDetect: (capture) {
              if (_handled) return;
              for (final barcode in capture.barcodes) {
                final raw = barcode.rawValue;
                if (raw == null) continue;
                try {
                  final candidate = qr.candidateFromPayload(raw);
                  setState(() => _handled = true);
                  widget.onScanned(candidate);
                  return;
                } on FormatException {
                  setState(() => _errorKey = 'invalid');
                }
              }
            },
          ),
        ),
        if (_errorKey != null)
          Padding(
            padding: const EdgeInsets.all(16),
            child: Text(s.qrPayloadInvalid),
          ),
      ],
    );
  }
}
