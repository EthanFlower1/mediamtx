# Camera Discovery Redesign Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the minimal Flutter camera discovery flow with a rich, multi-step experience that shows auth status, available streams, and camera capabilities before adding.

**Architecture:** Small backend changes (add `auth_required` field to discovery enrichment, cross-reference existing cameras in results endpoint). Main work is the Flutter UI: replace the discover tab's flat list with rich cards + a detail bottom sheet with credentials, stream selection, and capabilities display. All existing API endpoints are reused.

**Tech Stack:** Go (backend), Flutter/Dart with Riverpod (client), ONVIF WS-Discovery

---

### Task 1: Backend — Add `AuthRequired` field to `DiscoveredDevice`

**Files:**

- Modify: `internal/nvr/onvif/discovery.go:33-39` (DiscoveredDevice struct)
- Modify: `internal/nvr/onvif/discovery.go:180-242` (enrichDevice method)

- [ ] **Step 1: Add `AuthRequired` field to the struct**

In `internal/nvr/onvif/discovery.go`, update the `DiscoveredDevice` struct:

```go
// DiscoveredDevice represents an ONVIF device found during a WS-Discovery scan.
type DiscoveredDevice struct {
	XAddr        string         `json:"xaddr"`
	Manufacturer string         `json:"manufacturer"`
	Model        string         `json:"model"`
	Firmware     string         `json:"firmware"`
	AuthRequired bool           `json:"auth_required"`
	Profiles     []MediaProfile `json:"profiles,omitempty"`
}
```

- [ ] **Step 2: Update `enrichDevice` to set `AuthRequired`**

Replace the `enrichDevice` method to set `AuthRequired = true` when profile fetch fails (likely auth error) instead of silently returning:

```go
func (d *Discovery) enrichDevice(dev *DiscoveredDevice) {
	xaddr := xaddrToHost(dev.XAddr)
	if xaddr == "" {
		return
	}

	onvifDev, err := onviflib.NewDevice(onviflib.DeviceParams{
		Xaddr: xaddr,
	})
	if err != nil {
		dev.AuthRequired = true
		return
	}

	ctx := context.Background()

	// Fetch device info to fill in manufacturer/model/firmware if not from scopes.
	info, err := sdkdevice.Call_GetDeviceInformation(ctx, onvifDev, onvifdevice.GetDeviceInformation{})
	if err == nil {
		if dev.Manufacturer == "" {
			dev.Manufacturer = info.Manufacturer
		}
		if dev.Model == "" {
			dev.Model = info.Model
		}
		if dev.Firmware == "" {
			dev.Firmware = info.FirmwareVersion
		}
	}

	// Fetch media profiles — if this fails, the device likely requires auth.
	profilesResp, err := sdkmedia.Call_GetProfiles(ctx, onvifDev, onvifmedia.GetProfiles{})
	if err != nil {
		dev.AuthRequired = true
		return
	}

	for _, p := range profilesResp.Profiles {
		mp := MediaProfile{
			Token: string(p.Token),
			Name:  string(p.Name),
		}

		enc := p.VideoEncoderConfiguration
		mp.VideoCodec = string(enc.Encoding)
		mp.Width = int(enc.Resolution.Width)
		mp.Height = int(enc.Resolution.Height)

		streamResp, err := sdkmedia.Call_GetStreamUri(ctx, onvifDev, onvifmedia.GetStreamUri{
			ProfileToken: p.Token,
			StreamSetup: onviftypes.StreamSetup{
				Stream:    "RTP-Unicast",
				Transport: onviftypes.Transport{Protocol: "RTSP"},
			},
		})
		if err == nil {
			mp.StreamURI = string(streamResp.MediaUri.Uri)
		}

		dev.Profiles = append(dev.Profiles, mp)
	}
}
```

