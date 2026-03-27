import 'package:flutter/material.dart';

import '../../theme/nvr_colors.dart';
import '../../theme/nvr_typography.dart';

/// Extracts a bare IP address from an ONVIF xaddr URL such as
/// "http://192.168.1.100/onvif/device_service".
String _ipFromXaddr(String xaddr) {
  try {
    final uri = Uri.parse(xaddr);
    return uri.host;
  } catch (_) {
    return xaddr;
  }
}

/// A small pill/badge with colored background + border, mono text.
class _Pill extends StatelessWidget {
  const _Pill({required this.label, required this.color});

  final String label;
  final Color color;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 2),
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

/// A discovery result card for a single ONVIF device.
///
/// [device] keys expected from the backend:
///   - xaddr            : String   — ONVIF endpoint URL
///   - manufacturer     : String?
///   - model            : String?
///   - firmware_version : String?
///   - auth_required    : bool?
///   - existing_camera_id : String? — set if this device is already in the NVR
///   - profiles         : List?     — stream profiles returned after open probe
class DiscoveryCard extends StatelessWidget {
  const DiscoveryCard({
    super.key,
    required this.device,
    required this.onTap,
  });

  final Map<String, dynamic> device;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    final manufacturer = device['manufacturer'] as String? ?? '';
    final model = device['model'] as String? ?? '';
    final firmware = device['firmware_version'] as String? ?? '';
    final xaddr = device['xaddr'] as String? ?? '';
    final authRequired = device['auth_required'] as bool? ?? false;
    final alreadyAdded = (device['existing_camera_id'] as String?)?.isNotEmpty ?? false;
    final profiles = device['profiles'] as List<dynamic>? ?? [];

    final ip = _ipFromXaddr(xaddr);

    // Compose camera name: "Manufacturer Model" or fallback.
    final String cameraName;
    if (manufacturer.isNotEmpty && model.isNotEmpty) {
      cameraName = '$manufacturer $model';
    } else if (manufacturer.isNotEmpty) {
      cameraName = manufacturer;
    } else if (model.isNotEmpty) {
      cameraName = model;
    } else {
      cameraName = 'Unknown Camera';
    }

    // Subtitle parts joined by ·
    final subtitleParts = <String>[
      if (ip.isNotEmpty) ip,
      if (manufacturer.isNotEmpty) manufacturer,
      if (model.isNotEmpty) model,
      if (firmware.isNotEmpty) firmware,
    ];
    final subtitle = subtitleParts.join(' · ');

    // Status badge
    final Widget statusBadge;
    if (alreadyAdded) {
      statusBadge = const _Pill(label: 'ADDED', color: NvrColors.accent);
    } else if (authRequired) {
      statusBadge = const _Pill(label: 'AUTH REQUIRED', color: NvrColors.danger);
    } else {
      statusBadge = const _Pill(label: 'OPEN', color: NvrColors.success);
    }

    return Opacity(
      opacity: alreadyAdded ? 0.5 : 1.0,
      child: Material(
        color: NvrColors.bgSecondary,
        borderRadius: BorderRadius.circular(8),
        child: InkWell(
          onTap: alreadyAdded ? null : onTap,
          borderRadius: BorderRadius.circular(8),
          child: Container(
            padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 12),
            decoration: BoxDecoration(
              border: Border.all(color: NvrColors.border),
              borderRadius: BorderRadius.circular(8),
            ),
            child: Row(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                // Camera icon
                Padding(
                  padding: const EdgeInsets.only(top: 2),
                  child: Icon(
                    Icons.videocam_outlined,
                    color: alreadyAdded ? NvrColors.textMuted : NvrColors.accent,
                    size: 20,
                  ),
                ),
                const SizedBox(width: 12),

                // Text block
                Expanded(
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      // Name + status badge on same row
                      Row(
                        crossAxisAlignment: CrossAxisAlignment.center,
                        children: [
                          Expanded(
                            child: Text(
                              cameraName,
                              style: NvrTypography.cameraName,
                              maxLines: 1,
                              overflow: TextOverflow.ellipsis,
                            ),
                          ),
                          const SizedBox(width: 8),
                          statusBadge,
                        ],
                      ),
                      if (subtitle.isNotEmpty) ...[
                        const SizedBox(height: 4),
                        Text(
                          subtitle,
                          style: NvrTypography.monoLabel.copyWith(
                            color: NvrColors.textSecondary,
                          ),
                          maxLines: 1,
                          overflow: TextOverflow.ellipsis,
                        ),
                      ],
                      const SizedBox(height: 6),
                      // Bottom row: stream count badge or auth hint
                      if (!authRequired && profiles.isNotEmpty)
                        _Pill(
                          label: '${profiles.length} STREAM${profiles.length == 1 ? '' : 'S'}',
                          color: NvrColors.textSecondary,
                        )
                      else if (authRequired)
                        Text(
                          'Enter credentials to see streams',
                          style: NvrTypography.body.copyWith(
                            fontStyle: FontStyle.italic,
                            fontSize: 11,
                          ),
                        ),
                    ],
                  ),
                ),

                // Chevron
                const Padding(
                  padding: EdgeInsets.only(top: 2),
                  child: Icon(
                    Icons.chevron_right,
                    color: NvrColors.textMuted,
                    size: 18,
                  ),
                ),
              ],
            ),
          ),
        ),
      ),
    );
  }
}
