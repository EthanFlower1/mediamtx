# Plan 1: Foundation — Design System, Navigation Shell & Auth Screens

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Establish the Tactical HUD design system, rebuild the navigation shell with icon rail + camera panel, and restyle the auth screens — producing a working app shell.

**Architecture:** Replace `NvrColors` and `NvrTheme` with new Tactical HUD tokens. Build a custom widget library (analog slider, toggle, knob, segmented control, status badge, camera tile). Rebuild `AdaptiveLayout` as a custom icon rail + slide-out camera panel on desktop, bottom nav + bottom sheet on mobile. Restyle Login and Server Setup screens.

**Tech Stack:** Flutter 3.2+, Riverpod 2.4, GoRouter 14, Material 3, JetBrains Mono + IBM Plex Sans (bundled)

**Spec:** `docs/superpowers/specs/2026-03-26-flutter-nvr-ui-redesign.md`

---

## File Structure

### New Files
| File | Responsibility |
|---|---|
| `clients/flutter/assets/fonts/JetBrainsMono-*.ttf` | Bundled monospace font files (Regular, Medium, Bold) |
| `clients/flutter/assets/fonts/IBMPlexSans-*.ttf` | Bundled sans-serif font files (Light, Regular, Medium, SemiBold, Bold) |
| `clients/flutter/lib/theme/nvr_typography.dart` | Text style definitions using the two font families |
| `clients/flutter/lib/theme/nvr_animations.dart` | Animation duration and curve constants |
| `clients/flutter/lib/widgets/hud/analog_slider.dart` | Custom analog slider with tick marks and glow thumb |
| `clients/flutter/lib/widgets/hud/hud_toggle.dart` | Custom toggle switch with ON/OFF glow states |
| `clients/flutter/lib/widgets/hud/rotary_knob.dart` | Rotary knob control with notch marks |
| `clients/flutter/lib/widgets/hud/segmented_control.dart` | HUD-style segmented selector |
| `clients/flutter/lib/widgets/hud/status_badge.dart` | Status badge with glow dot and mono label |
| `clients/flutter/lib/widgets/hud/hud_button.dart` | Button variants (primary, secondary, danger, tactical) |
| `clients/flutter/lib/widgets/hud/corner_brackets.dart` | Corner bracket overlay for camera tiles |
| `clients/flutter/lib/widgets/hud/camera_tile.dart` | Camera feed tile with brackets, status, and name overlays |
| `clients/flutter/lib/widgets/shell/icon_rail.dart` | Desktop icon rail navigation (60px) |
| `clients/flutter/lib/widgets/shell/camera_panel.dart` | Slide-out camera list panel (230px) |
| `clients/flutter/lib/widgets/shell/navigation_shell.dart` | Adaptive layout: icon rail + panel (desktop) or bottom nav (mobile) |
| `clients/flutter/lib/widgets/shell/mobile_bottom_nav.dart` | Mobile bottom navigation bar (4 items) |
| `clients/flutter/lib/providers/camera_panel_provider.dart` | Camera panel state (open/closed, search, group filter) |
| `clients/flutter/lib/providers/grid_layout_provider.dart` | Grid slot assignments (persisted per user) |

### Modified Files
| File | Changes |
|---|---|
| `clients/flutter/lib/theme/nvr_colors.dart` | Replace all color values with Tactical HUD palette |
| `clients/flutter/lib/theme/nvr_theme.dart` | Rebuild theme with new colors, fonts, component styles |
| `clients/flutter/lib/app.dart` | No changes needed (already uses NvrTheme.dark()) |
| `clients/flutter/lib/main.dart` | No changes needed |
| `clients/flutter/lib/router/app_router.dart` | Rename `/cameras` → `/devices`, update shell to use NavigationShell, update index mapping to 4 destinations on mobile |
| `clients/flutter/lib/widgets/adaptive_layout.dart` | Replace with new NavigationShell (this file becomes unused) |
| `clients/flutter/pubspec.yaml` | Add font asset declarations |
| `clients/flutter/lib/screens/login_screen.dart` | Full rebuild with Tactical HUD styling |
| `clients/flutter/lib/screens/server_setup_screen.dart` | Full rebuild with Tactical HUD styling |

---

## Tasks

### Task 1: Bundle Fonts

**Files:**
- Create: `clients/flutter/assets/fonts/` (directory with font files)
- Modify: `clients/flutter/pubspec.yaml`

- [ ] **Step 1: Download and place font files**

Download JetBrains Mono (Regular 400, Medium 500, Bold 700) and IBM Plex Sans (Light 300, Regular 400, Medium 500, SemiBold 600, Bold 700) TTF files from Google Fonts. Place them in `clients/flutter/assets/fonts/`.

```bash
mkdir -p clients/flutter/assets/fonts
# Download JetBrains Mono
curl -L "https://fonts.google.com/download?family=JetBrains+Mono" -o /tmp/jetbrains.zip
unzip -o /tmp/jetbrains.zip -d /tmp/jetbrains
cp /tmp/jetbrains/static/JetBrainsMono-Regular.ttf clients/flutter/assets/fonts/
cp /tmp/jetbrains/static/JetBrainsMono-Medium.ttf clients/flutter/assets/fonts/
cp /tmp/jetbrains/static/JetBrainsMono-Bold.ttf clients/flutter/assets/fonts/

# Download IBM Plex Sans
curl -L "https://fonts.google.com/download?family=IBM+Plex+Sans" -o /tmp/ibmplex.zip
unzip -o /tmp/ibmplex.zip -d /tmp/ibmplex
cp /tmp/ibmplex/static/IBMPlexSans-Light.ttf clients/flutter/assets/fonts/
cp /tmp/ibmplex/static/IBMPlexSans-Regular.ttf clients/flutter/assets/fonts/
cp /tmp/ibmplex/static/IBMPlexSans-Medium.ttf clients/flutter/assets/fonts/
cp /tmp/ibmplex/static/IBMPlexSans-SemiBold.ttf clients/flutter/assets/fonts/
cp /tmp/ibmplex/static/IBMPlexSans-Bold.ttf clients/flutter/assets/fonts/
```

- [ ] **Step 2: Register fonts in pubspec.yaml**

Add to the `flutter:` section of `clients/flutter/pubspec.yaml`:

```yaml
  fonts:
    - family: JetBrainsMono
      fonts:
        - asset: assets/fonts/JetBrainsMono-Regular.ttf
          weight: 400
        - asset: assets/fonts/JetBrainsMono-Medium.ttf
          weight: 500
        - asset: assets/fonts/JetBrainsMono-Bold.ttf
          weight: 700
    - family: IBMPlexSans
      fonts:
        - asset: assets/fonts/IBMPlexSans-Light.ttf
          weight: 300
        - asset: assets/fonts/IBMPlexSans-Regular.ttf
          weight: 400
        - asset: assets/fonts/IBMPlexSans-Medium.ttf
          weight: 500
        - asset: assets/fonts/IBMPlexSans-SemiBold.ttf
          weight: 600
        - asset: assets/fonts/IBMPlexSans-Bold.ttf
          weight: 700
```

