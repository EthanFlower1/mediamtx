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
