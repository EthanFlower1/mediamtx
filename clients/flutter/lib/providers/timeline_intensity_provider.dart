import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'auth_provider.dart';

class IntensityBucket {
  final DateTime bucketStart;
  final int count;

  const IntensityBucket({required this.bucketStart, required this.count});

  factory IntensityBucket.fromJson(Map<String, dynamic> json) {
    return IntensityBucket(
      bucketStart: DateTime.parse(json['bucket_start'] as String).toLocal(),
      count: json['count'] as int,
    );
  }
}

typedef IntensityKey = ({String cameraId, String date, int bucketSeconds});

final intensityProvider =
    FutureProvider.family<List<IntensityBucket>, IntensityKey>(
  (ref, params) async {
    final api = ref.watch(apiClientProvider);
    if (api == null) return [];

    final res = await api.get<dynamic>(
      '/api/nvr/timeline/intensity',
      queryParameters: {
        'camera_id': params.cameraId,
        'date': params.date,
        'bucket_seconds': params.bucketSeconds.toString(),
      },
    );

    final data = res.data;
    if (data == null) return [];
    final list = data is List ? data : (data['buckets'] as List? ?? []);
    return list
        .map((e) => IntensityBucket.fromJson(e as Map<String, dynamic>))
        .toList();
  },
);
