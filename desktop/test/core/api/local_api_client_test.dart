import 'dart:convert';
import 'dart:io';

import 'package:agentpulse/core/api/local_api_client.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  late HttpServer server;
  late LocalApiClient api;
  final observed = <String, Object?>{};

  setUp(() async {
    observed.clear();
    server = await HttpServer.bind(InternetAddress.loopbackIPv4, 0);
    server.listen((request) async {
      observed['authorization'] = request.headers.value(
        HttpHeaders.authorizationHeader,
      );
      observed['method'] = request.method;
      observed['path'] = request.uri.path;
      observed['body'] = await utf8.decoder.bind(request).join();
      if (request.uri.path == '/api/v1/events') {
        request.response.headers.set(
          HttpHeaders.contentTypeHeader,
          'text/event-stream',
        );
        request.response.write('event: telemetry\n\n');
        await request.response.close();
        return;
      }
      if (request.uri.path == '/download') {
        request.response.headers.contentType = ContentType.binary;
        request.response.add([0x41, 0x50, 0x01]);
        await request.response.close();
        return;
      }
      request.response.headers.contentType = ContentType.json;
      if (request.uri.path == '/denied') {
        request.response.statusCode = HttpStatus.forbidden;
        request.response.write(jsonEncode({'message': 'local denial'}));
      } else {
        request.response.write(
          jsonEncode({'ok': true, 'method': request.method}),
        );
      }
      await request.response.close();
    });
    api = LocalApiClient(
      baseUri: Uri(
        scheme: 'http',
        host: InternetAddress.loopbackIPv4.address,
        port: server.port,
      ),
      token: 'test-session-token',
    );
  });

  tearDown(() async {
    await server.close(force: true);
  });

  test(
    'uses bearer auth and preserves JSON methods and request bodies',
    () async {
      expect((await api.get('/read'))['ok'], isTrue);
      expect(observed['authorization'], 'Bearer test-session-token');
      expect(observed['method'], 'GET');

      await api.post('/create', {'name': 'session'});
      expect(observed['method'], 'POST');
      expect(jsonDecode(observed['body']! as String), {'name': 'session'});

      await api.put('/update', {'enabled': true});
      expect(observed['method'], 'PUT');
      expect(jsonDecode(observed['body']! as String), {'enabled': true});

      await api.delete('/remove');
      expect(observed['method'], 'DELETE');
    },
  );

  test('exposes a safe server message for non-success responses', () async {
    await expectLater(
      api.get('/denied'),
      throwsA(
        isA<HttpException>().having(
          (error) => error.message,
          'message',
          'local denial',
        ),
      ),
    );
  });

  test(
    'opens an authenticated event stream and returns complete lines',
    () async {
      final connection = await api.openEventStream();
      addTearDown(connection.close);

      expect(await connection.lines.toList(), ['event: telemetry', '']);
      expect(observed['authorization'], 'Bearer test-session-token');
    },
  );

  test('downloads a response to the requested file', () async {
    final directory = await Directory.systemTemp.createTemp(
      'agentpulse-api-client-test-',
    );
    addTearDown(() => directory.delete(recursive: true));
    final destination = '${directory.path}${Platform.pathSeparator}export.db';

    await api.downloadTo('/download', destination);

    expect(await File(destination).readAsBytes(), [0x41, 0x50, 0x01]);
    expect(observed['authorization'], 'Bearer test-session-token');
  });
}
