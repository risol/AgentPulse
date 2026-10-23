import 'dart:async';
import 'dart:convert';
import 'dart:io';

import 'package:agentpulse/core/api/local_api_client.dart';
import 'package:agentpulse/features/settings/settings_controller.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  late HttpServer server;
  late LocalApiClient api;
  late SettingsController controller;
  final requests = <String>[];
  int retentionDays = 30;
  int receiverPort = 4318;
  Completer<void>? retentionGate;
  Completer<void>? retentionRequestStarted;

  setUp(() async {
    requests.clear();
    retentionDays = 30;
    receiverPort = 4318;
    retentionGate = null;
    retentionRequestStarted = null;
    server = await HttpServer.bind(InternetAddress.loopbackIPv4, 0);
    server.listen((request) async {
      requests.add('${request.method} ${request.uri.path}');
      if (request.headers.value(HttpHeaders.authorizationHeader) !=
          'Bearer settings-test-token') {
        request.response.statusCode = HttpStatus.unauthorized;
        await request.response.close();
        return;
      }

      final path = request.uri.path;
      if (path == '/api/v1/data/export') {
        request.response.write('sqlite-snapshot');
        await request.response.close();
        return;
      }

      Object response;
      if (path == '/api/v1/config/codex' && request.method == 'GET') {
        response = {
          'path': 'config.toml',
          'backups': ['config.toml.backup'],
        };
      } else if (path == '/api/v1/config/codex/preview') {
        response = {
          'path': 'config.toml',
          'changes': {'OTEL_EXPORTER_OTLP_ENDPOINT': 'http://127.0.0.1:4318'},
        };
      } else if (path == '/api/v1/config/codex/apply') {
        response = {
          'config': {
            'path': 'config.toml',
            'backups': ['config.toml.backup'],
          },
          'backup': 'config.toml.backup',
        };
      } else if (path == '/api/v1/config/codex/restore') {
        response = {
          'config': {
            'path': 'config.toml',
            'backups': ['config.toml.backup'],
          },
          'recovery_backup': 'config.toml.before-restore',
        };
      } else if (path == '/api/v1/settings/data-retention' &&
          request.method == 'GET') {
        if (retentionGate != null) {
          retentionRequestStarted?.complete();
          await retentionGate!.future;
        }
        response = {
          'retention_days': retentionDays,
          'database_size_bytes': 1024,
        };
      } else if (path == '/api/v1/settings/data-retention' &&
          request.method == 'PUT') {
        final body = jsonDecode(await utf8.decoder.bind(request).join()) as Map;
        retentionDays = (body['retention_days'] as num).toInt();
        response = {
          'retention_days': retentionDays,
          'database_size_bytes': 1024,
        };
      } else if (path == '/api/v1/settings/receiver' &&
          request.method == 'GET') {
        response = {'port': receiverPort};
      } else if (path == '/api/v1/settings/receiver' &&
          request.method == 'PUT') {
        final body = jsonDecode(await utf8.decoder.bind(request).join()) as Map;
        receiverPort = (body['port'] as num).toInt();
        response = {
          'settings': {'port': receiverPort},
          'restart_required': receiverPort != 4318,
        };
      } else if (path == '/api/v1/settings/data-retention/cleanup') {
        response = {'accepted': true};
      } else if (path == '/api/v1/data/history') {
        response = {'deleted_rows': 12};
      } else if (path == '/api/v1/status') {
        response = {
          'receiver': 'ready',
          'receiver_address': '127.0.0.1:$receiverPort',
          'schema_version': 10,
          'signals': {
            'logs': {'state': 'receiving'},
          },
          'ingest_queue': {'depth': 0, 'capacity': 100},
          'ingest_diagnostics': {'count_last_24_hours': 0, 'recent': []},
          'prompt': 'must not be exported',
        };
      } else if (path == '/api/v1/settings/logs/cleanup') {
        response = {'deleted_bytes': 2048};
      } else {
        request.response.statusCode = HttpStatus.notFound;
        response = {'message': 'unexpected route: $path'};
      }
      request.response.headers.contentType = ContentType.json;
      request.response.write(jsonEncode(response));
      await request.response.close();
    });
    api = LocalApiClient(
      baseUri: Uri(
        scheme: 'http',
        host: InternetAddress.loopbackIPv4.address,
        port: server.port,
      ),
      token: 'settings-test-token',
    );
    controller = SettingsController()
      ..setApiClient(api, backendVersion: 'test-build');
  });

  tearDown(() async {
    controller.dispose();
    await server.close(force: true);
  });

  test(
    'owns configuration, retention, export, and cleanup API workflows',
    () async {
      await controller.loadConfig();
      expect(controller.config['path'], 'config.toml');
      expect(controller.selectedBackup, 'config.toml.backup');

      await controller.loadPreview();
      expect(
        controller.preview['changes'],
        contains('OTEL_EXPORTER_OTLP_ENDPOINT'),
      );
      expect(await controller.applyConfig(), contains('配置已应用'));
      expect(controller.preview, isEmpty);
      expect(
        await controller.restoreConfig('config.toml.backup'),
        contains('配置已恢复'),
      );

      await controller.loadRetention();
      expect(controller.retentionDays, 30);
      await controller.loadReceiverSettings();
      expect(controller.receiverPort, 4318);
      controller.setReceiverPort(4545);
      expect(await controller.saveReceiverSettings(), isTrue);
      expect(receiverPort, 4545);
      controller.setRetentionDays(60);
      expect(await controller.saveRetention(), isTrue);
      expect(controller.retentionDays, 60);
      expect(await controller.requestCleanup(), isTrue);
      expect(await controller.clearHistory(), 12);

      final temporaryDirectory = await Directory.systemTemp.createTemp(
        'agentpulse-settings-controller-',
      );
      addTearDown(() => temporaryDirectory.delete(recursive: true));
      final databasePath =
          '${temporaryDirectory.path}${Platform.pathSeparator}data.db';
      final diagnosticsPath =
          '${temporaryDirectory.path}${Platform.pathSeparator}diagnostics.json';
      await controller.exportDataTo(databasePath);
      expect(await File(databasePath).readAsString(), 'sqlite-snapshot');

      await controller.exportDiagnosticsTo(diagnosticsPath);
      final diagnostics =
          jsonDecode(await File(diagnosticsPath).readAsString()) as Map;
      expect(diagnostics['application']['version'], 'test-build');
      expect(diagnostics['receiver']['address'], '127.0.0.1:4545');
      expect(diagnostics['privacy']['includes_telemetry_payloads'], isFalse);
      expect(diagnostics.containsKey('prompt'), isFalse);
      expect(await controller.cleanupLogs(), 2048);

      expect(requests, contains('POST /api/v1/config/codex/apply'));
      expect(requests, contains('POST /api/v1/config/codex/restore'));
      expect(requests, contains('PUT /api/v1/settings/data-retention'));
      expect(requests, contains('GET /api/v1/settings/receiver'));
      expect(requests, contains('PUT /api/v1/settings/receiver'));
      expect(requests, contains('DELETE /api/v1/data/history'));
    },
  );

  test(
    'clears settings state when the backend API client is detached',
    () async {
      controller.setApiClient(null);
      expect(controller.retention, isEmpty);
      expect(controller.retentionDays, 30);
      expect(controller.receiverPort, 4318);
    },
  );

  test(
    'ignores an in-flight response after the backend API client is detached',
    () async {
      retentionGate = Completer<void>();
      retentionRequestStarted = Completer<void>();
      final loading = controller.loadRetention();
      await retentionRequestStarted!.future;

      controller.setApiClient(null);
      retentionGate!.complete();
      await loading;

      expect(controller.retention, isEmpty);
      expect(controller.loadingRetention, isFalse);
    },
  );
}
