import 'package:flutter/material.dart';

import '../../ui/components/ap_components.dart';
import '../../ui/theme/ap_colors.dart';
import '../../ui/theme/ap_spacing.dart';
import '../../ui/theme/ap_typography.dart';

String formatSettingsBytes(dynamic raw) {
  final bytes = raw is num ? raw.toDouble() : 0.0;
  if (bytes < 1024) return '${bytes.toInt()} B';
  if (bytes < 1024 * 1024) return '${(bytes / 1024).toStringAsFixed(1)} KB';
  if (bytes < 1024 * 1024 * 1024) {
    return '${(bytes / (1024 * 1024)).toStringAsFixed(1)} MB';
  }
  return '${(bytes / (1024 * 1024 * 1024)).toStringAsFixed(2)} GB';
}

class SettingsPage extends StatelessWidget {
  const SettingsPage({
    required this.signals,
    required this.status,
    required this.backendVersion,
    required this.config,
    required this.preview,
    required this.selectedBackup,
    required this.retention,
    required this.retentionDays,
    required this.receiverPort,
    required this.receiverSettingsError,
    required this.loadingReceiverSettings,
    required this.savingReceiverSettings,
    required this.retentionError,
    required this.loadingRetention,
    required this.savingRetention,
    required this.requestingCleanup,
    required this.exportingData,
    required this.clearingHistory,
    required this.exportingDiagnostics,
    required this.cleaningLogs,
    required this.apiAvailable,
    required this.onSelectedBackupChanged,
    required this.onRetentionDaysChanged,
    required this.onReceiverPortChanged,
    required this.onSaveReceiverSettings,
    required this.onLoadReceiverSettings,
    required this.onLoadConfig,
    required this.onLoadPreview,
    required this.onApplyConfig,
    required this.onRestoreConfig,
    required this.onSaveRetention,
    required this.onRequestCleanup,
    required this.onExportData,
    required this.onClearHistory,
    required this.onExportDiagnostics,
    required this.onCleanupLogs,
    this.selectedSection = 0,
    this.onSectionChanged,
    this.processSettingsPage,
    this.privacySettingsPage,
    super.key,
  });

  final Map<String, dynamic> signals;
  final Map<String, dynamic> status;
  final String backendVersion;
  final Map<String, dynamic> config;
  final Map<String, dynamic> preview;
  final String? selectedBackup;
  final Map<String, dynamic> retention;
  final int retentionDays;
  final int receiverPort;
  final String? receiverSettingsError;
  final bool loadingReceiverSettings;
  final bool savingReceiverSettings;
  final String? retentionError;
  final bool loadingRetention;
  final bool savingRetention;
  final bool requestingCleanup;
  final bool exportingData;
  final bool clearingHistory;
  final bool exportingDiagnostics;
  final bool cleaningLogs;
  final bool apiAvailable;
  final ValueChanged<String?> onSelectedBackupChanged;
  final ValueChanged<int> onRetentionDaysChanged;
  final ValueChanged<int> onReceiverPortChanged;
  final VoidCallback onSaveReceiverSettings;
  final VoidCallback onLoadReceiverSettings;
  final VoidCallback onLoadConfig;
  final VoidCallback onLoadPreview;
  final VoidCallback onApplyConfig;
  final VoidCallback onRestoreConfig;
  final VoidCallback onSaveRetention;
  final VoidCallback onRequestCleanup;
  final VoidCallback onExportData;
  final VoidCallback onClearHistory;
  final VoidCallback onExportDiagnostics;
  final VoidCallback onCleanupLogs;
  final int selectedSection;
  final ValueChanged<int>? onSectionChanged;
  final Widget? processSettingsPage;
  final Widget? privacySettingsPage;

