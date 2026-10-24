import 'package:flutter/material.dart';

import '../../core/api/local_api_client.dart';
import '../../ui/components/ap_components.dart';
import '../../ui/theme/ap_colors.dart';
import '../../ui/theme/ap_spacing.dart';
import '../../ui/theme/ap_typography.dart';

class SessionExplorerPage extends StatefulWidget {
  const SessionExplorerPage({
    required this.apiClient,
    required this.onOpenSession,
    super.key,
  });

  final LocalApiClient? apiClient;
  final ValueChanged<Map<String, dynamic>> onOpenSession;

  @override
  State<SessionExplorerPage> createState() => _SessionExplorerPageState();
}

class _SessionExplorerPageState extends State<SessionExplorerPage> {
  final _modelController = TextEditingController();
  List<Map<String, dynamic>> _sessions = [];
  String _range = 'all';
  String? _cursor;
  String? _error;
  int _generation = 0;
  bool _loading = false;
  bool _loadingMore = false;
  String _sortKey = 'started_at';
  bool _sortAscending = false;

  @override
  void initState() {
    super.initState();
    _load();
  }

  @override
  void didUpdateWidget(covariant SessionExplorerPage oldWidget) {
    super.didUpdateWidget(oldWidget);
    if (!identical(oldWidget.apiClient, widget.apiClient)) _load();
  }

  @override
  void dispose() {
    _generation++;
    _modelController.dispose();
    super.dispose();
  }

  Future<void> _load({bool append = false}) async {
    final api = widget.apiClient;
    if (api == null) {
      setState(() {
        _sessions = [];
        _cursor = null;
        _error = '本地服务尚未就绪。';
        _loading = false;
        _loadingMore = false;
      });
      return;
    }
    final generation = ++_generation;
    final cursor = append ? _cursor : null;
    setState(() {
      if (append) {
        _loadingMore = true;
      } else {
        _loading = true;
        _sessions = [];
        _cursor = null;
      }
      _error = null;
    });

    final query = <String, String>{'range': _range, 'limit': '25'};
    final model = _modelController.text.trim();
    if (model.isNotEmpty) query['model'] = model;
    if (cursor != null) query['cursor'] = cursor;
    try {
      final response = await api.get(
        '/api/v1/sessions?${Uri(queryParameters: query).query}',
      );
      if (!mounted || generation != _generation) return;
      final items = ((response['items'] as List?) ?? const [])
          .map((value) => (value as Map).cast<String, dynamic>())
          .toList();
      setState(() {
        _sessions = append ? [..._sessions, ...items] : items;
        _cursor = response['next_cursor'] as String?;
      });
    } catch (error) {
      if (!mounted || generation != _generation) return;
      setState(() => _error = '读取会话失败：$error');
    } finally {
      if (mounted && generation == _generation) {
        setState(() {
          _loading = false;
          _loadingMore = false;
        });
      }
    }
  }

  void _selectRange(String? value) {
    if (value == null || value == _range) return;
    setState(() => _range = value);
    _load();
  }

  String _duration(dynamic value) {
    if (value is! num || value < 0) return '未知';
    if (value < 1000) return '${value.round()} 毫秒';
    final seconds = value / 1000;
    if (seconds < 60) return '${seconds.toStringAsFixed(1)} 秒';
    final minutes = seconds / 60;
    if (minutes < 60) return '${minutes.toStringAsFixed(1)} 分钟';
    return '${(minutes / 60).toStringAsFixed(1)} 小时';
  }

  String _time(dynamic value) {
    if (value is! String) return '时间未知';
    final parsed = DateTime.tryParse(value)?.toLocal();
    if (parsed == null) return '时间未知';
    String two(int number) => number.toString().padLeft(2, '0');
    return '${parsed.year}-${two(parsed.month)}-${two(parsed.day)} ${two(parsed.hour)}:${two(parsed.minute)}';
  }

