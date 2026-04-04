import 'package:dio/dio.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import '../providers/auth_provider.dart';
import '../theme/nvr_colors.dart';

class SetupScreen extends ConsumerStatefulWidget {
  const SetupScreen({super.key});

  @override
  ConsumerState<SetupScreen> createState() => _SetupScreenState();
}

class _SetupScreenState extends ConsumerState<SetupScreen> {
  final _formKey = GlobalKey<FormState>();
  final _usernameController = TextEditingController();
  final _passwordController = TextEditingController();
  final _confirmPasswordController = TextEditingController();
  bool _obscurePassword = true;
  bool _obscureConfirm = true;
  bool _isLoading = false;
  String? _error;
  String? _successMessage;

  @override
  void dispose() {
    _usernameController.dispose();
    _passwordController.dispose();
    _confirmPasswordController.dispose();
    super.dispose();
  }

  Future<void> _createAdmin() async {
    if (!_formKey.currentState!.validate()) return;

    setState(() {
      _isLoading = true;
      _error = null;
      _successMessage = null;
    });

    final authState = ref.read(authProvider);
    final serverUrl = authState.serverUrl;
    if (serverUrl == null || serverUrl.isEmpty) {
      setState(() {
        _isLoading = false;
        _error = 'No server configured. Go back and connect to a server first.';
      });
      return;
    }

    try {
      final dio = Dio();
      await dio.post(
        '$serverUrl/api/nvr/auth/setup',
        data: {
          'username': _usernameController.text.trim(),
          'password': _passwordController.text,
        },
        options: Options(
          receiveTimeout: const Duration(seconds: 10),
          sendTimeout: const Duration(seconds: 10),
        ),
      );

      if (!mounted) return;
      setState(() {
        _isLoading = false;
        _successMessage = 'Admin account created. You can now sign in.';
      });

      // Brief pause so the user can read the success message, then
      // invalidate the auth provider so the router redirects to login.
      await Future<void>.delayed(const Duration(seconds: 1));
      if (mounted) {
        ref.invalidate(authProvider);
      }
    } on DioException catch (e) {
      if (!mounted) return;
      String message;
      if (e.response != null) {
        final data = e.response!.data;
        if (data is Map && data['error'] != null) {
          message = data['error'].toString();
        } else {
          message = 'Server error (${e.response!.statusCode})';
        }
      } else {
        message = 'Could not reach server. Check your connection.';
      }
      setState(() {
        _isLoading = false;
        _error = message;
      });
    } catch (e) {
      if (!mounted) return;
      setState(() {
        _isLoading = false;
        _error = e.toString();
      });
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
            child: Card(
              color: NvrColors.of(context).bgSecondary,
              shape: RoundedRectangleBorder(
                borderRadius: BorderRadius.circular(16),
                side: BorderSide(color: NvrColors.of(context).border),
              ),
              child: Padding(
                padding: const EdgeInsets.all(32),
                child: Form(
                  key: _formKey,
                  child: Column(
                    mainAxisSize: MainAxisSize.min,
                    children: [
                      Icon(
                        Icons.admin_panel_settings,
                        size: 56,
                        color: NvrColors.of(context).accent,
                      ),
                      const SizedBox(height: 16),
                      Text(
                        'Initial Setup',
                        style: Theme.of(context).textTheme.headlineSmall?.copyWith(
                              color: NvrColors.of(context).textPrimary,
                              fontWeight: FontWeight.bold,
                            ),
                      ),
                      const SizedBox(height: 8),
                      Text(
                        'Create an administrator account to get started',
                        style: Theme.of(context).textTheme.bodyMedium?.copyWith(
                              color: NvrColors.of(context).textSecondary,
                            ),
                        textAlign: TextAlign.center,
                      ),
                      const SizedBox(height: 32),
                      TextFormField(
                        controller: _usernameController,
                        keyboardType: TextInputType.text,
                        autocorrect: false,
                        textInputAction: TextInputAction.next,
                        style: TextStyle(color: NvrColors.of(context).textPrimary),
                        decoration: InputDecoration(
                          labelText: 'Username',
                          labelStyle: TextStyle(color: NvrColors.of(context).textSecondary),
                          filled: true,
                          fillColor: NvrColors.of(context).bgInput,
                          border: OutlineInputBorder(
                            borderRadius: BorderRadius.circular(8),
                            borderSide: BorderSide(color: NvrColors.of(context).border),
                          ),
                          enabledBorder: OutlineInputBorder(
                            borderRadius: BorderRadius.circular(8),
                            borderSide: BorderSide(color: NvrColors.of(context).border),
                          ),
                          focusedBorder: OutlineInputBorder(
                            borderRadius: BorderRadius.circular(8),
                            borderSide: BorderSide(color: NvrColors.of(context).accent, width: 2),
                          ),
                          prefixIcon: Icon(Icons.person_outline, color: NvrColors.of(context).textMuted),
                        ),
                        validator: (value) {
                          if (value == null || value.trim().isEmpty) {
                            return 'Please enter a username';
                          }
                          if (value.trim().length < 3) {
                            return 'Username must be at least 3 characters';
                          }
                          return null;
                        },
                      ),
                      const SizedBox(height: 16),
                      TextFormField(
                        controller: _passwordController,
                        obscureText: _obscurePassword,
                        textInputAction: TextInputAction.next,
                        style: TextStyle(color: NvrColors.of(context).textPrimary),
                        decoration: InputDecoration(
                          labelText: 'Password',
                          labelStyle: TextStyle(color: NvrColors.of(context).textSecondary),
                          filled: true,
                          fillColor: NvrColors.of(context).bgInput,
                          border: OutlineInputBorder(
                            borderRadius: BorderRadius.circular(8),
                            borderSide: BorderSide(color: NvrColors.of(context).border),
                          ),
                          enabledBorder: OutlineInputBorder(
                            borderRadius: BorderRadius.circular(8),
                            borderSide: BorderSide(color: NvrColors.of(context).border),
                          ),
                          focusedBorder: OutlineInputBorder(
                            borderRadius: BorderRadius.circular(8),
                            borderSide: BorderSide(color: NvrColors.of(context).accent, width: 2),
                          ),
                          prefixIcon: Icon(Icons.lock_outline, color: NvrColors.of(context).textMuted),
                          suffixIcon: IconButton(
                            icon: Icon(
                              _obscurePassword ? Icons.visibility_off : Icons.visibility,
                              color: NvrColors.of(context).textMuted,
                            ),
                            onPressed: () => setState(() => _obscurePassword = !_obscurePassword),
                          ),
                        ),
                        validator: (value) {
                          if (value == null || value.isEmpty) {
                            return 'Please enter a password';
                          }
                          if (value.length < 8) {
                            return 'Password must be at least 8 characters';
                          }
                          return null;
                        },
                      ),
                      const SizedBox(height: 16),
                      TextFormField(
                        controller: _confirmPasswordController,
                        obscureText: _obscureConfirm,
                        textInputAction: TextInputAction.done,
                        style: TextStyle(color: NvrColors.of(context).textPrimary),
                        decoration: InputDecoration(
                          labelText: 'Confirm Password',
                          labelStyle: TextStyle(color: NvrColors.of(context).textSecondary),
                          filled: true,
                          fillColor: NvrColors.of(context).bgInput,
                          border: OutlineInputBorder(
                            borderRadius: BorderRadius.circular(8),
                            borderSide: BorderSide(color: NvrColors.of(context).border),
                          ),
                          enabledBorder: OutlineInputBorder(
                            borderRadius: BorderRadius.circular(8),
                            borderSide: BorderSide(color: NvrColors.of(context).border),
                          ),
                          focusedBorder: OutlineInputBorder(
                            borderRadius: BorderRadius.circular(8),
                            borderSide: BorderSide(color: NvrColors.of(context).accent, width: 2),
                          ),
                          prefixIcon: Icon(Icons.lock_outline, color: NvrColors.of(context).textMuted),
                          suffixIcon: IconButton(
                            icon: Icon(
                              _obscureConfirm ? Icons.visibility_off : Icons.visibility,
                              color: NvrColors.of(context).textMuted,
                            ),
                            onPressed: () => setState(() => _obscureConfirm = !_obscureConfirm),
                          ),
                        ),
                        validator: (value) {
                          if (value == null || value.isEmpty) {
                            return 'Please confirm your password';
                          }
                          if (value != _passwordController.text) {
                            return 'Passwords do not match';
                          }
                          return null;
                        },
                        onFieldSubmitted: (_) => _isLoading ? null : _createAdmin(),
                      ),
                      if (_error != null) ...[
                        const SizedBox(height: 16),
                        Container(
                          padding: const EdgeInsets.all(12),
                          decoration: BoxDecoration(
                            color: NvrColors.of(context).danger.withAlpha(26),
                            borderRadius: BorderRadius.circular(8),
                            border: Border.all(color: NvrColors.of(context).danger.withAlpha(77)),
                          ),
                          child: Row(
                            children: [
                              Icon(Icons.error_outline, color: NvrColors.of(context).danger, size: 18),
                              const SizedBox(width: 8),
                              Expanded(
                                child: Text(
                                  _error!,
                                  style: TextStyle(color: NvrColors.of(context).danger, fontSize: 13),
                                ),
                              ),
                            ],
                          ),
                        ),
                      ],
                      if (_successMessage != null) ...[
                        const SizedBox(height: 16),
                        Container(
                          padding: const EdgeInsets.all(12),
                          decoration: BoxDecoration(
                            color: NvrColors.of(context).success.withAlpha(26),
                            borderRadius: BorderRadius.circular(8),
                            border: Border.all(color: NvrColors.of(context).success.withAlpha(77)),
                          ),
                          child: Row(
                            children: [
                              Icon(Icons.check_circle_outline, color: NvrColors.of(context).success, size: 18),
                              const SizedBox(width: 8),
                              Expanded(
                                child: Text(
                                  _successMessage!,
                                  style: TextStyle(color: NvrColors.of(context).success, fontSize: 13),
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
                          onPressed: _isLoading ? null : _createAdmin,
                          style: FilledButton.styleFrom(
                            backgroundColor: NvrColors.of(context).accent,
                            disabledBackgroundColor: NvrColors.of(context).accent.withAlpha(128),
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
                                  'Create Admin Account',
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
