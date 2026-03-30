import 'package:dio/dio.dart';
import 'package:flutter/foundation.dart';

/// A timespan returned by the MediaMTX /list endpoint.
class PlaybackTimespan {
  final DateTime start;
  final double durationSecs;
  final String url;

  DateTime get end => start.add(Duration(
      microseconds: (durationSecs * Duration.microsecondsPerSecond).round()));

  const PlaybackTimespan({
    required this.start,
    required this.durationSecs,
    required this.url,
  });

  factory PlaybackTimespan.fromJson(Map<String, dynamic> json) {
    return PlaybackTimespan(
      start: DateTime.parse(json['start'] as String),
      durationSecs: (json['duration'] as num).toDouble(),
      url: json['url'] as String,
    );
  }

  /// Whether [time] falls within this timespan.
  bool contains(DateTime time) =>
      !time.isBefore(start) && time.isBefore(end);
}

class PlaybackService {
  final String serverUrl;
  PlaybackService({required this.serverUrl});

  /// Fetch the list of recorded timespans from the MediaMTX /list endpoint.
  /// Each timespan includes a pre-built /get URL.
  Future<List<PlaybackTimespan>> listTimespans({
    required String cameraPath,
    required DateTime start,
    required DateTime end,
  }) async {
    final uri = Uri.parse(serverUrl);
    final base = '${uri.scheme}://${uri.host}:9996/list';
    final url = Uri.parse(base).replace(queryParameters: {
      'path': cameraPath,
      'start': _toRfc3339(start),
      'end': _toRfc3339(end),
    }).toString();

    final dio = Dio();
    try {
      final response = await dio.get<List<dynamic>>(url);
      if (response.statusCode == 200 && response.data != null) {
        return response.data!
            .map((e) =>
                PlaybackTimespan.fromJson(e as Map<String, dynamic>))
            .toList();
      }
      return [];
    } catch (e) {
      debugPrint('[PlaybackService] listTimespans failed: $e');
      return [];
    } finally {
      dio.close();
    }
  }

  /// Builds a /get URL for playback starting at [start] for [durationSecs].
  String getUrl({
    required String cameraPath,
    required DateTime start,
    required int durationSecs,
    String? token,
  }) {
    final uri = Uri.parse(serverUrl);
    final base = '${uri.scheme}://${uri.host}:9996/get';
    final params = <String, String>{
      'path': cameraPath,
      'start': _toRfc3339(start),
      'duration': durationSecs.toString(),
      'format': 'mp4',
    };
    if (token != null && token.isNotEmpty) {
      params['jwt'] = token;
    }
    return Uri.parse(base).replace(queryParameters: params).toString();
  }

  /// Builds the HLS playlist URL (kept for backwards compatibility).
  String playlistUrl({
    required String cameraId,
    required String date,
    String? token,
    double? startSeconds,
  }) {
    final uri = Uri.parse(serverUrl);
    final params = <String, String>{'date': date};
    if (token != null && token.isNotEmpty) {
      params['token'] = token;
    }
    if (startSeconds != null && startSeconds > 0) {
      params['start'] = startSeconds.toStringAsFixed(1);
    }
    return Uri(
      scheme: uri.scheme,
      host: uri.host,
      port: uri.port,
      path: '/api/nvr/vod/$cameraId/playlist.m3u8',
      queryParameters: params,
    ).toString();
  }

  /// Builds a direct VoD URL (legacy — prefer [getUrl]).
  String clipUrl({
    required String cameraPath,
    required DateTime start,
    int durationSecs = 30,
    String? token,
  }) {
    return getUrl(
      cameraPath: cameraPath,
      start: start,
      durationSecs: durationSecs,
      token: token,
    );
  }

  static String _toRfc3339(DateTime d) {
    final offset = d.timeZoneOffset;
    final sign = offset.isNegative ? '-' : '+';
    final abs = offset.abs();
    final h = abs.inHours.toString().padLeft(2, '0');
    final m = (abs.inMinutes % 60).toString().padLeft(2, '0');
    return '${d.year}-${_p(d.month)}-${_p(d.day)}'
        'T${_p(d.hour)}:${_p(d.minute)}:${_p(d.second)}$sign$h:$m';
  }

  static String _p(int n) => n.toString().padLeft(2, '0');
}