  List<Map<String, dynamic>> get _sortedSessions {
    final values = [..._sessions];
    int compare(Map<String, dynamic> a, Map<String, dynamic> b) {
      final left = a[_sortKey];
      final right = b[_sortKey];
      int result;
      if (left is num && right is num) {
        result = left.compareTo(right);
      } else {
        result = '${left ?? ''}'.compareTo('${right ?? ''}');
      }
      return _sortAscending ? result : -result;
    }

    values.sort(compare);
    return values;
  }

  void _sort(String key, bool ascending) => setState(() {
    _sortKey = key;
    _sortAscending = ascending;
  });

  Widget _tableRow(Map<String, dynamic> session) {
    return Material(
      color: Colors.transparent,
      child: InkWell(
        onTap: () => widget.onOpenSession(session),
        child: Container(
          height: 38,
          padding: const EdgeInsets.symmetric(horizontal: 12),
          decoration: BoxDecoration(
            border: Border(bottom: BorderSide(color: ApColors.borderMuted)),
          ),
          child: Row(
            children: [
              SizedBox(
                width: 150,
                child: Text(
                  _time(session['started_at']),
                  style: ApTypography.code,
                ),
              ),
              SizedBox(
                width: 84,
                child: Text(
                  _duration(session['activity_duration_ms']),
                  style: ApTypography.small,
                ),
              ),
              SizedBox(
                width: 130,
                child: Text(
                  session['model'] as String? ?? '未知',
                  maxLines: 1,
                  overflow: TextOverflow.ellipsis,
                  style: ApTypography.small,
                ),
              ),
              SizedBox(
                width: 140,
                child: Text(
                  '${session['input_tokens'] ?? '—'} / ${session['output_tokens'] ?? '—'}',
                  style: ApTypography.small,
                ),
              ),
              SizedBox(
                width: 76,
                child: Text(
                  '${session['tool_call_count'] ?? 0}',
                  style: ApTypography.small,
                ),
              ),
              SizedBox(
                width: 76,
                child: Text(
                  '${session['command_count'] ?? 0}',
                  style: ApTypography.small,
                ),
              ),
              SizedBox(
                width: 70,
                child: Text(
                  '${session['failed_call_count'] ?? 0}',
                  style: ApTypography.small.copyWith(
                    color: ((session['failed_call_count'] as num?) ?? 0) > 0
                        ? ApColors.danger
                        : null,
                  ),
                ),
              ),
              SizedBox(
                width: 112,
                child: ApQualityBadge(
                  quality: ApQualityBadge.fromValue(session['data_quality']),
                ),
              ),
              IconButton(
                onPressed: () => widget.onOpenSession(session),
                tooltip: 'Open run',
                visualDensity: VisualDensity.compact,
                padding: EdgeInsets.zero,
                constraints: const BoxConstraints.tightFor(
                  width: 28,
                  height: 28,
                ),
                icon: const Icon(Icons.open_in_new, size: 15),
              ),
            ],
          ),
        ),
      ),
    );
  }

