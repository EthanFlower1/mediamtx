import 'package:flutter/material.dart';
import '../theme/nvr_colors.dart';

class CameraStatusBadge extends StatelessWidget {
  final String status;

  const CameraStatusBadge({super.key, required this.status});

  bool get _isOnline => status == 'connected' || status == 'online';

  Color get _color => _isOnline ? NvrColors.success : NvrColors.danger;

  String get _label => _isOnline ? 'Online' : 'Offline';

  @override
  Widget build(BuildContext context) {
    return Row(
      mainAxisSize: MainAxisSize.min,
      children: [
        Container(
          width: 8,
          height: 8,
          decoration: BoxDecoration(
            color: _color,
            shape: BoxShape.circle,
          ),
        ),
        const SizedBox(width: 5),
        Text(
          _label,
          style: TextStyle(
            color: _color,
            fontSize: 12,
            fontWeight: FontWeight.w500,
          ),
        ),
      ],
    );
  }
}
