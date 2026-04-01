import 'package:flutter/material.dart';

class NvrColors {
  NvrColors._();

  // Backgrounds
  static const bgPrimary = Color(0xFF0a0a0a);
  static const bgSecondary = Color(0xFF111111);
  static const bgTertiary = Color(0xFF1a1a1a);
  static const bgInput = Color(0xFF1a1a1a);

  // Accent
  static const accent = Color(0xFFf97316);
  static const accentHover = Color(0xFFea580c);

  // Text
  static const textPrimary = Color(0xFFe5e5e5);
  static const textSecondary = Color(0xFF737373);
  static const textMuted = Color(0xFF404040);

  // Status
  static const success = Color(0xFF22c55e);
  static const warning = Color(0xFFeab308);
  static const danger = Color(0xFFef4444);

  // Borders
  static const border = Color(0xFF262626);

  // Helpers for opacity variants
  static Color accentWith(double opacity) => accent.withOpacity(opacity);
  static Color dangerWith(double opacity) => danger.withOpacity(opacity);
  static Color successWith(double opacity) => success.withOpacity(opacity);
  static Color warningWith(double opacity) => warning.withOpacity(opacity);
}
