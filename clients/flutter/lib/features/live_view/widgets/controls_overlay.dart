// KAI-300 — Controls overlay: PTZ, talkback, snapshot, fullscreen, quality badge.
//
// Rendered on top of the video widget. Fades out after [_kAutoHideDelay] of
// inactivity; tapping the video layer restores it.
//
// White-label: all colors come from Theme / NvrColors. No hardcoded brand hex.
// i18n: all strings use [LiveViewStrings] (see live_view_screen.dart).

import 'dart:async';
import 'dart:ui';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../state/live_view_state.dart';
import '../../../api/ptz_api.dart';
import '../../../state/app_session.dart';
import '../../../theme/nvr_colors.dart';
import '../../../theme/nvr_typography.dart';
import '../../../theme/nvr_animations.dart';

const _kAutoHideDelay = Duration(seconds: 4);

/// Strings namespace — all user-facing text. Extend i18n when proper locale
/// files land (KAI-300 adds en/es/fr/de keys to i18n/).
class LiveViewStrings {
  static const snapshot = 'Snapshot';
  static const snapshotSaving = 'Saving\u2026';
  static const fullscreen = 'Fullscreen';
  static const exitFullscreen = 'Exit';
  static const muted = 'Muted';
  static const audio = 'Audio';
  static const talkback = 'Talk';
  static const talkbackHold = 'Hold';
  static const ptzZoom = 'ZOOM';
  static const back = 'Back';
  static const latencyMs = 'ms';
}

class ControlsOverlay extends ConsumerStatefulWidget {
  final String cameraId;
  final bool ptzCapable;

  /// Called when the user taps Snapshot. The caller owns the save logic.
  final Future<void> Function() onSnapshot;

  /// Called when the user taps Fullscreen / Exit Fullscreen.
  final VoidCallback onFullscreenToggle;

  /// Whether the view is currently in fullscreen mode.
  final bool isFullscreen;

  const ControlsOverlay({
    super.key,
    required this.cameraId,
    required this.ptzCapable,
    required this.onSnapshot,
    required this.onFullscreenToggle,
    this.isFullscreen = false,
  });

  @override
  ConsumerState<ControlsOverlay> createState() => _ControlsOverlayState();
}

class _ControlsOverlayState extends ConsumerState<ControlsOverlay> {
  bool _visible = true;
  bool _snapshotBusy = false;
  Timer? _hideTimer;

  @override
  void initState() {
    super.initState();
    _resetHideTimer();
  }

  @override
  void dispose() {
    _hideTimer?.cancel();
    super.dispose();
  }

  void _resetHideTimer() {
    _hideTimer?.cancel();
    if (!mounted) return;
    setState(() => _visible = true);
    _hideTimer = Timer(_kAutoHideDelay, () {
      if (mounted) setState(() => _visible = false);
    });
  }

  void _onTapLayer() => _resetHideTimer();

  Future<void> _handleSnapshot() async {
    if (_snapshotBusy) return;
    setState(() => _snapshotBusy = true);
    try {
      await widget.onSnapshot();
    } finally {
      if (mounted) setState(() => _snapshotBusy = false);
    }
  }

  Future<void> _sendPtz(PtzAction action) async {
    final session = ref.read(appSessionProvider);
    final conn = session.activeConnection;
    final token = session.accessToken;
    if (conn == null || token == null) return;
    const api = HttpPtzApi();
    await api.move(
      cameraId: widget.cameraId,
      baseUrl: conn.endpointUrl,
      accessToken: token,
      action: action,
    );
  }

