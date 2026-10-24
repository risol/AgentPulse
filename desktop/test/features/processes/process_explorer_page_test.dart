import 'package:agentpulse/features/processes/process_explorer_page.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  testWidgets('explains the disabled process monitor with empty states', (
    tester,
  ) async {
    tester.view.physicalSize = const Size(1440, 1000);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);

    await tester.pumpWidget(
      const MaterialApp(
        home: Scaffold(
          body: ProcessExplorerPage(
            status: {'enabled': false, 'state': 'disabled'},
            processes: [],
            loading: false,
            loadingMore: false,
            nextCursor: null,
            onRefresh: _noop,
            onLoadMore: _noop,
          ),
        ),
      ),
    );
    await tester.pumpAndSettle();

    expect(find.text('Processes'), findsOneWidget);
    expect(find.text('No process tree'), findsOneWidget);
    expect(find.text('No process samples'), findsOneWidget);
    expect(
      find.textContaining('Enable process monitoring in Settings'),
      findsOneWidget,
    );
    expect(tester.takeException(), isNull);
  });
}

void _noop() {}
