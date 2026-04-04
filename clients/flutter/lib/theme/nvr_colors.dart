import 'package:flutter/material.dart';

/// Theme-aware color scheme for NVR, implemented as a [ThemeExtension]
/// so it travels with the [ThemeData] and supports lerp transitions.
///
/// Usage:
///   final colors = NvrColors.of(context);
///   Container(color: colors.bgPrimary);
///
/// The legacy static constants (e.g. [NvrColors.accent]) remain available
/// for code that doesn't need theme-awareness (overlay HUDs that are always
/// dark, etc.).
class NvrColors extends ThemeExtension<NvrColors> {
  // ── instance fields (theme-aware) ──
  final Color bgPrimary;
  final Color bgSecondary;
  final Color bgTertiary;
  final Color bgInput;
  final Color accent;
  final Color accentHover;
  final Color textPrimary;
  final Color textSecondary;
  final Color textMuted;
  final Color success;
  final Color warning;
  final Color danger;
  final Color border;

  const NvrColors({
    required this.bgPrimary,
    required this.bgSecondary,
    required this.bgTertiary,
    required this.bgInput,
    required this.accent,
    required this.accentHover,
    required this.textPrimary,
    required this.textSecondary,
    required this.textMuted,
    required this.success,
    required this.warning,
    required this.danger,
    required this.border,
  });

  // ── dark palette (the original) ──
  static const dark = NvrColors(
    bgPrimary: Color(0xFF0a0a0a),
    bgSecondary: Color(0xFF111111),
    bgTertiary: Color(0xFF1a1a1a),
    bgInput: Color(0xFF1a1a1a),
    accent: Color(0xFFf97316),
    accentHover: Color(0xFFea580c),
    textPrimary: Color(0xFFe5e5e5),
    textSecondary: Color(0xFF737373),
    textMuted: Color(0xFF404040),
    success: Color(0xFF22c55e),
    warning: Color(0xFFeab308),
    danger: Color(0xFFef4444),
    border: Color(0xFF262626),
  );

  // ── light palette ──
  static const light = NvrColors(
    bgPrimary: Color(0xFFf5f5f5),
    bgSecondary: Color(0xFFffffff),
    bgTertiary: Color(0xFFe5e5e5),
    bgInput: Color(0xFFe5e5e5),
    accent: Color(0xFFea580c),
    accentHover: Color(0xFFc2410c),
    textPrimary: Color(0xFF171717),
    textSecondary: Color(0xFF525252),
    textMuted: Color(0xFFa3a3a3),
    success: Color(0xFF16a34a),
    warning: Color(0xFFca8a04),
    danger: Color(0xFFdc2626),
    border: Color(0xFFd4d4d4),
  );

  /// Resolve the current [NvrColors] from the nearest [Theme].
  static NvrColors of(BuildContext context) {
    return Theme.of(context).extension<NvrColors>()!;
  }

  // Helpers for opacity variants
  Color accentWith(double opacity) => accent.withOpacity(opacity);
  Color dangerWith(double opacity) => danger.withOpacity(opacity);
  Color successWith(double opacity) => success.withOpacity(opacity);
  Color warningWith(double opacity) => warning.withOpacity(opacity);

  @override
  NvrColors copyWith({
    Color? bgPrimary,
    Color? bgSecondary,
    Color? bgTertiary,
    Color? bgInput,
    Color? accent,
    Color? accentHover,
    Color? textPrimary,
    Color? textSecondary,
    Color? textMuted,
    Color? success,
    Color? warning,
    Color? danger,
    Color? border,
  }) {
    return NvrColors(
      bgPrimary: bgPrimary ?? this.bgPrimary,
      bgSecondary: bgSecondary ?? this.bgSecondary,
      bgTertiary: bgTertiary ?? this.bgTertiary,
      bgInput: bgInput ?? this.bgInput,
      accent: accent ?? this.accent,
      accentHover: accentHover ?? this.accentHover,
      textPrimary: textPrimary ?? this.textPrimary,
      textSecondary: textSecondary ?? this.textSecondary,
      textMuted: textMuted ?? this.textMuted,
      success: success ?? this.success,
      warning: warning ?? this.warning,
      danger: danger ?? this.danger,
      border: border ?? this.border,
    );
  }

  @override
  NvrColors lerp(NvrColors? other, double t) {
    if (other == null) return this;
    return NvrColors(
      bgPrimary: Color.lerp(bgPrimary, other.bgPrimary, t)!,
      bgSecondary: Color.lerp(bgSecondary, other.bgSecondary, t)!,
      bgTertiary: Color.lerp(bgTertiary, other.bgTertiary, t)!,
      bgInput: Color.lerp(bgInput, other.bgInput, t)!,
      accent: Color.lerp(accent, other.accent, t)!,
      accentHover: Color.lerp(accentHover, other.accentHover, t)!,
      textPrimary: Color.lerp(textPrimary, other.textPrimary, t)!,
      textSecondary: Color.lerp(textSecondary, other.textSecondary, t)!,
      textMuted: Color.lerp(textMuted, other.textMuted, t)!,
      success: Color.lerp(success, other.success, t)!,
      warning: Color.lerp(warning, other.warning, t)!,
      danger: Color.lerp(danger, other.danger, t)!,
      border: Color.lerp(border, other.border, t)!,
    );
  }
}
