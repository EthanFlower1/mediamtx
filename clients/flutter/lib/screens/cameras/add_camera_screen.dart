import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../providers/auth_provider.dart';
import '../../providers/cameras_provider.dart';
import '../../theme/nvr_colors.dart';
import '../../theme/nvr_typography.dart';
import '../../widgets/hud/hud_button.dart';
import '../../widgets/hud/segmented_control.dart';
import 'camera_detail_sheet.dart';
import 'discovery_card.dart';

class AddCameraScreen extends ConsumerStatefulWidget {
  const AddCameraScreen({super.key});

  @override
  ConsumerState<AddCameraScreen> createState() => _AddCameraScreenState();
}

class _AddCameraScreenState extends ConsumerState<AddCameraScreen> {
  int _selectedTab = 0;

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: NvrColors.bgPrimary,
      body: SafeArea(
        child: Column(
          children: [
            // ── Header ────────────────────────────────────────────────
            Padding(
              padding: const EdgeInsets.fromLTRB(8, 8, 16, 8),
              child: Row(
                children: [
                  IconButton(
                    icon: const Icon(Icons.arrow_back, color: NvrColors.textPrimary, size: 20),
                    onPressed: () => Navigator.of(context).pop(),
                  ),
                  const Expanded(
                    child: Text('Add Camera', style: NvrTypography.pageTitle),
                  ),
                ],
              ),
            ),

            const Divider(color: NvrColors.border, height: 1),

            // ── Tab selector ─────────────────────────────────────────
            Padding(
              padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 12),
              child: Align(
                alignment: Alignment.centerLeft,
                child: HudSegmentedControl<int>(
                  segments: const {0: 'DISCOVER', 1: 'MANUAL'},
                  selected: _selectedTab,
                  onChanged: (v) => setState(() => _selectedTab = v),
                ),
              ),
            ),

            // ── Tab body ─────────────────────────────────────────────
            Expanded(
              child: IndexedStack(
                index: _selectedTab,
                children: const [
                  _DiscoverTab(),
                  _ManualTab(),
                ],
              ),
            ),
          ],
        ),
      ),
    );
  }
}

// ---------------------------------------------------------------------------
// Discover tab
// ---------------------------------------------------------------------------
class _DiscoverTab extends ConsumerStatefulWidget {
  const _DiscoverTab();

  @override
  ConsumerState<_DiscoverTab> createState() => _DiscoverTabState();
}

class _DiscoverTabState extends ConsumerState<_DiscoverTab> {
  bool _discovering = false;
  bool _timedOut = false;
  List<Map<String, dynamic>> _results = [];
  String? _error;
  Timer? _pollTimer;
  Timer? _timeoutTimer;

  @override
  void dispose() {
    _pollTimer?.cancel();
    _timeoutTimer?.cancel();
    super.dispose();
  }

  Future<void> _startDiscovery() async {
    final api = ref.read(apiClientProvider);
    if (api == null) return;

    setState(() {
      _discovering = true;
      _timedOut = false;
      _results = [];
      _error = null;
    });

    try {
      await api.post('/cameras/discover');
    } catch (e) {
      setState(() {
        _error = 'Failed to start discovery: $e';
        _discovering = false;
      });
      return;
    }

    // Poll after 3 s, then every 3 s, 30 s total timeout
    _timeoutTimer = Timer(const Duration(seconds: 30), _onTimeout);
    _pollTimer = Timer(const Duration(seconds: 3), _pollResults);
  }

  void _onTimeout() {
    _pollTimer?.cancel();
    if (mounted) {
      setState(() {
        _discovering = false;
        _timedOut = true;
      });
    }
  }

  void _cancel() {
    _pollTimer?.cancel();
    _timeoutTimer?.cancel();
    setState(() {
      _discovering = false;
    });
  }

