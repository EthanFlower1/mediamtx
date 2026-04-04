import 'package:flutter/material.dart';
import 'nvr_colors.dart';

/// Typography styles for the NVR client.
///
/// Static getters return styles without color (inheriting from the theme).
/// Use [NvrTypography.of] to get styles pre-coloured for the current theme:
///
///   final t = NvrTypography.of(context);
///   Text('LABEL', style: t.monoLabel);
class NvrTypography {
  final NvrColors _c;
  NvrTypography._(this._c);

  static const _mono = 'JetBrainsMono';
  static const _sans = 'IBMPlexSans';

  /// Resolve typography styles using current theme colors.
  static NvrTypography of(BuildContext context) {
    return NvrTypography._(NvrColors.of(context));
  }

  // ── Monospace styles ──

  TextStyle get monoLabel => TextStyle(
        fontFamily: _mono,
        fontSize: 9,
        fontWeight: FontWeight.w500,
        letterSpacing: 1.5,
        color: _c.textMuted,
      );

  TextStyle get monoStatus => TextStyle(
        fontFamily: _mono,
        fontSize: 9,
        fontWeight: FontWeight.w500,
        letterSpacing: 1.0,
        color: _c.success,
      );

  TextStyle get monoTimestamp => TextStyle(
        fontFamily: _mono,
        fontSize: 12,
        fontWeight: FontWeight.w400,
        color: _c.accent,
      );

  TextStyle get monoData => TextStyle(
        fontFamily: _mono,
        fontSize: 12,
        fontWeight: FontWeight.w400,
        color: _c.textPrimary,
      );

  TextStyle get monoDataLarge => TextStyle(
        fontFamily: _mono,
        fontSize: 16,
        fontWeight: FontWeight.w500,
        color: _c.textPrimary,
      );

  TextStyle get monoSection => TextStyle(
        fontFamily: _mono,
        fontSize: 10,
        fontWeight: FontWeight.w700,
        letterSpacing: 2.0,
        color: _c.accent,
      );

  TextStyle get monoControl => TextStyle(
        fontFamily: _mono,
        fontSize: 9,
        fontWeight: FontWeight.w500,
        letterSpacing: 1.0,
        color: _c.textMuted,
      );

  // ── Sans styles ──

  TextStyle get pageTitle => TextStyle(
        fontFamily: _sans,
        fontSize: 16,
        fontWeight: FontWeight.w600,
        color: _c.textPrimary,
      );

  TextStyle get cameraName => TextStyle(
        fontFamily: _sans,
        fontSize: 13,
        fontWeight: FontWeight.w500,
        color: _c.textPrimary,
      );

  TextStyle get body => TextStyle(
        fontFamily: _sans,
        fontSize: 12,
        fontWeight: FontWeight.w400,
        height: 1.5,
        color: _c.textSecondary,
      );

  TextStyle get button => TextStyle(
        fontFamily: _sans,
        fontSize: 12,
        fontWeight: FontWeight.w600,
        color: _c.textPrimary,
      );

  TextStyle get alert => TextStyle(
        fontFamily: _sans,
        fontSize: 12,
        fontWeight: FontWeight.w400,
        color: _c.danger,
      );
}