- [ ] **Step 3: Verify fonts load**

```bash
cd clients/flutter && flutter pub get
```

- [ ] **Step 4: Commit**

```bash
git add clients/flutter/assets/fonts/ clients/flutter/pubspec.yaml
git commit -m "feat(ui): bundle JetBrains Mono and IBM Plex Sans fonts"
```

---

### Task 2: Redefine Color Palette

**Files:**
- Modify: `clients/flutter/lib/theme/nvr_colors.dart`

- [ ] **Step 1: Replace all color constants**

Replace the entire contents of `clients/flutter/lib/theme/nvr_colors.dart`:

```dart
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
```

- [ ] **Step 2: Commit**

```bash
git add clients/flutter/lib/theme/nvr_colors.dart
git commit -m "feat(ui): redefine color palette — Tactical HUD theme"
```

---

### Task 3: Typography & Animation Constants

**Files:**
- Create: `clients/flutter/lib/theme/nvr_typography.dart`
- Create: `clients/flutter/lib/theme/nvr_animations.dart`

- [ ] **Step 1: Create typography definitions**

Create `clients/flutter/lib/theme/nvr_typography.dart`:

```dart
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
```

- [ ] **Step 2: Create animation constants**

Create `clients/flutter/lib/theme/nvr_animations.dart`:

```dart
import 'package:flutter/material.dart';

class NvrAnimations {
  NvrAnimations._();

  // Micro-interactions (toggles, button presses)
  static const microDuration = Duration(milliseconds: 150);
  static const microCurve = Curves.easeOut;

  // Panel slide in/out
  static const panelDuration = Duration(milliseconds: 250);
  static const panelCurve = Curves.easeInOut;

  // Timeline seek animation
  static const seekDuration = Duration(milliseconds: 300);
  static const seekCurve = Curves.easeInOut;

  // Tour camera transition
  static const tourDuration = Duration(milliseconds: 400);

  // Overlay auto-hide
  static const overlayHideDelay = Duration(milliseconds: 3000);
  static const overlayFadeDuration = Duration(milliseconds: 200);
}
```

- [ ] **Step 3: Commit**

```bash
git add clients/flutter/lib/theme/nvr_typography.dart clients/flutter/lib/theme/nvr_animations.dart
git commit -m "feat(ui): add typography definitions and animation constants"
```

---

### Task 4: Rebuild NvrTheme

**Files:**
- Modify: `clients/flutter/lib/theme/nvr_theme.dart`

- [ ] **Step 1: Replace theme with Tactical HUD styling**

Replace the entire contents of `clients/flutter/lib/theme/nvr_theme.dart`:

```dart
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
```

- [ ] **Step 2: Commit**

```bash
git add clients/flutter/lib/theme/nvr_theme.dart
git commit -m "feat(ui): rebuild NvrTheme with Tactical HUD dark theme"
```

---

### Task 5: Build Custom HUD Widget Library

**Files:**
- Create: `clients/flutter/lib/widgets/hud/corner_brackets.dart`
- Create: `clients/flutter/lib/widgets/hud/status_badge.dart`
- Create: `clients/flutter/lib/widgets/hud/hud_button.dart`
- Create: `clients/flutter/lib/widgets/hud/segmented_control.dart`
- Create: `clients/flutter/lib/widgets/hud/analog_slider.dart`
- Create: `clients/flutter/lib/widgets/hud/hud_toggle.dart`
- Create: `clients/flutter/lib/widgets/hud/rotary_knob.dart`

This is a large task. Each widget should be built and tested individually. Below are the key widgets with their interfaces. The implementer should build each one as a self-contained widget following the spec's component definitions in the Design System section.

- [ ] **Step 1: Create CornerBrackets widget**

Create `clients/flutter/lib/widgets/hud/corner_brackets.dart`:

```dart
import 'package:flutter/material.dart';
import '../../theme/nvr_colors.dart';

/// Amber corner-bracket overlay for camera tiles.
/// Wraps a child widget with L-shaped brackets at each corner.
class CornerBrackets extends StatelessWidget {
  const CornerBrackets({
    super.key,
    required this.child,
    this.bracketSize = 16.0,
    this.strokeWidth = 2.0,
    this.color,
    this.padding = 6.0,
  });

  final Widget child;
  final double bracketSize;
  final double strokeWidth;
  final Color? color;
  final double padding;

  @override
  Widget build(BuildContext context) {
    final c = color ?? NvrColors.accent.withOpacity(0.4);
    return Stack(
      children: [
        child,
        Positioned(
          top: padding, left: padding,
          child: _Bracket(size: bracketSize, stroke: strokeWidth, color: c, corner: _Corner.topLeft),
        ),
        Positioned(
          top: padding, right: padding,
          child: _Bracket(size: bracketSize, stroke: strokeWidth, color: c, corner: _Corner.topRight),
        ),
        Positioned(
          bottom: padding, left: padding,
          child: _Bracket(size: bracketSize, stroke: strokeWidth, color: c, corner: _Corner.bottomLeft),
        ),
        Positioned(
          bottom: padding, right: padding,
          child: _Bracket(size: bracketSize, stroke: strokeWidth, color: c, corner: _Corner.bottomRight),
        ),
      ],
    );
  }
}

enum _Corner { topLeft, topRight, bottomLeft, bottomRight }

class _Bracket extends StatelessWidget {
  const _Bracket({required this.size, required this.stroke, required this.color, required this.corner});
  final double size;
  final double stroke;
  final Color color;
  final _Corner corner;

  @override
  Widget build(BuildContext context) {
    return SizedBox(
      width: size,
      height: size,
      child: CustomPaint(painter: _BracketPainter(stroke: stroke, color: color, corner: corner)),
    );
  }
}

class _BracketPainter extends CustomPainter {
  _BracketPainter({required this.stroke, required this.color, required this.corner});
  final double stroke;
  final Color color;
  final _Corner corner;

  @override
  void paint(Canvas canvas, Size size) {
    final paint = Paint()..color = color..strokeWidth = stroke..style = PaintingStyle.stroke;
    final path = Path();
    switch (corner) {
      case _Corner.topLeft:
        path.moveTo(0, size.height);
        path.lineTo(0, 0);
        path.lineTo(size.width, 0);
      case _Corner.topRight:
        path.moveTo(0, 0);
        path.lineTo(size.width, 0);
        path.lineTo(size.width, size.height);
      case _Corner.bottomLeft:
        path.moveTo(0, 0);
        path.lineTo(0, size.height);
        path.lineTo(size.width, size.height);
      case _Corner.bottomRight:
        path.moveTo(size.width, 0);
        path.lineTo(size.width, size.height);
        path.lineTo(0, size.height);
    }
    canvas.drawPath(path, paint);
  }

  @override
  bool shouldRepaint(covariant _BracketPainter old) =>
      old.color != color || old.stroke != stroke || old.corner != corner;
}
```

