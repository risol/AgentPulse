import 'dart:convert';
import 'dart:io';
import 'dart:typed_data';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:integration_test/integration_test.dart';
import 'package:agentpulse/main.dart' as agentpulse;

import 'support/fixed_workflow.dart';

void main() {
  IntegrationTestWidgetsFlutterBinding.ensureInitialized();

  testWidgets('local telemetry reaches the dashboard and feature pages', (
    tester,
  ) async {
    final address = Platform.environment['AGENTPULSE_E2E_OTLP_ADDRESS'];
    final resultPath = Platform.environment['AGENTPULSE_E2E_RESULT'];
    final processRootName =
        Platform.environment['AGENTPULSE_E2E_PROCESS_ROOT'] ?? 'codex.exe';
    expect(address, isNotNull);
    expect(resultPath, isNotNull);

    agentpulse.main();
    await tester.pump();
    await _pumpUntil(tester, () => _readLogCount(tester) == 0);
    expect(find.text('实时总览'), findsOneWidget);

    // Let the app establish its authenticated SSE subscription before sending.
    await tester.runAsync(
      () => Future<void>.delayed(const Duration(milliseconds: 600)),
    );
    await tester.pump();

    final workflowSessionID = 'e2e-${DateTime.now().millisecondsSinceEpoch}';

    final client = HttpClient();
    final latenciesMs = <int>[];
    const eventCount = 20;
    try {
      for (var index = 1; index <= eventCount; index++) {
        final stopwatch = Stopwatch()..start();
        await tester.runAsync(() => _sendLog(client, address!, index));
        await _pumpUntil(tester, () => _readLogCount(tester) == index);
        stopwatch.stop();
        latenciesMs.add(stopwatch.elapsedMilliseconds);
      }
    } finally {
      client.close(force: true);
    }

    final sortedLatencies = [...latenciesMs]..sort();
    final p95Index = (sortedLatencies.length * 0.95).ceil() - 1;
    final dashboardP95Ms = sortedLatencies[p95Index];
    final dashboardLogCount = _readLogCount(tester);
    expect(dashboardLogCount, eventCount);
    expect(dashboardP95Ms, lessThan(1000));
    await _captureE2eScreenshot(tester, 'dashboard');

    await tester.tap(find.text('进程').first);
    await _pumpUntil(tester, () => find.text('进程监控').evaluate().isNotEmpty);
    final switches = tester.widgetList<SwitchListTile>(
      find.byType(SwitchListTile),
    );
    expect(switches.length, greaterThanOrEqualTo(3));
    expect(switches.elementAt(0).value, isFalse);
    expect(switches.elementAt(1).value, isFalse);
    expect(switches.elementAt(2).value, isFalse);

    // Exercise process settings and keep sampling while the fixed workflow
    // runs. Paths and command lines remain disabled throughout.
    late final FixedWorkflowResult workflow;
    await _saveProcessMonitorSettings(
      tester,
      enabled: true,
      rootName: processRootName,
    );
    await _captureE2eScreenshot(tester, 'process-settings-details');
    try {
      await tester.runAsync(
        () => Future<void>.delayed(const Duration(milliseconds: 1500)),
      );
      final processPage = find.byType(ListView).last;
      await tester.drag(processPage, const Offset(0, 1200));
      await tester.pump(const Duration(milliseconds: 200));
      await _captureE2eScreenshot(tester, 'process-settings');
      await tester.tap(find.byTooltip('刷新'));
      final processPageScrollable = find
          .descendant(of: processPage, matching: find.byType(Scrollable))
          .first;
      await tester.scrollUntilVisible(
        find.text('正在采集进程快照'),
        200,
        scrollable: processPageScrollable,
      );
      await _pumpUntil(
        tester,
        () => find.text('正在采集进程快照').evaluate().isNotEmpty,
        timeout: const Duration(seconds: 20),
      );
      await _captureE2eScreenshot(tester, 'process-monitoring');

      final workflowResult = await tester.runAsync(
        () => runFixedWorkflow(
          address: address!,
          resultDirectory: File(resultPath!).parent.path,
          sessionID: workflowSessionID,
        ),
      );
      expect(workflowResult, isNotNull);
      workflow = workflowResult!;
      expect(workflow.exitCodes, [0, 17, 0]);

      // Give the 500 ms collector a final chance to observe process exits.
      await tester.runAsync(
        () => Future<void>.delayed(const Duration(milliseconds: 700)),
      );
    } finally {
      // Always turn monitoring back off before leaving the isolated app.
      await _saveProcessMonitorSettings(
        tester,
        enabled: false,
        rootName: processRootName,
      );
      final processPage = find.byType(ListView).last;
      final processPageScrollable = find
          .descendant(of: processPage, matching: find.byType(Scrollable))
          .first;
      await tester.scrollUntilVisible(
        find.text('进程监控已关闭'),
        200,
        scrollable: processPageScrollable,
      );
      await _pumpUntil(
        tester,
        () => find.text('进程监控已关闭').evaluate().isNotEmpty,
      );
    }

    await tester.tap(find.text('会话').first);
    await _pumpUntil(
      tester,
      () => find.text('会话 $workflowSessionID').evaluate().isNotEmpty,
    );
    await tester.tap(find.text('会话 $workflowSessionID'));
    await _pumpUntil(
      tester,
      () => find.text('会话详情 · $workflowSessionID').evaluate().isNotEmpty,
    );
    await tester.pumpAndSettle();
    final closeButton = find.text('关闭');
    expect(closeButton, findsOneWidget);
    final dialogViewport = MediaQuery.sizeOf(tester.element(closeButton));
    final closeButtonRect = tester.getRect(closeButton);
    expect(closeButtonRect.bottom, lessThanOrEqualTo(dialogViewport.height));
    expect(closeButtonRect.right, lessThanOrEqualTo(dialogViewport.width));
    await _captureE2eScreenshot(tester, 'session-detail');
    try {
      await _pumpUntil(
        tester,
        () =>
            find.text('工具 · exec_command').evaluate().length == 2 &&
            find.text('工具 · test_program').evaluate().length == 1,
        timeout: const Duration(seconds: 20),
      );
    } catch (error) {
      final visibleText = tester
          .widgetList<Text>(find.byType(Text))
          .map((widget) => widget.data)
          .whereType<String>()
          .join(' | ');
      debugPrint('E2E session detail diagnostic: $visibleText');
      rethrow;
    }
    final uiToolRowCount =
        find.text('工具 · exec_command').evaluate().length +
        find.text('工具 · test_program').evaluate().length;
    expect(uiToolRowCount, 3);

    final commandOutcomes = <String>{};
    for (final rowIndex in [0, 1]) {
      final rowLabel = find.text('工具 · exec_command').at(rowIndex);
      await tester.ensureVisible(rowLabel);
      await tester.tap(rowLabel);
      await tester.pump(const Duration(milliseconds: 100));
      final detailText = tester
          .widgetList<Text>(find.byType(Text))
          .map((widget) => widget.data)
          .whereType<String>();
      for (final value in detailText) {
        if (value.startsWith('结果：')) commandOutcomes.add(value.substring(3));
      }
    }
    expect(commandOutcomes, containsAll(['success', 'error']));
    expect(find.textContaining('PID：'), findsOneWidget);
    expect(find.text('重试序号：1'), findsOneWidget);

    await tester.tap(find.text('进程树'));
    await _pumpUntil(
      tester,
      () => find.text('powershell.exe').evaluate().isNotEmpty,
      timeout: const Duration(seconds: 20),
    );
    await _captureE2eScreenshot(tester, 'session-process-tree');
    expect(find.text('PID 明确关联 100%'), findsWidgets);
    await tester.tap(find.text('关闭'));
    await tester.pumpAndSettle();

    await tester.tap(find.byIcon(Icons.build_outlined));
    await _pumpUntil(tester, () => find.text('工具调用分析').evaluate().isNotEmpty);
    await _captureE2eScreenshot(tester, 'tool-analysis');

    await File(resultPath!).writeAsString(
      jsonEncode({
        'event_count': eventCount,
        'dashboard_log_count': dashboardLogCount,
        'telemetry_to_dashboard_p95_ms': dashboardP95Ms,
        'telemetry_to_dashboard_samples_ms': latenciesMs,
        'process_monitor_enabled_during_sample': true,
        'process_monitor_disabled_after_sample': true,
        'process_monitor_sampling_status_visible': true,
        'process_root_name': processRootName,
        'workflow_session_external_id': workflowSessionID,
        'workflow_span_ids': workflow.spanIDs,
        'workflow_tool_call_ids': workflow.toolCallIDs,
        'workflow_process_pids': workflow.processPids,
        'workflow_exit_codes': workflow.exitCodes,
        'workflow_ui_tool_rows': uiToolRowCount,
      }),
    );
  });
}