  @override
  Widget build(BuildContext context) {
    final section = selectedSection.clamp(0, 5).toInt();
    const labels = [
      'Codex',
      'Receiver',
      'Process Monitoring',
      'Privacy',
      'Data Retention',
      'Diagnostics & Export',
    ];
    const icons = [
      Icons.smart_toy_outlined,
      Icons.sensors_outlined,
      Icons.account_tree_outlined,
      Icons.privacy_tip_outlined,
      Icons.storage_outlined,
      Icons.medical_information_outlined,
    ];
    final titles = [
      'Codex connection',
      'OTLP receiver',
      'Process monitoring',
      'Privacy controls',
      'Data retention',
      'Diagnostics & export',
    ];
    final descriptions = [
      'Configure the local Codex telemetry connection.',
      'Set the loopback receiver endpoint used by Codex.',
      'Choose the process tree and sampling policy to capture.',
      'Review which process metadata is persisted locally.',
      'Manage local database retention and exports.',
      'Inspect backend health and export a sanitized support bundle.',
    ];
    Widget content = switch (section) {
      0 => _CodexConfigurationCard(
        signals: signals,
        status: status,
        config: config,
        preview: preview,
        selectedBackup: selectedBackup,
        onSelectedBackupChanged: onSelectedBackupChanged,
        onLoadConfig: onLoadConfig,
        onLoadPreview: onLoadPreview,
        onApplyConfig: onApplyConfig,
        onRestoreConfig: onRestoreConfig,
      ),
      1 => _ReceiverSettingsCard(
        port: receiverPort,
        error: receiverSettingsError,
        loading: loadingReceiverSettings,
        saving: savingReceiverSettings,
        apiAvailable: apiAvailable,
        onPortChanged: onReceiverPortChanged,
        onSave: onSaveReceiverSettings,
        onRefresh: onLoadReceiverSettings,
      ),
      2 =>
        processSettingsPage ??
            const ApEmptyState(title: 'Process settings unavailable'),
      3 => Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          privacySettingsPage ?? const SizedBox.shrink(),
          const SizedBox(height: 12),
          const ApPanel(
            title: 'TELEMETRY PAYLOADS',
            child: Text(
              'Tool input and output payloads are not persisted or exposed by the local API. Tool call records contain metadata such as name, duration, status and correlation IDs.',
              style: ApTypography.body,
            ),
          ),
        ],
      ),
      4 => _RetentionCard(
        retention: retention,
        retentionDays: retentionDays,
        error: retentionError,
        loading: loadingRetention,
        saving: savingRetention,
        requestingCleanup: requestingCleanup,
        exportingData: exportingData,
        clearingHistory: clearingHistory,
        onDaysChanged: onRetentionDaysChanged,
        onSave: onSaveRetention,
        onCleanup: onRequestCleanup,
        onExport: onExportData,
        onClear: onClearHistory,
      ),
      _ => _DiagnosticsCard(
        status: status,
        backendVersion: backendVersion,
        apiAvailable: apiAvailable,
        exporting: exportingDiagnostics,
        cleaning: cleaningLogs,
        onExport: onExportDiagnostics,
        onCleanup: onCleanupLogs,
      ),
    };
    return Row(
      children: [
        Container(
          width: 192,
          color: ApColors.surface0,
          child: ListView(
            padding: const EdgeInsets.symmetric(vertical: 12, horizontal: 10),
            children: [
              for (var i = 0; i < labels.length; i++)
                Padding(
                  padding: const EdgeInsets.symmetric(vertical: 2),
                  child: Material(
                    color: section == i
                        ? ApColors.accent.withValues(alpha: .13)
                        : Colors.transparent,
                    borderRadius: BorderRadius.circular(6),
                    child: InkWell(
                      onTap: () => onSectionChanged?.call(i),
                      borderRadius: BorderRadius.circular(6),
                      child: Padding(
                        padding: const EdgeInsets.symmetric(
                          horizontal: 10,
                          vertical: 9,
                        ),
                        child: Row(
                          children: [
                            Icon(
                              icons[i],
                              size: 16,
                              color: section == i
                                  ? ApColors.accentStrong
                                  : ApColors.textMuted,
                            ),
                            const SizedBox(width: 9),
                            Expanded(
                              child: Text(
                                labels[i],
                                style: ApTypography.small.copyWith(
                                  color: section == i
                                      ? ApColors.textPrimary
                                      : ApColors.textSecondary,
                                ),
                              ),
                            ),
                          ],
                        ),
                      ),
                    ),
                  ),
                ),
            ],
          ),
        ),
        const VerticalDivider(width: 1),
        Expanded(
          child: ListView(
            padding: const EdgeInsets.all(ApSpacing.page),
            children: [
              Text(titles[section], style: ApTypography.pageTitle),
              const SizedBox(height: 4),
              Text(descriptions[section], style: ApTypography.small),
              const SizedBox(height: 16),
              content,
            ],
          ),
        ),
      ],
    );
  }
}