- [ ] **Step 2: Create StatusBadge widget**

Create `clients/flutter/lib/widgets/hud/status_badge.dart`:

```dart
import 'package:flutter/material.dart';
import '../../theme/nvr_colors.dart';

class StatusBadge extends StatelessWidget {
  const StatusBadge({
    super.key,
    required this.label,
    required this.color,
    this.showDot = true,
  });

  final String label;
  final Color color;
  final bool showDot;

  factory StatusBadge.online() => const StatusBadge(label: 'ONLINE', color: NvrColors.success);
  factory StatusBadge.offline() => const StatusBadge(label: 'OFFLINE', color: NvrColors.danger);
  factory StatusBadge.degraded() => const StatusBadge(label: 'DEGRADED', color: NvrColors.warning);
  factory StatusBadge.live() => const StatusBadge(label: 'LIVE', color: NvrColors.success);
  factory StatusBadge.recording() => const StatusBadge(label: 'REC', color: NvrColors.danger, showDot: false);
  factory StatusBadge.motion() => const StatusBadge(label: 'MOTION', color: NvrColors.accent);

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 3),
      decoration: BoxDecoration(
        color: color.withOpacity(0.07),
        border: Border.all(color: color.withOpacity(0.27)),
        borderRadius: BorderRadius.circular(4),
      ),
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          if (showDot) ...[
            Container(
              width: 6, height: 6,
              decoration: BoxDecoration(
                shape: BoxShape.circle,
                color: color,
                boxShadow: [BoxShadow(color: color.withOpacity(0.5), blurRadius: 6)],
              ),
            ),
            const SizedBox(width: 5),
          ],
          Text(
            label,
            style: TextStyle(
              fontFamily: 'JetBrainsMono',
              fontSize: 9,
              fontWeight: FontWeight.w500,
              letterSpacing: 0.5,
              color: color,
            ),
          ),
        ],
      ),
    );
  }
}
```

- [ ] **Step 3: Create HudButton widget**

Create `clients/flutter/lib/widgets/hud/hud_button.dart`:

```dart
import 'package:flutter/material.dart';
import '../../theme/nvr_colors.dart';

enum HudButtonStyle { primary, secondary, danger, tactical }

class HudButton extends StatelessWidget {
  const HudButton({
    super.key,
    required this.label,
    required this.onPressed,
    this.style = HudButtonStyle.primary,
    this.icon,
  });

  final String label;
  final VoidCallback? onPressed;
  final HudButtonStyle style;
  final IconData? icon;

  @override
  Widget build(BuildContext context) {
    final (bg, fg, border) = switch (style) {
      HudButtonStyle.primary => (NvrColors.accent, NvrColors.bgPrimary, Colors.transparent),
      HudButtonStyle.secondary => (NvrColors.bgTertiary, NvrColors.textPrimary, NvrColors.border),
      HudButtonStyle.danger => (NvrColors.danger.withOpacity(0.13), NvrColors.danger, NvrColors.danger.withOpacity(0.27)),
      HudButtonStyle.tactical => (NvrColors.bgTertiary, NvrColors.accent, NvrColors.accent.withOpacity(0.27)),
    };

    final textStyle = style == HudButtonStyle.tactical
        ? TextStyle(fontFamily: 'JetBrainsMono', fontSize: 10, letterSpacing: 1, fontWeight: FontWeight.w500, color: fg)
        : TextStyle(fontFamily: 'IBMPlexSans', fontSize: 12, fontWeight: FontWeight.w600, color: fg);

    return Material(
      color: bg,
      borderRadius: BorderRadius.circular(4),
      child: InkWell(
        onTap: onPressed,
        borderRadius: BorderRadius.circular(4),
        child: Container(
          padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 8),
          decoration: BoxDecoration(
            border: Border.all(color: border),
            borderRadius: BorderRadius.circular(4),
          ),
          child: Row(
            mainAxisSize: MainAxisSize.min,
            children: [
              if (icon != null) ...[
                Icon(icon, size: 14, color: fg),
                const SizedBox(width: 6),
              ],
              Text(label, style: textStyle),
            ],
          ),
        ),
      ),
    );
  }
}
```

- [ ] **Step 4: Create SegmentedControl widget**

Create `clients/flutter/lib/widgets/hud/segmented_control.dart`:

```dart
import 'package:flutter/material.dart';
import '../../theme/nvr_colors.dart';

class HudSegmentedControl<T> extends StatelessWidget {
  const HudSegmentedControl({
    super.key,
    required this.segments,
    required this.selected,
    required this.onChanged,
  });

  final Map<T, String> segments;
  final T selected;
  final ValueChanged<T> onChanged;

  @override
  Widget build(BuildContext context) {
    final entries = segments.entries.toList();
    return Container(
      decoration: BoxDecoration(
        color: NvrColors.bgPrimary,
        border: Border.all(color: NvrColors.border),
        borderRadius: BorderRadius.circular(4),
      ),
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          for (int i = 0; i < entries.length; i++) ...[
            if (i > 0) Container(width: 1, height: 24, color: NvrColors.border),
            GestureDetector(
              onTap: () => onChanged(entries[i].key),
              child: Container(
                padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 5),
                color: entries[i].key == selected ? NvrColors.accent.withOpacity(0.13) : Colors.transparent,
                child: Text(
                  entries[i].value,
                  style: TextStyle(
                    fontFamily: 'JetBrainsMono',
                    fontSize: 9,
                    color: entries[i].key == selected ? NvrColors.accent : NvrColors.textMuted,
                  ),
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }
}
```

- [ ] **Step 5: Create AnalogSlider widget**

Create `clients/flutter/lib/widgets/hud/analog_slider.dart`:

```dart
import 'package:flutter/material.dart';
import '../../theme/nvr_colors.dart';
import '../../theme/nvr_animations.dart';

class AnalogSlider extends StatefulWidget {
  const AnalogSlider({
    super.key,
    required this.value,
    required this.onChanged,
    this.label,
    this.min = 0.0,
    this.max = 1.0,
    this.tickCount = 11,
    this.valueFormatter,
  });

  final double value;
  final ValueChanged<double> onChanged;
  final String? label;
  final double min;
  final double max;
  final int tickCount;
  final String Function(double)? valueFormatter;

  @override
  State<AnalogSlider> createState() => _AnalogSliderState();
}

class _AnalogSliderState extends State<AnalogSlider> {
  bool _dragging = false;

  double get _fraction => ((widget.value - widget.min) / (widget.max - widget.min)).clamp(0.0, 1.0);

  void _onPanUpdate(DragUpdateDetails details, BoxConstraints constraints) {
    final dx = details.localPosition.dx.clamp(0.0, constraints.maxWidth);
    final fraction = dx / constraints.maxWidth;
    final value = widget.min + fraction * (widget.max - widget.min);
    widget.onChanged(value.clamp(widget.min, widget.max));
  }

  @override
  Widget build(BuildContext context) {
    final displayValue = widget.valueFormatter?.call(widget.value) ??
        '${(widget.value * 100).round()}%';

    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      mainAxisSize: MainAxisSize.min,
      children: [
        if (widget.label != null)
          Padding(
            padding: const EdgeInsets.only(bottom: 6),
            child: Row(
              mainAxisAlignment: MainAxisAlignment.spaceBetween,
              children: [
                Text(widget.label!, style: TextStyle(
                  fontFamily: 'JetBrainsMono', fontSize: 9,
                  letterSpacing: 1, color: NvrColors.textMuted,
                )),
                Text(displayValue, style: TextStyle(
                  fontFamily: 'JetBrainsMono', fontSize: 9, color: NvrColors.accent,
                )),
              ],
            ),
          ),
        LayoutBuilder(builder: (context, constraints) {
          return GestureDetector(
            onPanStart: (_) => setState(() => _dragging = true),
            onPanUpdate: (d) => _onPanUpdate(d, constraints),
            onPanEnd: (_) => setState(() => _dragging = false),
            onTapDown: (d) {
              final fraction = d.localPosition.dx / constraints.maxWidth;
              final value = widget.min + fraction * (widget.max - widget.min);
              widget.onChanged(value.clamp(widget.min, widget.max));
            },
            child: SizedBox(
              height: 24,
              child: Stack(
                clipBehavior: Clip.none,
                alignment: Alignment.centerLeft,
                children: [
                  // Track
                  Container(
                    height: 6,
                    decoration: BoxDecoration(
                      color: NvrColors.bgTertiary,
                      border: Border.all(color: NvrColors.border),
                      borderRadius: BorderRadius.circular(3),
                    ),
                  ),
                  // Fill
                  FractionallySizedBox(
                    widthFactor: _fraction,
                    child: Container(
                      height: 6,
                      decoration: BoxDecoration(
                        gradient: LinearGradient(
                          colors: [NvrColors.accent, NvrColors.accent.withOpacity(0.4)],
                        ),
                        borderRadius: BorderRadius.circular(3),
                      ),
                    ),
                  ),
                  // Thumb
                  Positioned(
                    left: _fraction * constraints.maxWidth - 9,
                    child: AnimatedContainer(
                      duration: NvrAnimations.microDuration,
                      width: _dragging ? 20 : 18,
                      height: _dragging ? 20 : 18,
                      decoration: BoxDecoration(
                        shape: BoxShape.circle,
                        color: NvrColors.bgTertiary,
                        border: Border.all(color: NvrColors.accent, width: 2),
                        boxShadow: [
                          BoxShadow(
                            color: NvrColors.accent.withOpacity(_dragging ? 0.5 : 0.25),
                            blurRadius: _dragging ? 10 : 6,
                          ),
                        ],
                      ),
                    ),
                  ),
                ],
              ),
            ),
          );
        }),
        // Tick marks
        Padding(
          padding: const EdgeInsets.only(top: 4),
          child: Row(
            mainAxisAlignment: MainAxisAlignment.spaceBetween,
            children: List.generate(widget.tickCount, (_) =>
              Container(width: 1, height: 4, color: NvrColors.border),
            ),
          ),
        ),
      ],
    );
  }
}
```

- [ ] **Step 6: Create HudToggle widget**

Create `clients/flutter/lib/widgets/hud/hud_toggle.dart`:

```dart
import 'package:flutter/material.dart';
import '../../theme/nvr_colors.dart';
import '../../theme/nvr_animations.dart';

class HudToggle extends StatelessWidget {
  const HudToggle({
    super.key,
    required this.value,
    required this.onChanged,
    this.label,
    this.showStateLabel = true,
  });

  final bool value;
  final ValueChanged<bool> onChanged;
  final String? label;
  final bool showStateLabel;

  @override
  Widget build(BuildContext context) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      mainAxisSize: MainAxisSize.min,
      children: [
        if (label != null)
          Padding(
            padding: const EdgeInsets.only(bottom: 6),
            child: Text(label!, style: TextStyle(
              fontFamily: 'JetBrainsMono', fontSize: 9,
              letterSpacing: 1, color: NvrColors.textMuted,
            )),
          ),
        GestureDetector(
          onTap: () => onChanged(!value),
          child: AnimatedContainer(
            duration: NvrAnimations.microDuration,
            curve: NvrAnimations.microCurve,
            width: 44, height: 22,
            decoration: BoxDecoration(
              color: NvrColors.bgTertiary,
              borderRadius: BorderRadius.circular(11),
              border: Border.all(
                color: value ? NvrColors.accent : NvrColors.border,
                width: 2,
              ),
              boxShadow: value ? [
                BoxShadow(color: NvrColors.accent.withOpacity(0.2), blurRadius: 8),
              ] : null,
            ),
            child: AnimatedAlign(
              duration: NvrAnimations.microDuration,
              curve: NvrAnimations.microCurve,
              alignment: value ? Alignment.centerRight : Alignment.centerLeft,
              child: Padding(
                padding: const EdgeInsets.all(2),
                child: Container(
                  width: 14, height: 14,
                  decoration: BoxDecoration(
                    shape: BoxShape.circle,
                    color: value ? NvrColors.accent : NvrColors.textMuted,
                    boxShadow: value ? [
                      BoxShadow(color: NvrColors.accent.withOpacity(0.4), blurRadius: 6),
                    ] : null,
                  ),
                ),
              ),
            ),
          ),
        ),
        if (showStateLabel)
          Padding(
            padding: const EdgeInsets.only(top: 4),
            child: Text(
              value ? 'ON' : 'OFF',
              style: TextStyle(
                fontFamily: 'JetBrainsMono', fontSize: 8,
                letterSpacing: 1,
                color: value ? NvrColors.accent : NvrColors.textMuted,
              ),
            ),
          ),
      ],
    );
  }
}
```

- [ ] **Step 7: Create RotaryKnob widget**

Create `clients/flutter/lib/widgets/hud/rotary_knob.dart`:

```dart
import 'dart:math';
import 'package:flutter/material.dart';
import '../../theme/nvr_colors.dart';

class RotaryKnob extends StatefulWidget {
  const RotaryKnob({
    super.key,
    required this.value,
    required this.onChanged,
    this.label,
    this.min = 0.0,
    this.max = 1.0,
    this.size = 40.0,
    this.valueFormatter,
  });

  final double value;
  final ValueChanged<double> onChanged;
  final String? label;
  final double min;
  final double max;
  final double size;
  final String Function(double)? valueFormatter;

  @override
  State<RotaryKnob> createState() => _RotaryKnobState();
}

class _RotaryKnobState extends State<RotaryKnob> {
  double? _startAngle;
  double? _startValue;

  double get _fraction => ((widget.value - widget.min) / (widget.max - widget.min)).clamp(0.0, 1.0);
  // Map fraction to angle: -135° to +135° (270° sweep)
  double get _angle => -135 + _fraction * 270;

  void _onPanStart(DragStartDetails details) {
    final center = Offset(widget.size / 2, widget.size / 2);
    _startAngle = (details.localPosition - center).direction;
    _startValue = widget.value;
  }

  void _onPanUpdate(DragUpdateDetails details) {
    if (_startAngle == null || _startValue == null) return;
    final center = Offset(widget.size / 2, widget.size / 2);
    final currentAngle = (details.localPosition - center).direction;
    final delta = currentAngle - _startAngle!;
    final valueDelta = delta / pi * (widget.max - widget.min) * 0.5;
    final newValue = (_startValue! + valueDelta).clamp(widget.min, widget.max);
    widget.onChanged(newValue);
  }

  @override
  Widget build(BuildContext context) {
    final display = widget.valueFormatter?.call(widget.value) ??
        '${(widget.value * 100).round()}%';

    return Column(
      mainAxisSize: MainAxisSize.min,
      children: [
        if (widget.label != null)
          Padding(
            padding: const EdgeInsets.only(bottom: 6),
            child: Text(widget.label!, style: TextStyle(
              fontFamily: 'JetBrainsMono', fontSize: 9,
              letterSpacing: 1, color: NvrColors.textMuted,
            )),
          ),
        GestureDetector(
          onPanStart: _onPanStart,
          onPanUpdate: _onPanUpdate,
          child: SizedBox(
            width: widget.size,
            height: widget.size,
            child: CustomPaint(
              painter: _KnobPainter(angle: _angle),
            ),
          ),
        ),
        const SizedBox(height: 4),
        Text(display, style: TextStyle(
          fontFamily: 'JetBrainsMono', fontSize: 9, color: NvrColors.accent,
        )),
      ],
    );
  }
}

class _KnobPainter extends CustomPainter {
  _KnobPainter({required this.angle});
  final double angle;

  @override
  void paint(Canvas canvas, Size size) {
    final center = Offset(size.width / 2, size.height / 2);
    final radius = size.width / 2;

    // Body gradient
    final bodyPaint = Paint()
      ..shader = RadialGradient(
        center: const Alignment(-0.2, -0.2),
        colors: [NvrColors.bgTertiary, NvrColors.bgPrimary],
      ).createShader(Rect.fromCircle(center: center, radius: radius));
    canvas.drawCircle(center, radius, bodyPaint);

    // Border
    canvas.drawCircle(center, radius, Paint()
      ..color = NvrColors.border
      ..style = PaintingStyle.stroke
      ..strokeWidth = 2);

    // Notch marks
    final notchPaint = Paint()..color = NvrColors.border..strokeWidth = 1;
    for (var i = 0; i < 4; i++) {
      final a = i * pi / 2;
      final outer = center + Offset(cos(a), sin(a)) * (radius + 4);
      final inner = center + Offset(cos(a), sin(a)) * radius;
      canvas.drawLine(inner, outer, notchPaint);
    }

    // Indicator line
    final rad = angle * pi / 180;
    final indicatorPaint = Paint()
      ..color = NvrColors.accent
      ..strokeWidth = 2
      ..strokeCap = StrokeCap.round;
    final from = center + Offset(cos(rad), sin(rad)) * 4;
    final to = center + Offset(cos(rad), sin(rad)) * (radius - 4);
    canvas.drawLine(from, to, indicatorPaint);
  }

  @override
  bool shouldRepaint(covariant _KnobPainter old) => old.angle != angle;
}
```

- [ ] **Step 8: Commit all HUD widgets**

```bash
git add clients/flutter/lib/widgets/hud/
git commit -m "feat(ui): add HUD widget library — brackets, badges, buttons, slider, toggle, knob, segments"
```

---

### Task 6: Camera Panel Provider

**Files:**
- Create: `clients/flutter/lib/providers/camera_panel_provider.dart`

- [ ] **Step 1: Create camera panel state provider**

Create `clients/flutter/lib/providers/camera_panel_provider.dart`:

```dart
import 'package:flutter_riverpod/flutter_riverpod.dart';

class CameraPanelState {
  const CameraPanelState({
    this.isOpen = false,
    this.searchQuery = '',
    this.activeGroupId,
  });

  final bool isOpen;
  final String searchQuery;
  final String? activeGroupId;

  CameraPanelState copyWith({
    bool? isOpen,
    String? searchQuery,
    String? activeGroupId,
    bool clearGroupFilter = false,
  }) {
    return CameraPanelState(
      isOpen: isOpen ?? this.isOpen,
      searchQuery: searchQuery ?? this.searchQuery,
      activeGroupId: clearGroupFilter ? null : (activeGroupId ?? this.activeGroupId),
    );
  }
}

class CameraPanelNotifier extends StateNotifier<CameraPanelState> {
  CameraPanelNotifier() : super(const CameraPanelState());

  void toggle() => state = state.copyWith(isOpen: !state.isOpen);
  void open() => state = state.copyWith(isOpen: true);
  void close() => state = state.copyWith(isOpen: false);
  void setSearch(String query) => state = state.copyWith(searchQuery: query);
  void setGroupFilter(String? groupId) {
    if (groupId == state.activeGroupId) {
      state = state.copyWith(clearGroupFilter: true);
    } else {
      state = state.copyWith(activeGroupId: groupId);
    }
  }
}

final cameraPanelProvider =
    StateNotifierProvider<CameraPanelNotifier, CameraPanelState>(
  (ref) => CameraPanelNotifier(),
);
```

- [ ] **Step 2: Commit**

```bash
git add clients/flutter/lib/providers/camera_panel_provider.dart
git commit -m "feat(ui): add camera panel state provider"
```

---

### Task 7: Build Navigation Shell

**Files:**
- Create: `clients/flutter/lib/widgets/shell/icon_rail.dart`
- Create: `clients/flutter/lib/widgets/shell/mobile_bottom_nav.dart`
- Create: `clients/flutter/lib/widgets/shell/camera_panel.dart`
- Create: `clients/flutter/lib/widgets/shell/navigation_shell.dart`

- [ ] **Step 1: Create IconRail widget**

Create `clients/flutter/lib/widgets/shell/icon_rail.dart`:

```dart
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import '../../theme/nvr_colors.dart';
import '../../providers/notifications_provider.dart';

class IconRail extends ConsumerWidget {
  const IconRail({
    super.key,
    required this.selectedIndex,
    required this.onDestinationSelected,
    required this.onAlertsTap,
    required this.onCameraPanelToggle,
  });

  final int selectedIndex;
  final ValueChanged<int> onDestinationSelected;
  final VoidCallback onAlertsTap;
  final VoidCallback onCameraPanelToggle;

  static const _navItems = [
    (icon: Icons.videocam_outlined, activeIcon: Icons.videocam, label: 'Live'),
    (icon: Icons.access_time_outlined, activeIcon: Icons.access_time_filled, label: 'Playback'),
    (icon: Icons.search_outlined, activeIcon: Icons.search, label: 'Search'),
    (icon: Icons.camera_alt_outlined, activeIcon: Icons.camera_alt, label: 'Devices'),
  ];

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final unreadCount = ref.watch(notificationsProvider.select((s) => s.unreadCount));

    return Container(
      width: 60,
      color: NvrColors.bgSecondary,
      child: Column(
        children: [
          const SizedBox(height: 14),
          // Logo
          Transform.rotate(
            angle: 0.785398, // 45 degrees
            child: Container(
              width: 18, height: 18,
              decoration: BoxDecoration(
                border: Border.all(color: NvrColors.accent, width: 2),
              ),
            ),
          ),
          const SizedBox(height: 16),
          // Nav items
          for (int i = 0; i < _navItems.length; i++) ...[
            if (i == 3) ...[
              Padding(
                padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 6),
                child: Container(height: 1, color: NvrColors.border),
              ),
            ],
            _NavIcon(
              icon: i == selectedIndex ? _navItems[i].activeIcon : _navItems[i].icon,
              isActive: i == selectedIndex,
              onTap: () {
                if (i == selectedIndex) {
                  onCameraPanelToggle();
                } else {
                  onDestinationSelected(i);
                }
              },
              semanticLabel: _navItems[i].label,
            ),
            const SizedBox(height: 6),
          ],
          const Spacer(),
          // Alerts
          _NavIcon(
            icon: Icons.notifications_outlined,
            isActive: false,
            badge: unreadCount > 0 ? unreadCount : null,
            onTap: onAlertsTap,
            semanticLabel: 'Alerts',
          ),
          const SizedBox(height: 6),
          // Settings
          _NavIcon(
            icon: Icons.settings_outlined,
            isActive: false,
            muted: true,
            onTap: () => onDestinationSelected(4),
            semanticLabel: 'Settings',
          ),
          const SizedBox(height: 14),
        ],
      ),
    );
  }
}

class _NavIcon extends StatelessWidget {
  const _NavIcon({
    required this.icon,
    required this.isActive,
    required this.onTap,
    required this.semanticLabel,
    this.badge,
    this.muted = false,
  });

  final IconData icon;
  final bool isActive;
  final VoidCallback onTap;
  final String semanticLabel;
  final int? badge;
  final bool muted;

  @override
  Widget build(BuildContext context) {
    return Semantics(
      label: semanticLabel,
      button: true,
      child: Stack(
        clipBehavior: Clip.none,
        children: [
          // Active indicator bar
          if (isActive)
            Positioned(
              left: -10, top: 10, bottom: 10,
              child: Container(width: 3, decoration: BoxDecoration(
                color: NvrColors.accent,
                borderRadius: BorderRadius.circular(2),
              )),
            ),
          Material(
            color: isActive ? NvrColors.accent.withOpacity(0.13) : Colors.transparent,
            borderRadius: BorderRadius.circular(8),
            child: InkWell(
              borderRadius: BorderRadius.circular(8),
              onTap: onTap,
              child: Container(
                width: 40, height: 40,
                decoration: isActive ? BoxDecoration(
                  borderRadius: BorderRadius.circular(8),
                  border: Border.all(color: NvrColors.accent.withOpacity(0.27)),
                ) : null,
                child: Icon(
                  icon, size: 20,
                  color: isActive ? NvrColors.accent : muted ? NvrColors.textMuted : NvrColors.textSecondary,
                ),
              ),
            ),
          ),
          // Badge
          if (badge != null)
            Positioned(
              right: -2, top: -2,
              child: Container(
                padding: const EdgeInsets.all(3),
                decoration: BoxDecoration(
                  color: NvrColors.danger,
                  shape: BoxShape.circle,
                  border: Border.all(color: NvrColors.bgSecondary, width: 2),
                  boxShadow: [BoxShadow(color: NvrColors.danger.withOpacity(0.5), blurRadius: 6)],
                ),
                child: Text(
                  badge! > 9 ? '9+' : '$badge',
                  style: const TextStyle(fontSize: 7, fontWeight: FontWeight.bold, color: Colors.white),
                ),
              ),
            ),
        ],
      ),
    );
  }
}
```

- [ ] **Step 2: Create MobileBottomNav widget**

Create `clients/flutter/lib/widgets/shell/mobile_bottom_nav.dart`:

```dart
import 'package:flutter/material.dart';
import '../../theme/nvr_colors.dart';

class MobileBottomNav extends StatelessWidget {
  const MobileBottomNav({
    super.key,
    required this.selectedIndex,
    required this.onDestinationSelected,
  });

  final int selectedIndex;
  final ValueChanged<int> onDestinationSelected;

  static const _items = [
    (icon: Icons.videocam_outlined, activeIcon: Icons.videocam, label: 'LIVE'),
    (icon: Icons.access_time_outlined, activeIcon: Icons.access_time_filled, label: 'PLAYBACK'),
    (icon: Icons.search_outlined, activeIcon: Icons.search, label: 'SEARCH'),
    (icon: Icons.settings_outlined, activeIcon: Icons.settings, label: 'SETTINGS'),
  ];

  @override
  Widget build(BuildContext context) {
    return Container(
      decoration: const BoxDecoration(
        color: NvrColors.bgSecondary,
        border: Border(top: BorderSide(color: NvrColors.border)),
      ),
      child: SafeArea(
        child: SizedBox(
          height: 56,
          child: Row(
            mainAxisAlignment: MainAxisAlignment.spaceAround,
            children: [
              for (int i = 0; i < _items.length; i++)
                Expanded(
                  child: GestureDetector(
                    behavior: HitTestBehavior.opaque,
                    onTap: () => onDestinationSelected(i),
                    child: Column(
                      mainAxisAlignment: MainAxisAlignment.center,
                      children: [
                        Icon(
                          i == selectedIndex ? _items[i].activeIcon : _items[i].icon,
                          size: 20,
                          color: i == selectedIndex ? NvrColors.accent : NvrColors.textSecondary,
                        ),
                        const SizedBox(height: 3),
                        Text(
                          _items[i].label,
                          style: TextStyle(
                            fontFamily: 'JetBrainsMono',
                            fontSize: 8,
                            letterSpacing: 0.5,
                            color: i == selectedIndex ? NvrColors.accent : NvrColors.textSecondary,
                          ),
                        ),
                      ],
                    ),
                  ),
                ),
            ],
          ),
        ),
      ),
    );
  }
}
```

- [ ] **Step 3: Create CameraPanel widget**

Create `clients/flutter/lib/widgets/shell/camera_panel.dart`:

```dart
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import '../../theme/nvr_colors.dart';
import '../../theme/nvr_typography.dart';
import '../../providers/camera_panel_provider.dart';
import '../../providers/cameras_provider.dart';
import '../hud/status_badge.dart';

class CameraPanel extends ConsumerWidget {
  const CameraPanel({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final panelState = ref.watch(cameraPanelProvider);
    final camerasAsync = ref.watch(camerasProvider);

    return Container(
      width: 230,
      color: const Color(0xFF0e0e0e),
      child: Column(
        children: [
          // Header
          Container(
            padding: const EdgeInsets.fromLTRB(16, 14, 16, 10),
            decoration: const BoxDecoration(
              border: Border(bottom: BorderSide(color: NvrColors.border)),
            ),
            child: Row(
              children: [
                Text('CAMERAS', style: NvrTypography.monoSection),
                const Spacer(),
                GestureDetector(
                  onTap: () => ref.read(cameraPanelProvider.notifier).close(),
                  child: const Icon(Icons.close, size: 16, color: NvrColors.textMuted),
                ),
              ],
            ),
          ),
          // Search
          Padding(
            padding: const EdgeInsets.all(10),
            child: TextField(
              onChanged: (q) => ref.read(cameraPanelProvider.notifier).setSearch(q),
              style: const TextStyle(fontSize: 12, color: NvrColors.textPrimary),
              decoration: InputDecoration(
                hintText: 'Search cameras...',
                prefixIcon: const Icon(Icons.search, size: 16, color: NvrColors.textMuted),
                isDense: true,
                contentPadding: const EdgeInsets.symmetric(horizontal: 10, vertical: 8),
                filled: true,
                fillColor: NvrColors.bgTertiary,
                border: OutlineInputBorder(
                  borderRadius: BorderRadius.circular(6),
                  borderSide: const BorderSide(color: NvrColors.border),
                ),
              ),
            ),
          ),
          // Camera list
          Expanded(
            child: camerasAsync.when(
              data: (cameras) {
                final filtered = panelState.searchQuery.isEmpty
                    ? cameras
                    : cameras.where((c) => c.name.toLowerCase().contains(panelState.searchQuery.toLowerCase())).toList();

                if (filtered.isEmpty) {
                  return Center(child: Text('No cameras found', style: NvrTypography.body));
                }

                return ListView.builder(
                  padding: const EdgeInsets.symmetric(horizontal: 10),
                  itemCount: filtered.length,
                  itemBuilder: (context, index) {
                    final cam = filtered[index];
                    final isOnline = cam.status == 'online';
                    return Padding(
                      padding: const EdgeInsets.only(bottom: 3),
                      child: Container(
                        padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 6),
                        decoration: BoxDecoration(
                          borderRadius: BorderRadius.circular(6),
                          color: NvrColors.bgTertiary,
                          border: Border.all(color: NvrColors.border),
                        ),
                        child: Row(
                          children: [
                            Container(
                              width: 6, height: 6,
                              decoration: BoxDecoration(
                                shape: BoxShape.circle,
                                color: isOnline ? NvrColors.success : NvrColors.danger,
                                boxShadow: isOnline ? [
                                  BoxShadow(color: NvrColors.success.withOpacity(0.5), blurRadius: 4),
                                ] : null,
                              ),
                            ),
                            const SizedBox(width: 8),
                            // Thumbnail placeholder
                            Container(
                              width: 44, height: 26,
                              decoration: BoxDecoration(
                                color: NvrColors.border,
                                borderRadius: BorderRadius.circular(3),
                              ),
                            ),
                            const SizedBox(width: 8),
                            Expanded(
                              child: Column(
                                crossAxisAlignment: CrossAxisAlignment.start,
                                children: [
                                  Text(cam.name, style: const TextStyle(
                                    fontSize: 11, fontWeight: FontWeight.w500,
                                    color: NvrColors.textPrimary,
                                  ), overflow: TextOverflow.ellipsis),
                                  Text(cam.id.substring(0, 8).toUpperCase(), style: TextStyle(
                                    fontFamily: 'JetBrainsMono', fontSize: 8,
                                    color: NvrColors.textMuted,
                                  )),
                                ],
                              ),
                            ),
                            const Icon(Icons.drag_handle, size: 14, color: NvrColors.border),
                          ],
                        ),
                      ),
                    );
                  },
                );
              },
              loading: () => const Center(child: CircularProgressIndicator(color: NvrColors.accent)),
              error: (e, _) => Center(child: Text('Error loading cameras', style: NvrTypography.alert)),
            ),
          ),
        ],
      ),
    );
  }
}
```

- [ ] **Step 4: Create NavigationShell widget**

Create `clients/flutter/lib/widgets/shell/navigation_shell.dart`:

```dart
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import '../../theme/nvr_colors.dart';
import '../../theme/nvr_animations.dart';
import '../../providers/camera_panel_provider.dart';
import 'icon_rail.dart';
import 'mobile_bottom_nav.dart';
import 'camera_panel.dart';

class NavigationShell extends ConsumerWidget {
  const NavigationShell({
    super.key,
    required this.selectedIndex,
    required this.onDestinationSelected,
    required this.child,
  });

  final int selectedIndex;
  final ValueChanged<int> onDestinationSelected;
  final Widget child;

  void _onAlertsTap(BuildContext context) {
    // TODO: Show alerts panel (Plan 2)
  }

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final width = MediaQuery.of(context).size.width;
    final panelState = ref.watch(cameraPanelProvider);

    // Mobile: < 600px
    if (width < 600) {
      // Map mobile 4-item nav to router indices
      // Mobile: 0=Live, 1=Playback, 2=Search, 3=Settings(index 4 in router)
      final mobileIndex = selectedIndex == 4 ? 3 : selectedIndex.clamp(0, 2);
      return Scaffold(
        body: child,
        bottomNavigationBar: MobileBottomNav(
          selectedIndex: mobileIndex,
          onDestinationSelected: (i) {
            // Map mobile indices back: 0=Live, 1=Playback, 2=Search, 3=Settings(4)
            onDestinationSelected(i == 3 ? 4 : i);
          },
        ),
      );
    }

    // Desktop/Tablet: >= 600px
    final usePushPanel = width >= 1024;

    return Scaffold(
      body: Row(
        children: [
          IconRail(
            selectedIndex: selectedIndex.clamp(0, 3),
            onDestinationSelected: onDestinationSelected,
            onAlertsTap: () => _onAlertsTap(context),
            onCameraPanelToggle: () => ref.read(cameraPanelProvider.notifier).toggle(),
          ),
          Container(width: 1, color: NvrColors.border),
          // Camera panel (push or overlay based on breakpoint)
          if (usePushPanel && panelState.isOpen) ...[
            const CameraPanel(),
            Container(width: 1, color: NvrColors.border),
          ],
          // Main content
          Expanded(
            child: Stack(
              children: [
                child,
                // Overlay panel for tablet portrait (600-1024)
                if (!usePushPanel && panelState.isOpen) ...[
                  // Scrim
                  GestureDetector(
                    onTap: () => ref.read(cameraPanelProvider.notifier).close(),
                    child: Container(color: Colors.black54),
                  ),
                  // Panel
                  const Positioned(left: 0, top: 0, bottom: 0, child: CameraPanel()),
                ],
              ],
            ),
          ),
        ],
      ),
    );
  }
}
```

- [ ] **Step 5: Commit**

```bash
git add clients/flutter/lib/widgets/shell/
git commit -m "feat(ui): build navigation shell — icon rail, camera panel, mobile bottom nav"
```

