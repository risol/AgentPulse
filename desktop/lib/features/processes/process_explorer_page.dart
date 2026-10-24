import 'package:flutter/material.dart';

import '../../ui/components/ap_components.dart';
import '../../ui/theme/ap_colors.dart';
import '../../ui/theme/ap_spacing.dart';
import '../../ui/theme/ap_typography.dart';

class ProcessExplorerPage extends StatefulWidget {
  const ProcessExplorerPage({
    super.key,
    required this.status,
    required this.processes,
    required this.loading,
    required this.loadingMore,
    required this.nextCursor,
    this.error,
    required this.onRefresh,
    required this.onLoadMore,
  });
  final Map<String, dynamic> status;
  final List<Map<String, dynamic>> processes;
  final bool loading;
  final bool loadingMore;
  final String? nextCursor;
  final String? error;
  final VoidCallback onRefresh;
  final VoidCallback onLoadMore;
  @override
  State<ProcessExplorerPage> createState() => _ProcessExplorerPageState();
}

class _ProcessExplorerPageState extends State<ProcessExplorerPage> {
  Map<String, dynamic>? _selected;
  String _filter = '';
  List<Map<String, dynamic>> get _visible => widget.processes
      .where(
        (p) =>
            _filter.isEmpty ||
            '${p['image_name']} ${p['pid']} ${p['command_category'] ?? ''}'
                .toLowerCase()
                .contains(_filter.toLowerCase()),
      )
      .toList();
  int get _active =>
      widget.processes.where((p) => p['ended_at'] == null).length;
  String _memory(Object? value) =>
      value is num ? '${(value / 1024 / 1024).toStringAsFixed(1)} MB' : '—';
  String _time(Object? value) {
    if (value is! String) return '—';
    final d = DateTime.tryParse(value)?.toLocal();
    if (d == null) return '—';
    return '${d.hour.toString().padLeft(2, '0')}:${d.minute.toString().padLeft(2, '0')}:${d.second.toString().padLeft(2, '0')}';
  }

