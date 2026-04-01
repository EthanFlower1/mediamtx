import 'package:flutter/material.dart';
import '../theme/nvr_colors.dart';

void showErrorSnackBar(BuildContext context, String message) {
  ScaffoldMessenger.of(context).showSnackBar(
    SnackBar(
      backgroundColor: NvrColors.bgSecondary,
      content: Text(
        message,
        style: const TextStyle(color: NvrColors.danger, fontSize: 13),
      ),
      behavior: SnackBarBehavior.floating,
      duration: const Duration(seconds: 4),
    ),
  );
}
