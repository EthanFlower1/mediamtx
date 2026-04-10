import 'dart:math' as math;
import 'package:flutter/material.dart';

/// WCAG 2.1 contrast-ratio utilities.
///
/// Implements the relative-luminance and contrast-ratio formulas from
/// https://www.w3.org/TR/WCAG21/#dfn-contrast-ratio so that theme
/// colours can be validated at build time or in tests.
class ContrastChecker {
  ContrastChecker._(); // prevent instantiation

  /// Returns the WCAG relative luminance of [color].
  ///
  /// See https://www.w3.org/TR/WCAG21/#dfn-relative-luminance
  static double _relativeLuminance(Color color) {
    double linearize(int channel) {
      final s = channel / 255.0;
      return s <= 0.04045 ? s / 12.92 : math.pow((s + 0.055) / 1.055, 2.4).toDouble();
    }

    final r = linearize(color.red);
    final g = linearize(color.green);
    final b = linearize(color.blue);
    return 0.2126 * r + 0.7152 * g + 0.0722 * b;
  }

  /// Computes the WCAG contrast ratio between [foreground] and [background].
  ///
  /// The result ranges from 1.0 (identical) to 21.0 (black on white).
  static double contrastRatio(Color foreground, Color background) {
    final lum1 = _relativeLuminance(foreground);
    final lum2 = _relativeLuminance(background);
    final lighter = math.max(lum1, lum2);
    final darker = math.min(lum1, lum2);
    return (lighter + 0.05) / (darker + 0.05);
  }

  /// Returns `true` when the contrast ratio between [foreground] and
  /// [background] meets WCAG 2.1 **AA** requirements.
  ///
  /// Normal text needs >= 4.5:1; [largeText] (>= 18 pt or >= 14 pt bold)
  /// needs >= 3:1.
  static bool meetsAA(
    Color foreground,
    Color background, {
    bool largeText = false,
  }) {
    final ratio = contrastRatio(foreground, background);
    return ratio >= (largeText ? 3.0 : 4.5);
  }

  /// Returns `true` when the contrast ratio between [foreground] and
  /// [background] meets WCAG 2.1 **AAA** requirements.
  ///
  /// Normal text needs >= 7:1; [largeText] needs >= 4.5:1.
  static bool meetsAAA(
    Color foreground,
    Color background, {
    bool largeText = false,
  }) {
    final ratio = contrastRatio(foreground, background);
    return ratio >= (largeText ? 4.5 : 7.0);
  }
}
