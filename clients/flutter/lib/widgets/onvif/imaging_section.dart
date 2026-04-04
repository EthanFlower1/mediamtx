import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../theme/nvr_colors.dart';
import '../../theme/nvr_typography.dart';
import '../../providers/onvif_providers.dart';
import '../../providers/auth_provider.dart';
import '../hud/analog_slider.dart';

class ImagingSection extends ConsumerStatefulWidget {
  const ImagingSection({super.key, required this.cameraId});

  final String cameraId;

  @override
  ConsumerState<ImagingSection> createState() => _ImagingSectionState();
}

class _ImagingSectionState extends ConsumerState<ImagingSection> {
  // Local slider state so UI responds immediately without waiting for API.
  double? _brightness;
  double? _contrast;
  double? _saturation;
  double? _sharpness;

  Timer? _debounce;

  @override
  void dispose() {
    _debounce?.cancel();
    super.dispose();
  }

  void _onChanged({
    double? brightness,
    double? contrast,
    double? saturation,
    double? sharpness,
  }) {
    setState(() {
      if (brightness != null) _brightness = brightness;
      if (contrast != null) _contrast = contrast;
      if (saturation != null) _saturation = saturation;
      if (sharpness != null) _sharpness = sharpness;
    });

    _debounce?.cancel();
    _debounce = Timer(const Duration(milliseconds: 500), () {
      final api = ref.read(apiClientProvider);
      if (api == null) return;
      api.put('/cameras/${widget.cameraId}/settings', data: {
        'brightness': _brightness,
        'contrast': _contrast,
        'saturation': _saturation,
        'sharpness': _sharpness,
      });
    });
  }

  @override
  Widget build(BuildContext context) {
    final settingsAsync = ref.watch(imagingSettingsProvider(widget.cameraId));

    return settingsAsync.when(
      loading: () => const SizedBox.shrink(),
      error: (_, __) => const SizedBox.shrink(),
      data: (settings) {
        if (settings == null) return const SizedBox.shrink();

        // Initialise local state from provider on first build.
        _brightness ??= settings.brightness;
        _contrast ??= settings.contrast;
        _saturation ??= settings.saturation;
        _sharpness ??= settings.sharpness;

        return Container(
          decoration: BoxDecoration(
            color: NvrColors.of(context).bgSecondary,
            border: Border.all(color: NvrColors.of(context).border),
            borderRadius: BorderRadius.circular(4),
          ),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              // Header
              Padding(
                padding: const EdgeInsets.fromLTRB(12, 10, 12, 8),
                child: Text('IMAGING', style: NvrTypography.of(context).monoSection),
              ),
              Divider(height: 1, color: NvrColors.of(context).border),
              Padding(
                padding: const EdgeInsets.all(12),
                child: Column(
                  children: [
                    AnalogSlider(
                      label: 'BRIGHTNESS',
                      value: _brightness!,
                      min: 0.0,
                      max: 1.0,
                      onChanged: (v) => _onChanged(brightness: v),
                      valueFormatter: (v) => '${(v * 100).round()}%',
                    ),
                    const SizedBox(height: 16),
                    AnalogSlider(
                      label: 'CONTRAST',
                      value: _contrast!,
                      min: 0.0,
                      max: 1.0,
                      onChanged: (v) => _onChanged(contrast: v),
                      valueFormatter: (v) => '${(v * 100).round()}%',
                    ),
                    const SizedBox(height: 16),
                    AnalogSlider(
                      label: 'SATURATION',
                      value: _saturation!,
                      min: 0.0,
                      max: 1.0,
                      onChanged: (v) => _onChanged(saturation: v),
                      valueFormatter: (v) => '${(v * 100).round()}%',
                    ),
                    const SizedBox(height: 16),
                    AnalogSlider(
                      label: 'SHARPNESS',
                      value: _sharpness!,
                      min: 0.0,
                      max: 1.0,
                      onChanged: (v) => _onChanged(sharpness: v),
                      valueFormatter: (v) => '${(v * 100).round()}%',
                    ),
                  ],
                ),
              ),
            ],
          ),
        );
      },
    );
  }
}
