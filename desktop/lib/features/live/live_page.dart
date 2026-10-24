import 'package:flutter/material.dart';

import '../../ui/components/ap_components.dart';
import '../../ui/charts/metric_line_chart.dart';
import '../../ui/theme/ap_colors.dart';
import '../../ui/theme/ap_spacing.dart';
import '../../ui/theme/ap_typography.dart';

class LivePage extends StatelessWidget {
  const LivePage({
    super.key,
    required this.status,
    required this.summary,
    required this.sessions,
    required this.toolSummaries,
    required this.toolCalls,
    required this.processes,
    required this.trendPoints,
    required this.starting,
    this.error,
    required this.loading,
    required this.onRefresh,
    required this.onOpenRun,
    required this.onOpenRuns,
  });
  final Map<String, dynamic> status;
  final Map<String, dynamic> summary;
  final List<Map<String, dynamic>> sessions;
  final List<Map<String, dynamic>> toolSummaries;
  final List<Map<String, dynamic>> toolCalls;
  final List<Map<String, dynamic>> processes;
  final List<Map<String, dynamic>> trendPoints;
  final bool starting;
  final String? error;
  final bool loading;
  final VoidCallback onRefresh;
  final ValueChanged<Map<String, dynamic>> onOpenRun;
  final VoidCallback onOpenRuns;

  Map<String, dynamic>? get _recentRun =>
      sessions.isEmpty ? null : sessions.first;
  DateTime? _date(Object? value) =>
      value is String ? DateTime.tryParse(value)?.toLocal() : null;
  String _time(Object? value) {
    final at = _date(value);
    if (at == null) return '—';
    String two(int v) => v.toString().padLeft(2, '0');
    return '${two(at.hour)}:${two(at.minute)}:${two(at.second)}';
  }

