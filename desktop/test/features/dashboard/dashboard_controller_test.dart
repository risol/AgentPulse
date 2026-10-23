import 'dart:convert';
import 'dart:io';

import 'package:agentpulse/core/api/local_api_client.dart';
import 'package:agentpulse/features/dashboard/dashboard_controller.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  late HttpServer server;
  late LocalApiClient api;
  late DashboardController controller;
  final requests = <Uri>[];
  bool failTrends = false;
  bool pageToolCalls = false;

  setUp(() async {
    requests.clear();
    failTrends = false;
    pageToolCalls = false;
    server = await HttpServer.bind(InternetAddress.loopbackIPv4, 0);
    server.listen((request) async {
      requests.add(request.uri);
      final path = request.uri.path;
      final cursor = request.uri.queryParameters['cursor'];
      Object response;
      if (path == '/api/v1/status') {
        response = {'schema_version': 10, 'signals': {}};
      } else if (path == '/api/v1/dashboard/summary') {
        response = {
          'events': {'logs': 3},
        };
      } else if (path == '/api/v1/sessions') {
        response = cursor == null
            ? {
                'items': [
                  {'id': 'session-1'},
                ],
                'next_cursor': 'session-cursor',
              }
            : {
                'items': [
                  {'id': 'session-2'},
                ],
                'next_cursor': null,
              };
      } else if (path == '/api/v1/events/unassigned') {
        response = {
          'items': [
            {'id': 'unassigned-1'},
          ],
          'next_cursor': null,
        };
      } else if (path == '/api/v1/processes') {
        response = {
          'items': [
            {'pid': 42},
          ],
          'next_cursor': null,
        };
      } else if (path == '/api/v1/tools/summary') {
        response = {
          'items': [
            {'tool_name': 'shell'},
          ],
        };
      } else if (path == '/api/v1/tools/calls') {
        response = {
          'items': [
            {
              'tool_name': 'shell',
              'status': 'failed',
              'id': cursor == null ? 'call-1' : 'call-2',
            },
          ],
          'next_cursor': pageToolCalls && cursor == null ? 'call-cursor' : null,
        };
      } else if (path == '/api/v1/analytics/trends' && failTrends) {
        request.response.statusCode = HttpStatus.internalServerError;
        response = {'message': 'trend backend failed'};
      } else if (path == '/api/v1/analytics/trends') {
        response = {
          'points': [
            {'started_at_ns': 100, 'input_tokens': 5},
          ],
        };
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
      token: 'dashboard-test-token',
    );
    controller = DashboardController()..setApiClient(api);
  });

  tearDown(() async {
    controller.dispose();
    await server.close(force: true);
  });

  test(
    'loads feature data concurrently and pages sessions by cursor',
    () async {
      await controller.loadData(
        includeSessions: true,
        includeProcesses: true,
        includeTools: true,
        includeTrends: true,
      );

      expect(controller.status['schema_version'], 10);
      expect(controller.summary['events']['logs'], 3);
      expect(controller.sessions.single['id'], 'session-1');
      expect(controller.unassignedEvents.single['id'], 'unassigned-1');
      expect(controller.processes.single['pid'], 42);
      expect(controller.toolCalls.single['status'], 'failed');
      expect(controller.trendPoints.single['input_tokens'], 5);
      expect(controller.loadingSessions, isFalse);
      expect(controller.loadingProcesses, isFalse);
      expect(controller.loadingTools, isFalse);
      expect(controller.loadingTrends, isFalse);

      await controller.loadMoreSessions();

      expect(controller.sessions.map((session) => session['id']), [
        'session-1',
        'session-2',
      ]);
      expect(controller.sessionCursor, isNull);
      expect(requests.last.queryParameters['cursor'], 'session-cursor');
    },
  );

  test('filters tool summaries and every call page by session ID', () async {
    pageToolCalls = true;
    controller.setToolSessionFilter(' codex:session-1 ');
    await controller.loadData(includeTools: true);

    final summaryRequest = requests.firstWhere(
      (uri) => uri.path == '/api/v1/tools/summary',
    );
    final firstPage = requests.firstWhere(
      (uri) => uri.path == '/api/v1/tools/calls',
    );
    expect(summaryRequest.queryParameters['session_id'], 'codex:session-1');
    expect(firstPage.queryParameters['session_id'], 'codex:session-1');
    expect(controller.toolCallCursor, 'call-cursor');

    await controller.loadMoreToolCalls();
    expect(requests.last.queryParameters['session_id'], 'codex:session-1');
    expect(requests.last.queryParameters['cursor'], 'call-cursor');
    expect(controller.toolCalls.map((call) => call['id']), [
      'call-1',
      'call-2',
    ]);
  });

  test(
    'applies tool filters and exposes query errors without stale loading',
    () async {
      controller.setToolFilter('shell');
      controller.setToolOutcome('failed');
      await controller.loadData(includeTools: true);

      final callsRequest = requests.lastWhere(
        (uri) => uri.path == '/api/v1/tools/calls',
      );
      expect(callsRequest.queryParameters['tool_name'], 'shell');
      expect(callsRequest.queryParameters['outcome'], 'failed');

      failTrends = true;
      await controller.loadData(includeTrends: true);

      expect(controller.trendError, contains('trend backend failed'));
      expect(controller.queryError, contains('部分仪表盘数据读取失败'));
      expect(controller.loadingTrends, isFalse);
    },
  );
}