Future<void> _captureE2eScreenshot(WidgetTester tester, String name) async {
  final outputDirectory = Platform.environment['AGENTPULSE_E2E_SCREENSHOT_DIR'];
  final helperPath = Platform.environment['AGENTPULSE_E2E_SCREENSHOT_HELPER'];
  if (outputDirectory == null || helperPath == null) return;

  final result = await tester.runAsync(
    () => Process.run('powershell.exe', [
      '-NoLogo',
      '-NoProfile',
      '-NonInteractive',
      '-WindowStyle',
      'Hidden',
      '-File',
      helperPath,
      '-Name',
      name,
      '-OutputDirectory',
      outputDirectory,
    ]),
  );
  if (result == null || result.exitCode != 0) {
    throw StateError(
      'Could not capture the $name E2E screenshot: ${result?.stderr}',
    );
  }
  debugPrint('E2E screenshot: ${result.stdout}');
}

Future<void> _saveProcessMonitorSettings(
  WidgetTester tester, {
  required bool enabled,
  required String rootName,
}) async {
  final monitorSwitch = find.widgetWithText(SwitchListTile, '启用进程监控');
  final processPage = find.byType(ListView).last;
  final processPageScrollView = find
      .descendant(of: processPage, matching: find.byType(Scrollable))
      .first;
  await tester.drag(processPage, const Offset(0, 1200));
  await tester.pump(const Duration(milliseconds: 200));
  await tester.scrollUntilVisible(
    monitorSwitch,
    200,
    scrollable: processPageScrollView,
  );
  final current = tester.widget<SwitchListTile>(monitorSwitch).value;
  if (current != enabled) {
    await tester.tap(monitorSwitch);
    await tester.pump();
  }

  final rootProcessField = find.byType(TextField).first;
  await tester.scrollUntilVisible(
    rootProcessField,
    200,
    scrollable: processPageScrollView,
  );
  await tester.enterText(rootProcessField, rootName);
  await tester.scrollUntilVisible(
    find.text('保存设置'),
    200,
    scrollable: processPageScrollView,
  );
  await tester.tap(find.text('保存设置'));
  await tester.pump();
  await tester.runAsync(
    () => Future<void>.delayed(const Duration(milliseconds: 100)),
  );
}

