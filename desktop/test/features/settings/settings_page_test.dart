import 'package:agentpulse/features/settings/settings_page.dart';
import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  testWidgets('exposes Codex configuration and backup actions', (tester) async {
    _useLargeViewport(tester);
    var detected = 0;
    var previewed = 0;
    var applied = 0;
    var restored = 0;

    await tester.pumpWidget(
      MaterialApp(
        home: Scaffold(
          body: _settings(
            onLoadConfig: () => detected++,
            onLoadPreview: () => previewed++,
            onApplyConfig: () => applied++,
            onRestoreConfig: () => restored++,
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
    expect(tester.takeException(), isNull);
  });

  testWidgets('navigates to receiver and data retention settings', (
    tester,
  ) async {
    _useLargeViewport(tester);
    var selectedSection = 1;
    var savedReceiver = 0;
    var savedRetention = 0;
    int? selectedDays;
    int? selectedPort;

    await tester.pumpWidget(
      MaterialApp(
        home: Scaffold(
          body: StatefulBuilder(
            builder: (context, setState) => _settings(
              selectedSection: selectedSection,
              onSectionChanged: (section) =>
                  setState(() => selectedSection = section),
              onReceiverPortChanged: (port) => selectedPort = port,
              onSaveReceiverSettings: () => savedReceiver++,
              onRetentionDaysChanged: (days) => selectedDays = days,
              onSaveRetention: () => savedRetention++,
            ),
          ),
        ),
      ),
    );

    expect(find.text('本地接收器'), findsOneWidget);
    await tester.enterText(find.byType(TextField).first, '4545');
    expect(selectedPort, 4545);
    await tester.tap(find.text('保存并重启接收器'));
    expect(savedReceiver, 1);
    await tester.enterText(find.byType(TextField).first, '80');
    await tester.pumpAndSettle();
    expect(find.text('请输入 1024–65535 之间的端口'), findsOneWidget);

    await tester.tap(find.text('Data Retention'));
    await tester.pumpAndSettle();
    await tester.tap(find.text('保存设置'));
    expect(savedRetention, 1);

    await tester.tap(find.byType(DropdownButtonFormField<int>).last);
    await tester.pumpAndSettle();
    await tester.tap(find.text('7 天').last);

    expect(selectedDays, 7);
    expect(tester.takeException(), isNull);
  });
}

SettingsPage _settings({
  int selectedSection = 0,
  ValueChanged<int>? onSectionChanged,
  VoidCallback? onLoadConfig,
  VoidCallback? onLoadPreview,
  VoidCallback? onApplyConfig,
  VoidCallback? onRestoreConfig,
  ValueChanged<int>? onRetentionDaysChanged,
  ValueChanged<int>? onReceiverPortChanged,
  VoidCallback? onSaveReceiverSettings,
  VoidCallback? onSaveRetention,
}) => SettingsPage(
  selectedSection: selectedSection,
  onSectionChanged: onSectionChanged,
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
    'changes': {'OTEL_EXPORTER_OTLP_ENDPOINT': 'http://127.0.0.1:4318'},
    'restart_required': true,
  },
  selectedBackup: 'config.toml.backup',
  retention: {'database_size_bytes': 1024, 'status': const <String, dynamic>{}},
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
  onRetentionDaysChanged: onRetentionDaysChanged ?? (_) {},
  onReceiverPortChanged: onReceiverPortChanged ?? (_) {},
  onSaveReceiverSettings: onSaveReceiverSettings ?? () {},
  onLoadReceiverSettings: () {},
  onLoadConfig: onLoadConfig ?? () {},
  onLoadPreview: onLoadPreview ?? () {},
  onApplyConfig: onApplyConfig ?? () {},
  onRestoreConfig: onRestoreConfig ?? () {},
  onSaveRetention: onSaveRetention ?? () {},
  onRequestCleanup: () {},
  onExportData: () {},
  onClearHistory: () {},
  onExportDiagnostics: () {},
  onCleanupLogs: () {},
);

void _useLargeViewport(WidgetTester tester) {
  tester.view.physicalSize = const Size(1200, 1400);
  tester.view.devicePixelRatio = 1;
  addTearDown(tester.view.resetPhysicalSize);
  addTearDown(tester.view.resetDevicePixelRatio);
}
