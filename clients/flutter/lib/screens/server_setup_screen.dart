import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import '../providers/auth_provider.dart';
import '../theme/nvr_colors.dart';

class ServerSetupScreen extends ConsumerStatefulWidget {
  const ServerSetupScreen({super.key});

  @override
  ConsumerState<ServerSetupScreen> createState() => _ServerSetupScreenState();
}

class _ServerSetupScreenState extends ConsumerState<ServerSetupScreen> {
  final _formKey = GlobalKey<FormState>();
  final _urlController = TextEditingController(text: 'http://');
  bool _isLoading = false;
  String? _error;

  @override
  void dispose() {
    _urlController.dispose();
    super.dispose();
  }

  Future<void> _connect() async {
    if (!_formKey.currentState!.validate()) return;

    setState(() {
      _isLoading = true;
      _error = null;
    });

    final url = _urlController.text.trim();
    final authService = ref.read(authServiceProvider);
    final reachable = await authService.validateServer(url);

    if (!mounted) return;

    if (!reachable) {
      setState(() {
        _isLoading = false;
        _error = 'Server unreachable. Check the URL and try again.';
      });
      return;
    }

    await ref.read(authProvider.notifier).setServer(url);
    // AuthNotifier.setServer() updates state to unauthenticated,
    // which causes the router to navigate to login automatically.
    if (mounted) {
      setState(() => _isLoading = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: NvrColors.bgPrimary,
      body: Center(
        child: SingleChildScrollView(
          padding: const EdgeInsets.all(24),
          child: ConstrainedBox(
            constraints: const BoxConstraints(maxWidth: 400),
            child: Card(
              color: NvrColors.bgSecondary,
              shape: RoundedRectangleBorder(
                borderRadius: BorderRadius.circular(16),
                side: const BorderSide(color: NvrColors.border),
              ),
              child: Padding(
                padding: const EdgeInsets.all(32),
                child: Form(
                  key: _formKey,
                  child: Column(
                    mainAxisSize: MainAxisSize.min,
                    children: [
                      const Icon(
                        Icons.videocam,
                        size: 56,
                        color: NvrColors.accent,
                      ),
                      const SizedBox(height: 16),
                      Text(
                        'Connect to NVR',
                        style: Theme.of(context).textTheme.headlineSmall?.copyWith(
                              color: NvrColors.textPrimary,
                              fontWeight: FontWeight.bold,
                            ),
                      ),
                      const SizedBox(height: 8),
                      Text(
                        'Enter the address of your NVR server',
                        style: Theme.of(context).textTheme.bodyMedium?.copyWith(
                              color: NvrColors.textSecondary,
                            ),
                        textAlign: TextAlign.center,
                      ),
                      const SizedBox(height: 32),
                      TextFormField(
                        controller: _urlController,
                        keyboardType: TextInputType.url,
                        autocorrect: false,
                        style: const TextStyle(color: NvrColors.textPrimary),
                        decoration: InputDecoration(
                          labelText: 'Server URL',
                          hintText: 'http://192.168.1.100:8888',
                          labelStyle: const TextStyle(color: NvrColors.textSecondary),
                          hintStyle: const TextStyle(color: NvrColors.textMuted),
                          filled: true,
                          fillColor: NvrColors.bgInput,
                          border: OutlineInputBorder(
                            borderRadius: BorderRadius.circular(8),
                            borderSide: const BorderSide(color: NvrColors.border),
                          ),
                          enabledBorder: OutlineInputBorder(
                            borderRadius: BorderRadius.circular(8),
                            borderSide: const BorderSide(color: NvrColors.border),
                          ),
                          focusedBorder: OutlineInputBorder(
                            borderRadius: BorderRadius.circular(8),
                            borderSide: const BorderSide(color: NvrColors.accent, width: 2),
                          ),
                          prefixIcon: const Icon(Icons.link, color: NvrColors.textMuted),
                        ),
                        validator: (value) {
                          if (value == null || value.trim().isEmpty) {
                            return 'Please enter a server URL';
                          }
                          final trimmed = value.trim();
                          if (!trimmed.startsWith('http://') && !trimmed.startsWith('https://')) {
                            return 'URL must start with http:// or https://';
                          }
                          return null;
                        },
                        onFieldSubmitted: (_) => _isLoading ? null : _connect(),
                      ),
                      if (_error != null) ...[
                        const SizedBox(height: 16),
                        Container(
                          padding: const EdgeInsets.all(12),
                          decoration: BoxDecoration(
                            color: NvrColors.danger.withAlpha(26),
                            borderRadius: BorderRadius.circular(8),
                            border: Border.all(color: NvrColors.danger.withAlpha(77)),
                          ),
                          child: Row(
                            children: [
                              const Icon(Icons.error_outline, color: NvrColors.danger, size: 18),
                              const SizedBox(width: 8),
                              Expanded(
                                child: Text(
                                  _error!,
                                  style: const TextStyle(color: NvrColors.danger, fontSize: 13),
                                ),
                              ),
                            ],
                          ),
                        ),
                      ],
                      const SizedBox(height: 24),
                      SizedBox(
                        width: double.infinity,
                        height: 48,
                        child: FilledButton(
                          onPressed: _isLoading ? null : _connect,
                          style: FilledButton.styleFrom(
                            backgroundColor: NvrColors.accent,
                            disabledBackgroundColor: NvrColors.accent.withAlpha(128),
                            shape: RoundedRectangleBorder(
                              borderRadius: BorderRadius.circular(8),
                            ),
                          ),
                          child: _isLoading
                              ? const SizedBox(
                                  width: 20,
                                  height: 20,
                                  child: CircularProgressIndicator(
                                    strokeWidth: 2,
                                    color: Colors.white,
                                  ),
                                )
                              : const Text(
                                  'Connect',
                                  style: TextStyle(
                                    fontSize: 16,
                                    fontWeight: FontWeight.w600,
                                    color: Colors.white,
                                  ),
                                ),
                        ),
                      ),
                    ],
                  ),
                ),
              ),
            ),
          ),
        ),
      ),
    );
  }
}
