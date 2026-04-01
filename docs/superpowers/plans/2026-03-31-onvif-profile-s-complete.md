# ONVIF Profile S Complete Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Complete ONVIF Profile S client support across server (Go) and Flutter client in 4 phases — wire-up existing features, enhanced PTZ, media configuration, and device management.

**Architecture:** Vertical slices per phase. Each phase adds Go ONVIF methods, REST API endpoints, Flutter models/providers, and Flutter UI sections. All new ONVIF calls use `onvif-go` library methods on `*onvifgo.Client`. Flutter sections added as collapsible `_ExpandableSection` widgets in the existing camera detail screen.

**Tech Stack:** Go 1.25 + gin + onvif-go library (server), Flutter + Riverpod + Dio + Freezed (client)

**Spec:** `docs/superpowers/specs/2026-03-31-onvif-profile-s-complete-design.md`

---

## File Map

### Server (Go) — New/Modified Files

| File                                 | Responsibility                                                                          |
| ------------------------------------ | --------------------------------------------------------------------------------------- |
| `internal/nvr/onvif/ptz.go`          | Extend: AbsoluteMove, RelativeMove, SetPreset, RemovePreset, SetHomePosition, GetStatus |
| `internal/nvr/onvif/media_config.go` | New: Profile CRUD, video/audio encoder config get/set/options                           |
| `internal/nvr/onvif/device_mgmt.go`  | New: Date/time, hostname, network, device users, scopes, reboot                         |
| `internal/nvr/api/cameras.go`        | Extend: All new API endpoint handlers                                                   |
| `internal/nvr/api/router.go`         | Extend: Register new routes                                                             |

### Flutter — New Files

| File                                          | Responsibility                                                                                                     |
| --------------------------------------------- | ------------------------------------------------------------------------------------------------------------------ |
| `lib/models/device_info.dart`                 | DeviceInfo model                                                                                                   |
| `lib/models/ptz_status.dart`                  | PTZStatus model                                                                                                    |
| `lib/models/media_profile.dart`               | ProfileInfo, VideoEncoderConfig, AudioEncoderConfig, options models                                                |
| `lib/models/device_management.dart`           | DateTimeInfo, HostnameInfo, NetworkInterfaceInfo, etc.                                                             |
| `lib/providers/onvif_providers.dart`          | All ONVIF-related providers (device info, imaging, relay, presets, audio, PTZ status, media profiles, device mgmt) |
| `lib/widgets/onvif/device_info_section.dart`  | Device info display                                                                                                |
| `lib/widgets/onvif/imaging_section.dart`      | Imaging settings with API save                                                                                     |
| `lib/widgets/onvif/relay_section.dart`        | Relay output toggles                                                                                               |
| `lib/widgets/onvif/ptz_presets_section.dart`  | PTZ presets + home + set preset                                                                                    |
| `lib/widgets/onvif/audio_section.dart`        | Audio capabilities display                                                                                         |
| `lib/widgets/onvif/ptz_enhanced_section.dart` | Enhanced PTZ with position display                                                                                 |
| `lib/widgets/onvif/media_config_section.dart` | Media profiles + encoder config                                                                                    |
| `lib/widgets/onvif/device_mgmt_section.dart`  | Device management (system, network, users)                                                                         |

### Flutter — Modified Files

| File                                            | Change                         |
| ----------------------------------------------- | ------------------------------ |
| `lib/screens/cameras/camera_detail_screen.dart` | Add collapsible ONVIF sections |

---

## Phase 1: Wire-up Existing Features

### Task 1: Server — Device Info Endpoint

**Files:**

- Modify: `internal/nvr/api/cameras.go`
- Modify: `internal/nvr/api/router.go`

- [ ] **Step 1: Add GetDeviceInfo handler to cameras.go**

Add this handler method to `CameraHandler`:

```go
// GetDeviceInfo returns basic device information from the camera.
func (h *CameraHandler) GetDeviceInfo(c *gin.Context) {
	id := c.Param("id")
	cam, err := h.DB.GetCamera(id)
	if errors.Is(err, db.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "camera not found"})
		return
	}
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to retrieve camera", err)
		return
	}
	if cam.ONVIFEndpoint == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "camera has no ONVIF endpoint configured"})
		return
	}

	client, err := onvif.NewClient(cam.ONVIFEndpoint, cam.ONVIFUsername, h.decryptPassword(cam.ONVIFPassword))
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "failed to connect to camera"})
		return
	}

	ctx := c.Request.Context()
	info, err := client.Dev.GetDeviceInformation(ctx)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "failed to get device information"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"manufacturer":     info.Manufacturer,
		"model":            info.Model,
		"firmware_version": info.FirmwareVersion,
		"serial_number":    info.SerialNumber,
		"hardware_id":      info.HardwareID,
	})
}
```

- [ ] **Step 2: Register the route in router.go**

Add this line in the protected camera routes section (after the existing `GET /cameras/:id/ptz/capabilities` line):

```go
protected.GET("/cameras/:id/device-info", cameraHandler.GetDeviceInfo)
```

- [ ] **Step 3: Build and verify**

Run: `go build ./...`
Expected: Clean build with no errors.

- [ ] **Step 4: Commit**

```bash
git add internal/nvr/api/cameras.go internal/nvr/api/router.go
git commit -m "feat(api): add device-info endpoint for ONVIF device information"
```

---

### Task 2: Flutter — ONVIF Models

**Files:**

- Create: `clients/flutter/lib/models/device_info.dart`

- [ ] **Step 1: Create device_info.dart model**

```dart
class DeviceInfo {
  final String manufacturer;
  final String model;
  final String firmwareVersion;
  final String serialNumber;
  final String hardwareId;

  const DeviceInfo({
    required this.manufacturer,
    required this.model,
    required this.firmwareVersion,
    required this.serialNumber,
    required this.hardwareId,
  });

  factory DeviceInfo.fromJson(Map<String, dynamic> json) {
    return DeviceInfo(
      manufacturer: json['manufacturer'] as String? ?? '',
      model: json['model'] as String? ?? '',
      firmwareVersion: json['firmware_version'] as String? ?? '',
      serialNumber: json['serial_number'] as String? ?? '',
      hardwareId: json['hardware_id'] as String? ?? '',
    );
  }
}

class ImagingSettings {
  final double brightness;
  final double contrast;
  final double saturation;
  final double sharpness;

  const ImagingSettings({
    required this.brightness,
    required this.contrast,
    required this.saturation,
    required this.sharpness,
  });

  factory ImagingSettings.fromJson(Map<String, dynamic> json) {
    return ImagingSettings(
      brightness: (json['brightness'] as num?)?.toDouble() ?? 0.5,
      contrast: (json['contrast'] as num?)?.toDouble() ?? 0.5,
      saturation: (json['saturation'] as num?)?.toDouble() ?? 0.5,
      sharpness: (json['sharpness'] as num?)?.toDouble() ?? 0.5,
    );
  }

  Map<String, dynamic> toJson() => {
    'brightness': brightness,
    'contrast': contrast,
    'saturation': saturation,
    'sharpness': sharpness,
  };
}

class RelayOutput {
  final String token;
  final String mode;
  final String idleState;
  bool active;

  RelayOutput({
    required this.token,
    required this.mode,
    required this.idleState,
    this.active = false,
  });

  factory RelayOutput.fromJson(Map<String, dynamic> json) {
    return RelayOutput(
      token: json['token'] as String? ?? '',
      mode: json['mode'] as String? ?? '',
      idleState: json['idle_state'] as String? ?? '',
    );
  }
}

class PtzPreset {
  final String token;
  final String name;

  const PtzPreset({required this.token, required this.name});

  factory PtzPreset.fromJson(Map<String, dynamic> json) {
    return PtzPreset(
      token: json['token'] as String? ?? '',
      name: json['name'] as String? ?? '',
    );
  }
}

class AudioCapabilities {
  final bool hasBackchannel;
  final int audioSources;
  final int audioOutputs;

  const AudioCapabilities({
    required this.hasBackchannel,
    required this.audioSources,
    required this.audioOutputs,
  });

  factory AudioCapabilities.fromJson(Map<String, dynamic> json) {
    return AudioCapabilities(
      hasBackchannel: json['has_backchannel'] as bool? ?? false,
      audioSources: json['audio_sources'] as int? ?? 0,
      audioOutputs: json['audio_outputs'] as int? ?? 0,
    );
  }
}
```

- [ ] **Step 2: Commit**

```bash
git add clients/flutter/lib/models/device_info.dart
git commit -m "feat(flutter): add ONVIF models for device info, imaging, relay, presets, audio"
```

---

### Task 3: Flutter — ONVIF Providers

**Files:**

- Create: `clients/flutter/lib/providers/onvif_providers.dart`

- [ ] **Step 1: Create onvif_providers.dart**

```dart
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'auth_provider.dart';
import '../models/device_info.dart';

// ── Device Info ──────────────────────────────────────────────────────────

final deviceInfoProvider =
    FutureProvider.family<DeviceInfo?, String>((ref, cameraId) async {
  final api = ref.watch(apiClientProvider);
  if (api == null) return null;
  try {
    final res = await api.get('/cameras/$cameraId/device-info');
    return DeviceInfo.fromJson(res.data as Map<String, dynamic>);
  } catch (_) {
    return null;
  }
});

// ── Imaging Settings ─────────────────────────────────────────────────────

final imagingSettingsProvider =
    FutureProvider.family<ImagingSettings?, String>((ref, cameraId) async {
  final api = ref.watch(apiClientProvider);
  if (api == null) return null;
  try {
    final res = await api.get('/cameras/$cameraId/settings');
    return ImagingSettings.fromJson(res.data as Map<String, dynamic>);
  } catch (_) {
    return null;
  }
});

// ── Relay Outputs ────────────────────────────────────────────────────────

final relayOutputsProvider =
    FutureProvider.family<List<RelayOutput>, String>((ref, cameraId) async {
  final api = ref.watch(apiClientProvider);
  if (api == null) return [];
  try {
    final res = await api.get('/cameras/$cameraId/relay-outputs');
    final data = res.data;
    if (data is List) {
      return data
          .map((e) => RelayOutput.fromJson(e as Map<String, dynamic>))
          .toList();
    }
    return [];
  } catch (_) {
    return [];
  }
});

// ── PTZ Presets ──────────────────────────────────────────────────────────

final ptzPresetsProvider =
    FutureProvider.family<List<PtzPreset>, String>((ref, cameraId) async {
  final api = ref.watch(apiClientProvider);
  if (api == null) return [];
  try {
    final res = await api.get('/cameras/$cameraId/ptz/presets');
    final data = res.data;
    if (data is List) {
      return data
          .map((e) => PtzPreset.fromJson(e as Map<String, dynamic>))
          .toList();
    }
    return [];
  } catch (_) {
    return [];
  }
});

// ── Audio Capabilities ───────────────────────────────────────────────────

final audioCapabilitiesProvider =
    FutureProvider.family<AudioCapabilities?, String>((ref, cameraId) async {
  final api = ref.watch(apiClientProvider);
  if (api == null) return null;
  try {
    final res = await api.get('/cameras/$cameraId/audio/capabilities');
    return AudioCapabilities.fromJson(res.data as Map<String, dynamic>);
  } catch (_) {
    return null;
  }
});
```