  @override
  Widget build(BuildContext context) {
    final liveState = ref.watch(liveViewStateProvider);

    return GestureDetector(
      behavior: HitTestBehavior.translucent,
      onTap: _onTapLayer,
      child: AnimatedOpacity(
        opacity: _visible ? 1.0 : 0.0,
        duration: NvrAnimations.overlayFadeDuration,
        child: Stack(
          children: [
            // ── Top gradient + HUD ────────────────────────────────────────
            _TopBar(
              cameraName: liveState.cameraName ?? '',
              connectionLabel: liveState.connectionLabel,
              estimatedLatencyMs: liveState.estimatedLatencyMs,
              isFullscreen: widget.isFullscreen,
              onBack: widget.isFullscreen
                  ? widget.onFullscreenToggle
                  : null,
            ),

            // ── Bottom controls ───────────────────────────────────────────
            Align(
              alignment: Alignment.bottomCenter,
              child: _BottomBar(
                snapshotBusy: _snapshotBusy,
                isFullscreen: widget.isFullscreen,
                onSnapshot: _handleSnapshot,
                onFullscreenToggle: widget.onFullscreenToggle,
                onToggleAudio: () =>
                    ref.read(liveViewStateProvider.notifier).toggleAudioMute(),
                audioMuted: liveState.audioMuted,
                talkbackActive: liveState.talkbackActive,
                onTalkbackChanged: (active) => ref
                    .read(liveViewStateProvider.notifier)
                    .setTalkbackActive(active),
              ),
            ),

            // ── PTZ panel (right edge) ────────────────────────────────────
            if (widget.ptzCapable)
              Align(
                alignment: Alignment.centerRight,
                child: Padding(
                  padding: const EdgeInsets.only(right: 12),
                  child: _PtzPanel(onPtz: _sendPtz),
                ),
              ),
          ],
        ),
      ),
    );
  }
}

// ---------------------------------------------------------------------------
// Top bar
// ---------------------------------------------------------------------------

class _TopBar extends StatelessWidget {
  final String cameraName;
  final String? connectionLabel;
  final int? estimatedLatencyMs;
  final bool isFullscreen;
  final VoidCallback? onBack;

  const _TopBar({
    required this.cameraName,
    this.connectionLabel,
    this.estimatedLatencyMs,
    required this.isFullscreen,
    this.onBack,
  });

  @override
  Widget build(BuildContext context) {
    return Align(
      alignment: Alignment.topCenter,
      child: Container(
        decoration: const BoxDecoration(
          gradient: LinearGradient(
            begin: Alignment.topCenter,
            end: Alignment.bottomCenter,
            colors: [Colors.black87, Colors.transparent],
          ),
        ),
        padding: const EdgeInsets.fromLTRB(12, 40, 12, 24),
        child: Row(
          children: [
            if (onBack != null) ...[
              _PillButton(
                icon: Icons.arrow_back,
                label: LiveViewStrings.back,
                onTap: onBack!,
              ),
              const SizedBox(width: 8),
            ],
            const _LiveBadge(),
            const SizedBox(width: 8),
            Expanded(
              child: Text(
                cameraName,
                style: const TextStyle(
                  fontFamily: 'IBMPlexSans',
                  fontSize: 14,
                  fontWeight: FontWeight.w600,
                  color: Colors.white,
                ),
                overflow: TextOverflow.ellipsis,
              ),
            ),
            if (connectionLabel != null) ...[
              const SizedBox(width: 8),
              _QualityBadge(
                connectionLabel: connectionLabel!,
                latencyMs: estimatedLatencyMs,
              ),
            ],
          ],
        ),
      ),
    );
  }
}

// ---------------------------------------------------------------------------
// Bottom bar
// ---------------------------------------------------------------------------

class _BottomBar extends StatelessWidget {
  final bool snapshotBusy;
  final bool isFullscreen;
  final bool audioMuted;
  final bool talkbackActive;
  final VoidCallback onSnapshot;
  final VoidCallback onFullscreenToggle;
  final VoidCallback onToggleAudio;
  final ValueChanged<bool> onTalkbackChanged;

  const _BottomBar({
    required this.snapshotBusy,
    required this.isFullscreen,
    required this.audioMuted,
    required this.talkbackActive,
    required this.onSnapshot,
    required this.onFullscreenToggle,
    required this.onToggleAudio,
    required this.onTalkbackChanged,
  });

