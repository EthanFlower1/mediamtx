import 'package:flutter_riverpod/flutter_riverpod.dart';
import '../models/schedule_template.dart';
import 'auth_provider.dart';

final scheduleTemplatesProvider =
    FutureProvider<List<ScheduleTemplate>>((ref) async {
  final api = ref.watch(apiClientProvider);
  if (api == null) return [];

  final res = await api.get<dynamic>('/schedule-templates');
  final data = res.data as List<dynamic>? ?? [];
  return data
      .map((e) => ScheduleTemplate.fromJson(e as Map<String, dynamic>))
      .toList();
});
