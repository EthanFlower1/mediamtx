import 'package:flutter_riverpod/flutter_riverpod.dart';
import '../models/recording.dart';
import 'auth_provider.dart';

typedef RecordingsKey = ({String cameraId, String date});

final recordingSegmentsProvider =
    FutureProvider.family<List<RecordingSegment>, RecordingsKey>((ref, key) async {
  final api = ref.watch(apiClientProvider);
  if (api == null) return [];

  // Build start/end in local timezone so the query matches the selected
  // calendar day, not UTC day (which can be off by the timezone offset).
  final offset = DateTime.now().timeZoneOffset;
  final sign = offset.isNegative ? '-' : '+';
  final h = offset.abs().inHours.toString().padLeft(2, '0');
  final m = (offset.abs().inMinutes % 60).toString().padLeft(2, '0');
  final tz = '$sign$h:$m';
  final start = '${key.date}T00:00:00$tz';
  final end = '${key.date}T23:59:59$tz';

  final res = await api.get<dynamic>('/recordings', queryParameters: {
    'camera_id': key.cameraId,
    'start': start,
    'end': end,
  });

  final data = res.data;
  if (data == null) return [];
  final list = data is List ? data : (data['segments'] as List? ?? []);
  return list
      .map((e) => RecordingSegment.fromJson(e as Map<String, dynamic>))
      .toList();
});

final motionEventsProvider =
    FutureProvider.family<List<MotionEvent>, RecordingsKey>((ref, key) async {
  final api = ref.watch(apiClientProvider);
  if (api == null) return [];

  final res = await api.get<dynamic>(
    '/cameras/${key.cameraId}/motion-events',
    queryParameters: {'date': key.date},
  );

  final data = res.data;
  if (data == null) return [];
  final list = data is List ? data : (data['events'] as List? ?? []);
  return list
      .map((e) => MotionEvent.fromJson(e as Map<String, dynamic>))
      .toList();
});
