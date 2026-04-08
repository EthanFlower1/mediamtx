// KAI-296 — ManualUrlForm widget stub.
//
// Single-URL entry field with validation. On submit, wraps the URL in a
// [DiscoveryCandidate] (via [ManualDiscovery.submit]) and hands it to the
// caller for probing.

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../discovery.dart';
import '../discovery_providers.dart';

class ManualUrlForm extends ConsumerStatefulWidget {
  final ValueChanged<DiscoveryCandidate> onSubmit;

  const ManualUrlForm({super.key, required this.onSubmit});

  @override
  ConsumerState<ManualUrlForm> createState() => _ManualUrlFormState();
}

class _ManualUrlFormState extends ConsumerState<ManualUrlForm> {
  final _controller = TextEditingController();
  final _formKey = GlobalKey<FormState>();

  @override
  void dispose() {
    _controller.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final s = ref.watch(discoveryStringsProvider);
    final manual = ref.watch(manualDiscoveryProvider);

    return Form(
      key: _formKey,
      child: Column(
        mainAxisSize: MainAxisSize.min,
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          TextFormField(
            controller: _controller,
            keyboardType: TextInputType.url,
            autocorrect: false,
            decoration: InputDecoration(
              labelText: s.manualUrlLabel,
              hintText: s.manualUrlHint,
            ),
            validator: (raw) {
              final key = ManualDiscovery.validate(raw ?? '');
              switch (key) {
                case 'empty':
                  return s.manualUrlEmpty;
                case 'invalid':
                  return s.manualUrlInvalid;
                default:
                  return null;
              }
            },
          ),
          const SizedBox(height: 12),
          FilledButton(
            onPressed: () {
              if (_formKey.currentState?.validate() ?? false) {
                widget.onSubmit(manual.submit(_controller.text));
              }
            },
            child: Text(s.manualConnectButton),
          ),
        ],
      ),
    );
  }
}