  @override
  Widget build(BuildContext context) {
    final recent = _recentRun;
    final lastActivity = _date(recent?['last_activity_at']);
    final recentlyActive =
        lastActivity != null &&
        DateTime.now().difference(lastActivity).inMinutes < 2;
    final model = recent?['model'] as String? ?? 'Waiting for Codex';
    final latestTool = toolCalls.isEmpty ? null : toolCalls.first;
    final activeProcesses = processes
        .where((p) => p['ended_at'] == null)
        .toList();
    final newestTrend = trendPoints.reversed
        .cast<Map<String, dynamic>?>()
        .firstWhere(
          (point) => point != null && point['has_data'] == true,
          orElse: () => null,
        );
    final ttft = newestTrend?['average_ttft_ms'];
    final outputRate = newestTrend?['average_request_rate_tokens_per_second'];
    final toolP95 = toolSummaries
        .map((item) => item['p95_duration_ms'])
        .whereType<num>()
        .fold<num?>(null, (a, b) => a == null || b > a ? b : a);
    final failureCount = toolSummaries.fold<num>(
      0,
      (sum, item) => sum + ((item['failure_calls'] as num?) ?? 0),
    );
    final observedDuration = apDuration(recent?['activity_duration_ms']);
    final quality = ApQualityBadge.fromValue(recent?['data_quality']);

    return RefreshIndicator(
      onRefresh: () async => onRefresh(),
      child: ListView(
        padding: const EdgeInsets.all(ApSpacing.page),
        children: [
          Row(
            children: [
              const Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text('Live', style: ApTypography.pageTitle),
                    SizedBox(height: 3),
                    Text('Codex runtime activity', style: ApTypography.small),
                  ],
                ),
              ),
              if (loading)
                const SizedBox(
                  width: 15,
                  height: 15,
                  child: CircularProgressIndicator(strokeWidth: 2),
                ),
              IconButton(
                onPressed: onRefresh,
                tooltip: 'Refresh',
                icon: const Icon(Icons.refresh),
              ),
              TextButton(onPressed: onOpenRuns, child: const Text('All runs')),
            ],
          ),
          if (error != null) ...[
            const SizedBox(height: 12),
            ApErrorState(message: error!, onRetry: onRefresh),
          ],
          const SizedBox(height: 14),
          _ActiveRunHero(
            run: recent,
            receiving: _receiving,
            degraded: error != null,
            recentlyActive: recentlyActive,
            model: model,
            elapsed: observedDuration,
            latestTool: latestTool,
            process: activeProcesses.isEmpty ? null : activeProcesses.first,
            onOpen: recent == null ? null : () => onOpenRun(recent),
          ),
          const SizedBox(height: 12),
          LayoutBuilder(
            builder: (context, constraints) {
              final count = constraints.maxWidth < 760 ? 2 : 6;
              return GridView.count(
                crossAxisCount: count,
                shrinkWrap: true,
                physics: const NeverScrollableScrollPhysics(),
                crossAxisSpacing: 8,
                mainAxisSpacing: 8,
                childAspectRatio: count == 2 ? 2.1 : 1.65,
                children: [
                  ApMetricTile(
                    label: 'TTFT',
                    value: ttft is num ? apDuration(ttft) : '—',
                    icon: Icons.timer_outlined,
                    quality: ttft is num
                        ? ApQuality.partial
                        : ApQuality.unavailable,
                  ),
                  ApMetricTile(
                    label: 'Output rate',
                    value: outputRate is num
                        ? outputRate.toStringAsFixed(1)
                        : '—',
                    unit: outputRate is num ? 'tok/s' : null,
                    icon: Icons.speed,
                    quality: outputRate is num
                        ? ApQuality.partial
                        : ApQuality.unavailable,
                  ),
                  ApMetricTile(
                    label: 'Tool P95',
                    value: toolP95 == null ? '—' : apDuration(toolP95),
                    icon: Icons.build_outlined,
                    quality: toolP95 == null
                        ? ApQuality.unavailable
                        : ApQuality.complete,
                  ),
                  ApMetricTile(
                    label: 'Failures',
                    value: apCompactNumber(failureCount),
                    icon: Icons.error_outline,
                    tint: failureCount > 0 ? ApColors.danger : null,
                  ),
                  ApMetricTile(
                    label: 'Active proc',
                    value: '${activeProcesses.length}',
                    icon: Icons.memory_outlined,
                  ),
                  ApMetricTile(
                    label: 'Run quality',
                    value: recent == null ? '—' : _qualityLabel(quality),
                    icon: Icons.fact_check_outlined,
                    quality: recent == null ? ApQuality.unavailable : quality,
                  ),
                ],
              );
            },
          ),
          const SizedBox(height: 12),
          LayoutBuilder(
            builder: (context, constraints) {
              final wide = constraints.maxWidth >= 1040;
              final charts = Column(
                children: [
                  Row(
                    children: [
                      Expanded(
                        child: _TrendPanel(
                          title: 'Output token rate',
                          values: trendPoints
                              .map(
                                (p) =>
                                    p['average_request_rate_tokens_per_second'],
                              )
                              .toList(),
                          color: ApColors.model,
                          unit: 'tok/s',
                        ),
                      ),
                      const SizedBox(width: 10),
                      Expanded(
                        child: _TrendPanel(
                          title: 'TTFT',
                          values: trendPoints
                              .map((p) => p['average_ttft_ms'])
                              .toList(),
                          color: ApColors.accentStrong,
                          unit: 'ms',
                        ),
                      ),
                    ],
                  ),
                  const SizedBox(height: 10),
                  Row(
                    children: [
                      Expanded(
                        child: _TrendPanel(
                          title: 'Model latency',
                          values: trendPoints
                              .map((p) => p['average_duration_ms'])
                              .toList(),
                          color: ApColors.tool,
                          unit: 'ms',
                        ),
                      ),
                      const SizedBox(width: 10),
                      Expanded(
                        child: _TrendPanel(
                          title: 'Tool duration',
                          values: toolSummaries
                              .map((p) => p['p95_duration_ms'])
                              .toList(),
                          color: ApColors.command,
                          unit: 'ms',
                        ),
                      ),
                    ],
                  ),
                ],
              );
              final activity = _ActivityPanel(
                toolCalls: toolCalls,
                processes: activeProcesses,
                timeLabel: _time,
              );
              if (wide) {
                return Row(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Expanded(flex: 5, child: activity),
                    const SizedBox(width: 12),
                    Expanded(flex: 7, child: charts),
                  ],
                );
              }
              return Column(
                children: [activity, const SizedBox(height: 12), charts],
              );
            },
          ),
          const SizedBox(height: 12),
          _RecentRunsPanel(
            sessions: sessions.take(6).toList(),
            onOpen: onOpenRun,
            onSeeAll: onOpenRuns,
            timeLabel: _time,
          ),
        ],
      ),
    );
  }

  bool get _receiving {
    final values = (status['signals'] as Map?)?.values ?? const [];
    return values.any((value) => value is Map && value['state'] == 'receiving');
  }

  String _qualityLabel(ApQuality quality) => switch (quality) {
    ApQuality.complete => 'Complete',
    ApQuality.partial => 'Partial',
    ApQuality.exact => 'Exact',
    ApQuality.inferred => 'Inferred',
    ApQuality.estimated => 'Estimated',
    ApQuality.unavailable => 'Unavailable',
  };
}

