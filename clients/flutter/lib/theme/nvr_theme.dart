import 'package:flutter/material.dart';
import 'nvr_colors.dart';

class NvrTheme {
  NvrTheme._();

  static ThemeData dark() => _build(NvrColors.dark, Brightness.dark);
  static ThemeData light() => _build(NvrColors.light, Brightness.light);

  static ThemeData _build(NvrColors c, Brightness brightness) {
    final colorScheme = ColorScheme.fromSeed(
      seedColor: c.accent,
      brightness: brightness,
    ).copyWith(
      surface: c.bgPrimary,
      onSurface: c.textPrimary,
      primary: c.accent,
      onPrimary: brightness == Brightness.dark ? c.bgPrimary : Colors.white,
      secondary: c.bgTertiary,
      onSecondary: c.textPrimary,
      error: c.danger,
      onError: Colors.white,
      surfaceContainerHighest: c.bgSecondary,
    );

    return ThemeData(
      useMaterial3: true,
      colorScheme: colorScheme,
      scaffoldBackgroundColor: c.bgPrimary,
      fontFamily: 'IBMPlexSans',
      extensions: [c],
      appBarTheme: AppBarTheme(
        backgroundColor: c.bgPrimary,
        foregroundColor: c.textPrimary,
        elevation: 0,
        scrolledUnderElevation: 0,
      ),
      cardTheme: CardThemeData(
        color: c.bgSecondary,
        elevation: 0,
        shape: RoundedRectangleBorder(
          borderRadius: BorderRadius.circular(8),
          side: BorderSide(color: c.border),
        ),
      ),
      inputDecorationTheme: InputDecorationTheme(
        filled: true,
        fillColor: c.bgTertiary,
        hintStyle: TextStyle(
          fontFamily: 'IBMPlexSans',
          color: c.textMuted,
        ),
        labelStyle: TextStyle(
          fontFamily: 'JetBrainsMono',
          fontSize: 10,
          letterSpacing: 1.5,
          color: c.textMuted,
        ),
        border: OutlineInputBorder(
          borderRadius: BorderRadius.circular(6),
          borderSide: BorderSide(color: c.border),
        ),
        enabledBorder: OutlineInputBorder(
          borderRadius: BorderRadius.circular(6),
          borderSide: BorderSide(color: c.border),
        ),
        focusedBorder: OutlineInputBorder(
          borderRadius: BorderRadius.circular(6),
          borderSide: BorderSide(color: c.accent, width: 2),
        ),
        errorBorder: OutlineInputBorder(
          borderRadius: BorderRadius.circular(6),
          borderSide: BorderSide(color: c.danger),
        ),
        focusedErrorBorder: OutlineInputBorder(
          borderRadius: BorderRadius.circular(6),
          borderSide: BorderSide(color: c.danger, width: 2),
        ),
      ),
      elevatedButtonTheme: ElevatedButtonThemeData(
        style: ElevatedButton.styleFrom(
          backgroundColor: c.accent,
          foregroundColor: brightness == Brightness.dark ? c.bgPrimary : Colors.white,
          textStyle: const TextStyle(
            fontFamily: 'IBMPlexSans',
            fontSize: 12,
            fontWeight: FontWeight.w600,
          ),
          shape: RoundedRectangleBorder(
            borderRadius: BorderRadius.circular(4),
          ),
          elevation: 0,
          minimumSize: const Size(0, 44),
        ),
      ),
      outlinedButtonTheme: OutlinedButtonThemeData(
        style: OutlinedButton.styleFrom(
          foregroundColor: c.textPrimary,
          side: BorderSide(color: c.border),
          shape: RoundedRectangleBorder(
            borderRadius: BorderRadius.circular(4),
          ),
          minimumSize: const Size(0, 44),
        ),
      ),
      dividerTheme: DividerThemeData(
        color: c.border,
        space: 1,
        thickness: 1,
      ),
      snackBarTheme: SnackBarThemeData(
        backgroundColor: c.bgSecondary,
        contentTextStyle: TextStyle(
          fontFamily: 'IBMPlexSans',
          color: c.textPrimary,
        ),
        shape: RoundedRectangleBorder(
          borderRadius: BorderRadius.circular(6),
          side: BorderSide(color: c.border),
        ),
        behavior: SnackBarBehavior.floating,
      ),
      textTheme: TextTheme(
        headlineLarge: TextStyle(fontFamily: 'IBMPlexSans', color: c.textPrimary),
        headlineMedium: TextStyle(fontFamily: 'IBMPlexSans', color: c.textPrimary),
        titleLarge: TextStyle(fontFamily: 'IBMPlexSans', color: c.textPrimary, fontWeight: FontWeight.w600),
        titleMedium: TextStyle(fontFamily: 'IBMPlexSans', color: c.textPrimary),
        titleSmall: TextStyle(fontFamily: 'IBMPlexSans', color: c.textSecondary),
        bodyLarge: TextStyle(fontFamily: 'IBMPlexSans', color: c.textPrimary),
        bodyMedium: TextStyle(fontFamily: 'IBMPlexSans', color: c.textPrimary),
        bodySmall: TextStyle(fontFamily: 'IBMPlexSans', color: c.textSecondary),
        labelLarge: TextStyle(fontFamily: 'JetBrainsMono', color: c.textPrimary),
        labelMedium: TextStyle(fontFamily: 'JetBrainsMono', color: c.textSecondary),
        labelSmall: TextStyle(fontFamily: 'JetBrainsMono', color: c.textMuted),
      ),
    );
  }
}
