import 'package:agentpulse/core/api/local_api_client.dart';
import 'package:agentpulse/features/diagnostics/diagnostics_page.dart';
import 'package:agentpulse/features/live/live_page.dart';
import 'package:agentpulse/features/processes/process_explorer_page.dart';
import 'package:agentpulse/features/runs/run_detail_page.dart';
import 'package:agentpulse/features/sessions/session_explorer_page.dart';
import 'package:agentpulse/features/settings/settings_page.dart';
import 'package:agentpulse/features/tools/tool_analytics_page.dart';
import 'package:agentpulse/ui/shell/app_shell.dart';
import 'package:agentpulse/ui/theme/ap_theme.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  final api = _GoldenApi();

  testWidgets('1440x900 Live active', (tester) async {
    await _capture(
      tester,
      name: 'goldens/1440x900_live_active.png',
      selectedIndex: 0,
      child: LivePage(
        status: _readyStatus,
        summary: const {},
        sessions: _activeRun,
        toolSummaries: _toolSummaries,
        toolCalls: _toolCalls,
        processes: _processes,
        trendPoints: _trendPoints,
        starting: false,
        loading: false,
        onRefresh: _noop,
        onOpenRun: (_) {},
        onOpenRuns: _noop,
      ),
      sessions: _activeRun,
    );
  });

  testWidgets('1440x900 Live waiting', (tester) async {
    await _capture(
      tester,
      name: 'goldens/1440x900_live_waiting.png',
      selectedIndex: 0,
      status: _waitingStatus,
      child: LivePage(
        status: _waitingStatus,
        summary: const {},
        sessions: const [],
        toolSummaries: const [],
        toolCalls: const [],
        processes: const [],
        trendPoints: const [],
        starting: false,
        loading: false,
        onRefresh: _noop,
        onOpenRun: (_) {},
        onOpenRuns: _noop,
      ),
    );
  });

  testWidgets('1440x900 Runs history', (tester) async {
    await _capture(
      tester,
      name: 'goldens/1440x900_runs.png',
      selectedIndex: 1,
      child: SessionExplorerPage(apiClient: api, onOpenSession: (_) {}),
      pumpCount: 3,
    );
  });

  testWidgets('1440x900 Run Detail timeline', (tester) async {
    await _capture(
      tester,
      name: 'goldens/1440x900_run_detail_timeline.png',
      selectedIndex: 1,
      child: RunDetailPage(
        apiClient: api,
        run: _activeRun.first,
        onBack: _noop,
      ),
      sessions: _activeRun,
      pumpCount: 3,
    );
  });

  testWidgets('1440x900 Run Detail model breakdown', (tester) async {
    await _capture(
      tester,
      name: 'goldens/1440x900_run_detail_model.png',
      selectedIndex: 1,
      child: RunDetailPage(
        apiClient: api,
        run: _activeRun.first,
        onBack: _noop,
      ),
      sessions: _activeRun,
      pumpCount: 3,
      afterPump: (tester) async {
        await tester.tap(find.text('Model'));
        await tester.pump();
        await tester.pump(const Duration(milliseconds: 250));
      },
    );
  });

  testWidgets('1440x900 Tools analytics', (tester) async {
    await _capture(
      tester,
      name: 'goldens/1440x900_tools.png',
      selectedIndex: 2,
      child: ToolAnalyticsPage(
        range: '24h',
        summaries: _toolSummaries,
        calls: _toolCalls,
        loading: false,
        loadingMore: false,
        nextCursor: null,
        onRangeChanged: (_) {},
        onRefresh: _noop,
        onLoadMore: _noop,
      ),
    );
  });

  testWidgets('1440x900 Process Explorer', (tester) async {
    await _capture(
      tester,
      name: 'goldens/1440x900_processes.png',
      selectedIndex: 3,
      child: ProcessExplorerPage(
        status: const {'enabled': true, 'state': 'sampling'},
        processes: _processes,
        loading: false,
        loadingMore: false,
        nextCursor: null,
        onRefresh: _noop,
        onLoadMore: _noop,
      ),
    );
  });

  testWidgets('1440x900 Diagnostics', (tester) async {
    await _capture(
      tester,
      name: 'goldens/1440x900_diagnostics.png',
      selectedIndex: 4,
      child: DiagnosticsPage(
        apiClient: api,
        status: _readyStatus,
        backendVersion: '1.0.0',
        retention: const {'database_size_bytes': 7340032, 'retention_days': 30},
        onRefresh: _noop,
      ),
      pumpCount: 3,
    );
  });

  testWidgets('1440x900 Settings', (tester) async {
    await _capture(
      tester,
      name: 'goldens/1440x900_settings.png',
      selectedIndex: 5,
      child: _settingsPage(),
    );
  });

  testWidgets('1280x800 Live layout', (tester) async {
    await _capture(
      tester,
      size: const Size(1280, 800),
      name: 'goldens/1280x800_live.png',
      selectedIndex: 0,
      child: LivePage(
        status: _readyStatus,
        summary: const {},
        sessions: _activeRun,
        toolSummaries: _toolSummaries,
        toolCalls: _toolCalls,
        processes: _processes,
        trendPoints: _trendPoints,
        starting: false,
        loading: false,
        onRefresh: _noop,
        onOpenRun: (_) {},
        onOpenRuns: _noop,
      ),
      sessions: _activeRun,
    );
  });

  testWidgets('1100x720 Live compact layout', (tester) async {
    await _capture(
      tester,
      size: const Size(1100, 720),
      name: 'goldens/1100x720_live_compact.png',
      selectedIndex: 0,
      child: LivePage(
        status: _readyStatus,
        summary: const {},
        sessions: _activeRun,
        toolSummaries: _toolSummaries,
        toolCalls: _toolCalls,
        processes: _processes,
        trendPoints: _trendPoints,
        starting: false,
        loading: false,
        onRefresh: _noop,
        onOpenRun: (_) {},
        onOpenRuns: _noop,
      ),
      sessions: _activeRun,
    );
  });
}

