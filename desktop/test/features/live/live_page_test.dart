import 'package:agentpulse/features/live/live_page.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  testWidgets('shows an active recent run and opens it for analysis', (
    tester,
  ) async {
    _useDesktopViewport(tester);
    final now = DateTime.now().toIso8601String();
    Map<String, dynamic>? openedRun;

    await tester.pumpWidget(
      MaterialApp(
        home: Scaffold(
          body: LivePage(
            status: const {},
            summary: const {},
            sessions: [
              {
                'id': 'run-1',
                'external_id': 'run-1',
                'model': 'gpt-test',
                'started_at': now,
                'last_activity_at': now,
                'input_tokens': 100,
                'output_tokens': 25,
                'data_quality': 'partial',
              },
            ],
            toolSummaries: const [],
            toolCalls: const [
              {'tool_name': 'shell', 'duration_ms': 1200},
            ],
            processes: const [
              {'image_name': 'pwsh.exe', 'ended_at': null},
            ],
            trendPoints: const [],
            starting: false,
            loading: false,
            onRefresh: _noop,
            onOpenRun: (run) => openedRun = run,
            onOpenRuns: _noop,
          ),
        ),
      ),
    );
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 50));

    expect(find.text('RECENT ACTIVITY'), findsOneWidget);
    expect(find.text('gpt-test'), findsWidgets);
    expect(find.text('shell'), findsWidgets);
    await tester.tap(find.byTooltip('Open run'));
    expect(openedRun?['id'], 'run-1');
  });

  testWidgets('shows a degraded state when the receiver is unavailable', (
    tester,
  ) async {
    _useDesktopViewport(tester);
    await tester.pumpWidget(
      MaterialApp(
        home: Scaffold(
          body: LivePage(
            status: const {
              'signals': {
                'logs': {'state': 'stale'},
              },
            },
            summary: const {},
            sessions: const [],
            toolSummaries: const [],
            toolCalls: const [],
            processes: const [],
            trendPoints: const [],
            starting: false,
            error: 'Receiver unavailable',
            loading: false,
            onRefresh: _noop,
            onOpenRun: (_) {},
            onOpenRuns: _noop,
          ),
        ),
      ),
    );
    await tester.pumpAndSettle();

    expect(find.text('DEGRADED'), findsOneWidget);
    expect(find.text('WAITING'), findsNothing);
    expect(find.textContaining('Receiver unavailable'), findsOneWidget);
    expect(tester.takeException(), isNull);
  });

  testWidgets('shows waiting when no run has been observed', (tester) async {
    _useDesktopViewport(tester);
    await tester.pumpWidget(
      MaterialApp(
        home: Scaffold(
          body: LivePage(
            status: const {
              'signals': {
                'logs': {'state': 'stale'},
              },
            },
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
        ),
      ),
    );
    await tester.pumpAndSettle();

    expect(find.text('WAITING'), findsOneWidget);
    expect(find.text('Waiting for the next run'), findsOneWidget);
    expect(tester.takeException(), isNull);
  });
}

void _useDesktopViewport(WidgetTester tester) {
  tester.view.physicalSize = const Size(1440, 1100);
  tester.view.devicePixelRatio = 1;
  addTearDown(tester.view.resetPhysicalSize);
  addTearDown(tester.view.resetDevicePixelRatio);
}

void _noop() {}
