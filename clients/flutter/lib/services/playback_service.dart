class PlaybackService {
  final String serverUrl;
  PlaybackService({required this.serverUrl});

  String playbackUrl(String path, DateTime start, {double durationSecs = 86400}) {
    final uri = Uri.parse(serverUrl);
    final startIso = toLocalRfc3339(start);
    return '${uri.scheme}://${uri.host}:9996/get?path=${Uri.encodeComponent(path)}&start=${Uri.encodeComponent(startIso)}&duration=$durationSecs';
  }

  String playbackWsUrl() {
    final uri = Uri.parse(serverUrl);
    return 'ws://${uri.host}:${uri.port}/api/nvr/playback/ws';
  }

  String streamBaseUrl() {
    final uri = Uri.parse(serverUrl);
    return '${uri.scheme}://${uri.host}:${uri.port}';
  }

  static String toLocalRfc3339(DateTime d) {
    final offset = d.timeZoneOffset;
    final sign = offset.isNegative ? '-' : '+';
    final abs = offset.abs();
    final h = abs.inHours.toString().padLeft(2, '0');
    final m = (abs.inMinutes % 60).toString().padLeft(2, '0');
    return '${d.year}-${_p(d.month)}-${_p(d.day)}T${_p(d.hour)}:${_p(d.minute)}:${_p(d.second)}$sign$h:$m';
  }

  static String _p(int n) => n.toString().padLeft(2, '0');
}
