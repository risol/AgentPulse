import 'dart:async';

import 'package:agentpulse/core/api/local_api_client.dart';
import 'package:agentpulse/features/sessions/session_detail_dialog.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  testWidgets('loads detail endpoints with the internal session ID', (
    tester,
  ) async {
    _useDesktopViewport(tester);
    final api = _FakeSessionApiClient();
    await tester.pumpWidget(
      MaterialApp(
        home: Scaffold(
          body: SessionDetailDialog.forSession(
            titleId: 'external-display-id',
            sessionId: 'codex:internal-id',
            apiClient: api,
          ),
        ),
      ),
    );
    await tester.pumpAndSettle();

    expect(find.textContaining('会话详情 · external'), findsOneWidget);
    expect(
      api.paths,
      containsAll([
        '/api/v1/sessions/codex%3Ainternal-id/timeline?limit=100',
        '/api/v1/sessions/codex%3Ainternal-id/analysis',
        '/api/v1/sessions/codex%3Ainternal-id/processes',
      ]),
    );
  });

  testWidgets('shows analysis and selected waterfall record details', (
    tester,
  ) async {
    _useDesktopViewport(tester);
    final timeline = {
      'items': [
        {
          'kind': 'model_request',
          'start_at': '2026-10-23T10:00:00Z',
          'end_at': '2026-10-23T10:00:01Z',
          'model': 'gpt-test',
          'span_id': 'span-1',
          'data_quality': 'complete',
        },
        {
          'kind': 'tool_call',
          'tool': 'shell',
          'start_at': '2026-10-23T10:00:00.200Z',
          'end_at': '2026-10-23T10:00:00.800Z',
          'retry_index': 1,
          'process_pid': 4242,
          'parent_process_pid': 4000,
          'correlation_method': 'explicit_process_pid',
          'data_quality': 'complete',
        },
      ],
    };
    final analysis = {
      'activity_window_ms': 1000,
      'durations': {
        'llm_only': {'value_ns': 500000000},
        'command_only': {'value_ns': 200000000},
        'overlap': {'value_ns': 300000000},
        'unattributed': {'value_ns': 0},
      },
      'tokens': {
        'ttft_ms': {'value': 250},
        'request_rate': {'tokens_per_second': 10.0, 'quality': 'complete'},
        'generation_rate': {'tokens_per_second': 20.0, 'quality': 'partial'},
      },
      'input_tokens': 100,
      'output_tokens': 20,
      'reasoning_tokens': 4,
      'data_quality': 'partial',
      'source_event_ids': ['span-1'],
      'tools': [
        {
          'tool_name': 'shell',
          'total_calls': 2,
          'failure_calls': 1,
          'failure_rate': 0.5,
        },
      ],
    };
    final processes = {
      'items': [
        {
          'pid': 4242,
          'parent_pid': 4000,
          'image_name': 'pwsh.exe',
          'started_at': '2026-10-23T10:00:00Z',
          'ended_at': '2026-10-23T10:00:01Z',
          'cpu_time_ms': 12.0,
          'working_set_bytes': 10485760,
          'sample_quality': 'sampled',
          'association_method': 'explicit_process_pid',
          'association_confidence': 1.0,
          'associated_tool_name': 'shell',
        },
      ],
    };

    await tester.pumpWidget(
      MaterialApp(
        home: Scaffold(
          body: SessionDetailDialog(
            sessionId: 'session-12345678901234567890',
            data: Future.value([timeline, analysis, processes]),
          ),
        ),
      ),
    );
    await tester.pumpAndSettle();

    expect(find.text('会话详情 · session-1234567…'), findsOneWidget);
    expect(find.textContaining('Tokens 输入 100 / 输出 20 / 推理 4'), findsOneWidget);
    expect(
      find.textContaining('工具：shell: 2 次（失败 1，失败率 50.0%）'),
      findsOneWidget,
    );
    expect(find.text('选择一条时间线记录查看字段、数据质量和关联方式。'), findsOneWidget);

    await tester.tap(find.text('进程树'));
    await tester.pumpAndSettle();
    expect(find.text('pwsh.exe'), findsOneWidget);
    expect(find.text('PID 明确关联 100%'), findsOneWidget);
    expect(find.textContaining('内存 10.0 MB'), findsOneWidget);

    await tester.tap(find.text('时间线'));
    await tester.pumpAndSettle();

    await tester.tap(find.text('模型请求'));
    await tester.pump();
    expect(find.text('模型：gpt-test'), findsOneWidget);
    expect(find.text('Span：span-1'), findsOneWidget);

    await tester.tap(find.text('工具 · shell'));
    await tester.pump();
    expect(find.textContaining('重试序号：1'), findsOneWidget);
    expect(find.textContaining('PID：4242 · PPID：4000'), findsOneWidget);
    expect(find.textContaining('关联：explicit_process_pid'), findsOneWidget);
    expect(tester.takeException(), isNull);
  });

  testWidgets('distinguishes empty and failed timeline requests', (
    tester,
  ) async {
    _useDesktopViewport(tester);
    await tester.pumpWidget(
      MaterialApp(
        home: Scaffold(
          body: SessionDetailDialog(
            sessionId: 'session-empty',
            data: Future.value([
              {'items': []},
              const <String, dynamic>{},
              {'items': []},
            ]),
          ),
        ),
      ),
    );
    await tester.pumpAndSettle();
    expect(find.text('没有带可用时间的事件或 Span。'), findsOneWidget);

    final failedData = Completer<List<Map<String, dynamic>>>();
    await tester.pumpWidget(
      MaterialApp(
        home: Scaffold(
          body: SessionDetailDialog(
            sessionId: 'session-error',
            data: failedData.future,
          ),
        ),
      ),
    );
    failedData.completeError(StateError('timeline unavailable'));
    await tester.pump();
    await tester.pumpAndSettle();
    expect(
      find.textContaining('读取会话分析失败：Bad state: timeline unavailable'),
      findsOneWidget,
    );
    expect(tester.takeException(), isNull);
  });
}

class _FakeSessionApiClient extends LocalApiClient {
  _FakeSessionApiClient()
    : super(baseUri: Uri.parse('http://127.0.0.1'), token: 'test-token');

  final paths = <String>[];

  @override
  Future<Map<String, dynamic>> get(String path) async {
    paths.add(path);
    if (path.endsWith('/analysis')) return const <String, dynamic>{};
    return {'items': <Map<String, dynamic>>[]};
  }
}

void _useDesktopViewport(WidgetTester tester) {
  tester.view.physicalSize = const Size(1400, 1100);
  tester.view.devicePixelRatio = 1;
  addTearDown(tester.view.resetPhysicalSize);
  addTearDown(tester.view.resetDevicePixelRatio);
}