class _CodexConfigurationCard extends StatelessWidget {
  const _CodexConfigurationCard({
    required this.signals,
    required this.status,
    required this.config,
    required this.preview,
    required this.selectedBackup,
    required this.onSelectedBackupChanged,
    required this.onLoadConfig,
    required this.onLoadPreview,
    required this.onApplyConfig,
    required this.onRestoreConfig,
  });

  final Map<String, dynamic> signals;
  final Map<String, dynamic> status;
  final Map<String, dynamic> config;
  final Map<String, dynamic> preview;
  final String? selectedBackup;
  final ValueChanged<String?> onSelectedBackupChanged;
  final VoidCallback onLoadConfig;
  final VoidCallback onLoadPreview;
  final VoidCallback onApplyConfig;
  final VoidCallback onRestoreConfig;

  @override
  Widget build(BuildContext context) {
    final receiving = signals.values.any(
      (value) => value is Map && value['state'] == 'receiving',
    );
    final path = config['path'] as String? ?? '点击“检测配置”查找 Codex 配置文件';
    final receiverAddress = status['receiver_address'] as String?;
    final backups = (config['backups'] as List?)?.cast<String>() ?? [];
    return Card(
      elevation: 0,
      color: ApColors.surface1,
      child: Padding(
        padding: const EdgeInsets.all(20),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            const Text(
              '用户配置文件',
              style: TextStyle(fontWeight: FontWeight.w600, fontSize: 16),
            ),
            const SizedBox(height: 8),
            SelectableText(path),
            const SizedBox(height: 18),
            const Text('接收地址'),
            const SizedBox(height: 4),
            SelectableText(
              receiverAddress == null || receiverAddress.isEmpty
                  ? 'http://127.0.0.1:4318'
                  : 'http://$receiverAddress',
            ),
            const SizedBox(height: 18),
            _SettingsNotice(
              receiving ? '已收到 Codex 遥测。' : '配置写入后，请重新启动 Codex；收到真实数据后才会显示已连接。',
            ),
            const SizedBox(height: 16),
            Wrap(
              spacing: 10,
              runSpacing: 8,
              children: [
                OutlinedButton.icon(
                  onPressed: onLoadConfig,
                  icon: const Icon(Icons.search),
                  label: const Text('检测配置'),
                ),
                FilledButton.icon(
                  onPressed: onLoadPreview,
                  icon: const Icon(Icons.preview_outlined),
                  label: const Text('预览并配置'),
                ),
              ],
            ),
            if (preview.isNotEmpty) ...[
              const SizedBox(height: 20),
              const Divider(),
              const Text(
                '待应用的变更',
                style: TextStyle(fontWeight: FontWeight.w600),
              ),
              const SizedBox(height: 8),
              for (final entry
                  in (preview['changes'] as Map<String, dynamic>? ?? {})
                      .entries)
                Padding(
                  padding: const EdgeInsets.symmetric(vertical: 3),
                  child: Text('${entry.key}  →  ${entry.value}'),
                ),
              const SizedBox(height: 8),
              FilledButton(
                onPressed: onApplyConfig,
                child: const Text('备份并应用'),
              ),
              if (preview['restart_required'] == true)
                Padding(
                  padding: const EdgeInsets.only(top: 10),
                  child: Text(
                    '应用后请重新启动 Codex。',
                    style: const TextStyle(color: ApColors.textMuted),
                  ),
                ),
            ],
            if (backups.isNotEmpty) ...[
              const SizedBox(height: 24),
              const Divider(),
              const Text(
                '恢复配置备份',
                style: TextStyle(fontWeight: FontWeight.w600),
              ),
              const SizedBox(height: 10),
              Row(
                children: [
                  Expanded(
                    child: DropdownButtonFormField<String>(
                      initialValue: backups.contains(selectedBackup)
                          ? selectedBackup
                          : null,
                      decoration: const InputDecoration(
                        labelText: '选择备份',
                        border: OutlineInputBorder(),
                      ),
                      items: backups
                          .map(
                            (name) => DropdownMenuItem(
                              value: name,
                              child: Text(name),
                            ),
                          )
                          .toList(),
                      onChanged: onSelectedBackupChanged,
                    ),
                  ),
                  const SizedBox(width: 12),
                  OutlinedButton(
                    onPressed: selectedBackup == null ? null : onRestoreConfig,
                    child: const Text('恢复'),
                  ),
                ],
              ),
            ],
          ],
        ),
      ),
    );
  }
}