  @override
  Widget build(BuildContext context) {
    final cpu = widget.processes.fold<num>(
      0,
      (s, p) => s + ((p['cpu_time_ms'] as num?) ?? 0),
    );
    final peak = widget.processes
        .map((p) => p['working_set_bytes'])
        .whereType<num>()
        .fold<num?>(null, (a, b) => a == null || b > a ? b : a);
    final incomplete = widget.processes
        .where((p) => p['sample_quality'] != 'complete')
        .length;
    return ListView(
      padding: const EdgeInsets.all(ApSpacing.page),
      children: [
        Row(
          children: [
            const Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text('Processes', style: ApTypography.pageTitle),
                  SizedBox(height: 3),
                  Text(
                    'Process tree and resource history linked to Codex activity',
                    style: ApTypography.small,
                  ),
                ],
              ),
            ),
            Text('$_active active', style: ApTypography.small),
            const SizedBox(width: 12),
            IconButton(
              onPressed: widget.loading ? null : widget.onRefresh,
              tooltip: 'Refresh',
              icon: const Icon(Icons.refresh),
            ),
          ],
        ),
        const SizedBox(height: 14),
        Wrap(
          spacing: 8,
          runSpacing: 8,
          children: [
            ApMetricTile(label: 'CPU time', value: apDuration(cpu)),
            ApMetricTile(label: 'Peak memory', value: _memory(peak)),
            ApMetricTile(
              label: 'Captured',
              value: '${widget.processes.length}',
            ),
            ApMetricTile(
              label: 'Incomplete',
              value: '$incomplete',
              tint: incomplete > 0 ? ApColors.warning : null,
            ),
          ],
        ),
        if (widget.error != null) ...[
          const SizedBox(height: 12),
          ApErrorState(message: widget.error!, onRetry: widget.onRefresh),
        ],
        const SizedBox(height: 12),
        LayoutBuilder(
          builder: (context, c) {
            final wide = c.maxWidth >= 1080;
            final list = _treePanel();
            final inspector = _inspector(_selected);
            if (wide) {
              return SizedBox(
                height: 430,
                child: ApSplitView(
                  primary: list,
                  secondary: inspector,
                  secondaryWidth: 320,
                ),
              );
            }
            return Column(
              children: [
                SizedBox(height: 350, child: list),
                const SizedBox(height: 10),
                inspector,
              ],
            );
          },
        ),
        const SizedBox(height: 12),
        ApPanel(
          title: 'PROCESS HISTORY',
          trailing: SizedBox(
            width: 220,
            height: 32,
            child: TextField(
              onChanged: (v) => setState(() => _filter = v),
              decoration: const InputDecoration(
                prefixIcon: Icon(Icons.search, size: 16),
                hintText: 'Filter by name or PID',
              ),
            ),
          ),
          padding: const EdgeInsets.fromLTRB(0, 0, 0, 5),
          child: Column(
            children: [
              if (widget.loading && _visible.isEmpty)
                const LinearProgressIndicator(),
              if (_visible.isEmpty && !widget.loading)
                const SizedBox(
                  height: 140,
                  child: ApEmptyState(
                    title: 'No process samples',
                    message: 'Process monitoring may be disabled or no matching processes were captured.',
                  ),
                ),
              if (_visible.isNotEmpty) ...[
                Container(
                  height: 31,
                  color: ApColors.surface2,
                  padding: const EdgeInsets.symmetric(horizontal: 12),
                  child: const Row(
                    children: [
                      Expanded(flex: 2, child: Text('PROCESS')),
                      Expanded(child: Text('PID / PARENT')),
                      Expanded(child: Text('STARTED')),
                      Expanded(child: Text('CPU TIME')),
                      Expanded(child: Text('MEMORY')),
                      Expanded(child: Text('CATEGORY')),
                      Expanded(child: Text('QUALITY')),
                    ],
                  ),
                ),
                for (final p in _visible)
                  InkWell(
                    onTap: () => setState(() => _selected = p),
                    child: Container(
                      height: 36,
                      padding: const EdgeInsets.symmetric(horizontal: 12),
                      decoration: BoxDecoration(
                        color: identical(_selected, p)
                            ? ApColors.accent.withValues(alpha: .1)
                            : null,
                        border: const Border(
                          bottom: BorderSide(color: ApColors.borderMuted),
                        ),
                      ),
                      child: Row(
                        children: [
                          Expanded(
                            flex: 2,
                            child: Text(
                              p['image_name'] as String? ?? 'process',
                              style: ApTypography.small,
                            ),
                          ),
                          Expanded(
                            child: Text(
                              '${p['pid'] ?? '—'} / ${p['parent_pid'] ?? '—'}',
                              style: ApTypography.code,
                            ),
                          ),
                          Expanded(
                            child: Text(
                              _time(p['started_at']),
                              style: ApTypography.code,
                            ),
                          ),
                          Expanded(
                            child: Text(
                              apDuration(p['cpu_time_ms']),
                              style: ApTypography.small,
                            ),
                          ),
                          Expanded(
                            child: Text(
                              _memory(p['working_set_bytes']),
                              style: ApTypography.small,
                            ),
                          ),
                          Expanded(
                            child: Text(
                              '${p['command_category'] ?? '—'}',
                              style: ApTypography.small,
                            ),
                          ),
                          Expanded(
                            child: ApQualityBadge(
                              quality: ApQualityBadge.fromValue(
                                p['sample_quality'],
                              ),
                            ),
                          ),
                        ],
                      ),
                    ),
                  ),
                if (widget.nextCursor != null)
                  Padding(
                    padding: const EdgeInsets.all(9),
                    child: OutlinedButton(
                      onPressed: widget.loadingMore ? null : widget.onLoadMore,
                      child: Text(
                        widget.loadingMore ? 'Loading…' : 'Load more processes',
                      ),
                    ),
                  ),
              ],
            ],
          ),
        ),
        const SizedBox(height: 10),
        Text(
          'Resource values are snapshots. Correlation uses explicit PID when available and otherwise shows the backend correlation method and confidence.',
          style: ApTypography.micro,
        ),
      ],
    );
  }

  Widget _treePanel() => ApPanel(
    title: 'PROCESS TREE',
    trailing: ApStatusBadge(
      label: '${widget.status['state'] ?? 'disabled'}'.toUpperCase(),
      tone: widget.status['enabled'] == true ? ApTone.success : ApTone.warning,
    ),
    padding: const EdgeInsets.fromLTRB(0, 0, 0, 4),
    child: widget.processes.isEmpty
        ? const SizedBox(
            height: 250,
            child: ApEmptyState(
              title: 'No process tree',
              message: 'Enable process monitoring in Settings to capture process snapshots.',
            ),
          )
        : Column(
            children: [
              for (final p in widget.processes.take(80)) _treeRow(p),
              if (widget.processes.length > 80)
                Padding(
                  padding: const EdgeInsets.all(8),
                  child: Text(
                    '${widget.processes.length - 80} additional process records are available in history.',
                    style: ApTypography.micro,
                  ),
                ),
            ],
          ),
  );

  Widget _treeRow(Map<String, dynamic> p) {
    final pid = p['pid'];
    final parent = p['parent_pid'];
    final hasParent = widget.processes.any((x) => x['pid'] == parent);
    final child = hasParent && parent != pid;
    final active = p['ended_at'] == null;
    return InkWell(
      onTap: () => setState(() => _selected = p),
      child: Container(
        height: 35,
        padding: EdgeInsets.only(left: child ? 28 : 12, right: 12),
        color: identical(_selected, p)
            ? ApColors.accent.withValues(alpha: .1)
            : null,
        child: Row(
          children: [
            Icon(
              child ? Icons.subdirectory_arrow_right : Icons.circle,
              size: child ? 14 : 8,
              color: active ? ApColors.process : ApColors.textMuted,
            ),
            const SizedBox(width: 9),
            Expanded(
              child: Text(
                '${p['image_name'] ?? 'process'} · PID $pid',
                style: ApTypography.small,
              ),
            ),
            Text(apDuration(p['cpu_time_ms']), style: ApTypography.micro),
            const SizedBox(width: 12),
            Text(_memory(p['working_set_bytes']), style: ApTypography.micro),
          ],
        ),
      ),
    );
  }

  Widget _inspector(Map<String, dynamic>? p) => ApPanel(
    title: 'PROCESS INSPECTOR',
    padding: const EdgeInsets.all(14),
    child: p == null
        ? const SizedBox(
            height: 250,
            child: ApEmptyState(title: 'Select a process'),
          )
        : Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(
                '${p['image_name'] ?? 'process'}',
                style: ApTypography.pageTitle,
              ),
              const SizedBox(height: 8),
              ApStatusBadge(
                label: p['ended_at'] == null ? 'RUNNING' : 'EXITED',
                tone: p['ended_at'] == null ? ApTone.success : ApTone.neutral,
              ),
              const SizedBox(height: 16),
              _detail(
                'PID / Parent',
                '${p['pid'] ?? '—'} / ${p['parent_pid'] ?? '—'}',
              ),
              _detail('Started', p['started_at'] as String? ?? '—'),
              _detail('CPU time', apDuration(p['cpu_time_ms'])),
              _detail('Working set', _memory(p['working_set_bytes'])),
              _detail('Category', '${p['command_category'] ?? 'Unclassified'}'),
              _detail(
                'Associated tool',
                '${p['associated_tool_name'] ?? 'Unassigned'}',
              ),
              _detail('Correlation', '${p['association_method'] ?? '—'}'),
              const SizedBox(height: 8),
              ApQualityBadge(
                quality: ApQualityBadge.fromValue(p['sample_quality']),
              ),
              if ((p['executable_path'] as String? ?? '').isNotEmpty) ...[
                const SizedBox(height: 10),
                _detail('Executable path', p['executable_path'] as String),
              ],
              if ((p['command_line'] as String? ?? '').isNotEmpty) ...[
                const SizedBox(height: 4),
                _detail('Command line', p['command_line'] as String),
              ],
            ],
          ),
  );
  Widget _detail(String label, String value) => Padding(
    padding: const EdgeInsets.only(bottom: 9),
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
