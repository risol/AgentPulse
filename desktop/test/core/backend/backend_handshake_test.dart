import 'dart:async';
import 'dart:convert';

import 'package:agentpulse/core/backend/backend_session.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  test('accepts a versioned loopback handshake with a dynamic port', () {
    final handshake = BackendHandshake.parse(
      jsonEncode({
        'api_address': '127.0.0.1:43217',
        'api_token': 'session-secret',
        'backend_version': '1.0.0',
      }),
    );

    expect(handshake.apiUri, Uri.parse('http://127.0.0.1:43217'));
    expect(handshake.apiToken, 'session-secret');
    expect(handshake.version, '1.0.0');
  });

  test('rejects a non-loopback or incomplete API address', () {
    expect(
      () => BackendHandshake.parse(
        jsonEncode({
          'api_address': '192.168.1.10:43217',
          'api_token': 'session-secret',
        }),
      ),
      throwsFormatException,
    );
    expect(
      () => BackendHandshake.parse(
        jsonEncode({'api_address': '127.0.0.1:43217'}),
      ),
      throwsFormatException,
    );
  });

  test('turns a receiver port conflict into an actionable startup message', () {
    final error = BackendStartupException.fromFailure(
      StateError('backend exited before handshake'),
      stderr: 'listen tcp 127.0.0.1:4318: bind: Only one usage of each socket address',
      exitCode: 1,
    );

    expect(error.message, contains('4318 已被占用'));
    expect(error.message, contains('关闭现有接收器后重试'));
  });

  test('distinguishes startup timeout from an invalid handshake', () {
    expect(
      BackendStartupException.fromFailure(
        TimeoutException('startup timed out'),
        stderr: '',
      ).message,
      contains('就绪超时'),
    );
    expect(
      BackendStartupException.fromFailure(
        const FormatException('invalid JSON'),
        stderr: '',
      ).message,
      contains('无效的启动握手'),
    );
  });
}