Future<void> _capture(
  WidgetTester tester, {
  required String name,
  required int selectedIndex,
  required Widget child,
  Size size = const Size(1440, 900),
  Map<String, dynamic>? status,
  List<Map<String, dynamic>> sessions = const [],
  int pumpCount = 1,
  Future<void> Function(WidgetTester tester)? afterPump,
}) async {
  tester.view.physicalSize = size;
  tester.view.devicePixelRatio = 1;
  addTearDown(tester.view.resetPhysicalSize);
  addTearDown(tester.view.resetDevicePixelRatio);
  await tester.pumpWidget(
    MaterialApp(
      theme: ApTheme.dark,
      home: AppShell(
        selectedIndex: selectedIndex,
        onNavigate: (_) {},
        status: status ?? _readyStatus,
        backendVersion: '1.0.0',
        starting: false,
        apiAvailable: true,
        sessions: sessions,
        onRefresh: _noop,
        onRestart: _noop,
        child: child,
      ),
    ),
  );
  for (var i = 0; i < pumpCount; i++) {
    await tester.pump();
  }
  await tester.pump(const Duration(milliseconds: 250));
  await afterPump?.call(tester);
  expect(tester.takeException(), isNull);
  await expectLater(find.byType(MaterialApp), matchesGoldenFile(name));
}

SettingsPage _settingsPage() => SettingsPage(
  signals: (_readyStatus['signals'] as Map).cast<String, dynamic>(),
  status: _readyStatus,
  backendVersion: '1.0.0',
  config: const {
    'path': r'C:\Users\test\.codex\config.toml',
    'backups': ['config.toml.backup'],
  },
  preview: const {
    'changes': {
      'OTEL_EXPORTER_OTLP_ENDPOINT': 'http://127.0.0.1:4318',
      'OTEL_EXPORTER_OTLP_PROTOCOL': 'http/protobuf',
    },
    'restart_required': true,
  },
  selectedBackup: 'config.toml.backup',
  retention: const {
    'database_size_bytes': 7340032,
    'status': {'oldest_record_at': '2026-10-23T10:00:00Z'},
  },
  retentionDays: 30,
  receiverPort: 4318,
  receiverSettingsError: null,
  loadingReceiverSettings: false,
  savingReceiverSettings: false,
  retentionError: null,
  loadingRetention: false,
  savingRetention: false,
  requestingCleanup: false,
  exportingData: false,
  clearingHistory: false,
  exportingDiagnostics: false,
  cleaningLogs: false,
  apiAvailable: true,
  onSelectedBackupChanged: (_) {},
  onRetentionDaysChanged: (_) {},
  onReceiverPortChanged: (_) {},
  onSaveReceiverSettings: _noop,
  onLoadReceiverSettings: _noop,
  onLoadConfig: _noop,
  onLoadPreview: _noop,
  onApplyConfig: _noop,
  onRestoreConfig: _noop,
  onSaveRetention: _noop,
  onRequestCleanup: _noop,
  onExportData: _noop,
  onClearHistory: _noop,
  onExportDiagnostics: _noop,
  onCleanupLogs: _noop,
);

