import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../models/camera.dart';
import '../../providers/auth_provider.dart';
import '../../theme/nvr_colors.dart';
import 'recording_rules_screen.dart';
import 'zone_editor_screen.dart';

class CameraDetailScreen extends ConsumerStatefulWidget {
  final String cameraId;

  const CameraDetailScreen({super.key, required this.cameraId});

  @override
  ConsumerState<CameraDetailScreen> createState() => _CameraDetailScreenState();
}

class _CameraDetailScreenState extends ConsumerState<CameraDetailScreen>
    with SingleTickerProviderStateMixin {
  late final TabController _tabController;

  Camera? _camera;
  bool _loading = true;
  String? _error;

  @override
  void initState() {
    super.initState();
    _tabController = TabController(length: 5, vsync: this);
    _fetchCamera();
  }

  @override
  void dispose() {
    _tabController.dispose();
    super.dispose();
  }

  Future<void> _fetchCamera() async {
    setState(() {
      _loading = true;
      _error = null;
    });
    final api = ref.read(apiClientProvider);
    if (api == null) return;
    try {
      final res = await api.get<dynamic>('/cameras/${widget.cameraId}');
      setState(() {
        _camera = Camera.fromJson(res.data as Map<String, dynamic>);
        _loading = false;
      });
    } catch (e) {
      setState(() {
        _error = e.toString();
        _loading = false;
      });
    }
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: NvrColors.bgPrimary,
      appBar: AppBar(
        backgroundColor: NvrColors.bgSecondary,
        iconTheme: const IconThemeData(color: NvrColors.textPrimary),
        title: Text(
          _camera?.name ?? 'Camera',
          style: const TextStyle(color: NvrColors.textPrimary),
        ),
        bottom: _loading || _error != null
            ? null
            : TabBar(
                controller: _tabController,
                indicatorColor: NvrColors.accent,
                labelColor: NvrColors.accent,
                unselectedLabelColor: NvrColors.textMuted,
                isScrollable: true,
                tabs: const [
                  Tab(text: 'General'),
                  Tab(text: 'Recording'),
                  Tab(text: 'AI'),
                  Tab(text: 'Zones'),
                  Tab(text: 'Advanced'),
                ],
              ),
      ),
      body: _loading
          ? const Center(child: CircularProgressIndicator(color: NvrColors.accent))
          : _error != null
              ? Center(
                  child: Column(
                    mainAxisSize: MainAxisSize.min,
                    children: [
                      Text(_error!, style: const TextStyle(color: NvrColors.danger)),
                      TextButton(onPressed: _fetchCamera, child: const Text('Retry')),
                    ],
                  ),
                )
              : TabBarView(
                  controller: _tabController,
                  children: [
                    _GeneralTab(camera: _camera!, onRefresh: _fetchCamera),
                    RecordingRulesScreen(cameraId: widget.cameraId),
                    _AiTab(camera: _camera!, onRefresh: _fetchCamera),
                    ZoneEditorScreen(cameraId: widget.cameraId),
                    _AdvancedTab(camera: _camera!, onRefresh: _fetchCamera),
                  ],
                ),
    );
  }
}

// ---------------------------------------------------------------------------
// General tab
// ---------------------------------------------------------------------------
class _GeneralTab extends ConsumerStatefulWidget {
  final Camera camera;
  final VoidCallback onRefresh;

  const _GeneralTab({required this.camera, required this.onRefresh});

  @override
  ConsumerState<_GeneralTab> createState() => _GeneralTabState();
}

class _GeneralTabState extends ConsumerState<_GeneralTab> {
  final _formKey = GlobalKey<FormState>();
  late final TextEditingController _nameCtrl;
  late final TextEditingController _rtspCtrl;
  late final TextEditingController _onvifCtrl;
  bool _saving = false;

  @override
  void initState() {
    super.initState();
    _nameCtrl = TextEditingController(text: widget.camera.name);
    _rtspCtrl = TextEditingController(text: widget.camera.rtspUrl);
    _onvifCtrl = TextEditingController(text: widget.camera.onvifEndpoint);
  }

  @override
  void dispose() {
    _nameCtrl.dispose();
    _rtspCtrl.dispose();
    _onvifCtrl.dispose();
    super.dispose();
  }

