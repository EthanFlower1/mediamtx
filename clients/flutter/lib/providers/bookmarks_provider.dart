import 'package:flutter_riverpod/flutter_riverpod.dart';
import '../models/bookmark.dart';
import 'auth_provider.dart';

typedef BookmarksKey = ({String cameraId, String date});

final bookmarksProvider =
    FutureProvider.family<List<Bookmark>, BookmarksKey>((ref, key) async {
  final api = ref.watch(apiClientProvider);
  if (api == null) return [];

  final res = await api.get<dynamic>(
    '/api/nvr/bookmarks',
    queryParameters: {
      'camera_id': key.cameraId,
      'date': key.date,
    },
  );

  final data = res.data;
  if (data == null) return [];
  final list = data is List ? data : (data['bookmarks'] as List? ?? []);
  return list
      .map((e) => Bookmark.fromJson(e as Map<String, dynamic>))
      .toList();
});
