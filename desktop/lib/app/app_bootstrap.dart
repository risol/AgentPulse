import 'dart:async';

import 'package:flutter/material.dart';
import 'package:file_selector/file_selector.dart';

import '../core/api/local_api_client.dart';
import '../core/backend/backend_session.dart';
import '../features/dashboard/dashboard_controller.dart';
import '../features/dashboard/dashboard_refresh_coordinator.dart';
import '../features/diagnostics/diagnostics_page.dart';
import '../features/live/live_page.dart';
import '../features/processes/process_explorer_page.dart';
import '../features/processes/process_page.dart';
import '../features/runs/run_detail_page.dart';
import '../features/sessions/session_explorer_page.dart';
import '../features/settings/settings_controller.dart';
import '../features/settings/settings_page.dart';
import '../features/tools/tool_analytics_page.dart';
import '../ui/shell/app_shell.dart';

class AppBootstrap extends StatefulWidget {
  const AppBootstrap({super.key});

  @override
  State<AppBootstrap> createState() => _AppBootstrapState();
}

class _AppBootstrapState extends State<AppBootstrap> {
  final DashboardController _dashboard = DashboardController();
  final SettingsController _settings = SettingsController();
  BackendSession? _backendSession;
  LocalApiClient? _apiClient;
  String _backendVersion = 'development';
  String? _error;
  bool _starting = true;
  int _page = 0;
  int _settingsSection = 0;
  Map<String, dynamic>? _selectedRun;
  late final DashboardRefreshCoordinator _refreshCoordinator =
      DashboardRefreshCoordinator(
        loadData:
            ({
              required includeSessions,
              required includeProcesses,
              required includeTools,
              required includeTrends,
            }) => _dashboard.loadData(
              includeSessions: includeSessions,
              includeProcesses: includeProcesses,
              includeTools: includeTools,
              includeTrends: includeTrends,
            ),
        loadRetention: _settings.loadRetention,
        currentPage: () => _page,
      );

  Map<String, dynamic> get _status => _dashboard.status;
  Map<String, dynamic> get _summary => _dashboard.summary;
  Map<String, dynamic> get _processConfig => _dashboard.processConfig;
  List<Map<String, dynamic>> get _sessions => _dashboard.sessions;
  List<Map<String, dynamic>> get _toolSummaries => _dashboard.toolSummaries;
  List<Map<String, dynamic>> get _toolCalls => _dashboard.toolCalls;
  List<Map<String, dynamic>> get _processes => _dashboard.processes;
  List<Map<String, dynamic>> get _trendPoints => _dashboard.trendPoints;
  String? get _processCursor => _dashboard.processCursor;
  String? get _toolCallCursor => _dashboard.toolCallCursor;
  String get _range => _dashboard.range;
  String? get _toolError => _dashboard.toolError;
  String? get _processError => _dashboard.processError;
  bool get _loadingSessions => _dashboard.loadingSessions;
  bool get _loadingTools => _dashboard.loadingTools;
  bool get _loadingMoreToolCalls => _dashboard.loadingMoreToolCalls;
  bool get _loadingTrends => _dashboard.loadingTrends;
  bool get _loadingMoreProcesses => _dashboard.loadingMoreProcesses;
  bool get _loadingProcesses => _dashboard.loadingProcesses;

  @override
  void initState() {
    super.initState();
    _dashboard.addListener(_onDashboardChanged);
    _settings.addListener(_onSettingsChanged);
    _startBackend();
  }

  void _onDashboardChanged() {
    if (mounted) setState(() {});
  }

  void _onSettingsChanged() {
    if (mounted) setState(() {});
  }

