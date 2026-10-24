import 'package:agentpulse/features/processes/process_page.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  testWidgets('validates process monitoring roots before saving', (
    tester,
  ) async {
    _useLargeViewport(tester);
    var saves = 0;

    await tester.pumpWidget(
      MaterialApp(
        home: Scaffold(
          body: SingleChildScrollView(
            child: ProcessPage(
              configurationOnly: true,
              config: const {},
              status: const {'enabled': false, 'state': 'disabled'},
              processes: const [],
              nextCursor: null,
              loading: false,
              loadingMore: false,
              error: null,
              onSave: (_) => saves++,
              onRefresh: () {},
              onLoadMore: () {},
            ),
          ),
        ),
      ),
    );

    expect(
      tester.widgetList<SwitchListTile>(find.byType(SwitchListTile)).length,
      1,
    );
    await tester.tap(find.text('保存设置'));
    await tester.pump();

    expect(find.text('至少填写一个根进程名称。'), findsOneWidget);
    expect(saves, 0);
    expect(tester.takeException(), isNull);
  });

  testWidgets('saves process roots and exact-name category rules', (
    tester,
  ) async {
    _useLargeViewport(tester);
    Map<String, dynamic>? saved;

    await tester.pumpWidget(
      MaterialApp(
        home: Scaffold(
          body: SingleChildScrollView(
            child: ProcessPage(
              configurationOnly: true,
              config: const {},
              status: const {'enabled': false, 'state': 'disabled'},
              processes: const [],
              nextCursor: null,
              loading: false,
              loadingMore: false,
              error: null,
              onSave: (value) => saved = value,
              onRefresh: () {},
              onLoadMore: () {},
            ),
          ),
        ),
      ),
    );

    await tester.enterText(find.byType(TextField).first, 'codex.exe');
    await tester.enterText(
      find.byType(TextField).last,
      'shell=pwsh.exe,cmd.exe\nbuild=msbuild.exe',
    );
    await tester.tap(find.text('保存设置'));
    await tester.pump();

    expect(saved, {
      'enabled': false,
      'capture_executable_paths': false,
      'capture_command_lines': false,
      'sample_interval_ms': 500,
      'process_retention_days': 30,
      'root_process_names': ['codex.exe'],
      'category_rules': [
        {
          'category': 'shell',
          'process_names': ['pwsh.exe', 'cmd.exe'],
        },
        {
          'category': 'build',
          'process_names': ['msbuild.exe'],
        },
      ],
    });
    expect(find.textContaining('只填写文件名，不记录路径或命令行参数'), findsOneWidget);
    expect(find.textContaining('Save privacy settings'), findsNothing);
    expect(tester.takeException(), isNull);
  });

  testWidgets('persists the process metadata privacy switches', (tester) async {
    _useLargeViewport(tester);
    Map<String, dynamic>? saved;

    await tester.pumpWidget(
      MaterialApp(
        home: Scaffold(
          body: SingleChildScrollView(
            child: ProcessPage(
              privacyOnly: true,
              config: const {
                'enabled': true,
                'root_process_names': ['codex.exe'],
                'category_rules': [],
              },
              status: const {'enabled': true, 'state': 'sampling'},
              processes: const [],
              nextCursor: null,
              loading: false,
              loadingMore: false,
              error: null,
              onSave: (value) => saved = value,
              onRefresh: () {},
              onLoadMore: () {},
            ),
          ),
        ),
      ),
    );

    await tester.tap(
      find.widgetWithText(SwitchListTile, 'Capture executable path'),
    );
    await tester.tap(
      find.widgetWithText(SwitchListTile, 'Capture command line'),
    );
    await tester.tap(find.text('Save privacy settings'));
    await tester.pump();

    expect(saved?['capture_executable_paths'], isTrue);
    expect(saved?['capture_command_lines'], isTrue);
    expect(saved?['enabled'], isTrue);
    expect(saved?['root_process_names'], ['codex.exe']);
    expect(tester.takeException(), isNull);
  });
}

void _useLargeViewport(WidgetTester tester) {
  tester.view.physicalSize = const Size(1200, 1400);
  tester.view.devicePixelRatio = 1;
  addTearDown(tester.view.resetPhysicalSize);
  addTearDown(tester.view.resetDevicePixelRatio);
}