  @override
  Widget build(BuildContext context) => RefreshIndicator(
    onRefresh: _load,
    child: ListView(
      physics: const AlwaysScrollableScrollPhysics(),
      padding: const EdgeInsets.all(ApSpacing.page),
      children: [
        Row(
          children: [
            const Expanded(child: Text('Runs', style: ApTypography.pageTitle)),
            Text('${_sessions.length} loaded', style: ApTypography.small),
            IconButton(
              onPressed: _load,
              tooltip: 'Refresh runs',
              icon: const Icon(Icons.refresh),
            ),
          ],
        ),
        const SizedBox(height: 4),
        const Text(
          'Historical Codex executions · activity range reflects observed telemetry spans.',
          style: ApTypography.small,
        ),
        const SizedBox(height: 14),
        Wrap(
          spacing: 12,
          runSpacing: 10,
          crossAxisAlignment: WrapCrossAlignment.center,
          children: [
            SizedBox(
              width: 220,
              child: DropdownButtonFormField<String>(
                initialValue: _range,
                isExpanded: true,
                decoration: const InputDecoration(labelText: 'Time range'),
                items: const [
                  DropdownMenuItem(value: 'all', child: Text('All time')),
                  DropdownMenuItem(value: '24h', child: Text('Last 24 hours')),
                  DropdownMenuItem(value: '7d', child: Text('Last 7 days')),
                ],
                onChanged: _selectRange,
              ),
            ),
            SizedBox(
              width: 280,
              child: TextField(
                controller: _modelController,
                textInputAction: TextInputAction.search,
                onSubmitted: (_) => _load(),
                decoration: const InputDecoration(
                  labelText: 'Model contains',
                  hintText: 'e.g. gpt-5',
                  prefixIcon: Icon(Icons.search, size: 16),
                ),
              ),
            ),
            FilledButton.icon(
              onPressed: _loading ? null : _load,
              icon: const Icon(Icons.search),
              label: const Text('Filter'),
            ),
            TextButton(
              onPressed: () {
                setState(() {
                  _range = 'all';
                  _modelController.clear();
                });
                _load();
              },
              child: const Text('Reset'),
            ),
          ],
        ),
        const SizedBox(height: 8),
        const SizedBox(height: 12),
        if (_error != null)
          ApErrorState(message: _error!, onRetry: _loading ? null : _load),
        if (_loading && _sessions.isEmpty) const LinearProgressIndicator(),
        if (!_loading && _error == null && _sessions.isEmpty)
          const SizedBox(
            height: 280,
            child: ApEmptyState(
              title: 'No runs yet',
              message:
                  'AgentPulse is connected and waiting for Codex activity.',
            ),
          ),
        if (_sessions.isNotEmpty)
          Container(
            decoration: BoxDecoration(
              color: ApColors.surface1,
              border: Border.all(color: ApColors.borderMuted),
              borderRadius: BorderRadius.circular(8),
            ),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                Container(
                  height: 34,
                  padding: const EdgeInsets.symmetric(horizontal: 12),
                  decoration: const BoxDecoration(
                    color: ApColors.surface2,
                    borderRadius: BorderRadius.vertical(
                      top: Radius.circular(8),
                    ),
                  ),
                  child: Row(
                    children: [
                      _sortHeader('Started', 'started_at', 150),
                      _sortHeader('Duration', 'activity_duration_ms', 84),
                      _sortHeader('Model', 'model', 130),
                      _sortHeader('Input / Output', 'input_tokens', 140),
                      _sortHeader('Tools', 'tool_call_count', 76),
                      _sortHeader('Commands', 'command_count', 76),
                      _sortHeader('Failures', 'failed_call_count', 70),
                      const SizedBox(
                        width: 112,
                        child: Text('Quality', style: ApTypography.micro),
                      ),
                    ],
                  ),
                ),
                for (final session in _sortedSessions) _tableRow(session),
                Padding(
                  padding: const EdgeInsets.all(10),
                  child: Row(
                    children: [
                      Text(
                        'Select a run to inspect its execution details.',
                        style: ApTypography.micro,
                      ),
                      const Spacer(),
                      if (_cursor != null)
                        OutlinedButton.icon(
                          onPressed: _loadingMore
                              ? null
                              : () => _load(append: true),
                          icon: _loadingMore
                              ? const SizedBox(
                                  width: 14,
                                  height: 14,
                                  child: CircularProgressIndicator(
                                    strokeWidth: 2,
                                  ),
                                )
                              : const Icon(Icons.expand_more, size: 16),
                          label: Text(_loadingMore ? 'Loading' : 'Load more'),
                        ),
                    ],
                  ),
                ),
              ],
            ),
          ),
        if (_cursor != null) const SizedBox(height: 4),
      ],
    ),
  );

  Widget _sortHeader(String label, String key, double width) => SizedBox(
    width: width,
    child: InkWell(
      onTap: () => _sort(key, _sortKey == key ? !_sortAscending : false),
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
              _sortAscending ? Icons.arrow_upward : Icons.arrow_downward,
              size: 11,
              color: ApColors.accentStrong,
            ),
        ],
      ),
    ),
  );
}