- [ ] **Step 2: Commit**

```bash
git add clients/flutter/lib/providers/onvif_providers.dart
git commit -m "feat(flutter): add Riverpod providers for ONVIF features"
```

---

### Task 4: Flutter — ONVIF Section Widgets

**Files:**

- Create: `clients/flutter/lib/widgets/onvif/device_info_section.dart`
- Create: `clients/flutter/lib/widgets/onvif/imaging_section.dart`
- Create: `clients/flutter/lib/widgets/onvif/relay_section.dart`
- Create: `clients/flutter/lib/widgets/onvif/ptz_presets_section.dart`
- Create: `clients/flutter/lib/widgets/onvif/audio_section.dart`

- [ ] **Step 1: Create device_info_section.dart**

```dart
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import '../../models/device_info.dart';
import '../../providers/onvif_providers.dart';
import '../../theme/nvr_colors.dart';
import '../../theme/nvr_typography.dart';

class DeviceInfoSection extends ConsumerWidget {
  final String cameraId;
  const DeviceInfoSection({super.key, required this.cameraId});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final infoAsync = ref.watch(deviceInfoProvider(cameraId));

    return infoAsync.when(
      loading: () => const SizedBox.shrink(),
      error: (_, __) => const SizedBox.shrink(),
      data: (info) {
        if (info == null) return const SizedBox.shrink();
        return Container(
          padding: const EdgeInsets.all(12),
          decoration: BoxDecoration(
            color: NvrColors.bgSecondary,
            borderRadius: BorderRadius.circular(8),
            border: Border.all(color: NvrColors.border),
          ),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text('DEVICE INFO', style: NvrTypography.monoSection),
              const SizedBox(height: 10),
              _row('MANUFACTURER', info.manufacturer),
              _row('MODEL', info.model),
              _row('FIRMWARE', info.firmwareVersion),
              _row('SERIAL', info.serialNumber),
              if (info.hardwareId.isNotEmpty) _row('HARDWARE ID', info.hardwareId),
            ],
          ),
        );
      },
    );
  }

  Widget _row(String label, String value) {
    return Padding(
      padding: const EdgeInsets.only(bottom: 6),
      child: Row(
        children: [
          SizedBox(
            width: 120,
            child: Text(label, style: NvrTypography.monoLabel),
          ),
          Expanded(
            child: Text(
              value.isEmpty ? '—' : value,
              style: NvrTypography.monoData,
            ),
          ),
        ],
      ),
    );
  }
}
```

- [ ] **Step 2: Create imaging_section.dart**

This widget loads imaging settings from the API, shows sliders, and saves on change with debouncing.

```dart
import 'dart:async';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import '../../models/device_info.dart';
import '../../providers/auth_provider.dart';
import '../../providers/onvif_providers.dart';
import '../../theme/nvr_colors.dart';
import '../../theme/nvr_typography.dart';
import '../hud/analog_slider.dart';

class ImagingSection extends ConsumerStatefulWidget {
  final String cameraId;
  const ImagingSection({super.key, required this.cameraId});

  @override
  ConsumerState<ImagingSection> createState() => _ImagingSectionState();
}

class _ImagingSectionState extends ConsumerState<ImagingSection> {
  double _brightness = 0.5;
  double _contrast = 0.5;
  double _saturation = 0.5;
  double _sharpness = 0.5;
  bool _loaded = false;
  Timer? _saveTimer;

  @override
  void dispose() {
    _saveTimer?.cancel();
    super.dispose();
  }

  void _onSliderChanged() {
    _saveTimer?.cancel();
    _saveTimer = Timer(const Duration(milliseconds: 500), _save);
  }

  Future<void> _save() async {
    final api = ref.read(apiClientProvider);
    if (api == null) return;
    try {
      await api.put('/cameras/${widget.cameraId}/settings', data: {
        'brightness': _brightness,
        'contrast': _contrast,
        'saturation': _saturation,
        'sharpness': _sharpness,
      });
    } catch (_) {}
  }

  @override
  Widget build(BuildContext context) {
    final settingsAsync = ref.watch(imagingSettingsProvider(widget.cameraId));

    return settingsAsync.when(
      loading: () => const SizedBox.shrink(),
      error: (_, __) => const SizedBox.shrink(),
      data: (settings) {
        if (settings != null && !_loaded) {
          _brightness = settings.brightness;
          _contrast = settings.contrast;
          _saturation = settings.saturation;
          _sharpness = settings.sharpness;
          _loaded = true;
        }
        return Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            AnalogSlider(
              label: 'BRIGHTNESS',
              value: _brightness,
              onChanged: (v) {
                setState(() => _brightness = v);
                _onSliderChanged();
              },
              valueFormatter: (v) => '${(v * 100).round()}%',
            ),
            const SizedBox(height: 12),
            AnalogSlider(
              label: 'CONTRAST',
              value: _contrast,
              onChanged: (v) {
                setState(() => _contrast = v);
                _onSliderChanged();
              },
              valueFormatter: (v) => '${(v * 100).round()}%',
            ),
            const SizedBox(height: 12),
            AnalogSlider(
              label: 'SATURATION',
              value: _saturation,
              onChanged: (v) {
                setState(() => _saturation = v);
                _onSliderChanged();
              },
              valueFormatter: (v) => '${(v * 100).round()}%',
            ),
            const SizedBox(height: 12),
            AnalogSlider(
              label: 'SHARPNESS',
              value: _sharpness,
              onChanged: (v) {
                setState(() => _sharpness = v);
                _onSliderChanged();
              },
              valueFormatter: (v) => '${(v * 100).round()}%',
            ),
          ],
        );
      },
    );
  }
}
```

- [ ] **Step 3: Create relay_section.dart**

```dart
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import '../../models/device_info.dart';
import '../../providers/auth_provider.dart';
import '../../providers/onvif_providers.dart';
import '../../theme/nvr_colors.dart';
import '../../theme/nvr_typography.dart';
import '../hud/hud_toggle.dart';

class RelaySection extends ConsumerWidget {
  final String cameraId;
  const RelaySection({super.key, required this.cameraId});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final relaysAsync = ref.watch(relayOutputsProvider(cameraId));

    return relaysAsync.when(
      loading: () => const Center(child: CircularProgressIndicator(strokeWidth: 1)),
      error: (_, __) => Text('Failed to load relays', style: NvrTypography.body),
      data: (relays) {
        if (relays.isEmpty) {
          return Text('No relay outputs', style: NvrTypography.body);
        }
        return Column(
          children: [
            for (final relay in relays)
              Padding(
                padding: const EdgeInsets.only(bottom: 8),
                child: Row(
                  children: [
                    HudToggle(
                      value: relay.active,
                      onChanged: (v) async {
                        final api = ref.read(apiClientProvider);
                        if (api == null) return;
                        try {
                          await api.post(
                            '/cameras/$cameraId/relay-outputs/${relay.token}/state',
                            data: {'active': v},
                          );
                          ref.invalidate(relayOutputsProvider(cameraId));
                        } catch (_) {}
                      },
                    ),
                    const SizedBox(width: 12),
                    Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        Text(relay.token, style: NvrTypography.monoData),
                        Text('${relay.mode} / ${relay.idleState}',
                            style: NvrTypography.monoLabel),
                      ],
                    ),
                  ],
                ),
              ),
          ],
        );
      },
    );
  }
}
```

- [ ] **Step 4: Create ptz_presets_section.dart**

```dart
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import '../../models/device_info.dart';
import '../../providers/auth_provider.dart';
import '../../providers/onvif_providers.dart';
import '../../theme/nvr_colors.dart';
import '../../theme/nvr_typography.dart';
import '../hud/hud_button.dart';

class PtzPresetsSection extends ConsumerWidget {
  final String cameraId;
  const PtzPresetsSection({super.key, required this.cameraId});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final presetsAsync = ref.watch(ptzPresetsProvider(cameraId));

    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        // Home button
        Row(
          children: [
            HudButton(
              label: 'GO HOME',
              icon: Icons.home,
              style: HudButtonStyle.tactical,
              onPressed: () async {
                final api = ref.read(apiClientProvider);
                if (api == null) return;
                try {
                  await api.post('/cameras/$cameraId/ptz', data: {'action': 'home'});
                } catch (_) {}
              },
            ),
          ],
        ),
        const SizedBox(height: 12),
        // Presets list
        presetsAsync.when(
          loading: () => const Center(child: CircularProgressIndicator(strokeWidth: 1)),
          error: (_, __) => Text('Failed to load presets', style: NvrTypography.body),
          data: (presets) {
            if (presets.isEmpty) {
              return Text('No presets configured', style: NvrTypography.body);
            }
            return Column(
              children: [
                for (final preset in presets)
                  Padding(
                    padding: const EdgeInsets.only(bottom: 6),
                    child: Row(
                      children: [
                        Expanded(
                          child: Text(
                            preset.name.isNotEmpty ? preset.name : preset.token,
                            style: NvrTypography.monoData,
                          ),
                        ),
                        HudButton(
                          label: 'GO TO',
                          style: HudButtonStyle.secondary,
                          onPressed: () async {
                            final api = ref.read(apiClientProvider);
                            if (api == null) return;
                            try {
                              await api.post('/cameras/$cameraId/ptz', data: {
                                'action': 'preset',
                                'preset_token': preset.token,
                              });
                            } catch (_) {}
                          },
                        ),
                      ],
                    ),
                  ),
              ],
            );
          },
        ),
      ],
    );
  }
}
```

