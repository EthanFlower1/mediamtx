// KAI-296 — DiscoveryPicker widget stub.
//
// Radio-style picker that lets the user choose between the three discovery
// methods. This widget is deliberately thin — it renders three options, each
// routing through a caller-supplied callback. The parent screen wires those
// callbacks to navigation (push ManualUrlForm / MdnsBrowseList / QrScannerView).
//
// All strings are sourced from [DiscoveryStrings] via Riverpod — no literals.

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../state/home_directory_connection.dart';
import '../discovery_providers.dart';

class DiscoveryPicker extends ConsumerStatefulWidget {
  final ValueChanged<DiscoveryMethod> onPicked;

  const DiscoveryPicker({super.key, required this.onPicked});

  @override
  ConsumerState<DiscoveryPicker> createState() => _DiscoveryPickerState();
}

class _DiscoveryPickerState extends ConsumerState<DiscoveryPicker> {
  DiscoveryMethod? _selected;

  @override
  Widget build(BuildContext context) {
    final s = ref.watch(discoveryStringsProvider);
    final mdns = ref.watch(mdnsDiscoveryProvider);

    return Column(
      mainAxisSize: MainAxisSize.min,
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Padding(
          padding: const EdgeInsets.all(16),
          child: Text(
            s.pickerTitle,
            style: Theme.of(context).textTheme.titleLarge,
          ),
        ),
        RadioListTile<DiscoveryMethod>(
          value: DiscoveryMethod.manual,
          groupValue: _selected,
          title: Text(s.pickerMethodManual),
          onChanged: (v) => _pick(v),
        ),
        RadioListTile<DiscoveryMethod>(
          value: DiscoveryMethod.mdns,
          groupValue: _selected,
          title: Text(s.pickerMethodMdns),
          subtitle:
              mdns.isSupported ? null : Text(s.mdnsUnsupported),
          onChanged: mdns.isSupported ? (v) => _pick(v) : null,
        ),
        RadioListTile<DiscoveryMethod>(
          value: DiscoveryMethod.qrCode,
          groupValue: _selected,
          title: Text(s.pickerMethodQr),
          onChanged: (v) => _pick(v),
        ),
      ],
    );
  }

  void _pick(DiscoveryMethod? v) {
    if (v == null) return;
    setState(() => _selected = v);
    widget.onPicked(v);
  }
}