  Future<void> _pollResults() async {
    if (!mounted) return;
    final api = ref.read(apiClientProvider);
    if (api == null) return;

    try {
      final res = await api.get<dynamic>('/cameras/discover/results');
      final data = res.data as List<dynamic>? ?? [];
      if (mounted) {
        setState(() {
          _results = data.cast<Map<String, dynamic>>();
        });
      }
    } catch (_) {
      // swallow — keep polling
    }

    if (mounted && _discovering) {
      _pollTimer = Timer(const Duration(seconds: 3), _pollResults);
    }
  }

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.all(16),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          // ── Scan button or progress ──────────────────────────────
          if (!_discovering) ...[
            const Text(
              'Scan your local network for ONVIF-compatible cameras.',
              style: NvrTypography.body,
            ),
            const SizedBox(height: 16),
            HudButton(
              label: 'SCAN NETWORK',
              icon: Icons.radar,
              onPressed: _startDiscovery,
            ),
          ] else ...[
            Center(
              child: Column(
                children: [
                  SizedBox(
                    width: 56,
                    height: 56,
                    child: CircularProgressIndicator(
                      strokeWidth: 3,
                      color: NvrColors.accent,
                      backgroundColor: NvrColors.accent.withValues(alpha: 0.12),
                    ),
                  ),
                  const SizedBox(height: 12),
                  const Text(
                    'Scanning network...',
                    style: NvrTypography.monoLabel,
                  ),
                  const SizedBox(height: 8),
                  HudButton(
                    label: 'CANCEL',
                    style: HudButtonStyle.danger,
                    onPressed: _cancel,
                  ),
                ],
              ),
            ),
          ],

          if (_timedOut)
            Padding(
              padding: const EdgeInsets.only(top: 8),
              child: Text(
                'Discovery timed out after 30 seconds.',
                style: NvrTypography.monoLabel.copyWith(color: NvrColors.textSecondary),
                textAlign: TextAlign.center,
              ),
            ),

          if (_error != null)
            Padding(
              padding: const EdgeInsets.only(top: 8),
              child: Text(
                _error!,
                style: NvrTypography.body.copyWith(color: NvrColors.danger),
              ),
            ),

          const SizedBox(height: 16),

          // ── Results count header ─────────────────────────────────
          if (_results.isNotEmpty)
            Padding(
              padding: const EdgeInsets.only(bottom: 10),
              child: Text(
                '${_results.length} DEVICE${_results.length == 1 ? '' : 'S'} FOUND',
                style: NvrTypography.monoLabel,
              ),
            ),

          // ── Results ──────────────────────────────────────────────
          Expanded(
            child: _results.isEmpty
                ? Center(
                    child: Text(
                      _discovering
                          ? 'Looking for cameras...'
                          : 'No cameras found.\nTap "Scan Network" to start.',
                      style: NvrTypography.body,
                      textAlign: TextAlign.center,
                    ),
                  )
                : ListView.separated(
                    itemCount: _results.length,
                    separatorBuilder: (_, __) => const SizedBox(height: 8),
                    itemBuilder: (context, index) {
                      final device = _results[index];
                      return DiscoveryCard(
                        device: device,
                        onTap: () => CameraDetailSheet.show(context, device),
                      );
                    },
                  ),
          ),
        ],
      ),
    );
  }
}

// ---------------------------------------------------------------------------
// Manual tab
// ---------------------------------------------------------------------------
class _ManualTab extends ConsumerStatefulWidget {
  const _ManualTab();

  @override
  ConsumerState<_ManualTab> createState() => _ManualTabState();
}

class _ManualTabState extends ConsumerState<_ManualTab> {
  final _formKey = GlobalKey<FormState>();
  final _nameCtrl = TextEditingController();
  final _rtspCtrl = TextEditingController();
  final _onvifCtrl = TextEditingController();
  final _userCtrl = TextEditingController();
  final _passCtrl = TextEditingController();
  bool _saving = false;
  bool _testing = false;
  bool _obscurePass = true;

  @override
  void dispose() {
    _nameCtrl.dispose();
    _rtspCtrl.dispose();
    _onvifCtrl.dispose();
    _userCtrl.dispose();
    _passCtrl.dispose();
    super.dispose();
  }

