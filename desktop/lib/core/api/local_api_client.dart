import 'dart:convert';
import 'dart:io';

class LocalApiClient {
  LocalApiClient({required Uri baseUri, required String token})
    : this._(baseUri, token);

  LocalApiClient._(this._baseUri, this._token);

  final Uri _baseUri;
  final String _token;

  Future<Map<String, dynamic>> get(String path) => _request('GET', path);

  Future<Map<String, dynamic>> post(
    String path, [
    Map<String, dynamic>? body,
  ]) => _request('POST', path, body);

  Future<Map<String, dynamic>> put(String path, Map<String, dynamic> body) =>
      _request('PUT', path, body);

  Future<Map<String, dynamic>> delete(String path) => _request('DELETE', path);

  Future<void> downloadTo(String path, String destinationPath) async {
    final client = HttpClient();
    try {
      final request = await client.getUrl(_baseUri.resolve(path));
      _authorize(request);
      final response = await request.close();
      if (response.statusCode < 200 || response.statusCode >= 300) {
        final body = await response.transform(utf8.decoder).join();
        throw HttpException(
          '本地 API 返回 ${response.statusCode}: $body',
          uri: _baseUri,
        );
      }
      await response.pipe(File(destinationPath).openWrite());
    } finally {
      client.close(force: true);
    }
  }

  Future<ApiEventConnection> openEventStream() async {
    final client = HttpClient();
    try {
      final request = await client.getUrl(_baseUri.resolve('/api/v1/events'));
      _authorize(request);
      request.headers.set(HttpHeaders.acceptHeader, 'text/event-stream');
      final response = await request.close();
      if (response.statusCode != HttpStatus.ok) {
        final body = await response.transform(utf8.decoder).join();
        throw HttpException('事件推送返回 ${response.statusCode}: $body');
      }
      return ApiEventConnection._(client, response);
    } catch (_) {
      client.close(force: true);
      rethrow;
    }
  }

  Future<Map<String, dynamic>> _request(
    String method,
    String path, [
    Map<String, dynamic>? body,
  ]) async {
    final client = HttpClient();
    try {
      final request = await client.openUrl(method, _baseUri.resolve(path));
      _authorize(request);
      if (body != null) {
        request.headers.contentType = ContentType.json;
        request.write(jsonEncode(body));
      }
      final response = await request.close();
      final text = await response.transform(utf8.decoder).join();
      final decoded = _decodeObject(text);
      if (response.statusCode < 200 || response.statusCode >= 300) {
        final message = decoded?['message'] ?? decoded?['error'];
        throw HttpException(
          message is String && message.isNotEmpty
              ? message
              : '本地 API 返回 ${response.statusCode}',
          uri: _baseUri,
        );
      }
      if (decoded == null) {
        throw const FormatException('本地 API 返回了无效的 JSON 对象');
      }
      return decoded;
    } finally {
      client.close(force: true);
    }
  }

  Map<String, dynamic>? _decodeObject(String text) {
    if (text.isEmpty) return <String, dynamic>{};
    try {
      final decoded = jsonDecode(text);
      return decoded is Map ? decoded.cast<String, dynamic>() : null;
    } on FormatException {
      return null;
    }
  }

  void _authorize(HttpClientRequest request) {
    request.headers.set(HttpHeaders.authorizationHeader, 'Bearer $_token');
  }
}

class ApiEventConnection {
  ApiEventConnection._(this._client, this._response);

  final HttpClient _client;
  final HttpClientResponse _response;

  Stream<String> get lines =>
      _response.transform(utf8.decoder).transform(const LineSplitter());

  void close() => _client.close(force: true);
}