- [ ] **Step 5: Create audio_section.dart**

```dart
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import '../../providers/onvif_providers.dart';
import '../../theme/nvr_colors.dart';
import '../../theme/nvr_typography.dart';

class AudioSection extends ConsumerWidget {
  final String cameraId;
  const AudioSection({super.key, required this.cameraId});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final audioAsync = ref.watch(audioCapabilitiesProvider(cameraId));

    return audioAsync.when(
      loading: () => const SizedBox.shrink(),
      error: (_, __) => const SizedBox.shrink(),
      data: (caps) {
        if (caps == null) return const SizedBox.shrink();
        return Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            _statusRow('MICROPHONE', caps.audioSources > 0),
            const SizedBox(height: 6),
            _statusRow('SPEAKER / BACKCHANNEL', caps.hasBackchannel),
          ],
        );
      },
    );
  }

  Widget _statusRow(String label, bool supported) {
    return Row(
      children: [
        Icon(
          supported ? Icons.check_circle : Icons.cancel,
          size: 14,
          color: supported ? NvrColors.success : NvrColors.textMuted,
        ),
        const SizedBox(width: 8),
        Text(label, style: NvrTypography.monoLabel),
        const Spacer(),
        Text(
          supported ? 'YES' : 'NO',
          style: NvrTypography.monoData.copyWith(
            color: supported ? NvrColors.success : NvrColors.textMuted,
          ),
        ),
      ],
    );
  }
}
```

- [ ] **Step 6: Commit**

```bash
git add clients/flutter/lib/widgets/onvif/
git commit -m "feat(flutter): add ONVIF section widgets for device info, imaging, relay, PTZ presets, audio"
```

---

### Task 5: Flutter — Integrate Sections into Camera Detail Screen

**Files:**

- Modify: `clients/flutter/lib/screens/cameras/camera_detail_screen.dart`

- [ ] **Step 1: Add imports at top of camera_detail_screen.dart**

Add these imports alongside the existing imports:

```dart
import '../../widgets/onvif/device_info_section.dart';
import '../../widgets/onvif/imaging_section.dart';
import '../../widgets/onvif/relay_section.dart';
import '../../widgets/onvif/ptz_presets_section.dart';
import '../../widgets/onvif/audio_section.dart';
```

- [ ] **Step 2: Add DeviceInfoSection before the CONNECTION section**

Find the `CONNECTION` `_SectionCard` (around line 699) and insert before it:

```dart
if (_camera?.onvifEndpoint.isNotEmpty == true) ...[
  DeviceInfoSection(cameraId: widget.cameraId),
  const SizedBox(height: 12),
],
```

- [ ] **Step 3: Replace the existing IMAGING section in \_buildAdvancedSections**

Find the existing IMAGING `_ExpandableSection` (around line 838-868) and replace it with:

```dart
_ExpandableSection(
  title: 'IMAGING',
  children: [
    ImagingSection(cameraId: widget.cameraId),
  ],
),
```

- [ ] **Step 4: Replace the AUDIO section in \_buildAdvancedSections**

Find the existing AUDIO `_ExpandableSection` (around line 891-896) and replace it with:

```dart
_ExpandableSection(
  title: 'AUDIO',
  children: [
    AudioSection(cameraId: widget.cameraId),
  ],
),
```

- [ ] **Step 5: Add RELAY OUTPUTS section after AUDIO in \_buildAdvancedSections**

```dart
const SizedBox(height: 8),
if (_camera?.supportsRelay == true)
  _ExpandableSection(
    title: 'RELAY OUTPUTS',
    children: [
      RelaySection(cameraId: widget.cameraId),
    ],
  ),
```

- [ ] **Step 6: Add PTZ PRESETS section after RELAY OUTPUTS in \_buildAdvancedSections**

```dart
const SizedBox(height: 8),
if (_camera?.ptzCapable == true)
  _ExpandableSection(
    title: 'PTZ PRESETS',
    children: [
      PtzPresetsSection(cameraId: widget.cameraId),
    ],
  ),
```

- [ ] **Step 7: Remove old imaging state variables and slider code**

Remove the state variables `_brightness`, `_contrast`, `_saturation` from the State class (lines 64-66) since imaging is now managed by the `ImagingSection` widget.

- [ ] **Step 8: Build and verify Flutter app**

Run from `clients/flutter/`:

```bash
flutter analyze
flutter build macos --debug
```

Expected: No analysis errors, successful build.

- [ ] **Step 9: Commit**

```bash
git add clients/flutter/lib/screens/cameras/camera_detail_screen.dart
git commit -m "feat(flutter): integrate ONVIF sections into camera detail screen"
```

---

## Phase 2: Enhanced PTZ

### Task 6: Server — Enhanced PTZ Methods

**Files:**

- Modify: `internal/nvr/onvif/ptz.go`

- [ ] **Step 1: Add new PTZ methods to ptz.go**

Add these methods to `PTZController` after the existing `GetNodes` method:

```go
// AbsoluteMove moves the camera to an absolute position.
func (p *PTZController) AbsoluteMove(profileToken string, panPos, tiltPos, zoomPos float64) error {
	position := &onvifgo.PTZVector{
		PanTilt: &onvifgo.Vector2D{X: panPos, Y: tiltPos},
		Zoom:    &onvifgo.Vector1D{X: zoomPos},
	}

	ctx := context.Background()
	if err := p.dev.AbsoluteMove(ctx, profileToken, position, nil); err != nil {
		return fmt.Errorf("absolute move: %w", err)
	}
	return nil
}

// RelativeMove moves the camera by a relative offset.
func (p *PTZController) RelativeMove(profileToken string, panDelta, tiltDelta, zoomDelta float64) error {
	translation := &onvifgo.PTZVector{
		PanTilt: &onvifgo.Vector2D{X: panDelta, Y: tiltDelta},
		Zoom:    &onvifgo.Vector1D{X: zoomDelta},
	}

	ctx := context.Background()
	if err := p.dev.RelativeMove(ctx, profileToken, translation, nil); err != nil {
		return fmt.Errorf("relative move: %w", err)
	}
	return nil
}

// SetPreset saves the current position as a named preset. Returns the preset token.
func (p *PTZController) SetPreset(profileToken, presetName string) (string, error) {
	ctx := context.Background()
	token, err := p.dev.SetPreset(ctx, profileToken, presetName, "")
	if err != nil {
		return "", fmt.Errorf("set preset: %w", err)
	}
	return token, nil
}

// RemovePreset deletes a preset from the camera.
func (p *PTZController) RemovePreset(profileToken, presetToken string) error {
	ctx := context.Background()
	if err := p.dev.RemovePreset(ctx, profileToken, presetToken); err != nil {
		return fmt.Errorf("remove preset: %w", err)
	}
	return nil
}

// SetHomePosition saves the current position as the home position.
func (p *PTZController) SetHomePosition(profileToken string) error {
	ctx := context.Background()
	if err := p.dev.SetHomePosition(ctx, profileToken); err != nil {
		return fmt.Errorf("set home position: %w", err)
	}
	return nil
}

// PTZStatus holds the current PTZ position and movement state.
type PTZStatus struct {
	PanPosition  float64 `json:"pan_position"`
	TiltPosition float64 `json:"tilt_position"`
	ZoomPosition float64 `json:"zoom_position"`
	IsMoving     bool    `json:"is_moving"`
}

// GetStatus returns the current PTZ position and movement state.
func (p *PTZController) GetStatus(profileToken string) (*PTZStatus, error) {
	ctx := context.Background()
	status, err := p.dev.GetStatus(ctx, profileToken)
	if err != nil {
		return nil, fmt.Errorf("get PTZ status: %w", err)
	}

	result := &PTZStatus{}
	if status.Position != nil {
		if status.Position.PanTilt != nil {
			result.PanPosition = status.Position.PanTilt.X
			result.TiltPosition = status.Position.PanTilt.Y
		}
		if status.Position.Zoom != nil {
			result.ZoomPosition = status.Position.Zoom.X
		}
	}
	if status.MoveStatus != nil {
		result.IsMoving = status.MoveStatus.PanTilt != "IDLE" || status.MoveStatus.Zoom != "IDLE"
	}

	return result, nil
}
```

- [ ] **Step 2: Build and verify**

Run: `go build ./internal/nvr/onvif/`
Expected: Clean build.

- [ ] **Step 3: Commit**

```bash
git add internal/nvr/onvif/ptz.go
git commit -m "feat(onvif): add enhanced PTZ methods - absolute/relative move, set/remove preset, set home, get status"
```

---

### Task 7: Server — Enhanced PTZ API Endpoints

**Files:**

- Modify: `internal/nvr/api/cameras.go`
- Modify: `internal/nvr/api/router.go`

- [ ] **Step 1: Extend the PTZCommand handler in cameras.go**

Find the existing PTZCommand handler's switch statement on `req.Action` and add new cases:

```go
case "absolute_move":
	if err := ctrl.AbsoluteMove(profileToken, req.Pan, req.Tilt, req.Zoom); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "absolute move failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})

case "relative_move":
	if err := ctrl.RelativeMove(profileToken, req.Pan, req.Tilt, req.Zoom); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "relative move failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})

case "set_preset":
	name := req.PresetName
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "preset name is required"})
		return
	}
	token, err := ctrl.SetPreset(profileToken, name)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "set preset failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"token": token})

case "remove_preset":
	if req.PresetToken == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "preset_token is required"})
		return
	}
	if err := ctrl.RemovePreset(profileToken, req.PresetToken); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "remove preset failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})

case "set_home":
	if err := ctrl.SetHomePosition(profileToken); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "set home position failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
```

- [ ] **Step 2: Add PresetName field to the ptzRequest struct**

Find the `ptzRequest` struct and add:

```go
PresetName string `json:"name"`
```

- [ ] **Step 3: Add PTZStatus handler**

