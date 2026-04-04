import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import '../../models/media_profile.dart';
import '../../providers/auth_provider.dart';
import '../../providers/onvif_providers.dart';
import '../../theme/nvr_colors.dart';
import '../../theme/nvr_typography.dart';
import '../hud/hud_button.dart';
import '../hud/analog_slider.dart';

class MediaConfigSection extends ConsumerStatefulWidget {
  const MediaConfigSection({super.key, required this.cameraId});

  final String cameraId;

  @override
  ConsumerState<MediaConfigSection> createState() => _MediaConfigSectionState();
}

class _MediaConfigSectionState extends ConsumerState<MediaConfigSection> {
  String? _expandedToken;

  // Per-profile local edit state keyed by encoder token
  final Map<String, _EncoderEditState> _editStates = {};

  void _toggleExpanded(ProfileInfo profile) {
    setState(() {
      if (_expandedToken == profile.token) {
        _expandedToken = null;
      } else {
        _expandedToken = profile.token;
        // Seed local edit state from profile if not already present
        final enc = profile.videoEncoder;
        if (enc != null && !_editStates.containsKey(enc.token)) {
          _editStates[enc.token] = _EncoderEditState.fromConfig(enc);
        }
      }
    });
  }

  Future<void> _saveEncoder(String profileToken, String encoderToken) async {
    final api = ref.read(apiClientProvider);
    if (api == null) return;
    final state = _editStates[encoderToken];
    if (state == null) return;

    final body = {
      'token': encoderToken,
      'encoding': state.encoding,
      'width': state.resolution?.width ?? 0,
      'height': state.resolution?.height ?? 0,
      'quality': state.quality,
      'frame_rate': state.frameRate,
    };

    try {
      await api.put(
        '/cameras/${widget.cameraId}/media/video-encoder/$encoderToken',
        data: body,
      );
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(
            backgroundColor: NvrColors.of(context).success,
            content: Text('Encoder configuration saved'),
          ),
        );
      }
      ref.invalidate(mediaProfilesProvider(widget.cameraId));
    } catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(
            backgroundColor: NvrColors.of(context).danger,
            content: Text('Save failed: $e'),
          ),
        );
      }
    }
  }

  Future<void> _deleteProfile(String token) async {
    final api = ref.read(apiClientProvider);
    if (api == null) return;
    try {
      await api.delete('/cameras/${widget.cameraId}/media/profiles/$token');
      if (_expandedToken == token) {
        setState(() => _expandedToken = null);
      }
      ref.invalidate(mediaProfilesProvider(widget.cameraId));
    } catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(
            backgroundColor: NvrColors.of(context).danger,
            content: Text('Delete failed: $e'),
          ),
        );
      }
    }
  }

  Future<void> _addProfile() async {
    final nameCtrl = TextEditingController();
    final confirmed = await showDialog<bool>(
      context: context,
      builder: (ctx) => AlertDialog(
        backgroundColor: NvrColors.of(context).bgSecondary,
        title: Text('ADD PROFILE', style: NvrTypography.of(context).monoSection),
        content: TextField(
          controller: nameCtrl,
          style: TextStyle(color: NvrColors.of(context).textPrimary),
          decoration: InputDecoration(
            labelText: 'Profile Name',
            labelStyle: TextStyle(color: NvrColors.of(context).textMuted, fontSize: 12),
            enabledBorder: UnderlineInputBorder(
              borderSide: BorderSide(color: NvrColors.of(context).border),
            ),
            focusedBorder: UnderlineInputBorder(
              borderSide: BorderSide(color: NvrColors.of(context).accent),
            ),
          ),
          autofocus: true,
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.of(ctx).pop(false),
            child: Text('CANCEL',
                style: TextStyle(color: NvrColors.of(context).textMuted, fontSize: 11)),
          ),
          TextButton(
            onPressed: () => Navigator.of(ctx).pop(true),
            child: Text('CREATE',
                style: TextStyle(color: NvrColors.of(context).accent, fontSize: 11)),
          ),
        ],
      ),
    );
    nameCtrl.dispose();

    if (confirmed != true) return;
    final name = nameCtrl.text.trim();
    if (name.isEmpty) return;

    final api = ref.read(apiClientProvider);
    if (api == null) return;
    try {
      await api.post(
        '/cameras/${widget.cameraId}/media/profiles',
        data: {'name': name},
      );
      ref.invalidate(mediaProfilesProvider(widget.cameraId));
    } catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(
            backgroundColor: NvrColors.of(context).danger,
            content: Text('Create failed: $e'),
          ),
        );
      }
    }
  }

  @override
  Widget build(BuildContext context) {
    final profilesAsync = ref.watch(mediaProfilesProvider(widget.cameraId));

    return profilesAsync.when(
      loading: () => Padding(
        padding: EdgeInsets.symmetric(vertical: 12),
        child: Center(
          child: SizedBox(
            width: 16,
            height: 16,
            child: CircularProgressIndicator(
              strokeWidth: 1.5,
              color: NvrColors.of(context).accent,
            ),
          ),
        ),
      ),
      error: (_, __) => const SizedBox.shrink(),
      data: (profiles) {
        return Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            ...profiles.map((profile) => _ProfileCard(
                  key: ValueKey(profile.token),
                  profile: profile,
                  cameraId: widget.cameraId,
                  isExpanded: _expandedToken == profile.token,
                  editState: profile.videoEncoder != null
                      ? _editStates[profile.videoEncoder!.token]
                      : null,
                  onTap: () => _toggleExpanded(profile),
                  onDelete: () => _deleteProfile(profile.token),
                  onSave: profile.videoEncoder != null
                      ? () => _saveEncoder(profile.token, profile.videoEncoder!.token)
                      : null,
                  onEditStateChanged: (s) {
                    if (profile.videoEncoder != null) {
                      setState(() => _editStates[profile.videoEncoder!.token] = s);
                    }
                  },
                )),
            const SizedBox(height: 12),
            HudButton(
              label: 'ADD PROFILE',
              style: HudButtonStyle.secondary,
              icon: Icons.add,
              onPressed: _addProfile,
            ),
          ],
        );
      },
    );
  }
}

