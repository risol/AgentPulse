import 'package:agentpulse/core/api/local_api_client.dart';
import 'package:agentpulse/features/sessions/session_explorer_page.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  late _FakeApiClient api;

  setUp(() {
    api = _FakeApiClient();
  });

  testWidgets(
    'filters sessions, carries filters through paging, and opens detail',
    (tester) async {
      await tester.binding.setSurfaceSize(const Size(1200, 1000));
      addTearDown(() => tester.binding.setSurfaceSize(null));
      Map<String, dynamic>? openedSession;
      await tester.pumpWidget(
        MaterialApp(
          home: Scaffold(
            body: SessionExplorerPage(
              apiClient: api,
              onOpenSession: (session) => openedSession = session,
            ),
          ),
        ),
      );
      await tester.pumpAndSettle();

      expect(find.textContaining('工具 2 · 命令 1 · 失败 1'), findsOneWidget);
      expect(find.textContaining('可观测跨度 1.3 秒'), findsOneWidget);
      expect(find.textContaining('不提供可靠的会话状态或项目路径'), findsOneWidget);
      expect(api.requests.first.queryParameters['range'], 'all');

      await tester.tap(find.byType(DropdownButtonFormField<String>));
      await tester.pumpAndSettle();
      await tester.tap(find.text('最近 7 天'));
      await tester.pumpAndSettle();
      await tester.enterText(find.byType(TextField), 'gpt-5');
      await tester.tap(find.text('筛选'));
      await tester.pumpAndSettle();

      final filteredRequest = api.requests.lastWhere(
        (uri) => uri.path == '/api/v1/sessions',
      );
      expect(filteredRequest.queryParameters['range'], '7d');
      expect(filteredRequest.queryParameters['model'], 'gpt-5');

      await tester.tap(find.text('加载更多会话'));
      await tester.pumpAndSettle();
      final nextPage = api.requests.last;
      expect(nextPage.queryParameters['range'], '7d');
      expect(nextPage.queryParameters['model'], 'gpt-5');
      expect(nextPage.queryParameters['cursor'], 'session-cursor');
      expect(find.text('会话 session-2'), findsOneWidget);

      await tester.tap(find.text('会话 session-1').first);
      expect(openedSession?['id'], 'session-1');
    },
  );
}

class _FakeApiClient extends LocalApiClient {
  _FakeApiClient()
    : super(baseUri: Uri.parse('http://127.0.0.1'), token: 'test-token');

  final requests = <Uri>[];

  @override
  Future<Map<String, dynamic>> get(String path) async {
    final uri = Uri.parse('http://127.0.0.1$path');
    requests.add(uri);
    final isNextPage = uri.queryParameters['cursor'] != null;
    return {
      'items': [
        {
          'id': isNextPage ? 'session-2' : 'session-1',
          'external_id': isNextPage ? 'session-2' : 'session-1',
          'model': 'gpt-5-codex',
          'event_count': 4,
          'activity_duration_ms': 1250,
          'tool_call_count': 2,
          'command_count': 1,
          'failed_call_count': 1,
          'input_tokens': 100,
          'output_tokens': 50,
          'data_quality': 'partial',
          'last_activity_at': '2026-10-23T10:00:00Z',
        },
      ],
      'next_cursor': isNextPage ? null : 'session-cursor',
    };
  }
}