```go
// PTZStatus returns the current PTZ position and movement state.
func (h *CameraHandler) PTZStatus(c *gin.Context) {
	id := c.Param("id")
	cam, err := h.DB.GetCamera(id)
	if errors.Is(err, db.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "camera not found"})
		return
	}
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to retrieve camera", err)
		return
	}
	if cam.ONVIFEndpoint == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "camera has no ONVIF endpoint configured"})
		return
	}

	ctrl, err := onvif.NewPTZController(cam.ONVIFEndpoint, cam.ONVIFUsername, h.decryptPassword(cam.ONVIFPassword))
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "failed to connect to camera"})
		return
	}

	profileToken := cam.ONVIFProfileToken
	if profileToken == "" {
		profileToken = "000"
	}

	status, err := ctrl.GetStatus(profileToken)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "failed to get PTZ status"})
		return
	}

	c.JSON(http.StatusOK, status)
}
```

- [ ] **Step 4: Register PTZ status route in router.go**

```go
protected.GET("/cameras/:id/ptz/status", cameraHandler.PTZStatus)
```

- [ ] **Step 5: Build and verify**

Run: `go build ./...`
Expected: Clean build.

- [ ] **Step 6: Commit**

```bash
git add internal/nvr/api/cameras.go internal/nvr/api/router.go
git commit -m "feat(api): add enhanced PTZ endpoints - absolute/relative move, set/remove preset, set home, status"
```

---

### Task 8: Flutter — Enhanced PTZ UI

**Files:**

- Create: `clients/flutter/lib/models/ptz_status.dart`
- Create: `clients/flutter/lib/widgets/onvif/ptz_enhanced_section.dart`
- Modify: `clients/flutter/lib/providers/onvif_providers.dart`
- Modify: `clients/flutter/lib/screens/cameras/camera_detail_screen.dart`

- [ ] **Step 1: Create ptz_status.dart model**

```dart
class PtzStatus {
  final double panPosition;
  final double tiltPosition;
  final double zoomPosition;
  final bool isMoving;

  const PtzStatus({
    required this.panPosition,
    required this.tiltPosition,
    required this.zoomPosition,
    required this.isMoving,
  });

  factory PtzStatus.fromJson(Map<String, dynamic> json) {
    return PtzStatus(
      panPosition: (json['pan_position'] as num?)?.toDouble() ?? 0,
      tiltPosition: (json['tilt_position'] as num?)?.toDouble() ?? 0,
      zoomPosition: (json['zoom_position'] as num?)?.toDouble() ?? 0,
      isMoving: json['is_moving'] as bool? ?? false,
    );
  }
}
```

- [ ] **Step 2: Add PTZ status provider to onvif_providers.dart**

```dart
import '../models/ptz_status.dart';

final ptzStatusProvider =
    FutureProvider.family<PtzStatus?, String>((ref, cameraId) async {
  final api = ref.watch(apiClientProvider);
  if (api == null) return null;
  try {
    final res = await api.get('/cameras/$cameraId/ptz/status');
    return PtzStatus.fromJson(res.data as Map<String, dynamic>);
  } catch (_) {
    return null;
  }
});
```

- [ ] **Step 3: Create ptz_enhanced_section.dart**

This adds preset management (save/delete) and position display on top of the existing PTZ presets section:

```dart
import 'dart:async';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import '../../models/device_info.dart';
import '../../models/ptz_status.dart';
import '../../providers/auth_provider.dart';
import '../../providers/onvif_providers.dart';
import '../../theme/nvr_colors.dart';
import '../../theme/nvr_typography.dart';
import '../hud/hud_button.dart';

class PtzEnhancedSection extends ConsumerStatefulWidget {
  final String cameraId;
  const PtzEnhancedSection({super.key, required this.cameraId});

  @override
  ConsumerState<PtzEnhancedSection> createState() => _PtzEnhancedSectionState();
}

class _PtzEnhancedSectionState extends ConsumerState<PtzEnhancedSection> {
  Timer? _pollTimer;
  PtzStatus? _status;

  @override
  void initState() {
    super.initState();
    _pollTimer = Timer.periodic(const Duration(seconds: 2), (_) => _pollStatus());
    _pollStatus();
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
      if (mounted) {
        setState(() => _status = PtzStatus.fromJson(res.data as Map<String, dynamic>));
      }
    } catch (_) {}
  }

  Future<void> _savePreset() async {
    final name = await _showNameDialog();
    if (name == null || name.isEmpty) return;
    final api = ref.read(apiClientProvider);
    if (api == null) return;
    try {
      await api.post('/cameras/${widget.cameraId}/ptz', data: {
        'action': 'set_preset',
        'name': name,
      });
      ref.invalidate(ptzPresetsProvider(widget.cameraId));
    } catch (_) {}
  }

  Future<void> _removePreset(String token) async {
    final api = ref.read(apiClientProvider);
    if (api == null) return;
    try {
      await api.post('/cameras/${widget.cameraId}/ptz', data: {
        'action': 'remove_preset',
        'preset_token': token,
      });
      ref.invalidate(ptzPresetsProvider(widget.cameraId));
    } catch (_) {}
  }

  Future<void> _setHome() async {
    final api = ref.read(apiClientProvider);
    if (api == null) return;
    try {
      await api.post('/cameras/${widget.cameraId}/ptz', data: {'action': 'set_home'});
    } catch (_) {}
  }

  Future<String?> _showNameDialog() {
    final controller = TextEditingController();
    return showDialog<String>(
      context: context,
      builder: (ctx) => AlertDialog(
        backgroundColor: NvrColors.bgSecondary,
        title: Text('Save Preset', style: NvrTypography.pageTitle),
        content: TextField(
          controller: controller,
          autofocus: true,
          style: NvrTypography.monoData,
          decoration: const InputDecoration(hintText: 'Preset name'),
        ),
        actions: [
          TextButton(onPressed: () => Navigator.pop(ctx), child: const Text('Cancel')),
          TextButton(
            onPressed: () => Navigator.pop(ctx, controller.text),
            child: const Text('Save'),
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
        if (_status != null) ...[
          Container(
            padding: const EdgeInsets.all(8),
            decoration: BoxDecoration(
              color: NvrColors.bgTertiary,
              borderRadius: BorderRadius.circular(4),
            ),
            child: Row(
              mainAxisAlignment: MainAxisAlignment.spaceEvenly,
              children: [
                _posValue('PAN', _status!.panPosition),
                _posValue('TILT', _status!.tiltPosition),
                _posValue('ZOOM', _status!.zoomPosition),
                if (_status!.isMoving)
                  Text('MOVING', style: NvrTypography.monoStatus.copyWith(color: NvrColors.accent)),
              ],
            ),
          ),
          const SizedBox(height: 12),
        ],
        // Home controls
        Row(
          children: [
            HudButton(
              label: 'GO HOME',
              icon: Icons.home,
              style: HudButtonStyle.tactical,
              onPressed: () async {
                final api = ref.read(apiClientProvider);
                if (api == null) return;
                await api.post('/cameras/${widget.cameraId}/ptz', data: {'action': 'home'});
              },
            ),
            const SizedBox(width: 8),
            HudButton(
              label: 'SET HOME',
              icon: Icons.save,
              style: HudButtonStyle.secondary,
              onPressed: _setHome,
            ),
            const Spacer(),
            HudButton(
              label: 'SAVE PRESET',
              icon: Icons.add_location,
              style: HudButtonStyle.primary,
              onPressed: _savePreset,
            ),
          ],
        ),
        const SizedBox(height: 12),
        // Presets list
        presetsAsync.when(
          loading: () => const Center(child: CircularProgressIndicator(strokeWidth: 1)),
          error: (_, __) => Text('Failed to load presets', style: NvrTypography.body),
          data: (presets) {
            if (presets.isEmpty) {
              return Text('No presets configured', style: NvrTypography.body);
            }
            return Column(
              children: [
                for (final preset in presets)
                  Padding(
                    padding: const EdgeInsets.only(bottom: 6),
                    child: Row(
                      children: [
                        Expanded(
                          child: Text(
                            preset.name.isNotEmpty ? preset.name : preset.token,
                            style: NvrTypography.monoData,
                          ),
                        ),
                        HudButton(
                          label: 'GO TO',
                          style: HudButtonStyle.secondary,
                          onPressed: () async {
                            final api = ref.read(apiClientProvider);
                            if (api == null) return;
                            await api.post('/cameras/${widget.cameraId}/ptz', data: {
                              'action': 'preset',
                              'preset_token': preset.token,
                            });
                          },
                        ),
                        const SizedBox(width: 4),
                        IconButton(
                          icon: const Icon(Icons.delete, size: 16, color: NvrColors.danger),
                          onPressed: () => _removePreset(preset.token),
                          padding: EdgeInsets.zero,
                          constraints: const BoxConstraints(minWidth: 28, minHeight: 28),
                        ),
                      ],
                    ),
                  ),
              ],
            );
          },
        ),
      ],
    );
  }

  Widget _posValue(String label, double value) {
    return Column(
      children: [
        Text(label, style: NvrTypography.monoLabel),
        Text(value.toStringAsFixed(2), style: NvrTypography.monoData),
      ],
    );
  }
}
```

- [ ] **Step 4: Update camera detail screen to use PtzEnhancedSection**

Replace the PTZ PRESETS `_ExpandableSection` added in Task 5 with:

```dart
if (_camera?.ptzCapable == true)
  _ExpandableSection(
    title: 'PTZ CONTROL',
    children: [
      PtzEnhancedSection(cameraId: widget.cameraId),
    ],
  ),
```

Add import:

```dart
import '../../widgets/onvif/ptz_enhanced_section.dart';
```

- [ ] **Step 5: Build and verify**

```bash
cd clients/flutter && flutter analyze
```

- [ ] **Step 6: Commit**

```bash
git add clients/flutter/lib/models/ptz_status.dart clients/flutter/lib/widgets/onvif/ptz_enhanced_section.dart clients/flutter/lib/providers/onvif_providers.dart clients/flutter/lib/screens/cameras/camera_detail_screen.dart
git commit -m "feat(flutter): add enhanced PTZ UI with position display, preset management, set home"
```

---

## Phase 3: Media Configuration

### Task 9: Server — Media Configuration ONVIF Methods

**Files:**

- Create: `internal/nvr/onvif/media_config.go`

