import 'package:flutter/material.dart';

import '../../ui/components/ap_components.dart';
import '../../ui/theme/ap_colors.dart';
import '../../ui/theme/ap_spacing.dart';
import '../../ui/theme/ap_typography.dart';

class ToolAnalyticsPage extends StatefulWidget {
  const ToolAnalyticsPage({
    super.key,
    required this.range,
    required this.summaries,
    required this.calls,
    required this.loading,
    required this.loadingMore,
    required this.nextCursor,
    this.error,
    required this.onRangeChanged,
    required this.onRefresh,
    required this.onLoadMore,
  });
  final String range;
  final List<Map<String, dynamic>> summaries;
  final List<Map<String, dynamic>> calls;
  final bool loading;
  final bool loadingMore;
  final String? nextCursor;
  final String? error;
  final ValueChanged<String> onRangeChanged;
  final VoidCallback onRefresh;
  final VoidCallback onLoadMore;
  @override
  State<ToolAnalyticsPage> createState() => _ToolAnalyticsPageState();
}

class _ToolAnalyticsPageState extends State<ToolAnalyticsPage> {
  Map<String, dynamic>? _selectedCall;
  String _sortKey = 'total_calls';
  bool _descending = true;
  bool _slowOnly = false;

  List<Map<String, dynamic>> get _sortedSummaries {
    final values = [...widget.summaries];
    values.sort((a, b) {
      final left = a[_sortKey];
      final right = b[_sortKey];
      int order = left is num && right is num
          ? left.compareTo(right)
          : '${left ?? ''}'.compareTo('${right ?? ''}');
      return _descending ? -order : order;
    });
    return values;
  }

  void _sortBy(String key) => setState(() {
    if (_sortKey == key) {
      _descending = !_descending;
    } else {
      _sortKey = key;
      _descending = true;
    }
  });

  Widget _sortHeader(String label, String key, {int flex = 1}) => Expanded(
    flex: flex,
    child: InkWell(
      onTap: () => _sortBy(key),
      child: Row(
        children: [
          Flexible(
            child: Text(
              label,
              maxLines: 1,
              overflow: TextOverflow.ellipsis,
              style: ApTypography.micro,
            ),
          ),
          if (_sortKey == key)
            Icon(
              _descending ? Icons.arrow_downward : Icons.arrow_upward,
              size: 11,
              color: ApColors.accentStrong,
            ),
        ],
      ),
    ),
  );

  bool _failed(Map<String, dynamic> call) {
    final value = '${call['status'] ?? ''}'.toLowerCase();
    return value.contains('fail') || value.contains('error');
  }

  bool _succeeded(Map<String, dynamic> call) {
    final value = '${call['status'] ?? call['outcome'] ?? ''}'.toLowerCase();
    return value.contains('success') || value.contains('complete');
  }

  String _time(Object? value) {
    if (value is! num || value <= 0) return '—';
    final date = DateTime.fromMillisecondsSinceEpoch(
      value.toInt() ~/ 1000000,
      isUtc: true,
    ).toLocal();
    String two(int x) => x.toString().padLeft(2, '0');
    return '${two(date.hour)}:${two(date.minute)}:${two(date.second)}';
  }

  num? _maxDuration(String toolName) => widget.calls
      .where((call) => call['tool_name'] == toolName)
      .map((call) => call['duration_ms'])
      .whereType<num>()
      .fold<num?>(
        null,
        (best, current) => best == null || current > best ? current : best,
      );