class _ActiveRunHero extends StatelessWidget {
  const _ActiveRunHero({
    required this.run,
    required this.receiving,
    required this.degraded,
    required this.recentlyActive,
    required this.model,
    required this.elapsed,
    required this.latestTool,
    required this.process,
    this.onOpen,
  });
  final Map<String, dynamic>? run;
  final bool receiving;
  final bool degraded;
  final bool recentlyActive;
  final String model;
  final String elapsed;
  final Map<String, dynamic>? latestTool;
  final Map<String, dynamic>? process;
  final VoidCallback? onOpen;

  @override
  Widget build(BuildContext context) {
    final exists = run != null;
    return ApPanel(
      title: 'LATEST OBSERVED RUN',
      trailing: exists
          ? ApStatusBadge(
              label: recentlyActive ? 'RECENT ACTIVITY' : 'IDLE',
              tone: recentlyActive ? ApTone.success : ApTone.neutral,
              pulse: recentlyActive,
            )
          : ApStatusBadge(
              label: degraded
                  ? 'DEGRADED'
                  : receiving
                  ? 'MONITORING'
                  : 'WAITING',
              tone: degraded || !receiving ? ApTone.warning : ApTone.info,
            ),
      padding: const EdgeInsets.fromLTRB(16, 4, 16, 14),
      child: exists
          ? Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Row(
                  children: [
                    Expanded(
                      child: Text(
                        run?['model'] as String? ?? model,
                        style: ApTypography.pageTitle,
                      ),
                    ),
                    Text('observed $elapsed', style: ApTypography.code),
                    const SizedBox(width: 8),
                    IconButton(
                      onPressed: onOpen,
                      tooltip: 'Open run',
                      icon: const Icon(Icons.open_in_new, size: 16),
                    ),
                  ],
                ),
                Text(
                  'Run ${run?['external_id'] ?? run?['id'] ?? '—'}',
                  maxLines: 1,
                  overflow: TextOverflow.ellipsis,
                  style: ApTypography.small.copyWith(color: ApColors.textMuted),
                ),
                const SizedBox(height: 14),
                Wrap(
                  spacing: 32,
                  runSpacing: 10,
                  children: [
                    _HeroItem(
                      label: 'MODEL',
                      value: run?['model'] as String? ?? 'Unknown',
                      tone: ApColors.model,
                    ),
                    _HeroItem(
                      label: 'LATEST TOOL',
                      value: latestTool?['tool_name'] as String? ?? '—',
                      tone: ApColors.tool,
                    ),
                    _HeroItem(
                      label: 'COMMAND / PROCESS',
                      value: process?['image_name'] as String? ?? '—',
                      tone: ApColors.process,
                    ),
                    _HeroItem(
                      label: 'TOKENS',
                      value:
                          '${apCompactNumber(run?['input_tokens'])} in · ${apCompactNumber(run?['output_tokens'])} out',
                      tone: ApColors.textSecondary,
                    ),
                  ],
                ),
              ],
            )
          : Row(
              children: [
                Container(
                  width: 36,
                  height: 36,
                  decoration: BoxDecoration(
                    color: ApColors.surface2,
                    borderRadius: BorderRadius.circular(8),
                  ),
                  child: Icon(
                    Icons.radar,
                    color: receiving ? ApColors.info : ApColors.textMuted,
                  ),
                ),
                const SizedBox(width: 12),
                const Expanded(
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Text(
                        'Monitoring Codex',
                        style: ApTypography.sectionTitle,
                      ),
                      SizedBox(height: 3),
                      Text(
                        'Waiting for the next run',
                        style: ApTypography.small,
                      ),
                    ],
                  ),
                ),
              ],
            ),
    );
  }
}

