import 'package:agentpulse/features/tools/tool_analytics_page.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  testWidgets('highlights failed tools and filters slow calls', (tester) async {
    tester.view.physicalSize = const Size(1440, 1100);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);
    var range = '24h';

    await tester.pumpWidget(
      MaterialApp(
        home: Scaffold(
          body: ToolAnalyticsPage(
            range: range,
            summaries: const [
              {
                'tool_name': 'shell',
                'total_calls': 4,
                'failure_calls': 2,
                'failure_rate': 0.5,
                'p50_duration_ms': 300,
                'p95_duration_ms': 1800,
                'unknown_outcomes': 1,
              },
            ],
            calls: [
              {
                'tool_name': 'shell',
                'started_at_ns': DateTime.now().microsecondsSinceEpoch * 1000,
                'duration_ms': 1800,
                'status': 'failed',
                'retry_index': 1,
                'data_quality': 'partial',
              },
              {
                'tool_name': 'shell',
                'started_at_ns': DateTime.now().microsecondsSinceEpoch * 1000,
                'duration_ms': 200,
                'status': 'success',
                'retry_index': 0,
                'data_quality': 'complete',
              },
            ],
            loading: false,
            loadingMore: false,
            nextCursor: null,
            error: 'Some tool metrics are unavailable',
            onRangeChanged: (value) => range = value,
            onRefresh: _noop,
            onLoadMore: _noop,
          ),
        ),
      ),
    );
    await tester.pumpAndSettle();

    expect(
      find.textContaining('Some tool metrics are unavailable'),
      findsOneWidget,
    );
    expect(find.text('FAILURE / RETRY'), findsOneWidget);
    expect(find.text('Retried calls (loaded)'), findsOneWidget);
    expect(find.text('MEDIAN P50'), findsOneWidget);
    expect(find.text('MEDIAN P95'), findsOneWidget);
    expect(find.text('shell'), findsWidgets);

    await tester.tap(find.text('Slow ≥ 1s'));
    await tester.pumpAndSettle();
    expect(find.text('shell'), findsWidgets);
    expect(tester.takeException(), isNull);
  });
}

void _noop() {}