- [ ] **Step 3: Verify Go compiles**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go build ./internal/nvr/onvif/`
Expected: No errors

- [ ] **Step 4: Commit**

```bash
git add internal/nvr/onvif/discovery.go
git commit -m "feat(discovery): add auth_required field to DiscoveredDevice"
```

---

### Task 2: Backend — Cross-reference existing cameras in discovery results

**Files:**

- Modify: `internal/nvr/onvif/discovery.go:33-39` (add ExistingCameraID field)
- Modify: `internal/nvr/api/cameras.go:519-527` (DiscoverResults handler)

- [ ] **Step 1: Add `ExistingCameraID` field to struct**

In `internal/nvr/onvif/discovery.go`, add to `DiscoveredDevice`:

```go
type DiscoveredDevice struct {
	XAddr            string         `json:"xaddr"`
	Manufacturer     string         `json:"manufacturer"`
	Model            string         `json:"model"`
	Firmware         string         `json:"firmware"`
	AuthRequired     bool           `json:"auth_required"`
	ExistingCameraID string         `json:"existing_camera_id,omitempty"`
	Profiles         []MediaProfile `json:"profiles,omitempty"`
}
```

- [ ] **Step 2: Update `DiscoverResults` handler to cross-reference**

In `internal/nvr/api/cameras.go`, replace the `DiscoverResults` method:

```go
func (h *CameraHandler) DiscoverResults(c *gin.Context) {
	if h.Discovery == nil {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "ONVIF discovery not available"})
		return
	}

	results := h.Discovery.GetResults()

	// Cross-reference with existing cameras by ONVIF endpoint.
	cameras, err := h.DB.ListCameras()
	if err == nil {
		endpointToID := make(map[string]string)
		for _, cam := range cameras {
			if cam.ONVIFEndpoint != "" {
				endpointToID[cam.ONVIFEndpoint] = cam.ID
			}
		}
		for i := range results {
			if id, ok := endpointToID[results[i].XAddr]; ok {
				results[i].ExistingCameraID = id
			}
		}
	}

	c.JSON(http.StatusOK, results)
}
```

- [ ] **Step 3: Verify Go compiles**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go build ./internal/nvr/...`
Expected: No errors

- [ ] **Step 4: Commit**

```bash
git add internal/nvr/onvif/discovery.go internal/nvr/api/cameras.go
git commit -m "feat(discovery): cross-reference existing cameras in results"
```

---

### Task 3: Flutter — Discovery result card widget

**Files:**

- Create: `clients/flutter/lib/screens/cameras/discovery_card.dart`

- [ ] **Step 1: Create the discovery card widget**

Create `clients/flutter/lib/screens/cameras/discovery_card.dart`:

