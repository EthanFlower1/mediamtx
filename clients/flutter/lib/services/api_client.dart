import 'package:dio/dio.dart';
import 'auth_service.dart';

class ApiClient {
  final Dio dio;
  final AuthService _authService;
  final String serverUrl;

  ApiClient({required this.serverUrl, required AuthService authService})
      : _authService = authService,
        dio = Dio(BaseOptions(
          baseUrl: '$serverUrl/api/nvr',
          connectTimeout: const Duration(seconds: 10),
          receiveTimeout: const Duration(seconds: 30),
          headers: {'Content-Type': 'application/json'},
        )) {
    dio.interceptors.add(_AuthInterceptor(
      authService: _authService,
      serverUrl: serverUrl,
      dio: dio,
    ));
  }

  Future<Response<T>> get<T>(String path, {
    Map<String, dynamic>? queryParameters,
    Duration? receiveTimeout,
  }) =>
      dio.get<T>(path,
        queryParameters: queryParameters,
        options: receiveTimeout != null ? Options(receiveTimeout: receiveTimeout) : null,
      );

  Future<Response<T>> post<T>(String path, {dynamic data}) =>
      dio.post<T>(path, data: data);

  Future<Response<T>> put<T>(String path, {dynamic data}) =>
      dio.put<T>(path, data: data);

  Future<Response<T>> delete<T>(String path) =>
      dio.delete<T>(path);
}

class _AuthInterceptor extends Interceptor {
  final AuthService authService;
  final String serverUrl;
  final Dio dio;
  bool _isRefreshing = false;

  _AuthInterceptor({required this.authService, required this.serverUrl, required this.dio});

  @override
  Future<void> onRequest(RequestOptions options, RequestInterceptorHandler handler) async {
    final token = await authService.getAccessToken();
    if (token != null) {
      options.headers['Authorization'] = 'Bearer $token';
    }
    handler.next(options);
  }

  @override
  Future<void> onError(DioException err, ErrorInterceptorHandler handler) async {
    if (err.response?.statusCode == 401 && !_isRefreshing) {
      _isRefreshing = true;
      try {
        final result = await authService.refresh(serverUrl);
        _isRefreshing = false;
        if (result != null) {
          final opts = err.requestOptions;
          opts.headers['Authorization'] = 'Bearer ${result.accessToken}';
          final response = await dio.fetch(opts);
          handler.resolve(response);
          return;
        }
      } catch (_) {
        _isRefreshing = false;
      }
    }
    handler.next(err);
  }
}