- [ ] **Step 1: Create media_config.go**

```go
package onvif

import (
	"context"
	"fmt"

	onvifgo "github.com/0x524a/onvif-go"
)

// ProfileInfo holds full profile details including configurations.
type ProfileInfo struct {
	Token        string              `json:"token"`
	Name         string              `json:"name"`
	VideoSource  *VideoSourceInfo    `json:"video_source,omitempty"`
	VideoEncoder *VideoEncoderConfig `json:"video_encoder,omitempty"`
	AudioEncoder *AudioEncoderConfig `json:"audio_encoder,omitempty"`
	PTZConfig    *PTZConfigInfo      `json:"ptz_config,omitempty"`
}

type VideoSourceInfo struct {
	Token     string  `json:"token"`
	Framerate float64 `json:"framerate"`
	Width     int     `json:"width"`
	Height    int     `json:"height"`
}

type VideoEncoderConfig struct {
	Token            string  `json:"token"`
	Name             string  `json:"name"`
	Encoding         string  `json:"encoding"`
	Width            int     `json:"width"`
	Height           int     `json:"height"`
	Quality          float64 `json:"quality"`
	FrameRate        int     `json:"frame_rate"`
	BitrateLimit     int     `json:"bitrate_limit"`
	EncodingInterval int     `json:"encoding_interval"`
	GovLength        int     `json:"gov_length,omitempty"`
	H264Profile      string  `json:"h264_profile,omitempty"`
}

type VideoEncoderOptions struct {
	Encodings             []string     `json:"encodings"`
	Resolutions           []Resolution `json:"resolutions"`
	FrameRateRange        Range        `json:"frame_rate_range"`
	QualityRange          Range        `json:"quality_range"`
	BitrateRange          Range        `json:"bitrate_range,omitempty"`
	GovLengthRange        Range        `json:"gov_length_range,omitempty"`
	H264Profiles          []string     `json:"h264_profiles,omitempty"`
	EncodingIntervalRange Range        `json:"encoding_interval_range,omitempty"`
}

type AudioEncoderConfig struct {
	Token      string `json:"token"`
	Name       string `json:"name"`
	Encoding   string `json:"encoding"`
	Bitrate    int    `json:"bitrate"`
	SampleRate int    `json:"sample_rate"`
}

type AudioEncoderOptions struct {
	Encodings   []string `json:"encodings"`
	BitrateList []int    `json:"bitrate_list"`
	SampleRates []int    `json:"sample_rate_list"`
}

type Resolution struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

type Range struct {
	Min int `json:"min"`
	Max int `json:"max"`
}

type PTZConfigInfo struct {
	Token     string `json:"token"`
	Name      string `json:"name"`
	NodeToken string `json:"node_token"`
}

// GetProfilesFull returns all profiles with full configuration details.
func GetProfilesFull(xaddr, username, password string) ([]*ProfileInfo, error) {
	client, err := NewClient(xaddr, username, password)
	if err != nil {
		return nil, err
	}

	ctx := context.Background()
	profiles, err := client.Dev.GetProfiles(ctx)
	if err != nil {
		return nil, fmt.Errorf("get profiles: %w", err)
	}

	var result []*ProfileInfo
	for _, p := range profiles {
		info := &ProfileInfo{
			Token: p.Token,
			Name:  p.Name,
		}

		if p.VideoSourceConfiguration != nil {
			info.VideoSource = &VideoSourceInfo{
				Token: p.VideoSourceConfiguration.Token,
			}
			if p.VideoSourceConfiguration.Bounds != nil {
				info.VideoSource.Width = p.VideoSourceConfiguration.Bounds.Width
				info.VideoSource.Height = p.VideoSourceConfiguration.Bounds.Height
			}
		}

		if p.VideoEncoderConfiguration != nil {
			info.VideoEncoder = convertVideoEncoderConfig(p.VideoEncoderConfiguration)
		}

		if p.AudioEncoderConfiguration != nil {
			info.AudioEncoder = &AudioEncoderConfig{
				Token:      p.AudioEncoderConfiguration.Token,
				Name:       p.AudioEncoderConfiguration.Name,
				Encoding:   p.AudioEncoderConfiguration.Encoding,
				Bitrate:    p.AudioEncoderConfiguration.Bitrate,
				SampleRate: p.AudioEncoderConfiguration.SampleRate,
			}
		}

		if p.PTZConfiguration != nil {
			info.PTZConfig = &PTZConfigInfo{
				Token:     p.PTZConfiguration.Token,
				Name:      p.PTZConfiguration.Name,
				NodeToken: p.PTZConfiguration.NodeToken,
			}
		}

		result = append(result, info)
	}

	return result, nil
}

func convertVideoEncoderConfig(vec *onvifgo.VideoEncoderConfiguration) *VideoEncoderConfig {
	cfg := &VideoEncoderConfig{
		Token:    vec.Token,
		Name:     vec.Name,
		Encoding: vec.Encoding,
		Quality:  vec.Quality,
	}
	if vec.Resolution != nil {
		cfg.Width = vec.Resolution.Width
		cfg.Height = vec.Resolution.Height
	}
	if vec.RateControl != nil {
		cfg.FrameRate = vec.RateControl.FrameRateLimit
		cfg.BitrateLimit = vec.RateControl.BitrateLimit
		cfg.EncodingInterval = vec.RateControl.EncodingInterval
	}
	if vec.H264 != nil {
		cfg.GovLength = vec.H264.GovLength
		cfg.H264Profile = vec.H264.H264Profile
	}
	return cfg
}

// GetVideoSourcesList returns all video sources from the device.
func GetVideoSourcesList(xaddr, username, password string) ([]*VideoSourceInfo, error) {
	client, err := NewClient(xaddr, username, password)
	if err != nil {
		return nil, err
	}

	ctx := context.Background()
	sources, err := client.Dev.GetVideoSources(ctx)
	if err != nil {
		return nil, fmt.Errorf("get video sources: %w", err)
	}

	var result []*VideoSourceInfo
	for _, s := range sources {
		info := &VideoSourceInfo{
			Token:     s.Token,
			Framerate: s.Framerate,
		}
		if s.Resolution != nil {
			info.Width = s.Resolution.Width
			info.Height = s.Resolution.Height
		}
		result = append(result, info)
	}
	return result, nil
}

// GetVideoEncoderConfig returns the video encoder configuration for a given token.
func GetVideoEncoderConfig(xaddr, username, password, configToken string) (*VideoEncoderConfig, error) {
	client, err := NewClient(xaddr, username, password)
	if err != nil {
		return nil, err
	}

	ctx := context.Background()
	vec, err := client.Dev.GetVideoEncoderConfiguration(ctx, configToken)
	if err != nil {
		return nil, fmt.Errorf("get video encoder config: %w", err)
	}
	return convertVideoEncoderConfig(vec), nil
}

// SetVideoEncoderConfig updates a video encoder configuration on the device.
func SetVideoEncoderConfig(xaddr, username, password string, cfg *VideoEncoderConfig) error {
	client, err := NewClient(xaddr, username, password)
	if err != nil {
		return err
	}

	vec := &onvifgo.VideoEncoderConfiguration{
		Token:    cfg.Token,
		Name:     cfg.Name,
		Encoding: cfg.Encoding,
		Quality:  cfg.Quality,
		Resolution: &onvifgo.VideoResolution{
			Width:  cfg.Width,
			Height: cfg.Height,
		},
		RateControl: &onvifgo.VideoRateControl{
			FrameRateLimit:   cfg.FrameRate,
			BitrateLimit:     cfg.BitrateLimit,
			EncodingInterval: cfg.EncodingInterval,
		},
	}

	if cfg.H264Profile != "" {
		vec.H264 = &onvifgo.H264Configuration{
			GovLength:   cfg.GovLength,
			H264Profile: cfg.H264Profile,
		}
	}

	ctx := context.Background()
	if err := client.Dev.SetVideoEncoderConfiguration(ctx, vec, true); err != nil {
		return fmt.Errorf("set video encoder config: %w", err)
	}
	return nil
}

// GetVideoEncoderOpts returns the available options for a video encoder configuration.
func GetVideoEncoderOpts(xaddr, username, password, configToken string) (*VideoEncoderOptions, error) {
	client, err := NewClient(xaddr, username, password)
	if err != nil {
		return nil, err
	}

	ctx := context.Background()
	opts, err := client.Dev.GetVideoEncoderConfigurationOptions(ctx, configToken)
	if err != nil {
		return nil, fmt.Errorf("get video encoder options: %w", err)
	}

	result := &VideoEncoderOptions{}

	if opts.QualityRange != nil {
		result.QualityRange = Range{Min: opts.QualityRange.Min, Max: opts.QualityRange.Max}
	}

	if opts.JPEG != nil {
		result.Encodings = append(result.Encodings, "JPEG")
		for _, r := range opts.JPEG.ResolutionsAvailable {
			result.Resolutions = append(result.Resolutions, Resolution{Width: r.Width, Height: r.Height})
		}
		if opts.JPEG.FrameRateRange != nil {
			result.FrameRateRange = Range{Min: opts.JPEG.FrameRateRange.Min, Max: opts.JPEG.FrameRateRange.Max}
		}
	}

	if opts.H264 != nil {
		result.Encodings = append(result.Encodings, "H264")
		for _, r := range opts.H264.ResolutionsAvailable {
			result.Resolutions = append(result.Resolutions, Resolution{Width: r.Width, Height: r.Height})
		}
		if opts.H264.FrameRateRange != nil {
			result.FrameRateRange = Range{Min: opts.H264.FrameRateRange.Min, Max: opts.H264.FrameRateRange.Max}
		}
		if opts.H264.GovLengthRange != nil {
			result.GovLengthRange = Range{Min: opts.H264.GovLengthRange.Min, Max: opts.H264.GovLengthRange.Max}
		}
		for _, p := range opts.H264.H264ProfilesSupported {
			result.H264Profiles = append(result.H264Profiles, string(p))
		}
		if opts.H264.EncodingIntervalRange != nil {
			result.EncodingIntervalRange = Range{Min: opts.H264.EncodingIntervalRange.Min, Max: opts.H264.EncodingIntervalRange.Max}
		}
	}

	if opts.MPEG4 != nil {
		result.Encodings = append(result.Encodings, "MPEG4")
	}

	return result, nil
}

// CreateMediaProfile creates a new media profile on the device.
func CreateMediaProfile(xaddr, username, password, name string) (*ProfileInfo, error) {
	client, err := NewClient(xaddr, username, password)
	if err != nil {
		return nil, err
	}

	ctx := context.Background()
	p, err := client.Dev.CreateProfile(ctx, name, "")
	if err != nil {
		return nil, fmt.Errorf("create profile: %w", err)
	}

	return &ProfileInfo{Token: p.Token, Name: p.Name}, nil
}

// DeleteMediaProfile deletes a media profile from the device.
func DeleteMediaProfile(xaddr, username, password, token string) error {
	client, err := NewClient(xaddr, username, password)
	if err != nil {
		return err
	}

	ctx := context.Background()
	if err := client.Dev.DeleteProfile(ctx, token); err != nil {
		return fmt.Errorf("delete profile: %w", err)
	}
	return nil
}

// GetAudioEncoderCfg returns the audio encoder configuration for a given token.
func GetAudioEncoderCfg(xaddr, username, password, configToken string) (*AudioEncoderConfig, error) {
	client, err := NewClient(xaddr, username, password)
	if err != nil {
		return nil, err
	}

	ctx := context.Background()
	aec, err := client.Dev.GetAudioEncoderConfiguration(ctx, configToken)
	if err != nil {
		return nil, fmt.Errorf("get audio encoder config: %w", err)
	}

	return &AudioEncoderConfig{
		Token:      aec.Token,
		Name:       aec.Name,
		Encoding:   aec.Encoding,
		Bitrate:    aec.Bitrate,
		SampleRate: aec.SampleRate,
	}, nil
}

// SetAudioEncoderCfg updates an audio encoder configuration on the device.
func SetAudioEncoderCfg(xaddr, username, password string, cfg *AudioEncoderConfig) error {
	client, err := NewClient(xaddr, username, password)
	if err != nil {
		return err
	}

	aec := &onvifgo.AudioEncoderConfiguration{
		Token:      cfg.Token,
		Name:       cfg.Name,
		Encoding:   cfg.Encoding,
		Bitrate:    cfg.Bitrate,
		SampleRate: cfg.SampleRate,
	}

	ctx := context.Background()
	if err := client.Dev.SetAudioEncoderConfiguration(ctx, aec, true); err != nil {
		return fmt.Errorf("set audio encoder config: %w", err)
	}
	return nil
}

// AddVideoEncoderToProfile adds a video encoder configuration to a profile.
func AddVideoEncoderToProfile(xaddr, username, password, profileToken, configToken string) error {
	client, err := NewClient(xaddr, username, password)
	if err != nil {
		return err
	}

	ctx := context.Background()
	if err := client.Dev.AddVideoEncoderConfiguration(ctx, profileToken, configToken); err != nil {
		return fmt.Errorf("add video encoder to profile: %w", err)
	}
	return nil
}

// RemoveVideoEncoderFromProfile removes the video encoder from a profile.
func RemoveVideoEncoderFromProfile(xaddr, username, password, profileToken string) error {
	client, err := NewClient(xaddr, username, password)
	if err != nil {
		return err
	}

	ctx := context.Background()
	if err := client.Dev.RemoveVideoEncoderConfiguration(ctx, profileToken); err != nil {
		return fmt.Errorf("remove video encoder from profile: %w", err)
	}
	return nil
}

// AddAudioEncoderToProfile adds an audio encoder configuration to a profile.
func AddAudioEncoderToProfile(xaddr, username, password, profileToken, configToken string) error {
	client, err := NewClient(xaddr, username, password)
	if err != nil {
		return err
	}

	ctx := context.Background()
	if err := client.Dev.AddAudioEncoderConfiguration(ctx, profileToken, configToken); err != nil {
		return fmt.Errorf("add audio encoder to profile: %w", err)
	}
	return nil
}

// RemoveAudioEncoderFromProfile removes the audio encoder from a profile.
func RemoveAudioEncoderFromProfile(xaddr, username, password, profileToken string) error {
	client, err := NewClient(xaddr, username, password)
	if err != nil {
		return err
	}

	ctx := context.Background()
	if err := client.Dev.RemoveAudioEncoderConfiguration(ctx, profileToken); err != nil {
		return fmt.Errorf("remove audio encoder from profile: %w", err)
	}
	return nil
}
```

