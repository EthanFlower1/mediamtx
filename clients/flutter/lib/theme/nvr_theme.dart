import 'package:flutter/material.dart';
import 'nvr_colors.dart';

class NvrTheme {
  NvrTheme._();

  static ThemeData dark() {
    final colorScheme = ColorScheme.fromSeed(
      seedColor: NvrColors.accent,
      brightness: Brightness.dark,
    ).copyWith(
      surface: NvrColors.bgPrimary,
      onSurface: NvrColors.textPrimary,
      primary: NvrColors.accent,
      onPrimary: Colors.white,
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
      appBarTheme: const AppBarTheme(
        backgroundColor: NvrColors.bgSecondary,
        foregroundColor: NvrColors.textPrimary,
        elevation: 0,
        scrolledUnderElevation: 0,
      ),
      cardTheme: CardThemeData(
        color: NvrColors.bgSecondary,
        elevation: 0,
        shape: RoundedRectangleBorder(
          borderRadius: BorderRadius.circular(12),
          side: const BorderSide(color: NvrColors.border),
        ),
      ),
      inputDecorationTheme: InputDecorationTheme(
        filled: true,
        fillColor: NvrColors.bgInput,
        hintStyle: const TextStyle(color: NvrColors.textMuted),
        labelStyle: const TextStyle(color: NvrColors.textSecondary),
        border: OutlineInputBorder(
          borderRadius: BorderRadius.circular(8),
          borderSide: const BorderSide(color: NvrColors.border),
        ),
        enabledBorder: OutlineInputBorder(
          borderRadius: BorderRadius.circular(8),
          borderSide: const BorderSide(color: NvrColors.border),
        ),
        focusedBorder: OutlineInputBorder(
          borderRadius: BorderRadius.circular(8),
          borderSide: const BorderSide(color: NvrColors.accent, width: 2),
        ),
        errorBorder: OutlineInputBorder(
          borderRadius: BorderRadius.circular(8),
          borderSide: const BorderSide(color: NvrColors.danger),
        ),
        focusedErrorBorder: OutlineInputBorder(
          borderRadius: BorderRadius.circular(8),
          borderSide: const BorderSide(color: NvrColors.danger, width: 2),
        ),
      ),
      elevatedButtonTheme: ElevatedButtonThemeData(
        style: ElevatedButton.styleFrom(
          backgroundColor: NvrColors.accent,
          foregroundColor: Colors.white,
          shape: RoundedRectangleBorder(
            borderRadius: BorderRadius.circular(8),
          ),
          elevation: 0,
        ),
      ),
      navigationBarTheme: NavigationBarThemeData(
        backgroundColor: NvrColors.bgSecondary,
        indicatorColor: NvrColors.accent,
        iconTheme: WidgetStateProperty.resolveWith((states) {
          if (states.contains(WidgetState.selected)) {
            return const IconThemeData(color: Colors.white);
          }
          return const IconThemeData(color: NvrColors.textSecondary);
        }),
        labelTextStyle: WidgetStateProperty.resolveWith((states) {
          if (states.contains(WidgetState.selected)) {
            return const TextStyle(color: NvrColors.accent);
          }
          return const TextStyle(color: NvrColors.textSecondary);
        }),
      ),
      navigationRailTheme: const NavigationRailThemeData(
        backgroundColor: NvrColors.bgSecondary,
        selectedIconTheme: IconThemeData(color: Colors.white),
        unselectedIconTheme: IconThemeData(color: NvrColors.textSecondary),
        selectedLabelTextStyle: TextStyle(color: NvrColors.accent),
        unselectedLabelTextStyle: TextStyle(color: NvrColors.textSecondary),
        indicatorColor: NvrColors.accent,
      ),
      dividerTheme: const DividerThemeData(
        color: NvrColors.border,
        space: 1,
        thickness: 1,
      ),
      snackBarTheme: SnackBarThemeData(
        backgroundColor: NvrColors.bgSecondary,
        contentTextStyle: const TextStyle(color: NvrColors.textPrimary),
        shape: RoundedRectangleBorder(
          borderRadius: BorderRadius.circular(8),
          side: const BorderSide(color: NvrColors.border),
        ),
        behavior: SnackBarBehavior.floating,
      ),
      textTheme: const TextTheme(
        bodyLarge: TextStyle(color: NvrColors.textPrimary),
        bodyMedium: TextStyle(color: NvrColors.textPrimary),
        bodySmall: TextStyle(color: NvrColors.textSecondary),
        titleLarge: TextStyle(color: NvrColors.textPrimary),
        titleMedium: TextStyle(color: NvrColors.textPrimary),
        titleSmall: TextStyle(color: NvrColors.textSecondary),
        labelLarge: TextStyle(color: NvrColors.textPrimary),
        labelMedium: TextStyle(color: NvrColors.textSecondary),
        labelSmall: TextStyle(color: NvrColors.textMuted),
      ),
    );
  }
}
