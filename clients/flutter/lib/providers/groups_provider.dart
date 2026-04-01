import 'package:flutter_riverpod/flutter_riverpod.dart';
import '../models/camera_group.dart';
import 'auth_provider.dart';

final groupsProvider = FutureProvider<List<CameraGroup>>((ref) async {
  final api = ref.watch(apiClientProvider);
  if (api == null) return [];
  final res = await api.get('/camera-groups');
  return (res.data as List).map((e) => CameraGroup.fromJson(e as Map<String, dynamic>)).toList();
});