class _ReceiverSettingsCard extends StatefulWidget {
  const _ReceiverSettingsCard({
    required this.port,
    required this.error,
    required this.loading,
    required this.saving,
    required this.apiAvailable,
    required this.onPortChanged,
    required this.onSave,
    required this.onRefresh,
  });

  final int port;
  final String? error;
  final bool loading;
  final bool saving;
  final bool apiAvailable;
  final ValueChanged<int> onPortChanged;
  final VoidCallback onSave;
  final VoidCallback onRefresh;

  @override
  State<_ReceiverSettingsCard> createState() => _ReceiverSettingsCardState();
}

class _ReceiverSettingsCardState extends State<_ReceiverSettingsCard> {
  late final TextEditingController _portController = TextEditingController(
    text: widget.port.toString(),
  );

  @override
  void didUpdateWidget(covariant _ReceiverSettingsCard oldWidget) {
    super.didUpdateWidget(oldWidget);
    if (widget.port != oldWidget.port &&
        int.tryParse(_portController.text) != widget.port) {
      _portController.text = widget.port.toString();
    }
  }

  @override
  void dispose() {
    _portController.dispose();
    super.dispose();
  }

  int? get _validPort {
    final port = int.tryParse(_portController.text.trim());
    if (port == null || port < 1024 || port > 65535) return null;
    return port;
  }

  @override
  Widget build(BuildContext context) {
    final port = _validPort;
    return Card(
      elevation: 0,
      color: ApColors.surface1,
      child: Padding(
        padding: const EdgeInsets.all(20),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            const Text(
              '本地接收器',
              style: TextStyle(fontWeight: FontWeight.w600, fontSize: 16),
            ),
            const SizedBox(height: 6),
            Text(
              '默认端口为 4318。更改后会短暂重启接收器；如果 Codex 已接入，请重新预览并应用新地址，然后重启 Codex。',
              style: const TextStyle(color: ApColors.textMuted),
            ),
            if (widget.loading) ...[
              const SizedBox(height: 14),
              const LinearProgressIndicator(),
            ],
            const SizedBox(height: 16),
            Wrap(
              spacing: 12,
              runSpacing: 10,
              crossAxisAlignment: WrapCrossAlignment.center,
              children: [
                SizedBox(
                  width: 220,
                  child: TextField(
                    controller: _portController,
                    enabled:
                        widget.apiAvailable &&
                        !widget.loading &&
                        !widget.saving,
                    keyboardType: TextInputType.number,
                    decoration: InputDecoration(
                      labelText: 'OTLP/HTTP 端口',
                      border: const OutlineInputBorder(),
                      errorText: port == null ? '请输入 1024–65535 之间的端口' : null,
                    ),
                    onChanged: (value) {
                      final parsed = int.tryParse(value.trim());
                      if (parsed != null) widget.onPortChanged(parsed);
                      setState(() {});
                    },
                  ),
                ),
                FilledButton.icon(
                  onPressed:
                      !widget.apiAvailable ||
                          widget.loading ||
                          widget.saving ||
                          port == null
                      ? null
                      : widget.onSave,
                  icon: const Icon(Icons.restart_alt),
                  label: Text(widget.saving ? '保存并重启中…' : '保存并重启接收器'),
                ),
                OutlinedButton(
                  onPressed: !widget.apiAvailable || widget.loading
                      ? null
                      : widget.onRefresh,
                  child: const Text('重新读取'),
                ),
              ],
            ),
            if (widget.error != null) ...[
              const SizedBox(height: 10),
              _SettingsNotice(widget.error!, isError: true),
            ],
          ],
        ),
      ),
    );
  }
}