// ── Profile Card ─────────────────────────────────────────────────────────────

class _ProfileCard extends ConsumerWidget {
  const _ProfileCard({
    super.key,
    required this.profile,
    required this.cameraId,
    required this.isExpanded,
    required this.editState,
    required this.onTap,
    required this.onDelete,
    required this.onSave,
    required this.onEditStateChanged,
  });

  final ProfileInfo profile;
  final String cameraId;
  final bool isExpanded;
  final _EncoderEditState? editState;
  final VoidCallback onTap;
  final VoidCallback onDelete;
  final VoidCallback? onSave;
  final ValueChanged<_EncoderEditState> onEditStateChanged;

  String get _summary {
    final enc = profile.videoEncoder;
    if (enc == null) return profile.name;
    final codec = enc.encoding.isNotEmpty ? enc.encoding.toUpperCase() : '';
    final res = enc.width > 0 ? '${enc.width}x${enc.height}' : '';
    return [codec, res].where((s) => s.isNotEmpty).join(' ');
  }

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    return Container(
      margin: const EdgeInsets.only(bottom: 6),
      decoration: BoxDecoration(
        color: NvrColors.of(context).bgTertiary,
        border: Border.all(
          color: isExpanded ? NvrColors.of(context).accent.withValues(alpha: 0.5) : NvrColors.of(context).border,
        ),
        borderRadius: BorderRadius.circular(4),
      ),
      child: Column(
        children: [
          // Header row
          InkWell(
            onTap: onTap,
            borderRadius: const BorderRadius.vertical(top: Radius.circular(4)),
            child: Padding(
              padding: const EdgeInsets.fromLTRB(12, 10, 8, 10),
              child: Row(
                children: [
                  Expanded(
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        Text(profile.name, style: NvrTypography.of(context).cameraName),
                        if (_summary.isNotEmpty && _summary != profile.name)
                          Padding(
                            padding: const EdgeInsets.only(top: 2),
                            child: Text(_summary, style: NvrTypography.of(context).monoLabel),
                          ),
                      ],
                    ),
                  ),
                  Icon(
                    isExpanded ? Icons.expand_less : Icons.expand_more,
                    size: 16,
                    color: NvrColors.of(context).textMuted,
                  ),
                  const SizedBox(width: 4),
                  InkWell(
                    onTap: onDelete,
                    borderRadius: BorderRadius.circular(4),
                    child: Padding(
                      padding: const EdgeInsets.all(4),
                      child: Icon(Icons.delete_outline,
                          size: 16, color: NvrColors.of(context).danger.withValues(alpha: 0.7)),
                    ),
                  ),
                ],
              ),
            ),
          ),
          // Expanded encoder editor
          if (isExpanded && profile.videoEncoder != null)
            _EncoderEditor(
              cameraId: cameraId,
              encoder: profile.videoEncoder!,
              editState: editState ??
                  _EncoderEditState.fromConfig(profile.videoEncoder!),
              onChanged: onEditStateChanged,
              onSave: onSave,
            ),
        ],
      ),
    );
  }
}

