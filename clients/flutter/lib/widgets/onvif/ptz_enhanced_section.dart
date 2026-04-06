import 'dart:async';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import '../../models/ptz_status.dart';
import '../../providers/auth_provider.dart';
import '../../providers/onvif_providers.dart';
import '../../theme/nvr_colors.dart';
import '../../theme/nvr_typography.dart';
import '../hud/hud_button.dart';

class PtzEnhancedSection extends ConsumerStatefulWidget {
  const PtzEnhancedSection({super.key, required this.cameraId});

  final String cameraId;

  @override
  ConsumerState<PtzEnhancedSection> createState() => _PtzEnhancedSectionState();
}

class _PtzEnhancedSectionState extends ConsumerState<PtzEnhancedSection> {
  Timer? _pollTimer;
  PtzStatus? _status;

  @override
  void initState() {
    super.initState();
    _pollStatus();
    _pollTimer = Timer.periodic(const Duration(seconds: 5), (_) => _pollStatus());
  }

  @override
  void dispose() {
    _pollTimer?.cancel();
    super.dispose();
  }

  Future<void> _pollStatus() async {
    final api = ref.read(apiClientProvider);
    if (api == null) return;
    try {
      final res = await api.get('/cameras/${widget.cameraId}/ptz/status');
      final status = PtzStatus.fromJson(res.data as Map<String, dynamic>);
      if (mounted) {
        setState(() => _status = status);
      }
    } catch (_) {
      // Silently ignore poll errors
    }
  }

  Future<void> _sendPtzAction(Map<String, dynamic> data) async {
    final api = ref.read(apiClientProvider);
    if (api == null) return;
    await api.post('/cameras/${widget.cameraId}/ptz', data: data);
  }

  Future<void> _savePreset() async {
    final name = await showDialog<String>(
      context: context,
      builder: (ctx) {
        final ctrl = TextEditingController();
        return AlertDialog(
          backgroundColor: NvrColors.of(context).bgSecondary,
          shape: RoundedRectangleBorder(
            borderRadius: BorderRadius.circular(4),
            side: BorderSide(color: NvrColors.of(context).border),
          ),
          title: Text('SAVE PRESET', style: NvrTypography.of(context).monoSection),
          content: TextField(
            controller: ctrl,
            autofocus: true,
            style: NvrTypography.of(context).monoData,
            cursorColor: NvrColors.of(context).accent,
            decoration: InputDecoration(
              hintText: 'Preset name',
              hintStyle: NvrTypography.of(context).monoLabel,
              enabledBorder: UnderlineInputBorder(
                borderSide: BorderSide(color: NvrColors.of(context).border),
              ),
              focusedBorder: UnderlineInputBorder(
                borderSide: BorderSide(color: NvrColors.of(context).accent),
              ),
            ),
          ),
          actions: [
            TextButton(
              onPressed: () => Navigator.of(ctx).pop(),
              child: Text('CANCEL',
                  style: NvrTypography.of(context).monoLabel.copyWith(
                    color: NvrColors.of(context).textSecondary,
                  )),
            ),
            TextButton(
              onPressed: () => Navigator.of(ctx).pop(ctrl.text.trim()),
              child: Text('SAVE', style: NvrTypography.of(context).monoSection),
            ),
          ],
        );
      },
    );

    if (name == null || name.isEmpty) return;
    final api = ref.read(apiClientProvider);
    if (api == null) return;
    await api.post(
      '/cameras/${widget.cameraId}/ptz',
      data: {'action': 'set_preset', 'name': name},
    );
    ref.invalidate(ptzPresetsProvider(widget.cameraId));
  }

  Future<void> _deletePreset(String token) async {
    final api = ref.read(apiClientProvider);
    if (api == null) return;
    await api.post(
      '/cameras/${widget.cameraId}/ptz',
      data: {'action': 'remove_preset', 'preset_token': token},
    );
    ref.invalidate(ptzPresetsProvider(widget.cameraId));
  }

