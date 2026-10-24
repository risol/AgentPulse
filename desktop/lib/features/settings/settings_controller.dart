import 'dart:convert';
import 'dart:io';

import 'package:flutter/foundation.dart';

import '../../core/api/local_api_client.dart';

class SettingsController extends ChangeNotifier {
  LocalApiClient? _apiClient;
  String _backendVersion = 'development';
  int _generation = 0;
  bool _disposed = false;

  Map<String, dynamic> _config = {};
  Map<String, dynamic> _preview = {};
  Map<String, dynamic> _retention = {};
  Map<String, dynamic> _receiverSettings = {};
  String? _selectedBackup;
  String? _retentionError;
  bool _loadingRetention = false;
  bool _savingRetention = false;
  bool _requestingCleanup = false;
  bool _exportingData = false;
  bool _exportingDiagnostics = false;
  bool _cleaningLogs = false;
  bool _clearingHistory = false;
  int _retentionDays = 30;
  int _receiverPort = 4318;
  String? _receiverSettingsError;
  bool _loadingReceiverSettings = false;
  bool _savingReceiverSettings = false;

  Map<String, dynamic> get config => _config;
  Map<String, dynamic> get preview => _preview;
  Map<String, dynamic> get retention => _retention;
  Map<String, dynamic> get receiverSettings => _receiverSettings;
  String? get selectedBackup => _selectedBackup;
  String? get retentionError => _retentionError;
  bool get loadingRetention => _loadingRetention;
  bool get savingRetention => _savingRetention;
  bool get requestingCleanup => _requestingCleanup;
  bool get exportingData => _exportingData;
  bool get exportingDiagnostics => _exportingDiagnostics;
  bool get cleaningLogs => _cleaningLogs;
  bool get clearingHistory => _clearingHistory;
  int get retentionDays => _retentionDays;
  int get receiverPort => _receiverPort;
  String? get receiverSettingsError => _receiverSettingsError;
  bool get loadingReceiverSettings => _loadingReceiverSettings;
  bool get savingReceiverSettings => _savingReceiverSettings;

  void setApiClient(LocalApiClient? apiClient, {String? backendVersion}) {
    final version = backendVersion ?? _backendVersion;
    if (identical(_apiClient, apiClient) && _backendVersion == version) return;
    _apiClient = apiClient;
    _backendVersion = version;
    _generation++;
    _clearData();
    _notify();
  }

  void setSelectedBackup(String? backup) {
    if (_selectedBackup == backup) return;
    _selectedBackup = backup;
    _notify();
  }

  void setRetentionDays(int days) {
    if (_retentionDays == days) return;
    _retentionDays = days;
    _notify();
  }

  void setReceiverPort(int port) {
    if (_receiverPort == port) return;
    _receiverPort = port;
    _notify();
  }

  Future<void> loadReceiverSettings() async {
    final api = _apiClient;
    if (api == null) return;
    final generation = _generation;
    _loadingReceiverSettings = true;
    _notify();
    try {
      final snapshot = await api.get('/api/v1/settings/receiver');
      if (!_isCurrent(api, generation)) return;
      final settings =
          (snapshot['settings'] as Map?)?.cast<String, dynamic>() ??
          snapshot.cast<String, dynamic>();
      _receiverSettings = settings;
      _receiverPort = (settings['port'] as num?)?.toInt() ?? 4318;
      _receiverSettingsError = null;
    } catch (error) {
      if (_isCurrent(api, generation)) {
        _receiverSettingsError = '读取接收器设置失败：$error';
      }
    } finally {
      if (_isCurrent(api, generation)) {
        _loadingReceiverSettings = false;
        _notify();
      }
    }
  }

