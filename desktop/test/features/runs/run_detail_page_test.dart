import 'package:agentpulse/core/api/local_api_client.dart';
import 'package:agentpulse/features/runs/run_detail_page.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  testWidgets('loads run detail and opens a selected timeline record', (
    tester,
  ) async {
    _useDesktopViewport(tester);
    final api = _FakeRunApiClient();
    var backCalls = 0;

    await tester.pumpWidget(
      MaterialApp(
        home: Scaffold(
          body: RunDetailPage(
            apiClient: api,
            run: const {
              'id': 'codex:internal-id',
              'external_id': 'external-display-id',
              'model': 'gpt-test',
              'activity_duration_ms': 1200,
              'data_quality': 'partial',
            },
            onBack: () => backCalls++,
          ),
        ),
      ),
    );
    await tester.pumpAndSettle();

    expect(find.text('Run external-display-id'), findsOneWidget);
    expect(
      api.paths,
      containsAll([
        '/api/v1/sessions/codex%3Ainternal-id/timeline?limit=500',
        '/api/v1/sessions/codex%3Ainternal-id/analysis',
        '/api/v1/sessions/codex%3Ainternal-id/processes',
      ]),
    );
    expect(find.text('INSPECTOR'), findsOneWidget);
    expect(find.text('Select a span'), findsOneWidget);

    await tester.tap(find.text('模型请求'));
    await tester.pump();
    expect(find.text('gpt-test').last, findsOneWidget);
    expect(find.text('span-1'), findsOneWidget);

    await tester.tap(find.byTooltip('Hide inspector'));
    await tester.pump();
    expect(find.text('INSPECTOR'), findsNothing);
    await tester.tap(find.byTooltip('Show inspector'));
    await tester.pump();
    expect(find.text('INSPECTOR'), findsOneWidget);
    expect(find.text('span-1'), findsOneWidget);

    await tester.tap(find.text('Model'));
    await tester.pumpAndSettle();
    expect(find.text('TIME BREAKDOWN'), findsOneWidget);
    expect(
      find.textContaining('LLM and command intervals can overlap'),
      findsOneWidget,
    );

    await tester.tap(find.text('Data Quality'));
    await tester.pumpAndSettle();
    expect(find.text('PARTIAL'), findsWidgets);

    await tester.tap(find.byTooltip('Back to runs'));
    expect(backCalls, 1);
    expect(tester.takeException(), isNull);
  });

  testWidgets('renders a complete run without downgrading its quality', (
    tester,
  ) async {
    _useDesktopViewport(tester);
    final api = _FakeRunApiClient()..completeQuality = true;

    await tester.pumpWidget(
      MaterialApp(
        home: Scaffold(
          body: RunDetailPage(
            apiClient: api,
            run: const {
              'id': 'run-complete',
              'external_id': 'run-complete',
              'data_quality': 'complete',
            },
            onBack: _noop,
          ),
        ),
      ),
    );
    await tester.pumpAndSettle();

    await tester.tap(find.text('Data Quality'));
    await tester.pumpAndSettle();
    expect(find.text('COMPLETE'), findsWidgets);
    expect(tester.takeException(), isNull);
  });

  testWidgets('shows a recoverable state when run detail loading fails', (
    tester,
  ) async {
    _useDesktopViewport(tester);
    final api = _FakeRunApiClient()..failTimeline = true;

    await tester.pumpWidget(
      MaterialApp(
        home: Scaffold(
          body: RunDetailPage(
            apiClient: api,
            run: const {'id': 'run-1', 'external_id': 'run-1'},
            onBack: _noop,
          ),
        ),
      ),
    );
    await tester.pumpAndSettle();

    expect(find.textContaining('Unable to load run analysis'), findsOneWidget);
    expect(tester.takeException(), isNull);
  });
}

class _FakeRunApiClient extends LocalApiClient {
  _FakeRunApiClient()
    : super(baseUri: Uri.parse('http://127.0.0.1'), token: 'run-test-token');

  final paths = <String>[];
  bool failTimeline = false;
  bool completeQuality = false;

  @override
  Future<Map<String, dynamic>> get(String path) async {
    paths.add(path);
    if (failTimeline && path.contains('/timeline')) {
      throw StateError('timeline unavailable');
    }
    if (path.endsWith('/timeline?limit=500')) {
      return {
        'items': [
          {
            'kind': 'model_request',
            'start_at': '2026-10-23T10:00:00Z',
            'end_at': '2026-10-23T10:00:01Z',
            'duration_ms': 1000,
            'model': 'gpt-test',
            'span_id': 'span-1',
            'outcome': 'success',
            'data_quality': 'complete',
          },
          {
            'kind': 'tool_call',
            'start_at': '2026-10-23T10:00:00.200Z',
            'end_at': '2026-10-23T10:00:00.800Z',
            'duration_ms': 600,
            'tool': 'shell',
            'span_id': 'span-2',
            'outcome': 'error',
            'data_quality': 'partial',
            'correlation_method': 'explicit_process_pid',
          },
        ],
      };
    }
    if (path.endsWith('/analysis')) {
      return {
        'activity_window_ms': 1000,
        'data_quality': completeQuality ? 'complete' : 'partial',
        'durations': {
          'wall_clock': {'value_ns': 1000000000, 'quality': 'complete'},
          'llm_only': {'value_ns': 500000000, 'quality': 'complete'},
          'command_only': {'value_ns': 200000000, 'quality': 'partial'},
          'overlap': {'value_ns': 300000000, 'quality': 'estimated'},
          'unattributed': {'value_ns': 0, 'quality': 'complete'},
        },
        'tokens': {
          'ttft_ms': {'value': 250, 'quality': 'complete'},
          'request_rate': {'tokens_per_second': 10.0, 'quality': 'partial'},
        },
        'input_tokens': 100,
        'output_tokens': 20,
        'tools': [
          {
            'tool_name': 'shell',
            'total_calls': 2,
            'failure_calls': 1,
            'failure_rate': 0.5,
          },
        ],
      };
    }
    return const {'items': <Map<String, dynamic>>[]};
  }
}

void _useDesktopViewport(WidgetTester tester) {
  tester.view.physicalSize = const Size(1440, 1000);
  tester.view.devicePixelRatio = 1;
  addTearDown(tester.view.resetPhysicalSize);
  addTearDown(tester.view.resetDevicePixelRatio);
}

void _noop() {}