// ── Encoder Editor ────────────────────────────────────────────────────────────

class _EncoderEditor extends ConsumerWidget {
  const _EncoderEditor({
    required this.cameraId,
    required this.encoder,
    required this.editState,
    required this.onChanged,
    required this.onSave,
  });

  final String cameraId;
  final VideoEncoderConfig encoder;
  final _EncoderEditState editState;
  final ValueChanged<_EncoderEditState> onChanged;
  final VoidCallback? onSave;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final optionsKey =
        (cameraId: cameraId, configToken: encoder.token);
    final optionsAsync = ref.watch(videoEncoderOptionsProvider(optionsKey));

    return Container(
      decoration: BoxDecoration(
        border: Border(top: BorderSide(color: NvrColors.of(context).border)),
      ),
      padding: const EdgeInsets.all(12),
      child: optionsAsync.when(
        loading: () => Center(
          child: SizedBox(
            width: 14,
            height: 14,
            child: CircularProgressIndicator(
                strokeWidth: 1.5, color: NvrColors.of(context).accent),
          ),
        ),
        error: (_, __) => Text('Failed to load options',
            style: TextStyle(color: NvrColors.of(context).danger, fontSize: 11)),
        data: (options) {
          if (options == null) {
            return Text('No options available',
                style: TextStyle(color: NvrColors.of(context).textMuted, fontSize: 11));
          }

          final encodings = options.encodings.toSet().toList();
          final resolutions = options.resolutions.toSet().toList();
          final frRange = options.frameRateRange;
          final qRange = options.qualityRange;

          // Clamp current values to valid ranges
          final currentEncoding = encodings.contains(editState.encoding)
              ? editState.encoding
              : (encodings.isNotEmpty ? encodings.first : editState.encoding);

          final currentRes = resolutions.any(
                  (r) => r.width == editState.resolution?.width &&
                      r.height == editState.resolution?.height)
              ? editState.resolution
              : (resolutions.isNotEmpty ? resolutions.first : editState.resolution);

          final currentFr = editState.frameRate
              .clamp(frRange.min, frRange.max)
              .toDouble();

          final currentQ = editState.quality.clamp(
              qRange.min.toDouble(), qRange.max.toDouble());

          return Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              // Encoding dropdown
              if (encodings.isNotEmpty) ...[
                Text('ENCODING', style: NvrTypography.of(context).monoLabel),
                const SizedBox(height: 6),
                _NvrDropdown<String>(
                  value: currentEncoding,
                  items: encodings,
                  itemLabel: (e) => e.toUpperCase(),
                  onChanged: (v) {
                    if (v != null) {
                      onChanged(editState.copyWith(encoding: v));
                    }
                  },
                ),
                const SizedBox(height: 14),
              ],
              // Resolution dropdown
              if (resolutions.isNotEmpty) ...[
                Text('RESOLUTION', style: NvrTypography.of(context).monoLabel),
                const SizedBox(height: 6),
                _NvrDropdown<Resolution>(
                  value: currentRes,
                  items: resolutions,
                  itemLabel: (r) => r.toString(),
                  onChanged: (v) {
                    if (v != null) {
                      onChanged(editState.copyWith(resolution: v));
                    }
                  },
                ),
                const SizedBox(height: 14),
              ],
              // Quality slider
              AnalogSlider(
                label: 'QUALITY',
                value: currentQ,
                min: qRange.min.toDouble(),
                max: qRange.max.toDouble(),
                onChanged: (v) => onChanged(editState.copyWith(quality: v)),
                valueFormatter: (v) => v.round().toString(),
              ),
              const SizedBox(height: 14),
              // Frame rate slider
              AnalogSlider(
                label: 'FRAME RATE',
                value: currentFr,
                min: frRange.min.toDouble(),
                max: frRange.max.toDouble(),
                onChanged: (v) =>
                    onChanged(editState.copyWith(frameRate: v.round())),
                valueFormatter: (v) => '${v.round()} fps',
              ),
              const SizedBox(height: 14),
              // Save button
              HudButton(
                label: 'SAVE',
                style: HudButtonStyle.primary,
                onPressed: onSave,
              ),
            ],
          );
        },
      ),
    );
  }
}