- [ ] **Step 2: Build and verify**

Run: `go build ./internal/nvr/onvif/`
Expected: Clean build.

- [ ] **Step 3: Commit**

```bash
git add internal/nvr/onvif/media_config.go
git commit -m "feat(onvif): add media configuration methods - profiles, video/audio encoder config"
```

---

### Task 10: Server — Media Configuration API Endpoints

**Files:**

- Modify: `internal/nvr/api/cameras.go`
- Modify: `internal/nvr/api/router.go`

- [ ] **Step 1: Add media config handlers to cameras.go**

Add these handler methods to `CameraHandler`. Each follows the standard pattern: extract camera ID, validate ONVIF endpoint, call onvif method, return JSON.

```go
// GetMediaProfiles returns all media profiles with full configuration.
func (h *CameraHandler) GetMediaProfiles(c *gin.Context) {
	id := c.Param("id")
	cam, err := h.DB.GetCamera(id)
	if errors.Is(err, db.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "camera not found"})
		return
	}
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to retrieve camera", err)
		return
	}
	if cam.ONVIFEndpoint == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "camera has no ONVIF endpoint configured"})
		return
	}

	profiles, err := onvif.GetProfilesFull(cam.ONVIFEndpoint, cam.ONVIFUsername, h.decryptPassword(cam.ONVIFPassword))
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "failed to get profiles"})
		return
	}
	c.JSON(http.StatusOK, profiles)
}

// CreateMediaProfile creates a new media profile on the device.
func (h *CameraHandler) CreateMediaProfile(c *gin.Context) {
	id := c.Param("id")
	cam, err := h.DB.GetCamera(id)
	if errors.Is(err, db.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "camera not found"})
		return
	}
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to retrieve camera", err)
		return
	}
	if cam.ONVIFEndpoint == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "camera has no ONVIF endpoint configured"})
		return
	}

	var req struct {
		Name string `json:"name" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
	}

	profile, err := onvif.CreateMediaProfile(cam.ONVIFEndpoint, cam.ONVIFUsername, h.decryptPassword(cam.ONVIFPassword), req.Name)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "failed to create profile"})
		return
	}
	c.JSON(http.StatusCreated, profile)
}

// DeleteMediaProfile deletes a media profile from the device.
func (h *CameraHandler) DeleteMediaProfile(c *gin.Context) {
	id := c.Param("id")
	token := c.Param("token")
	cam, err := h.DB.GetCamera(id)
	if errors.Is(err, db.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "camera not found"})
		return
	}
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to retrieve camera", err)
		return
	}
	if cam.ONVIFEndpoint == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "camera has no ONVIF endpoint configured"})
		return
	}

	if err := onvif.DeleteMediaProfile(cam.ONVIFEndpoint, cam.ONVIFUsername, h.decryptPassword(cam.ONVIFPassword), token); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "failed to delete profile"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// GetVideoSources returns all video sources from the device.
func (h *CameraHandler) GetVideoSources(c *gin.Context) {
	id := c.Param("id")
	cam, err := h.DB.GetCamera(id)
	if errors.Is(err, db.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "camera not found"})
		return
	}
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to retrieve camera", err)
		return
	}
	if cam.ONVIFEndpoint == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "camera has no ONVIF endpoint configured"})
		return
	}

	sources, err := onvif.GetVideoSourcesList(cam.ONVIFEndpoint, cam.ONVIFUsername, h.decryptPassword(cam.ONVIFPassword))
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "failed to get video sources"})
		return
	}
	c.JSON(http.StatusOK, sources)
}

// GetVideoEncoder returns the video encoder configuration for a token.
func (h *CameraHandler) GetVideoEncoder(c *gin.Context) {
	id := c.Param("id")
	token := c.Param("token")
	cam, err := h.DB.GetCamera(id)
	if errors.Is(err, db.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "camera not found"})
		return
	}
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to retrieve camera", err)
		return
	}
	if cam.ONVIFEndpoint == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "camera has no ONVIF endpoint configured"})
		return
	}

	cfg, err := onvif.GetVideoEncoderConfig(cam.ONVIFEndpoint, cam.ONVIFUsername, h.decryptPassword(cam.ONVIFPassword), token)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "failed to get video encoder config"})
		return
	}
	c.JSON(http.StatusOK, cfg)
}

// UpdateVideoEncoder updates a video encoder configuration on the device.
func (h *CameraHandler) UpdateVideoEncoder(c *gin.Context) {
	id := c.Param("id")
	cam, err := h.DB.GetCamera(id)
	if errors.Is(err, db.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "camera not found"})
		return
	}
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to retrieve camera", err)
		return
	}
	if cam.ONVIFEndpoint == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "camera has no ONVIF endpoint configured"})
		return
	}

	var cfg onvif.VideoEncoderConfig
	if err := c.ShouldBindJSON(&cfg); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid video encoder config"})
		return
	}
	cfg.Token = c.Param("token")

	if err := onvif.SetVideoEncoderConfig(cam.ONVIFEndpoint, cam.ONVIFUsername, h.decryptPassword(cam.ONVIFPassword), &cfg); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "failed to update video encoder config"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// GetVideoEncoderOptions returns available options for a video encoder configuration.