class _RetentionCard extends StatelessWidget {
  const _RetentionCard({
    required this.retention,
    required this.retentionDays,
    required this.error,
    required this.loading,
    required this.saving,
    required this.requestingCleanup,
    required this.exportingData,
    required this.clearingHistory,
    required this.onDaysChanged,
    required this.onSave,
    required this.onCleanup,
    required this.onExport,
    required this.onClear,
  });

  final Map<String, dynamic> retention;
  final int retentionDays;
  final String? error;
  final bool loading;
  final bool saving;
  final bool requestingCleanup;
  final bool exportingData;
  final bool clearingHistory;
  final ValueChanged<int> onDaysChanged;
  final VoidCallback onSave;
  final VoidCallback onCleanup;
  final VoidCallback onExport;
  final VoidCallback onClear;

  String get _statusText {
    final status = (retention['status'] as Map?)?.cast<String, dynamic>() ?? {};
    if (status['running'] == true) return '清理任务正在后台执行。';
    final error = status['last_error'] as String?;
    if (error != null && error.isNotEmpty) return '上次清理未完成：$error';
    final rawTime = status['last_cleanup_at_ns'];
    if (rawTime is num && rawTime > 0) {
      final date = DateTime.fromMicrosecondsSinceEpoch((rawTime / 1000).round())
          .toLocal();
      final deleted = status['last_deleted_rows'] ?? 0;
      return '上次清理：${date.year}-${date.month.toString().padLeft(2, '0')}-${date.day.toString().padLeft(2, '0')} ${date.hour.toString().padLeft(2, '0')}:${date.minute.toString().padLeft(2, '0')}，清理 $deleted 条记录。';
    }
    return '应用启动后会自动检查过期数据。';
  }

  @override
  Widget build(BuildContext context) {
    final days = <int>{1, 7, 14, 30, 60, 90, 180, 365, retentionDays}.toList()
      ..sort();
    return Card(
      elevation: 0,
      color: ApColors.surface1,
      child: Padding(
        padding: const EdgeInsets.all(20),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            const Text(
              '遥测历史数据',
              style: TextStyle(fontWeight: FontWeight.w600, fontSize: 16),
            ),
            const SizedBox(height: 6),
            Text(
              '自动保留最近一段时间的遥测记录，进程历史使用独立设置。',
              style: const TextStyle(color: ApColors.textMuted),
            ),
            if (loading && retention.isEmpty) ...[
              const SizedBox(height: 16),
              const LinearProgressIndicator(),
            ],
            const SizedBox(height: 16),
            Wrap(
              spacing: 12,
              runSpacing: 10,
              crossAxisAlignment: WrapCrossAlignment.center,
              children: [
                SizedBox(
                  width: 190,
                  child: DropdownButtonFormField<int>(
                    initialValue: retentionDays,
                    decoration: const InputDecoration(
                      labelText: '保留时间',
                      border: OutlineInputBorder(),
                    ),
                    items: days
                        .map(
                          (day) => DropdownMenuItem<int>(
                            value: day,
                            child: Text('$day 天'),
                          ),
                        )
                        .toList(),
                    onChanged: saving
                        ? null
                        : (value) {
                            if (value != null) onDaysChanged(value);
                          },
                  ),
                ),
                FilledButton(
                  onPressed: saving ? null : onSave,
                  child: Text(saving ? '保存中…' : '保存设置'),
                ),
                OutlinedButton.icon(
                  onPressed: requestingCleanup ? null : onCleanup,
                  icon: const Icon(Icons.cleaning_services_outlined),
                  label: Text(requestingCleanup ? '正在提交…' : '立即清理过期数据'),
                ),
                OutlinedButton.icon(
                  onPressed: exportingData ? null : onExport,
                  icon: const Icon(Icons.save_alt_outlined),
                  label: Text(exportingData ? '正在导出…' : '导出本地数据'),
                ),
                OutlinedButton.icon(
                  onPressed: clearingHistory ? null : onClear,
                  icon: const Icon(Icons.delete_sweep_outlined),
                  label: Text(clearingHistory ? '正在清空…' : '清空全部历史'),
                ),
              ],
            ),
            const SizedBox(height: 14),
            Text(
              '数据库占用：${formatSettingsBytes(retention['database_size_bytes'])}',
            ),
            const SizedBox(height: 4),
            Text(_statusText),
            if (error != null) ...[
              const SizedBox(height: 10),
              _SettingsNotice(error!, isError: true),
            ],
          ],
        ),
      ),
    );
  }
}

