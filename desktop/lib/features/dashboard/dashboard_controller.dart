import 'package:flutter/foundation.dart';

import '../../core/api/local_api_client.dart';

class DashboardController extends ChangeNotifier {
  LocalApiClient? _apiClient;
  int _generation = 0;
  bool _disposed = false;

  Map<String, dynamic> _status = {};
  Map<String, dynamic> _summary = {};
  Map<String, dynamic> _processConfig = {};
  List<Map<String, dynamic>> _sessions = [];
  List<Map<String, dynamic>> _unassignedEvents = [];
  List<Map<String, dynamic>> _toolSummaries = [];
  List<Map<String, dynamic>> _toolCalls = [];
  List<Map<String, dynamic>> _processes = [];
  List<Map<String, dynamic>> _trendPoints = [];
  String? _sessionCursor;
  String? _unassignedCursor;
  String? _processCursor;
  String? _toolCallCursor;
  String _range = '24h';
  String _toolOutcome = 'all';
  String _toolFilter = 'all';
  String _toolSessionFilter = '';
  String? _queryError;
  String? _toolError;
  String? _processError;
  String? _trendError;
  String? _sessionError;
  String? _unassignedError;
  bool _loadingSessions = false;
  bool _loadingUnassigned = false;
  bool _loadingMoreSessions = false;
  bool _loadingMoreUnassigned = false;
  bool _loadingTools = false;
  bool _loadingMoreToolCalls = false;
  bool _loadingTrends = false;
  bool _loadingMoreProcesses = false;
  bool _loadingProcesses = false;

  Map<String, dynamic> get status => _status;
  Map<String, dynamic> get summary => _summary;
  Map<String, dynamic> get processConfig => _processConfig;
  List<Map<String, dynamic>> get sessions => _sessions;
  List<Map<String, dynamic>> get unassignedEvents => _unassignedEvents;
  List<Map<String, dynamic>> get toolSummaries => _toolSummaries;
  List<Map<String, dynamic>> get toolCalls => _toolCalls;
  List<Map<String, dynamic>> get processes => _processes;
  List<Map<String, dynamic>> get trendPoints => _trendPoints;
  String? get sessionCursor => _sessionCursor;
  String? get unassignedCursor => _unassignedCursor;
  String? get processCursor => _processCursor;
  String? get toolCallCursor => _toolCallCursor;
  String get range => _range;
  String get toolOutcome => _toolOutcome;
  String get toolFilter => _toolFilter;
  String get toolSessionFilter => _toolSessionFilter;
  String? get queryError => _queryError;
  String? get toolError => _toolError;
  String? get processError => _processError;
  String? get trendError => _trendError;
  String? get sessionError => _sessionError;
  String? get unassignedError => _unassignedError;
  bool get loadingSessions => _loadingSessions;
  bool get loadingUnassigned => _loadingUnassigned;
  bool get loadingMoreSessions => _loadingMoreSessions;
  bool get loadingMoreUnassigned => _loadingMoreUnassigned;
  bool get loadingTools => _loadingTools;
  bool get loadingMoreToolCalls => _loadingMoreToolCalls;
  bool get loadingTrends => _loadingTrends;
  bool get loadingMoreProcesses => _loadingMoreProcesses;
  bool get loadingProcesses => _loadingProcesses;

  void setApiClient(LocalApiClient? apiClient) {
    if (identical(_apiClient, apiClient)) return;
    _apiClient = apiClient;
    _generation++;
    if (apiClient == null) _clearData();
    _notify();
  }

  void setRange(String value) {
    if (_range == value) return;
    _range = value;
    _notify();
  }

  void setToolFilter(String value) {
    if (_toolFilter == value) return;
    _toolFilter = value;
    _notify();
  }

  void setToolOutcome(String value) {
    if (_toolOutcome == value) return;
    _toolOutcome = value;
    _notify();
  }

  void setToolSessionFilter(String value) {
    final normalized = value.trim();
    if (_toolSessionFilter == normalized) return;
    _toolSessionFilter = normalized;
    _notify();
  }