  @override
  Widget build(BuildContext context) {
    final total = widget.summaries.fold<num>(
      0,
      (sum, item) => sum + ((item['total_calls'] as num?) ?? 0),
    );
    final failed = widget.summaries.fold<num>(
      0,
      (sum, item) => sum + ((item['failure_calls'] as num?) ?? 0),
    );
    final p50s =
        widget.summaries
            .map((x) => x['p50_duration_ms'])
            .whereType<num>()
            .toList()
          ..sort();
    final p95s =
        widget.summaries
            .map((x) => x['p95_duration_ms'])
            .whereType<num>()
            .toList()
          ..sort();
    final retryCount = widget.calls
        .where((call) => (call['retry_index'] as num? ?? 0) > 0)
        .length;
    final slowCalls = widget.calls
        .where((call) => (call['duration_ms'] as num? ?? 0) >= 1000)
        .length;
    final failureRate = total == 0 ? null : failed / total;
    final visibleCalls = widget.calls
        .where(
          (call) => !_slowOnly || (call['duration_ms'] as num? ?? 0) >= 1000,
        )
        .toList();
    return ListView(
      padding: const EdgeInsets.all(ApSpacing.page),
      children: [
        Row(
          children: [
            const Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text('Tools', style: ApTypography.pageTitle),
                  SizedBox(height: 3),
                  Text(
                    'Cross-run tool call performance and failure analysis',
                    style: ApTypography.small,
                  ),
                ],
              ),
            ),
            DropdownButton<String>(
              value: widget.range,
              items: const [
                DropdownMenuItem(value: '15m', child: Text('15 min')),
                DropdownMenuItem(value: '1h', child: Text('1 hour')),
                DropdownMenuItem(value: '6h', child: Text('6 hours')),
                DropdownMenuItem(value: '24h', child: Text('24 hours')),
                DropdownMenuItem(value: '7d', child: Text('7 days')),
              ],
              onChanged: (v) {
                if (v != null) widget.onRangeChanged(v);
              },
            ),
            IconButton(
              onPressed: widget.loading ? null : widget.onRefresh,
              tooltip: 'Refresh',
              icon: const Icon(Icons.refresh),
            ),
          ],
        ),
        if (widget.error != null) ...[
          const SizedBox(height: 12),
          ApErrorState(message: widget.error!, onRetry: widget.onRefresh),
        ],
        const SizedBox(height: 14),
        LayoutBuilder(
          builder: (context, c) {
            final count = c.maxWidth < 780 ? 3 : 6;
            return GridView.count(
              shrinkWrap: true,
              physics: const NeverScrollableScrollPhysics(),
              crossAxisCount: count,
              crossAxisSpacing: 8,
              mainAxisSpacing: 8,
              childAspectRatio: count == 3 ? 2.1 : 1.55,
              children: [
                ApMetricTile(
                  label: 'Calls',
                  value: apCompactNumber(total),
                  icon: Icons.call_made,
                ),
                ApMetricTile(
                  label: 'Failure rate',
                  value: failureRate == null
                      ? '—'
                      : '${(failureRate * 100).toStringAsFixed(1)}%',
                  icon: Icons.error_outline,
                  tint: (failureRate ?? 0) > .1 ? ApColors.danger : null,
                ),
                ApMetricTile(
                  label: 'Median P50',
                  value: p50s.isEmpty
                      ? '—'
                      : apDuration(p50s[p50s.length ~/ 2]),
                  icon: Icons.speed,
                ),
                ApMetricTile(
                  label: 'Median P95',
                  value: p95s.isEmpty
                      ? '—'
                      : apDuration(p95s[p95s.length ~/ 2]),
                  icon: Icons.trending_up,
                ),
                ApMetricTile(
                  label: 'Retries (loaded)',
                  value: '$retryCount',
                  icon: Icons.replay,
                ),
                ApMetricTile(
                  label: 'Slow calls (loaded)',
                  value: '$slowCalls',
                  icon: Icons.hourglass_bottom,
                  tint: slowCalls > 0 ? ApColors.warning : null,
                ),
              ],
            );
          },
        ),
        const SizedBox(height: 12),
        LayoutBuilder(
          builder: (context, c) {
            if (c.maxWidth < 980) {
              return Column(
                children: [
                  _performancePanel(),
                  const SizedBox(height: 10),
                  _failurePanel(),
                ],
              );
            }
            return Row(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Expanded(flex: 7, child: _performancePanel()),
                const SizedBox(width: 10),
                Expanded(flex: 4, child: _failurePanel()),
              ],
            );
          },
        ),
        const SizedBox(height: 12),
        SizedBox(
          height: 400,
          child: ApSplitView(
            primary: _callsPanel(visibleCalls),
            secondary: _inspector(_selectedCall),
            secondaryWidth: 300,
          ),
        ),
        const SizedBox(height: 8),
        Text(
          '* Maximum is computed from loaded calls. Median P50/P95 aggregate per-tool summary percentiles.',
          style: ApTypography.micro,
        ),
      ],
    );
  }

  Widget _performancePanel() => ApPanel(
    title: 'TOOL PERFORMANCE',
    trailing: Text(
      '${widget.summaries.length} tools',
      style: ApTypography.micro,
    ),
    padding: const EdgeInsets.fromLTRB(0, 0, 0, 4),
    child: widget.summaries.isEmpty && !widget.loading
        ? const SizedBox(
            height: 180,
            child: ApEmptyState(
              title: 'No tool calls',
              message: 'Tool metrics will appear after Codex uses a tool.',
            ),
          )
        : Column(
            children: [
              Container(
                height: 31,
                color: ApColors.surface2,
                padding: const EdgeInsets.symmetric(horizontal: 12),
                child: Row(
                  children: [
                    _sortHeader('TOOL', 'tool_name', flex: 3),
                    _sortHeader('CALLS', 'total_calls'),
                    _sortHeader('FAIL', 'failure_calls'),
                    _sortHeader('FAIL %', 'failure_rate'),
                    _sortHeader('P50', 'p50_duration_ms'),
                    _sortHeader('P95', 'p95_duration_ms'),
                    const Expanded(
                      child: Text('MAX*', style: ApTypography.micro),
                    ),
                  ],
                ),
              ),
              for (final item in _sortedSummaries)
                InkWell(
                  onTap: () => setState(() {
                    _slowOnly = false;
                  }),
                  child: Container(
                    height: 35,
                    padding: const EdgeInsets.symmetric(horizontal: 12),
                    decoration: const BoxDecoration(
                      border: Border(
                        bottom: BorderSide(color: ApColors.borderMuted),
                      ),
                    ),
                    child: Row(
                      children: [
                        Expanded(
                          flex: 3,
                          child: Text(
                            item['tool_name'] as String? ?? 'unknown',
                            style: ApTypography.small,
                          ),
                        ),
                        Expanded(
                          child: Text(
                            '${item['total_calls'] ?? 0}',
                            style: ApTypography.small,
                          ),
                        ),
                        Expanded(
                          child: Text(
                            '${item['failure_calls'] ?? 0}',
                            style: ApTypography.small.copyWith(
                              color: ((item['failure_calls'] as num?) ?? 0) > 0
                                  ? ApColors.danger
                                  : null,
                            ),
                          ),
                        ),
                        Expanded(
                          child: Text(
                            item['failure_rate'] is num
                                ? '${((item['failure_rate'] as num) * 100).toStringAsFixed(1)}%'
                                : '—',
                            style: ApTypography.small,
                          ),
                        ),
                        Expanded(
                          child: Text(
                            apDuration(item['p50_duration_ms']),
                            style: ApTypography.small,
                          ),
                        ),
                        Expanded(
                          child: Text(
                            apDuration(item['p95_duration_ms']),
                            style: ApTypography.small,
                          ),
                        ),
                        Expanded(
                          child: Text(
                            apDuration(
                              _maxDuration('${item['tool_name'] ?? ''}'),
                            ),
                            style: ApTypography.small,
                          ),
                        ),
                      ],
                    ),
                  ),
                ),
            ],
          ),
  );

  Widget _failurePanel() {
    final failed = widget.summaries.fold<num>(
      0,
      (sum, item) => sum + ((item['failure_calls'] as num?) ?? 0),
    );
    final unknown = widget.summaries.fold<num>(
      0,
      (sum, item) => sum + ((item['unknown_outcomes'] as num?) ?? 0),
    );
    final retried = widget.calls
        .where((call) => (call['retry_index'] as num? ?? 0) > 0)
        .length;
    return ApPanel(
      title: 'FAILURE / RETRY',
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          _DistributionRow(
            label: 'Failed',
            value: failed.toInt(),
            tone: ApColors.danger,
          ),
          const SizedBox(height: 12),
          _DistributionRow(
            label: 'Retried calls (loaded)',
            value: retried,
            tone: ApColors.warning,
          ),
          const SizedBox(height: 12),
          _DistributionRow(
            label: 'Unknown outcome',
            value: unknown.toInt(),
            tone: ApColors.textMuted,
          ),
          const SizedBox(height: 13),
          Text(
            'Percentiles use available non-negative durations. Retry counts reflect the loaded call page.',
            style: ApTypography.micro,
          ),
        ],
      ),
    );
  }

  Widget _callsPanel(List<Map<String, dynamic>> calls) => ApPanel(
    title: 'CALLS',
    trailing: Wrap(
      spacing: 4,
      children: [
        TextButton(
          onPressed: () => setState(() => _slowOnly = false),
          child: Text(
            'All',
            style: TextStyle(
              color: !_slowOnly ? ApColors.accentStrong : ApColors.textMuted,
            ),
          ),
        ),
        TextButton(
          onPressed: () => setState(() => _slowOnly = true),
          child: Text(
            'Slow ≥ 1s',
            style: TextStyle(
              color: _slowOnly ? ApColors.accentStrong : ApColors.textMuted,
            ),
          ),
        ),
      ],
    ),
    padding: EdgeInsets.zero,
    expandChild: true,
    child: Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        if (widget.loading && calls.isEmpty) const LinearProgressIndicator(),
        if (calls.isEmpty && !widget.loading)
          const Expanded(
            child: ApEmptyState(title: 'No calls match this view'),
          ),
        if (calls.isNotEmpty) ...[
          Container(
            height: 31,
            color: ApColors.surface2,
            padding: const EdgeInsets.symmetric(horizontal: 12),
            child: const Row(
              children: [
                Expanded(flex: 2, child: Text('TOOL')),
                Expanded(child: Text('STARTED')),
                Expanded(child: Text('DURATION')),
                Expanded(child: Text('STATUS')),
                Expanded(child: Text('RETRY')),
                Expanded(child: Text('QUALITY')),
              ],
            ),
          ),
          Expanded(
            child: ListView.builder(
              itemCount: calls.length + (widget.nextCursor == null ? 0 : 1),
              itemBuilder: (context, index) => index < calls.length
                  ? _callRow(calls[index])
                  : Padding(
                      padding: const EdgeInsets.all(9),
                      child: OutlinedButton(
                        onPressed: widget.loadingMore
                            ? null
                            : widget.onLoadMore,
                        child: Text(
                          widget.loadingMore ? 'Loading…' : 'Load more calls',
                        ),
                      ),
                    ),
            ),
          ),
        ],
      ],
    ),
  );

  Widget _callRow(Map<String, dynamic> call) => InkWell(
    onTap: () => setState(() => _selectedCall = call),
    child: Container(
      height: 35,
      padding: const EdgeInsets.symmetric(horizontal: 12),
      decoration: BoxDecoration(
        color: identical(call, _selectedCall)
            ? ApColors.accent.withValues(alpha: .1)
            : null,
        border: Border(bottom: BorderSide(color: ApColors.borderMuted)),
      ),
      child: Row(
        children: [
          Expanded(
            flex: 2,
            child: Text(
              call['tool_name'] as String? ?? 'unknown',
              style: ApTypography.small,
            ),
          ),
          Expanded(
            child: Text(_time(call['started_at_ns']), style: ApTypography.code),
          ),
          Expanded(
            child: Text(
              apDuration(call['duration_ms']),
              style: ApTypography.small,
            ),
          ),
          Expanded(
            child: Text(
              call['status'] as String? ?? 'unknown',
              style: ApTypography.small.copyWith(
                color: _failed(call) ? ApColors.danger : ApColors.textSecondary,
              ),
            ),
          ),
          Expanded(
            child: Text(
              '${call['retry_index'] ?? 0}',
              style: ApTypography.small,
            ),
          ),
          Expanded(
            child: ApQualityBadge(
              quality: ApQualityBadge.fromValue(call['data_quality']),
            ),
          ),
        ],
      ),
    ),
  );

  Widget _inspector(Map<String, dynamic>? call) => ApPanel(
    title: 'CALL INSPECTOR',
    padding: const EdgeInsets.all(14),
    child: call == null
        ? const SizedBox(
            height: 200,
            child: ApEmptyState(
              title: 'Select a call',
              message: 'Select a row to inspect safe call metadata.',
            ),
          )
        : Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(
                call['tool_name'] as String? ?? 'unknown',
                style: ApTypography.pageTitle,
              ),
              const SizedBox(height: 8),
              ApStatusBadge(
                label: call['status'] as String? ?? 'UNKNOWN',
                tone: _failed(call)
                    ? ApTone.danger
                    : _succeeded(call)
                    ? ApTone.success
                    : ApTone.neutral,
              ),
              const SizedBox(height: 16),
              _detail('Duration', apDuration(call['duration_ms'])),
              _detail('Started', _time(call['started_at_ns'])),
              _detail('Retry index', '${call['retry_index'] ?? 0}'),
              _detail('Session', '${call['session_id'] ?? 'unassigned'}'),
              _detail('Trace ID', '${call['trace_id'] ?? '—'}'),
              _detail('Span ID', '${call['span_id'] ?? '—'}'),
              _detail('Correlation', '${call['correlation_method'] ?? '—'}'),
              const SizedBox(height: 10),
              ApQualityBadge(
                quality: ApQualityBadge.fromValue(call['data_quality']),
              ),
              const SizedBox(height: 12),
              Text(
                'Tool input and output are intentionally omitted from this privacy-safe view.',
                style: ApTypography.micro,
              ),
            ],
          ),
  );

  Widget _detail(String label, String value) => Padding(
    padding: const EdgeInsets.only(bottom: 10),
    child: Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(label.toUpperCase(), style: ApTypography.micro),
        const SizedBox(height: 2),
        SelectableText(value, style: ApTypography.small),
      ],
    ),
  );
}

class _DistributionRow extends StatelessWidget {
  const _DistributionRow({
    required this.label,
    required this.value,
    required this.tone,
  });
  final String label;
  final int value;
  final Color tone;
  @override
  Widget build(BuildContext context) => Row(
    children: [
      Container(
        width: 7,
        height: 7,
        decoration: BoxDecoration(
          color: tone,
          borderRadius: BorderRadius.circular(9),
        ),
      ),
      const SizedBox(width: 8),
      Expanded(child: Text(label, style: ApTypography.small)),
      Text('$value', style: ApTypography.sectionTitle.copyWith(color: tone)),
    ],
  );
}