func (h *CameraHandler) GetVideoEncoderOptions(c *gin.Context) {
	id := c.Param("id")
	token := c.Param("token")
	cam, err := h.DB.GetCamera(id)
	if errors.Is(err, db.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "camera not found"})
		return
	}
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to retrieve camera", err)
		return
	}
	if cam.ONVIFEndpoint == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "camera has no ONVIF endpoint configured"})
		return
	}

	opts, err := onvif.GetVideoEncoderOpts(cam.ONVIFEndpoint, cam.ONVIFUsername, h.decryptPassword(cam.ONVIFPassword), token)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "failed to get video encoder options"})
		return
	}
	c.JSON(http.StatusOK, opts)
}
```

- [ ] **Step 2: Register media config routes in router.go**

```go
// Media configuration
protected.GET("/cameras/:id/media/profiles", cameraHandler.GetMediaProfiles)
protected.POST("/cameras/:id/media/profiles", cameraHandler.CreateMediaProfile)
protected.DELETE("/cameras/:id/media/profiles/:token", cameraHandler.DeleteMediaProfile)
protected.GET("/cameras/:id/media/video-sources", cameraHandler.GetVideoSources)
protected.GET("/cameras/:id/media/video-encoder/:token", cameraHandler.GetVideoEncoder)
protected.PUT("/cameras/:id/media/video-encoder/:token", cameraHandler.UpdateVideoEncoder)
protected.GET("/cameras/:id/media/video-encoder/:token/options", cameraHandler.GetVideoEncoderOptions)
```

- [ ] **Step 3: Build and verify**

Run: `go build ./...`
Expected: Clean build.

- [ ] **Step 4: Commit**

```bash
git add internal/nvr/api/cameras.go internal/nvr/api/router.go
git commit -m "feat(api): add media configuration endpoints - profiles CRUD, video encoder config, options"
```

---

### Task 11: Flutter — Media Configuration UI

**Files:**

- Create: `clients/flutter/lib/models/media_profile.dart`
- Create: `clients/flutter/lib/widgets/onvif/media_config_section.dart`
- Modify: `clients/flutter/lib/providers/onvif_providers.dart`
- Modify: `clients/flutter/lib/screens/cameras/camera_detail_screen.dart`

- [ ] **Step 1: Create media_profile.dart models**

Create models for ProfileInfo, VideoEncoderConfig, VideoEncoderOptions, AudioEncoderConfig, VideoSourceInfo, and Resolution that match the server JSON responses. Use plain classes with fromJson factories (same pattern as device_info.dart). All field names use snake_case JSON keys matching the Go struct tags.

- [ ] **Step 2: Add media config providers to onvif_providers.dart**

Add `mediaProfilesProvider(cameraId)` and `videoSourcesProvider(cameraId)` family providers following the same pattern as existing providers. Also add `videoEncoderOptionsProvider` as a family provider keyed on `({String cameraId, String configToken})`.

- [ ] **Step 3: Create media_config_section.dart widget**

Build a `MediaConfigSection` ConsumerStatefulWidget that:

- Loads profiles via `mediaProfilesProvider`
- Shows each profile as a tappable card with codec/resolution summary
- On tap, expands to show video encoder dropdowns and sliders populated from the options endpoint
- Includes "Save" button that PUTs to `/cameras/:id/media/video-encoder/:token`
- Includes "Add Profile" button that POSTs to `/cameras/:id/media/profiles`
- Includes delete (swipe or icon) that DELETEs `/cameras/:id/media/profiles/:token`

Follow the `StreamCard` pattern for expandable profile cards.

- [ ] **Step 4: Add MEDIA CONFIGURATION section to camera detail screen**

Add import and `_ExpandableSection` for media config in `_buildAdvancedSections`, after the ONVIF CONFIGURATION section:

```dart
const SizedBox(height: 8),
_ExpandableSection(
  title: 'MEDIA CONFIGURATION',
  children: [
    MediaConfigSection(cameraId: widget.cameraId),
  ],
),
```

- [ ] **Step 5: Build and verify**

```bash
cd clients/flutter && flutter analyze
```

- [ ] **Step 6: Commit**

```bash
git add clients/flutter/lib/models/media_profile.dart clients/flutter/lib/widgets/onvif/media_config_section.dart clients/flutter/lib/providers/onvif_providers.dart clients/flutter/lib/screens/cameras/camera_detail_screen.dart
git commit -m "feat(flutter): add media configuration UI - profile management, video encoder settings"
```

---

## Phase 4: Device Management

### Task 12: Server — Device Management ONVIF Methods

**Files:**

- Create: `internal/nvr/onvif/device_mgmt.go`

- [ ] **Step 1: Create device_mgmt.go**

```go
package onvif

import (
	"context"
	"fmt"

	onvifgo "github.com/0x524a/onvif-go"
)

type DateTimeInfo struct {
	Type           string `json:"type"`
	DaylightSaving bool   `json:"daylight_saving"`
	Timezone       string `json:"timezone"`
	UTCTime        string `json:"utc_time"`
	LocalTime      string `json:"local_time"`
}

type HostnameInfo struct {
	FromDHCP bool   `json:"from_dhcp"`
	Name     string `json:"name"`
}

type NetworkInterfaceInfo struct {
	Token   string      `json:"token"`
	Enabled bool        `json:"enabled"`
	MAC     string      `json:"mac"`
	IPv4    *IPv4Config `json:"ipv4,omitempty"`
}

type IPv4Config struct {
	Enabled bool   `json:"enabled"`
	DHCP    bool   `json:"dhcp"`
	Address string `json:"address"`
	Prefix  int    `json:"prefix_length"`
}

type NetworkProtocolInfo struct {
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
	Port    int    `json:"port"`
}

type DNSInfo struct {
	FromDHCP bool     `json:"from_dhcp"`
	Servers  []string `json:"servers"`
}

type NTPInfo struct {
	FromDHCP bool     `json:"from_dhcp"`
	Servers  []string `json:"servers"`
}

type DeviceUser struct {
	Username string `json:"username"`
	Role     string `json:"role"`
}

func GetSystemDateAndTime(xaddr, username, password string) (*DateTimeInfo, error) {
	client, err := NewClient(xaddr, username, password)
	if err != nil {
		return nil, err
	}

	ctx := context.Background()
	dt, err := client.Dev.GetSystemDateAndTime(ctx)
	if err != nil {
		return nil, fmt.Errorf("get system date and time: %w", err)
	}

	result := &DateTimeInfo{}
	if m, ok := dt.(map[string]interface{}); ok {
		if t, ok := m["DateTimeType"].(string); ok {
			result.Type = t
		}
	}
	return result, nil
}

func GetDeviceHostname(xaddr, username, password string) (*HostnameInfo, error) {
	client, err := NewClient(xaddr, username, password)
	if err != nil {
		return nil, err
	}

	ctx := context.Background()
	info, err := client.Dev.GetHostname(ctx)
	if err != nil {
		return nil, fmt.Errorf("get hostname: %w", err)
	}

	return &HostnameInfo{
		FromDHCP: info.FromDHCP,
		Name:     info.Name,
	}, nil
}

func SetDeviceHostname(xaddr, username, password, name string) error {
	client, err := NewClient(xaddr, username, password)
	if err != nil {
		return err
	}

	ctx := context.Background()
	if err := client.Dev.SetHostname(ctx, name); err != nil {
		return fmt.Errorf("set hostname: %w", err)
	}
	return nil
}

func DeviceReboot(xaddr, username, password string) (string, error) {
	client, err := NewClient(xaddr, username, password)
	if err != nil {
		return "", err
	}

	ctx := context.Background()
	msg, err := client.Dev.SystemReboot(ctx)
	if err != nil {
		return "", fmt.Errorf("reboot: %w", err)
	}
	return msg, nil
}

func GetNetworkInterfaces(xaddr, username, password string) ([]*NetworkInterfaceInfo, error) {
	client, err := NewClient(xaddr, username, password)
	if err != nil {
		return nil, err
	}

	ctx := context.Background()
	ifaces, err := client.Dev.GetNetworkInterfaces(ctx)
	if err != nil {
		return nil, fmt.Errorf("get network interfaces: %w", err)
	}

	var result []*NetworkInterfaceInfo
	for _, iface := range ifaces {
		info := &NetworkInterfaceInfo{
			Token:   iface.Token,
			Enabled: iface.Enabled,
		}
		if iface.Info != nil {
			info.MAC = iface.Info.HwAddress
		}
		if iface.IPv4 != nil && iface.IPv4.Enabled {
			info.IPv4 = &IPv4Config{
				Enabled: true,
				DHCP:    iface.IPv4.Config != nil && iface.IPv4.Config.DHCP,
			}
			if iface.IPv4.Config != nil && len(iface.IPv4.Config.Manual) > 0 {
				info.IPv4.Address = iface.IPv4.Config.Manual[0].Address
				info.IPv4.Prefix = iface.IPv4.Config.Manual[0].PrefixLength
			}
		}
		result = append(result, info)
	}
	return result, nil
}

func GetNetworkProtocols(xaddr, username, password string) ([]*NetworkProtocolInfo, error) {
	client, err := NewClient(xaddr, username, password)
	if err != nil {
		return nil, err
	}

	ctx := context.Background()
	protocols, err := client.Dev.GetNetworkProtocols(ctx)
	if err != nil {
		return nil, fmt.Errorf("get network protocols: %w", err)
	}

	var result []*NetworkProtocolInfo
	for _, p := range protocols {
		port := 0
		if len(p.Port) > 0 {
			port = p.Port[0]
		}
		result = append(result, &NetworkProtocolInfo{
			Name:    p.Name,
			Enabled: p.Enabled,
			Port:    port,
		})
	}
	return result, nil
}

func SetNetworkProtocols(xaddr, username, password string, protocols []*NetworkProtocolInfo) error {
	client, err := NewClient(xaddr, username, password)
	if err != nil {
		return err
	}

	var onvifProtos []*onvifgo.NetworkProtocol
	for _, p := range protocols {
		onvifProtos = append(onvifProtos, &onvifgo.NetworkProtocol{
			Name:    p.Name,
			Enabled: p.Enabled,
			Port:    []int{p.Port},
		})
	}

	ctx := context.Background()
	if err := client.Dev.SetNetworkProtocols(ctx, onvifProtos); err != nil {
		return fmt.Errorf("set network protocols: %w", err)
	}
	return nil
}

func GetDeviceUsers(xaddr, username, password string) ([]*DeviceUser, error) {
	client, err := NewClient(xaddr, username, password)
	if err != nil {
		return nil, err
	}

	ctx := context.Background()
	users, err := client.Dev.GetUsers(ctx)
	if err != nil {
		return nil, fmt.Errorf("get users: %w", err)
	}

	var result []*DeviceUser
	for _, u := range users {
		result = append(result, &DeviceUser{
			Username: u.Username,
			Role:     string(u.UserLevel),
		})
	}
	return result, nil
}