  Future<void> _save() async {
    if (!(_formKey.currentState?.validate() ?? false)) return;
    final api = ref.read(apiClientProvider);
    if (api == null) return;
    setState(() => _saving = true);
    try {
      await api.put('/cameras/${widget.camera.id}', data: {
        'name': _nameCtrl.text.trim(),
        'rtsp_url': _rtspCtrl.text.trim(),
        'onvif_endpoint': _onvifCtrl.text.trim(),
      });
      widget.onRefresh();
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          const SnackBar(backgroundColor: NvrColors.success, content: Text('Saved')),
        );
      }
    } catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(backgroundColor: NvrColors.danger, content: Text('Error: $e')),
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
            _NvrField(controller: _nameCtrl, label: 'Name', validator: (v) {
              if (v == null || v.trim().isEmpty) return 'Required';
              return null;
            }),
            const SizedBox(height: 16),
            _NvrField(
              controller: _rtspCtrl,
              label: 'RTSP URL',
              keyboardType: TextInputType.url,
              validator: (v) {
                if (v == null || v.trim().isEmpty) return 'Required';
                if (!v.trim().startsWith('rtsp://')) return 'Must start with rtsp://';
                return null;
              },
            ),
            const SizedBox(height: 16),
            _NvrField(
              controller: _onvifCtrl,
              label: 'ONVIF Endpoint (optional)',
              hint: 'http://192.168.1.100/onvif/device_service',
              keyboardType: TextInputType.url,
            ),
            const SizedBox(height: 24),
            ElevatedButton(
              style: ElevatedButton.styleFrom(
                backgroundColor: NvrColors.accent,
                foregroundColor: Colors.white,
                padding: const EdgeInsets.symmetric(vertical: 14),
              ),
              onPressed: _saving ? null : _save,
              child: _saving
                  ? const SizedBox(
                      height: 18,
                      width: 18,
                      child: CircularProgressIndicator(strokeWidth: 2, color: Colors.white),
                    )
                  : const Text('Save'),
            ),
          ],
        ),
      ),
    );
  }
}

// ---------------------------------------------------------------------------
// AI tab
// ---------------------------------------------------------------------------
class _AiTab extends ConsumerStatefulWidget {
  final Camera camera;
  final VoidCallback onRefresh;

  const _AiTab({required this.camera, required this.onRefresh});

  @override
  ConsumerState<_AiTab> createState() => _AiTabState();
}

class _AiTabState extends ConsumerState<_AiTab> {
  late bool _aiEnabled;
  late final TextEditingController _subStreamCtrl;
  late double _confidence;
  bool _saving = false;

  @override
  void initState() {
    super.initState();
    _aiEnabled = widget.camera.aiEnabled;
    _subStreamCtrl = TextEditingController(text: widget.camera.subStreamUrl);
    _confidence = 50.0; // default — server should return this field eventually
  }

  @override
  void dispose() {
    _subStreamCtrl.dispose();
    super.dispose();
  }

  Future<void> _save() async {
    final api = ref.read(apiClientProvider);
    if (api == null) return;
    setState(() => _saving = true);
    try {
      await api.put('/cameras/${widget.camera.id}/ai', data: {
        'ai_enabled': _aiEnabled,
        'sub_stream_url': _subStreamCtrl.text.trim(),
        'confidence': _confidence / 100.0,
      });
      widget.onRefresh();
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          const SnackBar(backgroundColor: NvrColors.success, content: Text('AI settings saved')),
        );
      }
    } catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(backgroundColor: NvrColors.danger, content: Text('Error: $e')),
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
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Row(
            children: [
              const Expanded(
                child: Text(
                  'Enable AI detection',
                  style: TextStyle(color: NvrColors.textPrimary, fontSize: 15),
                ),
              ),
              Switch(
                value: _aiEnabled,
                onChanged: (v) => setState(() => _aiEnabled = v),
                activeThumbColor: NvrColors.accent,
              ),
            ],
          ),
          const Divider(color: NvrColors.border),
          const SizedBox(height: 8),
          _NvrField(
            controller: _subStreamCtrl,
            label: 'Sub-stream URL (for AI)',
            hint: 'rtsp://... (lower resolution stream)',
            keyboardType: TextInputType.url,
          ),
          const SizedBox(height: 20),
          Row(
            children: [
              const SizedBox(
                width: 130,
                child: Text(
                  'Confidence threshold',
                  style: TextStyle(color: NvrColors.textSecondary, fontSize: 13),
                ),
              ),
              Expanded(
                child: Slider(
                  value: _confidence,
                  min: 20,
                  max: 90,
                  divisions: 14,
                  activeColor: NvrColors.accent,
                  inactiveColor: NvrColors.bgTertiary,
                  onChanged: (v) => setState(() => _confidence = v),
                ),
              ),
              SizedBox(
                width: 40,
                child: Text(
                  '${_confidence.round()}%',
                  style: const TextStyle(color: NvrColors.textPrimary, fontSize: 12),
                  textAlign: TextAlign.right,
                ),
              ),
            ],
          ),
          const SizedBox(height: 24),
          ElevatedButton(
            style: ElevatedButton.styleFrom(
              backgroundColor: NvrColors.accent,
              foregroundColor: Colors.white,
              padding: const EdgeInsets.symmetric(vertical: 14),
            ),
            onPressed: _saving ? null : _save,
            child: _saving
                ? const SizedBox(
                    height: 18,
                    width: 18,
                    child: CircularProgressIndicator(strokeWidth: 2, color: Colors.white),
                  )
                : const Text('Save AI Settings'),
          ),
        ],
      ),
    );
  }
}