  Future<bool?> saveReceiverSettings() async {
    final api = _apiClient;
    if (api == null) return null;
    final generation = _generation;
    _savingReceiverSettings = true;
    _notify();
    try {
      final snapshot = await api.put('/api/v1/settings/receiver', {
        'port': _receiverPort,
      });
      if (!_isCurrent(api, generation)) return null;
      final settings =
          (snapshot['settings'] as Map?)?.cast<String, dynamic>() ??
          <String, dynamic>{'port': _receiverPort};
      _receiverSettings = settings;
      _receiverPort = (settings['port'] as num?)?.toInt() ?? _receiverPort;
      _receiverSettingsError = null;
      return snapshot['receiver_restarted'] == true ||
          (snapshot['receiver_restarted'] == null &&
              snapshot['restart_required'] == true);
    } catch (error) {
      if (_isCurrent(api, generation)) {
        _receiverSettingsError = '保存接收器设置失败：$error';
      }
      return null;
    } finally {
      if (_isCurrent(api, generation)) {
        _savingReceiverSettings = false;
        _notify();
      }
    }
  }

  Future<void> loadRetention() async {
    final api = _apiClient;
    if (api == null) return;
    final generation = _generation;
    _loadingRetention = true;
    _notify();
    try {
      final snapshot = await api.get('/api/v1/settings/data-retention');
      if (!_isCurrent(api, generation)) return;
      _retention = snapshot;
      _retentionDays = (snapshot['retention_days'] as num?)?.toInt() ?? 30;
      _retentionError = null;
    } catch (error) {
      if (_isCurrent(api, generation)) {
        _retentionError = '读取数据保留设置失败：$error';
      }
    } finally {
      if (_isCurrent(api, generation)) {
        _loadingRetention = false;
        _notify();
      }
    }
  }

  Future<bool> saveRetention() async {
    final api = _apiClient;
    if (api == null) return false;
    final generation = _generation;
    _savingRetention = true;
    _notify();
    try {
      final snapshot = await api.put('/api/v1/settings/data-retention', {
        'retention_days': _retentionDays,
      });
      if (!_isCurrent(api, generation)) return false;
      _retention = snapshot;
      _retentionError = null;
      return true;
    } catch (error) {
      if (_isCurrent(api, generation)) {
        _retentionError = '保存数据保留设置失败：$error';
      }
      return false;
    } finally {
      if (_isCurrent(api, generation)) {
        _savingRetention = false;
        _notify();
      }
    }
  }

  Future<bool> requestCleanup() async {
    final api = _apiClient;
    if (api == null) return false;
    final generation = _generation;
    _requestingCleanup = true;
    _notify();
    try {
      await api.post('/api/v1/settings/data-retention/cleanup');
      if (!_isCurrent(api, generation)) return false;
      await loadRetention();
      return true;
    } catch (error) {
      if (_isCurrent(api, generation)) {
        _retentionError = '启动数据清理失败：$error';
      }
      return false;
    } finally {
      if (_isCurrent(api, generation)) {
        _requestingCleanup = false;
        _notify();
      }
    }
  }

  Future<void> loadConfig() async {
    final api = _apiClient;
    if (api == null) return;
    final generation = _generation;
    final config = await api.get('/api/v1/config/codex');
    if (!_isCurrent(api, generation)) return;
    _config = config;
    final backups = (config['backups'] as List?)?.cast<String>() ?? [];
    if (!backups.contains(_selectedBackup)) {
      _selectedBackup = backups.isEmpty ? null : backups.first;
    }
    _notify();
  }

  Future<void> loadPreview() async {
    final api = _apiClient;
    if (api == null) return;
    final generation = _generation;
    final preview = await api.get('/api/v1/config/codex/preview');
    if (!_isCurrent(api, generation)) return;
    _preview = preview;
    _notify();
  }

  Future<String?> applyConfig() async {
    final api = _apiClient;
    if (api == null) return null;
    final generation = _generation;
    final result = await api.post('/api/v1/config/codex/apply');
    if (!_isCurrent(api, generation)) return null;
    _config = (result['config'] as Map).cast<String, dynamic>();
    _preview = {};
    _notify();
    final backup = result['backup'];
    return backup == '' ? '配置已经是最新状态。' : '配置已应用，备份为 $backup。请重新启动 Codex。';
  }

  Future<String?> restoreConfig(String backup) async {
    final api = _apiClient;
    if (api == null) return null;
    final generation = _generation;
    final result = await api.post('/api/v1/config/codex/restore', {
      'backup': backup,
    });
    if (!_isCurrent(api, generation)) return null;
    _config = (result['config'] as Map).cast<String, dynamic>();
    _notify();
    return '配置已恢复，当前文件备份为 ${result['recovery_backup']}。请重新启动 Codex。';
  }

