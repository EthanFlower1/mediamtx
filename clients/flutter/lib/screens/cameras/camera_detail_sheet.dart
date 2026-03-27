import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../providers/auth_provider.dart';
import '../../providers/cameras_provider.dart';
import '../../theme/nvr_colors.dart';
import '../../theme/nvr_typography.dart';
import '../../widgets/hud/hud_button.dart';

/// Extracts a bare IP address (and optional port) from an ONVIF xaddr URL.
String _ipFromXaddr(String xaddr) {
  try {
    final uri = Uri.parse(xaddr);
    return uri.host;
  } catch (_) {
    return xaddr;
  }
}

/// Injects [username] and [password] into an RTSP URL if they are non-empty.
/// e.g. rtsp://192.168.1.10/stream → rtsp://admin:pass@192.168.1.10/stream
String _injectCredentials(String rtsp, String user, String pass) {
  if (user.isEmpty && pass.isEmpty) return rtsp;
  try {
    final uri = Uri.parse(rtsp);
    return uri
        .replace(
          userInfo: pass.isNotEmpty ? '$user:$pass' : user,
        )
        .toString();
  } catch (_) {
    return rtsp;
  }
}

/// A small capability chip.
class _CapBadge extends StatelessWidget {
  const _CapBadge({required this.label, required this.supported});

  final String label;
  final bool supported;

  @override
  Widget build(BuildContext context) {
    final color = supported ? NvrColors.accent : NvrColors.textMuted;
    return Container(
      margin: const EdgeInsets.only(right: 6, bottom: 6),
      padding: const EdgeInsets.symmetric(horizontal: 7, vertical: 3),
      decoration: BoxDecoration(
        color: color.withValues(alpha: 0.1),
        border: Border.all(color: color.withValues(alpha: 0.3)),
        borderRadius: BorderRadius.circular(3),
      ),
      child: Text(
        label,
        style: TextStyle(
          fontFamily: 'JetBrainsMono',
          fontSize: 8,
          fontWeight: FontWeight.w700,
          letterSpacing: 0.8,
          color: color,
        ),
      ),
    );
  }
}

/// Status badge pill (no dot).
class _StatusPill extends StatelessWidget {
  const _StatusPill({required this.label, required this.color});

  final String label;
  final Color color;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 7, vertical: 3),
      decoration: BoxDecoration(
        color: color.withValues(alpha: 0.1),
        border: Border.all(color: color.withValues(alpha: 0.3)),
        borderRadius: BorderRadius.circular(3),
      ),
      child: Text(
        label,
        style: TextStyle(
          fontFamily: 'JetBrainsMono',
          fontSize: 8,
          fontWeight: FontWeight.w700,
          letterSpacing: 1.0,
          color: color,
        ),
      ),
    );
  }
}

// ---------------------------------------------------------------------------
// CameraDetailSheet
// ---------------------------------------------------------------------------

class CameraDetailSheet extends ConsumerStatefulWidget {
  const CameraDetailSheet({super.key, required this.device});

  final Map<String, dynamic> device;

  /// Opens the sheet as a modal bottom sheet.
  static void show(BuildContext context, Map<String, dynamic> device) {
    showModalBottomSheet(
      context: context,
      isScrollControlled: true,
      backgroundColor: NvrColors.bgSecondary,
      shape: const RoundedRectangleBorder(
        borderRadius: BorderRadius.vertical(top: Radius.circular(16)),
      ),
      builder: (ctx) {
        return DraggableScrollableSheet(
          expand: false,
          initialChildSize: 0.85,
          maxChildSize: 0.95,
          minChildSize: 0.5,
          builder: (_, controller) => CameraDetailSheet(device: device),
        );
      },
    );
  }

  @override
  ConsumerState<CameraDetailSheet> createState() => _CameraDetailSheetState();
}

class _CameraDetailSheetState extends ConsumerState<CameraDetailSheet> {
  final _userCtrl = TextEditingController();
  final _passCtrl = TextEditingController();
  final _nameCtrl = TextEditingController();

  bool _probing = false;
  String? _probeError;
  List<Map<String, dynamic>> _profiles = [];
  Map<String, dynamic>? _capabilities;
  int? _selectedProfileIndex;
  bool _adding = false;
  bool _obscurePass = true;

  Map<String, dynamic> get _device => widget.device;

