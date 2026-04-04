import 'dart:ui';

import 'package:flutter/material.dart';

import '../../services/api_client.dart';
import '../../theme/nvr_colors.dart';
import '../../theme/nvr_typography.dart';

class PtzControls extends StatelessWidget {
  final ApiClient apiClient;
  final String cameraId;

  const PtzControls({
    super.key,
    required this.apiClient,
    required this.cameraId,
  });

  Future<void> _sendPtz(String action) async {
    try {
      await apiClient.post(
        '/cameras/$cameraId/ptz',
        data: {'action': action},
      );
    } catch (_) {
      // PTZ failures are fire-and-forget; ignore silently
    }
  }

  @override
  Widget build(BuildContext context) {
    return Column(
      mainAxisSize: MainAxisSize.min,
      crossAxisAlignment: CrossAxisAlignment.center,
      children: [
        // ── D-pad ──────────────────────────────────────────────────────────
        _DpadUp(onTap: () => _sendPtz('up')),
        const SizedBox(height: 2),
        Row(
          mainAxisSize: MainAxisSize.min,
          children: [
            _DpadButton(
              icon: Icons.keyboard_arrow_left,
              onTap: () => _sendPtz('left'),
            ),
            const SizedBox(width: 2),
            _DpadHome(onTap: () => _sendPtz('stop')),
            const SizedBox(width: 2),
            _DpadButton(
              icon: Icons.keyboard_arrow_right,
              onTap: () => _sendPtz('right'),
            ),
          ],
        ),
        const SizedBox(height: 2),
        _DpadDown(onTap: () => _sendPtz('down')),

        const SizedBox(height: 12),

        // ── Zoom control ───────────────────────────────────────────────────
        Text('ZOOM', style: NvrTypography.of(context).monoControl),
        const SizedBox(height: 6),
        _DpadButton(
          icon: Icons.add,
          onTap: () => _sendPtz('zoom_in'),
        ),
        const SizedBox(height: 2),
        _DpadButton(
          icon: Icons.remove,
          onTap: () => _sendPtz('zoom_out'),
        ),
      ],
    );
  }
}

// ── D-pad directional button ──────────────────────────────────────────────────

class _DpadButton extends StatelessWidget {
  final IconData icon;
  final VoidCallback onTap;

  const _DpadButton({required this.icon, required this.onTap});

  @override
  Widget build(BuildContext context) {
    return GestureDetector(
      onTap: onTap,
      child: ClipRRect(
        borderRadius: BorderRadius.circular(6),
        child: BackdropFilter(
          filter: ImageFilter.blur(sigmaX: 4, sigmaY: 4),
          child: Container(
            width: 28,
            height: 28,
            decoration: BoxDecoration(
              color: NvrColors.of(context).bgSecondary.withValues(alpha: 0.8),
              borderRadius: BorderRadius.circular(6),
              border: Border.all(
                color: NvrColors.of(context).border,
                width: 1,
              ),
            ),
            child: Center(
              child: Icon(
                icon,
                size: 16,
                color: NvrColors.of(context).textPrimary,
              ),
            ),
          ),
        ),
      ),
    );
  }
}

// ── Dedicated up/down wrappers (same as _DpadButton, kept for clarity) ────────

class _DpadUp extends StatelessWidget {
  final VoidCallback onTap;
  const _DpadUp({required this.onTap});

  @override
  Widget build(BuildContext context) =>
      _DpadButton(icon: Icons.keyboard_arrow_up, onTap: onTap);
}

class _DpadDown extends StatelessWidget {
  final VoidCallback onTap;
  const _DpadDown({required this.onTap});

  @override
  Widget build(BuildContext context) =>
      _DpadButton(icon: Icons.keyboard_arrow_down, onTap: onTap);
}

// ── Home / stop button (center circle) ────────────────────────────────────────

class _DpadHome extends StatelessWidget {
  final VoidCallback onTap;
  const _DpadHome({required this.onTap});

  @override
  Widget build(BuildContext context) {
    return GestureDetector(
      onTap: onTap,
      child: ClipOval(
        child: BackdropFilter(
          filter: ImageFilter.blur(sigmaX: 4, sigmaY: 4),
          child: Container(
            width: 22,
            height: 22,
            decoration: BoxDecoration(
              shape: BoxShape.circle,
              color: NvrColors.of(context).bgSecondary.withValues(alpha: 0.8),
              border: Border.all(
                color: NvrColors.of(context).accent,
                width: 1,
              ),
            ),
            child: Center(
              child: Container(
                width: 6,
                height: 6,
                decoration: BoxDecoration(
                  shape: BoxShape.circle,
                  color: NvrColors.of(context).accent,
                ),
              ),
            ),
          ),
        ),
      ),
    );
  }
}