  Future<void> exportDataTo(String destinationPath) async {
    final api = _apiClient;
    if (api == null) return;
    final generation = _generation;
    _exportingData = true;
    _notify();
    File? temporaryFile;
    try {
      temporaryFile = File(
        '$destinationPath.${DateTime.now().microsecondsSinceEpoch}.partial',
      );
      await api.downloadTo('/api/v1/data/export', temporaryFile.path);
      await temporaryFile.copy(destinationPath);
      await temporaryFile.delete();
    } finally {
      if (temporaryFile != null && await temporaryFile.exists()) {
        await temporaryFile.delete();
      }
      if (_isCurrent(api, generation)) {
        _exportingData = false;
        _notify();
      }
    }
  }

  Future<int?> clearHistory() async {
    final api = _apiClient;
    if (api == null) return null;
    final generation = _generation;
    _clearingHistory = true;
    _notify();
    try {
      final result = await api.delete('/api/v1/data/history');
      if (!_isCurrent(api, generation)) return null;
      return (result['deleted_rows'] as num?)?.toInt() ?? 0;
    } finally {
      if (_isCurrent(api, generation)) {
        _clearingHistory = false;
        _notify();
      }
    }
  }

  Future<void> exportDiagnosticsTo(String destinationPath) async {
    final api = _apiClient;
    if (api == null) return;
    final generation = _generation;
    _exportingDiagnostics = true;
    _notify();
    try {
      final status = await api.get('/api/v1/status');
      if (!_isCurrent(api, generation)) return;
      final diagnosticSnapshot =
          (status['ingest_diagnostics'] as Map?)?.cast<String, dynamic>() ??
          const <String, dynamic>{};
      final safeBundle = <String, dynamic>{
        'format_version': 1,
        'generated_at_utc': DateTime.now().toUtc().toIso8601String(),
        'application': {'version': _backendVersion},
        'receiver': {
          'state': status['receiver'],
          'address': status['receiver_address'],
          'schema_version': status['schema_version'],
          'signals': status['signals'],
        },
        'ingest': {
          'queue': status['ingest_queue'],
          'diagnostics': {
            'count_last_24_hours': diagnosticSnapshot['count_last_24_hours'],
            'recent': diagnosticSnapshot['recent'] ?? const [],
          },
        },
        'privacy': {
          'includes_telemetry_payloads': false,
          'includes_process_details': false,
          'includes_codex_configuration': false,
        },
      };
      await File(destinationPath).writeAsString(
        const JsonEncoder.withIndent('  ').convert(safeBundle),
        flush: true,
      );
    } finally {
      if (_isCurrent(api, generation)) {
        _exportingDiagnostics = false;
        _notify();
      }
    }
  }

  Future<int?> cleanupLogs() async {
    final api = _apiClient;
    if (api == null) return null;
    final generation = _generation;
    _cleaningLogs = true;
    _notify();
    try {
      final result = await api.post('/api/v1/settings/logs/cleanup');
      if (!_isCurrent(api, generation)) return null;
      return (result['deleted_bytes'] as num?)?.toInt() ?? 0;
    } finally {
      if (_isCurrent(api, generation)) {
        _cleaningLogs = false;
        _notify();
      }
    }
  }

  bool _isCurrent(LocalApiClient api, int generation) =>
      !_disposed && identical(_apiClient, api) && _generation == generation;

  void _clearData() {
    _config = {};
    _preview = {};
    _retention = {};
    _receiverSettings = {};
    _selectedBackup = null;
    _retentionError = null;
    _loadingRetention = false;
    _savingRetention = false;
    _requestingCleanup = false;
    _exportingData = false;
    _exportingDiagnostics = false;
    _cleaningLogs = false;
    _clearingHistory = false;
    _retentionDays = 30;
    _receiverPort = 4318;
    _receiverSettingsError = null;
    _loadingReceiverSettings = false;
    _savingReceiverSettings = false;
  }

  void _notify() {
    if (!_disposed) notifyListeners();
  }

  @override
  void dispose() {
    _disposed = true;
    super.dispose();
  }
}