class _HeroItem extends StatelessWidget {
  const _HeroItem({
    required this.label,
    required this.value,
    required this.tone,
  });
  final String label;
  final String value;
  final Color tone;
  @override
  Widget build(BuildContext context) => ConstrainedBox(
    constraints: const BoxConstraints(minWidth: 130, maxWidth: 250),
    child: Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(label, style: ApTypography.micro.copyWith(letterSpacing: .6)),
        const SizedBox(height: 3),
        Text(
          value,
          maxLines: 1,
          overflow: TextOverflow.ellipsis,
          style: ApTypography.body.copyWith(
            color: tone,
            fontWeight: FontWeight.w600,
          ),
        ),
      ],
    ),
  );
}

class _ActivityPanel extends StatelessWidget {
  const _ActivityPanel({
    required this.toolCalls,
    required this.processes,
    required this.timeLabel,
  });
  final List<Map<String, dynamic>> toolCalls;
  final List<Map<String, dynamic>> processes;
  final String Function(Object?) timeLabel;
  @override
  Widget build(BuildContext context) {
    final entries = <_ActivityEntry>[];
    for (final call in toolCalls.take(8)) {
      entries.add(
        _ActivityEntry(
          timeLabel(
            call['started_at_ns'] == null
                ? null
                : DateTime.fromMillisecondsSinceEpoch(
                    (call['started_at_ns'] as num).toInt() ~/ 1000000,
                    isUtc: true,
                  ).toIso8601String(),
          ),
          'TOOL',
          call['tool_name'] as String? ?? 'tool',
          call['status'] as String? ?? 'observed',
          ApColors.tool,
        ),
      );
    }
    for (final process in processes.take(5)) {
      entries.add(
        _ActivityEntry(
          timeLabel(process['started_at']),
          'PROCESS',
          process['image_name'] as String? ?? 'process',
          'PID ${process['pid'] ?? '—'}',
          ApColors.process,
        ),
      );
    }
    entries.sort((a, b) => b.time.compareTo(a.time));
    return ApPanel(
      title: 'LIVE ACTIVITY',
      trailing: Text('${entries.length} events', style: ApTypography.micro),
      padding: const EdgeInsets.fromLTRB(0, 0, 0, 6),
      child: entries.isEmpty
          ? const SizedBox(
              height: 160,
              child: ApEmptyState(
                title: 'No recent activity',
                message: 'New tool calls and processes will appear here.',
              ),
            )
          : Column(
              children: [
                for (final e in entries.take(9))
                  Container(
                    height: 31,
                    padding: const EdgeInsets.symmetric(horizontal: 14),
                    child: Row(
                      children: [
                        Text(
                          e.time,
                          style: ApTypography.code.copyWith(
                            color: ApColors.textMuted,
                          ),
                        ),
                        const SizedBox(width: 10),
                        SizedBox(
                          width: 58,
                          child: Text(
                            e.kind,
                            style: ApTypography.micro.copyWith(
                              color: e.color,
                              fontWeight: FontWeight.w700,
                            ),
                          ),
                        ),
                        Expanded(
                          child: Text(
                            e.name,
                            maxLines: 1,
                            overflow: TextOverflow.ellipsis,
                            style: ApTypography.small,
                          ),
                        ),
                        Text(e.detail, style: ApTypography.micro),
                      ],
                    ),
                  ),
              ],
            ),
    );
  }
}