  Future<void> loadData({
    bool includeSessions = false,
    bool includeProcesses = false,
    bool includeTools = false,
    bool includeTrends = false,
  }) async {
    final api = _apiClient;
    if (api == null) return;
    final generation = _generation;
    if (includeProcesses) _loadingProcesses = true;
    if (includeSessions) {
      _loadingSessions = true;
      _loadingUnassigned = true;
      _sessionError = null;
      _unassignedError = null;
    }
    if (includeTools) _loadingTools = true;
    if (includeTrends) _loadingTrends = true;
    _notify();

    int? sessionsIndex;
    int? unassignedIndex;
    int? processesIndex;
    int? toolsSummaryIndex;
    int? toolCallsIndex;
    int? trendsIndex;
    try {
      final requests = <Future<Map<String, dynamic>>>[
        api.get('/api/v1/status'),
        api.get('/api/v1/dashboard/summary?range=$_range'),
      ];
      if (includeSessions) {
        sessionsIndex = requests.length;
        requests.add(api.get('/api/v1/sessions?limit=10'));
        unassignedIndex = requests.length;
        requests.add(api.get('/api/v1/events/unassigned?limit=10'));
      }
      if (includeProcesses) {
        processesIndex = requests.length;
        requests.add(api.get('/api/v1/processes?limit=100'));
      }
      if (includeTools) {
        toolsSummaryIndex = requests.length;
        final summaryFilters = <String>['range=$_range'];
        if (_toolSessionFilter.isNotEmpty) {
          summaryFilters.add(
            'session_id=${Uri.encodeQueryComponent(_toolSessionFilter)}',
          );
        }
        requests.add(
          api.get('/api/v1/tools/summary?${summaryFilters.join('&')}'),
        );
        toolCallsIndex = requests.length;
        final filters = <String>[];
        if (_toolSessionFilter.isNotEmpty) {
          filters.add(
            'session_id=${Uri.encodeQueryComponent(_toolSessionFilter)}',
          );
        }
        if (_toolOutcome != 'all') filters.add('outcome=$_toolOutcome');
        if (_toolFilter != 'all') {
          filters.add('tool_name=${Uri.encodeQueryComponent(_toolFilter)}');
        }
        final suffix = filters.isEmpty ? '' : '&${filters.join('&')}';
        requests.add(
          api.get('/api/v1/tools/calls?range=$_range&limit=25$suffix'),
        );
      }
      if (includeTrends) {
        trendsIndex = requests.length;
        requests.add(api.get('/api/v1/analytics/trends?range=$_range'));
      }
      final values = await Future.wait(requests);
      if (!_isCurrent(api, generation)) return;
      _status = values[0];
      _summary = values[1];
      if (sessionsIndex != null) {
        _sessions = _items(values[sessionsIndex]['items']);
        _sessionCursor = values[sessionsIndex]['next_cursor'] as String?;
      }
      if (unassignedIndex != null) {
        _unassignedEvents = _items(values[unassignedIndex]['items']);
        _unassignedCursor = values[unassignedIndex]['next_cursor'] as String?;
      }
      if (processesIndex != null) {
        _processes = _items(values[processesIndex]['items']);
        _processCursor = values[processesIndex]['next_cursor'] as String?;
        _processError = null;
      }
      if (toolsSummaryIndex != null) {
        _toolSummaries = _items(values[toolsSummaryIndex]['items']);
        final callsIndex = toolCallsIndex!;
        _toolCalls = _items(values[callsIndex]['items']);
        _toolCallCursor = values[callsIndex]['next_cursor'] as String?;
        _toolError = null;
      }
      if (trendsIndex != null) {
        _trendPoints = _items(values[trendsIndex]['points']);
        _trendError = null;
      }
      _queryError = null;
    } catch (error) {
      if (!_isCurrent(api, generation)) return;
      if (includeSessions) {
        _sessionError = '读取会话失败：$error';
        _unassignedError = '读取未关联事件失败：$error';
      }
      if (includeProcesses) _processError = '读取进程数据失败：$error';
      if (includeTools) _toolError = '读取工具分析失败：$error';
      if (includeTrends) _trendError = '读取趋势数据失败：$error';
      _queryError = '部分仪表盘数据读取失败：$error';
    } finally {
      if (_isCurrent(api, generation)) {
        if (includeSessions) {
          _loadingSessions = false;
          _loadingUnassigned = false;
        }
        if (includeProcesses) _loadingProcesses = false;
        if (includeTools) _loadingTools = false;
        if (includeTrends) _loadingTrends = false;
        _notify();
      }
    }
  }

  Future<void> loadMoreSessions() => _loadMore(
    cursor: _sessionCursor,
    limit: 10,
    path: '/api/v1/sessions',
    isLoading: _loadingMoreSessions,
    setLoading: (value) => _loadingMoreSessions = value,
    onItems: (items, cursor) {
      _sessions = [..._sessions, ...items];
      _sessionCursor = cursor;
    },
    onError: (error) => _queryError = '读取更多会话失败：$error',
  );

  Future<void> loadMoreUnassignedEvents() => _loadMore(
    cursor: _unassignedCursor,
    limit: 10,
    path: '/api/v1/events/unassigned',
    isLoading: _loadingMoreUnassigned,
    setLoading: (value) => _loadingMoreUnassigned = value,
    onItems: (items, cursor) {
      _unassignedEvents = [..._unassignedEvents, ...items];
      _unassignedCursor = cursor;
    },
    onError: (error) => _queryError = '读取未关联事件失败：$error',
  );