  Future<void> _startBackend() async {
    _refreshCoordinator.stop();
    final previous = _backendSession;
    _backendSession = null;
    _apiClient = null;
    _dashboard.setApiClient(null);
    _settings.setApiClient(null);
    if (previous != null) await previous.stop();
    if (!mounted) return;
    setState(() {
      _starting = true;
      _error = null;
    });
    try {
      final session = await BackendSession.start();
      _backendSession = session;
      final api = LocalApiClient(
        baseUri: session.apiUri,
        token: session.apiToken,
      );
      _apiClient = api;
      _dashboard.setApiClient(api);
      _backendVersion = session.version;
      _settings.setApiClient(api, backendVersion: session.version);
      if (!mounted) {
        _backendSession = null;
        _apiClient = null;
        _dashboard.setApiClient(null);
        _settings.setApiClient(null);
        unawaited(session.stop());
        return;
      }
      setState(() => _starting = false);
      await _loadData(
        includeSessions: true,
        includeProcesses: true,
        includeTools: true,
        includeTrends: true,
      );
      if (!mounted) {
        _backendSession = null;
        _apiClient = null;
        _dashboard.setApiClient(null);
        _settings.setApiClient(null);
        unawaited(session.stop());
        return;
      }
      _refreshCoordinator.start(api);
      session.exitCode.then((code) {
        if (!mounted || !identical(_backendSession, session)) return;
        _refreshCoordinator.stop();
        _dashboard.setApiClient(null);
        _settings.setApiClient(null);
        setState(() {
          _backendSession = null;
          _apiClient = null;
          _error = '本地采集服务已停止（$code）';
        });
      });
    } catch (error) {
      final failedBackend = _backendSession;
      _backendSession = null;
      _apiClient = null;
      _dashboard.setApiClient(null);
      _settings.setApiClient(null);
      if (failedBackend != null) await failedBackend.stop();
      if (!mounted) return;
      setState(() {
        _starting = false;
        _error = '无法启动本地采集服务：$error';
      });
    }
  }

  Future<void> _loadData({
    bool includeSessions = false,
    bool includeProcesses = false,
    bool includeTools = false,
    bool includeTrends = false,
  }) => _dashboard.loadData(
    includeSessions: includeSessions,
    includeProcesses: includeProcesses,
    includeTools: includeTools,
    includeTrends: includeTrends,
  );
  Future<void> _loadProcessPage() => _loadData(includeProcesses: true);

  Future<void> _loadProcessSettings() => _dashboard.loadProcessConfig();

  Future<void> _loadToolPage() => _loadData(includeTools: true);

  Future<void> _loadMoreToolCalls() => _dashboard.loadMoreToolCalls();

  Future<void> _loadMoreProcesses() => _dashboard.loadMoreProcesses();

  Future<void> _saveProcessConfig(Map<String, dynamic> config) async {
    final saved = await _dashboard.saveProcessConfig(config);
    if (saved && mounted) _showMessage('进程监控设置已保存。');
  }

  ProcessPage _processSettingsPage({required bool privacyOnly}) => ProcessPage(
    config: _processConfig,
    status:
        (_status['process_monitor'] as Map?)?.cast<String, dynamic>() ??
        const <String, dynamic>{},
    processes: const [],
    nextCursor: null,
    loading: false,
    loadingMore: false,
    error: null,
    onSave: _saveProcessConfig,
    onRefresh: _loadProcessSettings,
    onLoadMore: () {},
    configurationOnly: !privacyOnly,
    privacyOnly: privacyOnly,
  );

