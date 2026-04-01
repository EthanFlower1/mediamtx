import 'package:flutter/material.dart';
import 'nvr_colors.dart';

class NvrTypography {
  NvrTypography._();

  static const _mono = 'JetBrainsMono';
  static const _sans = 'IBMPlexSans';

  // Monospace — status labels, camera IDs, timestamps, data values
  static const TextStyle monoLabel = TextStyle(
    fontFamily: _mono,
    fontSize: 9,
    fontWeight: FontWeight.w500,
    letterSpacing: 1.5,
    color: NvrColors.textMuted,
  );

  static const TextStyle monoStatus = TextStyle(
    fontFamily: _mono,
    fontSize: 9,
    fontWeight: FontWeight.w500,
    letterSpacing: 1.0,
    color: NvrColors.success,
  );

  static const TextStyle monoTimestamp = TextStyle(
    fontFamily: _mono,
    fontSize: 12,
    fontWeight: FontWeight.w400,
    color: NvrColors.accent,
  );

  static const TextStyle monoData = TextStyle(
    fontFamily: _mono,
    fontSize: 12,
    fontWeight: FontWeight.w400,
    color: NvrColors.textPrimary,
  );

  static const TextStyle monoDataLarge = TextStyle(
    fontFamily: _mono,
    fontSize: 16,
    fontWeight: FontWeight.w500,
    color: NvrColors.textPrimary,
  );

  static const TextStyle monoSection = TextStyle(
    fontFamily: _mono,
    fontSize: 10,
    fontWeight: FontWeight.w700,
    letterSpacing: 2.0,
    color: NvrColors.accent,
  );

  static const TextStyle monoControl = TextStyle(
    fontFamily: _mono,
    fontSize: 9,
    fontWeight: FontWeight.w500,
    letterSpacing: 1.0,
    color: NvrColors.textMuted,
  );

  // Sans — page titles, camera names, descriptions, buttons
  static const TextStyle pageTitle = TextStyle(
    fontFamily: _sans,
    fontSize: 16,
    fontWeight: FontWeight.w600,
    color: NvrColors.textPrimary,
  );

  static const TextStyle cameraName = TextStyle(
    fontFamily: _sans,
    fontSize: 13,
    fontWeight: FontWeight.w500,
    color: NvrColors.textPrimary,
  );

  static const TextStyle body = TextStyle(
    fontFamily: _sans,
    fontSize: 12,
    fontWeight: FontWeight.w400,
    height: 1.5,
    color: NvrColors.textSecondary,
  );

  static const TextStyle button = TextStyle(
    fontFamily: _sans,
    fontSize: 12,
    fontWeight: FontWeight.w600,
    color: NvrColors.textPrimary,
  );

  static const TextStyle alert = TextStyle(
    fontFamily: _sans,
    fontSize: 12,
    fontWeight: FontWeight.w400,
    color: NvrColors.danger,
  );
}
