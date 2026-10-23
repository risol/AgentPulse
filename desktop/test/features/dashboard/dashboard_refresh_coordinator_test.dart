import 'dart:async';
import 'dart:convert';
import 'dart:io';

import 'package:agentpulse/core/api/local_api_client.dart';
import 'package:agentpulse/features/dashboard/dashboard_refresh_coordinator.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  late HttpServer server;
  late LocalApiClient api;
  late StreamController<String> eventController;
  late Completer<void> eventSubscribed;
  final requests = <String>[];
  int currentPage = 0;
  int retentionLoads = 0;
  int dashboardLoads = 0;
  final loadOptions = <Map<String, bool>>[];

  setUp(() async {
    requests.clear();
    eventSubscribed = Completer<void>();
    eventController = StreamController<String>(
      onListen: () => eventSubscribed.complete(),
    );
    currentPage = 0;
    retentionLoads = 0;
    dashboardLoads = 0;
    loadOptions.clear();
    server = await HttpServer.bind(InternetAddress.loopbackIPv4, 0);
    server.listen((request) async {
      requests.add(request.uri.path);
      request.response.headers.contentType = ContentType.json;
      request.response.write(jsonEncode({'ok': true}));
      await request.response.close();
    });
    api = LocalApiClient(
      baseUri: Uri(
        scheme: 'http',
        host: InternetAddress.loopbackIPv4.address,
        port: server.port,
      ),
      token: 'refresh-test-token',
    );
  });

  tearDown(() async {
    if (!eventController.isClosed) await eventController.close();
    await server.close(force: true);
  });

  test(
    'debounces telemetry events and loads data for the active page',
    () async {
      final coordinator = DashboardRefreshCoordinator(
        loadData:
            ({
              required includeSessions,
              required includeProcesses,
              required includeTools,
              required includeTrends,
            }) async {
              dashboardLoads++;
              loadOptions.add({
                'sessions': includeSessions,
                'processes': includeProcesses,
                'tools': includeTools,
                'trends': includeTrends,
              });
            },
        loadRetention: () async => retentionLoads++,
        currentPage: () => currentPage,
        eventSourceFactory: (_) async => _TestDashboardEventSource(
          eventController.stream,
          () => unawaited(eventController.close()),
        ),
        pollingInterval: const Duration(hours: 1),
        sessionRefreshInterval: const Duration(hours: 1),
        eventDebounce: const Duration(milliseconds: 40),
        reconnectDelay: const Duration(hours: 1),
      );
      addTearDown(coordinator.dispose);

      coordinator.start(api);
      await eventSubscribed.future;
      eventController.add('event: telemetry');
      await Future<void>.delayed(const Duration(milliseconds: 5));
      eventController.add('event: telemetry');
      eventController.add('event: telemetry');
      await Future<void>.delayed(const Duration(milliseconds: 80));

      expect(dashboardLoads, 1);
      expect(loadOptions.single, {
        'sessions': true,
        'processes': false,
        'tools': false,
        'trends': true,
      });

      currentPage = 3;
      eventController.add('event: telemetry');
      await Future<void>.delayed(const Duration(milliseconds: 80));

      expect(dashboardLoads, 2);
      expect(loadOptions.last, {
        'sessions': true,
        'processes': false,
        'tools': false,
        'trends': false,
      });
      expect(retentionLoads, 1);
    },
  );

  test('polls the active feature and stops when detached', () async {
    currentPage = 2;
    final coordinator = DashboardRefreshCoordinator(
      loadData:
          ({
            required includeSessions,
            required includeProcesses,
            required includeTools,
            required includeTrends,
          }) async {
            dashboardLoads++;
            loadOptions.add({
              'sessions': includeSessions,
              'processes': includeProcesses,
              'tools': includeTools,
              'trends': includeTrends,
            });
          },
      loadRetention: () async => retentionLoads++,
      currentPage: () => currentPage,
      eventSourceFactory: (_) async => _TestDashboardEventSource(
        eventController.stream,
        () => unawaited(eventController.close()),
      ),
      pollingInterval: const Duration(milliseconds: 20),
      sessionRefreshInterval: const Duration(hours: 1),
      reconnectDelay: const Duration(hours: 1),
    );

    coordinator.start(api);
    await Future<void>.delayed(const Duration(milliseconds: 60));
    coordinator.stop();
    final loadsAtStop = dashboardLoads;
    await Future<void>.delayed(const Duration(milliseconds: 60));

    expect(loadsAtStop, greaterThan(0));
    expect(dashboardLoads, loadsAtStop);
    expect(loadOptions.first['tools'], isTrue);
    expect(requests, isEmpty);
  });
}

class _TestDashboardEventSource implements DashboardEventSource {
  const _TestDashboardEventSource(this.lines, this._onClose);

  @override
  final Stream<String> lines;

  final void Function() _onClose;

  @override
  void close() => _onClose();
}
