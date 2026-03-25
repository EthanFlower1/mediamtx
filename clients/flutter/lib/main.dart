import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:fvp/fvp.dart';
import 'app.dart';

void main() {
  WidgetsFlutterBinding.ensureInitialized();
  registerWith(); // fvp: use libmdk as video_player backend
  runApp(const ProviderScope(child: NvrApp()));
}