  @override
  void initState() {
    super.initState();
    final manufacturer = _device['manufacturer'] as String? ?? '';
    final model = _device['model'] as String? ?? '';
    if (manufacturer.isNotEmpty && model.isNotEmpty) {
      _nameCtrl.text = '$manufacturer $model';
    } else if (manufacturer.isNotEmpty) {
      _nameCtrl.text = manufacturer;
    } else if (model.isNotEmpty) {
      _nameCtrl.text = model;
    }

    // Pre-populate profiles if the backend already returned them (open cameras).
    final existingProfiles = _device['profiles'] as List<dynamic>?;
    if (existingProfiles != null && existingProfiles.isNotEmpty) {
      _profiles = existingProfiles.cast<Map<String, dynamic>>();
    }
  }

  @override
  void dispose() {
    _userCtrl.dispose();
    _passCtrl.dispose();
    _nameCtrl.dispose();
    super.dispose();
  }

  // ── Probe ─────────────────────────────────────────────────────────────────

  Future<void> _probe() async {
    final api = ref.read(apiClientProvider);
    if (api == null) return;

    setState(() {
      _probing = true;
      _probeError = null;
      _profiles = [];
      _capabilities = null;
      _selectedProfileIndex = null;
    });

    try {
      final res = await api.post<dynamic>('/cameras/probe', data: {
        'xaddr': _device['xaddr'] ?? '',
        'username': _userCtrl.text.trim(),
        'password': _passCtrl.text,
      });
      final data = res.data as Map<String, dynamic>? ?? {};
      final profiles = (data['profiles'] as List<dynamic>? ?? []).cast<Map<String, dynamic>>();
      final caps = data['capabilities'] as Map<String, dynamic>?;
      setState(() {
        _profiles = profiles;
        _capabilities = caps;
        if (profiles.isNotEmpty) _selectedProfileIndex = 0;
      });
    } catch (e) {
      setState(() => _probeError = e.toString());
    } finally {
      if (mounted) setState(() => _probing = false);
    }
  }

  // ── Add camera ────────────────────────────────────────────────────────────

  Future<void> _addCamera() async {
    if (_selectedProfileIndex == null) return;
    final api = ref.read(apiClientProvider);
    if (api == null) return;

    final profile = _profiles[_selectedProfileIndex!];
    final rawRtsp = profile['rtsp_uri'] as String? ?? profile['rtsp_url'] as String? ?? '';
    final rtspUrl = _injectCredentials(
      rawRtsp,
      _userCtrl.text.trim(),
      _passCtrl.text,
    );

    setState(() => _adding = true);
    try {
      await api.post('/cameras', data: {
        'name': _nameCtrl.text.trim(),
        'rtsp_url': rtspUrl,
        'onvif_endpoint': _device['xaddr'] ?? '',
        'onvif_username': _userCtrl.text.trim(),
        'onvif_password': _passCtrl.text,
        'onvif_profile_token': profile['token'] ?? profile['name'] ?? '',
      });
      ref.invalidate(camerasProvider);
      if (mounted) {
        // Pop the sheet, then the add camera screen.
        Navigator.of(context).pop();
        Navigator.of(context).pop();
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
    } finally {
      if (mounted) setState(() => _adding = false);
    }
  }

  // ── Helpers ───────────────────────────────────────────────────────────────

  InputDecoration _inputDecoration({String? hint, Widget? suffix}) {
    return InputDecoration(
      hintText: hint,
      hintStyle: const TextStyle(color: NvrColors.textMuted),
      filled: true,
      fillColor: NvrColors.bgPrimary,
      contentPadding: const EdgeInsets.symmetric(horizontal: 10, vertical: 10),
      suffixIcon: suffix,
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
    );
  }

  Widget _sectionLabel(String text) {
    return Padding(
      padding: const EdgeInsets.only(bottom: 8),
      child: Text(text, style: NvrTypography.monoSection),
    );
  }

  // ── Build ─────────────────────────────────────────────────────────────────

  @override
  Widget build(BuildContext context) {
    final manufacturer = _device['manufacturer'] as String? ?? '';
    final model = _device['model'] as String? ?? '';
    final firmware = _device['firmware_version'] as String? ?? '';
    final xaddr = _device['xaddr'] as String? ?? '';
    final authRequired = _device['auth_required'] as bool? ?? false;
    final alreadyAdded = (_device['existing_camera_id'] as String?)?.isNotEmpty ?? false;
    final ip = _ipFromXaddr(xaddr);

    final subtitleParts = <String>[
      if (ip.isNotEmpty) ip,
      if (manufacturer.isNotEmpty) manufacturer,
      if (firmware.isNotEmpty) firmware,
    ];

    final Widget statusBadge;
    if (alreadyAdded) {
      statusBadge = const _StatusPill(label: 'ADDED', color: NvrColors.accent);
    } else if (authRequired) {
      statusBadge = const _StatusPill(label: 'AUTH REQUIRED', color: NvrColors.danger);
    } else {
      statusBadge = const _StatusPill(label: 'OPEN', color: NvrColors.success);
    }

    final canAdd = _selectedProfileIndex != null && !_adding;

    return Container(
      decoration: const BoxDecoration(
        color: NvrColors.bgSecondary,
        borderRadius: BorderRadius.vertical(top: Radius.circular(16)),
      ),
      child: ListView(
        padding: const EdgeInsets.fromLTRB(16, 0, 16, 32),
        children: [
          // ── Drag handle ─────────────────────────────────────────────────
          Center(
            child: Padding(
              padding: const EdgeInsets.symmetric(vertical: 12),
              child: Container(
                width: 40,
                height: 4,
                decoration: BoxDecoration(
                  color: NvrColors.bgTertiary,
                  borderRadius: BorderRadius.circular(2),
                ),
              ),
            ),
          ),

          // ── Header ──────────────────────────────────────────────────────
          Row(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      _nameCtrl.text.isNotEmpty
                          ? _nameCtrl.text
                          : (model.isNotEmpty ? model : 'Unknown Camera'),
                      style: NvrTypography.pageTitle,
                    ),
                    if (subtitleParts.isNotEmpty) ...[
                      const SizedBox(height: 4),
                      Text(
                        subtitleParts.join(' · '),
                        style: NvrTypography.monoLabel.copyWith(
                          color: NvrColors.textSecondary,
                        ),
                      ),
                    ],
                  ],
                ),
              ),
              const SizedBox(width: 12),
              statusBadge,
            ],
          ),