  Widget _buildPositionDisplay() {
    final status = _status;
    return Container(
      decoration: BoxDecoration(
        color: NvrColors.of(context).bgTertiary,
        border: Border.all(color: NvrColors.of(context).border),
        borderRadius: BorderRadius.circular(4),
      ),
      padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 10),
      child: Row(
        children: [
          _PositionColumn(label: 'PAN', value: status?.panPosition),
          const SizedBox(width: 16),
          _PositionColumn(label: 'TILT', value: status?.tiltPosition),
          const SizedBox(width: 16),
          _PositionColumn(label: 'ZOOM', value: status?.zoomPosition),
          const Spacer(),
          if (status?.isMoving == true)
            Text(
              'MOVING',
              style: NvrTypography.of(context).monoSection.copyWith(
                color: NvrColors.of(context).accent,
                fontSize: 9,
              ),
            ),
        ],
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    final presetsAsync = ref.watch(ptzPresetsProvider(widget.cameraId));

    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        // Position display
        _buildPositionDisplay(),
        const SizedBox(height: 12),

        // Home controls
        Row(
          children: [
            HudButton(
              label: 'GO HOME',
              icon: Icons.home,
              style: HudButtonStyle.tactical,
              onPressed: () => _sendPtzAction({'action': 'home'}),
            ),
            const SizedBox(width: 8),
            HudButton(
              label: 'SET HOME',
              style: HudButtonStyle.secondary,
              onPressed: () => _sendPtzAction({'action': 'set_home'}),
            ),
          ],
        ),
        const SizedBox(height: 8),

        // Save preset button
        HudButton(
          label: 'SAVE PRESET',
          icon: Icons.add,
          style: HudButtonStyle.secondary,
          onPressed: _savePreset,
        ),

        // Presets list
        presetsAsync.when(
          loading: () => const SizedBox.shrink(),
          error: (_, __) => const SizedBox.shrink(),
          data: (presets) {
            if (presets.isEmpty) return const SizedBox.shrink();
            return Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                const SizedBox(height: 12),
                Divider(height: 1, color: NvrColors.of(context).border),
                const SizedBox(height: 12),
                ...presets.map((preset) => Padding(
                      padding: const EdgeInsets.only(bottom: 8),
                      child: Row(
                        children: [
                          Expanded(
                            child: Column(
                              crossAxisAlignment: CrossAxisAlignment.start,
                              children: [
                                Text(
                                  preset.name.isNotEmpty
                                      ? preset.name
                                      : preset.token,
                                  style: NvrTypography.of(context).monoData,
                                ),
                                const SizedBox(height: 2),
                                Text(
                                  preset.token,
                                  style: NvrTypography.of(context).monoLabel,
                                ),
                              ],
                            ),
                          ),
                          HudButton(
                            label: 'GO TO',
                            style: HudButtonStyle.secondary,
                            onPressed: () => _sendPtzAction({
                              'action': 'preset',
                              'preset_token': preset.token,
                            }),
                          ),
                          const SizedBox(width: 8),
                          IconButton(
                            icon: Icon(
                              Icons.delete_outline,
                              size: 16,
                              color: NvrColors.of(context).danger,
                            ),
                            padding: EdgeInsets.zero,
                            constraints: const BoxConstraints(
                              minWidth: 28,
                              minHeight: 28,
                            ),
                            onPressed: () => _deletePreset(preset.token),
                          ),
                        ],
                      ),
                    )),
              ],
            );
          },
        ),
      ],
    );
  }
}

class _PositionColumn extends StatelessWidget {
  const _PositionColumn({required this.label, required this.value});

  final String label;
  final double? value;

  @override
  Widget build(BuildContext context) {
    final display = value != null ? value!.toStringAsFixed(2) : '--';
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(label, style: NvrTypography.of(context).monoLabel),
        const SizedBox(height: 2),
        Text(display, style: NvrTypography.of(context).monoData),
      ],
    );
  }
}
