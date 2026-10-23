import 'package:agentpulse/features/settings/settings_page.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  testWidgets('exposes direct Codex configuration and retention actions', (
    tester,
  ) async {
    tester.view.physicalSize = const Size(1200, 1400);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);
    var detected = 0;
    var previewed = 0;
    var applied = 0;
    var restored = 0;
    var savedRetention = 0;
    var savedReceiver = 0;
    int? selectedDays;
    int? selectedPort;

    await tester.pumpWidget(
      MaterialApp(
        home: Scaffold(
          body: SettingsPage(
            signals: {
              'logs': {'state': 'receiving'},
            },
            status: const {},
            backendVersion: 'test',
            config: {
              'path': r'C:\Users\test\.codex\config.toml',
              'backups': ['config.toml.backup'],
            },
            preview: {
              'changes': {
                'OTEL_EXPORTER_OTLP_ENDPOINT': 'http://127.0.0.1:4318',
              },
              'restart_required': true,
            },
            selectedBackup: 'config.toml.backup',
            retention: {
              'database_size_bytes': 1024,
              'status': const <String, dynamic>{},
            },
            retentionDays: 30,
            receiverPort: 4318,
            receiverSettingsError: null,
            loadingReceiverSettings: false,
            savingReceiverSettings: false,
            retentionError: null,
            loadingRetention: false,
            savingRetention: false,
            requestingCleanup: false,
            exportingData: false,
            clearingHistory: false,
            exportingDiagnostics: false,
            cleaningLogs: false,
            apiAvailable: true,
            onSelectedBackupChanged: (_) {},
            onRetentionDaysChanged: (days) => selectedDays = days,
            onReceiverPortChanged: (port) => selectedPort = port,
            onSaveReceiverSettings: () => savedReceiver++,
            onLoadReceiverSettings: () {},
            onLoadConfig: () => detected++,
            onLoadPreview: () => previewed++,
            onApplyConfig: () => applied++,
            onRestoreConfig: () => restored++,
            onSaveRetention: () => savedRetention++,
            onRequestCleanup: () {},
            onExportData: () {},
            onClearHistory: () {},
            onExportDiagnostics: () {},
            onCleanupLogs: () {},
          ),
        ),
      ),
    );

    await tester.tap(find.text('检测配置'));
    await tester.tap(find.text('预览并配置'));
    await tester.tap(find.text('备份并应用'));
    await tester.tap(find.text('恢复'));

    expect(detected, 1);
    expect(previewed, 1);
    expect(applied, 1);
    expect(restored, 1);
    expect(find.textContaining('OTEL_EXPORTER_OTLP_ENDPOINT'), findsOneWidget);
    expect(find.text('本地接收器'), findsOneWidget);
    expect(find.text('保存并重启接收器'), findsOneWidget);
    await tester.enterText(find.byType(TextField), '4545');
    expect(selectedPort, 4545);
    await tester.tap(find.text('保存并重启接收器'));
    expect(savedReceiver, 1);
    await tester.enterText(find.byType(TextField), '80');
    await tester.pumpAndSettle();
    expect(find.text('请输入 1024–65535 之间的端口'), findsOneWidget);

    await tester.tap(find.text('保存设置'));
    expect(savedRetention, 1);

    final retentionDropdown = find.byType(DropdownButtonFormField<int>).last;
    await tester.tap(retentionDropdown);
    await tester.pumpAndSettle();
    await tester.tap(find.text('7 天').last);

    expect(selectedDays, 7);
  });
}