```dart
import 'package:flutter/material.dart';
import '../../theme/nvr_colors.dart';
import '../../theme/nvr_typography.dart';

class DiscoveryCard extends StatelessWidget {
  final Map<String, dynamic> device;
  final VoidCallback onTap;

  const DiscoveryCard({
    super.key,
    required this.device,
    required this.onTap,
  });

  String get _name {
    final mfr = device['manufacturer'] as String? ?? '';
    final model = device['model'] as String? ?? '';
    if (mfr.isNotEmpty && model.isNotEmpty) return '$mfr $model';
    if (model.isNotEmpty) return model;
    if (mfr.isNotEmpty) return mfr;
    return 'Unknown Camera';
  }

  String get _ip {
    final xaddr = device['xaddr'] as String? ?? '';
    final uri = Uri.tryParse(xaddr);
    return uri?.host ?? xaddr;
  }

  String get _subtitle {
    final mfr = device['manufacturer'] as String? ?? '';
    final model = device['model'] as String? ?? '';
    final fw = device['firmware'] as String? ?? '';
    final parts = <String>[
      _ip,
      if (mfr.isNotEmpty) mfr,
      if (model.isNotEmpty) model,
      if (fw.isNotEmpty) 'FW $fw',
    ];
    return parts.join(' · ');
  }

  bool get _authRequired => device['auth_required'] == true;
  bool get _alreadyAdded =>
      (device['existing_camera_id'] as String? ?? '').isNotEmpty;

  List<String> get _capabilities {
    final profiles = device['profiles'] as List<dynamic>? ?? [];
    final caps = <String>[];
    if (profiles.isNotEmpty) {
      caps.add('${profiles.length} STREAM${profiles.length == 1 ? '' : 'S'}');
    }
    return caps;
  }

  @override
  Widget build(BuildContext context) {
    final isAdded = _alreadyAdded;

    return GestureDetector(
      onTap: onTap,
      child: Opacity(
        opacity: isAdded ? 0.5 : 1.0,
        child: Container(
          padding: const EdgeInsets.all(12),
          decoration: BoxDecoration(
            color: NvrColors.bgSecondary,
            borderRadius: BorderRadius.circular(6),
            border: Border.all(color: NvrColors.border),
          ),
          child: Row(
            children: [
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    // Name + status badge
                    Row(
                      children: [
                        Flexible(
                          child: Text(
                            _name,
                            style: NvrTypography.cameraName,
                            overflow: TextOverflow.ellipsis,
                          ),
                        ),
                        const SizedBox(width: 8),
                        _statusBadge(),
                      ],
                    ),
                    const SizedBox(height: 4),
                    // IP · Manufacturer · Model
                    Text(_subtitle, style: NvrTypography.monoLabel),
                    // Capability pills
                    if (!_authRequired && _capabilities.isNotEmpty) ...[
                      const SizedBox(height: 6),
                      Wrap(
                        spacing: 4,
                        runSpacing: 4,
                        children: _capabilities
                            .map((c) => _capBadge(c))
                            .toList(),
                      ),
                    ],
                    if (_authRequired)
                      Padding(
                        padding: const EdgeInsets.only(top: 6),
                        child: Text(
                          'Enter credentials to see streams',
                          style: NvrTypography.monoLabel.copyWith(
                            fontStyle: FontStyle.italic,
                            color: NvrColors.textSecondary,
                          ),
                        ),
                      ),
                  ],
                ),
              ),
              const SizedBox(width: 8),
              Icon(
                Icons.chevron_right,
                color: isAdded ? NvrColors.textMuted : NvrColors.textSecondary,
                size: 20,
              ),
            ],
          ),
        ),
      ),
    );
  }

  Widget _statusBadge() {
    if (_alreadyAdded) {
      return _badge('ADDED', NvrColors.accent, NvrColors.accent);
    }
    if (_authRequired) {
      return _badge('AUTH REQUIRED', NvrColors.danger, NvrColors.danger);
    }
    return _badge('OPEN', NvrColors.success, NvrColors.success);
  }

  Widget _badge(String text, Color textColor, Color borderColor) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 2),
      decoration: BoxDecoration(
        color: borderColor.withValues(alpha: 0.1),
        borderRadius: BorderRadius.circular(3),
        border: Border.all(color: borderColor.withValues(alpha: 0.3)),
      ),
      child: Text(
        text,
        style: TextStyle(
          fontFamily: 'JetBrainsMono',
          fontSize: 8,
          fontWeight: FontWeight.w600,
          letterSpacing: 0.5,
          color: textColor,
        ),
      ),
    );
  }

  Widget _capBadge(String label) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 2),
      decoration: BoxDecoration(
        color: NvrColors.bgTertiary,
        borderRadius: BorderRadius.circular(3),
        border: Border.all(color: NvrColors.border),
      ),
      child: Text(
        label,
        style: NvrTypography.monoLabel.copyWith(
          color: NvrColors.textSecondary,
          fontSize: 8,
        ),
      ),
    );
  }
}
```

- [ ] **Step 2: Verify Flutter analysis passes**

Run: `cd /Users/ethanflower/personal_projects/mediamtx/clients/flutter && flutter analyze lib/screens/cameras/discovery_card.dart`
Expected: No issues found

- [ ] **Step 3: Commit**

```bash
git add clients/flutter/lib/screens/cameras/discovery_card.dart
git commit -m "feat(flutter): add rich discovery result card widget"
```

---

### Task 4: Flutter — Camera detail bottom sheet

**Files:**

- Create: `clients/flutter/lib/screens/cameras/camera_detail_sheet.dart`

- [ ] **Step 1: Create the detail bottom sheet widget**

Create `clients/flutter/lib/screens/cameras/camera_detail_sheet.dart`:

```dart
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../providers/auth_provider.dart';
import '../../providers/cameras_provider.dart';
import '../../theme/nvr_colors.dart';
import '../../theme/nvr_typography.dart';
import '../../widgets/hud/hud_button.dart';

class CameraDetailSheet extends ConsumerStatefulWidget {
  final Map<String, dynamic> device;

  const CameraDetailSheet({super.key, required this.device});

  static Future<void> show(BuildContext context, Map<String, dynamic> device) {
    return showModalBottomSheet<void>(
      context: context,
      isScrollControlled: true,
      backgroundColor: NvrColors.bgSecondary,
      shape: const RoundedRectangleBorder(
        borderRadius: BorderRadius.vertical(top: Radius.circular(12)),
      ),
      builder: (_) => DraggableScrollableSheet(
        expand: false,
        initialChildSize: 0.85,
        maxChildSize: 0.95,
        minChildSize: 0.5,
        builder: (context, scrollController) => CameraDetailSheet(
          device: device,
        ),
      ),
    );
  }

  @override
  ConsumerState<CameraDetailSheet> createState() => _CameraDetailSheetState();
}

class _CameraDetailSheetState extends ConsumerState<CameraDetailSheet> {
  final _userCtrl = TextEditingController();
  final _passCtrl = TextEditingController();
  final _nameCtrl = TextEditingController();
  bool _obscurePass = true;

  bool _probing = false;
  String? _probeError;
  List<Map<String, dynamic>> _profiles = [];
  Map<String, dynamic>? _capabilities;
  int _selectedProfileIndex = 0;
  bool _adding = false;

  @override
  void initState() {
    super.initState();
    final mfr = widget.device['manufacturer'] as String? ?? '';
    final model = widget.device['model'] as String? ?? '';
    _nameCtrl.text = [mfr, model].where((s) => s.isNotEmpty).join(' ');

    // If profiles already available (open camera), populate them.
    final existing = widget.device['profiles'] as List<dynamic>? ?? [];
    if (existing.isNotEmpty) {
      _profiles = existing.cast<Map<String, dynamic>>();
    }
  }

  @override
  void dispose() {
    _userCtrl.dispose();
    _passCtrl.dispose();
    _nameCtrl.dispose();
    super.dispose();
  }

  String get _ip {
    final xaddr = widget.device['xaddr'] as String? ?? '';
    final uri = Uri.tryParse(xaddr);
    return uri?.host ?? xaddr;
  }

  bool get _authRequired => widget.device['auth_required'] == true;
  bool get _alreadyAdded =>
      (widget.device['existing_camera_id'] as String? ?? '').isNotEmpty;

  Future<void> _probe() async {
    final api = ref.read(apiClientProvider);
    if (api == null) return;

    setState(() {
      _probing = true;
      _probeError = null;
    });

    try {
      final res = await api.post<Map<String, dynamic>>('/cameras/probe', data: {
        'xaddr': widget.device['xaddr'],
        'username': _userCtrl.text.trim(),
        'password': _passCtrl.text,
      });

      final data = res.data ?? {};
      final profiles = (data['profiles'] as List<dynamic>? ?? [])
          .cast<Map<String, dynamic>>();

      setState(() {
        _profiles = profiles;
        _capabilities = data['capabilities'] as Map<String, dynamic>?;
        _selectedProfileIndex = 0;
        _probing = false;
      });
    } catch (e) {
      setState(() {
        _probeError = e.toString().replaceFirst('DioException [unknown]:', '').trim();
        _probing = false;
      });
    }
  }

  Future<void> _addCamera() async {
    final api = ref.read(apiClientProvider);
    if (api == null) return;
    if (_profiles.isEmpty) return;

    final profile = _profiles[_selectedProfileIndex];
    var rtspUrl = profile['stream_uri'] as String? ?? '';

    // Inject credentials into RTSP URL if provided.
    final user = _userCtrl.text.trim();
    final pass = _passCtrl.text;
    if (user.isNotEmpty && rtspUrl.startsWith('rtsp://')) {
      final uri = Uri.parse(rtspUrl);
      final authed = uri.replace(userInfo: '$user:$pass');
      rtspUrl = authed.toString();
    }

    setState(() => _adding = true);

    try {
      await api.post('/cameras', data: {
        'name': _nameCtrl.text.trim(),
        'rtsp_url': rtspUrl,
        'onvif_endpoint': widget.device['xaddr'] ?? '',
        'onvif_username': user,
        'onvif_password': pass,
        'onvif_profile_token': profile['token'] ?? '',
      });
      ref.invalidate(camerasProvider);
      if (mounted) {
        Navigator.of(context).pop(); // close sheet
        Navigator.of(context).pop(); // back to camera list
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

  @override
  Widget build(BuildContext context) {
    return Container(
      color: NvrColors.bgSecondary,
      child: ListView(
        padding: const EdgeInsets.fromLTRB(16, 8, 16, 32),
        children: [
          // Drag handle
          Center(
            child: Container(
              width: 40,
              height: 4,
              margin: const EdgeInsets.only(bottom: 16),
              decoration: BoxDecoration(
                color: NvrColors.bgTertiary,
                borderRadius: BorderRadius.circular(2),
              ),
            ),
          ),

          // ── Header ──────────────────────────────────────────────
          _buildHeader(),
          const SizedBox(height: 20),

          // ── Credentials ─────────────────────────────────────────
          _buildCredentialsSection(),
          const SizedBox(height: 20),

          // ── Streams ─────────────────────────────────────────────
          if (_profiles.isNotEmpty) ...[
            _buildStreamsSection(),
            const SizedBox(height: 20),
          ],

          // ── Capabilities ────────────────────────────────────────
          if (_capabilities != null) ...[
            _buildCapabilitiesSection(),
            const SizedBox(height: 20),
          ],

          // ── Camera name ─────────────────────────────────────────
          if (_profiles.isNotEmpty) ...[
            _buildNameSection(),
            const SizedBox(height: 20),

            // ── Add button ──────────────────────────────────────────
            _buildAddButton(),
          ],
        ],
      ),
    );
  }

  Widget _buildHeader() {
    final mfr = widget.device['manufacturer'] as String? ?? '';
    final model = widget.device['model'] as String? ?? '';
    final fw = widget.device['firmware'] as String? ?? '';
    final name = [mfr, model].where((s) => s.isNotEmpty).join(' ');
    final sub = [_ip, if (mfr.isNotEmpty) mfr, if (fw.isNotEmpty) 'FW $fw']
        .join(' · ');

    return Row(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Expanded(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(
                name.isNotEmpty ? name : 'Unknown Camera',
                style: NvrTypography.pageTitle,
              ),
              const SizedBox(height: 4),
              Text(sub, style: NvrTypography.monoLabel),
            ],
          ),
        ),
        if (_alreadyAdded)
          _badge('ALREADY ADDED', NvrColors.accent)
        else if (_authRequired && _profiles.isEmpty)
          _badge('AUTH REQUIRED', NvrColors.danger)
        else if (_profiles.isNotEmpty)
          _badge('OPEN', NvrColors.success),
      ],
    );
  }

  Widget _buildCredentialsSection() {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        const Text('CREDENTIALS', style: NvrTypography.monoSection),
        const SizedBox(height: 8),
        Row(
          children: [
            Expanded(
              child: _field(
                controller: _userCtrl,
                hint: 'Username',
              ),
            ),
            const SizedBox(width: 8),
            Expanded(
              child: _field(
                controller: _passCtrl,
                hint: 'Password',
                obscure: _obscurePass,
                suffix: IconButton(
                  icon: Icon(
                    _obscurePass ? Icons.visibility_off : Icons.visibility,
                    color: NvrColors.textMuted,
                    size: 16,
                  ),
                  onPressed: () =>
                      setState(() => _obscurePass = !_obscurePass),
                  padding: EdgeInsets.zero,
                  constraints:
                      const BoxConstraints(minWidth: 32, minHeight: 32),
                ),
              ),
            ),
          ],
        ),
        const SizedBox(height: 10),
        SizedBox(
          width: double.infinity,
          child: HudButton(
            label: _probing ? 'PROBING...' : 'PROBE CAMERA',
            icon: Icons.wifi_find,
            onPressed: _probing ? null : _probe,
          ),
        ),
        if (_probeError != null)
          Padding(
            padding: const EdgeInsets.only(top: 8),
            child: Text(
              _probeError!,
              style: NvrTypography.body.copyWith(color: NvrColors.danger),
            ),
          ),
      ],
    );
  }

  Widget _buildStreamsSection() {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(
          'AVAILABLE STREAMS',
          style: NvrTypography.monoSection,
        ),
        const SizedBox(height: 8),
        for (int i = 0; i < _profiles.length; i++) ...[
          _streamCard(i),
          if (i < _profiles.length - 1) const SizedBox(height: 6),
        ],
      ],
    );
  }

  Widget _streamCard(int index) {
    final p = _profiles[index];
    final selected = index == _selectedProfileIndex;
    final name = p['name'] as String? ?? 'Profile $index';
    final w = p['width'] as int? ?? 0;
    final h = p['height'] as int? ?? 0;
    final codec = p['video_codec'] as String? ?? '';
    final token = p['token'] as String? ?? '';
    final uri = p['stream_uri'] as String? ?? '';
    final res = (w > 0 && h > 0) ? '${w}×$h' : '';
    final detail =
        [res, codec, token].where((s) => s.isNotEmpty).join(' · ');

    return GestureDetector(
      onTap: () => setState(() => _selectedProfileIndex = index),
      child: Container(
        padding: const EdgeInsets.all(12),
        decoration: BoxDecoration(
          color: NvrColors.bgPrimary,
          borderRadius: BorderRadius.circular(6),
          border: Border.all(
            color: selected ? NvrColors.accent : NvrColors.border,
            width: selected ? 2 : 1,
          ),
        ),
        child: Row(
          children: [
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(name, style: NvrTypography.cameraName),
                  if (detail.isNotEmpty) ...[
                    const SizedBox(height: 2),
                    Text(detail, style: NvrTypography.monoLabel),
                  ],
                  if (uri.isNotEmpty) ...[
                    const SizedBox(height: 2),
                    Text(
                      uri,
                      style: NvrTypography.monoLabel.copyWith(
                        fontSize: 8,
                        color: NvrColors.textMuted,
                      ),
                      overflow: TextOverflow.ellipsis,
                    ),
                  ],
                ],
              ),
            ),
            Icon(
              selected ? Icons.radio_button_checked : Icons.radio_button_off,
              color: selected ? NvrColors.accent : NvrColors.textMuted,
              size: 20,
            ),
          ],
        ),
      ),
    );
  }

  Widget _buildCapabilitiesSection() {
    final caps = _capabilities!;
    final items = <_CapItem>[
      _CapItem('Media', caps['media'] == true),
      _CapItem('Events', caps['events'] == true),
      _CapItem('Analytics', caps['analytics'] == true),
      _CapItem('PTZ', caps['ptz'] == true),
      _CapItem('Audio', caps['audio_backchannel'] == true),
      _CapItem('Imaging', caps['imaging'] == true),
      _CapItem('Recording', caps['recording'] == true),
    ];

    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        const Text('CAPABILITIES', style: NvrTypography.monoSection),
        const SizedBox(height: 8),
        Wrap(
          spacing: 6,
          runSpacing: 6,
          children: items.map((item) {
            return Container(
              padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 4),
              decoration: BoxDecoration(
                color: NvrColors.bgTertiary,
                borderRadius: BorderRadius.circular(4),
                border: Border.all(
                  color: item.supported
                      ? NvrColors.accent.withValues(alpha: 0.3)
                      : NvrColors.border,
                ),
              ),
              child: Text(
                item.name.toUpperCase(),
                style: TextStyle(
                  fontFamily: 'JetBrainsMono',
                  fontSize: 9,
                  fontWeight: FontWeight.w500,
                  letterSpacing: 0.5,
                  color: item.supported
                      ? NvrColors.accent
                      : NvrColors.textMuted,
                ),
              ),
            );
          }).toList(),
        ),
      ],
    );
  }

  Widget _buildNameSection() {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        const Text('CAMERA NAME', style: NvrTypography.monoSection),
        const SizedBox(height: 8),
        _field(controller: _nameCtrl, hint: 'Camera name'),
      ],
    );
  }

  Widget _buildAddButton() {
    return SizedBox(
      width: double.infinity,
      child: ElevatedButton(
        onPressed: _adding || _profiles.isEmpty || _alreadyAdded
            ? null
            : _addCamera,
        style: ElevatedButton.styleFrom(
          backgroundColor: NvrColors.success,
          foregroundColor: Colors.white,
          disabledBackgroundColor: NvrColors.success.withValues(alpha: 0.3),
          padding: const EdgeInsets.symmetric(vertical: 14),
          shape: RoundedRectangleBorder(
            borderRadius: BorderRadius.circular(6),
          ),
          elevation: 0,
        ),
        child: Text(
          _adding
              ? 'ADDING...'
              : _alreadyAdded
                  ? 'ALREADY ADDED'
                  : 'ADD CAMERA',
          style: const TextStyle(
            fontFamily: 'JetBrainsMono',
            fontSize: 11,
            fontWeight: FontWeight.w700,
            letterSpacing: 1.0,
          ),
        ),
      ),
    );
  }

  Widget _field({
    required TextEditingController controller,
    required String hint,
    bool obscure = false,
    Widget? suffix,
  }) {
    return TextField(
      controller: controller,
      obscureText: obscure,
      style: const TextStyle(
        color: NvrColors.textPrimary,
        fontFamily: 'JetBrainsMono',
        fontSize: 12,
      ),
      decoration: InputDecoration(
        hintText: hint,
        hintStyle: const TextStyle(color: NvrColors.textMuted, fontSize: 12),
        filled: true,
        fillColor: NvrColors.bgPrimary,
        contentPadding:
            const EdgeInsets.symmetric(horizontal: 10, vertical: 10),
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
      ),
    );
  }

  Widget _badge(String text, Color color) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 3),
      decoration: BoxDecoration(
        color: color.withValues(alpha: 0.1),
        borderRadius: BorderRadius.circular(4),
        border: Border.all(color: color.withValues(alpha: 0.3)),
      ),
      child: Text(
        text,
        style: TextStyle(
          fontFamily: 'JetBrainsMono',
          fontSize: 9,
          fontWeight: FontWeight.w600,
          letterSpacing: 0.5,
          color: color,
        ),
      ),
    );
  }
}

class _CapItem {
  final String name;
  final bool supported;
  const _CapItem(this.name, this.supported);
}
```