class _GoldenApi extends LocalApiClient {
  _GoldenApi()
    : super(baseUri: Uri.parse('http://127.0.0.1'), token: 'golden-test-token');

  @override
  Future<Map<String, dynamic>> get(String path) async {
    if (path.contains('/timeline')) return {'items': _timeline};
    if (path.endsWith('/analysis')) return _analysis;
    if (path.endsWith('/processes')) return {'items': _processes};
    if (path.endsWith('/diagnostics/ingest')) {
      return {
        'recent': [
          {
            'signal': 'traces',
            'status': 'accepted',
            'received_at': '2026-10-23T10:01:00Z',
            'records': 4,
          },
          {
            'signal': 'logs',
            'status': 'rejected',
            'received_at': '2026-10-23T10:00:00Z',
            'records': 2,
          },
        ],
      };
    }
    return {'items': _runs, 'next_cursor': null};
  }
}

final _activeRun = [
  {
    'id': 'codex:session-001',
    'external_id': 'session-001',
    'model': 'gpt-5-codex',
    'last_activity_at': DateTime.now().toIso8601String(),
    'activity_duration_ms': 84200,
    'input_tokens': 5832,
    'output_tokens': 1240,
    'tool_call_count': 4,
    'command_count': 2,
    'failed_call_count': 1,
    'data_quality': 'partial',
  },
];

final _runs = [
  {
    'id': 'codex:session-001',
    'external_id': 'session-001',
    'started_at': '2026-10-23T09:58:35Z',
    'model': 'gpt-5-codex',
    'activity_duration_ms': 84200,
    'input_tokens': 5832,
    'output_tokens': 1240,
    'tool_call_count': 4,
    'command_count': 2,
    'failed_call_count': 1,
    'data_quality': 'partial',
  },
  {
    'id': 'codex:session-002',
    'external_id': 'session-002',
    'started_at': '2026-10-22T17:18:21Z',
    'model': 'gpt-5-codex-mini',
    'activity_duration_ms': 12600,
    'input_tokens': 2210,
    'output_tokens': 487,
    'tool_call_count': 2,
    'command_count': 1,
    'failed_call_count': 0,
    'data_quality': 'complete',
  },
  {
    'id': 'codex:session-003',
    'external_id': 'session-003',
    'started_at': '2026-10-22T14:06:02Z',
    'model': 'gpt-5-codex',
    'activity_duration_ms': 225000,
    'input_tokens': 10420,
    'output_tokens': 3310,
    'tool_call_count': 7,
    'command_count': 4,
    'failed_call_count': 2,
    'data_quality': 'partial',
  },
];

final _toolSummaries = [
  {
    'tool_name': 'shell',
    'total_calls': 18,
    'failure_calls': 3,
    'failure_rate': 0.1667,
    'p50_duration_ms': 420.0,
    'p95_duration_ms': 1890.0,
    'unknown_outcomes': 1,
  },
  {
    'tool_name': 'apply_patch',
    'total_calls': 8,
    'failure_calls': 0,
    'failure_rate': 0.0,
    'p50_duration_ms': 135.0,
    'p95_duration_ms': 540.0,
    'unknown_outcomes': 0,
  },
  {
    'tool_name': 'read_file',
    'total_calls': 13,
    'failure_calls': 1,
    'failure_rate': 0.0769,
    'p50_duration_ms': 46.0,
    'p95_duration_ms': 184.0,
    'unknown_outcomes': 0,
  },
];

final _toolCalls = [
  {
    'tool_name': 'shell',
    'started_at_ns':
        DateTime.parse('2026-10-23T10:00:12Z').microsecondsSinceEpoch * 1000,
    'duration_ms': 1890.0,
    'status': 'failed',
    'retry_index': 1,
    'session_id': 'codex:session-001',
    'data_quality': 'partial',
    'correlation_method': 'explicit_process_pid',
  },
  {
    'tool_name': 'apply_patch',
    'started_at_ns':
        DateTime.parse('2026-10-23T10:00:05Z').microsecondsSinceEpoch * 1000,
    'duration_ms': 135.0,
    'status': 'success',
    'retry_index': 0,
    'session_id': 'codex:session-001',
    'data_quality': 'complete',
  },
  {
    'tool_name': 'read_file',
    'started_at_ns':
        DateTime.parse('2026-10-23T10:00:02Z').microsecondsSinceEpoch * 1000,
    'duration_ms': 46.0,
    'status': 'success',
    'retry_index': 0,
    'session_id': 'codex:session-001',
    'data_quality': 'complete',
  },
];