class _DiagnosticsCard extends StatelessWidget {
  const _DiagnosticsCard({
    required this.status,
    required this.backendVersion,
    required this.apiAvailable,
    required this.exporting,
    required this.cleaning,
    required this.onExport,
    required this.onCleanup,
  });

  final Map<String, dynamic> status;
  final String backendVersion;
  final bool apiAvailable;
  final bool exporting;
  final bool cleaning;
  final VoidCallback onExport;
  final VoidCallback onCleanup;

  @override
  Widget build(BuildContext context) => Card(
    elevation: 0,
    color: ApColors.surface1,
    child: Padding(
      padding: const EdgeInsets.all(20),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          const Text(
            '诊断与版本',
            style: TextStyle(fontWeight: FontWeight.w600, fontSize: 16),
          ),
          const SizedBox(height: 8),
          Text(
            '后端版本：$backendVersion · 数据库结构：v${status['schema_version'] ?? '—'}',
          ),
          const SizedBox(height: 6),
          Text(
            '诊断包只包含版本、信号状态、队列统计和脱敏错误代码，不包含遥测内容、进程详情或 Codex 配置。',
            style: const TextStyle(color: ApColors.textMuted),
          ),
          const SizedBox(height: 6),
          Text(
            '本地日志最多保留 3 个轮换文件，每个不超过 5 MB；14 天前的日志会自动清理。',
            style: const TextStyle(color: ApColors.textMuted),
          ),
          const SizedBox(height: 14),
          Wrap(
            spacing: 10,
            runSpacing: 8,
            children: [
              OutlinedButton.icon(
                onPressed: !apiAvailable || exporting ? null : onExport,
                icon: const Icon(Icons.health_and_safety_outlined),
                label: Text(exporting ? '正在生成…' : '导出脱敏诊断包'),
              ),
              OutlinedButton.icon(
                onPressed: !apiAvailable || cleaning ? null : onCleanup,
                icon: const Icon(Icons.delete_outline),
                label: Text(cleaning ? '正在清理…' : '清理过期日志'),
              ),
            ],
          ),
        ],
      ),
    ),
  );
}

class _SettingsNotice extends StatelessWidget {
  const _SettingsNotice(this.message, {this.isError = false});

  final String message;
  final bool isError;

  @override
  Widget build(BuildContext context) => Container(
    padding: const EdgeInsets.all(14),
    decoration: BoxDecoration(
      color: isError
          ? ApColors.danger.withValues(alpha: .1)
          : ApColors.accent.withValues(alpha: .1),
      borderRadius: BorderRadius.circular(10),
    ),
    child: Text(
      message,
      style: TextStyle(
        color: isError ? ApColors.danger : ApColors.accentStrong,
      ),
    ),
  );
}
