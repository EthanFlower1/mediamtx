import 'package:flutter/material.dart';

/// Helpers for managing keyboard / switch-access focus traversal order.
///
/// Ensures that focus moves through UI elements in a logical, predictable
/// sequence as required by WCAG 2.1 SC 2.4.3 (Focus Order).
class FocusOrderHelper {
  FocusOrderHelper._(); // prevent instantiation

  /// Creates [count] [FocusNode]s that can be assigned to widgets in the
  /// desired traversal order and later disposed by the caller.
  static List<FocusNode> createOrderedNodes(int count) {
    return List<FocusNode>.generate(
      count,
      (index) => FocusNode(debugLabel: 'a11y_ordered_$index'),
    );
  }

  /// Wraps [child] in a [FocusTraversalOrder] with the given numeric
  /// [order], allowing explicit control of the tab / d-pad sequence.
  ///
  /// Must be placed inside a [FocusTraversalGroup] configured with an
  /// [OrderedTraversalPolicy] for the ordering to take effect.
  static Widget withOrder(Widget child, double order) {
    return FocusTraversalOrder(
      order: NumericFocusOrder(order),
      child: child,
    );
  }
}
