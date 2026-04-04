import 'package:flutter/material.dart';
import '../theme/nvr_colors.dart';

void showErrorSnackBar(BuildContext context, String message) {
  ScaffoldMessenger.of(context).showSnackBar(
    SnackBar(
      backgroundColor: NvrColors.of(context).bgSecondary,
      content: Text(
        message,
        style: TextStyle(color: NvrColors.of(context).danger, fontSize: 13),
      ),
      behavior: SnackBarBehavior.floating,
      duration: const Duration(seconds: 4),
    ),
  );
}
