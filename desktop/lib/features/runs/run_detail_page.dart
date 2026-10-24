import 'package:flutter/material.dart';

import '../../core/api/local_api_client.dart';
import '../../ui/components/ap_components.dart';
import '../../ui/theme/ap_colors.dart';
import '../../ui/theme/ap_spacing.dart';
import '../../ui/theme/ap_typography.dart';
import '../sessions/session_process_tree.dart';
import '../sessions/waterfall_timeline.dart';

class RunDetailPage extends StatefulWidget {
  const RunDetailPage({
    super.key,
    required this.apiClient,
    required this.run,
    required this.onBack,
  });
  final LocalApiClient apiClient;
  final Map<String, dynamic> run;
  final VoidCallback onBack;
  @override
  State<RunDetailPage> createState() => _RunDetailPageState();
}

class _RunDetailPageState extends State<RunDetailPage>
    with SingleTickerProviderStateMixin {
  late final TabController _tabs = TabController(length: 5, vsync: this);
  List<Map<String, dynamic>> _timeline = [];
  List<Map<String, dynamic>> _processes = [];
  Map<String, dynamic> _analysis = {};
  Map<String, dynamic>? _selectedRecord;
  Object? _error;
  bool _loading = true;
  bool _inspectorOpen = true;
  int _generation = 0;

  String get _id => widget.run['id'] as String? ?? '';
  String get _titleId {
    final external = widget.run['external_id'] as String?;
    return external?.isNotEmpty == true ? external! : _id;
  }

  @override
  void initState() {
    super.initState();
    _load();
  }

  @override
  void didUpdateWidget(covariant RunDetailPage oldWidget) {
    super.didUpdateWidget(oldWidget);
    if (oldWidget.run['id'] != widget.run['id']) _load();
  }

  @override
  void dispose() {
    _generation++;
    _tabs.dispose();
    super.dispose();
  }

  Future<void> _load() async {
    final generation = ++_generation;
    setState(() {
      _loading = true;
      _error = null;
    });
    final endpoint = '/api/v1/sessions/${Uri.encodeComponent(_id)}';
    try {
      final values = await Future.wait([
        widget.apiClient.get('$endpoint/timeline?limit=500'),
        widget.apiClient.get('$endpoint/analysis'),
        widget.apiClient.get('$endpoint/processes'),
      ]);
      if (!mounted || generation != _generation) return;
      setState(() {
        _timeline = _items(values[0]['items']);
        _analysis = values[1];
        _processes = _items(values[2]['items']);
        _loading = false;
      });
    } catch (error) {
      if (mounted && generation == _generation) {
        setState(() {
          _loading = false;
          _error = error;
        });
      }
    }
  }

  List<Map<String, dynamic>> _items(Object? raw) => ((raw as List?) ?? const [])
      .whereType<Map>()
      .map((v) => v.cast<String, dynamic>())
      .toList();

  @override
  Widget build(BuildContext context) {
    final durations =
        (_analysis['durations'] as Map?)?.cast<String, dynamic>() ??
        const <String, dynamic>{};
    final tokens =
        (_analysis['tokens'] as Map?)?.cast<String, dynamic>() ??
        const <String, dynamic>{};
    final ttft =
        (tokens['ttft_ms'] as Map?)?.cast<String, dynamic>() ??
        const <String, dynamic>{};
    final requestRate =
        (tokens['request_rate'] as Map?)?.cast<String, dynamic>() ??
        const <String, dynamic>{};
    final duration = widget.run['activity_duration_ms'];
    return Column(
      children: [
        Container(
          height: 56,
          padding: const EdgeInsets.symmetric(horizontal: 20),
          decoration: const BoxDecoration(
            color: ApColors.surface0,
            border: Border(bottom: BorderSide(color: ApColors.borderMuted)),
          ),
          child: Row(
            children: [
              IconButton(
                onPressed: widget.onBack,
                tooltip: 'Back to runs',
                icon: const Icon(Icons.arrow_back),
              ),
              const SizedBox(width: 4),
              Column(
                mainAxisAlignment: MainAxisAlignment.center,
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text('Run $_titleId', style: ApTypography.sectionTitle),
                  Text(
                    '${widget.run['model'] ?? 'Unknown model'} · Codex · ${_analysis['data_quality'] ?? widget.run['data_quality'] ?? 'partial'} data',
                    style: ApTypography.micro,
                  ),
                ],
              ),
              const Spacer(),
              const ApStatusBadge(label: 'OBSERVED', tone: ApTone.info),
              const SizedBox(width: 12),
              Text(apDuration(duration), style: ApTypography.code),
              IconButton(
                onPressed: _loading ? null : _load,
                tooltip: 'Refresh run',
                icon: const Icon(Icons.refresh, size: 17),
              ),
            ],
          ),
        ),
        SizedBox(
          height: 86,
          child: Padding(
            padding: const EdgeInsets.fromLTRB(20, 12, 20, 10),
            child: LayoutBuilder(
              builder: (context, c) {
                final count = c.maxWidth < 760 ? 3 : 6;
                return GridView.count(
                  crossAxisCount: count,
                  crossAxisSpacing: 8,
                  physics: const NeverScrollableScrollPhysics(),
                  childAspectRatio: count == 3 ? 2.8 : 1.9,
                  children: [
                    ApMetricTile(
                      label: 'Duration',
                      value: apDuration(duration),
                    ),
                    ApMetricTile(
                      label: 'LLM only',
                      value: _durationNs(durations['llm_only']),
                      quality: _metricQuality(durations['llm_only']),
                    ),
                    ApMetricTile(
                      label: 'Command only',
                      value: _durationNs(durations['command_only']),
                      quality: _metricQuality(durations['command_only']),
                    ),
                    ApMetricTile(
                      label: 'Input',
                      value: apCompactNumber(_analysis['input_tokens']),
                      unit: 'tok',
                    ),
                    ApMetricTile(
                      label: 'Output',
                      value: apCompactNumber(_analysis['output_tokens']),
                      unit: 'tok',
                    ),
                    ApMetricTile(
                      label: 'TTFT',
                      value: ttft['value'] is num
                          ? apDuration(ttft['value'])
                          : '—',
                      quality: _metricQuality(ttft),
                    ),
                  ],
                );
              },
            ),
          ),
        ),
        Container(
          height: 42,
          alignment: Alignment.centerLeft,
          padding: const EdgeInsets.symmetric(horizontal: 18),
          decoration: const BoxDecoration(
            border: Border(
              top: BorderSide(color: ApColors.borderMuted),
              bottom: BorderSide(color: ApColors.borderMuted),
            ),
          ),
          child: TabBar(
            controller: _tabs,
            isScrollable: true,
            tabAlignment: TabAlignment.start,
            tabs: const [
              Tab(text: 'Timeline'),
              Tab(text: 'Model'),
              Tab(text: 'Tools'),
              Tab(text: 'Processes'),
              Tab(text: 'Data Quality'),
            ],
            labelStyle: ApTypography.small.copyWith(
              fontWeight: FontWeight.w600,
            ),
            unselectedLabelStyle: ApTypography.small,
            indicatorSize: TabBarIndicatorSize.label,
          ),
        ),
        if (_loading) const LinearProgressIndicator(minHeight: 2),
        if (_error != null)
          Padding(
            padding: const EdgeInsets.all(12),
            child: ApErrorState(
              message: 'Unable to load run analysis: $_error',
              onRetry: _load,
            ),
          ),
        Expanded(
          child: _loading || _error != null
              ? const SizedBox.shrink()
              : TabBarView(
                  controller: _tabs,
                  children: [
                    _timelineTab(),
                    _modelTab(durations, tokens, ttft, requestRate),
                    _toolsTab(),
                    _processTab(),
                    _qualityTab(),
                  ],
                ),
        ),
      ],
    );
  }

  Widget _timelineTab() => Padding(
    padding: const EdgeInsets.all(ApSpacing.md),
    child: ApSplitView(
      primary: ApPanel(
        title: 'EXECUTION TIMELINE',
        trailing: Row(
          mainAxisSize: MainAxisSize.min,
          children: [
            Text('${_timeline.length} records', style: ApTypography.micro),
            const SizedBox(width: 8),
            IconButton(
              onPressed: () => setState(() => _inspectorOpen = !_inspectorOpen),
              tooltip: _inspectorOpen ? 'Hide inspector' : 'Show inspector',
              visualDensity: VisualDensity.compact,
              padding: EdgeInsets.zero,
              constraints: const BoxConstraints.tightFor(width: 28, height: 28),
              icon: Icon(
                _inspectorOpen ? Icons.visibility_off : Icons.visibility,
                size: 16,
              ),
            ),
          ],
        ),
        padding: const EdgeInsets.all(10),
        expandChild: true,
        child: _timeline.isEmpty
            ? const ApEmptyState(
                title: 'No timeline records',
                message: 'This run has no timestamped spans or events.',
              )
            : WaterfallTimeline(
                records: _timeline,
                showDetails: false,
                onSelect: (record) => setState(() => _selectedRecord = record),
              ),
      ),
      secondary: _recordInspector(_selectedRecord),
      showSecondary: _inspectorOpen,
    ),
  );

  Widget _recordInspector(Map<String, dynamic>? record) => ApPanel(
    title: 'INSPECTOR',
    padding: const EdgeInsets.all(14),
    child: record == null
        ? const SizedBox(
            height: 300,
            child: ApEmptyState(
              title: 'Select a span',
              message: 'Choose a timeline row to inspect its timing, status and correlation.',
            ),
          )
        : Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(
                record['tool'] as String? ??
                    record['model'] as String? ??
                    record['kind'] as String? ??
                    'Activity',
                style: ApTypography.pageTitle,
              ),
              const SizedBox(height: 8),
              ApStatusBadge(
                label: '${record['outcome'] ?? 'OBSERVED'}'.toUpperCase(),
                tone: _recordFailed(record)
                    ? ApTone.danger
                    : _recordSucceeded(record)
                    ? ApTone.success
                    : ApTone.neutral,
              ),
              const SizedBox(height: 16),
              _detail('Kind', '${record['kind'] ?? 'event'}'),
              _detail('Duration', apDuration(record['duration_ms'])),
              _detail(
                'Started',
                '${record['start_at'] ?? record['timestamp'] ?? '—'}',
              ),
              _detail('Span ID', '${record['span_id'] ?? '—'}'),
              _detail('Tool call ID', '${record['tool_call_id'] ?? '—'}'),
              _detail('Process PID', '${record['process_pid'] ?? '—'}'),
              _detail('Correlation', '${record['correlation_method'] ?? '—'}'),
              const SizedBox(height: 8),
              ApQualityBadge(
                quality: ApQualityBadge.fromValue(record['data_quality']),
              ),
            ],
          ),
  );

  Widget _modelTab(
    Map<String, dynamic> durations,
    Map<String, dynamic> tokens,
    Map<String, dynamic> ttft,
    Map<String, dynamic> requestRate,
  ) {
    final wall = _nsValue((durations['wall_clock'] as Map?)?['value_ns']);
    final llm = _nsValue((durations['llm_only'] as Map?)?['value_ns']);
    final command = _nsValue((durations['command_only'] as Map?)?['value_ns']);
    final overlap = _nsValue((durations['overlap'] as Map?)?['value_ns']);
    final unattributed = _nsValue(
      (durations['unattributed'] as Map?)?['value_ns'],
    );
    final bars = [
      ('LLM only', llm, ApColors.model),
      ('Command only', command, ApColors.command),
      ('Overlap', overlap, ApColors.tool),
      ('Unattributed', unattributed, ApColors.inferred),
    ];
    return ListView(
      padding: const EdgeInsets.all(ApSpacing.page),
      children: [
        ApPanel(
          title: 'TIME BREAKDOWN',
          trailing: Text(
            'Total observed ${apDuration(wall == null ? null : wall / 1000000)}',
            style: ApTypography.code,
          ),
          child: Column(
            children: [
              for (final (label, value, color) in bars)
                _breakdownRow(label, value, wall, color),
              const SizedBox(height: 8),
              const Text(
                'LLM and command intervals can overlap. Values are unavailable if either interval source is missing.',
                style: ApTypography.micro,
              ),
            ],
          ),
        ),
        const SizedBox(height: 12),
        Wrap(
          spacing: 8,
          runSpacing: 8,
          children: [
            ApMetricTile(
              label: 'Input tokens',
              value: apCompactNumber(_analysis['input_tokens']),
              quality: _analysis['input_tokens'] == null
                  ? ApQuality.unavailable
                  : ApQuality.partial,
            ),
            ApMetricTile(
              label: 'Output tokens',
              value: apCompactNumber(_analysis['output_tokens']),
              quality: _analysis['output_tokens'] == null
                  ? ApQuality.unavailable
                  : ApQuality.partial,
            ),
            ApMetricTile(
              label: 'Reasoning',
              value: apCompactNumber(_analysis['reasoning_tokens']),
              quality: _analysis['reasoning_tokens'] == null
                  ? ApQuality.unavailable
                  : ApQuality.partial,
            ),
            ApMetricTile(
              label: 'TTFT',
              value: ttft['value'] is num ? apDuration(ttft['value']) : '—',
              quality: _metricQuality(ttft),
            ),
            ApMetricTile(
              label: 'Request rate',
              value: requestRate['tokens_per_second'] is num
                  ? (requestRate['tokens_per_second'] as num).toStringAsFixed(1)
                  : '—',
              unit: 'tok/s',
              quality: _metricQuality(requestRate),
            ),
            ApMetricTile(
              label: 'Generation rate',
              value:
                  ((tokens['generation_rate'] as Map?)?['tokens_per_second']
                      is num)
                  ? ((tokens['generation_rate'] as Map)['tokens_per_second']
                            as num)
                        .toStringAsFixed(1)
                  : '—',
              unit: 'tok/s',
              quality: _metricQuality(tokens['generation_rate']),
            ),
          ],
        ),
      ],
    );
  }

  Widget _breakdownRow(String label, num? valueNS, num? wallNS, Color color) {
    final ratio = valueNS == null || wallNS == null || wallNS <= 0
        ? 0.0
        : (valueNS / wallNS).clamp(0.0, 1.0).toDouble();
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 7),
      child: Row(
        children: [
          SizedBox(width: 130, child: Text(label, style: ApTypography.small)),
          Expanded(
            child: ClipRRect(
              borderRadius: BorderRadius.circular(3),
              child: LinearProgressIndicator(
                value: ratio,
                minHeight: 12,
                color: color,
                backgroundColor: ApColors.surface3,
              ),
            ),
          ),
          const SizedBox(width: 12),
          SizedBox(
            width: 100,
            child: Text(
              valueNS == null ? '—' : apDuration(valueNS / 1000000),
              textAlign: TextAlign.end,
              style: ApTypography.code,
            ),
          ),
          SizedBox(
            width: 56,
            child: Text(
              valueNS == null || wallNS == null || wallNS == 0
                  ? '—'
                  : '${(ratio * 100).round()}%',
              textAlign: TextAlign.end,
              style: ApTypography.micro,
            ),
          ),
        ],
      ),
    );
  }

  Widget _toolsTab() {
    final tools = _items(_analysis['tools']);
    final calls = _timeline
        .where(
          (r) =>
              '${r['kind']}'.toLowerCase().contains('tool') ||
              '${r['tool'] ?? ''}'.isNotEmpty,
        )
        .toList();
    return ListView(
      padding: const EdgeInsets.all(ApSpacing.page),
      children: [
        Wrap(
          spacing: 8,
          children: [
            ApMetricTile(
              label: 'Calls',
              value: apCompactNumber(
                tools.fold<num>(
                  0,
                  (s, x) => s + ((x['total_calls'] as num?) ?? 0),
                ),
              ),
            ),
            ApMetricTile(
              label: 'Failed',
              value: apCompactNumber(
                tools.fold<num>(
                  0,
                  (s, x) => s + ((x['failure_calls'] as num?) ?? 0),
                ),
              ),
              tint: ApColors.danger,
            ),
            ApMetricTile(label: 'Visible calls', value: '${calls.length}'),
          ],
        ),
        const SizedBox(height: 12),
        ApPanel(
          title: 'TOOL SUMMARY',
          padding: const EdgeInsets.fromLTRB(0, 0, 0, 6),
          child: tools.isEmpty
              ? const SizedBox(
                  height: 180,
                  child: ApEmptyState(
                    title: 'No tool calls recorded for this run',
                  ),
                )
              : Column(
                  children: [
                    Container(
                      height: 32,
                      color: ApColors.surface2,
                      padding: const EdgeInsets.symmetric(horizontal: 14),
                      child: const Row(
                        children: [
                          Expanded(flex: 2, child: Text('TOOL')),
                          Expanded(child: Text('CALLS')),
                          Expanded(child: Text('FAILED')),
                          Expanded(child: Text('FAIL RATE')),
                          Expanded(child: Text('P50')),
                          Expanded(child: Text('P95')),
                        ],
                      ),
                    ),
                    for (final tool in tools)
                      Container(
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
                                '${tool['tool_name'] ?? 'unknown'}',
                                style: ApTypography.small,
                              ),
                            ),
                            Expanded(
                              child: Text(
                                '${tool['total_calls'] ?? 0}',
                                style: ApTypography.small,
                              ),
                            ),
                            Expanded(
                              child: Text(
                                '${tool['failure_calls'] ?? 0}',
                                style: ApTypography.small,
                              ),
                            ),
                            Expanded(
                              child: Text(
                                tool['failure_rate'] is num
                                    ? '${((tool['failure_rate'] as num) * 100).toStringAsFixed(1)}%'
                                    : '—',
                                style: ApTypography.small,
                              ),
                            ),
                            Expanded(
                              child: Text(
                                apDuration(tool['p50_duration_ms']),
                                style: ApTypography.small,
                              ),
                            ),
                            Expanded(
                              child: Text(
                                apDuration(tool['p95_duration_ms']),
                                style: ApTypography.small,
                              ),
                            ),
                          ],
                        ),
                      ),
                  ],
                ),
        ),
        const SizedBox(height: 12),
        ApPanel(
          title: 'TOOL EVENTS',
          padding: const EdgeInsets.fromLTRB(0, 0, 0, 5),
          child: calls.isEmpty
              ? const SizedBox(
                  height: 100,
                  child: ApEmptyState(title: 'No timestamped tool records'),
                )
              : Column(
                  children: [
                    for (final call in calls)
                      InkWell(
                        onTap: () => setState(() => _selectedRecord = call),
                        child: ListTile(
                          dense: true,
                          title: Text(
                            '${call['tool'] ?? call['kind'] ?? 'tool'}',
                          ),
                          subtitle: Text(
                            '${call['start_at'] ?? call['timestamp'] ?? '—'} · ${apDuration(call['duration_ms'])}',
                          ),
                          trailing: ApQualityBadge(
                            quality: ApQualityBadge.fromValue(
                              call['data_quality'],
                            ),
                          ),
                        ),
                      ),
                  ],
                ),
        ),
      ],
    );
  }

  Widget _processTab() => ListView(
    padding: const EdgeInsets.all(ApSpacing.page),
    children: [
      Wrap(
        spacing: 8,
        children: [
          ApMetricTile(label: 'Processes', value: '${_processes.length}'),
          ApMetricTile(
            label: 'CPU time',
            value: apDuration(
              _processes.fold<num>(
                0,
                (s, p) => s + ((p['cpu_time_ms'] as num?) ?? 0),
              ),
            ),
          ),
          ApMetricTile(
            label: 'Peak memory',
            value: _memory(
              _processes
                  .map((p) => p['working_set_bytes'])
                  .whereType<num>()
                  .fold<num?>(null, (a, b) => a == null || b > a ? b : a),
            ),
          ),
        ],
      ),
      const SizedBox(height: 12),
      ApPanel(
        title: 'PROCESS TREE',
        child: _processes.isEmpty
            ? const SizedBox(
                height: 180,
                child: ApEmptyState(
                  title: 'No correlated processes',
                  message: 'Process monitoring may be disabled or no process was correlated with this run.',
                ),
              )
            : SessionProcessTree(processes: _processes),
      ),
      const SizedBox(height: 12),
      ApPanel(
        title: 'PROCESS DETAILS',
        padding: const EdgeInsets.fromLTRB(0, 0, 0, 5),
        child: Column(
          children: [
            for (final p in _processes)
              ListTile(
                dense: true,
                leading: const Icon(Icons.memory_outlined),
                title: Text(
                  '${p['image_name'] ?? 'process'} · PID ${p['pid'] ?? '—'}',
                ),
                subtitle: Text(
                  'CPU ${apDuration(p['cpu_time_ms'])} · ${_memory(p['working_set_bytes'])} · ${p['association_method'] ?? 'unassigned'}',
                ),
                trailing: ApQualityBadge(
                  quality: ApQualityBadge.fromValue(p['sample_quality']),
                ),
              ),
          ],
        ),
      ),
    ],
  );

  Widget _qualityTab() {
    final eventIds = (_analysis['source_event_ids'] as List?) ?? const [];
    return ListView(
      padding: const EdgeInsets.all(ApSpacing.page),
      children: [
        Wrap(
          spacing: 8,
          children: [
            ApMetricTile(
              label: 'Run quality',
              value: '${_analysis['data_quality'] ?? 'partial'}'.toUpperCase(),
              quality: ApQualityBadge.fromValue(_analysis['data_quality']),
            ),
            ApMetricTile(
              label: 'Source events',
              value: '${eventIds.length}',
              quality: _analysis['source_event_ids_truncated'] == true
                  ? ApQuality.partial
                  : ApQuality.complete,
            ),
            ApMetricTile(
              label: 'Timeline records',
              value: '${_timeline.length}',
            ),
          ],
        ),
        const SizedBox(height: 12),
        ApPanel(
          title: 'DATA QUALITY',
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              _qualityLine('Session association', _analysis['data_quality']),
              _qualityLine(
                'Activity window',
                (_analysis['durations'] as Map?)?['wall_clock'] is Map
                    ? ((_analysis['durations'] as Map)['wall_clock']
                          as Map)['quality']
                    : 'unavailable',
              ),
              _qualityLine(
                'Token rate',
                (_analysis['tokens'] as Map?)?['request_rate'] is Map
                    ? ((_analysis['tokens'] as Map)['request_rate']
                          as Map)['quality']
                    : 'unavailable',
              ),
              _qualityLine(
                'TTFT',
                (_analysis['tokens'] as Map?)?['ttft_ms'] is Map
                    ? ((_analysis['tokens'] as Map)['ttft_ms']
                          as Map)['quality']
                    : 'unavailable',
              ),
              const SizedBox(height: 10),
              Text(
                'Data Quality describes what can be supported by collected telemetry. Missing values are left unavailable instead of being fabricated.',
                style: ApTypography.small.copyWith(color: ApColors.textMuted),
              ),
            ],
          ),
        ),
      ],
    );
  }

  Widget _qualityLine(String label, Object? quality) => Padding(
    padding: const EdgeInsets.symmetric(vertical: 7),
    child: Row(
      children: [
        Expanded(child: Text(label, style: ApTypography.body)),
        ApQualityBadge(quality: ApQualityBadge.fromValue(quality)),
      ],
    ),
  );
  Widget _detail(String label, String value) => Padding(
    padding: const EdgeInsets.only(bottom: 10),
    child: Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(label.toUpperCase(), style: ApTypography.micro),
        const SizedBox(height: 3),
        SelectableText(value, style: ApTypography.small),
      ],
    ),
  );
  num? _nsValue(Object? value) => value is num ? value : null;
  String _durationNs(Object? metric) {
    final value = (metric as Map?)?['value_ns'];
    return value is num ? apDuration(value / 1000000) : '—';
  }

  ApQuality _metricQuality(Object? metric) =>
      ApQualityBadge.fromValue((metric as Map?)?['quality']);
  bool _recordFailed(Map<String, dynamic> record) {
    final outcome = '${record['outcome'] ?? ''}'.toLowerCase();
    return outcome.contains('fail') || outcome.contains('error');
  }

  bool _recordSucceeded(Map<String, dynamic> record) {
    final outcome = '${record['outcome'] ?? ''}'.toLowerCase();
    return outcome.contains('success') || outcome.contains('complete');
  }

  String _memory(Object? value) =>
      value is num ? '${(value / 1024 / 1024).toStringAsFixed(1)} MB' : '—';
}
