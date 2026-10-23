import 'package:agentpulse/features/dashboard/dashboard_overview.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  testWidgets(
    'shows distinct empty states for sessions and unassigned events',
    (tester) async {
      await tester.binding.setSurfaceSize(const Size(1200, 1600));
      addTearDown(() => tester.binding.setSurfaceSize(null));
      await tester.pumpWidget(MaterialApp(home: Scaffold(body: _testPage())));

      await tester.scrollUntilVisible(
        find.text('收到包含会话标识的遥测后，会在这里显示会话。'),
        300,
        scrollable: find.byType(Scrollable).first,
      );
      expect(find.text('收到包含会话标识的遥测后，会在这里显示会话。'), findsOneWidget);
      await tester.scrollUntilVisible(
        find.text('当前没有未关联事件。'),
        300,
        scrollable: find.byType(Scrollable).first,
      );
      expect(find.text('当前没有未关联事件。'), findsOneWidget);
    },
  );

  testWidgets('exposes restart recovery and forwards the selected range', (
    tester,
  ) async {
    await tester.binding.setSurfaceSize(const Size(1200, 1600));
    addTearDown(() => tester.binding.setSurfaceSize(null));
    var restartCount = 0;
    String? selectedRange;
    await tester.pumpWidget(
      MaterialApp(
        home: Scaffold(
          body: _testPage(
            backendError: '服务未就绪',
            apiAvailable: false,
            onRestartBackend: () => restartCount++,
            onRangeChanged: (value) => selectedRange = value,
          ),
        ),
      ),
    );

    await tester.tap(find.text('重新启动'));
    expect(restartCount, 1);

    await tester.tap(find.byType(DropdownButton<String>).first);
    await tester.pumpAndSettle();
    await tester.tap(find.text('7 天').last);

    expect(selectedRange, '7d');
  });
}

DashboardOverview _testPage({
  String? backendError,
  bool apiAvailable = true,
  VoidCallback? onRestartBackend,
  ValueChanged<String>? onRangeChanged,
}) => DashboardOverview(
  status: const {
    'signals': {'logs': {}, 'metrics': {}, 'traces': {}},
  },
  summary: const {
    'events': {'logs': 0, 'metrics': 0, 'traces': 0},
  },
  starting: false,
  apiAvailable: apiAvailable,
  backendError: backendError,
  queryError: null,
  range: '1h',
  trendPoints: const [],
  loadingTrends: false,
  trendError: null,
  sessions: const [],
  sessionCursor: null,
  loadingSessions: false,
  loadingMoreSessions: false,
  sessionError: null,
  unassignedEvents: const [],
  unassignedCursor: null,
  loadingUnassigned: false,
  loadingMoreUnassigned: false,
  unassignedError: null,
  onRangeChanged: onRangeChanged ?? (_) {},
  onRefresh: () {},
  onRestartBackend: onRestartBackend ?? () {},
  onLoadMoreSessions: () {},
  onRetrySessions: () {},
  onOpenSession: (_) {},
  onLoadMoreUnassigned: () {},
  onRetryUnassigned: () {},
);