Future<void> _pumpUntil(
  WidgetTester tester,
  bool Function() condition, {
  Duration timeout = const Duration(seconds: 30),
}) async {
  final deadline = DateTime.now().add(timeout);
  while (DateTime.now().isBefore(deadline)) {
    await tester.pump(const Duration(milliseconds: 20));
    if (condition()) return;
    await tester.runAsync(
      () => Future<void>.delayed(const Duration(milliseconds: 20)),
    );
  }
  fail('Timed out waiting for the Windows app UI to update.');
}

int? _readLogCount(WidgetTester tester) {
  final cards = find.ancestor(of: find.text('日志'), matching: find.byType(Card));
  if (cards.evaluate().isEmpty) return null;
  final textWidgets = tester.widgetList<Text>(
    find.descendant(of: cards.first, matching: find.byType(Text)),
  );
  for (final widget in textWidgets) {
    final count = int.tryParse(widget.data?.replaceAll(',', '') ?? '');
    if (count != null) return count;
  }
  return null;
}

Future<void> _sendLog(HttpClient client, String address, int index) async {
  final request = await client.postUrl(Uri.parse('http://$address/v1/logs'));
  request.headers.set(HttpHeaders.contentTypeHeader, 'application/x-protobuf');
  request.add(_logsRequest(index));
  final response = await request.close();
  await response.drain<void>();
  if (response.statusCode != HttpStatus.ok) {
    throw HttpException('OTLP log export returned ${response.statusCode}');
  }
}

List<int> _logsRequest(int index) {
  final timestamp = DateTime.now().microsecondsSinceEpoch * 1000;
  final timestampBytes = ByteData(8)..setUint64(0, timestamp, Endian.little);
  final body = _lengthDelimitedField(
    5,
    _lengthDelimitedField(1, utf8.encode('agentpulse-e2e-$index')),
  );
  final record = <int>[
    ..._varint((1 << 3) | 1),
    ...timestampBytes.buffer.asUint8List(),
    ...body,
  ];
  final scopeLogs = _lengthDelimitedField(2, record);
  final resourceLogs = _lengthDelimitedField(2, scopeLogs);
  return _lengthDelimitedField(1, resourceLogs);
}

List<int> _lengthDelimitedField(int field, List<int> bytes) => [
  ..._varint((field << 3) | 2),
  ..._varint(bytes.length),
  ...bytes,
];

List<int> _varint(int value) {
  var remaining = value;
  final bytes = <int>[];
  while (remaining >= 0x80) {
    bytes.add((remaining & 0x7f) | 0x80);
    remaining >>= 7;
  }
  bytes.add(remaining);
  return bytes;
}