// ---------------------------------------------------------------------------
// Advanced tab
// ---------------------------------------------------------------------------
class _AdvancedTab extends ConsumerStatefulWidget {
  final Camera camera;
  final VoidCallback onRefresh;

  const _AdvancedTab({required this.camera, required this.onRefresh});

  @override
  ConsumerState<_AdvancedTab> createState() => _AdvancedTabState();
}

class _AdvancedTabState extends ConsumerState<_AdvancedTab> {
  late double _motionTimeout;
  late final TextEditingController _retentionCtrl;
  bool _saving = false;

  @override
  void initState() {
    super.initState();
    _motionTimeout = widget.camera.motionTimeoutSeconds.toDouble();
    _retentionCtrl =
        TextEditingController(text: widget.camera.retentionDays.toString());
  }

  @override
  void dispose() {
    _retentionCtrl.dispose();
    super.dispose();
  }

  Future<void> _save() async {
    final api = ref.read(apiClientProvider);
    if (api == null) return;
    final retention = int.tryParse(_retentionCtrl.text.trim()) ?? 30;
    setState(() => _saving = true);
    try {
      await api.put('/cameras/${widget.camera.id}', data: {
        'motion_timeout_seconds': _motionTimeout.round(),
        'retention_days': retention,
      });
      widget.onRefresh();
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          const SnackBar(backgroundColor: NvrColors.success, content: Text('Saved')),
        );
      }
    } catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(backgroundColor: NvrColors.danger, content: Text('Error: $e')),
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
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Row(
            children: [
              const SizedBox(
                width: 130,
                child: Text(
                  'Motion timeout',
                  style: TextStyle(color: NvrColors.textSecondary, fontSize: 13),
                ),
              ),
              Expanded(
                child: Slider(
                  value: _motionTimeout,
                  min: 1,
                  max: 60,
                  divisions: 59,
                  activeColor: NvrColors.accent,
                  inactiveColor: NvrColors.bgTertiary,
                  onChanged: (v) => setState(() => _motionTimeout = v),
                ),
              ),
              SizedBox(
                width: 40,
                child: Text(
                  '${_motionTimeout.round()}s',
                  style: const TextStyle(color: NvrColors.textPrimary, fontSize: 12),
                  textAlign: TextAlign.right,
                ),
              ),
            ],
          ),
          const SizedBox(height: 16),
          _NvrField(
            controller: _retentionCtrl,
            label: 'Retention (days)',
            keyboardType: TextInputType.number,
            validator: (v) {
              final n = int.tryParse(v ?? '');
              if (n == null || n < 1) return 'Enter a valid number of days';
              return null;
            },
          ),
          const SizedBox(height: 24),
          ElevatedButton(
            style: ElevatedButton.styleFrom(
              backgroundColor: NvrColors.accent,
              foregroundColor: Colors.white,
              padding: const EdgeInsets.symmetric(vertical: 14),
            ),
            onPressed: _saving ? null : _save,
            child: _saving
                ? const SizedBox(
                    height: 18,
                    width: 18,
                    child: CircularProgressIndicator(strokeWidth: 2, color: Colors.white),
                  )
                : const Text('Save'),
          ),
        ],
      ),
    );
  }
}

// ---------------------------------------------------------------------------
// Shared form field widget
// ---------------------------------------------------------------------------
class _NvrField extends StatelessWidget {
  final TextEditingController controller;
  final String label;
  final String? hint;
  final TextInputType? keyboardType;
  final String? Function(String?)? validator;

  const _NvrField({
    required this.controller,
    required this.label,
    this.hint,
    this.keyboardType,
    this.validator,
  });

  @override
  Widget build(BuildContext context) {
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
