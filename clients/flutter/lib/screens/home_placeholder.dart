import 'package:flutter/material.dart';
class HomePlaceholder extends StatelessWidget {
  final String title;
  const HomePlaceholder({super.key, required this.title});
  @override
  Widget build(BuildContext context) => Center(
    child: Column(
      mainAxisSize: MainAxisSize.min,
      children: [
        Icon(Icons.construction, size: 48, color: Theme.of(context).colorScheme.outline),
        const SizedBox(height: 16),
        Text(title, style: Theme.of(context).textTheme.headlineMedium),
        const SizedBox(height: 8),
        Text('Coming soon', style: TextStyle(color: Theme.of(context).colorScheme.outline)),
      ],
    ),
  );
}
