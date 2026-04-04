import 'package:flutter/material.dart';
import 'nvr_colors.dart';

class NvrTheme {
  NvrTheme._();

  static ThemeData light() {
    final colorScheme = ColorScheme.fromSeed(
      seedColor: NvrColors.accent,
      brightness: Brightness.light,
    ).copyWith(
      surface: const Color(0xFFF5F5F5),
      onSurface: const Color(0xFF1a1a1a),
      primary: NvrColors.accent,
      onPrimary: Colors.white,
      secondary: const Color(0xFFE5E5E5),
      onSecondary: const Color(0xFF1a1a1a),
      error: NvrColors.danger,
      onError: Colors.white,
      surfaceContainerHighest: const Color(0xFFFFFFFF),
    );

    return ThemeData(
      useMaterial3: true,
      colorScheme: colorScheme,
      scaffoldBackgroundColor: const Color(0xFFF5F5F5),
      fontFamily: 'IBMPlexSans',
      appBarTheme: const AppBarTheme(
        backgroundColor: Color(0xFFFFFFFF),
        foregroundColor: Color(0xFF1a1a1a),
        elevation: 0,
        scrolledUnderElevation: 0,
      ),
      cardTheme: CardThemeData(
        color: const Color(0xFFFFFFFF),
        elevation: 0,
        shape: RoundedRectangleBorder(
          borderRadius: BorderRadius.circular(8),
          side: const BorderSide(color: Color(0xFFE5E5E5)),
        ),
      ),
      inputDecorationTheme: InputDecorationTheme(
        filled: true,
        fillColor: const Color(0xFFFFFFFF),
        hintStyle: const TextStyle(
          fontFamily: 'IBMPlexSans',
          color: Color(0xFF9CA3AF),
        ),
        labelStyle: const TextStyle(
          fontFamily: 'JetBrainsMono',
          fontSize: 10,
          letterSpacing: 1.5,
          color: Color(0xFF9CA3AF),
        ),
        border: OutlineInputBorder(
          borderRadius: BorderRadius.circular(6),
          borderSide: const BorderSide(color: Color(0xFFE5E5E5)),
        ),
        enabledBorder: OutlineInputBorder(
          borderRadius: BorderRadius.circular(6),
          borderSide: const BorderSide(color: Color(0xFFE5E5E5)),
        ),
        focusedBorder: OutlineInputBorder(
          borderRadius: BorderRadius.circular(6),
          borderSide: const BorderSide(color: NvrColors.accent, width: 2),
        ),
        errorBorder: OutlineInputBorder(
          borderRadius: BorderRadius.circular(6),
          borderSide: const BorderSide(color: NvrColors.danger),
        ),
        focusedErrorBorder: OutlineInputBorder(
          borderRadius: BorderRadius.circular(6),
          borderSide: const BorderSide(color: NvrColors.danger, width: 2),
        ),
      ),
      elevatedButtonTheme: ElevatedButtonThemeData(
        style: ElevatedButton.styleFrom(
          backgroundColor: NvrColors.accent,
          foregroundColor: Colors.white,
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
          foregroundColor: const Color(0xFF1a1a1a),
          side: const BorderSide(color: Color(0xFFE5E5E5)),
          shape: RoundedRectangleBorder(
            borderRadius: BorderRadius.circular(4),
          ),
          minimumSize: const Size(0, 44),
        ),
      ),
      dividerTheme: const DividerThemeData(
        color: Color(0xFFE5E5E5),
        space: 1,
        thickness: 1,
      ),
      snackBarTheme: SnackBarThemeData(
        backgroundColor: const Color(0xFFFFFFFF),
        contentTextStyle: const TextStyle(
          fontFamily: 'IBMPlexSans',
          color: Color(0xFF1a1a1a),
        ),
        shape: RoundedRectangleBorder(
          borderRadius: BorderRadius.circular(6),
          side: const BorderSide(color: Color(0xFFE5E5E5)),
        ),
        behavior: SnackBarBehavior.floating,
      ),
      textTheme: const TextTheme(
        headlineLarge: TextStyle(fontFamily: 'IBMPlexSans', color: Color(0xFF1a1a1a)),
        headlineMedium: TextStyle(fontFamily: 'IBMPlexSans', color: Color(0xFF1a1a1a)),
        titleLarge: TextStyle(fontFamily: 'IBMPlexSans', color: Color(0xFF1a1a1a), fontWeight: FontWeight.w600),
        titleMedium: TextStyle(fontFamily: 'IBMPlexSans', color: Color(0xFF1a1a1a)),
        titleSmall: TextStyle(fontFamily: 'IBMPlexSans', color: Color(0xFF6B7280)),
        bodyLarge: TextStyle(fontFamily: 'IBMPlexSans', color: Color(0xFF1a1a1a)),
        bodyMedium: TextStyle(fontFamily: 'IBMPlexSans', color: Color(0xFF1a1a1a)),
        bodySmall: TextStyle(fontFamily: 'IBMPlexSans', color: Color(0xFF6B7280)),
        labelLarge: TextStyle(fontFamily: 'JetBrainsMono', color: Color(0xFF1a1a1a)),
        labelMedium: TextStyle(fontFamily: 'JetBrainsMono', color: Color(0xFF6B7280)),
        labelSmall: TextStyle(fontFamily: 'JetBrainsMono', color: Color(0xFF9CA3AF)),
      ),
    );
  }