func CreateDeviceUser(xaddr, adminUser, adminPass, username, password, role string) error {
	client, err := NewClient(xaddr, adminUser, adminPass)
	if err != nil {
		return err
	}

	ctx := context.Background()
	if err := client.Dev.CreateUsers(ctx, []*onvifgo.User{{
		Username:  username,
		Password:  password,
		UserLevel: onvifgo.UserLevel(role),
	}}); err != nil {
		return fmt.Errorf("create user: %w", err)
	}
	return nil
}

func DeleteDeviceUser(xaddr, adminUser, adminPass, username string) error {
	client, err := NewClient(xaddr, adminUser, adminPass)
	if err != nil {
		return err
	}

	ctx := context.Background()
	if err := client.Dev.DeleteUsers(ctx, []string{username}); err != nil {
		return fmt.Errorf("delete user: %w", err)
	}
	return nil
}

func SetDeviceUser(xaddr, adminUser, adminPass, username, password, role string) error {
	client, err := NewClient(xaddr, adminUser, adminPass)
	if err != nil {
		return err
	}

	ctx := context.Background()
	if err := client.Dev.SetUser(ctx, &onvifgo.User{
		Username:  username,
		Password:  password,
		UserLevel: onvifgo.UserLevel(role),
	}); err != nil {
		return fmt.Errorf("set user: %w", err)
	}
	return nil
}

func GetDeviceScopes(xaddr, username, password string) ([]string, error) {
	client, err := NewClient(xaddr, username, password)
	if err != nil {
		return nil, err
	}

	ctx := context.Background()
	scopes, err := client.Dev.GetScopes(ctx)
	if err != nil {
		return nil, fmt.Errorf("get scopes: %w", err)
	}

	var result []string
	for _, s := range scopes {
		result = append(result, s.ScopeItem)
	}
	return result, nil
}

func GetNTPConfig(xaddr, username, password string) (*NTPInfo, error) {
	client, err := NewClient(xaddr, username, password)
	if err != nil {
		return nil, err
	}

	ctx := context.Background()
	ntp, err := client.Dev.GetNTP(ctx)
	if err != nil {
		return nil, fmt.Errorf("get NTP config: %w", err)
	}

	result := &NTPInfo{
		FromDHCP: ntp.FromDHCP,
	}
	for _, s := range ntp.NTPManual {
		if s.IPv4Address != "" {
			result.Servers = append(result.Servers, s.IPv4Address)
		} else if s.DNSname != "" {
			result.Servers = append(result.Servers, s.DNSname)
		}
	}
	return result, nil
}

func GetDNSConfig(xaddr, username, password string) (*DNSInfo, error) {
	client, err := NewClient(xaddr, username, password)
	if err != nil {
		return nil, err
	}

	ctx := context.Background()
	dns, err := client.Dev.GetDNS(ctx)
	if err != nil {
		return nil, fmt.Errorf("get DNS config: %w", err)
	}

	result := &DNSInfo{
		FromDHCP: dns.FromDHCP,
	}
	for _, s := range dns.DNSManual {
		if s.IPv4Address != "" {
			result.Servers = append(result.Servers, s.IPv4Address)
		}
	}
	return result, nil
}
```

- [ ] **Step 2: Build and verify**

Run: `go build ./internal/nvr/onvif/`

Note: Some types like `onvifgo.UserLevel`, `onvifgo.NetworkProtocol`, `onvifgo.User`, `iface.Info`, etc. may need field name adjustments based on the actual `onvif-go` library types. Fix any compilation errors by checking the library's type definitions.

- [ ] **Step 3: Commit**

```bash
git add internal/nvr/onvif/device_mgmt.go
git commit -m "feat(onvif): add device management methods - datetime, hostname, network, users, scopes, reboot"
```

---

### Task 13: Server — Device Management API Endpoints

**Files:**

- Modify: `internal/nvr/api/cameras.go`
- Modify: `internal/nvr/api/router.go`

- [ ] **Step 1: Add device management handlers to cameras.go**

Add handlers following the standard pattern. Each handler: extracts camera ID, validates ONVIF endpoint, calls the corresponding `onvif.` function, returns JSON. Create handlers for:

- `GetDeviceDateTime` → calls `onvif.GetSystemDateAndTime`
- `GetDeviceHostnameHandler` → calls `onvif.GetDeviceHostname`
- `SetDeviceHostnameHandler` → calls `onvif.SetDeviceHostname`
- `RebootDevice` → calls `onvif.DeviceReboot` (with confirmation via request body `{"confirm": true}`)
- `GetDeviceScopes` → calls `onvif.GetDeviceScopes`
- `GetNetworkInterfacesHandler` → calls `onvif.GetNetworkInterfaces`
- `GetNetworkProtocolsHandler` → calls `onvif.GetNetworkProtocols`
- `SetNetworkProtocolsHandler` → calls `onvif.SetNetworkProtocols`
- `GetDNSConfigHandler` → calls `onvif.GetDNSConfig`
- `GetNTPConfigHandler` → calls `onvif.GetNTPConfig`
- `GetDeviceUsersHandler` → calls `onvif.GetDeviceUsers`
- `CreateDeviceUserHandler` → calls `onvif.CreateDeviceUser`
- `UpdateDeviceUserHandler` → calls `onvif.SetDeviceUser`
- `DeleteDeviceUserHandler` → calls `onvif.DeleteDeviceUser`

- [ ] **Step 2: Register device management routes in router.go**

```go
// Device management
protected.GET("/cameras/:id/device/datetime", cameraHandler.GetDeviceDateTime)
protected.GET("/cameras/:id/device/hostname", cameraHandler.GetDeviceHostnameHandler)
protected.PUT("/cameras/:id/device/hostname", cameraHandler.SetDeviceHostnameHandler)
protected.POST("/cameras/:id/device/reboot", cameraHandler.RebootDevice)
protected.GET("/cameras/:id/device/scopes", cameraHandler.GetDeviceScopes)
protected.GET("/cameras/:id/device/network/interfaces", cameraHandler.GetNetworkInterfacesHandler)
protected.GET("/cameras/:id/device/network/protocols", cameraHandler.GetNetworkProtocolsHandler)
protected.PUT("/cameras/:id/device/network/protocols", cameraHandler.SetNetworkProtocolsHandler)
protected.GET("/cameras/:id/device/network/dns", cameraHandler.GetDNSConfigHandler)
protected.GET("/cameras/:id/device/network/ntp", cameraHandler.GetNTPConfigHandler)
protected.GET("/cameras/:id/device/users", cameraHandler.GetDeviceUsersHandler)
protected.POST("/cameras/:id/device/users", cameraHandler.CreateDeviceUserHandler)
protected.PUT("/cameras/:id/device/users/:username", cameraHandler.UpdateDeviceUserHandler)
protected.DELETE("/cameras/:id/device/users/:username", cameraHandler.DeleteDeviceUserHandler)
```

- [ ] **Step 3: Build and verify**

Run: `go build ./...`

- [ ] **Step 4: Commit**

```bash
git add internal/nvr/api/cameras.go internal/nvr/api/router.go
git commit -m "feat(api): add device management endpoints - datetime, hostname, network, users, reboot"
```

---

### Task 14: Flutter — Device Management UI

**Files:**

- Create: `clients/flutter/lib/models/device_management.dart`
- Create: `clients/flutter/lib/widgets/onvif/device_mgmt_section.dart`
- Modify: `clients/flutter/lib/providers/onvif_providers.dart`
- Modify: `clients/flutter/lib/screens/cameras/camera_detail_screen.dart`

- [ ] **Step 1: Create device_management.dart models**

Create plain class models for `DateTimeInfo`, `HostnameInfo`, `NetworkInterfaceInfo`, `IPv4Config`, `NetworkProtocolInfo`, `DNSInfo`, `NTPInfo`, and `DeviceUser`. Each with `fromJson` factory. Field names match the server JSON keys.

- [ ] **Step 2: Add device management providers to onvif_providers.dart**

Add family providers for:

- `deviceDateTimeProvider(cameraId)`
- `deviceHostnameProvider(cameraId)`
- `networkInterfacesProvider(cameraId)`
- `networkProtocolsProvider(cameraId)`
- `deviceUsersProvider(cameraId)`
- `ntpConfigProvider(cameraId)`

All follow the same pattern as existing providers.

- [ ] **Step 3: Create device_mgmt_section.dart widget**

Build a `DeviceMgmtSection` ConsumerStatefulWidget with three sub-sections:

**System:** Shows date/time info, hostname field with save button, "Reboot Device" danger button with confirmation dialog.

**Network:** Read-only interface list (token, MAC, IP, DHCP), protocol list with enabled toggles and port fields with save button.

**Device Users:** User list with role badges, "Add User" button opening a dialog, edit/delete per user.

Use `_SectionCard`-like containers for each sub-section, `HudToggle` for toggles, `HudButton` for actions, standard TextField for inputs.

- [ ] **Step 4: Add DEVICE MANAGEMENT section to camera detail screen**

Add import and `_ExpandableSection` in `_buildAdvancedSections`:

```dart
const SizedBox(height: 8),
_ExpandableSection(
  title: 'DEVICE MANAGEMENT',
  children: [
    DeviceMgmtSection(cameraId: widget.cameraId),
  ],
),
```

- [ ] **Step 5: Build and verify**

```bash
cd clients/flutter && flutter analyze
```

- [ ] **Step 6: Commit**

```bash
git add clients/flutter/lib/models/device_management.dart clients/flutter/lib/widgets/onvif/device_mgmt_section.dart clients/flutter/lib/providers/onvif_providers.dart clients/flutter/lib/screens/cameras/camera_detail_screen.dart
git commit -m "feat(flutter): add device management UI - system, network, device users"
```

---

## Final Verification

### Task 15: Full Build and Integration Test

- [ ] **Step 1: Full Go build**

```bash
go build ./...
```

- [ ] **Step 2: Run existing Go tests**

```bash
go test ./internal/nvr/onvif/ -v
```

- [ ] **Step 3: Full Flutter build**

```bash
cd clients/flutter && flutter analyze && flutter build macos --debug
```

- [ ] **Step 4: Final commit with any fixes**

If any fixes were needed, commit them.