---

### Task 8: Update Router

**Files:**
- Modify: `clients/flutter/lib/router/app_router.dart`

- [ ] **Step 1: Update router to use NavigationShell and rename cameras → devices**

Update `clients/flutter/lib/router/app_router.dart`. Key changes:
- Replace `AdaptiveLayout` with `NavigationShell` in the ShellRoute builder
- Rename `/cameras` path to `/devices`
- Update `_indexFromPath` to map the new paths
- Map settings to index 4

```dart
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import '../providers/auth_provider.dart';
import '../widgets/shell/navigation_shell.dart';
import '../screens/login_screen.dart';
import '../screens/server_setup_screen.dart';
import '../screens/setup_screen.dart';
import '../screens/live_view/live_view_screen.dart';
import '../screens/live_view/fullscreen_view.dart';
import '../screens/playback/playback_screen.dart';
import '../screens/search/clip_search_screen.dart';
import '../screens/cameras/camera_list_screen.dart';
import '../screens/cameras/add_camera_screen.dart';
import '../screens/cameras/camera_detail_screen.dart';
import '../screens/settings/settings_screen.dart';
import '../models/camera.dart';

int _indexFromPath(String path) {
  if (path.startsWith('/live')) return 0;
  if (path.startsWith('/playback')) return 1;
  if (path.startsWith('/search')) return 2;
  if (path.startsWith('/devices')) return 3;
  if (path.startsWith('/settings')) return 4;
  return 0;
}

void _navigateToIndex(BuildContext context, int index) {
  const paths = ['/live', '/playback', '/search', '/devices', '/settings'];
  context.go(paths[index]);
}

final routerProvider = Provider<GoRouter>((ref) {
  final authState = ref.watch(authProvider);

  return GoRouter(
    initialLocation: '/live',
    redirect: (context, state) {
      final status = authState.status;
      final path = state.uri.path;
      final isAuthRoute = path == '/login' || path == '/server-setup' || path == '/setup';

      if (status == AuthStatus.serverNeeded && path != '/server-setup') return '/server-setup';
      if (status == AuthStatus.unauthenticated && !isAuthRoute) return '/login';
      if (status == AuthStatus.authenticated && isAuthRoute) return '/live';
      return null;
    },
    routes: [
      GoRoute(path: '/server-setup', builder: (_, __) => const ServerSetupScreen()),
      GoRoute(path: '/login', builder: (_, __) => const LoginScreen()),
      GoRoute(path: '/setup', builder: (_, __) => const SetupScreen()),
      ShellRoute(
        builder: (context, state, child) {
          final index = _indexFromPath(state.uri.path);
          return NavigationShell(
            selectedIndex: index,
            onDestinationSelected: (i) => _navigateToIndex(context, i),
            child: child,
          );
        },
        routes: [
          GoRoute(path: '/live', builder: (_, __) => const LiveViewScreen(), routes: [
            GoRoute(path: 'fullscreen', builder: (_, state) =>
              FullscreenView(camera: state.extra as Camera)),
          ]),
          GoRoute(path: '/playback', builder: (_, __) => const PlaybackScreen()),
          GoRoute(path: '/search', builder: (_, __) => const ClipSearchScreen()),
          GoRoute(path: '/devices', builder: (_, __) => const CameraListScreen(), routes: [
            GoRoute(path: 'add', builder: (_, __) => const AddCameraScreen()),
            GoRoute(path: ':id', builder: (_, state) =>
              CameraDetailScreen(cameraId: state.pathParameters['id']!)),
          ]),
          GoRoute(path: '/settings', builder: (_, __) => const SettingsScreen()),
        ],
      ),
    ],
  );
});
```

- [ ] **Step 2: Update any `/cameras` references in screen files to `/devices`**

Search for `context.go('/cameras` or `context.push('/cameras` across all screen files and update to `/devices`.

```bash
cd clients/flutter && grep -rl "'/cameras" lib/screens/ | head -20
```

Update each occurrence.

- [ ] **Step 3: Commit**

```bash
git add clients/flutter/lib/router/app_router.dart
git add -u clients/flutter/lib/screens/
git commit -m "feat(ui): update router — NavigationShell, /cameras → /devices"
```

---

### Task 9: Rebuild Login Screen

**Files:**
- Modify: `clients/flutter/lib/screens/login_screen.dart`

- [ ] **Step 1: Rebuild LoginScreen with Tactical HUD styling**

Rebuild `clients/flutter/lib/screens/login_screen.dart` preserving the existing auth logic (`_signIn` method and `authProvider` usage) but replacing all UI with the new design system:

- `bgPrimary` background, center-aligned card
- Rotated diamond logo at top with `accent` color
- App name "MediaMTX NVR" in `pageTitle` style
- Username/Password fields with JetBrains Mono labels (`USERNAME`, `PASSWORD`)
- "Sign In" button using primary `ElevatedButton` style
- Error state: shake animation + `danger` error message
- Server URL at bottom in `textMuted` with "Change" link

Key structural change: preserve `ConsumerStatefulWidget`, `_signIn()`, `_formKey`, and all controller logic. Only change the `build()` method's widget tree.

- [ ] **Step 2: Commit**

```bash
git add clients/flutter/lib/screens/login_screen.dart
git commit -m "feat(ui): rebuild LoginScreen with Tactical HUD styling"
```

---

### Task 10: Rebuild Server Setup Screen

**Files:**
- Modify: `clients/flutter/lib/screens/server_setup_screen.dart`

- [ ] **Step 1: Rebuild ServerSetupScreen with Tactical HUD styling**

Same approach as LoginScreen — preserve `_connect()` logic and state management, replace UI:

- `bgPrimary` background, center-aligned card
- Rotated diamond logo
- `SERVER URL` label (JetBrains Mono uppercase)
- URL input field with `http://` placeholder
- "Connect" button (primary style)
- Error state: `danger` colored border on input + error message below
- Loading spinner during connection test

- [ ] **Step 2: Commit**

```bash
git add clients/flutter/lib/screens/server_setup_screen.dart
git commit -m "feat(ui): rebuild ServerSetupScreen with Tactical HUD styling"
```

---

### Task 11: Verify and Clean Up

- [ ] **Step 1: Run Flutter analyze**

```bash
cd clients/flutter && flutter analyze
```

Fix any lint errors or warnings.

- [ ] **Step 2: Verify the app builds and navigates**

```bash
cd clients/flutter && flutter build apk --debug 2>&1 | tail -5
```

- [ ] **Step 3: Remove old AdaptiveLayout if no longer imported**

Check if `clients/flutter/lib/widgets/adaptive_layout.dart` is still imported anywhere. If not, delete it or leave it for now (Plan 2 screens may still reference it during transition).

```bash
cd clients/flutter && grep -rl "adaptive_layout" lib/ | head
```

- [ ] **Step 4: Final commit**

```bash
git add -u clients/flutter/
git commit -m "chore(ui): fix lint warnings and cleanup after foundation build"
```
