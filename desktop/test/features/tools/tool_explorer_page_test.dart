import 'package:agentpulse/features/tools/tool_explorer_page.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  testWidgets('empty summaries and calls have separate empty states', (
    tester,
  ) async {
    await tester.pumpWidget(
      MaterialApp(
        home: Scaffold(
          body: ToolExplorerPage(
            range: '1h',
            toolFilter: 'all',
            sessionFilter: '',
            outcomeFilter: 'all',
            summaries: const [],
            calls: const [],
            nextCallCursor: null,
            loading: false,
            loadingMore: false,
            error: null,
            onRangeChanged: (_) {},
            onToolFilterChanged: (_) {},
            onSessionFilterChanged: (_) {},
            onOutcomeChanged: (_) {},
            onRefresh: () {},
            onLoadMore: () {},
          ),
        ),
      ),
    );

    expect(find.text('所选时间范围内没有工具调用。'), findsOneWidget);
    expect(find.text('此筛选条件下没有调用记录。'), findsOneWidget);
  });

  testWidgets('applies a pasted session filter', (tester) async {
    String? appliedSession;
    await tester.pumpWidget(
      MaterialApp(
        home: Scaffold(
          body: ToolExplorerPage(
            range: '1h',
            toolFilter: 'all',
            sessionFilter: '',
            outcomeFilter: 'all',
            summaries: const [],
            calls: const [],
            nextCallCursor: null,
            loading: false,
            loadingMore: false,
            error: null,
            onRangeChanged: (_) {},
            onToolFilterChanged: (_) {},
            onSessionFilterChanged: (value) => appliedSession = value,
            onOutcomeChanged: (_) {},
            onRefresh: () {},
            onLoadMore: () {},
          ),
        ),
      ),
    );

    await tester.enterText(
      find.byKey(const Key('tool-session-filter')),
      'codex:session-1',
    );
    await tester.tap(find.text('筛选会话'));
    expect(appliedSession, 'codex:session-1');
  });

  testWidgets('renders call details and invokes pagination callback', (
    tester,
  ) async {
    var loadMoreCalled = false;
    await tester.pumpWidget(
      MaterialApp(
        home: Scaffold(
          body: ToolExplorerPage(
            range: '1h',
            toolFilter: 'all',
            sessionFilter: '',
            outcomeFilter: 'all',
            summaries: const [
              {
                'tool_name': 'shell',
                'total_calls': 2,
                'success_calls': 1,
                'failure_calls': 1,
                'unknown_outcomes': 0,
                'p50_duration_ms': 25,
                'p95_duration_ms': 80,
                'failure_rate': 0.5,
                'data_quality': 'complete',
              },
            ],
            calls: const [
              {
                'tool_name': 'shell',
                'status': 'success',
                'retry_index': 0,
                'duration_ms': 25,
                'data_quality': 'complete',
              },
            ],
            nextCallCursor: 'next-page',
            loading: false,
            loadingMore: false,
            error: null,
            onRangeChanged: (_) {},
            onToolFilterChanged: (_) {},
            onSessionFilterChanged: (_) {},
            onOutcomeChanged: (_) {},
            onRefresh: () {},
            onLoadMore: () => loadMoreCalled = true,
          ),
        ),
      ),
    );

    expect(find.text('2 次'), findsOneWidget);
    expect(find.textContaining('重试序号 0'), findsOneWidget);
    expect(find.text('加载更多调用'), findsOneWidget);

    await tester.tap(find.text('加载更多调用'));
    expect(loadMoreCalled, isTrue);
  });
}
