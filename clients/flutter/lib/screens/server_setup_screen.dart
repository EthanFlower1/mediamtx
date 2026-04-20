import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import '../providers/auth_provider.dart';
import '../theme/nvr_colors.dart';
import '../theme/nvr_typography.dart';

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
      backgroundColor: NvrColors.of(context).bgPrimary,
      body: Center(
        child: SingleChildScrollView(
          padding: const EdgeInsets.all(24),
          child: ConstrainedBox(
            constraints: const BoxConstraints(maxWidth: 400),
            child: Container(
              decoration: BoxDecoration(
                color: NvrColors.of(context).bgSecondary,
                borderRadius: BorderRadius.circular(8),
                border: Border.all(color: NvrColors.of(context).border),
              ),
              child: Padding(
                padding: const EdgeInsets.all(32),
                child: Form(
                  key: _formKey,
                  child: Column(
                    mainAxisSize: MainAxisSize.min,
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      // Raikada logo
                      Center(
                        child: Image.asset(
                          'assets/raikada-logo-no-bg.png',
                          width: 80,
                          height: 80,
                        ),
                      ),
                      const SizedBox(height: 12),
                      Center(
                        child: Text(
                          'RAIKADA',
                          style: NvrTypography.of(context).monoSection,
                        ),
                      ),
                      const SizedBox(height: 8),
                      Center(
                        child: Text(
                          'Connect to your NVR server',
                          style: NvrTypography.of(context).body,
                          textAlign: TextAlign.center,
                        ),
                      ),
                      const SizedBox(height: 32),

                      // Server URL field
                      Text('SERVER URL', style: NvrTypography.of(context).monoLabel),
                      const SizedBox(height: 6),
                      TextFormField(
                        controller: _urlController,
                        keyboardType: TextInputType.url,
                        autocorrect: false,
                        style: TextStyle(
                          color: NvrColors.of(context).textPrimary,
                          fontFamily: 'IBMPlexSans',
                          fontSize: 14,
                        ),
                        decoration: InputDecoration(
                          filled: true,
                          fillColor: NvrColors.of(context).bgInput,
                          hintText: 'http://192.168.1.100:8888',
                          hintStyle: TextStyle(color: NvrColors.of(context).textMuted),
                          contentPadding: const EdgeInsets.symmetric(
                            horizontal: 12,
                            vertical: 14,
                          ),
                          border: OutlineInputBorder(
                            borderRadius: BorderRadius.circular(4),
                            borderSide: BorderSide(color: NvrColors.of(context).border),
                          ),
                          enabledBorder: OutlineInputBorder(
                            borderRadius: BorderRadius.circular(4),
                            borderSide: BorderSide(color: NvrColors.of(context).border),
                          ),
                          focusedBorder: OutlineInputBorder(
                            borderRadius: BorderRadius.circular(4),
                            borderSide:
                                BorderSide(color: NvrColors.of(context).accent, width: 2),
                          ),
                          errorBorder: OutlineInputBorder(
                            borderRadius: BorderRadius.circular(4),
                            borderSide: BorderSide(color: NvrColors.of(context).danger),
                          ),
                          focusedErrorBorder: OutlineInputBorder(
                            borderRadius: BorderRadius.circular(4),
                            borderSide:
                                BorderSide(color: NvrColors.of(context).danger, width: 2),
                          ),
                          errorStyle: NvrTypography.of(context).alert,
                        ),
                        validator: (value) {
                          if (value == null || value.trim().isEmpty) {
                            return 'Please enter a server URL';
                          }
                          final trimmed = value.trim();
                          if (!trimmed.startsWith('http://') &&
                              !trimmed.startsWith('https://')) {
                            return 'URL must start with http:// or https://';
                          }
                          return null;
                        },
                        onFieldSubmitted: (_) => _isLoading ? null : _connect(),
                      ),

                      // Error message
                      if (_error != null) ...[
                        const SizedBox(height: 12),
                        Text(_error!, style: NvrTypography.of(context).alert),
                      ],

                      const SizedBox(height: 24),

                      // Connect button
                      SizedBox(
                        width: double.infinity,
                        height: 44,
                        child: ElevatedButton(
                          onPressed: _isLoading ? null : _connect,
                          child: _isLoading
                              ? const SizedBox(
                                  width: 18,
                                  height: 18,
                                  child: CircularProgressIndicator(
                                    strokeWidth: 2,
                                    color: Colors.white,
                                  ),
                                )
                              : const Text('CONNECT'),
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