  Future<void> loadMoreToolCalls() async {
    final cursor = _toolCallCursor;
    final api = _apiClient;
    if (api == null || cursor == null || _loadingMoreToolCalls) return;
    final generation = _generation;
    _loadingMoreToolCalls = true;
    _notify();
    try {
      final filters = <String>[
        'range=$_range',
        'limit=25',
        'cursor=${Uri.encodeQueryComponent(cursor)}',
      ];
      if (_toolSessionFilter.isNotEmpty) {
        filters.add(
          'session_id=${Uri.encodeQueryComponent(_toolSessionFilter)}',
        );
      }
      if (_toolOutcome != 'all') filters.add('outcome=$_toolOutcome');
      if (_toolFilter != 'all') {
        filters.add('tool_name=${Uri.encodeQueryComponent(_toolFilter)}');
      }
      final page = await api.get('/api/v1/tools/calls?${filters.join('&')}');
      if (!_isCurrent(api, generation)) return;
      _toolCalls = [..._toolCalls, ..._items(page['items'])];
      _toolCallCursor = page['next_cursor'] as String?;
      _toolError = null;
    } catch (error) {
      if (_isCurrent(api, generation)) {
        _toolError = '读取更多工具调用失败：$error';
      }
    } finally {
      if (_isCurrent(api, generation)) {
        _loadingMoreToolCalls = false;
        _notify();
      }
    }
  }

  Future<void> loadMoreProcesses() => _loadMore(
    cursor: _processCursor,
    limit: 100,
    path: '/api/v1/processes',
    isLoading: _loadingMoreProcesses,
    setLoading: (value) => _loadingMoreProcesses = value,
    onItems: (items, cursor) {
      _processes = [..._processes, ...items];
      _processCursor = cursor;
      _processError = null;
    },
    onError: (error) => _processError = '读取更多进程记录失败：$error',
  );

  Future<void> loadProcessConfig() async {
    final api = _apiClient;
    if (api == null) return;
    final generation = _generation;
    try {
      final response = await api.get('/api/v1/settings/process-monitor');
      if (!_isCurrent(api, generation)) return;
      _processConfig = (response['config'] as Map).cast<String, dynamic>();
      _processError = null;
    } catch (error) {
      if (_isCurrent(api, generation)) {
        _processError = '读取监控设置失败：$error';
      }
    }
    _notify();
  }

  Future<bool> saveProcessConfig(Map<String, dynamic> config) async {
    final api = _apiClient;
    if (api == null) return false;
    final generation = _generation;
    try {
      final response = await api.put(
        '/api/v1/settings/process-monitor',
        config,
      );
      if (!_isCurrent(api, generation)) return false;
      _processConfig = (response['config'] as Map).cast<String, dynamic>();
      _processError = null;
      _notify();
      await loadData(includeProcesses: true);
      return _isCurrent(api, generation);
    } catch (error) {
      if (_isCurrent(api, generation)) {
        _processError = '保存监控设置失败：$error';
        _notify();
      }
      return false;
    }
  }

  Future<void> _loadMore({
    required String? cursor,
    required int limit,
    required String path,
    required bool isLoading,
    required void Function(bool) setLoading,
    required void Function(List<Map<String, dynamic>>, String?) onItems,
    required void Function(Object) onError,
  }) async {
    final api = _apiClient;
    if (api == null || cursor == null || isLoading) return;
    final generation = _generation;
    setLoading(true);
    _notify();
    try {
      final separator = path.contains('?') ? '&' : '?';
      final page = await api.get(
        '$path${separator}limit=$limit&cursor=${Uri.encodeQueryComponent(cursor)}',
      );
      if (!_isCurrent(api, generation)) return;
      onItems(_items(page['items']), page['next_cursor'] as String?);
    } catch (error) {
      if (_isCurrent(api, generation)) onError(error);
    } finally {
      if (_isCurrent(api, generation)) {
        setLoading(false);
        _notify();
      }
    }
  }

  bool _isCurrent(LocalApiClient api, int generation) =>
      !_disposed && identical(_apiClient, api) && _generation == generation;

  List<Map<String, dynamic>> _items(Object? raw) => ((raw as List?) ?? const [])
      .map((value) => (value as Map).cast<String, dynamic>())
      .toList();

  void _clearData() {
    _status = {};
    _summary = {};
    _processConfig = {};
    _sessions = [];
    _unassignedEvents = [];
    _toolSummaries = [];
    _toolCalls = [];
    _processes = [];
    _trendPoints = [];
    _sessionCursor = null;
    _unassignedCursor = null;
    _processCursor = null;
    _toolCallCursor = null;
    _queryError = null;
    _toolError = null;
    _processError = null;
    _trendError = null;
    _sessionError = null;
    _unassignedError = null;
    _loadingSessions = false;
    _loadingUnassigned = false;
    _loadingMoreSessions = false;
    _loadingMoreUnassigned = false;
    _loadingTools = false;
    _loadingMoreToolCalls = false;
    _loadingTrends = false;
    _loadingMoreProcesses = false;
    _loadingProcesses = false;
  }

  void _notify() {
    if (!_disposed) notifyListeners();
  }

  @override
  void dispose() {
    _disposed = true;
    _apiClient = null;
    _generation++;
    super.dispose();
  }
}
