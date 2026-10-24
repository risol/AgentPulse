import 'package:flutter/material.dart';

import '../../core/api/local_api_client.dart';
import '../../ui/components/ap_components.dart';
import '../../ui/theme/ap_colors.dart';
import '../../ui/theme/ap_spacing.dart';
import '../../ui/theme/ap_typography.dart';

class DiagnosticsPage extends StatefulWidget {
  const DiagnosticsPage({
    super.key,
    required this.apiClient,
    required this.status,
    required this.backendVersion,
    this.retention = const {},
    required this.onRefresh,
  });
  final LocalApiClient? apiClient;
  final Map<String, dynamic> status;
  final String backendVersion;
  final Map<String, dynamic> retention;
  final VoidCallback onRefresh;
  @override
  State<DiagnosticsPage> createState() => _DiagnosticsPageState();
}

class _DiagnosticsPageState extends State<DiagnosticsPage> {
  Map<String, dynamic> _ingest = {};
  Object? _error;
  bool _loading = false;
  int _generation = 0;
  @override
  void initState() {
    super.initState();
    _load();
  }

  @override
  void didUpdateWidget(covariant DiagnosticsPage oldWidget) {
    super.didUpdateWidget(oldWidget);
    if (!identical(oldWidget.apiClient, widget.apiClient) ||
        oldWidget.status != widget.status) {
      _load();
    }
  }

  @override
  void dispose() {
    _generation++;
    super.dispose();
  }

