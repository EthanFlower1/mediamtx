// KAI-156 -- Hold-to-talk button widget.
//
// A long-press-activated talkback button that starts audio talkback on press
// and stops on release. Visual state reflects the current [TalkbackState].
//
// Accessibility: full Semantics labels, announced state changes, excludes
// decorative icons from the semantic tree.

import 'dart:ui';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../services/talkback_service.dart';
import '../services/talkback_provider.dart';
import '../../../theme/nvr_colors.dart';
import '../../../theme/nvr_typography.dart';

/// Hold-to-talk button for audio talkback.
///
/// Long-press starts the talkback session; releasing ends it. The button
/// visually reflects the current [TalkbackState] with icon, label, and color
/// changes.
class HoldToTalkButton extends ConsumerWidget {
  /// The WebRTC endpoint URL for talkback audio.
  final String endpointUrl;

  /// Bearer token for authenticating the talkback connection.
  final String accessToken;

  /// Label shown when idle (localizable).
  final String idleLabel;

  /// Label shown when talkback is active / connecting (localizable).
  final String activeLabel;

  const HoldToTalkButton({
    super.key,
    required this.endpointUrl,
    required this.accessToken,
    this.idleLabel = 'Talk',
    this.activeLabel = 'Hold',
  });

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final service = ref.watch(talkbackServiceProvider);
    final stateAsync = ref.watch(talkbackStateStreamProvider);

    final talkbackState =
        stateAsync.valueOrNull ?? service.currentState;

    final isActive = talkbackState == TalkbackState.active;
    final isConnecting = talkbackState == TalkbackState.connecting ||
        talkbackState == TalkbackState.acquiringMic;
    final isError = talkbackState == TalkbackState.error;
    final highlight = isActive || isConnecting;

    final icon = _iconFor(talkbackState);
    final label = _labelFor(talkbackState);
    final semanticLabel = _semanticLabelFor(talkbackState);

    return Semantics(
      button: true,
      label: semanticLabel,
      hint: 'Long press and hold to talk',
      child: GestureDetector(
        onLongPressStart: (_) => _onPressStart(service),
        onLongPressEnd: (_) => _onPressEnd(service),
        child: ClipRRect(
          borderRadius: BorderRadius.circular(20),
          child: BackdropFilter(
            filter: ImageFilter.blur(sigmaX: 6, sigmaY: 6),
            child: AnimatedContainer(
              duration: const Duration(milliseconds: 200),
              padding:
                  const EdgeInsets.symmetric(horizontal: 12, vertical: 7),
              decoration: BoxDecoration(
                color: _backgroundColor(context, talkbackState),
                borderRadius: BorderRadius.circular(20),
                border: Border.all(
                  color: _borderColor(context, talkbackState),
                ),
              ),
              child: Row(
                mainAxisSize: MainAxisSize.min,
                children: [
                  if (isConnecting)
                    SizedBox(
                      width: 14,
                      height: 14,
                      child: CircularProgressIndicator(
                        strokeWidth: 2,
                        valueColor: AlwaysStoppedAnimation<Color>(
                          NvrColors.of(context).accent,
                        ),
                      ),
                    )
                  else
                    Icon(
                      icon,
                      size: 14,
                      color: highlight
                          ? NvrColors.of(context).accent
                          : isError
                              ? NvrColors.of(context).error
                              : NvrColors.of(context).textPrimary,
                    ),
                  const SizedBox(width: 6),
                  Text(
                    label,
                    style: NvrTypography.of(context).button.copyWith(
                          fontSize: 11,
                          color: highlight
                              ? NvrColors.of(context).accent
                              : isError
                                  ? NvrColors.of(context).error
                                  : NvrColors.of(context).textPrimary,
                        ),
                  ),
                ],
              ),
            ),
          ),
        ),
      ),
    );
  }

  // ── Helpers ──────────────────────────────────────────────────────────────

  void _onPressStart(TalkbackService service) {
    service.startTalkback(
      endpointUrl: endpointUrl,
      accessToken: accessToken,
    );
  }

  void _onPressEnd(TalkbackService service) {
    service.stopTalkback();
  }

  IconData _iconFor(TalkbackState state) {
    switch (state) {
      case TalkbackState.active:
        return Icons.mic;
      case TalkbackState.acquiringMic:
      case TalkbackState.connecting:
        return Icons.mic_none;
      case TalkbackState.error:
        return Icons.mic_off;
      case TalkbackState.idle:
        return Icons.mic_none;
    }
  }

  String _labelFor(TalkbackState state) {
    switch (state) {
      case TalkbackState.active:
        return activeLabel;
      case TalkbackState.acquiringMic:
      case TalkbackState.connecting:
        return activeLabel;
      case TalkbackState.error:
        return idleLabel;
      case TalkbackState.idle:
        return idleLabel;
    }
  }

  String _semanticLabelFor(TalkbackState state) {
    switch (state) {
      case TalkbackState.idle:
        return 'Talkback, inactive. Long press and hold to talk.';
      case TalkbackState.acquiringMic:
        return 'Talkback, acquiring microphone.';
      case TalkbackState.connecting:
        return 'Talkback, connecting.';
      case TalkbackState.active:
        return 'Talkback, active. Release to stop.';
      case TalkbackState.error:
        return 'Talkback, error. Long press and hold to retry.';
    }
  }

  Color _backgroundColor(BuildContext context, TalkbackState state) {
    final colors = NvrColors.of(context);
    switch (state) {
      case TalkbackState.active:
        return colors.accent.withValues(alpha: 0.3);
      case TalkbackState.acquiringMic:
      case TalkbackState.connecting:
        return colors.accent.withValues(alpha: 0.15);
      case TalkbackState.error:
        return colors.danger.withValues(alpha: 0.15);
      case TalkbackState.idle:
        return colors.bgSecondary.withValues(alpha: 0.75);
    }
  }

  Color _borderColor(BuildContext context, TalkbackState state) {
    final colors = NvrColors.of(context);
    switch (state) {
      case TalkbackState.active:
      case TalkbackState.acquiringMic:
      case TalkbackState.connecting:
        return colors.accent;
      case TalkbackState.error:
        return colors.danger;
      case TalkbackState.idle:
        return colors.border;
    }
  }
}
