import 'package:dio/dio.dart';
import 'package:flutter/foundation.dart';

import '../models/detection_frame.dart';

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

  /// Fetch historical detections for a camera within a time range.
  /// Used by the playback overlay to batch-load detections for a segment.
  Future<List<PlaybackDetection>> fetchDetections({
    required String cameraId,
    required DateTime start,
    required DateTime end,
    String? token,
  }) async {
    final uri = Uri.parse(serverUrl);
    final params = <String, String>{
      'start': start.toUtc().toIso8601String(),
      'end': end.toUtc().toIso8601String(),
    };

    final url = Uri(
      scheme: uri.scheme,
      host: uri.host,
      port: uri.port,
      path: '/api/nvr/cameras/$cameraId/detections',
      queryParameters: params,
    ).toString();

    final dio = Dio();
    try {
      final options = Options();
      if (token != null && token.isNotEmpty) {
        options.headers = {'Authorization': 'Bearer $token'};
      }
      final response = await dio.get<List<dynamic>>(url, options: options);
      if (response.statusCode == 200 && response.data != null) {
        return response.data!
            .map((e) =>
                PlaybackDetection.fromJson(e as Map<String, dynamic>))
            .toList();
      }
      return [];
    } catch (e) {
      debugPrint('[PlaybackService] fetchDetections failed: $e');
      return [];
    } finally {
      dio.close();
    }
  }

  /// Fetch aligned timeline data for multiple cameras on a given date.
  /// Returns a map of camera ID to list of time ranges (start/end pairs).
  /// Uses the GET /api/nvr/timeline/multi endpoint.
  Future<Map<String, List<({DateTime start, DateTime end})>>> fetchMultiTimeline({
    required List<String> cameraIds,
    required String date,
    String? token,
  }) async {
    final uri = Uri.parse(serverUrl);
    final url = Uri(
      scheme: uri.scheme,
      host: uri.host,
      port: uri.port,
      path: '/api/nvr/timeline/multi',
      queryParameters: {
        'cameras': cameraIds.join(','),
        'date': date,
      },
    ).toString();

    final dio = Dio();
    try {
      final options = Options();
      if (token != null && token.isNotEmpty) {
        options.headers = {'Authorization': 'Bearer $token'};
      }
      final response = await dio.get<dynamic>(url, options: options);
      if (response.statusCode == 200 && response.data != null) {
        final data = response.data as Map<String, dynamic>;
        final cameras = data['cameras'] as List<dynamic>? ?? [];
        final result = <String, List<({DateTime start, DateTime end})>>{};
        for (final entry in cameras) {
          final map = entry as Map<String, dynamic>;
          final camId = map['camera_id'] as String;
          final segments = (map['segments'] as List<dynamic>? ?? [])
              .map((s) {
                final seg = s as Map<String, dynamic>;
                return (
                  start: DateTime.parse(seg['start'] as String),
                  end: DateTime.parse(seg['end'] as String),
                );
              })
              .toList();
          result[camId] = segments;
        }
        return result;
      }
      return {};
    } catch (e) {
      debugPrint('[PlaybackService] fetchMultiTimeline failed: $e');
      return {};
    } finally {
      dio.close();
    }
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
