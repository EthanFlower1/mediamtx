import 'package:flutter_riverpod/flutter_riverpod.dart';
import '../models/camera.dart';
import 'auth_provider.dart';

final camerasProvider = FutureProvider<List<Camera>>((ref) async {
  final api = ref.watch(apiClientProvider);
  if (api == null) return [];
  final res = await api.get('/cameras');
  return (res.data as List).map((e) => Camera.fromJson(e as Map<String, dynamic>)).toList();
});