final _processes = [
  {
    'pid': 4212,
    'parent_pid': 4010,
    'image_name': 'pwsh.exe',
    'started_at': '2026-10-23T10:00:12Z',
    'cpu_time_ms': 828.0,
    'working_set_bytes': 104857600,
    'sample_quality': 'complete',
    'command_category': 'shell',
    'association_method': 'explicit_process_pid',
    'association_confidence': 1.0,
    'associated_tool_name': 'shell',
  },
  {
    'pid': 4388,
    'parent_pid': 4212,
    'image_name': 'python.exe',
    'started_at': '2026-10-23T10:00:14Z',
    'cpu_time_ms': 1210.0,
    'working_set_bytes': 176160768,
    'sample_quality': 'sampled',
    'command_category': 'build',
    'association_method': 'parent_pid_time_overlap',
    'association_confidence': 0.82,
    'associated_tool_name': 'shell',
  },
];

final _timeline = [
  {
    'kind': 'model_request',
    'start_at': '2026-10-23T10:00:00Z',
    'end_at': '2026-10-23T10:00:04.600Z',
    'duration_ms': 4600.0,
    'model': 'gpt-5-codex',
    'span_id': 'span-model-001',
    'outcome': 'success',
    'data_quality': 'complete',
  },
  {
    'kind': 'tool_call',
    'start_at': '2026-10-23T10:00:01.200Z',
    'end_at': '2026-10-23T10:00:03.090Z',
    'duration_ms': 1890.0,
    'tool': 'shell',
    'span_id': 'span-tool-001',
    'outcome': 'failed',
    'data_quality': 'partial',
    'correlation_method': 'explicit_process_pid',
  },
];

final _analysis = {
  'activity_window_ms': 84200,
  'data_quality': 'partial',
  'input_tokens': 5832,
  'output_tokens': 1240,
  'durations': {
    'wall_clock': {'value_ns': 84200000000, 'quality': 'complete'},
    'llm_only': {'value_ns': 48200000000, 'quality': 'complete'},
    'command_only': {'value_ns': 21400000000, 'quality': 'partial'},
    'overlap': {'value_ns': 12800000000, 'quality': 'estimated'},
    'unattributed': {'value_ns': 1800000000, 'quality': 'partial'},
  },
  'tokens': {
    'ttft_ms': {'value': 820.0, 'quality': 'complete'},
    'request_rate': {'tokens_per_second': 28.4, 'quality': 'partial'},
    'generation_rate': {'tokens_per_second': 42.1, 'quality': 'partial'},
  },
  'tools': [
    {
      'tool_name': 'shell',
      'total_calls': 4,
      'failure_calls': 1,
      'failure_rate': 0.25,
    },
  ],
};

final _trendPoints = [
  {
    'timestamp': '2026-10-23T09:58:00Z',
    'has_data': true,
    'average_ttft_ms': 920.0,
    'average_request_rate_tokens_per_second': 24.2,
    'average_duration_ms': 1650.0,
  },
  {
    'timestamp': '2026-10-23T09:59:00Z',
    'has_data': true,
    'average_ttft_ms': 740.0,
    'average_request_rate_tokens_per_second': 31.8,
    'average_duration_ms': 1210.0,
  },
  {
    'timestamp': '2026-10-23T10:00:00Z',
    'has_data': true,
    'average_ttft_ms': 820.0,
    'average_request_rate_tokens_per_second': 28.4,
    'average_duration_ms': 1390.0,
  },
];

final _readyStatus = {
  'receiver': 'ready',
  'receiver_address': '127.0.0.1:4318',
  'schema_version': 1,
  'signals': {
    'logs': {'state': 'receiving'},
    'metrics': {'state': 'receiving'},
    'traces': {'state': 'receiving'},
  },
  'process_monitor': {'enabled': true, 'state': 'sampling'},
  'ingest_queue': {'depth': 2, 'capacity': 256, 'rejected': 1},
};

final _waitingStatus = {
  'receiver': 'ready',
  'receiver_address': '127.0.0.1:4318',
  'schema_version': 1,
  'signals': {
    'logs': {'state': 'stale'},
    'metrics': {'state': 'missing'},
    'traces': {'state': 'missing'},
  },
  'process_monitor': {'enabled': false, 'state': 'disabled'},
  'ingest_queue': {'depth': 0, 'capacity': 256, 'rejected': 0},
};

void _noop() {}