- [ ] **Step 2: Verify Flutter analysis passes**

Run: `cd /Users/ethanflower/personal_projects/mediamtx/clients/flutter && flutter analyze lib/screens/cameras/camera_detail_sheet.dart`
Expected: No issues found

- [ ] **Step 3: Commit**

```bash
git add clients/flutter/lib/screens/cameras/camera_detail_sheet.dart
git commit -m "feat(flutter): add camera detail bottom sheet with auth, streams, capabilities"
```

---

### Task 5: Flutter — Wire up discover tab with new widgets

**Files:**

- Modify: `clients/flutter/lib/screens/cameras/add_camera_screen.dart:81-328` (\_DiscoverTab section)

- [ ] **Step 1: Replace the discover tab internals**

In `clients/flutter/lib/screens/cameras/add_camera_screen.dart`, replace the entire `_DiscoverTab` widget class and its state class (lines 81-328) with the following. Add the imports at the top of the file:

Add these imports after the existing imports at the top of the file:

```dart
import 'discovery_card.dart';
import 'camera_detail_sheet.dart';
```

Replace the `_DiscoverTab` and `_DiscoverTabState` classes (lines 81-328):

```dart
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
    setState(() => _discovering = false);
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
    } catch (_) {}

    if (mounted && _discovering) {
      _pollTimer = Timer(const Duration(seconds: 3), _pollResults);
    }
  }

  void _openDetail(Map<String, dynamic> device) {
    CameraDetailSheet.show(context, device);
  }

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.all(16),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
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
                      backgroundColor: NvrColors.accent.withOpacity(0.12),
                    ),
                  ),
                  const SizedBox(height: 12),
                  const Text('Scanning network...', style: NvrTypography.monoLabel),
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

          // Results header
          if (_results.isNotEmpty)
            Padding(
              padding: const EdgeInsets.only(bottom: 8),
              child: Text(
                '${_results.length} DEVICE${_results.length == 1 ? '' : 'S'} FOUND',
                style: NvrTypography.monoLabel,
              ),
            ),

          // Results list
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
                        onTap: () => _openDetail(device),
                      );
                    },
                  ),
          ),
        ],
      ),
    );
  }
}
```

- [ ] **Step 2: Verify Flutter analysis passes**

Run: `cd /Users/ethanflower/personal_projects/mediamtx/clients/flutter && flutter analyze lib/screens/cameras/`
Expected: No issues found (or only pre-existing info-level warnings)

- [ ] **Step 3: Commit**

```bash
git add clients/flutter/lib/screens/cameras/add_camera_screen.dart
git commit -m "feat(flutter): wire discovery tab to rich cards and detail sheet"
```

---

### Task 6: Build and verify end-to-end

**Files:** None (verification only)

- [ ] **Step 1: Build the Go server**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go build -o mediamtx .`
Expected: Build succeeds

- [ ] **Step 2: Build the Flutter app**

Run: `cd /Users/ethanflower/personal_projects/mediamtx/clients/flutter && flutter build macos`
Expected: Build succeeds

- [ ] **Step 3: Run full Flutter analysis**

Run: `cd /Users/ethanflower/personal_projects/mediamtx/clients/flutter && flutter analyze lib/screens/cameras/`
Expected: No errors (info/warning-level issues are acceptable)

- [ ] **Step 4: Commit if any fixes were needed**

```bash
git add -A
git commit -m "fix: address any build issues from discovery redesign"
```
