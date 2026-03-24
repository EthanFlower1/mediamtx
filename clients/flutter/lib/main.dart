import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

void main() {
  WidgetsFlutterBinding.ensureInitialized();
  runApp(
    const ProviderScope(
      child: MaterialApp(
        title: 'MediaMTX NVR',
        home: Scaffold(
          body: Center(child: Text('MediaMTX NVR — Foundation')),
        ),
      ),
    ),
  );
}