  @override
  void dispose() {
    _refreshCoordinator.dispose();
    _dashboard.removeListener(_onDashboardChanged);
    _settings.removeListener(_onSettingsChanged);
    _dashboard.dispose();
    _settings.dispose();
    final backend = _backendSession;
    if (backend != null) unawaited(backend.stop());
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final processStatus =
        (_status['process_monitor'] as Map?)?.cast<String, dynamic>() ??
        const <String, dynamic>{};
    final child = _selectedRun != null && _apiClient != null
        ? RunDetailPage(
            apiClient: _apiClient!,
            run: _selectedRun!,
            onBack: () => setState(() => _selectedRun = null),
          )
        : switch (_page) {
            0 => LivePage(
              status: _status,
              summary: _summary,
              sessions: _sessions,
              toolSummaries: _toolSummaries,
              toolCalls: _toolCalls,
              processes: _processes,
              trendPoints: _trendPoints,
              starting: _starting,
              error: _error,
              loading:
                  _loadingSessions ||
                  _loadingProcesses ||
                  _loadingTools ||
                  _loadingTrends,
              onRefresh: () => _loadData(
                includeSessions: true,
                includeProcesses: true,
                includeTools: true,
                includeTrends: true,
              ),
              onOpenRun: _openSession,
              onOpenRuns: () => _navigate(1),
            ),
            1 => SessionExplorerPage(
              apiClient: _apiClient,
              onOpenSession: _openSession,
            ),
            2 => ToolAnalyticsPage(
              range: _range,
              summaries: _toolSummaries,
              calls: _toolCalls,
              loading: _loadingTools,
              loadingMore: _loadingMoreToolCalls,
              nextCursor: _toolCallCursor,
              error: _toolError,
              onRangeChanged: (value) {
                _dashboard.setRange(value);
                _loadToolPage();
              },
              onRefresh: _loadToolPage,
              onLoadMore: _loadMoreToolCalls,
            ),
            3 => ProcessExplorerPage(
              status: processStatus,
              processes: _processes,
              loading: _loadingProcesses,
              loadingMore: _loadingMoreProcesses,
              nextCursor: _processCursor,
              error: _processError,
              onRefresh: _loadProcessPage,
              onLoadMore: _loadMoreProcesses,
            ),
            4 => DiagnosticsPage(
              apiClient: _apiClient,
              status: _status,
              backendVersion: _backendVersion,
              retention: _settings.retention,
              onRefresh: () => _loadData(),
            ),
            _ => SettingsPage(
              selectedSection: _settingsSection,
              onSectionChanged: (section) {
                setState(() => _settingsSection = section);
                if (section == 2 || section == 3) _loadProcessSettings();
              },
              processSettingsPage: _processSettingsPage(privacyOnly: false),
              privacySettingsPage: _processSettingsPage(privacyOnly: true),
              signals:
                  (_status['signals'] as Map?)?.cast<String, dynamic>() ??
                  const {},
              status: _status,
              backendVersion: _backendVersion,
              config: _settings.config,
              preview: _settings.preview,
              selectedBackup: _settings.selectedBackup,
              retention: _settings.retention,
              retentionDays: _settings.retentionDays,
              receiverPort: _settings.receiverPort,
              receiverSettingsError: _settings.receiverSettingsError,
              loadingReceiverSettings: _settings.loadingReceiverSettings,
              savingReceiverSettings: _settings.savingReceiverSettings,
              retentionError: _settings.retentionError,
              loadingRetention: _settings.loadingRetention,
              savingRetention: _settings.savingRetention,
              requestingCleanup: _settings.requestingCleanup,
              exportingData: _settings.exportingData,
              clearingHistory: _settings.clearingHistory,
              exportingDiagnostics: _settings.exportingDiagnostics,
              cleaningLogs: _settings.cleaningLogs,
              apiAvailable: _apiClient != null,
              onSelectedBackupChanged: _settings.setSelectedBackup,
              onRetentionDaysChanged: _settings.setRetentionDays,
              onReceiverPortChanged: _settings.setReceiverPort,
              onSaveReceiverSettings: _saveReceiverSettings,
              onLoadReceiverSettings: _loadReceiverSettings,
              onLoadConfig: _loadConfig,
              onLoadPreview: _loadPreview,
              onApplyConfig: _applyConfig,
              onRestoreConfig: _restoreConfig,
              onSaveRetention: _saveRetention,
              onRequestCleanup: _requestCleanup,
              onExportData: _exportData,
              onClearHistory: _clearHistory,
              onExportDiagnostics: _exportDiagnostics,
              onCleanupLogs: _cleanupLogs,
            ),
          };
    return AppShell(
      selectedIndex: _selectedRun != null ? 1 : _page,
      onNavigate: _navigate,
      status: _status,
      backendVersion: _backendVersion,
      starting: _starting,
      apiAvailable: _apiClient != null,
      backendError: _error,
      sessions: _sessions,
      onRefresh: () => _loadData(
        includeSessions: true,
        includeProcesses: _page == 0 || _page == 3,
        includeTools: _page == 2,
        includeTrends: _page == 0,
      ),
      onRestart: _startBackend,
      child: child,
    );
  }