class _ActivityEntry {
  const _ActivityEntry(
    this.time,
    this.kind,
    this.name,
    this.detail,
    this.color,
  );
  final String time, kind, name, detail;
  final Color color;
}

class _TrendPanel extends StatelessWidget {
  const _TrendPanel({
    required this.title,
    required this.values,
    required this.color,
    required this.unit,
  });
  final String title;
  final List<Object?> values;
  final Color color;
  final String unit;
  @override
  Widget build(BuildContext context) {
    final points = values.whereType<num>().map((e) => e.toDouble()).toList();
    final last = points.isEmpty ? null : points.last;
    return ApPanel(
      title: title,
      trailing: Text(
        last == null ? '—' : '${last.toStringAsFixed(1)} $unit',
        style: ApTypography.code.copyWith(color: color),
      ),
      padding: const EdgeInsets.fromLTRB(14, 0, 14, 12),
      child: MetricLineChart(values: points, color: color, height: 116),
    );
  }
}

class _RecentRunsPanel extends StatelessWidget {
  const _RecentRunsPanel({
    required this.sessions,
    required this.onOpen,
    required this.onSeeAll,
    required this.timeLabel,
  });
  final List<Map<String, dynamic>> sessions;
  final ValueChanged<Map<String, dynamic>> onOpen;
  final VoidCallback onSeeAll;
  final String Function(Object?) timeLabel;
  @override
  Widget build(BuildContext context) => ApPanel(
    title: 'RECENT RUNS',
    trailing: TextButton(onPressed: onSeeAll, child: const Text('View all')),
    padding: const EdgeInsets.fromLTRB(0, 0, 0, 5),
    child: sessions.isEmpty
        ? const SizedBox(
            height: 150,
            child: ApEmptyState(
              title: 'No runs yet',
              message:
                  'AgentPulse is connected and waiting for Codex activity.',
            ),
          )
        : Column(
            children: [
              Container(
                height: 30,
                color: ApColors.surface2,
                padding: const EdgeInsets.symmetric(horizontal: 14),
                child: const Row(
                  children: [
                    Expanded(flex: 2, child: Text('STARTED')),
                    Expanded(flex: 3, child: Text('MODEL')),
                    Expanded(child: Text('DURATION')),
                    Expanded(child: Text('TOKENS')),
                    Expanded(child: Text('TOOLS')),
                    Expanded(child: Text('FAIL')),
                  ],
                ),
              ),
              for (final run in sessions)
                InkWell(
                  onDoubleTap: () => onOpen(run),
                  onTap: () => onOpen(run),
                  child: Container(
                    height: 36,
                    padding: const EdgeInsets.symmetric(horizontal: 14),
                    decoration: const BoxDecoration(
                      border: Border(
                        bottom: BorderSide(color: ApColors.borderMuted),
                      ),
                    ),
                    child: Row(
                      children: [
                        Expanded(
                          flex: 2,
                          child: Text(
                            timeLabel(run['started_at']),
                            style: ApTypography.code,
                          ),
                        ),
                        Expanded(
                          flex: 3,
                          child: Text(
                            run['model'] as String? ?? 'Unknown',
                            maxLines: 1,
                            overflow: TextOverflow.ellipsis,
                            style: ApTypography.small,
                          ),
                        ),
                        Expanded(
                          child: Text(
                            apDuration(run['activity_duration_ms']),
                            style: ApTypography.small,
                          ),
                        ),
                        Expanded(
                          child: Text(
                            '${apCompactNumber(run['input_tokens'])} / ${apCompactNumber(run['output_tokens'])}',
                            style: ApTypography.small,
                          ),
                        ),
                        Expanded(
                          child: Text(
                            '${run['tool_call_count'] ?? 0}',
                            style: ApTypography.small,
                          ),
                        ),
                        Expanded(
                          child: Text(
                            '${run['failed_call_count'] ?? 0}',
                            style: ApTypography.small.copyWith(
                              color: (run['failed_call_count'] as num? ?? 0) > 0
                                  ? ApColors.danger
                                  : ApColors.textSecondary,
                            ),
                          ),
                        ),
                      ],
                    ),
                  ),
                ),
            ],
          ),
  );
}