// ── NVR-styled dropdown ───────────────────────────────────────────────────────

class _NvrDropdown<T> extends StatelessWidget {
  const _NvrDropdown({
    required this.value,
    required this.items,
    required this.itemLabel,
    required this.onChanged,
  });

  final T? value;
  final List<T> items;
  final String Function(T) itemLabel;
  final ValueChanged<T?> onChanged;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 2),
      decoration: BoxDecoration(
        color: NvrColors.of(context).bgSecondary,
        border: Border.all(color: NvrColors.of(context).border),
        borderRadius: BorderRadius.circular(4),
      ),
      child: DropdownButtonHideUnderline(
        child: DropdownButton<T>(
          value: value,
          isExpanded: true,
          dropdownColor: NvrColors.of(context).bgSecondary,
          iconEnabledColor: NvrColors.of(context).textMuted,
          style: TextStyle(
            fontFamily: 'JetBrainsMono',
            fontSize: 11,
            color: NvrColors.of(context).textPrimary,
            letterSpacing: 0.5,
          ),
          items: items
              .map((item) => DropdownMenuItem<T>(
                    value: item,
                    child: Text(itemLabel(item)),
                  ))
              .toList(),
          onChanged: onChanged,
        ),
      ),
    );
  }
}

// ── Edit state ────────────────────────────────────────────────────────────────

class _EncoderEditState {
  final String encoding;
  final Resolution? resolution;
  final double quality;
  final int frameRate;

  const _EncoderEditState({
    required this.encoding,
    required this.resolution,
    required this.quality,
    required this.frameRate,
  });

  factory _EncoderEditState.fromConfig(VideoEncoderConfig cfg) =>
      _EncoderEditState(
        encoding: cfg.encoding,
        resolution: cfg.width > 0
            ? Resolution(width: cfg.width, height: cfg.height)
            : null,
        quality: cfg.quality,
        frameRate: cfg.frameRate,
      );

  _EncoderEditState copyWith({
    String? encoding,
    Resolution? resolution,
    double? quality,
    int? frameRate,
  }) =>
      _EncoderEditState(
        encoding: encoding ?? this.encoding,
        resolution: resolution ?? this.resolution,
        quality: quality ?? this.quality,
        frameRate: frameRate ?? this.frameRate,
      );
}