  @override
  Widget build(BuildContext context) {
    return Container(
      decoration: const BoxDecoration(
        gradient: LinearGradient(
          begin: Alignment.bottomCenter,
          end: Alignment.topCenter,
          colors: [Colors.black87, Colors.transparent],
        ),
      ),
      padding: const EdgeInsets.fromLTRB(12, 24, 12, 32),
      child: Wrap(
        alignment: WrapAlignment.center,
        spacing: 8,
        runSpacing: 8,
        children: [
          // Audio mute toggle
          _PillButton(
            icon: audioMuted ? Icons.volume_off : Icons.volume_up,
            label: audioMuted ? LiveViewStrings.muted : LiveViewStrings.audio,
            onTap: onToggleAudio,
          ),

          // Talkback hold-to-talk
          GestureDetector(
            onLongPressStart: (_) => onTalkbackChanged(true),
            onLongPressEnd: (_) => onTalkbackChanged(false),
            child: _PillButton(
              icon: talkbackActive ? Icons.mic : Icons.mic_none,
              label: talkbackActive
                  ? LiveViewStrings.talkbackHold
                  : LiveViewStrings.talkback,
              onTap: () {}, // tap is a no-op; long-press activates
              highlight: talkbackActive,
            ),
          ),

          // Snapshot
          _PillButton(
            icon: snapshotBusy ? Icons.hourglass_empty : Icons.photo_camera,
            label: snapshotBusy
                ? LiveViewStrings.snapshotSaving
                : LiveViewStrings.snapshot,
            onTap: snapshotBusy ? () {} : onSnapshot,
          ),

          // Fullscreen toggle
          _PillButton(
            icon: isFullscreen ? Icons.fullscreen_exit : Icons.fullscreen,
            label: isFullscreen
                ? LiveViewStrings.exitFullscreen
                : LiveViewStrings.fullscreen,
            onTap: onFullscreenToggle,
          ),
        ],
      ),
    );
  }
}

// ---------------------------------------------------------------------------
// PTZ panel
// ---------------------------------------------------------------------------

class _PtzPanel extends StatelessWidget {
  final Future<void> Function(PtzAction) onPtz;

  const _PtzPanel({required this.onPtz});

  @override
  Widget build(BuildContext context) {
    return Column(
      mainAxisSize: MainAxisSize.min,
      children: [
        _PtzButton(icon: Icons.keyboard_arrow_up, onTap: () => onPtz(PtzAction.up)),
        const SizedBox(height: 2),
        Row(
          mainAxisSize: MainAxisSize.min,
          children: [
            _PtzButton(icon: Icons.keyboard_arrow_left, onTap: () => onPtz(PtzAction.left)),
            const SizedBox(width: 2),
            _PtzStopButton(onTap: () => onPtz(PtzAction.stop)),
            const SizedBox(width: 2),
            _PtzButton(icon: Icons.keyboard_arrow_right, onTap: () => onPtz(PtzAction.right)),
          ],
        ),
        const SizedBox(height: 2),
        _PtzButton(icon: Icons.keyboard_arrow_down, onTap: () => onPtz(PtzAction.down)),
        const SizedBox(height: 10),
        Text(LiveViewStrings.ptzZoom,
            style: NvrTypography.of(context).monoControl),
        const SizedBox(height: 4),
        _PtzButton(icon: Icons.add, onTap: () => onPtz(PtzAction.zoomIn)),
        const SizedBox(height: 2),
        _PtzButton(icon: Icons.remove, onTap: () => onPtz(PtzAction.zoomOut)),
      ],
    );
  }
}

class _PtzButton extends StatelessWidget {
  final IconData icon;
  final VoidCallback onTap;
  const _PtzButton({required this.icon, required this.onTap});

  @override
  Widget build(BuildContext context) {
    return GestureDetector(
      onTap: onTap,
      child: ClipRRect(
        borderRadius: BorderRadius.circular(6),
        child: BackdropFilter(
          filter: ImageFilter.blur(sigmaX: 4, sigmaY: 4),
          child: Container(
            width: 32,
            height: 32,
            decoration: BoxDecoration(
              color: NvrColors.of(context).bgSecondary.withValues(alpha: 0.8),
              borderRadius: BorderRadius.circular(6),
              border: Border.all(color: NvrColors.of(context).border),
            ),
            child: Icon(icon, size: 18, color: NvrColors.of(context).textPrimary),
          ),
        ),
      ),
    );
  }
}

