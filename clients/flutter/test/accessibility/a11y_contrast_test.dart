import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

import 'package:mediamtx/accessibility/a11y_contrast.dart';

void main() {
  group('ContrastChecker', () {
    // --- contrastRatio ---

    test('white on black returns ~21:1', () {
      final ratio = ContrastChecker.contrastRatio(Colors.white, Colors.black);
      expect(ratio, closeTo(21.0, 0.1));
    });

    test('black on black returns 1:1', () {
      final ratio = ContrastChecker.contrastRatio(Colors.black, Colors.black);
      expect(ratio, closeTo(1.0, 0.01));
    });

    test('same colour returns 1:1', () {
      const c = Color(0xFF336699);
      final ratio = ContrastChecker.contrastRatio(c, c);
      expect(ratio, closeTo(1.0, 0.01));
    });

    test('ratio is symmetric (order-independent)', () {
      final ab = ContrastChecker.contrastRatio(Colors.blue, Colors.white);
      final ba = ContrastChecker.contrastRatio(Colors.white, Colors.blue);
      expect(ab, closeTo(ba, 0.001));
    });

    // --- meetsAA ---

    test('white on black passes AA for normal text', () {
      expect(
        ContrastChecker.meetsAA(Colors.white, Colors.black),
        isTrue,
      );
    });

    test('black on black fails AA', () {
      expect(
        ContrastChecker.meetsAA(Colors.black, Colors.black),
        isFalse,
      );
    });

    test('gray on white edge case — below 4.5:1 for normal text', () {
      // Colors.grey is #9E9E9E, ratio against white is ~3.9:1
      expect(
        ContrastChecker.meetsAA(Colors.grey, Colors.white),
        isFalse,
      );
    });

    test('gray on white passes AA for large text (>= 3:1)', () {
      expect(
        ContrastChecker.meetsAA(Colors.grey, Colors.white, largeText: true),
        isTrue,
      );
    });

    // --- meetsAAA ---

    test('white on black passes AAA', () {
      expect(
        ContrastChecker.meetsAAA(Colors.white, Colors.black),
        isTrue,
      );
    });

    test('gray on white fails AAA for normal text', () {
      expect(
        ContrastChecker.meetsAAA(Colors.grey, Colors.white),
        isFalse,
      );
    });

    test('dark gray on white passes AA but not AAA for normal text', () {
      // #595959 on white gives ~7.0:1 — right at the boundary
      const darkGray = Color(0xFF595959);
      final ratio = ContrastChecker.contrastRatio(darkGray, Colors.white);
      // Should pass AA (>= 4.5)
      expect(ratio >= 4.5, isTrue);
      // Exact AAA depends on rounding; verify the ratio is near 7.
      expect(ratio, closeTo(7.0, 0.5));
    });

    test('white on black passes AAA for large text', () {
      expect(
        ContrastChecker.meetsAAA(Colors.white, Colors.black, largeText: true),
        isTrue,
      );
    });
  });
}