          const SizedBox(height: 20),
          const Divider(color: NvrColors.border, height: 1),
          const SizedBox(height: 20),

          // ── Credentials ─────────────────────────────────────────────────
          _sectionLabel('CREDENTIALS'),
          Row(
            children: [
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    const Text('USERNAME', style: NvrTypography.monoLabel),
                    const SizedBox(height: 5),
                    TextField(
                      controller: _userCtrl,
                      style: const TextStyle(
                        color: NvrColors.textPrimary,
                        fontFamily: 'JetBrainsMono',
                        fontSize: 12,
                      ),
                      decoration: _inputDecoration(hint: 'admin'),
                    ),
                  ],
                ),
              ),
              const SizedBox(width: 10),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    const Text('PASSWORD', style: NvrTypography.monoLabel),
                    const SizedBox(height: 5),
                    TextField(
                      controller: _passCtrl,
                      obscureText: _obscurePass,
                      style: const TextStyle(
                        color: NvrColors.textPrimary,
                        fontFamily: 'JetBrainsMono',
                        fontSize: 12,
                      ),
                      decoration: _inputDecoration(
                        hint: '••••••••',
                        suffix: IconButton(
                          icon: Icon(
                            _obscurePass ? Icons.visibility_off : Icons.visibility,
                            color: NvrColors.textMuted,
                            size: 16,
                          ),
                          onPressed: () => setState(() => _obscurePass = !_obscurePass),
                        ),
                      ),
                    ),
                  ],
                ),
              ),
            ],
          ),

          const SizedBox(height: 12),
          HudButton(
            label: _probing ? 'PROBING...' : 'PROBE CAMERA',
            icon: Icons.wifi_find,
            style: HudButtonStyle.secondary,
            onPressed: _probing ? null : _probe,
          ),

          if (_probeError != null) ...[
            const SizedBox(height: 8),
            Text(
              _probeError!,
              style: NvrTypography.body.copyWith(color: NvrColors.danger),
            ),
          ],

          // ── Stream profiles ──────────────────────────────────────────────
          if (_profiles.isNotEmpty) ...[
            const SizedBox(height: 20),
            const Divider(color: NvrColors.border, height: 1),
            const SizedBox(height: 20),
            _sectionLabel('STREAMS'),
            ..._profiles.asMap().entries.map((entry) {
              final idx = entry.key;
              final profile = entry.value;
              final isSelected = _selectedProfileIndex == idx;

              final name = profile['name'] as String? ?? 'Stream ${idx + 1}';
              final token = profile['token'] as String? ?? '';
              final resolution = profile['resolution'] as String? ?? '';
              final codec = profile['codec'] as String? ?? profile['encoding'] as String? ?? '';
              final rtsp = profile['rtsp_uri'] as String? ?? profile['rtsp_url'] as String? ?? '';

              final detailParts = <String>[
                if (resolution.isNotEmpty) resolution,
                if (codec.isNotEmpty) codec,
                if (token.isNotEmpty) token,
              ];

              return GestureDetector(
                onTap: () => setState(() => _selectedProfileIndex = idx),
                child: Container(
                  margin: const EdgeInsets.only(bottom: 8),
                  padding: const EdgeInsets.all(12),
                  decoration: BoxDecoration(
                    color: NvrColors.bgTertiary,
                    border: Border.all(
                      color: isSelected ? NvrColors.accent : NvrColors.border,
                      width: isSelected ? 1.5 : 1,
                    ),
                    borderRadius: BorderRadius.circular(6),
                  ),
                  child: Row(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      // Radio indicator
                      Padding(
                        padding: const EdgeInsets.only(top: 2),
                        child: Container(
                          width: 14,
                          height: 14,
                          decoration: BoxDecoration(
                            shape: BoxShape.circle,
                            border: Border.all(
                              color: isSelected ? NvrColors.accent : NvrColors.textMuted,
                              width: 1.5,
                            ),
                            color: isSelected ? NvrColors.accent : Colors.transparent,
                          ),
                          child: isSelected
                              ? const Icon(Icons.check, size: 8, color: NvrColors.bgPrimary)
                              : null,
                        ),
                      ),
                      const SizedBox(width: 10),
                      Expanded(
                        child: Column(
                          crossAxisAlignment: CrossAxisAlignment.start,
                          children: [
                            Text(name, style: NvrTypography.cameraName),
                            if (detailParts.isNotEmpty) ...[
                              const SizedBox(height: 3),
                              Text(
                                detailParts.join(' · '),
                                style: NvrTypography.monoLabel.copyWith(
                                  color: NvrColors.textSecondary,
                                ),
                              ),
                            ],
                            if (rtsp.isNotEmpty) ...[
                              const SizedBox(height: 4),
                              Text(
                                rtsp,
                                style: NvrTypography.monoLabel.copyWith(
                                  color: NvrColors.textMuted,
                                  fontSize: 8,
                                ),
                                maxLines: 1,
                                overflow: TextOverflow.ellipsis,
                              ),
                            ],
                          ],
                        ),
                      ),
                    ],
                  ),
                ),
              );
            }),
          ],

          // ── Capabilities ─────────────────────────────────────────────────
          if (_capabilities != null) ...[
            const SizedBox(height: 20),
            const Divider(color: NvrColors.border, height: 1),
            const SizedBox(height: 20),
            _sectionLabel('CAPABILITIES'),
            Wrap(
              children: [
                for (final cap in [
                  'media',
                  'events',
                  'analytics',
                  'ptz',
                  'audio',
                  'imaging',
                  'recording',
                ])
                  _CapBadge(
                    label: cap.toUpperCase(),
                    supported: _capabilities![cap] == true,
                  ),
              ],
            ),
          ],

          // ── Camera name field ─────────────────────────────────────────────
          const SizedBox(height: 20),
          const Divider(color: NvrColors.border, height: 1),
          const SizedBox(height: 20),
          _sectionLabel('CAMERA NAME'),
          TextField(
            controller: _nameCtrl,
            style: const TextStyle(
              color: NvrColors.textPrimary,
              fontFamily: 'JetBrainsMono',
              fontSize: 12,
            ),
            decoration: _inputDecoration(hint: 'e.g. Front Door'),
            onChanged: (_) => setState(() {}),
          ),

          // ── Add button ────────────────────────────────────────────────────
          const SizedBox(height: 20),
          HudButton(
            label: _adding ? 'ADDING...' : 'ADD CAMERA',
            icon: Icons.add,
            onPressed: canAdd ? _addCamera : null,
          ),
          if (_selectedProfileIndex == null && _profiles.isNotEmpty)
            Padding(
              padding: const EdgeInsets.only(top: 6),
              child: Text(
                'Select a stream above to continue.',
                style: NvrTypography.monoLabel.copyWith(color: NvrColors.textSecondary),
                textAlign: TextAlign.center,
              ),
            ),
          if (_profiles.isEmpty)
            Padding(
              padding: const EdgeInsets.only(top: 6),
              child: Text(
                'Probe the camera to discover available streams.',
                style: NvrTypography.monoLabel.copyWith(color: NvrColors.textSecondary),
                textAlign: TextAlign.center,
              ),
            ),
        ],
      ),
    );
  }
}
