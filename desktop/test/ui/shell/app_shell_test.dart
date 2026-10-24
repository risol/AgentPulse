import 'package:agentpulse/ui/shell/app_shell.dart';
import 'package:agentpulse/ui/theme/ap_theme.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  testWidgets('uses compact navigation at the minimum desktop width', (
    tester,
  ) async {
    tester.view.physicalSize = const Size(1100, 720);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);
    var selectedPage = 0;
    var retries = 0;

    await tester.pumpWidget(
      MaterialApp(
        theme: ApTheme.dark,
        home: AppShell(
          selectedIndex: selectedPage,
          onNavigate: (value) => selectedPage = value,
          status: const {},
          backendVersion: 'test',
          starting: false,
          apiAvailable: false,
          backendError: 'Backend unavailable',
          onRestart: () => retries++,
          child: const Center(child: Text('Workspace')),
        ),
      ),
    );
    await tester.pumpAndSettle();

    expect(find.byTooltip('Runs'), findsOneWidget);
    await tester.tap(find.byTooltip('Runs'));
    expect(selectedPage, 1);
    await tester.tap(find.text('Retry connection'));
    expect(retries, 1);
    expect(tester.takeException(), isNull);
  });
}