  Future<void> _testConnection() async {
    final api = ref.read(apiClientProvider);
    if (api == null) return;
    setState(() => _testing = true);
    try {
      await api.post('/cameras/probe', data: {
        'endpoint': _onvifCtrl.text.trim(),
        'username': _userCtrl.text.trim(),
        'password': _passCtrl.text,
      });
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          const SnackBar(backgroundColor: NvrColors.success, content: Text('Connection successful')),
        );
      }
    } catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(backgroundColor: NvrColors.danger, content: Text('Connection failed: $e')),
        );
      }
    } finally {
      if (mounted) setState(() => _testing = false);
    }
  }

  Future<void> _submit() async {
    if (!(_formKey.currentState?.validate() ?? false)) return;
    final api = ref.read(apiClientProvider);
    if (api == null) return;

    setState(() => _saving = true);
    try {
      await api.post('/cameras', data: {
        'name': _nameCtrl.text.trim(),
        'rtsp_url': _rtspCtrl.text.trim(),
        'username': _userCtrl.text.trim(),
        'password': _passCtrl.text,
      });
      ref.invalidate(camerasProvider);
      if (mounted) {
        Navigator.of(context).pop();
      }
    } catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(
            backgroundColor: NvrColors.danger,
            content: Text('Failed to add camera: $e'),
          ),
        );
      }
    } finally {
      if (mounted) setState(() => _saving = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    return SingleChildScrollView(
      padding: const EdgeInsets.all(16),
      child: Form(
        key: _formKey,
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            _buildField(
              controller: _nameCtrl,
              label: 'CAMERA NAME',
              hint: 'e.g. Front Door',
              validator: (v) => v == null || v.trim().isEmpty ? 'Name is required' : null,
            ),
            const SizedBox(height: 12),
            _buildField(
              controller: _rtspCtrl,
              label: 'RTSP URL',
              hint: 'rtsp://192.168.1.100:554/stream',
              keyboardType: TextInputType.url,
              validator: (v) {
                if (v == null || v.trim().isEmpty) return 'RTSP URL is required';
                if (!v.trim().startsWith('rtsp://')) return 'Must start with rtsp://';
                return null;
              },
            ),
            const SizedBox(height: 12),
            _buildField(
              controller: _onvifCtrl,
              label: 'ONVIF ENDPOINT',
              hint: 'http://192.168.1.100/onvif/device_service',
              keyboardType: TextInputType.url,
            ),
            const SizedBox(height: 12),
            _buildField(
              controller: _userCtrl,
              label: 'USERNAME',
              hint: 'admin',
            ),
            const SizedBox(height: 12),
            _buildPasswordField(),
            const SizedBox(height: 20),

            // Test Connection
            HudButton(
              label: _testing ? 'TESTING...' : 'TEST CONNECTION',
              style: HudButtonStyle.secondary,
              icon: Icons.wifi_find,
              onPressed: _testing ? null : _testConnection,
            ),

            const SizedBox(height: 10),

            // Add Camera
            HudButton(
              label: _saving ? 'ADDING...' : 'ADD CAMERA',
              icon: Icons.add,
              onPressed: _saving ? null : _submit,
            ),

            const SizedBox(height: 24),
          ],
        ),
      ),
    );
  }

  Widget _buildField({
    required TextEditingController controller,
    required String label,
    String? hint,
    TextInputType? keyboardType,
    String? Function(String?)? validator,
  }) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(label, style: NvrTypography.monoLabel),
        const SizedBox(height: 5),
        TextFormField(
          controller: controller,
          keyboardType: keyboardType,
          style: const TextStyle(
            color: NvrColors.textPrimary,
            fontFamily: 'JetBrainsMono',
            fontSize: 12,
          ),
          validator: validator,
          decoration: _inputDecoration(hint: hint),
        ),
      ],
    );
  }

  Widget _buildPasswordField() {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        const Text('PASSWORD', style: NvrTypography.monoLabel),
        const SizedBox(height: 5),
        TextFormField(
          controller: _passCtrl,
          obscureText: _obscurePass,
          style: const TextStyle(
            color: NvrColors.textPrimary,
            fontFamily: 'JetBrainsMono',
            fontSize: 12,
          ),
          decoration: _inputDecoration(hint: '••••••••').copyWith(
            suffixIcon: IconButton(
              icon: Icon(
                _obscurePass ? Icons.visibility_off : Icons.visibility,
                color: NvrColors.textMuted,
                size: 18,
              ),
              onPressed: () => setState(() => _obscurePass = !_obscurePass),
            ),
          ),
        ),
      ],
    );
  }

  InputDecoration _inputDecoration({String? hint}) {
    return InputDecoration(
      hintText: hint,
      hintStyle: const TextStyle(color: NvrColors.textMuted),
      filled: true,
      fillColor: NvrColors.bgInput,
      contentPadding: const EdgeInsets.symmetric(horizontal: 10, vertical: 10),
      border: OutlineInputBorder(
        borderRadius: BorderRadius.circular(6),
        borderSide: const BorderSide(color: NvrColors.border),
      ),
      enabledBorder: OutlineInputBorder(
        borderRadius: BorderRadius.circular(6),
        borderSide: const BorderSide(color: NvrColors.border),
      ),
      focusedBorder: OutlineInputBorder(
        borderRadius: BorderRadius.circular(6),
        borderSide: const BorderSide(color: NvrColors.accent),
      ),
      errorBorder: OutlineInputBorder(
        borderRadius: BorderRadius.circular(6),
        borderSide: const BorderSide(color: NvrColors.danger),
      ),
    );
  }
}