  void _navigate(int value) {
    setState(() {
      _selectedRun = null;
      _page = value;
    });
    if (value == 0 || value == 1) {
      _loadData(
        includeSessions: true,
        includeProcesses: value == 0,
        includeTools: value == 0,
        includeTrends: value == 0,
      );
    }
    if (value == 2) _loadToolPage();
    if (value == 3) _loadProcessPage();
    if (value == 4) {
      _loadRetention();
      _loadData();
    }
    if (value == 5) {
      _loadConfig();
      _loadRetention();
      _loadReceiverSettings();
      _loadProcessSettings();
    }
  }

  Future<void> _loadRetention() => _settings.loadRetention();

  Future<void> _loadReceiverSettings() => _settings.loadReceiverSettings();

  Future<void> _saveReceiverSettings() async {
    final receiverRestarted = await _settings.saveReceiverSettings();
    if (!mounted || receiverRestarted == null) return;
    if (!receiverRestarted) {
      _showMessage('接收器端口未变化，无需重启。');
      return;
    }
    await Future.wait([
      _loadData(),
      _settings.loadReceiverSettings(),
      _loadConfig(),
      _loadPreview(),
    ]);
    if (mounted) {
      _showMessage('接收器已切换到新端口。请检查 Codex 配置预览，备份并应用新地址。');
    }
  }

  Future<void> _saveRetention() async {
    if (await _settings.saveRetention() && mounted) {
      _showMessage('遥测保留期已保存，后台会按新设置清理过期数据。');
    }
  }

