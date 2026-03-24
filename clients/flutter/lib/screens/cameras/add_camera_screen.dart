import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../providers/auth_provider.dart';
import '../../providers/cameras_provider.dart';
import '../../theme/nvr_colors.dart';

class AddCameraScreen extends ConsumerStatefulWidget {
  const AddCameraScreen({super.key});

  @override
  ConsumerState<AddCameraScreen> createState() => _AddCameraScreenState();
}

class _AddCameraScreenState extends ConsumerState<AddCameraScreen>
    with SingleTickerProviderStateMixin {
  late final TabController _tabController;

  @override
  void initState() {
    super.initState();
    _tabController = TabController(length: 2, vsync: this);
  }

  @override
  void dispose() {
    _tabController.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: NvrColors.bgPrimary,
      appBar: AppBar(
        backgroundColor: NvrColors.bgSecondary,
        title: const Text(
          'Add Camera',
          style: TextStyle(color: NvrColors.textPrimary),
        ),
        iconTheme: const IconThemeData(color: NvrColors.textPrimary),
        bottom: TabBar(
          controller: _tabController,
          indicatorColor: NvrColors.accent,
          labelColor: NvrColors.accent,
          unselectedLabelColor: NvrColors.textMuted,
          tabs: const [
            Tab(text: 'Discover'),
            Tab(text: 'Manual'),
          ],
        ),
      ),
      body: TabBarView(
        controller: _tabController,
        children: const [
          _DiscoverTab(),
          _ManualTab(),
        ],
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

  Future<void> _addDiscovered(Map<String, dynamic> cam) async {
    final api = ref.read(apiClientProvider);
    if (api == null) return;

    try {
      await api.post('/cameras', data: {
        'name': cam['name'] ?? cam['ip'] ?? 'Camera',
        'rtsp_url': cam['rtsp_url'] ?? '',
        'onvif_endpoint': cam['onvif_endpoint'] ?? '',
      });
      ref.invalidate(camerasProvider);
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          const SnackBar(
            backgroundColor: NvrColors.success,
            content: Text('Camera added successfully'),
          ),
        );
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
    }
  }

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.all(16),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          const Text(
            'Scan your local network for ONVIF-compatible cameras.',
            style: TextStyle(color: NvrColors.textSecondary, fontSize: 13),
          ),
          const SizedBox(height: 16),
          if (!_discovering)
            ElevatedButton.icon(
              style: ElevatedButton.styleFrom(
                backgroundColor: NvrColors.accent,
                foregroundColor: Colors.white,
                padding: const EdgeInsets.symmetric(vertical: 14),
              ),
              onPressed: _startDiscovery,
              icon: const Icon(Icons.search),
              label: const Text('Start Discovery'),
            )
          else
            Column(
              children: [
                const LinearProgressIndicator(color: NvrColors.accent),
                const SizedBox(height: 8),
                Row(
                  mainAxisAlignment: MainAxisAlignment.center,
                  children: [
                    const Text(
                      'Scanning network…',
                      style: TextStyle(color: NvrColors.textSecondary, fontSize: 13),
                    ),
                    const SizedBox(width: 12),
                    TextButton(
                      onPressed: _cancel,
                      child: const Text('Cancel', style: TextStyle(color: NvrColors.danger)),
                    ),
                  ],
                ),
              ],
            ),
          if (_timedOut)
            const Padding(
              padding: EdgeInsets.only(top: 8),
              child: Text(
                'Discovery timed out after 30 seconds.',
                style: TextStyle(color: NvrColors.textMuted, fontSize: 12),
                textAlign: TextAlign.center,
              ),
            ),
          if (_error != null)
            Padding(
              padding: const EdgeInsets.only(top: 8),
              child: Text(
                _error!,
                style: const TextStyle(color: NvrColors.danger, fontSize: 12),
              ),
            ),
          const SizedBox(height: 16),
          Expanded(
            child: _results.isEmpty
                ? Center(
                    child: Text(
                      _discovering
                          ? 'Looking for cameras…'
                          : 'No cameras found. Tap "Start Discovery" to scan.',
                      style: const TextStyle(color: NvrColors.textMuted, fontSize: 13),
                      textAlign: TextAlign.center,
                    ),
                  )
                : ListView.separated(
                    itemCount: _results.length,
                    separatorBuilder: (_, __) =>
                        const Divider(color: NvrColors.border, height: 1),
                    itemBuilder: (context, index) {
                      final cam = _results[index];
                      return ListTile(
                        tileColor: NvrColors.bgSecondary,
                        leading: const CircleAvatar(
                          backgroundColor: NvrColors.bgTertiary,
                          child: Icon(Icons.videocam, color: NvrColors.accent, size: 20),
                        ),
                        title: Text(
                          cam['name'] as String? ?? cam['ip'] as String? ?? 'Unknown',
                          style: const TextStyle(color: NvrColors.textPrimary),
                        ),
                        subtitle: Text(
                          cam['ip'] as String? ?? '',
                          style: const TextStyle(color: NvrColors.textMuted, fontSize: 12),
                        ),
                        trailing: TextButton(
                          onPressed: () => _addDiscovered(cam),
                          child: const Text('Add', style: TextStyle(color: NvrColors.accent)),
                        ),
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
  final _userCtrl = TextEditingController();
  final _passCtrl = TextEditingController();
  bool _saving = false;
  bool _obscurePass = true;

  @override
  void dispose() {
    _nameCtrl.dispose();
    _rtspCtrl.dispose();
    _userCtrl.dispose();
    _passCtrl.dispose();
    super.dispose();
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
              label: 'Camera Name',
              hint: 'e.g. Front Door',
              validator: (v) => v == null || v.trim().isEmpty ? 'Name is required' : null,
            ),
            const SizedBox(height: 16),
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
            const SizedBox(height: 16),
            _buildField(
              controller: _userCtrl,
              label: 'Username (optional)',
              hint: 'admin',
            ),
            const SizedBox(height: 16),
            TextFormField(
              controller: _passCtrl,
              obscureText: _obscurePass,
              style: const TextStyle(color: NvrColors.textPrimary),
              decoration: InputDecoration(
                labelText: 'Password (optional)',
                labelStyle: const TextStyle(color: NvrColors.textMuted),
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
                  borderSide: const BorderSide(color: NvrColors.accent),
                ),
                suffixIcon: IconButton(
                  icon: Icon(
                    _obscurePass ? Icons.visibility_off : Icons.visibility,
                    color: NvrColors.textMuted,
                  ),
                  onPressed: () => setState(() => _obscurePass = !_obscurePass),
                ),
              ),
            ),
            const SizedBox(height: 24),
            ElevatedButton(
              style: ElevatedButton.styleFrom(
                backgroundColor: NvrColors.accent,
                foregroundColor: Colors.white,
                padding: const EdgeInsets.symmetric(vertical: 14),
              ),
              onPressed: _saving ? null : _submit,
              child: _saving
                  ? const SizedBox(
                      height: 18,
                      width: 18,
                      child: CircularProgressIndicator(strokeWidth: 2, color: Colors.white),
                    )
                  : const Text('Add Camera'),
            ),
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
    return TextFormField(
      controller: controller,
      keyboardType: keyboardType,
      style: const TextStyle(color: NvrColors.textPrimary),
      validator: validator,
      decoration: InputDecoration(
        labelText: label,
        hintText: hint,
        labelStyle: const TextStyle(color: NvrColors.textMuted),
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
          borderSide: const BorderSide(color: NvrColors.accent),
        ),
        errorBorder: OutlineInputBorder(
          borderRadius: BorderRadius.circular(8),
          borderSide: const BorderSide(color: NvrColors.danger),
        ),
      ),
    );
  }
}
