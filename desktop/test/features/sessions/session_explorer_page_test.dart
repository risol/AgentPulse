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

      expect(find.text('1.3 秒'), findsOneWidget);
      expect(find.text('gpt-5-codex'), findsOneWidget);
      expect(
        find.textContaining('activity range reflects observed telemetry spans'),
        findsOneWidget,
      );
      expect(api.requests.first.queryParameters['range'], 'all');

      await tester.tap(find.byType(DropdownButtonFormField<String>));
      await tester.pumpAndSettle();
      await tester.tap(find.text('Last 7 days'));
      await tester.pumpAndSettle();
      await tester.enterText(find.byType(TextField), 'gpt-5');
      await tester.tap(find.text('Filter'));
      await tester.pumpAndSettle();

      final filteredRequest = api.requests.lastWhere(
        (uri) => uri.path == '/api/v1/sessions',
      );
      expect(filteredRequest.queryParameters['range'], '7d');
      expect(filteredRequest.queryParameters['model'], 'gpt-5');

      await tester.tap(find.text('Load more'));
      await tester.pumpAndSettle();
      final nextPage = api.requests.last;
      expect(nextPage.queryParameters['range'], '7d');
      expect(nextPage.queryParameters['model'], 'gpt-5');
      expect(nextPage.queryParameters['cursor'], 'session-cursor');
      expect(find.text('gpt-5-codex'), findsNWidgets(2));

      await tester.tap(find.text('gpt-5-codex').first);
      expect(openedSession?['id'], 'session-1');
    },
  );

  testWidgets('shows a useful empty state when there are no runs', (
    tester,
  ) async {
    await tester.binding.setSurfaceSize(const Size(1200, 1000));
    addTearDown(() => tester.binding.setSurfaceSize(null));

    await tester.pumpWidget(
      MaterialApp(
        home: Scaffold(
          body: SessionExplorerPage(
            apiClient: _FakeApiClient(empty: true),
            onOpenSession: (_) {},
          ),
        ),
      ),
    );
    await tester.pumpAndSettle();

    expect(find.text('No runs yet'), findsOneWidget);
    expect(find.textContaining('waiting for Codex activity'), findsOneWidget);
    expect(tester.takeException(), isNull);
  });
}

class _FakeApiClient extends LocalApiClient {
  _FakeApiClient({this.empty = false})
    : super(baseUri: Uri.parse('http://127.0.0.1'), token: 'test-token');

  final bool empty;
  final requests = <Uri>[];

  @override
  Future<Map<String, dynamic>> get(String path) async {
    final uri = Uri.parse('http://127.0.0.1$path');
    requests.add(uri);
    final isNextPage = uri.queryParameters['cursor'] != null;
    return {
      'items': empty
          ? <Map<String, dynamic>>[]
          : [
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
      'next_cursor': empty || isNextPage ? null : 'session-cursor',
    };
  }
}