  Future<void> _requestCleanup() async {
    if (_apiClient == null) return;
    final approved = await showDialog<bool>(
      context: context,
      builder: (context) => AlertDialog(
        title: const Text('立即清理过期遥测？'),
        content: Text(
          '将删除早于当前保留期（${_settings.retentionDays} 天）的遥测记录。进程历史采用独立保留设置。',
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(context, false),
            child: const Text('取消'),
          ),
          FilledButton(
            onPressed: () => Navigator.pop(context, true),
            child: const Text('开始清理'),
          ),
        ],
      ),
    );
    if (approved != true) return;
    if (await _settings.requestCleanup() && mounted) {
      _showMessage('清理任务已在后台启动。');
    }
  }

  Future<void> _exportData() async {
    if (_apiClient == null) return;
    final now = DateTime.now();
    final name =
        'agentpulse-data-${now.year}${now.month.toString().padLeft(2, '0')}${now.day.toString().padLeft(2, '0')}.db';
    final destination = await getSaveLocation(
      suggestedName: name,
      acceptedTypeGroups: const [
        XTypeGroup(label: 'SQLite 数据库', extensions: ['db']),
      ],
    );
    if (destination == null || !mounted) return;
    try {
      await _settings.exportDataTo(destination.path);
      if (mounted) _showMessage('数据已导出到 ${destination.path}');
    } catch (error) {
      if (mounted) _showMessage('导出本地数据失败：$error');
    }
  }

  Future<void> _clearHistory() async {
    if (_apiClient == null) return;
    final approved = await showDialog<bool>(
      context: context,
      builder: (context) => AlertDialog(
        title: const Text('永久删除全部历史记录？'),
        content: const Text(
          '将删除已采集的遥测、会话、工具调用和进程历史，且无法恢复。遥测保留设置和 Codex 配置会保留。建议先导出本地数据。',
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(context, false),
            child: const Text('取消'),
          ),
          FilledButton(
            style: FilledButton.styleFrom(
              backgroundColor: const Color(0xffb3261e),
            ),
            onPressed: () => Navigator.pop(context, true),
            child: const Text('删除全部历史'),
          ),
        ],
      ),
    );
    if (approved != true) return;
    try {
      final deletedRows = await _settings.clearHistory();
      if (deletedRows == null) return;
      await _loadData(
        includeSessions: true,
        includeProcesses: true,
        includeTools: true,
      );
      await _loadRetention();
      if (mounted) _showMessage('已清理 $deletedRows 条历史记录。');
    } catch (error) {
      if (mounted) _showMessage('清空历史记录失败：$error');
    }
  }

  void _openSession(Map<String, dynamic> session) {
    final id = session['id'] as String?;
    if (id == null || _apiClient == null) return;
    setState(() {
      _selectedRun = session;
      _page = 1;
    });
  }

  Future<void> _exportDiagnostics() async {
    if (_apiClient == null) return;
    final now = DateTime.now();
    final name =
        'agentpulse-diagnostics-${now.year}${now.month.toString().padLeft(2, '0')}${now.day.toString().padLeft(2, '0')}.json';
    final destination = await getSaveLocation(
      suggestedName: name,
      acceptedTypeGroups: const [
        XTypeGroup(label: 'JSON 诊断包', extensions: ['json']),
      ],
    );
    if (destination == null || !mounted) return;
    try {
      await _settings.exportDiagnosticsTo(destination.path);
      if (mounted) _showMessage('脱敏诊断包已保存到 ${destination.path}');
    } catch (error) {
      if (mounted) _showMessage('导出脱敏诊断包失败：$error');
    }
  }

  Future<void> _cleanupLogs() async {
    if (_apiClient == null) return;
    final approved = await showDialog<bool>(
      context: context,
      builder: (context) => AlertDialog(
        title: const Text('清理过期应用日志？'),
        content: const Text('将删除 14 天前的轮换日志，不影响近期日志、遥测数据或 Codex 配置。'),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(context, false),
            child: const Text('取消'),
          ),
          FilledButton(
            onPressed: () => Navigator.pop(context, true),
            child: const Text('清理日志'),
          ),
        ],
      ),
    );
    if (approved != true) return;
    try {
      final deletedBytes = await _settings.cleanupLogs();
      if (deletedBytes == null) return;
      if (mounted) {
        _showMessage('过期日志清理完成，释放 ${formatSettingsBytes(deletedBytes)}。');
      }
    } catch (error) {
      if (mounted) _showMessage('清理过期日志失败：$error');
    }
  }

  Future<void> _loadConfig() async {
    try {
      await _settings.loadConfig();
    } catch (error) {
      _showMessage('检测配置失败：$error');
    }
  }

  Future<void> _loadPreview() async {
    try {
      await _settings.loadPreview();
    } catch (error) {
      _showMessage('无法安全预览配置：$error');
    }
  }

  Future<void> _applyConfig() async {
    final approved = await showDialog<bool>(
      context: context,
      builder: (context) => AlertDialog(
        title: const Text('备份并应用 Codex 配置？'),
        content: Text(
          '将只更新预览中的 OTel 设置，并在同一目录备份现有文件：\n${_settings.preview['path'] ?? _settings.config['path'] ?? ''}\n\n完成后需要重新启动 Codex。',
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(context, false),
            child: const Text('取消'),
          ),
          FilledButton(
            onPressed: () => Navigator.pop(context, true),
            child: const Text('备份并应用'),
          ),
        ],
      ),
    );
    if (approved != true) return;
    try {
      final message = await _settings.applyConfig();
      if (mounted && message != null) _showMessage(message);
    } catch (error) {
      _showMessage('应用配置失败：$error');
    }
  }

  Future<void> _restoreConfig() async {
    final backup = _settings.selectedBackup;
    if (backup == null) return;
    final approved = await showDialog<bool>(
      context: context,
      builder: (context) => AlertDialog(
        title: const Text('恢复 Codex 配置？'),
        content: Text('将用备份“$backup”替换当前配置，并先备份当前文件。'),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(context, false),
            child: const Text('取消'),
          ),
          FilledButton(
            onPressed: () => Navigator.pop(context, true),
            child: const Text('恢复'),
          ),
        ],
      ),
    );
    if (approved != true) return;
    try {
      final message = await _settings.restoreConfig(backup);
      if (mounted && message != null) _showMessage(message);
    } catch (error) {
      _showMessage('恢复配置失败：$error');
    }
  }

  void _showMessage(String message) {
    if (!mounted) return;
    ScaffoldMessenger.of(context)
        .showSnackBar(SnackBar(content: Text(message)));
  }
}
