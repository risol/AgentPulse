import 'dart:async';

import '../../core/api/local_api_client.dart';

typedef DashboardLoadData = Future<void> Function({
  required bool includeSessions,
  required bool includeProcesses,
  required bool includeTools,
  required bool includeTrends,
});

typedef DashboardEventSourceFactory = Future<DashboardEventSource> Function(
  LocalApiClient apiClient,
);

abstract interface class DashboardEventSource {
  Stream<String> get lines;

  void close();
}

class DashboardRefreshCoordinator {
  DashboardRefreshCoordinator({
    required this.loadData,
    required this.loadRetention,
    required this.currentPage,
    DashboardEventSourceFactory? eventSourceFactory,
    this.pollingInterval = const Duration(seconds: 5),
    this.sessionRefreshInterval = const Duration(seconds: 30),
    this.reconnectDelay = const Duration(seconds: 3),
    this.eventDebounce = const Duration(milliseconds: 250),
  }) : eventSourceFactory = eventSourceFactory ?? _openApiEventSource;

  final DashboardLoadData loadData;
  final Future<void> Function() loadRetention;
  final int Function() currentPage;
  final DashboardEventSourceFactory eventSourceFactory;
  final Duration pollingInterval;
  final Duration sessionRefreshInterval;
  final Duration reconnectDelay;
  final Duration eventDebounce;

  LocalApiClient? _apiClient;
  DashboardEventSource? _eventSource;
  Timer? _pollingTimer;
  Timer? _sessionTimer;
  Timer? _reconnectTimer;
  Timer? _debounceTimer;
  bool _disposed = false;

  void start(LocalApiClient apiClient) {
    if (_disposed) {
      throw StateError('Dashboard refresh coordinator is disposed');
    }
    stop();
    _apiClient = apiClient;
    _pollingTimer = Timer.periodic(pollingInterval, (_) {
      final page = currentPage();
      _runLoadData(
        includeSessions: false,
        includeProcesses: page == 1,
        includeTools: page == 2,
        includeTrends: false,
      );
      if (page == 3) _runLoadRetention();
    });
    _sessionTimer = Timer.periodic(sessionRefreshInterval, (_) {
      final page = currentPage();
      _runLoadData(
        includeSessions: true,
        includeProcesses: page == 1,
        includeTools: page == 2,
        includeTrends: page == 0,
      );
    });
    unawaited(_connectEventStream(apiClient));
  }

  void stop() {
    _pollingTimer?.cancel();
    _sessionTimer?.cancel();
    _reconnectTimer?.cancel();
    _debounceTimer?.cancel();
    _pollingTimer = null;
    _sessionTimer = null;
    _reconnectTimer = null;
    _debounceTimer = null;
    _eventSource?.close();
    _eventSource = null;
    _apiClient = null;
  }

  Future<void> _connectEventStream(LocalApiClient apiClient) async {
    if (_disposed || !identical(_apiClient, apiClient)) return;
    DashboardEventSource? eventSource;
    try {
      eventSource = await eventSourceFactory(apiClient);
      if (_disposed || !identical(_apiClient, apiClient)) {
        eventSource.close();
        return;
      }
      _eventSource = eventSource;
      await for (final line in eventSource.lines) {
        if (line == 'event: telemetry') _scheduleLiveRefresh(apiClient);
      }
    } catch (_) {
      // Periodic refresh remains active if SSE is unavailable.
    } finally {
      eventSource?.close();
      if (identical(_eventSource, eventSource)) _eventSource = null;
      if (!_disposed && identical(_apiClient, apiClient)) {
        _reconnectTimer = Timer(reconnectDelay, () {
          unawaited(_connectEventStream(apiClient));
        });
      }
    }
  }

  void _scheduleLiveRefresh(LocalApiClient apiClient) {
    _debounceTimer?.cancel();
    _debounceTimer = Timer(eventDebounce, () {
      if (_disposed || !identical(_apiClient, apiClient)) return;
      final page = currentPage();
      _runLoadData(
        includeSessions: true,
        includeProcesses: page == 1,
        includeTools: page == 2,
        includeTrends: page == 0,
      );
      if (page == 3) _runLoadRetention();
    });
  }

  void _runLoadData({
    required bool includeSessions,
    required bool includeProcesses,
    required bool includeTools,
    required bool includeTrends,
  }) {
    unawaited(
      loadData(
        includeSessions: includeSessions,
        includeProcesses: includeProcesses,
        includeTools: includeTools,
        includeTrends: includeTrends,
      ).catchError((_) {}),
    );
  }

  void _runLoadRetention() {
    unawaited(loadRetention().catchError((_) {}));
  }

  void dispose() {
    if (_disposed) return;
    stop();
    _disposed = true;
  }

  static Future<DashboardEventSource> _openApiEventSource(
    LocalApiClient apiClient,
  ) async => _ApiDashboardEventSource(await apiClient.openEventStream());
}

class _ApiDashboardEventSource implements DashboardEventSource {
  _ApiDashboardEventSource(this._connection);

  final ApiEventConnection _connection;

  @override
  Stream<String> get lines => _connection.lines;

  @override
  void close() => _connection.close();
}