  static ThemeData dark() {
    final colorScheme = ColorScheme.fromSeed(
      seedColor: NvrColors.accent,
      brightness: Brightness.dark,
    ).copyWith(
      surface: NvrColors.bgPrimary,
      onSurface: NvrColors.textPrimary,
      primary: NvrColors.accent,
      onPrimary: NvrColors.bgPrimary,
      secondary: NvrColors.bgTertiary,
      onSecondary: NvrColors.textPrimary,
      error: NvrColors.danger,
      onError: Colors.white,
      surfaceContainerHighest: NvrColors.bgSecondary,
    );

    return ThemeData(
      useMaterial3: true,
      colorScheme: colorScheme,
      scaffoldBackgroundColor: NvrColors.bgPrimary,
      fontFamily: 'IBMPlexSans',
      appBarTheme: const AppBarTheme(
        backgroundColor: NvrColors.bgPrimary,
        foregroundColor: NvrColors.textPrimary,
        elevation: 0,
        scrolledUnderElevation: 0,
      ),
      cardTheme: CardThemeData(
        color: NvrColors.bgSecondary,
        elevation: 0,
        shape: RoundedRectangleBorder(
          borderRadius: BorderRadius.circular(8),
          side: const BorderSide(color: NvrColors.border),
        ),
      ),
      inputDecorationTheme: InputDecorationTheme(
        filled: true,
        fillColor: NvrColors.bgTertiary,
        hintStyle: const TextStyle(
          fontFamily: 'IBMPlexSans',
          color: NvrColors.textMuted,
        ),
        labelStyle: const TextStyle(
          fontFamily: 'JetBrainsMono',
          fontSize: 10,
          letterSpacing: 1.5,
          color: NvrColors.textMuted,
        ),
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
          borderSide: const BorderSide(color: NvrColors.accent, width: 2),
        ),
        errorBorder: OutlineInputBorder(
          borderRadius: BorderRadius.circular(6),
          borderSide: const BorderSide(color: NvrColors.danger),
        ),
        focusedErrorBorder: OutlineInputBorder(
          borderRadius: BorderRadius.circular(6),
          borderSide: const BorderSide(color: NvrColors.danger, width: 2),
        ),
      ),
      elevatedButtonTheme: ElevatedButtonThemeData(
        style: ElevatedButton.styleFrom(
          backgroundColor: NvrColors.accent,
          foregroundColor: NvrColors.bgPrimary,
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
          foregroundColor: NvrColors.textPrimary,
          side: const BorderSide(color: NvrColors.border),
          shape: RoundedRectangleBorder(
            borderRadius: BorderRadius.circular(4),
          ),
          minimumSize: const Size(0, 44),
        ),
      ),
      dividerTheme: const DividerThemeData(
        color: NvrColors.border,
        space: 1,
        thickness: 1,
      ),
      snackBarTheme: SnackBarThemeData(
        backgroundColor: NvrColors.bgSecondary,
        contentTextStyle: const TextStyle(
          fontFamily: 'IBMPlexSans',
          color: NvrColors.textPrimary,
        ),
        shape: RoundedRectangleBorder(
          borderRadius: BorderRadius.circular(6),
          side: const BorderSide(color: NvrColors.border),
        ),
        behavior: SnackBarBehavior.floating,
      ),
      textTheme: const TextTheme(
        headlineLarge: TextStyle(fontFamily: 'IBMPlexSans', color: NvrColors.textPrimary),
        headlineMedium: TextStyle(fontFamily: 'IBMPlexSans', color: NvrColors.textPrimary),
        titleLarge: TextStyle(fontFamily: 'IBMPlexSans', color: NvrColors.textPrimary, fontWeight: FontWeight.w600),
        titleMedium: TextStyle(fontFamily: 'IBMPlexSans', color: NvrColors.textPrimary),
        titleSmall: TextStyle(fontFamily: 'IBMPlexSans', color: NvrColors.textSecondary),
        bodyLarge: TextStyle(fontFamily: 'IBMPlexSans', color: NvrColors.textPrimary),
        bodyMedium: TextStyle(fontFamily: 'IBMPlexSans', color: NvrColors.textPrimary),
        bodySmall: TextStyle(fontFamily: 'IBMPlexSans', color: NvrColors.textSecondary),
        labelLarge: TextStyle(fontFamily: 'JetBrainsMono', color: NvrColors.textPrimary),
        labelMedium: TextStyle(fontFamily: 'JetBrainsMono', color: NvrColors.textSecondary),
        labelSmall: TextStyle(fontFamily: 'JetBrainsMono', color: NvrColors.textMuted),
      ),
    );
  }
}
