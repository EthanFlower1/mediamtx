// KAI-297 — LocalLoginForm widget.
//
// Email + password fields with inline validation. On submit, calls
// `loginStateProvider.submitLocal` and lets [LoginScreen] react to the
// resulting state transitions. Strings come from `authStringsProvider`.

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../state/home_directory_connection.dart';
import '../auth_providers.dart';

class LocalLoginForm extends ConsumerStatefulWidget {
  final HomeDirectoryConnection connection;

  const LocalLoginForm({super.key, required this.connection});

  @override
  ConsumerState<LocalLoginForm> createState() => _LocalLoginFormState();
}

class _LocalLoginFormState extends ConsumerState<LocalLoginForm> {
  final _formKey = GlobalKey<FormState>();
  final _emailCtrl = TextEditingController();
  final _passwordCtrl = TextEditingController();

  @override
  void dispose() {
    _emailCtrl.dispose();
    _passwordCtrl.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final s = ref.watch(authStringsProvider);
    final loginState = ref.watch(loginStateProvider);
    final notifier = ref.read(loginStateProvider.notifier);

    return Form(
      key: _formKey,
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        mainAxisSize: MainAxisSize.min,
        children: [
          Text(s.localFormHeader, style: Theme.of(context).textTheme.titleSmall),
          const SizedBox(height: 12),
          TextFormField(
            controller: _emailCtrl,
            keyboardType: TextInputType.emailAddress,
            autocorrect: false,
            enabled: !loginState.isSubmitting,
            decoration: InputDecoration(
              labelText: s.localFormEmailLabel,
              hintText: s.localFormEmailHint,
            ),
            validator: (raw) =>
                (raw == null || raw.trim().isEmpty) ? s.localFormEmailEmpty : null,
          ),
          const SizedBox(height: 12),
          TextFormField(
            controller: _passwordCtrl,
            obscureText: true,
            enabled: !loginState.isSubmitting,
            decoration: InputDecoration(
              labelText: s.localFormPasswordLabel,
              hintText: s.localFormPasswordHint,
            ),
            validator: (raw) =>
                (raw == null || raw.isEmpty) ? s.localFormPasswordEmpty : null,
          ),
          const SizedBox(height: 16),
          FilledButton(
            onPressed: loginState.isSubmitting
                ? null
                : () {
                    if (_formKey.currentState?.validate() ?? false) {
                      notifier.submitLocal(
                        connection: widget.connection,
                        username: _emailCtrl.text.trim(),
                        password: _passwordCtrl.text,
                      );
                    }
                  },
            child: loginState.phase == _phaseSubmitting
                ? const SizedBox(
                    width: 18,
                    height: 18,
                    child: CircularProgressIndicator(strokeWidth: 2),
                  )
                : Text(s.localFormSubmit),
          ),
        ],
      ),
    );
  }
}

const _phaseSubmitting = LoginPhase.submitting;