  Future<void> _load() async {
    final api = widget.apiClient;
    final gen = ++_generation;
    if (api == null) {
      setState(() => _error = 'Local backend is unavailable.');
      return;
    }
    setState(() => _loading = true);
    try {
      final value = await api.get('/api/v1/diagnostics/ingest');
      if (mounted && gen == _generation) setState(() => _ingest = value);
    } catch (error) {
      if (mounted && gen == _generation) setState(() => _error = error);
    } finally {
      if (mounted && gen == _generation) setState(() => _loading = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final signals =
        (widget.status['signals'] as Map?)?.cast<String, dynamic>() ??
        const <String, dynamic>{};
    final queue =
        (widget.status['ingest_queue'] as Map?)?.cast<String, dynamic>() ??
        const <String, dynamic>{};
    final recent = ((_ingest['recent'] as List?) ?? const [])
        .whereType<Map>()
        .map((e) => e.cast<String, dynamic>())
        .toList();
    final receiverAddress =
        widget.status['receiver_address'] as String? ?? '127.0.0.1:4318';
    final receiverReady = widget.status['receiver'] == 'ready';
    return ListView(
      padding: const EdgeInsets.all(ApSpacing.page),
      children: [
        Row(
          children: [
            const Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text('Diagnostics', style: ApTypography.pageTitle),
                  SizedBox(height: 3),
                  Text(
                    'Receiver, ingest pipeline, storage and backend health',
                    style: ApTypography.small,
                  ),
                ],
              ),
            ),
            if (_loading)
              const SizedBox(
                width: 14,
                height: 14,
                child: CircularProgressIndicator(strokeWidth: 2),
              ),
            IconButton(
              onPressed: () {
                widget.onRefresh();
                _load();
              },
              tooltip: 'Refresh diagnostics',
              icon: const Icon(Icons.refresh),
            ),
          ],
        ),
        if (_error != null) ...[
          const SizedBox(height: 12),
          ApErrorState(
            message: 'Unable to load ingest diagnostics: $_error',
            onRetry: _load,
          ),
        ],
        const SizedBox(height: 14),
        LayoutBuilder(
          builder: (context, c) {
            final count = c.maxWidth < 740 ? 1 : 2;
            return GridView.count(
              shrinkWrap: true,
              physics: const NeverScrollableScrollPhysics(),
              crossAxisCount: count,
              crossAxisSpacing: 10,
              mainAxisSpacing: 10,
              childAspectRatio: count == 1 ? 4.0 : 2.25,
              children: [
                ApPanel(
                  title: 'OTLP RECEIVER',
                  trailing: ApStatusBadge(
                    label: receiverReady ? 'HEALTHY' : 'UNAVAILABLE',
                    tone: receiverReady ? ApTone.success : ApTone.warning,
                  ),
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Text(
                        receiverReady
                            ? 'Listening on loopback'
                            : 'Receiver status is unavailable',
                        style: ApTypography.small,
                      ),
                      const SizedBox(height: 6),
                      ApCodeValue(receiverAddress),
                    ],
                  ),
                ),
                ApPanel(
                  title: 'INGEST QUEUE',
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Text(
                        '${queue['depth'] ?? 0} / ${queue['capacity'] ?? '—'} queued',
                        style: ApTypography.sectionTitle,
                      ),
                      const SizedBox(height: 6),
                      LinearProgressIndicator(
                        value:
                            (queue['depth'] is num &&
                                queue['capacity'] is num &&
                                (queue['capacity'] as num) > 0)
                            ? ((queue['depth'] as num) /
                                      (queue['capacity'] as num))
                                  .clamp(0, 1)
                                  .toDouble()
                            : 0,
                        color: (queue['rejected'] as num? ?? 0) > 0
                            ? ApColors.warning
                            : ApColors.accent,
                      ),
                      const SizedBox(height: 6),
                      Text(
                        'Rejected ${queue['rejected'] ?? 0}',
                        style: ApTypography.micro,
                      ),
                    ],
                  ),
                ),
              ],
            );
          },
        ),
        const SizedBox(height: 10),
        ApPanel(
          title: 'SIGNALS',
          trailing: Text(
            'last 24h: ${widget.status['schema_version'] ?? 'schema'}',
            style: ApTypography.micro,
          ),
          padding: const EdgeInsets.fromLTRB(0, 0, 0, 5),
          child: Column(
            children: [
              Container(
                height: 32,
                color: ApColors.surface2,
                padding: const EdgeInsets.symmetric(horizontal: 14),
                child: const Row(
                  children: [
                    Expanded(child: Text('SIGNAL')),
                    Expanded(child: Text('STATE')),
                    Expanded(flex: 2, child: Text('LAST RECEIVED')),
                    Expanded(child: Text('STATUS')),
                  ],
                ),
              ),
              for (final name in ['logs', 'metrics', 'traces'])
                _signalRow(name, signals[name]),
            ],
          ),
        ),
        const SizedBox(height: 10),
        LayoutBuilder(
          builder: (context, c) {
            final wide = c.maxWidth > 1050;
            final details = _ingestPanel(recent);
            final backend = _backendPanel();
            if (wide) {
              return Row(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Expanded(child: details),
                  const SizedBox(width: 10),
                  Expanded(child: backend),
                ],
              );
            }
            return Column(
              children: [details, const SizedBox(height: 10), backend],
            );
          },
        ),
        const SizedBox(height: 10),
        ApPanel(
          title: 'STORAGE',
          child: Wrap(
            spacing: 28,
            runSpacing: 10,
            children: [
              _storageValue(
                'Database size',
                _formatBytes(
                  widget.retention['database_size_bytes'] ??
                      widget.retention['database_bytes'],
                ),
              ),
              _storageValue(
                'Retention',
                widget.retention['retention_days'] == null
                    ? '—'
                    : '${widget.retention['retention_days']} days',
              ),
              _storageValue(
                'Schema version',
                '${widget.status['schema_version'] ?? '—'}',
              ),
            ],
          ),
        ),
      ],
    );
  }

  Widget _signalRow(String name, dynamic raw) {
    final value =
        (raw as Map?)?.cast<String, dynamic>() ?? const <String, dynamic>{};
    final state = value['state'] as String? ?? 'missing';
    final tone = state == 'receiving'
        ? ApTone.success
        : state == 'stale'
        ? ApTone.warning
        : ApTone.neutral;
    return Container(
      height: 38,
      padding: const EdgeInsets.symmetric(horizontal: 14),
      decoration: const BoxDecoration(
        border: Border(bottom: BorderSide(color: ApColors.borderMuted)),
      ),
      child: Row(
        children: [
          Expanded(child: Text(name.toUpperCase(), style: ApTypography.small)),
          Expanded(child: Text(state, style: ApTypography.small)),
          Expanded(
            flex: 2,
            child: Text(
              value['last_received_at'] as String? ?? 'No telemetry received',
              style: ApTypography.code,
            ),
          ),
          Expanded(
            child: ApStatusBadge(label: state.toUpperCase(), tone: tone),
          ),
        ],
      ),
    );
  }

  Widget _ingestPanel(List<Map<String, dynamic>> recent) => ApPanel(
    title: 'INGEST DIAGNOSTICS',
    trailing: Text(
      '${_ingest['count_last_24_hours'] ?? 0} events / 24h',
      style: ApTypography.micro,
    ),
    padding: const EdgeInsets.fromLTRB(0, 0, 0, 5),
    child: recent.isEmpty
        ? const SizedBox(
            height: 120,
            child: ApEmptyState(title: 'No ingest errors recorded'),
          )
        : Column(
            children: [
              for (final item in recent)
                ListTile(
                  dense: true,
                  leading: const Icon(
                    Icons.warning_amber_rounded,
                    color: ApColors.warning,
                    size: 17,
                  ),
                  title: Text(
                    '${item['code'] ?? 'diagnostic'}',
                    style: ApTypography.small,
                  ),
                  subtitle: Text(
                    '${item['signal'] ?? 'unknown'} · ${item['occurred_at_ns'] ?? ''}',
                    style: ApTypography.micro,
                  ),
                ),
            ],
          ),
  );
  Widget _backendPanel() => ApPanel(
    title: 'BACKEND',
    child: Wrap(
      spacing: 28,
      runSpacing: 14,
      children: [
        _storageValue('Version', widget.backendVersion),
        _storageValue(
          'API',
          widget.apiClient == null ? 'Offline' : 'Connected',
        ),
        _storageValue(
          'Receiver',
          widget.status['receiver'] as String? ?? 'unknown',
        ),
        _storageValue(
          'Process monitor',
          ((widget.status['process_monitor'] as Map?)?['state'])?.toString() ??
              'unknown',
        ),
      ],
    ),
  );
  Widget _storageValue(String label, String value) => Column(
    crossAxisAlignment: CrossAxisAlignment.start,
    mainAxisSize: MainAxisSize.min,
    children: [
      Text(label.toUpperCase(), style: ApTypography.micro),
      const SizedBox(height: 3),
      Text(value, style: ApTypography.body),
    ],
  );
  String _formatBytes(Object? raw) {
    if (raw is! num) return '—';
    if (raw < 1024) return '${raw.toInt()} B';
    if (raw < 1024 * 1024) return '${(raw / 1024).toStringAsFixed(1)} KB';
    return '${(raw / 1024 / 1024).toStringAsFixed(1)} MB';
  }
}
