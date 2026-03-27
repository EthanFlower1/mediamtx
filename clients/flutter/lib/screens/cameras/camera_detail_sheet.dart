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
  if (rtsp.isEmpty || (user.isEmpty && pass.isEmpty)) return rtsp;
  if (!rtsp.startsWith('rtsp://')) return rtsp;
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
  final _rtspCtrl = TextEditingController();

  bool _probing = false;
  bool _probed = false;
  String? _probeError;
  List<Map<String, dynamic>> _profiles = [];
  Map<String, dynamic>? _capabilities;
  // Per-stream role assignments: index → set of roles
  final Map<int, Set<String>> _streamRoles = {};
  bool _adding = false;
  bool _obscurePass = true;

  static const _allRoles = ['live_view', 'recording', 'mobile', 'ai_detection'];

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
    _rtspCtrl.dispose();
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
      _streamRoles.clear();
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
      // Auto-assign default roles: highest res → live_view, lowest → recording+ai+mobile
      _streamRoles.clear();
      for (int i = 0; i < profiles.length; i++) {
        if (profiles.length == 1) {
          _streamRoles[i] = {'live_view', 'recording', 'ai_detection', 'mobile'};
        } else if (i == 0) {
          _streamRoles[i] = {'live_view'};
        } else if (i == profiles.length - 1) {
          _streamRoles[i] = {'recording', 'ai_detection', 'mobile'};
        } else {
          _streamRoles[i] = {};
        }
      }
      setState(() {
        _profiles = profiles;
        _capabilities = caps;
        _probed = true;
      });
    } catch (e) {
      setState(() => _probeError = e.toString());
    } finally {
      if (mounted) setState(() => _probing = false);
    }
  }

  // ── Add camera ────────────────────────────────────────────────────────────

  Future<void> _addCamera() async {
    final api = ref.read(apiClientProvider);
    if (api == null) return;

    final user = _userCtrl.text.trim();
    final pass = _passCtrl.text;

    // Find the primary RTSP URL (first stream with live_view role, or first stream, or manual input).
    String rtspUrl = '';
    String profileToken = '';

    if (_profiles.isNotEmpty) {
      // Find live_view stream for primary URL
      for (int i = 0; i < _profiles.length; i++) {
        if (_streamRoles[i]?.contains('live_view') ?? false) {
          final rawRtsp = _profiles[i]['stream_uri'] as String? ?? _profiles[i]['rtsp_url'] as String? ?? '';
          rtspUrl = _injectCredentials(rawRtsp, user, pass);
          profileToken = _profiles[i]['token'] as String? ?? '';
          break;
        }
      }
      // Fall back to first stream if no live_view assigned
      if (rtspUrl.isEmpty) {
        final rawRtsp = _profiles[0]['stream_uri'] as String? ?? _profiles[0]['rtsp_url'] as String? ?? '';
        rtspUrl = _injectCredentials(rawRtsp, user, pass);
        profileToken = _profiles[0]['token'] as String? ?? '';
      }
    } else {
      rtspUrl = _injectCredentials(_rtspCtrl.text.trim(), user, pass);
    }

    if (rtspUrl.isEmpty || !rtspUrl.startsWith('rtsp://')) return;

    // Build profiles array with roles for the backend to create streams
    final profilesData = <Map<String, dynamic>>[];
    for (int i = 0; i < _profiles.length; i++) {
      final p = _profiles[i];
      final roles = _streamRoles[i] ?? <String>{};
      final rawRtsp = p['stream_uri'] as String? ?? p['rtsp_url'] as String? ?? '';
      profilesData.add({
        'name': p['name'] as String? ?? 'Stream ${i + 1}',
        'rtsp_url': _injectCredentials(rawRtsp, user, pass),
        'profile_token': p['token'] as String? ?? '',
        'video_codec': p['video_codec'] as String? ?? p['codec'] as String? ?? '',
        'width': p['width'] as int? ?? 0,
        'height': p['height'] as int? ?? 0,
        'roles': roles.join(','),
      });
    }

    setState(() => _adding = true);
    try {
      await api.post('/cameras', data: {
        'name': _nameCtrl.text.trim(),
        'rtsp_url': rtspUrl,
        'onvif_endpoint': _device['xaddr'] ?? '',
        'onvif_username': user,
        'onvif_password': pass,
        'onvif_profile_token': profileToken,
        if (profilesData.isNotEmpty) 'profiles': profilesData,
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

    final hasStreams = _profiles.isNotEmpty ||
        _rtspCtrl.text.trim().startsWith('rtsp://');
    final canAdd = hasStreams && !_adding;

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

          // ── Stream profiles with role toggles ──────────────────────────
          if (_profiles.isNotEmpty) ...[
            const SizedBox(height: 20),
            const Divider(color: NvrColors.border, height: 1),
            const SizedBox(height: 20),
            _sectionLabel('STREAMS'),
            Text(
              'Tap roles to assign each stream\'s purpose.',
              style: NvrTypography.body.copyWith(fontSize: 11),
            ),
            const SizedBox(height: 10),
            ..._profiles.asMap().entries.map((entry) {
              final idx = entry.key;
              final profile = entry.value;
              final roles = _streamRoles[idx] ?? <String>{};

              final name = profile['name'] as String? ?? 'Stream ${idx + 1}';
              final w = profile['width'] as int? ?? 0;
              final h = profile['height'] as int? ?? 0;
              final codec = profile['video_codec'] as String? ?? profile['codec'] as String? ?? '';
              final rtsp = profile['stream_uri'] as String? ?? profile['rtsp_url'] as String? ?? '';
              final res = (w > 0 && h > 0) ? '${w}×$h' : '';

              final detailParts = <String>[
                if (res.isNotEmpty) res,
                if (codec.isNotEmpty) codec,
              ];

              return Container(
                margin: const EdgeInsets.only(bottom: 8),
                padding: const EdgeInsets.all(12),
                decoration: BoxDecoration(
                  color: NvrColors.bgTertiary,
                  border: Border.all(color: NvrColors.border),
                  borderRadius: BorderRadius.circular(6),
                ),
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
                    const SizedBox(height: 8),
                    // Role toggle chips
                    Wrap(
                      spacing: 6,
                      runSpacing: 6,
                      children: _allRoles.map((role) {
                        final active = roles.contains(role);
                        return GestureDetector(
                          onTap: () {
                            setState(() {
                              final set = _streamRoles.putIfAbsent(idx, () => {});
                              if (active) {
                                set.remove(role);
                              } else {
                                set.add(role);
                              }
                            });
                          },
                          child: Container(
                            padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 4),
                            decoration: BoxDecoration(
                              color: active
                                  ? NvrColors.accent.withValues(alpha: 0.15)
                                  : NvrColors.bgPrimary,
                              borderRadius: BorderRadius.circular(4),
                              border: Border.all(
                                color: active
                                    ? NvrColors.accent
                                    : NvrColors.border,
                              ),
                            ),
                            child: Text(
                              role.replaceAll('_', ' ').toUpperCase(),
                              style: TextStyle(
                                fontFamily: 'JetBrainsMono',
                                fontSize: 8,
                                fontWeight: FontWeight.w600,
                                letterSpacing: 0.5,
                                color: active ? NvrColors.accent : NvrColors.textMuted,
                              ),
                            ),
                          ),
                        );
                      }).toList(),
                    ),
                  ],
                ),
              );
            }),
          ],

          // ── Manual RTSP URL (when probe found no streams) ────────────────
          if (_probed && _profiles.isEmpty) ...[
            const SizedBox(height: 20),
            const Divider(color: NvrColors.border, height: 1),
            const SizedBox(height: 20),
            _sectionLabel('RTSP URL'),
            Text(
              'No streams could be auto-discovered. Enter the RTSP URL manually.',
              style: NvrTypography.body.copyWith(fontSize: 11),
            ),
            const SizedBox(height: 8),
            TextField(
              controller: _rtspCtrl,
              style: const TextStyle(
                color: NvrColors.textPrimary,
                fontFamily: 'JetBrainsMono',
                fontSize: 12,
              ),
              decoration: _inputDecoration(hint: 'rtsp://192.168.1.111:554/stream'),
              onChanged: (_) => setState(() {}),
            ),
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
          if (_profiles.isEmpty && !_probed)
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
