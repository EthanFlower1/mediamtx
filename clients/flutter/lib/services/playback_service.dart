class PlaybackService {
  final String serverUrl;
  PlaybackService({required this.serverUrl});

  /// Builds the HLS playlist URL for a camera on a given date.
  String playlistUrl({
    required String cameraId,
    required String date,
    String? token,
  }) {
    final uri = Uri.parse(serverUrl);
    final params = <String, String>{'date': date};
    if (token != null && token.isNotEmpty) {
      params['token'] = token;
    }
    return Uri(
      scheme: uri.scheme,
      host: uri.host,
      port: uri.port,
      path: '/api/nvr/vod/$cameraId/playlist.m3u8',
      queryParameters: params,
    ).toString();
  }

  /// Builds a direct VoD URL for a short clip (uses MediaMTX /get endpoint).
  String clipUrl({
    required String cameraPath,
    required DateTime start,
    int durationSecs = 30,
    String? token,
  }) {
    final uri = Uri.parse(serverUrl);
    final startStr = _toRfc3339(start);
    final base = '${uri.scheme}://${uri.host}:9996/get';
    final params = <String, String>{
      'path': cameraPath,
      'start': startStr,
      'duration': durationSecs.toString(),
    };
    if (token != null && token.isNotEmpty) {
      params['jwt'] = token;
    }
    return Uri.parse(base).replace(queryParameters: params).toString();
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
