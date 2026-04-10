import 'package:flutter/material.dart';
import 'package:flutter/semantics.dart';

/// Accessibility semantics utilities for Section 508 / WCAG 2.1 AA compliance.
///
/// Provides static helpers that wrap widgets with appropriate semantic
/// annotations so assistive technologies (TalkBack, VoiceOver) can
/// interpret the UI correctly.
class A11ySemantics {
  A11ySemantics._(); // prevent instantiation

  /// Wraps [child] in a [Semantics] node with the given [label].
  static Widget withLabel(Widget child, String label) {
    return Semantics(
      label: label,
      child: child,
    );
  }

  /// Wraps [child] as a semantic button with [label] and optional [onTap].
  static Widget button({
    required Widget child,
    required String label,
    VoidCallback? onTap,
  }) {
    return Semantics(
      button: true,
      label: label,
      onTap: onTap,
      child: child,
    );
  }

  /// Wraps [child] as a semantic image with the given [description].
  static Widget image(Widget child, String description) {
    return Semantics(
      image: true,
      label: description,
      child: child,
    );
  }

  /// Wraps [child] as a live region so assistive technologies announce
  /// dynamic content updates automatically.
  static Widget liveRegion(Widget child, String label) {
    return Semantics(
      liveRegion: true,
      label: label,
      child: child,
    );
  }

  /// Excludes purely decorative [child] from the semantics tree.
  static Widget excludeDecorative(Widget child) {
    return ExcludeSemantics(child: child);
  }

  /// Announces [message] to assistive technologies via
  /// [SemanticsService.announce]. Useful for transient state changes
  /// (e.g. "Stream connected", "Recording started").
  static void announce(String message) {
    SemanticsService.announce(message, TextDirection.ltr);
  }
}
