import 'package:agentpulse/features/processes/process_page.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  testWidgets('explains the disabled monitor and validates required roots', (
    tester,
  ) async {
    tester.view.physicalSize = const Size(1200, 1400);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);
    var saves = 0;

    await tester.pumpWidget(
      MaterialApp(
        home: Scaffold(
          body: ProcessPage(
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
    );

    expect(find.text('进程监控已关闭'), findsOneWidget);
    expect(find.text('启用监控并保存后，进程快照会显示在这里。'), findsOneWidget);
    expect(
      tester
          .widgetList<SwitchListTile>(find.byType(SwitchListTile))
          .map((tile) => tile.value),
      [false, false, false],
    );
    await tester.tap(find.text('保存设置'));
    await tester.pump();

    expect(find.text('至少填写一个根进程名称。'), findsOneWidget);
    expect(saves, 0);
    expect(tester.takeException(), isNull);
  });

  testWidgets('saves root process and exact-name category rules', (
    tester,
  ) async {
    tester.view.physicalSize = const Size(1200, 1400);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);
    Map<String, dynamic>? saved;

    await tester.pumpWidget(
      MaterialApp(
        home: Scaffold(
          body: ProcessPage(
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
    );

    await tester.enterText(find.byType(TextField).first, 'codex.exe');
    await tester.enterText(
      find.byType(TextField).last,
      'shell=pwsh.exe,cmd.exe\nbuild=msbuild.exe',
    );
    await tester.tap(find.widgetWithText(SwitchListTile, '保存可执行文件路径'));
    await tester.tap(find.widgetWithText(SwitchListTile, '保存命令行参数'));
    await tester.tap(find.text('保存设置'));
    await tester.pump();

    expect(saved, {
      'enabled': false,
      'capture_executable_paths': true,
      'capture_command_lines': true,
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
    expect(find.textContaining('当前用户目录替换为 %USERPROFILE%'), findsOneWidget);
    expect(find.textContaining('其他参数仍可能含提示词或敏感内容'), findsOneWidget);
    expect(tester.takeException(), isNull);
  });
}