class _PtzStopButton extends StatelessWidget {
  final VoidCallback onTap;
  const _PtzStopButton({required this.onTap});

  @override
  Widget build(BuildContext context) {
    return GestureDetector(
      onTap: onTap,
      child: ClipOval(
        child: BackdropFilter(
          filter: ImageFilter.blur(sigmaX: 4, sigmaY: 4),
          child: Container(
            width: 26,
            height: 26,
            decoration: BoxDecoration(
              shape: BoxShape.circle,
              color: NvrColors.of(context).bgSecondary.withValues(alpha: 0.8),
              border: Border.all(color: NvrColors.of(context).accent),
            ),
            child: Center(
              child: Container(
                width: 7,
                height: 7,
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

// ---------------------------------------------------------------------------
// Shared pill button
// ---------------------------------------------------------------------------

class _PillButton extends StatelessWidget {
  final IconData icon;
  final String label;
  final VoidCallback onTap;
  final bool highlight;

  const _PillButton({
    required this.icon,
    required this.label,
    required this.onTap,
    this.highlight = false,
  });

  @override
  Widget build(BuildContext context) {
    return GestureDetector(
      onTap: onTap,
      child: ClipRRect(
        borderRadius: BorderRadius.circular(20),
        child: BackdropFilter(
          filter: ImageFilter.blur(sigmaX: 6, sigmaY: 6),
          child: Container(
            padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 7),
            decoration: BoxDecoration(
              color: highlight
                  ? NvrColors.of(context).accent.withValues(alpha: 0.3)
                  : NvrColors.of(context).bgSecondary.withValues(alpha: 0.75),
              borderRadius: BorderRadius.circular(20),
              border: Border.all(
                color: highlight
                    ? NvrColors.of(context).accent
                    : NvrColors.of(context).border,
              ),
            ),
            child: Row(
              mainAxisSize: MainAxisSize.min,
              children: [
                Icon(icon,
                    size: 14,
                    color: highlight
                        ? NvrColors.of(context).accent
                        : NvrColors.of(context).textPrimary),
                const SizedBox(width: 6),
                Text(
                  label,
                  style: NvrTypography.of(context).button.copyWith(
                        fontSize: 11,
                        color: highlight
                            ? NvrColors.of(context).accent
                            : NvrColors.of(context).textPrimary,
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

// ---------------------------------------------------------------------------
// Live badge
// ---------------------------------------------------------------------------

class _LiveBadge extends StatelessWidget {
  const _LiveBadge();

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 2),
      decoration: BoxDecoration(
        color: Colors.red,
        borderRadius: BorderRadius.circular(4),
      ),
      child: const Text(
        'LIVE',
        style: TextStyle(
          fontFamily: 'JetBrainsMono',
          fontSize: 9,
          fontWeight: FontWeight.w700,
          color: Colors.white,
          letterSpacing: 1.2,
        ),
      ),
    );
  }
}

// ---------------------------------------------------------------------------
// Quality badge
// ---------------------------------------------------------------------------

class _QualityBadge extends StatelessWidget {
  final String connectionLabel;
  final int? latencyMs;

  const _QualityBadge({required this.connectionLabel, this.latencyMs});

  @override
  Widget build(BuildContext context) {
    final latencyText =
        latencyMs != null ? ' $latencyMs${LiveViewStrings.latencyMs}' : '';
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 2),
      decoration: BoxDecoration(
        color: NvrColors.of(context).bgSecondary.withValues(alpha: 0.75),
        borderRadius: BorderRadius.circular(4),
        border: Border.all(color: NvrColors.of(context).border),
      ),
      child: Text(
        '$connectionLabel$latencyText',
        style: TextStyle(
          fontFamily: 'JetBrainsMono',
          fontSize: 9,
          fontWeight: FontWeight.w500,
          color: NvrColors.of(context).textSecondary,
        ),
      ),
    );
  }
}
